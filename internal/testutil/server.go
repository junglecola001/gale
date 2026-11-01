// Package testutil is the download server the tests and cmd/testserver run
// against: a range-capable file server whose failures are switches.
package testutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Options configures the server's behavior.
type Options struct {
	Size int64 // the resource's length in bytes (default 4 MiB)
	// Name is the file name the server suggests in Content-Disposition
	// (empty: none is sent).
	Name string
	// NoRange answers every request with 200 and the whole body, as a
	// server without Range support does.
	NoRange bool
	// DropAfter cuts the first response off after this many bytes,
	// leaving the connection closed under the client.
	DropAfter int64
	// DropEvery cuts every response that grows past DropAfter, not just
	// the first.
	DropEvery bool
	// ETagChangeAfter changes the ETag (and the bytes behind it) every
	// this many bytes served, as a file that is replaced while it is
	// downloaded. ETagChanges caps how many times that happens; zero
	// changes it forever.
	ETagChangeAfter int64
	// ETagChangeRequests changes the ETag every this many finished
	// responses instead of bytes served: identical runs for every client.
	ETagChangeRequests int
	ETagChanges        int
	// Rate limits the whole server to this many bytes per second.
	Rate int64
	// Chunked answers with no Content-Length and a chunked body.
	Chunked bool
	// DelayFirst holds the first request's headers back this long, so that
	// the other connections start first.
	DelayFirst time.Duration
	// Encoding, when not empty, is sent as Content-Encoding.
	Encoding string
	// Status answers with this status instead of serving, e.g. 403 or 429.
	Status int
}

// Handler serves one resource under every path, its failures as Options say.
type Handler struct {
	opts Options

	mu         sync.Mutex
	served     int64
	generation int
	requests   int
	dropped    bool
	active     int
	peak       int
	completed  int
	nextFree   time.Time
}

// NewHandler returns a handler for opts.
func NewHandler(opts Options) *Handler {
	if opts.Size == 0 {
		opts.Size = 4 << 20
	}
	return &Handler{opts: opts}
}

// Generation is how many times the content has changed.
func (h *Handler) Generation() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.generation
}

// ETag is the current strong validator.
func (h *Handler) ETag() string {
	return fmt.Sprintf(`"gen%d"`, h.Generation())
}

// PeakConcurrent is the most requests the server had in flight at once.
func (h *Handler) PeakConcurrent() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.peak
}

// Served is how many body bytes the server has written so far.
func (h *Handler) Served() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.served
}

// Content is the resource as it is now.
func (h *Handler) Content() []byte {
	out := make([]byte, h.opts.Size)
	gen := h.Generation()
	for i := range out {
		out[i] = byteAt(gen, int64(i))
	}
	return out
}

