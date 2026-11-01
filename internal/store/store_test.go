package store

import (
	"path/filepath"
	"testing"

	"gale/internal/model"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "gale.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sample(name string) *model.Download {
	return &model.Download{
		URL:          "https://example.com/" + name,
		FileName:     name,
		Dir:          `C:\Downloads`,
		TempPath:     `C:\Downloads\.gale\1.part`,
		TotalBytes:   1000,
		Downloaded:   0,
		Status:       model.Queued,
		Connections:  8,
		Category:     "other",
		CreatedAt:    1700000000000,
		Referrer:     "https://example.com",
		UserAgent:    "test-agent",
		Cookies:      "a=b",
		ETag:         `"abc"`,
		LastModified: "Mon, 01 Jan 2024 00:00:00 GMT",
		MIME:         "application/octet-stream",
	}
}

func TestInsertGetDeleteCascades(t *testing.T) {
	s := openTest(t)
	d := sample("a.bin")
	segs := []model.Segment{{Index: 0, Start: 0, End: 499, Pos: 0}, {Index: 1, Start: 500, End: 999, Pos: 500}}
	id, err := s.Insert(d, segs)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id == 0 || d.ID != id {
		t.Fatalf("id = %d, d.ID = %d", id, d.ID)
	}

	got, gotSegs, err := s.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FileName != d.FileName || got.URL != d.URL || got.Cookies != "a=b" || got.ETag != `"abc"` {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if !got.Resumable == false || got.TotalBytes != 1000 || got.Connections != 8 {
		t.Fatalf("scalars mismatch: %+v", got)
	}
	if len(gotSegs) != 2 || gotSegs[1].Start != 500 || gotSegs[1].End != 999 {
		t.Fatalf("segments = %+v", gotSegs)
	}

	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}

	if err := s.Delete(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM segments WHERE download_id = ?", id).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("segments left after delete: %d", n)
	}
}

func TestSaveProgressSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gale.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	d := sample("b.bin")
	id, err := s.Insert(d, []model.Segment{{Index: 0, Start: 0, End: 999, Pos: 0}})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	// A second segment appears through dynamic splitting, the first's cursor moves.
	segs := []model.Segment{
		{Index: 0, Start: 0, End: 399, Pos: 400},
		{Index: 1, Start: 400, End: 999, Pos: 700},
	}
	if err := s.SaveProgress(id, 700, segs); err != nil {
		t.Fatalf("save progress: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, gotSegs, err := s2.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Downloaded != 700 {
		t.Fatalf("downloaded = %d", got.Downloaded)
	}
	if len(gotSegs) != 2 || gotSegs[0].Pos != 400 || gotSegs[1].End != 999 || gotSegs[1].Pos != 700 {
		t.Fatalf("segments = %+v", gotSegs)
	}

	// Saving fewer segments drops the rest.
	if err := s2.SaveProgress(id, 1000, []model.Segment{{Index: 0, Start: 0, End: 999, Pos: 1000}}); err != nil {
		t.Fatalf("save progress 2: %v", err)
	}
	if _, gotSegs, _ := s2.Get(id); len(gotSegs) != 1 {
		t.Fatalf("segments after shrink = %+v", gotSegs)
	}
}

func TestStatusQueries(t *testing.T) {
	s := openTest(t)
	run := sample("run.bin")
	run.Status = model.Downloading
	idRun, _ := s.Insert(run, nil)
	queued := sample("q.bin")
	queued.Status = model.Queued
	queued.CreatedAt = 1700000000001
	idQueued, _ := s.Insert(queued, nil)
	paused := sample("p.bin")
	paused.Status = model.Paused
	if _, err := s.Insert(paused, nil); err != nil {
		t.Fatal(err)
	}

	if err := s.MarkInterrupted(); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	ids, err := s.AutoResumeIDs()
	if err != nil {
		t.Fatalf("auto resume ids: %v", err)
	}
	if len(ids) != 1 || ids[0] != idRun {
		t.Fatalf("auto resume ids = %v, want [%d]", ids, idRun)
	}
	next, err := s.NextQueued()
	if err != nil || next == nil || next.ID != idQueued {
		t.Fatalf("next queued = %+v, %v", next, err)
	}
}

func TestSettings(t *testing.T) {
	s := openTest(t)
	if _, ok, err := s.GetSetting("app"); err != nil || ok {
		t.Fatalf("get missing = %v, %v", ok, err)
	}
	if err := s.SetSetting("app", `{"a":1}`); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.SetSetting("app", `{"a":2}`); err != nil {
		t.Fatalf("set again: %v", err)
	}
	v, ok, err := s.GetSetting("app")
	if err != nil || !ok || v != `{"a":2}` {
		t.Fatalf("get = %q, %v, %v", v, ok, err)
	}
}
