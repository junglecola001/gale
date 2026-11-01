package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gale/internal/model"
	"gale/internal/settings"
	"gale/internal/store"
	"gale/internal/testutil"
)

// harness is one engine on a fresh database, a download directory of its
// own, and a test server.
type harness struct {
	t    *testing.T
	e    *Engine
	st   *store.Store
	set  *settings.Store
	srv  *testutil.Server
	dir  string
	done chan model.Finished

	mu       sync.Mutex
	progress map[int64]model.Progress
	changed  []model.Download
}

func newHarness(t *testing.T, opts testutil.Options, tweak func(*settings.Data)) *harness {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "gale.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	set, err := settings.Load(st, settings.Defaults(filepath.Join(dir, "downloads")))
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if tweak != nil {
		d := set.Get()
		tweak(&d)
		if _, err := set.Update(d); err != nil {
			t.Fatalf("settings update: %v", err)
		}
	}
	h := &harness{
		t:        t,
		st:       st,
		set:      set,
		srv:      testutil.New(t, opts),
		dir:      dir,
		done:     make(chan model.Finished, 32),
		progress: make(map[int64]model.Progress),
	}
	h.e = New(st, set, Hooks{
		Progress: func(p model.Progress) {
			h.mu.Lock()
			h.progress[p.ID] = p
			h.mu.Unlock()
		},
		Changed: func(d model.Download) {
			h.mu.Lock()
			h.changed = append(h.changed, d)
			h.mu.Unlock()
		},
		Finished: func(f model.Finished) { h.done <- f },
	})
	if err := h.e.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		h.e.Shutdown()
		st.Close()
	})
	return h
}

func (h *harness) add(req model.AddRequest) model.Download {
	h.t.Helper()
	if req.URL == "" {
		req.URL = h.srv.URL("file.bin")
	}
	d, err := h.e.Add(context.Background(), req)
	if err != nil {
		h.t.Fatalf("add: %v", err)
	}
	return d
}

func (h *harness) waitFinished(t *testing.T, timeout time.Duration) model.Finished {
	t.Helper()
	select {
	case f := <-h.done:
		return f
	case <-time.After(timeout):
		t.Fatalf("download did not finish within %s", timeout)
		return model.Finished{}
	}
}

func (h *harness) get(id int64) model.Detail {
	h.t.Helper()
	detail, err := h.e.Get(id)
	if err != nil {
		h.t.Fatalf("get %d: %v", id, err)
	}
	return detail
}

// assertContent compares the downloaded file with what the server holds now.
func (h *harness) assertContent(id int64) {
	h.t.Helper()
	d := h.get(id).Download
	data, err := os.ReadFile(filepath.Join(d.Dir, d.FileName))
	if err != nil {
		h.t.Fatalf("read downloaded file: %v", err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != h.srv.H.SHA256() {
		h.t.Fatalf("content mismatch: %d bytes downloaded, got %s want %s", len(data), got, h.srv.H.SHA256())
	}
}

func TestDownloadMultiConnection(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 4 << 20}, nil)
	d := h.add(model.AddRequest{Connections: 4, Start: true})
	f := h.waitFinished(t, 30*time.Second)
	if f.Status != model.Completed {
		t.Fatalf("finished = %+v", f)
	}
	h.assertContent(d.ID)
	segments := h.get(d.ID).Segments
	if len(segments) < 4 {
		t.Fatalf("segments = %d, want at least 4", len(segments))
	}
	total := int64(0)
	for _, seg := range segments {
		if !seg.Done() {
			t.Fatalf("segment %+v not done", seg)
		}
		total += seg.End - seg.Start + 1
	}
	if total != 4<<20 {
		t.Fatalf("segments cover %d bytes, want %d", total, 4<<20)
	}
}

