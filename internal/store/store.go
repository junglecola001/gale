// Package store keeps downloads, their segments and the settings in SQLite.
//
// One connection, WAL, and a schema version: the engine writes a download's
// segment cursors once a second, so a crash loses at most that much.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	"gale/internal/model"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Store is the app's database.
type Store struct {
	db *sql.DB
}

const columns = `id, url, final_url, file_name, dir, temp_path, total_bytes,
downloaded, etag, last_modified, mime, status, error_code, error_message,
resumable, auto_resume, needs_probe, connections, category, referrer,
user_agent, cookies, created_at, started_at, finished_at`

// Open opens the database at path, creating and migrating it as needed.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: busy_timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: foreign_keys: %w", err)
	}
	s := &Store{db: db}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: user_version: %w", err)
	}
	switch {
	case version > 1:
		db.Close()
		return nil, fmt.Errorf("store: schema version %d is newer than this app understands", version)
	case version == 1:
		return s, nil
	}
	if err := s.execScript(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return s, nil
}

// execScript runs each statement of script in turn. Some of them (PRAGMA
// journal_mode) return a row, so they run through a query and their result
// is dropped.
func (s *Store) execScript(script string) error {
	for _, stmt := range strings.Split(script, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		rows, err := s.db.Query(stmt)
		if err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
		for rows.Next() {
			// Values of PRAGMAs are not interesting.
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

type scanner interface {
	Scan(dest ...any) error
}

func scanDownload(sc scanner) (model.Download, error) {
	var d model.Download
	err := sc.Scan(&d.ID, &d.URL, &d.FinalURL, &d.FileName, &d.Dir, &d.TempPath,
		&d.TotalBytes, &d.Downloaded, &d.ETag, &d.LastModified, &d.MIME,
		&d.Status, &d.ErrorCode, &d.ErrorMessage, &d.Resumable, &d.AutoResume,
		&d.NeedsProbe, &d.Connections, &d.Category, &d.Referrer, &d.UserAgent,
		&d.Cookies, &d.CreatedAt, &d.StartedAt, &d.FinishedAt)
	return d, err
}

// List returns every download, newest first.
func (s *Store) List() ([]model.Download, error) {
	rows, err := s.db.Query("SELECT " + columns + " FROM downloads ORDER BY created_at DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Download{}
	for rows.Next() {
		d, err := scanDownload(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Get returns one download with its segments.
func (s *Store) Get(id int64) (model.Download, []model.Segment, error) {
	d, err := scanDownload(s.db.QueryRow("SELECT "+columns+" FROM downloads WHERE id = ?", id))
	if err != nil {
		return model.Download{}, nil, err
	}
	rows, err := s.db.Query("SELECT idx, start_off, end_off, pos FROM segments WHERE download_id = ? ORDER BY idx", id)
	if err != nil {
		return model.Download{}, nil, err
	}
	defer rows.Close()
	segs := []model.Segment{}
	for rows.Next() {
		var seg model.Segment
		if err := rows.Scan(&seg.Index, &seg.Start, &seg.End, &seg.Pos); err != nil {
			return model.Download{}, nil, err
		}
		segs = append(segs, seg)
	}
	return d, segs, rows.Err()
}

// Insert stores d and its segments, and returns the new row's id.
func (s *Store) Insert(d *model.Download, segs []model.Segment) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO downloads (url, final_url, file_name, dir,
temp_path, total_bytes, downloaded, etag, last_modified, mime, status,
error_code, error_message, resumable, auto_resume, needs_probe, connections,
category, referrer, user_agent, cookies, created_at, started_at, finished_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.URL, d.FinalURL, d.FileName, d.Dir, d.TempPath, d.TotalBytes,
		d.Downloaded, d.ETag, d.LastModified, d.MIME, d.Status, d.ErrorCode,
		d.ErrorMessage, d.Resumable, d.AutoResume, d.NeedsProbe, d.Connections,
		d.Category, d.Referrer, d.UserAgent, d.Cookies, d.CreatedAt,
		d.StartedAt, d.FinishedAt)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, seg := range segs {
		if _, err := tx.Exec("INSERT INTO segments (download_id, idx, start_off, end_off, pos) VALUES (?,?,?,?,?)",
			id, seg.Index, seg.Start, seg.End, seg.Pos); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	d.ID = id
	return id, nil
}

// Update writes every column of d.
func (s *Store) Update(d *model.Download) error {
	_, err := s.db.Exec(`UPDATE downloads SET url=?, final_url=?, file_name=?,
dir=?, temp_path=?, total_bytes=?, downloaded=?, etag=?, last_modified=?,
mime=?, status=?, error_code=?, error_message=?, resumable=?, auto_resume=?,
needs_probe=?, connections=?, category=?, referrer=?, user_agent=?, cookies=?,
created_at=?, started_at=?, finished_at=? WHERE id=?`,
		d.URL, d.FinalURL, d.FileName, d.Dir, d.TempPath, d.TotalBytes,
		d.Downloaded, d.ETag, d.LastModified, d.MIME, d.Status, d.ErrorCode,
		d.ErrorMessage, d.Resumable, d.AutoResume, d.NeedsProbe, d.Connections,
		d.Category, d.Referrer, d.UserAgent, d.Cookies, d.CreatedAt,
		d.StartedAt, d.FinishedAt, d.ID)
	return err
}

// SaveProgress stores how far a download got in one transaction: the byte
// count and each segment's cursor.
func (s *Store) SaveProgress(id, downloaded int64, segs []model.Segment) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE downloads SET downloaded = ? WHERE id = ?", downloaded, id); err != nil {
		return err
	}
	for _, seg := range segs {
		if _, err := tx.Exec(`INSERT INTO segments (download_id, idx, start_off, end_off, pos)
VALUES (?,?,?,?,?) ON CONFLICT(download_id, idx) DO UPDATE SET
start_off=excluded.start_off, end_off=excluded.end_off, pos=excluded.pos`,
			id, seg.Index, seg.Start, seg.End, seg.Pos); err != nil {
			return err
		}
	}
	keep := make([]any, 0, len(segs)+1)
	keep = append(keep, id)
	placeholders := make([]string, 0, len(segs))
	for _, seg := range segs {
		placeholders = append(placeholders, "?")
		keep = append(keep, seg.Index)
	}
	query := "DELETE FROM segments WHERE download_id = ?"
	if len(placeholders) > 0 {
		query += " AND idx NOT IN (" + strings.Join(placeholders, ",") + ")"
	}
	if _, err := tx.Exec(query, keep...); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete removes a download and its segments.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM downloads WHERE id = ?", id)
	return err
}

// MarkInterrupted turns the downloads that were running when the app stopped
// into paused ones that resume at the next start.
func (s *Store) MarkInterrupted() error {
	_, err := s.db.Exec("UPDATE downloads SET status = ?, auto_resume = 1 WHERE status = ?",
		model.Paused, model.Downloading)
	return err
}

// AutoResumeIDs lists the paused downloads that asked to resume.
func (s *Store) AutoResumeIDs() ([]int64, error) {
	rows, err := s.db.Query("SELECT id FROM downloads WHERE status = ? AND auto_resume = 1 ORDER BY created_at, id",
		model.Paused)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// NextQueued returns the oldest queued download, or nil when none waits.
func (s *Store) NextQueued() (*model.Download, error) {
	d, err := scanDownload(s.db.QueryRow("SELECT "+columns+
		" FROM downloads WHERE status = ? ORDER BY created_at, id LIMIT 1", model.Queued))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Count counts the downloads in a status.
func (s *Store) Count(status model.Status) (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM downloads WHERE status = ?", status).Scan(&n)
	return n, err
}

// GetSetting reads a setting, false when it was never written.
func (s *Store) GetSetting(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetSetting writes a setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
