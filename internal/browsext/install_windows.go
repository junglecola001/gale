// Package browsext installs the native messaging host that lets the browser
// extension hand downloads to Gale.
package browsext

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// HostName is the manifest name and the registry key the extension asks for.
const HostName = "app.gale.host"

// AutoHost is the browserHost setting that registers every detected browser.
const AutoHost = "auto"

// Paths are where the app keeps the pieces the host needs.
type Paths struct {
	Exe       string // the app's executable, which the manifest points at
	Resources string // the directory holding extension/
	UserData  string // where the native manifest goes
}

// Browser is one Chromium-family browser Gale can hand downloads to. The
// extension only runs in Chromium browsers, so Firefox and its relatives are
// neither detected nor registered.
type Browser struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Installed  bool   `json:"installed"`
	Registered bool   `json:"registered"`
}

// State is what the settings page shows about the browser integration.
type State struct {
	HostInstalled bool      `json:"hostInstalled"`
	ExtensionID   string    `json:"extensionId"`
	ExtensionDir  string    `json:"extensionDir"`
	Browsers      []Browser `json:"browsers"`
}

// LaunchResult says what happened when Gale tried to load the unpacked
// extension: which browsers it started, which ones are already running and
// need a restart, and which ones do not accept command-line extensions.
type LaunchResult struct {
	Started []string `json:"started"`
	Running []string `json:"running"`
	Manual  []string `json:"manual"`
}

// NativeManifest is the file Chrome and Edge read for a host.
type NativeManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// browserSpec is one browser's registry key, executable and profile
// directory. Detection checks the executable first and the profile directory
// second, which also finds a browser installed outside the usual places.
type browserSpec struct {
	id            string
	name          string
	key           string
	appPath       string
	exes          []string
	profiles      []string
	extensionsURL string
	manual        bool // the branded build ignores --load-extension
}

