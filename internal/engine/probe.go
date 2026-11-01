package engine

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gale/internal/fsutil"
	"gale/internal/model"
	"gale/internal/netx"
)

// ProbeRequest is what a probe needs to look like the browser that linked
// the file.
type ProbeRequest struct {
	URL       string
	Referrer  string
	UserAgent string
	Cookies   string
}

// probeResult is the preview the page sees, plus the validators the engine
// keeps to resume the file later.
type probeResult struct {
	model.Preview
	ETag         string
	LastModified string
}

const (
	probeTimeout     = 20 * time.Second
	probeRetries     = 3
	probeMaxRedirect = 10
)

// probeBackoffBase is the first of the 1, 2, 4 second waits between the
// retries of a request the server answered with 429 or 5xx. Tests shorten it.
var probeBackoffBase = time.Second

// Probe asks the server what the URL holds, without downloading it: one
// ranged GET for a single byte, dropped as soon as the headers arrive.
func Probe(ctx context.Context, tr *http.Transport, req ProbeRequest) (model.Preview, error) {
	res, err := probe(ctx, tr, req)
	return res.Preview, err
}

func probe(ctx context.Context, tr *http.Transport, req ProbeRequest) (probeResult, error) {
	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return probeResult{}, failf(CodeHTTP, "不支持的链接: %s", req.URL)
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	var last error
	for attempt := 0; attempt <= probeRetries; attempt++ {
		if attempt > 0 {
			delay := probeBackoffBase << (attempt - 1)
			select {
			case <-ctx.Done():
				return probeResult{}, errFromContext(ctx, last)
			case <-time.After(delay):
			}
		}
		res, err := probeOnce(ctx, tr, req)
		if err == nil {
			return res, nil
		}
		var fe *Error
		if errors.As(err, &fe) && fe.Code == CodeRetry {
			last = err
			continue
		}
		return probeResult{}, err
	}
	return probeResult{}, last
}

func errFromContext(ctx context.Context, last error) error {
	if last != nil {
		return last
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return failf(CodeTimeout, "连接超时")
	}
	return failf(CodeCanceled, "已取消")
}

func probeOnce(ctx context.Context, tr *http.Transport, req ProbeRequest) (probeResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return probeResult{}, failf(CodeHTTP, "无效的链接: %s", req.URL)
	}
	ua := req.UserAgent
	if ua == "" {
		ua = netx.UserAgent
	}
	httpReq.Header.Set("User-Agent", ua)
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("Accept-Encoding", "identity")
	httpReq.Header.Set("Range", "bytes=0-0")
	if req.Referrer != "" {
		httpReq.Header.Set("Referer", req.Referrer)
	}
	if req.Cookies != "" {
		httpReq.Header.Set("Cookie", req.Cookies)
	}

	client := &http.Client{Transport: tr}
	origin := httpReq.URL
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= probeMaxRedirect {
			return fmt.Errorf("too many redirects")
		}
		if !strings.EqualFold(next.URL.Host, origin.Host) {
			next.Header.Del("Cookie")
			next.Header.Del("Authorization")
		}
		next.Header.Set("User-Agent", ua)
		if req.Referrer != "" {
			next.Header.Set("Referer", req.Referrer)
		}
		return nil
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return probeResult{}, classifyTransportError(ctx, err)
	}
	defer resp.Body.Close()

	if enc := strings.TrimSpace(resp.Header.Get("Content-Encoding")); enc != "" && !strings.EqualFold(enc, "identity") {
		return probeResult{}, failf(CodeEncoding, "服务器返回了压缩内容 (Content-Encoding: %s)", enc)
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
		start, _, total, err := netx.ParseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			return probeResult{}, failf(CodeRange, "服务器返回了无法理解的分段信息")
		}
		if start != 0 {
			return probeResult{}, failf(CodeRange, "服务器从错误的位置开始响应")
		}
		return finish(resp, total, total > 0 && hasStrongValidator(resp)), nil
	case http.StatusOK:
		size := resp.ContentLength
		if size < 0 {
			size = -1
		}
		return finish(resp, size, false), nil
	case http.StatusRequestedRangeNotSatisfiable:
		_, _, total, err := netx.ParseContentRange(resp.Header.Get("Content-Range"))
		if err != nil || total < 0 {
			return probeResult{}, failf(CodeHTTP, "服务器拒绝了请求 (HTTP 416)")
		}
		return finish(resp, total, total > 0 && hasStrongValidator(resp)), nil
	}

	switch code := resp.StatusCode; {
	case code == http.StatusUnauthorized, code == http.StatusProxyAuthRequired:
		return probeResult{}, failf(CodeAuth, "需要登录或授权 (HTTP %d)", code)
	case code == http.StatusTooManyRequests, code >= 500:
		return probeResult{}, failf(CodeRetry, "服务器暂时不可用 (HTTP %d)", code)
	default:
		return probeResult{}, failf(CodeHTTP, "下载失败 (HTTP %d)", code)
	}
}

// finish builds the result from the response's headers.
func finish(resp *http.Response, size int64, resumable bool) probeResult {
	return probeResult{
		Preview: model.Preview{
			FinalURL:  resp.Request.URL.String(),
			FileName:  suggestedName(resp),
			MIME:      mediaType(resp.Header.Get("Content-Type")),
			Size:      size,
			Resumable: resumable,
		},
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
}

func hasStrongValidator(resp *http.Response) bool {
	etag := resp.Header.Get("ETag")
	if etag != "" && !isWeakETag(etag) {
		return true
	}
	return resp.Header.Get("Last-Modified") != ""
}

func mediaType(v string) string {
	if v == "" {
		return ""
	}
	if t, _, err := mime.ParseMediaType(v); err == nil {
		return t
	}
	return strings.TrimSpace(strings.Split(v, ";")[0])
}

// suggestedName is the name the server hints at, else the URL's last path
// segment, else a timestamped one, all cleaned for Windows.
func suggestedName(resp *http.Response) string {
	if name := netx.FilenameFromDisposition(resp.Header.Get("Content-Disposition")); name != "" {
		return fsutil.SanitizeFilename(name)
	}
	path := resp.Request.URL.Path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	if unescaped, err := url.PathUnescape(path); err == nil {
		path = unescaped
	}
	if strings.TrimSpace(path) == "" {
		path = "download-" + time.Now().Format("20060102-150405")
	}
	return fsutil.SanitizeFilename(path)
}

// classifyTransportError maps a failed request to one of the codes.
func classifyTransportError(ctx context.Context, err error) error {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTimeout {
			return failf(CodeDNS, "无法解析域名 %s（DNS 超时）", dnsErr.Name)
		}
		return failf(CodeDNS, "无法解析域名 %s", dnsErr.Name)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return failf(CodeTimeout, "连接超时")
	}
	if errors.Is(err, context.Canceled) {
		return failf(CodeCanceled, "已取消")
	}
	var recordErr tls.RecordHeaderError
	var verifyErr *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var invalidCert x509.CertificateInvalidError
	if errors.As(err, &recordErr) || errors.As(err, &verifyErr) ||
		errors.As(err, &unknownAuthority) || errors.As(err, &hostnameErr) || errors.As(err, &invalidCert) {
		return failf(CodeTLS, "TLS 证书或加密协商失败")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return failf(CodeTimeout, "连接超时")
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return failf(CodeNet, "连接被中断")
	}
	return failf(CodeNet, "网络错误: %s", err.Error())
}
