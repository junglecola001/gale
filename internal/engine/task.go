package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gale/internal/fsutil"
	"gale/internal/model"
	"gale/internal/netx"

	"golang.org/x/sys/windows"
)

const (
	progressEvery = 500 * time.Millisecond
	persistEvery  = time.Second
	stallTimeout  = 60 * time.Second
	idleWait      = 150 * time.Millisecond
	splitWait     = 300 * time.Millisecond
)

// errCanceled is what a download that was paused or removed returns: no
// error to show the user, the status tells the story.
var errCanceled = errors.New("engine: canceled")

// restartError asks the task to probe the file again and start over.
type restartError struct {
	why string
}

func (e *restartError) Error() string { return e.why }

// fetchError is one attempt's failure: whether to try again, and how soon.
type fetchError struct {
	err           error
	retry         bool
	after         time.Duration
	restart       bool
	rangeViolated bool
}

func (e *fetchError) Error() string { return e.err.Error() }
func (e *fetchError) Unwrap() error { return e.err }

// task is one download while it runs.
type task struct {
	e *Engine
	d model.Download

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// dlCtx is the context of the current download attempt: a restart
	// cancels it without touching the task's own context.
	dlCtx    context.Context
	dlCancel context.CancelFunc

	client *http.Client
	file   *os.File

	mu      sync.Mutex
	segs    []*segmentState
	nextIdx int
	err     error
	removed bool

	workCh chan *segmentState

	workers sync.WaitGroup

	speed atomic.Int64
	eta   atomic.Int64

	rangeViolated  bool
	autoResumeExit atomic.Bool
	filePath       string

	emitMu     sync.Mutex
	lastEmitAt time.Time
	lastEmit   int64
}

func (e *Engine) newTask(d model.Download) *task {
	ctx, cancel := context.WithCancel(context.Background())
	t := &task{
		e:      e,
		d:      d,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		client: &http.Client{Transport: e.tr},
		workCh: make(chan *segmentState, 64),
	}
	return t
}

// loadSegments fills the task from the store's cursors.
func (t *task) loadSegments() error {
	_, segs, err := t.e.st.Get(t.d.ID)
	if err != nil {
		return failf(CodeInternal, "读取下载记录失败: %v", err)
	}
	for _, seg := range segs {
		t.segs = append(t.segs, newSegmentState(seg))
		if seg.Index >= t.nextIdx {
			t.nextIdx = seg.Index + 1
		}
	}
	return nil
}

func (t *task) wait() {
	<-t.done
}

func (t *task) minSeg() int64 { return t.e.set.Get().MinSegmentBytes }

func (t *task) connections() int {
	n := t.d.Connections
	if n <= 0 {
		n = t.e.set.Get().Connections
	}
	return n
}

func (t *task) run() {
	defer close(t.done)
	defer func() {
		if r := recover(); r != nil {
			t.e.onTaskExit(t, failf(CodeInternal, "内部错误: %v", r))
		}
	}()
	t.e.logf(t.d.ID, "开始下载 %s", t.d.URL)
	t.e.onTaskExit(t, t.execute())
}

// execute probes, prepares and downloads, restarting once if the file
// changed while it was being fetched.
func (t *task) execute() error {
	restarts := 0
	for {
		if t.d.NeedsProbe {
			if err := t.probeRemote(); err != nil {
				return err
			}
		}
		if err := t.prepare(restarts > 0); err != nil {
			return err
		}
		if t.d.TotalBytes == 0 {
			return t.complete()
		}
		err := t.download()
		if err == nil {
			return t.complete()
		}
		var re *restartError
		if errors.As(err, &re) {
			restarts++
			if restarts > 1 {
				return failf(CodeChanged, "文件在下载过程中发生了变化，已停止（%s）", re.why)
			}
			t.e.logf(t.d.ID, "文件已变化，重新获取信息: %s", re.why)
			if err := t.probeRemote(); err != nil {
				return err
			}
			continue
		}
		return err
	}
}

