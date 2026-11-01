// Package engine downloads files: several connections per file, segments
// that split as connections idle, cursors in SQLite so a crash costs a
// second, and a global rate limit.
package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gale/internal/fsutil"
	"gale/internal/logx"
	"gale/internal/model"
	"gale/internal/netx"
	"gale/internal/settings"
	"gale/internal/store"

	"golang.org/x/time/rate"
)

// Hooks are what the engine tells the app: progress many times a second,
// status changes, the end of a download, and the totals.
type Hooks struct {
	Progress func(model.Progress)
	Changed  func(model.Download)
	Finished func(model.Finished)
	Stats    func(model.Stats)
}

// Engine runs the downloads.
type Engine struct {
	st    *store.Store
	set   *settings.Store
	tr    *http.Transport
	hooks Hooks

	limiter  atomic.Pointer[rate.Limiter]
	renameMu sync.Mutex

	mu       sync.Mutex
	tasks    map[int64]*task
	slots    int
	active   int
	stopping bool
	pumping  bool

	logMu sync.Mutex
	logs  map[int64]*ring

	statsStop chan struct{}
	stopOnce  sync.Once
}

// New returns an engine on st, reading its settings from set.
func New(st *store.Store, set *settings.Store, hooks Hooks) *Engine {
	e := &Engine{
		st:        st,
		set:       set,
		tr:        netx.NewTransport(),
		hooks:     hooks,
		tasks:     make(map[int64]*task),
		logs:      make(map[int64]*ring),
		slots:     set.Get().MaxConcurrent,
		statsStop: make(chan struct{}),
	}
	if l := set.Get().SpeedLimit; l > 0 {
		e.limiter.Store(rate.NewLimiter(rate.Limit(l), chunkSize))
	}
	return e
}

// Start marks the downloads a previous run interrupted, resumes the ones
// that asked for it, and starts what was queued.
func (e *Engine) Start() error {
	if err := e.st.MarkInterrupted(); err != nil {
		return err
	}
	if e.set.Get().AutoResume {
		ids, err := e.st.AutoResumeIDs()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := e.Resume(id); err != nil {
				logx.Logger().Warn("resume failed", "id", id, "error", err)
			}
		}
	}
	go e.reportStats()
	e.pump()
	return nil
}

// Shutdown stops every download, saving each cursor so the next start
// resumes them.
func (e *Engine) Shutdown() {
	e.stopOnce.Do(e.shutdown)
}

func (e *Engine) shutdown() {
	e.mu.Lock()
	e.stopping = true
	running := make([]*task, 0, len(e.tasks))
	for _, t := range e.tasks {
		t.autoResumeExit.Store(true)
		running = append(running, t)
	}
	e.mu.Unlock()
	close(e.statsStop)
	for _, t := range running {
		t.cancel()
	}
	deadline := time.After(2 * time.Second)
	for _, t := range running {
		select {
		case <-t.done:
		case <-deadline:
			return
		}
	}
}

// SetMaxConcurrent changes how many downloads run at once.
func (e *Engine) SetMaxConcurrent(n int) {
	if n < 1 {
		n = 1
	}
	e.mu.Lock()
	e.slots = n
	e.mu.Unlock()
	e.pump()
}

// SetSpeedLimit changes the global limit, in bytes per second; 0 removes it.
func (e *Engine) SetSpeedLimit(bytesPerSec int64) {
	if bytesPerSec <= 0 {
		e.limiter.Store(nil)
		return
	}
	e.limiter.Store(rate.NewLimiter(rate.Limit(bytesPerSec), chunkSize))
}

// acquire waits for the rate limiter to allow n more bytes.
func (e *Engine) acquire(ctx context.Context, n int) error {
	lim := e.limiter.Load()
	if lim == nil {
		return nil
	}
	if n > lim.Burst() {
		n = lim.Burst()
	}
	return lim.WaitN(ctx, n)
}

// List returns every download, newest first.
func (e *Engine) List() ([]model.Download, error) { return e.st.List() }

// Get returns one download with its segments.
func (e *Engine) Get(id int64) (model.Detail, error) {
	d, segs, err := e.st.Get(id)
	if err != nil {
		return model.Detail{}, err
	}
	if t := e.task(id); t != nil {
		d = e.liveDownload(t)
		segs = t.segmentModels()
	}
	return model.Detail{Download: d, Segments: segs}, nil
}

