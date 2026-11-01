package service

import (
	"gale/internal/model"

	"github.com/egoist/mygo"
)

// The events the Go side sends the page. `mygo generate` turns them into
// typed `events.*.on(listener)` functions in src/mygo.ts.
var (
	// DownloadUpdated is a download whose status, name or error changed.
	DownloadUpdated = mygo.NewEvent[model.Download]("downloads:updated")
	// DownloadProgress is a running download's bytes, speed and ETA.
	DownloadProgress = mygo.NewEvent[model.Progress]("downloads:progress")
	// DownloadFinished is a download that completed or failed.
	DownloadFinished = mygo.NewEvent[model.Finished]("downloads:finished")
	// StatsUpdated is the engine's totals, once a second.
	StatsUpdated = mygo.NewEvent[model.Stats]("stats:updated")
	// UiAdd asks the page to open the add dialog, with a URL when a second
	// instance was started with one.
	UiAdd = mygo.NewEvent[model.AddRequest]("ui:add")
	// ThemeChanged follows the system appearance: "dark" or "light".
	ThemeChanged = mygo.NewEvent[string]("ui:theme")
)