// probeRemote asks the server about the file, then plans the segments.
func (t *task) probeRemote() error {
	before := t.d.TotalBytes
	quick := t.d.NeedsProbe
	res, err := probe(t.ctx, t.e.tr, ProbeRequest{
		URL:       t.d.URL,
		Referrer:  t.d.Referrer,
		UserAgent: t.d.UserAgent,
		Cookies:   t.d.Cookies,
	})
	if err != nil {
		return err
	}
	t.d.FinalURL = res.FinalURL
	if t.d.FileName == "" {
		t.d.FileName = res.FileName
	}
	t.d.TotalBytes = res.Size
	t.d.ETag = res.ETag
	t.d.LastModified = res.LastModified
	t.d.MIME = res.MIME
	t.d.Resumable = res.Resumable
	if t.d.Category == "" {
		t.d.Category = categoryFor(t.d.FileName)
	}
	if quick {
		// Added by the browser before we knew what the file was: now that
		// we do, it goes to its category's directory and under its own
		// name, unless the name came with the link.
		if set := t.e.set.Get(); set.Categorize {
			t.d.Dir = filepath.Join(set.DownloadDir, categoryDir(t.d.Category))
		}
	}
	t.d.NeedsProbe = false
	t.e.logf(t.d.ID, "探测结果 %s: %d 字节, 可续传 %v", t.d.FileName, t.d.TotalBytes, t.d.Resumable)

	if t.d.TotalBytes > 0 {
		if free, err := fsutil.FreeSpace(t.d.Dir); err == nil && free < uint64(t.d.TotalBytes)+(64<<20) {
			return failf(CodeDiskFull, "磁盘剩余空间不足")
		}
	}
	if t.rangeViolated && before == t.d.TotalBytes && len(t.segs) > 0 {
		// The server says our cursors are past the end of an unchanged
		// file: every byte is already there.
		t.mu.Lock()
		for _, seg := range t.segs {
			if end := seg.end.Load(); end >= 0 {
				seg.pos.Store(end + 1)
			}
		}
		t.mu.Unlock()
	}
	return nil
}

