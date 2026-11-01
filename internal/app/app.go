// Package app is Gale's shell: one instance, a window and a tray icon, the
// engine behind them, and the wiring that turns the engine's reports into
// events the page listens to.
package app

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo"

	"gale/internal/bridge"
	"gale/internal/browsext"
	"gale/internal/engine"
	"gale/internal/logx"
	"gale/internal/model"
	"gale/internal/service"
	"gale/internal/settings"
	"gale/internal/store"
)

const (
	appName    = "Gale"
	appVersion = "0.1.0"
)

// Main starts the app, or the native messaging host when a browser started
// the executable.
func Main() int {
	if isNativeHostLaunch(os.Args) {
		return bridge.RunHost(os.Stdin, os.Stdout, hostDirs())
	}
	if os.Getenv("MYGO_GENERATE") == "" && !mygo.App.RequestSingleInstanceLock() {
		return 0
	}

	a := newApp()
	mygo.Bind(a.downloads, a.settings, a.system)
	mygo.App.OnSecondInstance(a.onSecondInstance)
	mygo.App.OnBeforeQuit(a.onBeforeQuit)
	mygo.App.WhenReady(a.start)
	if err := mygo.App.Run(); err != nil {
		log.Println(err)
		return 1
	}
	return 0
}

// isNativeHostLaunch reports whether Chrome or Edge started this executable
// to talk to its extension: it passes the extension's origin and a parent
// window, in an order that is not worth assuming.
func isNativeHostLaunch(args []string) bool {
	for _, arg := range args[1:] {
		if arg == "--native-host" || strings.HasPrefix(arg, "chrome-extension://") {
			return true
		}
	}
	return false
}

func hostDirs() bridge.HostDirs {
	userData, _ := mygo.App.Path(mygo.PathUserData)
	logs, _ := mygo.App.Path(mygo.PathLogs)
	return bridge.HostDirs{UserData: userData, Logs: logs}
}

type paths struct {
	userData  string
	logs      string
	downloads string
	resources string
	exe       string
}

func resolvePaths() paths {
	p := paths{}
	p.userData, _ = mygo.App.Path(mygo.PathUserData)
	p.logs, _ = mygo.App.Path(mygo.PathLogs)
	p.downloads, _ = mygo.App.Path(mygo.PathDownloads)
	p.resources, _ = mygo.App.Path(mygo.PathResources)
	p.exe, _ = os.Executable()
	return p
}

type app struct {
	ready chan struct{}

	downloads *service.Downloads
	settings  *service.Settings
	system    *service.System

	paths paths
	st    *store.Store
	set   *settings.Store
	eng   *engine.Engine
	srv   *bridge.Server
	win   *mygo.Window
	// takeoverWin shows what the browser handed over. It is created on the
	// first download and reused for the next ones.
	takeoverWin *mygo.Window
	tray        *mygo.Tray

	limitItems map[int64]*mygo.MenuItem

	mu               sync.Mutex
	quitting         bool
	closeToTray      bool
	notifyOnComplete bool
	progress         map[int64]model.Progress
}

func newApp() *app {
	ready := make(chan struct{})
	paths := resolvePaths()
	return &app{
		ready:     ready,
		downloads: service.NewDownloads(ready),
		settings:  service.NewSettings(ready),
		system:    service.NewSystem(service.SystemPaths{Logs: paths.logs}),
		paths:     paths,
		progress:  make(map[int64]model.Progress),
	}
}

