package fsutil

import (
	"path/filepath"
	"strings"
)

// longPathAt is where the Win32 API stops taking a plain path, in UTF-16
// code units. Paths from there on need the \\?\ form.
const longPathAt = 240

// LongPath returns p in the form the Win32 file APIs always accept: paths
// that may run into MAX_PATH get the \\?\ prefix, UNC paths the \\?\UNC\
// form. Shorter paths, and paths already prefixed, come back unchanged.
func LongPath(p string) string {
	if p == "" || strings.HasPrefix(p, `\\?\`) || strings.HasPrefix(p, `\\.\`) {
		return p
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	if len(abs) < longPathAt && !strings.HasPrefix(abs, `\\`) {
		return abs
	}
	if strings.HasPrefix(abs, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(abs, `\\`)
	}
	return `\\?\` + abs
}