func TestPauseResume(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 4 << 20, Rate: 1 << 20}, nil)
	d := h.add(model.AddRequest{Connections: 4, Start: true})

	deadline := time.Now().Add(20 * time.Second)
	for {
		detail := h.get(d.ID)
		if detail.Download.Downloaded >= 1<<20 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no progress before the deadline")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := h.e.Pause(d.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	paused := h.get(d.ID)
	if paused.Download.Status != model.Paused {
		t.Fatalf("status after pause = %s", paused.Download.Status)
	}
	saved := paused.Download.Downloaded

	if err := h.e.Resume(d.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	f := h.waitFinished(t, 60*time.Second)
	if f.Status != model.Completed {
		t.Fatalf("finished = %+v", f)
	}
	if saved == 0 {
		t.Fatalf("nothing was saved before the pause")
	}
	h.assertContent(d.ID)
}

func TestRetryAfterConnectionDrop(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 4 << 20, DropAfter: 1 << 20}, nil)
	d := h.add(model.AddRequest{Connections: 1, Start: true})
	f := h.waitFinished(t, 60*time.Second)
	if f.Status != model.Completed {
		t.Fatalf("finished = %+v", f)
	}
	h.assertContent(d.ID)
}

func TestContentChangeRestartsOnce(t *testing.T) {
	h := newHarness(t, testutil.Options{
		Size:               8 << 20,
		DropAfter:          2 << 20,
		ETagChangeRequests: 2,
		ETagChanges:        1,
	}, nil)
	d := h.add(model.AddRequest{Connections: 1, Start: true})
	f := h.waitFinished(t, 120*time.Second)
	if f.Status != model.Completed {
		for _, line := range h.e.Logs(d.ID) {
			t.Log(line)
		}
		t.Fatalf("finished = %+v", f)
	}
	h.assertContent(d.ID)
}

func TestContentChangeTwiceFails(t *testing.T) {
	h := newHarness(t, testutil.Options{
		Size:               8 << 20,
		DropAfter:          2 << 20,
		DropEvery:          true,
		ETagChangeRequests: 2,
		ETagChanges:        2,
	}, nil)
	d := h.add(model.AddRequest{Connections: 1, Start: true})
	f := h.waitFinished(t, 120*time.Second)
	if f.Status != model.Failed || f.ErrorCode != CodeChanged {
		for _, line := range h.e.Logs(d.ID) {
			t.Log(line)
		}
		t.Fatalf("finished = %+v, want E_CHANGED", f)
	}
}

func TestNoRangeDownloadsSingleStream(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 1 << 20, NoRange: true}, nil)
	d := h.add(model.AddRequest{Connections: 8, Start: true})
	f := h.waitFinished(t, 60*time.Second)
	if f.Status != model.Completed {
		t.Fatalf("finished = %+v", f)
	}
	detail := h.get(d.ID)
	if len(detail.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(detail.Segments))
	}
	if detail.Download.Resumable {
		t.Fatalf("server without ranges marked resumable")
	}
	if peak := h.srv.H.PeakConcurrent(); peak > 2 {
		t.Fatalf("peak concurrency = %d, want a single connection", peak)
	}
	h.assertContent(d.ID)
}

func TestUnknownLengthCompletes(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 2 << 20, Chunked: true}, nil)
	d := h.add(model.AddRequest{Connections: 4, Start: true})
	f := h.waitFinished(t, 60*time.Second)
	if f.Status != model.Completed {
		t.Fatalf("finished = %+v", f)
	}
	got := h.get(d.ID)
	if got.Download.TotalBytes != 2<<20 || got.Download.Downloaded != 2<<20 {
		t.Fatalf("sizes = %d/%d", got.Download.Downloaded, got.Download.TotalBytes)
	}
	h.assertContent(d.ID)
}

func TestDynamicSplitReachesConnections(t *testing.T) {
	h := newHarness(t, testutil.Options{
		Size:       8 << 20,
		Rate:       4 << 20,
		DelayFirst: 300 * time.Millisecond,
	}, func(d *settings.Data) {
		d.MinSegmentBytes = 256 << 10
	})
	d := h.add(model.AddRequest{Connections: 8, Start: true})
	f := h.waitFinished(t, 120*time.Second)
	if f.Status != model.Completed {
		t.Fatalf("finished = %+v", f)
	}
	segments := h.get(d.ID).Segments
	if len(segments) != 8 {
		t.Fatalf("segments = %d, want the 8 the connections asked for", len(segments))
	}
	if peak := h.srv.H.PeakConcurrent(); peak != 8 {
		t.Fatalf("peak concurrency = %d, want 8", peak)
	}
	h.assertContent(d.ID)
}

