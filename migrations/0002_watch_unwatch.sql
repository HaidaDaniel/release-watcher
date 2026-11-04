ALTER TABLE watch ADD COLUMN removed_at TEXT;

CREATE INDEX idx_watch_removed ON watch (removed_at, enabled);
