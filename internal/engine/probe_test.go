package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gale/internal/model"
	"gale/internal/netx"
	"gale/internal/testutil"
)

func probeURL(t *testing.T, url string) (model.Preview, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return Probe(ctx, netx.NewTransport(), ProbeRequest{URL: url})
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var fe *Error
	if !errors.As(err, &fe) {
		t.Fatalf("error %v is not an engine error", err)
	}
	if fe.Code != code {
		t.Fatalf("code = %s (%s), want %s", fe.Code, fe.Message, code)
	}
}

func TestProbeRangeServer(t *testing.T) {
	s := testutil.New(t, testutil.Options{Size: 5000, Name: "movie.mkv"})
	p, err := probeURL(t, s.URL("ignored"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if p.Size != 5000 || !p.Resumable {
		t.Fatalf("preview = %+v, want size 5000 resumable", p)
	}
	if p.FileName != "movie.mkv" {
		t.Fatalf("fileName = %q", p.FileName)
	}
	if p.MIME != "application/octet-stream" {
		t.Fatalf("mime = %q", p.MIME)
	}
	if p.FinalURL != s.URL("ignored") {
		t.Fatalf("finalUrl = %q", p.FinalURL)
	}
}

func TestProbeNoRange(t *testing.T) {
	s := testutil.New(t, testutil.Options{Size: 1234, NoRange: true})
	p, err := probeURL(t, s.URL("file.bin"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if p.Size != 1234 || p.Resumable {
		t.Fatalf("preview = %+v, want size 1234 not resumable", p)
	}
	if p.FileName != "file.bin" {
		t.Fatalf("fileName = %q, want the URL's", p.FileName)
	}
}

func TestProbeChunked(t *testing.T) {
	s := testutil.New(t, testutil.Options{Size: 900, Chunked: true})
	p, err := probeURL(t, s.URL("stream.bin"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if p.Size != -1 || p.Resumable {
		t.Fatalf("preview = %+v, want unknown size, not resumable", p)
	}
}

func TestProbeWeakETagIsNotResumable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `W/"weak"`)
		w.Header().Set("Content-Range", "bytes 0-0/42")
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer srv.Close()
	p, err := probeURL(t, srv.URL+"/x.bin")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if p.Resumable {
		t.Fatalf("weak ETag made it resumable: %+v", p)
	}
}

func TestProbeLastModifiedIsResumable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
		w.Header().Set("Content-Range", "bytes 0-0/42")
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer srv.Close()
	p, err := probeURL(t, srv.URL+"/x.bin")
	if err != nil || !p.Resumable || p.Size != 42 {
		t.Fatalf("preview = %+v, %v", p, err)
	}
}

func TestProbeUnsatisfiedRange(t *testing.T) {
	s := testutil.New(t, testutil.Options{Size: 777, Status: http.StatusRequestedRangeNotSatisfiable})
	p, err := probeURL(t, s.URL("gone.bin"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if p.Size != 777 {
		t.Fatalf("size = %d, want 777 from Content-Range", p.Size)
	}
	if p.Resumable {
		t.Fatalf("no validator but resumable: %+v", p)
	}
}

func TestProbeEncodingRejected(t *testing.T) {
	s := testutil.New(t, testutil.Options{Size: 100, Encoding: "gzip"})
	_, err := probeURL(t, s.URL("x.bin"))
	wantCode(t, err, CodeEncoding)
}

func TestProbeErrors(t *testing.T) {
	shortenBackoff(t)
	cases := []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, CodeAuth},
		{http.StatusProxyAuthRequired, CodeAuth},
		{http.StatusForbidden, CodeHTTP},
		{http.StatusNotFound, CodeHTTP},
		{http.StatusTooManyRequests, CodeRetry},
		{http.StatusInternalServerError, CodeRetry},
	}
	for _, c := range cases {
		s := testutil.New(t, testutil.Options{Size: 10, Status: c.status})
		_, err := probeURL(t, s.URL("x.bin"))
		// 429 and 5xx retry three times before giving up: keep the test quick.
		if c.status == http.StatusTooManyRequests || c.status >= 500 {
			if err == nil {
				t.Fatalf("status %d: no error", c.status)
			}
			wantCode(t, err, c.code)
			continue
		}
		wantCode(t, err, c.code)
	}
}

func TestProbeRetriesThenSucceeds(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Range", "bytes 0-0/99")
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer srv.Close()
	p, err := probeURL(t, srv.URL+"/x.bin")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if hits != 2 || p.Size != 99 {
		t.Fatalf("hits = %d, preview = %+v", hits, p)
	}
}

func TestProbeDNSError(t *testing.T) {
	_, err := probeURL(t, "http://nonexistent.invalid/x.bin")
	wantCode(t, err, CodeDNS)
}

func TestProbeRejectsSchemes(t *testing.T) {
	_, err := probeURL(t, "ftp://example.com/x.bin")
	wantCode(t, err, CodeHTTP)
}

func TestProbeRedirectsAndDropsCredentials(t *testing.T) {
	var other string
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != "" {
			t.Errorf("cookie leaked across hosts: %q", got)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("user agent lost on redirect")
		}
		w.Header().Set("Content-Range", "bytes 0-0/5")
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer second.Close()
	other = second.URL

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other+"/final.bin", http.StatusFound)
	}))
	defer first.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := Probe(ctx, netx.NewTransport(), ProbeRequest{
		URL:     first.URL + "/start.bin",
		Cookies: "session=secret",
	})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !strings.HasSuffix(p.FinalURL, "/final.bin") || p.FileName != "final.bin" {
		t.Fatalf("preview = %+v", p)
	}
}