// start opens everything the page needs, then lets its calls through.
func (a *app) start() {
	if os.Getenv("MYGO_GENERATE") != "" {
		return
	}
	if err := logx.Init(filepath.Join(a.paths.logs, "gale.log")); err != nil {
		log.Println("log file:", err)
	}

	st, err := store.Open(filepath.Join(a.paths.userData, "gale.db"))
	if err != nil {
		mygo.Dialog.Error(appName, "无法打开数据库："+err.Error())
		mygo.App.Quit()
		return
	}
	a.st = st

	set, err := settings.Load(st, settings.Defaults(a.paths.downloads))
	if err != nil {
		mygo.Dialog.Error(appName, "无法读取设置："+err.Error())
		mygo.App.Quit()
		return
	}
	a.set = set

	a.eng = engine.New(st, set, a.hooks())
	if err := a.applySettings(set.Get()); err != nil {
		logx.Logger().Warn("applying settings failed", "error", err)
	}
	if err := a.eng.Start(); err != nil {
		logx.Logger().Warn("starting downloads failed", "error", err)
	}

	if srv, err := bridge.Start(a.paths.userData, appVersion,
		a.takeover,
		func() bool { return a.set.Get().BrowserEnabled },
	); err != nil {
		logx.Logger().Warn("browser bridge unavailable", "error", err)
	} else {
		a.srv = srv
	}

	if set.Get().BrowserEnabled {
		if _, err := browsext.Install(a.browsextPaths()); err != nil {
			logx.Logger().Warn("browser host install failed", "error", err)
		}
	}

	mygo.Theme.OnUpdated(a.onThemeUpdated)
	a.onThemeUpdated()
	a.startTray()
	a.createWindow()

	service.WireDownloads(a.downloads, a.eng, st)
	service.WireSettings(a.settings, service.SettingsDeps{
		Store:       st,
		Settings:    set,
		Browser:     a.browserStatus,
		InstallHost: func() (browsext.State, error) { return browsext.Install(a.browsextPaths()) },
		LaunchExtension: func(host string) (browsext.LaunchResult, error) {
			return browsext.Launch(a.browsextPaths(), host)
		},
		OpenExtensionPage: browsext.OpenExtensionPage,
		UninstallHost:     func() error { return browsext.Uninstall(a.browsextPaths()) },
		Apply:             a.applySettings,
	})
	close(a.ready)
}

func (a *app) browsextPaths() browsext.Paths {
	return browsext.Paths{Exe: a.paths.exe, Resources: a.paths.resources, UserData: a.paths.userData}
}

