package netx

import (
	"errors"
	"strconv"
	"strings"
)

// ErrBadContentRange means a Content-Range header did not parse.
var ErrBadContentRange = errors.New("netx: malformed Content-Range")

// ParseContentRange reads "bytes start-end/total", where total may be "*"
// (-1). The unsatisfied form "bytes *\/N" gives start and end -1 with the
// total.
func ParseContentRange(v string) (start, end, total int64, err error) {
	s := strings.TrimSpace(v)
	const prefix = "bytes"
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return 0, 0, 0, ErrBadContentRange
	}
	s = strings.TrimSpace(s[len(prefix):])
	slash := strings.IndexByte(s, '/')
	if slash < 0 {
		return 0, 0, 0, ErrBadContentRange
	}
	rangePart := strings.TrimSpace(s[:slash])
	totalPart := strings.TrimSpace(s[slash+1:])

	if totalPart == "*" {
		total = -1
	} else {
		total, err = strconv.ParseInt(totalPart, 10, 64)
		if err != nil || total < 0 {
			return 0, 0, 0, ErrBadContentRange
		}
	}

	if rangePart == "*" {
		return -1, -1, total, nil
	}
	dash := strings.IndexByte(rangePart, '-')
	if dash <= 0 {
		return 0, 0, 0, ErrBadContentRange
	}
	start, err = strconv.ParseInt(strings.TrimSpace(rangePart[:dash]), 10, 64)
	if err != nil || start < 0 {
		return 0, 0, 0, ErrBadContentRange
	}
	end, err = strconv.ParseInt(strings.TrimSpace(rangePart[dash+1:]), 10, 64)
	if err != nil || end < start {
		return 0, 0, 0, ErrBadContentRange
	}
	if total >= 0 && end >= total {
		return 0, 0, 0, ErrBadContentRange
	}
	return start, end, total, nil
}