// liveDownload is the row as the running task has it now.
func (e *Engine) liveDownload(t *task) model.Download {
	d := t.d
	d.Downloaded = t.downloadedBytes()
	d.Speed = t.speed.Load()
	if eta := t.eta.Load(); eta >= 0 {
		d.ETA = eta
	}
	return d
}

func (e *Engine) task(id int64) *task {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tasks[id]
}

// Preview asks the server about a URL without adding it.
func (e *Engine) Preview(ctx context.Context, req model.AddRequest) (model.Preview, error) {
	res, err := probe(ctx, e.tr, ProbeRequest{
		URL:       strings.TrimSpace(req.URL),
		Referrer:  req.Referer,
		UserAgent: req.UserAgent,
		Cookies:   req.Cookies,
	})
	if err != nil {
		return model.Preview{}, err
	}
	return res.Preview, nil
}

// Add probes the URL and stores the download, then starts it or leaves it
// paused, as the request says.
func (e *Engine) Add(ctx context.Context, req model.AddRequest) (model.Download, error) {
	u := strings.TrimSpace(req.URL)
	if u == "" {
		return model.Download{}, failf(CodeHTTP, "请填写下载链接")
	}
	res, err := probe(ctx, e.tr, ProbeRequest{
		URL:       u,
		Referrer:  req.Referer,
		UserAgent: req.UserAgent,
		Cookies:   req.Cookies,
	})
	if err != nil {
		return model.Download{}, err
	}

	name := req.FileName
	if name == "" {
		name = res.FileName
	}
	name = fsutil.SanitizeFilename(name)
	cat := req.Category
	if !validCategory(cat) {
		cat = categoryFor(name)
	}
	dir := req.Dir
	if dir == "" {
		dir = e.baseDir(cat)
	}
	if err := fsutil.EnsureDir(dir); err != nil {
		return model.Download{}, failf(CodeDisk, "无法创建目录 %s: %v", dir, err)
	}
	if err := fsutil.EnsureDir(filepath.Join(dir, ".gale")); err != nil {
		return model.Download{}, failf(CodeDisk, "无法创建目录 %s: %v", dir, err)
	}
	if res.Size > 0 {
		free, err := fsutil.FreeSpace(dir)
		if err == nil && free < uint64(res.Size)+(64<<20) {
			return model.Download{}, failf(CodeDiskFull, "磁盘剩余空间不足")
		}
	}

	conns := req.Connections
	if conns <= 0 {
		conns = e.set.Get().Connections
	}
	conns = min(max(conns, settings.MinConnections), settings.MaxConnections)

	now := time.Now().UnixMilli()
	d := model.Download{
		URL:          u,
		FinalURL:     res.FinalURL,
		FileName:     name,
		Dir:          dir,
		TotalBytes:   res.Size,
		ETag:         res.ETag,
		LastModified: res.LastModified,
		MIME:         res.MIME,
		Resumable:    res.Resumable,
		Connections:  conns,
		Category:     cat,
		CreatedAt:    now,
		Referrer:     req.Referer,
		UserAgent:    req.UserAgent,
		Cookies:      req.Cookies,
	}
	if req.Start {
		d.Status = model.Queued
	} else {
		d.Status = model.Paused
	}

	if res.Size == 0 {
		// Nothing to fetch: put the empty file where it belongs.
		return e.addEmpty(&d, name)
	}

	segs := planSegments(res.Size, int64(conns), e.set.Get().MinSegmentBytes)
	id, err := e.st.Insert(&d, segs)
	if err != nil {
		return model.Download{}, failf(CodeInternal, "保存下载记录失败: %v", err)
	}
	d.TempPath = filepath.Join(dir, ".gale", fmt.Sprintf("%d.part", id))
	if err := preallocate(d.TempPath, res.Size); err != nil {
		e.st.Delete(id)
		return model.Download{}, err
	}
	if err := e.st.Update(&d); err != nil {
		return model.Download{}, failf(CodeInternal, "保存下载记录失败: %v", err)
	}
	e.resetLog(d.ID)
	e.hooks.Changed(d)
	if req.Start {
		e.pump()
	}
	return d, nil
}