func browserSpecs() []browserSpec {
	pf := os.Getenv("ProgramFiles")
	pf86 := os.Getenv("ProgramFiles(x86)")
	local := os.Getenv("LOCALAPPDATA")
	roaming := os.Getenv("APPDATA")
	under := func(root string, parts ...string) string {
		if root == "" {
			return ""
		}
		return filepath.Join(append([]string{root}, parts...)...)
	}
	nonEmpty := func(paths ...string) []string {
		out := make([]string, 0, len(paths))
		for _, path := range paths {
			if path != "" {
				out = append(out, path)
			}
		}
		return out
	}
	return []browserSpec{
		{
			id:      "chrome",
			name:    "Google Chrome",
			key:     `Software\Google\Chrome\NativeMessagingHosts\` + HostName,
			appPath: "chrome.exe",
			exes: nonEmpty(
				under(pf, "Google", "Chrome", "Application", "chrome.exe"),
				under(pf86, "Google", "Chrome", "Application", "chrome.exe"),
				under(local, "Google", "Chrome", "Application", "chrome.exe"),
			),
			profiles: nonEmpty(under(local, "Google", "Chrome", "User Data")),
			// Chrome branded builds stopped honoring --load-extension in
			// 2025; the extensions page is the only way left.
			extensionsURL: "chrome://extensions/",
			manual:        true,
		},
		{
			id:      "edge",
			name:    "Microsoft Edge",
			key:     `Software\Microsoft\Edge\NativeMessagingHosts\` + HostName,
			appPath: "msedge.exe",
			exes: nonEmpty(
				under(pf, "Microsoft", "Edge", "Application", "msedge.exe"),
				under(pf86, "Microsoft", "Edge", "Application", "msedge.exe"),
				under(local, "Microsoft", "Edge", "Application", "msedge.exe"),
			),
			profiles:      nonEmpty(under(local, "Microsoft", "Edge", "User Data")),
			extensionsURL: "edge://extensions/",
		},
		{
			id:      "brave",
			name:    "Brave",
			key:     `Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + HostName,
			appPath: "brave.exe",
			exes: nonEmpty(
				under(pf, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
				under(pf86, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
				under(local, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			),
			profiles:      nonEmpty(under(local, "BraveSoftware", "Brave-Browser", "User Data")),
			extensionsURL: "brave://extensions/",
		},
		{
			id:      "vivaldi",
			name:    "Vivaldi",
			key:     `Software\Vivaldi\NativeMessagingHosts\` + HostName,
			appPath: "vivaldi.exe",
			exes: nonEmpty(
				under(local, "Vivaldi", "Application", "vivaldi.exe"),
				under(pf, "Vivaldi", "Application", "vivaldi.exe"),
				under(pf86, "Vivaldi", "Application", "vivaldi.exe"),
			),
			profiles:      nonEmpty(under(local, "Vivaldi", "User Data")),
			extensionsURL: "vivaldi://extensions/",
		},
		{
			id:      "opera",
			name:    "Opera",
			key:     `Software\Opera Software\NativeMessagingHosts\` + HostName,
			appPath: "opera.exe",
			exes: nonEmpty(
				under(local, "Programs", "Opera", "opera.exe"),
				under(local, "Programs", "Opera", "launcher.exe"),
				under(pf, "Opera", "opera.exe"),
				under(pf86, "Opera", "opera.exe"),
			),
			profiles: nonEmpty(
				under(roaming, "Opera Software", "Opera Stable"),
				under(roaming, "Opera Software", "Opera GX Stable"),
			),
			extensionsURL: "opera://extensions/",
		},
		{
			id:      "chromium",
			name:    "Chromium",
			key:     `Software\Chromium\NativeMessagingHosts\` + HostName,
			appPath: "chromium.exe",
			exes: nonEmpty(
				under(pf, "Chromium", "Application", "chrome.exe"),
				under(pf86, "Chromium", "Application", "chrome.exe"),
				under(local, "Chromium", "Application", "chrome.exe"),
			),
			profiles:      nonEmpty(under(local, "Chromium", "User Data")),
			extensionsURL: "chrome://extensions/",
		},
	}
}

// appPathExe resolves an executable through App Paths, where every browser
// installer registers it.
func appPathExe(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	path := `Software\Microsoft\Windows\CurrentVersion\App Paths\` + name
	views := []struct {
		root   registry.Key
		access uint32
	}{
		{registry.CURRENT_USER, 0},
		{registry.LOCAL_MACHINE, registry.WOW64_64KEY},
		{registry.LOCAL_MACHINE, registry.WOW64_32KEY},
	}
	for _, view := range views {
		k, err := registry.OpenKey(view.root, path, registry.QUERY_VALUE|view.access)
		if err != nil {
			continue
		}
		value, _, err := k.GetStringValue("")
		k.Close()
		if err != nil {
			continue
		}
		value = strings.Trim(value, `"`)
		if info, err := os.Stat(value); err == nil && !info.IsDir() {
			return value, true
		}
	}
	return "", false
}

// executable returns the browser's executable, preferring App Paths and
// falling back to the candidate install directories.
func (s browserSpec) executable() string {
	if path, ok := appPathExe(s.appPath); ok {
		return path
	}
	for _, exe := range s.exes {
		if info, err := os.Stat(exe); err == nil && !info.IsDir() {
			return exe
		}
	}
	return ""
}

func (s browserSpec) installed() bool {
	if s.executable() != "" {
		return true
	}
	for _, dir := range s.profiles {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// browserRunning reports whether any process with this executable's name is
// running. A running Chromium browser forwards new launches to itself and
// ignores --load-extension, so the extension can only be loaded on a fresh
// start.
var browserRunning = func(exe string) bool {
	name := filepath.Base(exe)
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), name) {
			return true
		}
	}
	return false
}

// ExtensionDir is where the unpacked extension lives.
func ExtensionDir(p Paths) string { return filepath.Join(p.Resources, "extension") }

// startBrowser starts a browser and lets it run independently.
var startBrowser = func(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// Launch starts the selected browsers with the unpacked extension loaded.
// A browser that is already running cannot be changed from the command line,
// so it is reported instead of restarted: restarting a browser is the user's
// call, and would lose what they are doing.
func Launch(p Paths, host string) (LaunchResult, error) {
	targets, err := selectBrowsers(host)
	if err != nil {
		return LaunchResult{}, err
	}
	if len(targets) == 0 {
		return LaunchResult{}, errors.New("browsext: no detected browser")
	}
	dir := ExtensionDir(p)
	info, err := os.Stat(dir)
	if err != nil {
		return LaunchResult{}, fmt.Errorf("browsext: extension directory: %w", err)
	}
	if !info.IsDir() {
		return LaunchResult{}, fmt.Errorf("browsext: extension path %q is not a directory", dir)
	}
	return launchSpecs(targets, dir)
}

func launchSpecs(targets []browserSpec, dir string) (LaunchResult, error) {
	result := LaunchResult{}
	for _, spec := range targets {
		exe := spec.executable()
		if spec.manual || exe == "" {
			result.Manual = append(result.Manual, spec.name)
			continue
		}
		if browserRunning(exe) {
			result.Running = append(result.Running, spec.name)
			continue
		}
		if err := startBrowser(exe, "--load-extension="+dir); err != nil {
			return result, fmt.Errorf("browsext: starting %s: %w", spec.name, err)
		}
		result.Started = append(result.Started, spec.name)
	}
	return result, nil
}

// OpenExtensionPage opens each selected browser's extension manager, where a
// user can load the unpacked extension by hand.
func OpenExtensionPage(host string) ([]string, error) {
	targets, err := selectBrowsers(host)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, errors.New("browsext: no detected browser")
	}
	opened := make([]string, 0, len(targets))
	for _, spec := range targets {
		exe := spec.executable()
		if exe == "" || spec.extensionsURL == "" {
			continue
		}
		if err := startBrowser(exe, spec.extensionsURL); err != nil {
			return opened, fmt.Errorf("browsext: starting %s: %w", spec.name, err)
		}
		opened = append(opened, spec.name)
	}
	if len(opened) == 0 {
		return nil, errors.New("browsext: no detected browser")
	}
	return opened, nil
}

// ManifestPath is where the native host manifest is written.
func ManifestPath(p Paths) string {
	return filepath.Join(p.UserData, "native", HostName+".json")
}

// ExtensionID computes the ID Chrome gives an extension from the "key" in
// its manifest: the first 16 bytes of the SHA-256 of the DER public key,
// each nibble mapped to a letter a–p.
func ExtensionID(key string) (string, error) {
	if key == "" {
		return "", errors.New("browsext: the extension manifest has no key")
	}
	der, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("browsext: the extension key is not base64: %w", err)
	}
	sum := sha256.Sum256(der)
	out := make([]byte, 32)
	for i, b := range sum[:16] {
		out[i*2] = 'a' + b>>4
		out[i*2+1] = 'a' + b&0x0f
	}
	return string(out), nil
}

// readKey reads the "key" of the extension manifest.
func readKey(p Paths) (string, error) {
	raw, err := os.ReadFile(filepath.Join(ExtensionDir(p), "manifest.json"))
	if err != nil {
		return "", fmt.Errorf("browsext: reading the extension manifest: %w", err)
	}
	var manifest struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("browsext: parsing the extension manifest: %w", err)
	}
	return manifest.Key, nil
}

// manifestInstalled reports whether the native manifest exists and points at
// this executable. A manifest left by an older install names another path and
// must be rewritten.
func manifestInstalled(p Paths) bool {
	raw, err := os.ReadFile(ManifestPath(p))
	if err != nil {
		return false
	}
	var manifest NativeManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(manifest.Path), filepath.Clean(p.Exe))
}

