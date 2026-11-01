package service

import (
	"context"
	"errors"

	"gale/internal/browsext"
	"gale/internal/settings"
	"gale/internal/store"

	"github.com/egoist/mygo"
)

// BrowserStatus is what the settings page shows about the browser
// integration.
type BrowserStatus struct {
	HostInstalled bool               `json:"hostInstalled"`
	ExtensionID   string             `json:"extensionId"`
	ExtensionDir  string             `json:"extensionDir"`
	Connected     bool               `json:"connected"`
	LastPing      int64              `json:"lastPing"`
	Browsers      []browsext.Browser `json:"browsers"`
}

// SettingsDeps is what the settings service needs from the app.
type SettingsDeps struct {
	Store    *store.Store
	Settings *settings.Store
	// Browser reports the host and extension state.
	Browser func() BrowserStatus
	// InstallHost writes and registers the native messaging host for every
	// supported browser.
	InstallHost func() (browsext.State, error)
	// LaunchExtension starts the selected browsers with the unpacked
	// extension loaded.
	LaunchExtension func(host string) (browsext.LaunchResult, error)
	// OpenExtensionPage opens the selected browsers' extension managers.
	OpenExtensionPage func(host string) ([]string, error)
	// UninstallHost removes it again.
	UninstallHost func() error
	// Apply carries a changed setting to the engine, the theme and the tray.
	Apply func(settings.Data) error
}

// Settings is the service behind `Settings.*` in the page.
type Settings struct {
	ready chan struct{}
	deps  SettingsDeps
}

// NewSettings returns the service; its methods wait for ready.
func NewSettings(ready chan struct{}) *Settings {
	return &Settings{ready: ready}
}

// WireSettings hands the service what it works with, as a function because
// mygo binds every exported method of a service.
func WireSettings(s *Settings, deps SettingsDeps) { s.deps = deps }

func (s *Settings) wait() { <-s.ready }

// Get returns the current settings.
func (s *Settings) Get() settings.Data {
	s.wait()
	return s.deps.Settings.Get()
}

// Update validates and stores the settings, then applies them.
func (s *Settings) Update(d settings.Data) (settings.Data, error) {
	s.wait()
	before := s.deps.Settings.Get()
	after, err := s.deps.Settings.Update(d)
	if err != nil {
		return before, err
	}
	if after.BrowserEnabled {
		if !before.BrowserEnabled {
			if _, err := s.deps.InstallHost(); err != nil {
				reverted := after
				reverted.BrowserEnabled = before.BrowserEnabled
				reverted.BrowserHost = before.BrowserHost
				s.deps.Settings.Update(reverted)
				return s.deps.Settings.Get(), err
			}
		}
	} else if before.BrowserEnabled {
		if err := s.deps.UninstallHost(); err != nil {
			return after, err
		}
	}
	if err := s.deps.Apply(after); err != nil {
		return after, err
	}
	return after, nil
}

// ChooseDir asks the user for a directory; "" when they cancel.
func (s *Settings) ChooseDir(ctx context.Context) (string, error) {
	s.wait()
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:     "选择下载目录",
		Directory: true,
	})
	if err != nil {
		if errors.Is(err, mygo.ErrCanceled) {
			return "", nil
		}
		return "", err
	}
	if len(paths) == 0 {
		return "", nil
	}
	return paths[0], nil
}

// BrowserStatus reports whether the host is installed and the extension is
// connected.
func (s *Settings) BrowserStatus() BrowserStatus {
	s.wait()
	return s.deps.Browser()
}

// InstallBrowserHost turns the integration on and force-installs the native
// host, so the button also repairs an install that is already enabled.
func (s *Settings) InstallBrowserHost() (BrowserStatus, error) {
	s.wait()
	d := s.deps.Settings.Get()
	if d.BrowserEnabled {
		if _, err := s.deps.InstallHost(); err != nil {
			return s.deps.Browser(), err
		}
		return s.deps.Browser(), nil
	}
	d.BrowserEnabled = true
	if _, err := s.Update(d); err != nil {
		return s.deps.Browser(), err
	}
	return s.deps.Browser(), nil
}

// LaunchBrowserExtension loads the unpacked extension in the selected
// browsers, starting a browser when it is not running.
func (s *Settings) LaunchBrowserExtension() (browsext.LaunchResult, error) {
	s.wait()
	return s.deps.LaunchExtension(s.deps.Settings.Get().BrowserHost)
}

// OpenExtensionPage opens the selected browsers' extension managers, for a
// browser that cannot load the unpacked extension from the command line.
func (s *Settings) OpenExtensionPage() ([]string, error) {
	s.wait()
	return s.deps.OpenExtensionPage(s.deps.Settings.Get().BrowserHost)
}

// SetBrowserEnabled turns the browser integration on or off, installing or
// removing the native host with it. A failed install leaves the setting as
// it was.
func (s *Settings) SetBrowserEnabled(enabled bool) (BrowserStatus, error) {
	s.wait()
	d := s.deps.Settings.Get()
	if d.BrowserEnabled == enabled {
		return s.deps.Browser(), nil
	}
	d.BrowserEnabled = enabled
	if _, err := s.Update(d); err != nil {
		return s.deps.Browser(), err
	}
	return s.deps.Browser(), nil
}