// prepare opens the temporary file and gives the task its segments.
func (t *task) prepare(reset bool) error {
	galeDir := filepath.Join(t.d.Dir, ".gale")
	if err := fsutil.EnsureDir(galeDir); err != nil {
		return failf(CodeDisk, "无法创建目录 %s: %v", galeDir, err)
	}
	if t.d.TempPath == "" {
		t.d.TempPath = filepath.Join(galeDir, fmt.Sprintf("%d.part", t.d.ID))
	}
	if t.file == nil {
		f, err := os.OpenFile(fsutil.LongPath(t.d.TempPath), os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return failf(CodeDisk, "无法打开临时文件: %v", err)
		}
		t.file = f
	}
	if total := t.d.TotalBytes; total > 0 {
		info, err := t.file.Stat()
		if err != nil {
			return failf(CodeDisk, "无法读取临时文件: %v", err)
		}
		if info.Size() != total {
			if err := t.file.Truncate(total); err != nil {
				return diskError(err)
			}
			reset = true
		}
	}
	if !t.d.Resumable {
		reset = true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if reset || len(t.segs) == 0 {
		t.replan()
	}
	return nil
}

// replan lays out the segments afresh from the current cursor nothing: a
// download that starts over. Called with t.mu held.
func (t *task) replan() {
	total := t.d.TotalBytes
	t.segs = nil
	t.nextIdx = 0
	if total == 0 {
		return
	}
	if total < 0 {
		seg := newSegmentState(model.Segment{Index: 0, Start: 0, End: -1, Pos: 0})
		t.segs = []*segmentState{seg}
		t.nextIdx = 1
		return
	}
	plan := planSegments(total, int64(t.connections()), t.minSeg())
	t.segs = make([]*segmentState, 0, len(plan))
	t.nextIdx = len(plan)
	for _, seg := range plan {
		t.segs = append(t.segs, newSegmentState(seg))
	}
}

// download runs the workers until every segment is done or one fails.
func (t *task) download() error {
	t.mu.Lock()
	t.err = nil
	for _, seg := range t.segs {
		if seg.done() {
			continue
		}
		t.workCh <- seg
	}
	workers := targetConnections(t.d.TotalBytes, t.connections(), t.minSeg())
	t.mu.Unlock()

	t.dlCtx, t.dlCancel = context.WithCancel(t.ctx)
	defer t.dlCancel()

	stop := make(chan struct{})
	go t.tick(stop)

	t.workers.Add(workers)
	for range workers {
		go t.worker()
	}
	t.workers.Wait()
	close(stop)

	t.mu.Lock()
	err := t.err
	t.mu.Unlock()
	if errors.As(err, new(*restartError)) {
		return err
	}
	if t.ctx.Err() != nil {
		return errCanceled
	}
	if err != nil {
		return err
	}
	if !t.allDone() {
		return failf(CodeInternal, "分段没有全部完成")
	}
	return nil
}

// tick reports progress and persists the cursors while the download runs.
func (t *task) tick(stop <-chan struct{}) {
	progress := time.NewTicker(progressEvery)
	persist := time.NewTicker(persistEvery)
	defer progress.Stop()
	defer persist.Stop()
	for {
		select {
		case <-progress.C:
			t.emitProgress()
		case <-persist.C:
			t.persist()
		case <-stop:
			t.emitProgress()
			t.persist()
			return
		case <-t.ctx.Done():
			t.emitProgress()
			t.persist()
			return
		}
	}
}

func (t *task) persist() {
	if err := t.e.st.SaveProgress(t.d.ID, t.downloadedBytes(), t.segmentModels()); err != nil {
		t.e.logf(t.d.ID, "保存进度失败: %v", err)
	}
}

func (t *task) emitProgress() {
	now := time.Now()
	downloaded := t.downloadedBytes()
	total := t.d.TotalBytes

	t.emitMu.Lock()
	var speed int64
	if !t.lastEmitAt.IsZero() {
		dt := now.Sub(t.lastEmitAt).Seconds()
		if dt > 0 {
			instant := int64(float64(downloaded-t.lastEmit) / dt)
			if instant < 0 {
				instant = 0
			}
			if t.speed.Load() == 0 {
				speed = instant
			} else {
				speed = int64(0.7*float64(t.speed.Load()) + 0.3*float64(instant))
			}
		}
	}
	t.lastEmitAt = now
	t.lastEmit = downloaded
	t.emitMu.Unlock()

	var eta int64 = -1
	if total > 0 && speed > 0 {
		eta = (total - downloaded) / speed
	}
	t.speed.Store(speed)
	t.eta.Store(eta)
	t.e.hooks.Progress(model.Progress{ID: t.d.ID, Downloaded: downloaded, Total: total, Speed: speed, ETA: eta})
}

func (t *task) worker() {
	defer t.workers.Done()
	defer func() {
		if r := recover(); r != nil {
			t.abort(failf(CodeInternal, "内部错误: %v", r))
		}
	}()
	for {
		seg := t.takeWork()
		if seg == nil {
			return
		}
		if err := t.downloadSegment(seg); err != nil {
			t.abort(err)
			return
		}
	}
}

// abort records the first failure and stops the other workers of this
// attempt.
func (t *task) abort(err error) {
	if errors.Is(err, errCanceled) || t.ctx.Err() != nil {
		if t.dlCancel != nil {
			t.dlCancel()
		}
		return
	}
	t.mu.Lock()
	if t.err == nil {
		t.err = err
	}
	t.mu.Unlock()
	if t.dlCancel != nil {
		t.dlCancel()
	}
}

// takeWork hands the worker a segment: one from the queue, else the tail of
// the largest running segment, else nothing once the task is done.
func (t *task) takeWork() *segmentState {
	for {
		select {
		case seg := <-t.workCh:
			t.markTaken(seg)
			return seg
		default:
		}
		if seg, ok := t.splitLargest(); ok {
			return seg
		}
		if !t.anyRemaining() {
			return nil
		}
		select {
		case seg := <-t.workCh:
			t.markTaken(seg)
			return seg
		case <-time.After(idleWait):
		case <-t.attemptCtx().Done():
			return nil
		}
	}
}

func (t *task) markTaken(seg *segmentState) {
	t.mu.Lock()
	seg.taken = true
	t.mu.Unlock()
}

func (t *task) anyRemaining() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, seg := range t.segs {
		if !seg.done() {
			return true
		}
	}
	return false
}

