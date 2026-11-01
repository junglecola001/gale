package browsext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectBrowsersRejectsUnknown(t *testing.T) {
	if _, err := selectBrowsers("no-such-browser"); err == nil {
		t.Fatal("unknown browser accepted")
	}
}

func TestAutoHostSelectsOnlyInstalled(t *testing.T) {
	// Auto resolves against what is installed; every browser it selects must
	// report as installed.
	targets, err := selectBrowsers(AutoHost)
	if err != nil {
		t.Fatalf("auto: %v", err)
	}
	for _, target := range targets {
		if !target.installed() {
			t.Fatalf("auto selected uninstalled %s", target.id)
		}
	}
}

func TestLaunchSpecsReportsStartedRunningManual(t *testing.T) {
	dir := t.TempDir()
	edgeExe := filepath.Join(dir, "msedge.exe")
	braveExe := filepath.Join(dir, "brave.exe")
	for _, path := range []string{edgeExe, braveExe} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	oldStart, oldRunning := startBrowser, browserRunning
	t.Cleanup(func() { startBrowser, browserRunning = oldStart, oldRunning })
	var started [][]string
	startBrowser = func(exe string, args ...string) error {
		started = append(started, append([]string{exe}, args...))
		return nil
	}
	browserRunning = func(exe string) bool { return filepath.Base(exe) == "brave.exe" }

	extensionDir := filepath.Join(dir, "extension")
	result, err := launchSpecs([]browserSpec{
		{id: "edge", name: "Edge", exes: []string{edgeExe}},
		{id: "brave", name: "Brave", exes: []string{braveExe}},
		{id: "chrome", name: "Chrome", exes: []string{edgeExe}, manual: true},
	}, extensionDir)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if len(result.Started) != 1 || result.Started[0] != "Edge" {
		t.Fatalf("started = %v", result.Started)
	}
	if len(result.Running) != 1 || result.Running[0] != "Brave" {
		t.Fatalf("running = %v", result.Running)
	}
	if len(result.Manual) != 1 || result.Manual[0] != "Chrome" {
		t.Fatalf("manual = %v", result.Manual)
	}
	if len(started) != 1 || started[0][0] != edgeExe || started[0][1] != "--load-extension="+extensionDir {
		t.Fatalf("started commands = %v", started)
	}
}