// applySettings carries a changed setting to the engine, the theme, the tray
// and the window.
func (a *app) applySettings(d settings.Data) error {
	switch {
	case a.eng != nil:
		a.eng.SetMaxConcurrent(d.MaxConcurrent)
		a.eng.SetSpeedLimit(d.SpeedLimit)
	}
	switch d.Theme {
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
	mygo.App.SetOpenAtLogin(d.OpenAtLogin)
	a.mu.Lock()
	a.closeToTray = d.CloseToTray
	a.notifyOnComplete = d.NotifyOnComplete
	a.mu.Unlock()
	return nil
}

func (a *app) browserStatus() service.BrowserStatus {
	st := browsext.Status(a.browsextPaths())
	out := service.BrowserStatus{
		HostInstalled: st.HostInstalled,
		ExtensionID:   st.ExtensionID,
		ExtensionDir:  st.ExtensionDir,
		Browsers:      st.Browsers,
	}
	if a.srv != nil {
		out.Connected = a.srv.Connected()
		out.LastPing = a.srv.LastPing()
	}
	return out
}

// hooks turns the engine's reports into events and window updates.
func (a *app) hooks() engine.Hooks {
	return engine.Hooks{
		Progress: func(p model.Progress) {
			a.mu.Lock()
			a.progress[p.ID] = p
			a.mu.Unlock()
			service.DownloadProgress.Broadcast(p)
			a.updateProgressBar()
		},
		Changed: func(d model.Download) {
			service.DownloadUpdated.Broadcast(d)
		},
		Finished: func(f model.Finished) {
			a.mu.Lock()
			delete(a.progress, f.ID)
			a.mu.Unlock()
			service.DownloadFinished.Broadcast(f)
			a.updateProgressBar()
			a.onFinished(f)
		},
		Stats: func(s model.Stats) {
			service.StatsUpdated.Broadcast(s)
			a.updateTrayTooltip(s)
		},
	}
}

func (a *app) updateProgressBar() {
	a.mu.Lock()
	var done, total int64
	for _, p := range a.progress {
		if p.Total > 0 {
			done += p.Downloaded
			total += p.Total
		}
	}
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	if total <= 0 {
		win.SetProgressBar(mygo.ProgressBar{})
		return
	}
	win.SetProgressBar(mygo.ProgressBar{State: mygo.ProgressNormal, Value: float64(done) / float64(total)})
}

func (a *app) updateTrayTooltip(s model.Stats) {
	tray := a.tray
	if tray == nil {
		return
	}
	if s.Speed > 0 {
		tray.SetToolTip(appName + " · " + humanSpeed(s.Speed))
		return
	}
	tray.SetToolTip(appName)
}

func humanSpeed(bytesPerSecond int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	value := float64(bytesPerSecond)
	i := 0
	for value >= 1000 && i < len(units)-1 {
		value /= 1000
		i++
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + " " + units[i] + "/s"
}

// onFinished reports a download's end: a notification when the window is not
// in front, and a flashing taskbar button.
func (a *app) onFinished(f model.Finished) {
	a.mu.Lock()
	win := a.win
	notify := a.notifyOnComplete
	a.mu.Unlock()
	if win == nil {
		return
	}
	if f.Status == model.Completed {
		win.FlashFrame(true)
		if notify && !win.IsFocused() && mygo.NotificationsSupported() {
			n := mygo.NewNotification(mygo.NotificationOptions{
				ID:    "gale-download-" + strconv.FormatInt(f.ID, 10),
				Title: "下载完成",
				Body:  filepath.Base(f.FilePath),
			})
			if err := n.Show(); err != nil {
				logx.Logger().Warn("notification failed", "error", err)
			}
			n.OnClick(func() { a.showWindow() })
		}
		return
	}
	if notify && !win.IsFocused() && mygo.NotificationsSupported() {
		n := mygo.NewNotification(mygo.NotificationOptions{
			ID:    "gale-download-" + strconv.FormatInt(f.ID, 10),
			Title: "下载失败",
			Body:  f.ErrorMessage,
		})
		if err := n.Show(); err != nil {
			logx.Logger().Warn("notification failed", "error", err)
		}
		n.OnClick(func() { a.showWindow() })
	}
}

func (a *app) onThemeUpdated() {
	theme := "light"
	if mygo.Theme.IsDark() {
		theme = "dark"
	}
	service.ThemeChanged.Broadcast(theme)
}

func (a *app) startTray() {
	icon, err := os.ReadFile(filepath.Join(a.paths.resources, "icon.png"))
	if err != nil {
		logx.Logger().Warn("tray icon missing", "error", err)
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "main",
		Icon:    icon,
		ToolTip: appName,
		Menu:    a.trayMenu(),
	})
	if err != nil {
		logx.Logger().Warn("tray unavailable", "error", err)
		a.mu.Lock()
		a.closeToTray = false
		a.mu.Unlock()
		return
	}
	a.tray = tray
}

func (a *app) trayMenu() *mygo.Menu {
	limitItem := func(label string, bytesPerSecond int64) *mygo.MenuItem {
		return &mygo.MenuItem{
			ID:      "limit-" + label,
			Label:   label,
			Type:    mygo.MenuItemRadio,
			Checked: a.set != nil && a.set.Get().SpeedLimit == bytesPerSecond,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				a.setLimit(bytesPerSecond)
			},
		}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{ID: "show", Label: "显示主窗口", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		mygo.Separator(),
		{ID: "resume-all", Label: "全部开始", Click: func(*mygo.MenuItem, *mygo.Window) {
			if a.eng != nil {
				a.eng.ResumeAll()
			}
		}},
		{ID: "pause-all", Label: "全部暂停", Click: func(*mygo.MenuItem, *mygo.Window) {
			if a.eng != nil {
				a.eng.PauseAll()
			}
		}},
		{ID: "limit", Label: "限速", Submenu: []*mygo.MenuItem{
			limitItem("不限速", 0),
			limitItem("1 MB/s", 1_000_000),
			limitItem("5 MB/s", 5_000_000),
			limitItem("10 MB/s", 10_000_000),
			limitItem("20 MB/s", 20_000_000),
		}},
		mygo.Separator(),
		{ID: "quit", Label: "退出", Role: mygo.RoleQuit, Click: func(*mygo.MenuItem, *mygo.Window) {
			mygo.App.Quit()
		}},
	})
}