// addEmpty stores a download of a zero-byte file as completed.
func (e *Engine) addEmpty(d *model.Download, name string) (model.Download, error) {
	target, err := fsutil.UniquePath(d.Dir, name)
	if err != nil {
		return model.Download{}, failf(CodeRename, "无法确定文件名: %v", err)
	}
	f, err := os.OpenFile(fsutil.LongPath(target), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return model.Download{}, diskError(err)
	}
	f.Close()
	d.FileName = filepath.Base(target)
	d.Status = model.Completed
	d.FinishedAt = time.Now().UnixMilli()
	if _, err := e.st.Insert(d, nil); err != nil {
		return model.Download{}, failf(CodeInternal, "保存下载记录失败: %v", err)
	}
	e.hooks.Changed(*d)
	return *d, nil
}

// preallocate creates the temporary file at its final size, so that the
// writes of every segment land on allocated blocks. An unknown size (negative)
// only creates the file.
func preallocate(path string, size int64) error {
	f, err := os.OpenFile(fsutil.LongPath(path), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return diskError(err)
	}
	defer f.Close()
	if size < 0 {
		return nil
	}
	if err := f.Truncate(size); err != nil {
		os.Remove(fsutil.LongPath(path))
		return diskError(err)
	}
	return nil
}

// AddQuick stores a download without probing it, so that a browser handing
// a file over gets its answer at once. The task probes when it starts.
func (e *Engine) AddQuick(req model.AddRequest) (int64, error) {
	u := strings.TrimSpace(req.URL)
	if u == "" {
		return 0, failf(CodeHTTP, "请填写下载链接")
	}
	name := req.FileName
	if name != "" {
		name = fsutil.SanitizeFilename(name)
	}
	dir := req.Dir
	if dir == "" {
		dir = e.set.Get().DownloadDir
	}
	if err := fsutil.EnsureDir(filepath.Join(dir, ".gale")); err != nil {
		return 0, failf(CodeDisk, "无法创建目录 %s: %v", dir, err)
	}
	d := model.Download{
		URL:         u,
		FinalURL:    u,
		FileName:    name,
		Dir:         dir,
		TotalBytes:  -1,
		Status:      model.Queued,
		Connections: e.set.Get().Connections,
		Category:    categoryFor(name),
		CreatedAt:   time.Now().UnixMilli(),
		Referrer:    req.Referer,
		UserAgent:   req.UserAgent,
		Cookies:     req.Cookies,
		NeedsProbe:  true,
	}
	id, err := e.st.Insert(&d, nil)
	if err != nil {
		return 0, failf(CodeInternal, "保存下载记录失败: %v", err)
	}
	d.TempPath = filepath.Join(dir, ".gale", fmt.Sprintf("%d.part", id))
	if err := e.st.Update(&d); err != nil {
		return id, failf(CodeInternal, "保存下载记录失败: %v", err)
	}
	e.resetLog(id)
	e.hooks.Changed(d)
	e.pump()
	return id, nil
}

// baseDir is where a download of this category goes.
func (e *Engine) baseDir(category string) string {
	set := e.set.Get()
	if set.Categorize {
		return filepath.Join(set.DownloadDir, categoryDir(category))
	}
	return set.DownloadDir
}

// Pause stops a download and keeps its cursors for a later resume.
func (e *Engine) Pause(id int64) error {
	if t := e.task(id); t != nil {
		t.cancel()
		t.wait()
	}
	d, segs, err := e.st.Get(id)
	if err != nil {
		return err
	}
	if d.Status == model.Completed || d.Status == model.Failed {
		return nil
	}
	d.Status = model.Paused
	d.AutoResume = false
	d.Downloaded = sumSegments(segs)
	if err := e.st.Update(&d); err != nil {
		return err
	}
	e.hooks.Changed(d)
	e.pump()
	return nil
}

// Resume queues a paused or failed download.
func (e *Engine) Resume(id int64) error {
	d, _, err := e.st.Get(id)
	if err != nil {
		return err
	}
	switch d.Status {
	case model.Completed, model.Downloading, model.Queued:
		return nil
	}
	d.Status = model.Queued
	d.AutoResume = false
	d.ErrorCode, d.ErrorMessage = "", ""
	if err := e.st.Update(&d); err != nil {
		return err
	}
	e.hooks.Changed(d)
	e.pump()
	return nil
}

