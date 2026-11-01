// Package netx holds the HTTP client the engine downloads with: one
// connection per segment, no TLS resumption sharing surprises, and the
// parsers for the headers that decide how a download proceeds.
package netx

import (
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"time"
)

// UserAgent is what requests carry when the download has none of its own.
// It is fixed rather than built from the running system so that a server
// answers the same way on every machine.
const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// NewTransport returns the transport downloads use. HTTP/2 is off: a
// segment needs its own TCP connection so that the server's per-connection
// limits and the loss of one stream do not stall the others.
func NewTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    true,
		ForceAttemptHTTP2:     false,
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
}

// NewClient returns a client on tr, carrying extra headers on every request.
// Each download gets its own client so a cookie jar is its own too.
func NewClient(tr *http.Transport, extra http.Header) *http.Client {
	return &http.Client{
		Transport: headerTransport{base: tr, extra: extra},
	}
}

type headerTransport struct {
	base  http.RoundTripper
	extra http.Header
}

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(t.extra) > 0 && req.Header != nil {
		for k, vs := range t.extra {
			if _, ok := req.Header[k]; ok {
				continue
			}
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
	}
	return t.base.RoundTrip(req)
}

// RangeHeader is the Range header for the inclusive byte range [start, end].
// An end below start asks for everything from start on.
func RangeHeader(start, end int64) string {
	if end < start {
		return "bytes=" + strconv.FormatInt(start, 10) + "-"
	}
	return "bytes=" + strconv.FormatInt(start, 10) + "-" + strconv.FormatInt(end, 10)
}
