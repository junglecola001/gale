package service

import "github.com/egoist/mygo"

// SystemPaths are the directories the page can reveal from the settings.
type SystemPaths struct {
	Logs string
}

// System is the service behind `System.*`: the few things the page needs
// from the desktop that a web page cannot do itself.
type System struct {
	paths SystemPaths
}

// NewSystem returns the service with the app's directories.
func NewSystem(paths SystemPaths) *System { return &System{paths: paths} }

// ReadClipboardText returns the clipboard's text, for Ctrl+V and the paste
// button: WebView2 does not let the page read it without a permission
// prompt of its own.
func (System) ReadClipboardText() string { return mygo.Clipboard.ReadText() }

// WriteClipboardText puts text on the clipboard, for copying a link or the
// extension's ID.
func (System) WriteClipboardText(text string) { mygo.Clipboard.WriteText(text) }

// OpenExternal opens a URL in the user's browser.
func (System) OpenExternal(url string) error { return mygo.Shell.OpenExternal(url) }

// ShowInFolder reveals a path in the file manager.
func (System) ShowInFolder(path string) error {
	if path == "" {
		return nil
	}
	mygo.Shell.ShowItemInFolder(path)
	return nil
}

// LogsDir is where the app writes its log.
func (s System) LogsDir() string { return s.paths.Logs }

// Version is the app's version.
func (System) Version() string { return mygo.App.Version() }

// DarkMode reports the system appearance, so the page can paint in it before
// the first theme event arrives.
func (System) DarkMode() bool { return mygo.Theme.IsDark() }