// registryTarget returns the manifest path a browser's host key holds.
func registryTarget(key string) (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	value, _, err := k.GetStringValue("")
	k.Close()
	if err != nil {
		return "", false
	}
	return value, true
}

func (s browserSpec) registered(p Paths) bool {
	value, ok := registryTarget(s.key)
	return ok && strings.EqualFold(filepath.Clean(value), filepath.Clean(ManifestPath(p)))
}

// Browsers returns every supported browser, the ones this computer has
// installed, and whether the host is registered for each.
func Browsers(p Paths) []Browser {
	specs := browserSpecs()
	manifest := manifestInstalled(p)
	out := make([]Browser, 0, len(specs))
	for _, spec := range specs {
		out = append(out, Browser{
			ID:         spec.id,
			Name:       spec.name,
			Installed:  spec.installed(),
			Registered: manifest && spec.registered(p),
		})
	}
	return out
}

// selectBrowsers resolves a browserHost setting: AutoHost means every
// installed browser, and any other value names exactly one browser.
func selectBrowsers(host string) ([]browserSpec, error) {
	specs := browserSpecs()
	if host == "" || host == AutoHost {
		out := make([]browserSpec, 0, len(specs))
		for _, spec := range specs {
			if spec.installed() {
				out = append(out, spec)
			}
		}
		return out, nil
	}
	for _, spec := range specs {
		if spec.id == host {
			return []browserSpec{spec}, nil
		}
	}
	return nil, fmt.Errorf("browsext: unknown browser %q", host)
}

