package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/store"
	"github.com/HaidaDaniel/release-watcher/migrations"
)

func mustTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

func TestSchemaTwoDatabaseMigratesDeliveriesToCanceledStatus(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range all {
		if migration.Version > 2 {
			continue
		}
		if _, err := db.ExecContext(ctx, migration.SQL); err != nil {
			t.Fatalf("apply migration %d: %v", migration.Version, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, migration.Version, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	const now = "2026-01-01T00:00:00Z"
	inserts := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO work (id, original_title, created_at, updated_at) VALUES ('work_old', 'Old', ?, ?)`, []any{now, now}},
		{`INSERT INTO source (id, kind, uri, external_id, fields_available, created_at, updated_at) VALUES ('src_old', 'rss', 'https://example.test/feed', 'old', '[]', ?, ?)`, []any{now, now}},
		{`INSERT INTO publication (id, work_id, source_id, publication_type, original_url, status, created_at, updated_at) VALUES ('pub_old', 'work_old', 'src_old', 'wn', 'https://example.test/feed', 'unknown', ?, ?)`, []any{now, now}},
		{`INSERT INTO watch (id, publication_id, enabled, interval_seconds, mode, next_check_at, created_at, updated_at) VALUES ('watch_old', 'pub_old', 1, 86400, 'catalog', ?, ?, ?)`, []any{now, now, now}},
		{`INSERT INTO publication (id, work_id, source_id, publication_type, original_url, status, created_at, updated_at) VALUES ('pub_removed', 'work_old', 'src_old', 'wn', 'https://example.test/removed', 'unknown', ?, ?)`, []any{now, now}},
		{`INSERT INTO watch (id, publication_id, enabled, interval_seconds, mode, next_check_at, removed_at, created_at, updated_at) VALUES ('watch_removed', 'pub_removed', 0, 86400, 'catalog', ?, ?, ?, ?)`, []any{now, now, now, now}},
		{`INSERT INTO event (id, event_type, publication_id, watch_id, source_id, external_work_reference, release_ids, payload, dedupe_key, schema_version, detected_at, created_at) VALUES ('evt_old', 'release.added', 'pub_old', 'watch_old', 'src_old', 'feed:old', '[]', '{}', 'old-key', '1.0', ?, ?)`, []any{now, now}},
		{`INSERT INTO event (id, event_type, publication_id, watch_id, source_id, external_work_reference, release_ids, payload, dedupe_key, schema_version, detected_at, created_at) VALUES ('evt_removed', 'release.added', 'pub_removed', 'watch_removed', 'src_old', 'feed:old', '[]', '{}', 'old-key-2', '1.0', ?, ?)`, []any{now, now}},
		{`INSERT INTO notification_delivery (id, event_id, channel, status, attempts, next_attempt_at, created_at, updated_at) VALUES ('del_old', 'evt_old', 'telegram', 'pending', 0, ?, ?, ?)`, []any{now, now, now}},
		{`INSERT INTO notification_delivery (id, event_id, channel, status, attempts, next_attempt_at, created_at, updated_at) VALUES ('del_removed', 'evt_removed', 'telegram', 'failed', 1, ?, ?, ?)`, []any{now, now, now}},
	}
	for _, insert := range inserts {
		if _, err := db.ExecContext(ctx, insert.sql, insert.args...); err != nil {
			t.Fatalf("insert old data: %v\n%s", err, insert.sql)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	version, err := s.SchemaVersion(ctx)
	if err != nil || version != 3 {
		t.Fatalf("schema version = %d err=%v", version, err)
	}
	deliveries, err := s.Queries().ListDeliveriesByEvent(ctx, "evt_old")
	if err != nil || len(deliveries) != 1 || deliveries[0].Status != domain.DeliveryPending {
		t.Fatalf("deliveries after migration = %+v err=%v", deliveries, err)
	}
	removedDeliveries, err := s.Queries().ListDeliveriesByEvent(ctx, "evt_removed")
	if err != nil || len(removedDeliveries) != 1 || removedDeliveries[0].Status != domain.DeliveryCanceled {
		t.Fatalf("removed-watch deliveries after migration = %+v err=%v", removedDeliveries, err)
	}
	canceled, err := s.Queries().CancelPendingDeliveriesForWatch(ctx, "watch_old", mustTime(now))
	if err != nil || canceled != 1 {
		t.Fatalf("cancel = %d err=%v", canceled, err)
	}
	deliveries, err = s.Queries().ListDeliveriesByEvent(ctx, "evt_old")
	if err != nil || len(deliveries) != 1 || deliveries[0].Status != domain.DeliveryCanceled {
		t.Fatalf("deliveries after cancel = %+v err=%v", deliveries, err)
	}
}
