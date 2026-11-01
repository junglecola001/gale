// Package logx writes the app's log: JSON lines to a file that rotates, and
// nothing that would leak a cookie or an Authorization header.
package logx

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	maxSize   = 5 << 20 // rotate past this many bytes
	keepFiles = 3       // gale.log.1 … gale.log.3
)

// Redacted stands in for secrets in the log.
const Redacted = "<redacted>"

// Logger is the app's logger, a discarding one until Init runs.
var (
	mu      sync.Mutex
	logger  = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	current *rotating
)

// Logger returns the app's logger.
func Logger() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	return logger
}

// Init sends the log to path, rotating it as it grows. Failures to write are
// not fatal: they leave the logger on its previous writer.
func Init(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	w, err := openRotating(path)
	if err != nil {
		return err
	}
	l := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mu.Lock()
	logger = l
	if current != nil {
		current.file.Close()
	}
	current = w
	mu.Unlock()
	slog.SetDefault(l)
	return nil
}

// Reset closes the log file and goes back to the stderr default, which tests
// do to release a temporary file.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		current.file.Close()
		current = nil
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)
}

// rotating writes to path and moves it aside once it passes maxSize.
type rotating struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

func openRotating(path string) (*rotating, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &rotating{path: path, file: f, size: info.Size()}, nil
}

func (r *rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(p)) > maxSize {
		r.rotate()
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate renames the current file to .1, shifting the older ones up and
// dropping the oldest. A failed rename leaves the file in place.
func (r *rotating) rotate() {
	r.file.Close()
	for i := keepFiles - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
	}
	if err := os.Rename(r.path, r.path+".1"); err != nil {
		// Another process holds it; keep appending rather than lose the log.
		if f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			r.file = f
		}
		return
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	r.file = f
	r.size = 0
}