func TestSpeedLimit(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 8 << 20}, func(d *settings.Data) {
		d.SpeedLimit = 2 << 20
	})
	start := time.Now()
	d := h.add(model.AddRequest{Connections: 4, Start: true})
	f := h.waitFinished(t, 60*time.Second)
	elapsed := time.Since(start)
	if f.Status != model.Completed {
		t.Fatalf("finished = %+v", f)
	}
	if elapsed < 3*time.Second || elapsed > 5*time.Second {
		t.Fatalf("8 MiB at 2 MiB/s took %s, want about 4s", elapsed)
	}
	h.assertContent(d.ID)
}

func TestAddQuickProbesOnStart(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 3 << 20, Name: "quick.iso"}, nil)
	id, err := h.e.AddQuick(model.AddRequest{URL: h.srv.URL("ignored")})
	if err != nil {
		t.Fatalf("add quick: %v", err)
	}
	f := h.waitFinished(t, 60*time.Second)
	if f.Status != model.Completed {
		t.Fatalf("finished = %+v", f)
	}
	detail := h.get(id)
	if detail.Download.FileName != "quick.iso" {
		t.Fatalf("fileName = %q, want the server's", detail.Download.FileName)
	}
	if detail.Download.TotalBytes != 3<<20 {
		t.Fatalf("total = %d", detail.Download.TotalBytes)
	}
	h.assertContent(id)
}

func TestAddPausedAndResume(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 2 << 20}, nil)
	d := h.add(model.AddRequest{Connections: 2, Start: false})
	if d.Status != model.Paused {
		t.Fatalf("status = %s, want paused", d.Status)
	}
	select {
	case f := <-h.done:
		t.Fatalf("paused download finished: %+v", f)
	case <-time.After(300 * time.Millisecond):
	}
	if err := h.e.Resume(d.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	f := h.waitFinished(t, 60*time.Second)
	if f.Status != model.Completed {
		t.Fatalf("finished = %+v", f)
	}
	h.assertContent(d.ID)
}

func TestRemoveDeletesFiles(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 4 << 20, Rate: 256 << 10}, nil)
	d := h.add(model.AddRequest{Connections: 2, Start: true})
	time.Sleep(300 * time.Millisecond)
	if err := h.e.Remove(d.ID, true); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, _, err := h.st.Get(d.ID); err == nil {
		t.Fatalf("row still there after remove")
	}
	matches, _ := filepath.Glob(filepath.Join(h.dir, "downloads", ".gale", "*.part"))
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestDiskFullIsReported(t *testing.T) {
	// A petabyte is more than any disk here, so the space check at add time
	// refuses it.
	h := newHarness(t, testutil.Options{Size: 1 << 50}, nil)
	_, err := h.e.Add(context.Background(), model.AddRequest{URL: h.srv.URL("huge.bin"), Start: true})
	wantCode(t, err, CodeDiskFull)
}

func TestPreview(t *testing.T) {
	h := newHarness(t, testutil.Options{Size: 4096, Name: "preview.txt"}, nil)
	p, err := h.e.Preview(context.Background(), model.AddRequest{URL: h.srv.URL("x")})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if p.Size != 4096 || p.FileName != "preview.txt" || !p.Resumable {
		t.Fatalf("preview = %+v", p)
	}
	if _, err := h.e.Preview(context.Background(), model.AddRequest{URL: "ftp://x/y"}); err == nil {
		t.Fatalf("ftp URL was accepted")
	}
	var fe *Error
	if err := h.e.Pause(9999); !errors.As(err, &fe) && err == nil {
		t.Fatalf("pause of a missing download returned nil")
	}
}
