// Package fsutil names and places the files the engine writes, with the
// care Windows needs: reserved names, trailing dots, long paths.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// maxNameUnits is the room a file name may take in UTF-16 code units, well
// under Windows' 255 so that " (12)" still fits beside a long extension.
const maxNameUnits = 200

// reserved are the device names Windows refuses as file names, with or
// without an extension.
var reserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// SanitizeFilename turns s into a name Windows accepts: no path separators
// or control characters, no trailing dots or spaces, not a device name, and
// short enough to write.
func SanitizeFilename(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r < 0x20, r == 0x7f:
			b.WriteByte('_')
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	name := strings.TrimRight(b.String(), " .")
	if name == "" {
		name = "download"
	}
	if dot := strings.IndexByte(name, '.'); dot > 0 && reserved[strings.ToUpper(name[:dot])] {
		name = "_" + name
	} else if reserved[strings.ToUpper(name)] {
		name = "_" + name
	}
	return truncate(name, maxNameUnits)
}

// truncate shortens name to max UTF-16 units, keeping its extension.
func truncate(name string, max int) string {
	if len(utf16.Encode([]rune(name))) <= max {
		return name
	}
	ext := filepath.Ext(name)
	if len(utf16.Encode([]rune(ext))) > 32 {
		ext = ""
	}
	base := strings.TrimSuffix(name, ext)
	room := max - len(utf16.Encode([]rune(ext)))
	units := 0
	cut := len(base)
	for i, r := range base {
		n := len(utf16.Encode([]rune{r}))
		if units+n > room {
			cut = i
			break
		}
		units += n
	}
	base = strings.TrimRight(base[:cut], " .")
	if base == "" {
		base = "download"
	}
	return base + ext
}

// UniquePath returns dir/name, or "name (1).ext", "name (2).ext" and so on
// when those exist.
func UniquePath(dir, name string) (string, error) {
	if err := EnsureDir(dir); err != nil {
		return "", err
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(dir, name)
	for i := 1; i < 10000; i++ {
		_, err := os.Stat(LongPath(candidate))
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
	}
	return "", fmt.Errorf("fsutil: no free name for %q in %q", name, dir)
}

// EnsureDir creates dir when it is missing.
func EnsureDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("fsutil: empty directory")
	}
	return os.MkdirAll(LongPath(dir), 0o755)
}