// Retry queues a failed download again, starting over when the failure
// makes the bytes on disk untrustworthy.
func (e *Engine) Retry(id int64) error {
	d, segs, err := e.st.Get(id)
	if err != nil {
		return err
	}
	if d.Status != model.Failed {
		return nil
	}
	if d.ErrorCode == CodeChanged || d.ErrorCode == CodeRange {
		for i := range segs {
			segs[i].Pos = segs[i].Start
		}
		if err := e.st.SaveProgress(id, 0, segs); err != nil {
			return err
		}
	}
	return e.Resume(id)
}

// Remove deletes a download, its temporary file and, if asked, the file it
// produced.
func (e *Engine) Remove(id int64, deleteFile bool) error {
	if t := e.task(id); t != nil {
		t.removed = true
		t.cancel()
		t.wait()
	}
	d, _, err := e.st.Get(id)
	if err == nil {
		if d.TempPath != "" {
			os.Remove(fsutil.LongPath(d.TempPath))
		}
		if deleteFile {
			os.Remove(fsutil.LongPath(filepath.Join(d.Dir, d.FileName)))
		}
	}
	e.logMu.Lock()
	delete(e.logs, id)
	e.logMu.Unlock()
	if err := e.st.Delete(id); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	e.pump()
	return nil
}

// RemoveMany removes several downloads.
func (e *Engine) RemoveMany(ids []int64, deleteFile bool) error {
	for _, id := range ids {
		if err := e.Remove(id, deleteFile); err != nil {
			return err
		}
	}
	return nil
}