// Install writes the native manifest and registers it for every supported
// browser. The key is harmless where the extension is not loaded, and
// registering everywhere keeps the extension working when the user switches
// browsers or changes which browser Gale launches.
func Install(p Paths) (State, error) {
	key, err := readKey(p)
	if err != nil {
		return State{}, err
	}
	id, err := ExtensionID(key)
	if err != nil {
		return State{}, err
	}
	manifest := NativeManifest{
		Name:           HostName,
		Description:    "Gale download bridge",
		Path:           p.Exe,
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://" + id + "/"},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return State{}, err
	}
	if err := os.MkdirAll(filepath.Dir(ManifestPath(p)), 0o755); err != nil {
		return State{}, err
	}
	if err := os.WriteFile(ManifestPath(p), raw, 0o644); err != nil {
		return State{}, err
	}
	for _, spec := range browserSpecs() {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, spec.key, registry.SET_VALUE)
		if err != nil {
			return State{}, fmt.Errorf("browsext: %s: %w", spec.key, err)
		}
		err = k.SetStringValue("", ManifestPath(p))
		k.Close()
		if err != nil {
			return State{}, fmt.Errorf("browsext: %s: %w", spec.key, err)
		}
	}
	return Status(p), nil
}

// Uninstall removes the registry keys and the native manifest.
func Uninstall(p Paths) error {
	var errs []error
	for _, spec := range browserSpecs() {
		if err := registry.DeleteKey(registry.CURRENT_USER, spec.key); err != nil && !errors.Is(err, registry.ErrNotExist) {
			errs = append(errs, fmt.Errorf("browsext: %s: %w", spec.key, err))
		}
	}
	if err := os.Remove(ManifestPath(p)); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	os.Remove(filepath.Dir(ManifestPath(p)))
	return errors.Join(errs...)
}

// Status reports whether the host manifest points at this executable and
// every supported browser has it registered.
func Status(p Paths) State {
	st := State{ExtensionDir: ExtensionDir(p), Browsers: Browsers(p)}
	if key, err := readKey(p); err == nil {
		if id, err := ExtensionID(key); err == nil {
			st.ExtensionID = id
		}
	}
	st.HostInstalled = len(st.Browsers) > 0
	for _, browser := range st.Browsers {
		if !browser.Registered {
			st.HostInstalled = false
			break
		}
	}
	return st
}
