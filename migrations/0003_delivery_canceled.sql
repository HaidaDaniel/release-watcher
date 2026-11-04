CREATE TABLE notification_delivery_new (
    id              TEXT PRIMARY KEY,
    event_id        TEXT NOT NULL REFERENCES event (id) ON DELETE CASCADE,
    channel         TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed', 'canceled')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT NOT NULL,
    last_error      TEXT,
    sent_at         TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (event_id, channel)
);

INSERT INTO notification_delivery_new
    (id, event_id, channel, status, attempts, next_attempt_at, last_error, sent_at, created_at, updated_at)
SELECT id, event_id, channel, status, attempts, next_attempt_at, last_error, sent_at, created_at, updated_at
FROM notification_delivery;

DROP TABLE notification_delivery;
ALTER TABLE notification_delivery_new RENAME TO notification_delivery;

CREATE INDEX idx_delivery_due ON notification_delivery (status, next_attempt_at);

UPDATE notification_delivery
SET status = 'canceled', last_error = 'watch already removed'
WHERE status IN ('pending', 'failed')
  AND event_id IN (
    SELECT event.id FROM event JOIN watch ON watch.id = event.watch_id
    WHERE watch.removed_at IS NOT NULL
  );
