CREATE TABLE work (
    id             TEXT PRIMARY KEY,
    original_title TEXT NOT NULL,
    title_ru       TEXT,
    title_en       TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE TABLE source (
    id               TEXT PRIMARY KEY,
    kind             TEXT NOT NULL CHECK (kind IN ('naro', 'rss', 'atom')),
    uri              TEXT NOT NULL,
    external_id      TEXT NOT NULL,
    fields_available TEXT NOT NULL DEFAULT '[]',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    UNIQUE (kind, external_id)
);

CREATE INDEX idx_source_uri ON source (uri);

CREATE TABLE publication (
    id                TEXT PRIMARY KEY,
    work_id           TEXT NOT NULL REFERENCES work (id) ON DELETE CASCADE,
    source_id         TEXT NOT NULL REFERENCES source (id) ON DELETE CASCADE,
    publication_type  TEXT NOT NULL DEFAULT 'unknown' CHECK (publication_type IN ('wn', 'ln', 'translation', 'other', 'unknown')),
    original_url      TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('ongoing', 'completed', 'unknown')),
    status_source     TEXT,
    last_published_at TEXT,
    last_checked_at   TEXT,
    next_check_at     TEXT,
    release_count     INTEGER,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    UNIQUE (source_id, original_url)
);

CREATE INDEX idx_publication_work ON publication (work_id);
CREATE INDEX idx_publication_source ON publication (source_id);

CREATE TABLE watch (
    id                      TEXT PRIMARY KEY,
    publication_id          TEXT NOT NULL UNIQUE REFERENCES publication (id) ON DELETE CASCADE,
    enabled                 INTEGER NOT NULL DEFAULT 1,
    interval_seconds        INTEGER NOT NULL,
    baseline_established_at TEXT,
    baseline_count          INTEGER,
    mode                    TEXT NOT NULL DEFAULT 'catalog' CHECK (mode IN ('catalog', 'count_only')),
    next_check_at           TEXT NOT NULL,
    last_check_at           TEXT,
    last_success_at         TEXT,
    last_error_category     TEXT,
    consecutive_failures    INTEGER NOT NULL DEFAULT 0,
    paused_at               TEXT,
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);

CREATE INDEX idx_watch_due ON watch (enabled, next_check_at);

CREATE TABLE release (
    id                TEXT PRIMARY KEY,
    publication_id    TEXT NOT NULL REFERENCES publication (id) ON DELETE CASCADE,
    external_id       TEXT NOT NULL,
    kind              TEXT NOT NULL CHECK (kind IN ('chapter', 'volume', 'book', 'entry', 'other')),
    number            TEXT,
    title             TEXT,
    url               TEXT,
    published_at      TEXT,
    source_updated_at TEXT,
    content_hash      TEXT NOT NULL,
    is_baseline       INTEGER NOT NULL DEFAULT 0,
    first_seen_at     TEXT NOT NULL,
    last_seen_at      TEXT NOT NULL,
    removed_at        TEXT,
    UNIQUE (publication_id, external_id)
);

CREATE INDEX idx_release_publication ON release (publication_id, first_seen_at);
CREATE INDEX idx_release_active ON release (publication_id, removed_at);

CREATE TABLE scan_run (
    id              TEXT PRIMARY KEY,
    watch_id        TEXT NOT NULL REFERENCES watch (id) ON DELETE CASCADE,
    publication_id  TEXT NOT NULL REFERENCES publication (id) ON DELETE CASCADE,
    started_at      TEXT NOT NULL,
    finished_at     TEXT NOT NULL,
    result          TEXT NOT NULL CHECK (result IN ('baseline', 'ok', 'unchanged', 'error')),
    events_created  INTEGER NOT NULL DEFAULT 0,
    error_code      TEXT,
    error_message   TEXT,
    duration_ms     INTEGER NOT NULL
);

CREATE INDEX idx_scan_run_watch ON scan_run (watch_id, finished_at);

CREATE TABLE event (
    id                      TEXT PRIMARY KEY,
    event_type              TEXT NOT NULL CHECK (event_type IN ('release.added', 'release.updated', 'release.removed', 'publication.metadata_changed', 'publication.status_changed', 'release.count_changed')),
    publication_id          TEXT NOT NULL REFERENCES publication (id) ON DELETE CASCADE,
    watch_id                TEXT NOT NULL REFERENCES watch (id) ON DELETE CASCADE,
    source_id               TEXT NOT NULL REFERENCES source (id) ON DELETE CASCADE,
    external_work_reference TEXT NOT NULL,
    release_ids             TEXT NOT NULL DEFAULT '[]',
    previous_count          INTEGER,
    current_count           INTEGER,
    count_delta             INTEGER,
    payload                 TEXT NOT NULL,
    dedupe_key              TEXT NOT NULL UNIQUE,
    schema_version          TEXT NOT NULL,
    detected_at             TEXT NOT NULL,
    published_at            TEXT,
    url                     TEXT,
    created_at              TEXT NOT NULL
);

CREATE INDEX idx_event_publication ON event (publication_id, detected_at, id);
CREATE INDEX idx_event_detected ON event (detected_at, id);

CREATE TABLE notification_delivery (
    id              TEXT PRIMARY KEY,
    event_id        TEXT NOT NULL REFERENCES event (id) ON DELETE CASCADE,
    channel         TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT NOT NULL,
    last_error      TEXT,
    sent_at         TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (event_id, channel)
);

CREATE INDEX idx_delivery_due ON notification_delivery (status, next_attempt_at);

CREATE TABLE setting (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