// PauseAll stops everything that is running or waiting.
func (e *Engine) PauseAll() error {
	list, err := e.st.List()
	if err != nil {
		return err
	}
	for _, d := range list {
		if d.Status == model.Downloading || d.Status == model.Queued {
			if err := e.Pause(d.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// ResumeAll queues every paused download.
func (e *Engine) ResumeAll() error {
	list, err := e.st.List()
	if err != nil {
		return err
	}
	for _, d := range list {
		if d.Status == model.Paused {
			if err := e.Resume(d.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Stats is the engine at a glance.
func (e *Engine) Stats() model.Stats {
	e.mu.Lock()
	active := len(e.tasks)
	var speed int64
	for _, t := range e.tasks {
		speed += t.speed.Load()
	}
	e.mu.Unlock()
	queued, err := e.st.Count(model.Queued)
	if err != nil {
		queued = 0
	}
	return model.Stats{Active: active, Queued: queued, Speed: speed}
}

func (e *Engine) reportStats() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.statsStop:
			return
		case <-ticker.C:
			if e.hooks.Stats != nil {
				e.hooks.Stats(e.Stats())
			}
		}
	}
}

// Logs returns the tail of a download's log.
func (e *Engine) Logs(id int64) []string {
	e.logMu.Lock()
	r := e.logs[id]
	e.logMu.Unlock()
	if r == nil {
		return []string{}
	}
	return r.snapshot()
}

func (e *Engine) resetLog(id int64) {
	e.logMu.Lock()
	e.logs[id] = &ring{}
	e.logMu.Unlock()
}

func (e *Engine) logf(id int64, format string, args ...any) {
	line := time.Now().Format("15:04:05") + " " + fmt.Sprintf(format, args...)
	e.logMu.Lock()
	r := e.logs[id]
	if r == nil {
		r = &ring{}
		e.logs[id] = r
	}
	e.logMu.Unlock()
	r.add(line)
	logx.Logger().Info(line, "download", id)
}

// pump starts queued downloads while there is room.
func (e *Engine) pump() {
	e.mu.Lock()
	if e.pumping {
		e.mu.Unlock()
		return
	}
	e.pumping = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.pumping = false
		e.mu.Unlock()
	}()

	for {
		e.mu.Lock()
		if e.stopping || e.active >= e.slots {
			e.mu.Unlock()
			return
		}
		e.mu.Unlock()

		d, err := e.st.NextQueued()
		if err != nil || d == nil {
			return
		}
		e.mu.Lock()
		if _, busy := e.tasks[d.ID]; busy {
			e.mu.Unlock()
			return
		}
		e.active++
		e.mu.Unlock()

		d.Status = model.Downloading
		d.ErrorCode, d.ErrorMessage = "", ""
		if d.StartedAt == 0 {
			d.StartedAt = time.Now().UnixMilli()
		}
		if err := e.st.Update(d); err != nil {
			e.releaseSlot()
			logx.Logger().Warn("starting download failed", "id", d.ID, "error", err)
			return
		}
		t := e.newTask(*d)
		if err := t.loadSegments(); err != nil {
			e.releaseSlot()
			d.Status = model.Failed
			code, msg := codeOf(err)
			d.ErrorCode, d.ErrorMessage = code, msg
			e.st.Update(d)
			e.hooks.Changed(*d)
			return
		}
		e.mu.Lock()
		e.tasks[d.ID] = t
		e.mu.Unlock()
		e.hooks.Changed(*d)
		go t.run()
	}
}

func (e *Engine) releaseSlot() {
	e.mu.Lock()
	if e.active > 0 {
		e.active--
	}
	e.mu.Unlock()
}

// onTaskExit stores the task's last state and reports it.
func (e *Engine) onTaskExit(t *task, err error) {
	e.mu.Lock()
	delete(e.tasks, t.d.ID)
	if e.active > 0 {
		e.active--
	}
	removed := t.removed
	e.mu.Unlock()

	if t.file != nil {
		t.file.Close()
		t.file = nil
	}
	if removed {
		e.pump()
		return
	}

	downloaded := t.downloadedBytes()
	segs := t.segmentModels()
	if err := e.st.SaveProgress(t.d.ID, downloaded, segs); err != nil {
		logx.Logger().Warn("saving progress failed", "id", t.d.ID, "error", err)
	}

	switch {
	case err == nil:
		t.d.Status = model.Completed
		t.d.ErrorCode, t.d.ErrorMessage = "", ""
		t.d.FinishedAt = time.Now().UnixMilli()
		if t.d.Downloaded <= 0 {
			t.d.Downloaded = downloaded
		}
		if t.d.TotalBytes <= 0 {
			t.d.TotalBytes = downloaded
			t.d.Downloaded = downloaded
		}
		if uerr := e.st.Update(&t.d); uerr != nil {
			logx.Logger().Warn("saving download failed", "id", t.d.ID, "error", uerr)
		}
		t.e.hooks.Changed(t.d)
		e.hooks.Finished(model.Finished{
			ID:       t.d.ID,
			Status:   model.Completed,
			FilePath: filepath.Join(t.d.Dir, t.d.FileName),
		})
	case errors.Is(err, errCanceled):
		t.d.Status = model.Paused
		t.d.AutoResume = t.autoResumeExit.Load()
		t.d.Downloaded = downloaded
		if uerr := e.st.Update(&t.d); uerr != nil {
			logx.Logger().Warn("saving download failed", "id", t.d.ID, "error", uerr)
		}
		e.hooks.Changed(t.d)
	default:
		code, msg := codeOf(err)
		t.d.Status = model.Failed
		t.d.ErrorCode, t.d.ErrorMessage = code, msg
		t.d.Downloaded = downloaded
		t.d.AutoResume = false
		t.d.FinishedAt = time.Now().UnixMilli()
		e.logf(t.d.ID, "失败 [%s] %s", code, msg)
		if uerr := e.st.Update(&t.d); uerr != nil {
			logx.Logger().Warn("saving download failed", "id", t.d.ID, "error", uerr)
		}
		e.hooks.Changed(t.d)
		e.hooks.Finished(model.Finished{
			ID:           t.d.ID,
			Status:       model.Failed,
			ErrorCode:    code,
			ErrorMessage: msg,
		})
	}
	e.pump()
}

func sumSegments(segs []model.Segment) int64 {
	var sum int64
	for _, s := range segs {
		if n := s.Pos - s.Start; n > 0 {
			sum += n
		}
	}
	return sum
}

// ring is a download's log: the last maxLogLines lines.
type ring struct {
	mu    sync.Mutex
	lines []string
}

const maxLogLines = 200

func (r *ring) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
	if len(r.lines) > maxLogLines {
		r.lines = r.lines[len(r.lines)-maxLogLines:]
	}
}

func (r *ring) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.lines))
	copy(out, r.lines)
	return out
}
