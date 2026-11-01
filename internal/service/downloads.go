// Package service is what the page calls: the downloads, the settings and a
// few system actions, bound with mygo.Bind.
package service

import (
	"context"
	"errors"
	"path/filepath"

	"gale/internal/engine"
	"gale/internal/logx"
	"gale/internal/model"
	"gale/internal/store"

	"github.com/egoist/mygo"
)

// Downloads is the service behind `Downloads.*` in the page.
type Downloads struct {
	ready chan struct{}
	eng   *engine.Engine
	st    *store.Store
}

// NewDownloads returns the service; its methods wait for ready, which the
// app closes once the engine exists.
func NewDownloads(ready chan struct{}) *Downloads {
	return &Downloads{ready: ready}
}

// WireDownloads hands the service the engine and store it works with. It is
// a function rather than a method because mygo binds every exported method
// of a service, and a method taking these types could not cross IPC.
func WireDownloads(d *Downloads, eng *engine.Engine, st *store.Store) {
	d.eng = eng
	d.st = st
}

func (d *Downloads) wait() {
	<-d.ready
}

// List returns every download, newest first.
func (d *Downloads) List() []model.Download {
	d.wait()
	list, err := d.eng.List()
	if err != nil {
		logx.Logger().Warn("listing downloads failed", "error", err)
		return []model.Download{}
	}
	return list
}

// Get returns one download with its segments.
func (d *Downloads) Get(id int64) (model.Detail, error) {
	d.wait()
	return d.eng.Get(id)
}

// Logs returns the tail of a download's log.
func (d *Downloads) Logs(id int64) ([]string, error) {
	d.wait()
	return d.eng.Logs(id), nil
}

// Preview asks the server about a URL before it is downloaded.
func (d *Downloads) Preview(ctx context.Context, req model.AddRequest) (model.Preview, error) {
	d.wait()
	return d.eng.Preview(ctx, req)
}

// Add downloads a URL.
func (d *Downloads) Add(ctx context.Context, req model.AddRequest) (model.Download, error) {
	d.wait()
	return d.eng.Add(ctx, req)
}

// Pause stops a download, keeping what it has for a later resume.
func (d *Downloads) Pause(id int64) error {
	d.wait()
	return d.eng.Pause(id)
}

// Resume continues a paused download.
func (d *Downloads) Resume(id int64) error {
	d.wait()
	return d.eng.Resume(id)
}

// Retry queues a failed download again.
func (d *Downloads) Retry(id int64) error {
	d.wait()
	return d.eng.Retry(id)
}

// Remove deletes a download, and its file when deleteFile says so.
func (d *Downloads) Remove(id int64, deleteFile bool) error {
	d.wait()
	return d.eng.Remove(id, deleteFile)
}

// RemoveMany deletes several downloads.
func (d *Downloads) RemoveMany(ids []int64, deleteFile bool) error {
	d.wait()
	return d.eng.RemoveMany(ids, deleteFile)
}

// PauseAll stops everything that is running or queued.
func (d *Downloads) PauseAll() error {
	d.wait()
	return d.eng.PauseAll()
}

// ResumeAll queues every paused download.
func (d *Downloads) ResumeAll() error {
	d.wait()
	return d.eng.ResumeAll()
}

// Reveal shows a download's file, or its temporary file while it runs, in
// the file manager.
func (d *Downloads) Reveal(id int64) error {
	d.wait()
	row, _, err := d.st.Get(id)
	if err != nil {
		return err
	}
	if row.Status == model.Completed {
		mygo.Shell.ShowItemInFolder(filepath.Join(row.Dir, row.FileName))
		return nil
	}
	if row.TempPath != "" {
		mygo.Shell.ShowItemInFolder(row.TempPath)
		return nil
	}
	return mygo.Shell.OpenPath(row.Dir)
}

// Open opens a completed download with its default application.
func (d *Downloads) Open(id int64) error {
	d.wait()
	row, _, err := d.st.Get(id)
	if err != nil {
		return err
	}
	if row.Status != model.Completed {
		return errors.New("下载尚未完成")
	}
	return mygo.Shell.OpenPath(filepath.Join(row.Dir, row.FileName))
}