// SHA256 is the hex digest of Content.
func (h *Handler) SHA256() string {
	sum := sha256.New()
	gen := h.Generation()
	buf := make([]byte, 64<<10)
	for off := int64(0); off < h.opts.Size; {
		n := int64(len(buf))
		if h.opts.Size-off < n {
			n = h.opts.Size - off
		}
		for i := int64(0); i < n; i++ {
			buf[i] = byteAt(gen, off+i)
		}
		sum.Write(buf[:n])
		off += n
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// byteAt is the deterministic content of a generation: the same byte for the
// same (generation, offset), so that ranges can be served without buffering.
func byteAt(gen int, i int64) byte {
	x := uint64(i)*0x9E3779B97F4A7C15 + uint64(gen+1)*0x85EBCA77C2B2AE63
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 29
	return byte(x >> 24)
}

// advance counts served bytes and changes the ETag when they reach the next
// multiple of ETagChangeAfter.
func (h *Handler) advance(n int64) {
	h.served += n
	if h.opts.ETagChangeAfter <= 0 {
		return
	}
	if h.served < int64(h.generation+1)*h.opts.ETagChangeAfter {
		return
	}
	if h.opts.ETagChanges > 0 && h.generation >= h.opts.ETagChanges {
		return
	}
	h.generation++
}

// wait throttles the response to Options.Rate.
func (h *Handler) wait(n int) {
	if h.opts.Rate <= 0 {
		return
	}
	need := time.Duration(int64(n) * int64(time.Second) / h.opts.Rate)
	h.mu.Lock()
	now := time.Now()
	if h.nextFree.Before(now) {
		h.nextFree = now
	}
	h.nextFree = h.nextFree.Add(need)
	at := h.nextFree
	h.mu.Unlock()
	if d := time.Until(at); d > 0 {
		time.Sleep(d)
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.requests++
	first := h.requests == 1
	h.active++
	if h.active > h.peak {
		h.peak = h.active
	}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.active--
		h.completed++
		h.mu.Unlock()
	}()

	if first && h.opts.DelayFirst > 0 {
		time.Sleep(h.opts.DelayFirst)
	}

	header := w.Header()
	if h.opts.Status != 0 {
		if h.opts.Status == http.StatusRequestedRangeNotSatisfiable {
			header.Set("Content-Range", fmt.Sprintf("bytes */%d", h.opts.Size))
		}
		w.WriteHeader(h.opts.Status)
		return
	}

	h.mu.Lock()
	if h.opts.ETagChangeRequests > 0 && h.generation < h.opts.ETagChanges &&
		h.completed >= (h.generation+1)*h.opts.ETagChangeRequests {
		h.generation++
	}
	etag := fmt.Sprintf(`"gen%d"`, h.generation)
	gen := h.generation
	drop := h.opts.DropAfter > 0 && (h.opts.DropEvery || !h.dropped)
	h.mu.Unlock()

	lastMod := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Format(http.TimeFormat)
	start, end := int64(0), h.opts.Size-1
	partial := false

	if !h.opts.NoRange && !h.opts.Chunked {
		if rng := r.Header.Get("Range"); rng != "" {
			if s, e, ok := parseRange(rng, h.opts.Size); ok {
				if ir := r.Header.Get("If-Range"); ir == "" || ir == etag || ir == lastMod {
					start, end, partial = s, e, true
				}
			}
		}
	}

	header.Set("ETag", etag)
	header.Set("Last-Modified", lastMod)
	header.Set("Accept-Ranges", "bytes")
	header.Set("Content-Type", "application/octet-stream")
	if h.opts.Name != "" {
		header.Set("Content-Disposition", `attachment; filename="`+h.opts.Name+`"`)
	}
	if h.opts.Encoding != "" {
		header.Set("Content-Encoding", h.opts.Encoding)
	}
	length := end - start + 1
	if partial {
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, h.opts.Size))
	}
	if !h.opts.Chunked && h.opts.Encoding == "" {
		header.Set("Content-Length", strconv.FormatInt(length, 10))
	}
	if partial {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	limit := length
	if drop && h.opts.DropAfter < limit {
		limit = h.opts.DropAfter
	}
	buf := make([]byte, 32<<10)
	written := int64(0)
	for written < limit {
		n := int64(len(buf))
		if limit-written < n {
			n = limit - written
		}
		for i := int64(0); i < n; i++ {
			buf[i] = byteAt(gen, start+written+i)
		}
		h.wait(int(n))
		if _, err := w.Write(buf[:n]); err != nil {
			return
		}
		written += n
		h.mu.Lock()
		h.advance(n)
		h.mu.Unlock()
		if h.opts.Chunked {
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
	if drop && written < length {
		h.mu.Lock()
		h.dropped = true
		h.mu.Unlock()
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				conn.Close()
			}
		}
	}
}

// parseRange reads "bytes=start-end", "bytes=start-" and "bytes=-suffix".
func parseRange(v string, size int64) (start, end int64, ok bool) {
	if !strings.HasPrefix(v, "bytes=") {
		return 0, 0, false
	}
	spec := strings.TrimPrefix(v, "bytes=")
	if strings.Contains(spec, ",") {
		return 0, 0, false
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false
	}
	first, last := spec[:dash], spec[dash+1:]
	if first == "" {
		suffix, err := strconv.ParseInt(last, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, true
	}
	s, err := strconv.ParseInt(first, 10, 64)
	if err != nil || s < 0 || s >= size {
		return 0, 0, false
	}
	e := size - 1
	if last != "" {
		e, err = strconv.ParseInt(last, 10, 64)
		if err != nil || e < s {
			return 0, 0, false
		}
		if e >= size {
			e = size - 1
		}
	}
	return s, e, true
}

// Server is a test server around a Handler.
type Server struct {
	*httptest.Server
	H *Handler
}

// New starts a server for opts and closes it when the test ends.
func New(t testing.TB, opts Options) *Server {
	t.Helper()
	h := NewHandler(opts)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return &Server{Server: ts, H: h}
}

// URL is the address of name on the server.
func (s *Server) URL(name string) string {
	return s.Server.URL + "/file/" + name
}
