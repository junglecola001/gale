package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	long := strings.Repeat("a", 300) + ".zip"
	cases := []struct{ in, want string }{
		{"report.pdf", "report.pdf"},
		{`a<b>c:d"e/f\g|h?i*j.zip`, "a_b_c_d_e_f_g_h_i_j.zip"},
		{"tab\there.txt", "tab_here.txt"},
		{"trailing...", "trailing"},
		{"   ", "download"},
		{"", "download"},
		{"CON", "_CON"},
		{"con.txt", "_con.txt"},
		{"LPT9.tar.gz", "_LPT9.tar.gz"},
		{"console.txt", "console.txt"},
		{"中文文件名.mp4", "中文文件名.mp4"},
	}
	for _, c := range cases {
		if got := SanitizeFilename(c.in); got != c.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := SanitizeFilename(long); len([]rune(got)) > maxNameUnits || !strings.HasSuffix(got, ".zip") {
		t.Errorf("long name not truncated: %d runes, %q", len([]rune(got)), got)
	}
	// A cut must not leave a trailing dot or space behind.
	if got := SanitizeFilename(strings.Repeat("x", 199) + " .zip"); strings.Contains(got, " .zip") {
		t.Errorf("cut left a trailing space: %q", got)
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	first, err := UniquePath(dir, "file.bin")
	if err != nil {
		t.Fatalf("unique path: %v", err)
	}
	if first != filepath.Join(dir, "file.bin") {
		t.Fatalf("first = %q", first)
	}
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := UniquePath(dir, "file.bin")
	if err != nil {
		t.Fatalf("unique path 2: %v", err)
	}
	if second != filepath.Join(dir, "file (1).bin") {
		t.Fatalf("second = %q", second)
	}
	if err := os.WriteFile(second, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := UniquePath(dir, "file.bin")
	if err != nil || third != filepath.Join(dir, "file (2).bin") {
		t.Fatalf("third = %q, %v", third, err)
	}
}

func TestEnsureDirAndFreeSpace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deeper")
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("dir not created: %v", err)
	}
	free, err := FreeSpace(dir)
	if err != nil {
		t.Fatalf("free space: %v", err)
	}
	if free == 0 {
		t.Fatalf("free space reported 0")
	}
}

func TestLongPath(t *testing.T) {
	short := `C:\Users\me\file.bin`
	if got := LongPath(short); got != short {
		t.Errorf("short path changed: %q", got)
	}
	if got := LongPath(`\\?\C:\already\long`); got != `\\?\C:\already\long` {
		t.Errorf("prefixed path changed: %q", got)
	}
	long := `C:\` + strings.Repeat("d", 300)
	if got := LongPath(long); !strings.HasPrefix(got, `\\?\`) {
		t.Errorf("long path not prefixed: %q", got)
	}
	unc := `\\server\share\` + strings.Repeat("d", 300)
	if got := LongPath(unc); !strings.HasPrefix(got, `\\?\UNC\server\share\`) {
		t.Errorf("UNC path not prefixed: %q", got)
	}
}
