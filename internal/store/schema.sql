PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS downloads (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  url TEXT NOT NULL, final_url TEXT NOT NULL DEFAULT '',
  file_name TEXT NOT NULL, dir TEXT NOT NULL, temp_path TEXT NOT NULL,
  total_bytes INTEGER NOT NULL DEFAULT -1, downloaded INTEGER NOT NULL DEFAULT 0,
  etag TEXT NOT NULL DEFAULT '', last_modified TEXT NOT NULL DEFAULT '',
  mime TEXT NOT NULL DEFAULT '', status TEXT NOT NULL,
  error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
  resumable INTEGER NOT NULL DEFAULT 0, auto_resume INTEGER NOT NULL DEFAULT 0,
  needs_probe INTEGER NOT NULL DEFAULT 0,
  connections INTEGER NOT NULL DEFAULT 8, category TEXT NOT NULL DEFAULT 'other',
  referrer TEXT NOT NULL DEFAULT '', user_agent TEXT NOT NULL DEFAULT '',
  cookies TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL, started_at INTEGER NOT NULL DEFAULT 0,
  finished_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_downloads_status ON downloads(status, created_at);
CREATE TABLE IF NOT EXISTS segments (
  download_id INTEGER NOT NULL REFERENCES downloads(id) ON DELETE CASCADE,
  idx INTEGER NOT NULL, start_off INTEGER NOT NULL,
  end_off INTEGER NOT NULL, pos INTEGER NOT NULL,
  PRIMARY KEY (download_id, idx)
);
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
PRAGMA user_version = 1;