// splitLargest asks the biggest running segment for its tail. The segment's
// worker hands one over at its next chunk, or the request is dropped.
func (t *task) splitLargest() (*segmentState, bool) {
	minSeg := t.minSeg()
	t.mu.Lock()
	var best *segmentState
	var bestRemaining int64
	for _, seg := range t.segs {
		if !seg.taken || seg.done() {
			continue
		}
		if rem := seg.remaining(); rem > bestRemaining {
			best, bestRemaining = seg, rem
		}
	}
	if best == nil || bestRemaining <= 2*minSeg || !best.splitReq.CompareAndSwap(false, true) {
		t.mu.Unlock()
		return nil, false
	}
	t.mu.Unlock()

	select {
	case tail := <-best.tailCh:
		t.mu.Lock()
		tail.index = t.nextIdx
		t.nextIdx++
		t.segs = append(t.segs, tail)
		tail.taken = true
		t.mu.Unlock()
		t.e.logf(t.d.ID, "切分分段 #%d: %d-%d", tail.index, tail.start, tail.end.Load())
		return tail, true
	case <-time.After(splitWait):
		best.splitReq.Store(false)
		return nil, false
	case <-t.attemptCtx().Done():
		best.splitReq.Store(false)
		return nil, false
	}
}

// handleSplit is the owner's side of a split: it hands the tail of its
// range to the worker that asked for it and carries on with the first half.
func (t *task) handleSplit(seg *segmentState) {
	if !seg.splitReq.Load() {
		return
	}
	pos := seg.pos.Load()
	end := seg.end.Load()
	if end < 0 || end-pos+1 <= 2*t.minSeg() {
		seg.splitReq.Store(false)
		return
	}
	remaining := end - pos + 1
	cut := pos + remaining/2
	tail := &segmentState{start: cut, tailCh: make(chan *segmentState)}
	tail.end.Store(end)
	tail.pos.Store(cut)
	select {
	case seg.tailCh <- tail:
		seg.end.Store(cut - 1)
		seg.splitReq.Store(false)
	case <-time.After(splitWait):
		seg.splitReq.Store(false)
	case <-t.attemptCtx().Done():
	}
}

// downloadSegment fetches a segment, retrying what is worth retrying.
func (t *task) downloadSegment(seg *segmentState) error {
	retries := 0
	maxRetries := t.e.set.Get().MaxRetries
	for {
		err := t.fetch(seg)
		if err == nil {
			return nil
		}
		var fe *fetchError
		if !errors.As(err, &fe) {
			return err
		}
		if fe.restart {
			t.rangeViolated = fe.rangeViolated
			return &restartError{why: fe.err.Error()}
		}
		if !fe.retry {
			return fe.err
		}
		retries++
		if retries > maxRetries {
			return fe.err
		}
		wait := retryBackoff(retries)
		if fe.after > wait {
			wait = fe.after
		}
		t.e.logf(t.d.ID, "分段 #%d 第 %d 次重试: %v", seg.index, retries, fe.err)
		select {
		case <-t.attemptCtx().Done():
			return errCanceled
		case <-time.After(wait):
		}
	}
}

// retryBackoff is 1, 2, 4, 8, 16, 30, 30… seconds with a little jitter.
func retryBackoff(attempt int) time.Duration {
	steps := [...]int{1, 2, 4, 8, 16, 30}
	seconds := steps[min(attempt-1, len(steps)-1)]
	return time.Duration(seconds)*time.Second + time.Duration(rand.IntN(500))*time.Millisecond
}

