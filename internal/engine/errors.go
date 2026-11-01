package engine

import (
	"errors"
	"fmt"
)

// The codes a failed download carries. The page turns each into a sentence;
// the numbering is fixed, so a stored row keeps its meaning.
const (
	CodeDNS      = "E_DNS"
	CodeNet      = "E_NET"
	CodeTLS      = "E_TLS"
	CodeTimeout  = "E_TIMEOUT"
	CodeHTTP     = "E_HTTP"
	CodeAuth     = "E_AUTH"
	CodeRetry    = "E_RETRY"
	CodeDiskFull = "E_DISK_FULL"
	CodeDisk     = "E_DISK"
	CodeRange    = "E_RANGE"
	CodeChanged  = "E_CHANGED"
	CodeEncoding = "E_ENCODING"
	CodeRename   = "E_RENAME"
	CodeCanceled = "E_CANCELED"
	CodeInternal = "E_INTERNAL"
)

// Error is a failure with one of the codes above.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func failf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// codeOf returns the code of err: its own, E_CANCELED for a canceled
// context, or E_INTERNAL.
func codeOf(err error) (code, message string) {
	if err == nil {
		return "", ""
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code, e.Message
	}
	return CodeInternal, err.Error()
}
