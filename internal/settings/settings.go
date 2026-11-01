// Package settings keeps the app's preferences: the defaults, the JSON row
// they live in, and the bounds each field accepts.
package settings

import (
	"encoding/json"
	"path/filepath"
	"sync"

	"gale/internal/store"
)

// The bounds of the numeric settings.
const (
	MinConnections = 1
	MaxConnections = 32
	MinConcurrent  = 1
	MaxConcurrent  = 8
	MinSegmentMin  = 256 << 10
	MaxSegmentMax  = 16 << 20
	MinRetries     = 1
	MaxRetries     = 20
)

// Data is the whole of the settings, as the page and the engine see them.
type Data struct {
	DownloadDir      string `json:"downloadDir"`
	Connections      int    `json:"connections"`
	MaxConcurrent    int    `json:"maxConcurrent"`
	MinSegmentBytes  int64  `json:"minSegmentBytes"`
	SpeedLimit       int64  `json:"speedLimit"` // bytes per second, 0 = no limit
	MaxRetries       int    `json:"maxRetries"`
	AutoResume       bool   `json:"autoResume"`
	Categorize       bool   `json:"categorize"`
	Theme            string `json:"theme"` // system, dark, light
	CloseToTray      bool   `json:"closeToTray"`
	NotifyOnComplete bool   `json:"notifyOnComplete"`
	OpenAtLogin      bool   `json:"openAtLogin"`
	BrowserEnabled   bool   `json:"browserEnabled"`
	BrowserHost      string `json:"browserHost"` // auto, or one browser id
}

// Defaults returns the settings of a first run, saving into dir.
func Defaults(downloadDir string) Data {
	return Data{
		DownloadDir:      downloadDir,
		Connections:      8,
		MaxConcurrent:    3,
		MinSegmentBytes:  1 << 20,
		SpeedLimit:       0,
		MaxRetries:       8,
		AutoResume:       true,
		Categorize:       false,
		Theme:            "system",
		CloseToTray:      true,
		NotifyOnComplete: true,
		OpenAtLogin:      false,
		BrowserEnabled:   false,
		BrowserHost:      "auto",
	}
}

const key = "app"

// Store holds the settings and writes them back when they change.
type Store struct {
	mu   sync.RWMutex
	st   *store.Store
	data Data
}

// Load reads the settings, filling in the defaults for whatever was never
// saved or no longer makes sense.
func Load(st *store.Store, defaults Data) (*Store, error) {
	s := &Store{st: st, data: defaults}
	raw, ok, err := st.GetSetting(key)
	if err != nil {
		return nil, err
	}
	if ok {
		if err := json.Unmarshal([]byte(raw), &s.data); err != nil {
			// A setting file from a broken install: keep what parses, else
			// the defaults, rather than refusing to start.
			s.data = defaults
		}
	}
	s.data = clamp(s.data, defaults)
	return s, nil
}

// Get returns the current settings.
func (s *Store) Get() Data {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// Update validates, stores and returns the settings as they now are.
func (s *Store) Update(d Data) (Data, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d = clamp(d, s.data)
	raw, err := json.Marshal(d)
	if err != nil {
		return s.data, err
	}
	if err := s.st.SetSetting(key, string(raw)); err != nil {
		return s.data, err
	}
	s.data = d
	return s.data, nil
}

// clamp brings every field into range. The defaults fill in what is empty.
func clamp(d, base Data) Data {
	if d.DownloadDir == "" {
		d.DownloadDir = base.DownloadDir
	} else if abs, err := filepath.Abs(d.DownloadDir); err == nil {
		d.DownloadDir = abs
	}
	d.Connections = clampInt(d.Connections, MinConnections, MaxConnections, base.Connections)
	d.MaxConcurrent = clampInt(d.MaxConcurrent, MinConcurrent, MaxConcurrent, base.MaxConcurrent)
	d.MinSegmentBytes = clampInt64(d.MinSegmentBytes, MinSegmentMin, MaxSegmentMax, base.MinSegmentBytes)
	d.MaxRetries = clampInt(d.MaxRetries, MinRetries, MaxRetries, base.MaxRetries)
	if d.SpeedLimit < 0 {
		d.SpeedLimit = 0
	}
	switch d.Theme {
	case "system", "dark", "light":
	default:
		d.Theme = base.Theme
	}
	if d.BrowserHost == "" {
		d.BrowserHost = base.BrowserHost
	}
	if d.BrowserHost == "" {
		d.BrowserHost = "auto"
	}
	return d
}

func clampInt(v, lo, hi, fallback int) int {
	if v == 0 {
		v = fallback
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt64(v, lo, hi, fallback int64) int64 {
	if v == 0 {
		v = fallback
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