// setLimit changes the global speed limit from the tray, and remembers it in
// the settings so the settings page shows it.
func (a *app) setLimit(bytesPerSecond int64) {
	if a.set == nil {
		return
	}
	d := a.set.Get()
	d.SpeedLimit = bytesPerSecond
	if _, err := a.set.Update(d); err != nil {
		logx.Logger().Warn("saving the speed limit failed", "error", err)
		return
	}
	a.applySettings(d)
	if a.eng != nil {
		a.eng.SetSpeedLimit(bytesPerSecond)
	}
	for value, item := range a.limitItems {
		item.SetChecked(value == bytesPerSecond)
	}
}

func (a *app) createWindow() {
	hidden := mygo.App.WasOpenedAtLogin()
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           appName,
		Frameless:       true,
		Width:           1180,
		Height:          760,
		MinWidth:        960,
		MinHeight:       620,
		StateKey:        "main",
		BackgroundColor: "light-dark(#f6f7f9, #0b0c0f)",
		URL:             "/",
		Hidden:          hidden,
	})
	a.mu.Lock()
	a.win = win
	a.mu.Unlock()

	win.OnClose(func(e *mygo.CloseEvent) {
		a.mu.Lock()
		keep := a.closeToTray && !a.quitting && a.tray != nil
		a.mu.Unlock()
		if keep {
			e.PreventDefault()
			win.Hide()
		}
	})
	win.OnFocus(func() { win.FlashFrame(false) })
	if !hidden {
		win.Show()
	}
}

func (a *app) showWindow() {
	win := a.win
	if win == nil {
		return
	}
	win.Show()
	win.Focus()
}

// takeover takes a download the browser hands over, and shows it in a window
// of its own: the browser's own download stops, so the page says where the
// bytes went.
func (a *app) takeover(req model.AddRequest) (int64, error) {
	id, err := a.eng.AddQuick(req)
	if err != nil {
		return id, err
	}
	a.showTakeover(id)
	return id, nil
}

// showTakeover opens the takeover window on id, or points the open one at it.
func (a *app) showTakeover(id int64) {
	page := "/?takeover=" + strconv.FormatInt(id, 10)
	a.mu.Lock()
	win := a.takeoverWin
	a.mu.Unlock()
	if win != nil && !win.IsDestroyed() {
		if err := win.Page().LoadURL(page); err != nil {
			logx.Logger().Warn("loading the takeover page failed", "error", err)
		}
		if win.IsMinimized() {
			win.Restore()
		}
		win.Show()
		win.Focus()
		return
	}
	win = mygo.NewWindow(mygo.WindowOptions{
		Title:           "下载详情",
		Frameless:       true,
		Width:           480,
		Height:          620,
		MinWidth:        400,
		MinHeight:       380,
		BackgroundColor: "light-dark(#f6f7f9, #0b0c0f)",
		URL:             page,
	})
	a.mu.Lock()
	a.takeoverWin = win
	a.mu.Unlock()
	win.OnClosed(func() {
		a.mu.Lock()
		if a.takeoverWin == win {
			a.takeoverWin = nil
		}
		a.mu.Unlock()
	})
}

// onSecondInstance shows the window, and opens the add dialog for a URL the
// second instance was started with.
func (a *app) onSecondInstance(args []string, workingDir string) {
	a.showWindow()
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
			service.UiAdd.Broadcast(model.AddRequest{URL: arg})
			return
		}
	}
}

func (a *app) onBeforeQuit(e *mygo.QuitEvent) {
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	if a.eng != nil {
		a.eng.Shutdown()
	}
	if a.srv != nil {
		a.srv.Close()
	}
	if a.st != nil {
		a.st.Close()
	}
}
