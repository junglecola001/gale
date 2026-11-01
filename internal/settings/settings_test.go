package settings

import (
	"path/filepath"
	"testing"

	"gale/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "gale.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestLoadDefaultsAndPersist(t *testing.T) {
	st := openStore(t)
	dir := filepath.Join(t.TempDir(), "downloads")
	s, err := Load(st, Defaults(dir))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := s.Get()
	if got.DownloadDir != dir || got.Connections != 8 || got.MaxConcurrent != 3 || !got.AutoResume {
		t.Fatalf("defaults = %+v", got)
	}
	if got.BrowserHost != "auto" {
		t.Fatalf("browser host = %q", got.BrowserHost)
	}

	changed := got
	changed.Connections = 16
	changed.Theme = "dark"
	changed.Categorize = true
	changed.BrowserHost = "edge"
	saved, err := s.Update(changed)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if saved.Connections != 16 || saved.Theme != "dark" || !saved.Categorize || saved.BrowserHost != "edge" {
		t.Fatalf("saved = %+v", saved)
	}

	// A second load reads what the first wrote.
	s2, err := Load(st, Defaults(dir))
	if err != nil {
		t.Fatalf("load 2: %v", err)
	}
	if s2.Get().Connections != 16 || s2.Get().Theme != "dark" || s2.Get().BrowserHost != "edge" {
		t.Fatalf("reloaded = %+v", s2.Get())
	}
}

func TestUpdateClamps(t *testing.T) {
	st := openStore(t)
	s, _ := Load(st, Defaults(t.TempDir()))
	d := s.Get()
	d.Connections = 900
	d.MaxConcurrent = 0
	d.MinSegmentBytes = 1
	d.MaxRetries = -4
	d.SpeedLimit = -10
	d.Theme = "purple"
	d.DownloadDir = ""

	saved, err := s.Update(d)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if saved.Connections != MaxConnections {
		t.Fatalf("connections = %d", saved.Connections)
	}
	if saved.MaxConcurrent < MinConcurrent || saved.MaxConcurrent > MaxConcurrent {
		t.Fatalf("maxConcurrent = %d", saved.MaxConcurrent)
	}
	if saved.MinSegmentBytes != MinSegmentMin {
		t.Fatalf("minSegmentBytes = %d", saved.MinSegmentBytes)
	}
	if saved.MaxRetries != MinRetries {
		t.Fatalf("maxRetries = %d", saved.MaxRetries)
	}
	if saved.SpeedLimit != 0 {
		t.Fatalf("speedLimit = %d", saved.SpeedLimit)
	}
	if saved.Theme != "system" {
		t.Fatalf("theme = %q", saved.Theme)
	}
	if saved.DownloadDir == "" {
		t.Fatalf("download dir emptied")
	}
}
