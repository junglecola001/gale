// Package bridge is the loopback HTTP server the browser extension talks to,
// and the native messaging host that feeds it.
package bridge

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"gale/internal/logx"
	"gale/internal/model"
)

// FileName is the JSON file, in the user data directory, with the port and
// token a native host needs to reach the running app.
const FileName = "bridge.json"

// Info is what bridge.json holds.
type Info struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// AddFunc stores a download the browser handed over and returns its id.
type AddFunc func(req model.AddRequest) (int64, error)

// Server is the app's loopback endpoint.
type Server struct {
	ln       net.Listener
	info     Info
	path     string
	add      AddFunc
	enabled  func() bool
	version  string
	srv      *http.Server
	lastPing atomic.Int64
}

// Start listens on a free loopback port, writes bridge.json and serves until
// Close.
func Start(userDataDir, version string, add AddFunc, enabled func() bool) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		ln.Close()
		return nil, err
	}
	s := &Server{
		ln:      ln,
		path:    filepath.Join(userDataDir, FileName),
		add:     add,
		enabled: enabled,
		version: version,
		info: Info{
			Port:  ln.Addr().(*net.TCPAddr).Port,
			Token: hex.EncodeToString(token),
			PID:   os.Getpid(),
		},
	}
	if err := s.writeInfo(); err != nil {
		ln.Close()
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ping", s.handlePing)
	mux.HandleFunc("/v1/add", s.handleAdd)
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logx.Logger().Warn("bridge stopped", "error", err)
		}
	}()
	return s, nil
}

// Port is the port the server listens on.
func (s *Server) Port() int { return s.info.Port }

// LastPing is when the extension last called /v1/ping, in unix milliseconds.
func (s *Server) LastPing() int64 { return s.lastPing.Load() }

// Connected reports whether the extension pinged in the last 90 seconds.
func (s *Server) Connected() bool {
	last := s.lastPing.Load()
	return last > 0 && time.Since(time.UnixMilli(last)) < 90*time.Second
}

// writeInfo stores bridge.json atomically, so a host never reads half of it.
func (s *Server) writeInfo() error {
	raw, err := json.Marshal(s.info)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Close stops serving and removes bridge.json.
func (s *Server) Close() error {
	os.Remove(s.path)
	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}

// ReadInfo reads bridge.json from a user data directory.
func ReadInfo(userDataDir string) (Info, error) {
	var info Info
	raw, err := os.ReadFile(filepath.Join(userDataDir, FileName))
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return info, err
	}
	if info.Port == 0 || info.Token == "" {
		return info, fmt.Errorf("bridge: incomplete %s", FileName)
	}
	return info, nil
}

func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	got := r.Header.Get("X-Gale-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.info.Token)) != 1 {
		http.Error(w, `{"ok":false,"error":"unauthorized"}`, http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"ok":false,"error":"method"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(w, r) {
		return
	}
	s.lastPing.Store(time.Now().UnixMilli())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": "Gale", "version": s.version})
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"ok":false,"error":"method"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(w, r) {
		return
	}
	if s.enabled != nil && !s.enabled() {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "browser capture is off"})
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"ok": false, "error": "content type"})
		return
	}
	var req model.AddRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad request"})
		return
	}
	if err := validate(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, err := s.add(req)
	if err != nil {
		logx.Logger().Warn("bridge add failed", "error", err, "url", req.URL)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	logx.Logger().Info("bridge added a download", "id", id, "url", req.URL)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func validate(req *model.AddRequest) error {
	req.URL = strings.TrimSpace(req.URL)
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		return errors.New("only http and https links are accepted")
	}
	if len(req.URL) > 8192 {
		return errors.New("link too long")
	}
	if req.Referer != "" && !strings.HasPrefix(req.Referer, "http://") && !strings.HasPrefix(req.Referer, "https://") {
		req.Referer = ""
	}
	req.UserAgent = stripNewlines(req.UserAgent)
	req.Cookies = stripNewlines(req.Cookies)
	req.FileName = stripNewlines(req.FileName)
	if len(req.Cookies) > 16<<10 {
		return errors.New("cookies too long")
	}
	return nil
}

func stripNewlines(v string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