// fetch makes one ranged request and writes what it reads.
func (t *task) fetch(seg *segmentState) error {
	pos := seg.pos.Load()
	end := seg.end.Load()
	if end >= 0 && pos > end {
		return nil
	}

	target := t.d.FinalURL
	if target == "" {
		target = t.d.URL
	}
	reqCtx, cancel := context.WithCancel(t.attemptCtx())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		return &fetchError{err: failf(CodeHTTP, "无效的链接: %s", target)}
	}
	ua := t.d.UserAgent
	if ua == "" {
		ua = netx.UserAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Range", netx.RangeHeader(pos, end))
	if v := t.validator(); v != "" {
		req.Header.Set("If-Range", v)
	}
	if t.d.Referrer != "" {
		req.Header.Set("Referer", t.d.Referrer)
	}
	if t.d.Cookies != "" {
		req.Header.Set("Cookie", t.d.Cookies)
	}

	var lastRead atomic.Int64
	lastRead.Store(time.Now().UnixNano())
	stalled := make(chan struct{})
	go func() {
		defer close(stalled)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-reqCtx.Done():
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastRead.Load())) > stallTimeout {
					t.e.logf(t.d.ID, "分段 #%d 停滞超过 %s，重连", seg.index, stallTimeout)
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-stalled
	}()

	resp, err := t.client.Do(req)
	if err != nil {
		if t.ctx.Err() != nil {
			return errCanceled
		}
		if reqCtx.Err() != nil {
			return &fetchError{err: failf(CodeTimeout, "连接停滞，重试"), retry: true}
		}
		return &fetchError{err: classifyTransportError(reqCtx, err), retry: true}
	}
	defer resp.Body.Close()
	lastRead.Store(time.Now().UnixNano())
	t.e.logf(t.d.ID, "分段 #%d 请求 %d-%d → HTTP %d %s", seg.index, pos, end, resp.StatusCode, resp.Header.Get("Content-Range"))

	if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		return &fetchError{err: failf(CodeEncoding, "服务器返回了压缩内容 (%s)", enc)}
	}

	switch code := resp.StatusCode; {
	case code == http.StatusPartialContent:
		start, _, total, err := netx.ParseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			return &fetchError{err: failf(CodeRange, "服务器返回了无法理解的分段信息")}
		}
		if start != pos {
			return &fetchError{restart: true, err: failf(CodeChanged, "服务器从 %d 而非 %d 开始响应", start, pos)}
		}
		if total >= 0 {
			if t.d.TotalBytes >= 0 && total != t.d.TotalBytes {
				return &fetchError{restart: true, err: failf(CodeChanged, "文件大小从 %d 变成了 %d", t.d.TotalBytes, total)}
			}
			if t.d.TotalBytes < 0 {
				t.d.TotalBytes = total
				seg.end.Store(total - 1)
			}
		}
	case code == http.StatusOK:
		if t.d.Resumable || pos != 0 || seg.start != 0 {
			return &fetchError{restart: true, err: failf(CodeChanged, "服务器忽略了 Range 请求")}
		}
	case code == http.StatusRequestedRangeNotSatisfiable:
		return &fetchError{restart: true, rangeViolated: true, err: failf(CodeRange, "服务器拒绝了 %d-%d", pos, end)}
	case code == http.StatusUnauthorized, code == http.StatusProxyAuthRequired:
		return &fetchError{err: failf(CodeAuth, "需要登录或授权 (HTTP %d)", code)}
	case code == http.StatusTooManyRequests:
		return &fetchError{err: failf(CodeRetry, "服务器限流 (HTTP 429)"), retry: true, after: retryAfter(resp)}
	case code == http.StatusRequestTimeout, code == http.StatusTooEarly:
		return &fetchError{err: failf(CodeRetry, "服务器要求稍后重试 (HTTP %d)", code), retry: true}
	case code >= 500:
		return &fetchError{err: failf(CodeRetry, "服务器错误 (HTTP %d)", code), retry: true}
	default:
		return &fetchError{err: failf(CodeHTTP, "下载失败 (HTTP %d)", code)}
	}

	buf := make([]byte, chunkSize)
	for {
		if t.ctx.Err() != nil {
			return errCanceled
		}
		t.handleSplit(seg)
		end := seg.end.Load()
		if end >= 0 && pos > end {
			return nil
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			lastRead.Store(time.Now().UnixNano())
			if err := t.e.acquire(t.ctx, n); err != nil {
				return errCanceled
			}
			if _, werr := t.file.WriteAt(buf[:n], pos); werr != nil {
				return &fetchError{err: diskError(werr)}
			}
			pos += int64(n)
			seg.pos.Store(pos)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				if seg.end.Load() < 0 {
					seg.end.Store(pos - 1)
					if t.d.TotalBytes < 0 {
						t.d.TotalBytes = pos
					}
					return nil
				}
				if end := seg.end.Load(); pos > end {
					return nil
				}
				return &fetchError{err: failf(CodeNet, "连接提前结束"), retry: true}
			}
			if t.ctx.Err() != nil {
				return errCanceled
			}
			if reqCtx.Err() != nil {
				return &fetchError{err: failf(CodeTimeout, "连接停滞，重试"), retry: true}
			}
			return &fetchError{err: classifyTransportError(reqCtx, rerr), retry: true}
		}
		if end >= 0 && pos > end {
			return nil
		}
	}
}

