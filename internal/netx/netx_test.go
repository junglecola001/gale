package netx

import (
	"errors"
	"testing"
)

func TestParseContentRange(t *testing.T) {
	cases := []struct {
		in                string
		start, end, total int64
		err               bool
	}{
		{"bytes 0-0/12345", 0, 0, 12345, false},
		{"bytes 100-199/2000", 100, 199, 2000, false},
		{"bytes 0-99/*", 0, 99, -1, false},
		{"bytes */1234", -1, -1, 1234, false},
		{"BYTES 5-6/7", 5, 6, 7, false},
		{"bytes 5-5/5", 0, 0, 0, true},  // end past the last byte
		{"bytes 9-5/20", 0, 0, 0, true}, // backwards
		{"bytes 0-1", 0, 0, 0, true},    // no total
		{"bytes -1-2/3", 0, 0, 0, true}, // no start
		{"items 0-1/2", 0, 0, 0, true},  // wrong unit
		{"", 0, 0, 0, true},             //
		{"bytes x-y/2", 0, 0, 0, true},  // not numbers
		{"bytes 0-1/-2", 0, 0, 0, true}, // negative total
		{"bytes */0", -1, -1, 0, false}, // empty resource
	}
	for _, c := range cases {
		start, end, total, err := ParseContentRange(c.in)
		if c.err {
			if !errors.Is(err, ErrBadContentRange) {
				t.Errorf("ParseContentRange(%q) err = %v, want ErrBadContentRange", c.in, err)
			}
			continue
		}
		if err != nil || start != c.start || end != c.end || total != c.total {
			t.Errorf("ParseContentRange(%q) = %d,%d,%d,%v want %d,%d,%d", c.in, start, end, total, err, c.start, c.end, c.total)
		}
	}
}

func TestRangeHeader(t *testing.T) {
	if got := RangeHeader(0, 99); got != "bytes=0-99" {
		t.Errorf("RangeHeader(0,99) = %q", got)
	}
	if got := RangeHeader(500, -1); got != "bytes=500-" {
		t.Errorf("RangeHeader(500,-1) = %q", got)
	}
}

func TestFilenameFromDisposition(t *testing.T) {
	cases := []struct{ in, want string }{
		{`attachment; filename="report.pdf"`, "report.pdf"},
		{`attachment; filename*=UTF-8''%E4%B8%AD.zip`, "中.zip"},
		{`inline`, ""},
		{"", ""},
		{`attachment; filename="a b.iso"`, "a b.iso"},
	}
	for _, c := range cases {
		if got := FilenameFromDisposition(c.in); got != c.want {
			t.Errorf("FilenameFromDisposition(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