func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(v); err == nil {
		d := time.Duration(seconds) * time.Second
		if d > 120*time.Second {
			d = 120 * time.Second
		}
		return d
	}
	if at, err := http.ParseTime(v); err == nil {
		if d := time.Until(at); d > 0 && d <= 120*time.Second {
			return d
		}
	}
	return 0
}

func diskError(err error) error {
	if errors.Is(err, windows.ERROR_DISK_FULL) || errors.Is(err, windows.ERROR_HANDLE_DISK_FULL) {
		return failf(CodeDiskFull, "磁盘空间不足")
	}
	return failf(CodeDisk, "写入文件失败: %v", err)
}

// validator is what If-Range compares: a strong ETag, else Last-Modified.
func (t *task) validator() string {
	if t.d.ETag != "" && !isWeakETag(t.d.ETag) {
		return t.d.ETag
	}
	return t.d.LastModified
}

func isWeakETag(v string) bool {
	return len(v) > 1 && (v[0] == 'W' || v[0] == 'w') && v[1] == '/'
}

func (t *task) allDone() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, seg := range t.segs {
		if !seg.done() {
			return false
		}
	}
	return true
}

func (t *task) segmentModels() []model.Segment {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]model.Segment, 0, len(t.segs))
	for _, seg := range t.segs {
		out = append(out, seg.model())
	}
	return out
}

func (t *task) downloadedBytes() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	var sum int64
	for _, seg := range t.segs {
		sum += seg.downloaded()
	}
	return sum
}

// complete flushes the temporary file and gives it its final name.
func (t *task) complete() error {
	if t.file != nil {
		if err := t.file.Sync(); err != nil {
			return diskError(err)
		}
		if t.d.TotalBytes > 0 {
			if err := t.file.Truncate(t.d.TotalBytes); err != nil {
				return diskError(err)
			}
		}
		if err := t.file.Close(); err != nil {
			return diskError(err)
		}
		t.file = nil
	}

	t.e.renameMu.Lock()
	defer t.e.renameMu.Unlock()
	target, err := fsutil.UniquePath(t.d.Dir, t.d.FileName)
	if err != nil {
		return failf(CodeRename, "无法确定文件名: %v", err)
	}
	var renameErr error
	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(200 * time.Millisecond)
		}
		renameErr = os.Rename(fsutil.LongPath(t.d.TempPath), fsutil.LongPath(target))
		if renameErr == nil {
			break
		}
	}
	if renameErr != nil {
		return failf(CodeRename, "无法重命名文件: %v", renameErr)
	}
	t.d.FileName = filepath.Base(target)
	t.filePath = target
	t.d.Downloaded = t.d.TotalBytes
	if t.d.Downloaded < 0 {
		t.d.Downloaded = t.downloadedBytes()
	}
	t.e.logf(t.d.ID, "下载完成: %s", target)
	t.d.TempPath = ""
	return nil
}

// attemptCtx is the current download attempt's context, the task's own
// before one started.
func (t *task) attemptCtx() context.Context {
	if t.dlCtx != nil {
		return t.dlCtx
	}
	return t.ctx
}
