package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/id"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

func openStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release-watcher.db")
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

type fixture struct {
	work        *domain.Work
	source      *domain.Source
	publication *domain.Publication
	watch       *domain.Watch
}

func createFixture(t *testing.T, s *store.Store, suffix string, enabled bool, nextCheck time.Time) fixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	ru := "Русское название"
	f := fixture{
		work: &domain.Work{
			ID:            id.New("work"),
			OriginalTitle: "Original " + suffix,
			TitleRU:       &ru,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		source: &domain.Source{
			ID:              id.New("src"),
			Kind:            domain.SourceNaro,
			URI:             "https://ncode.syosetu.com/" + suffix + "/",
			ExternalID:      suffix,
			FieldsAvailable: []string{"title", "release_catalog"},
			CreatedAt:       now,
			UpdatedAt:       now,
		},
	}
	f.publication = &domain.Publication{
		ID:           id.New("pub"),
		WorkID:       f.work.ID,
		SourceID:     f.source.ID,
		Type:         domain.PublicationWN,
		OriginalURL:  f.source.URI,
		Status:       domain.StatusCompleted,
		StatusSource: "naro:end",
		NextCheckAt:  &nextCheck,
		Mode:         domain.ModeCatalog,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	f.watch = &domain.Watch{
		ID:              id.New("watch"),
		PublicationID:   f.publication.ID,
		Enabled:         enabled,
		IntervalSeconds: 86400,
		Mode:            domain.ModeCatalog,
		NextCheckAt:     nextCheck,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.InTx(ctx, func(q *store.Queries) error {
		if err := q.CreateWork(ctx, f.work); err != nil {
			return err
		}
		if err := q.CreateSource(ctx, f.source); err != nil {
			return err
		}
		if err := q.CreatePublication(ctx, f.publication); err != nil {
			return err
		}
		return q.CreateWatch(ctx, f.watch)
	}); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	return f
}

func TestMigrationsIdempotentAndRestart(t *testing.T) {
	s, path := openStore(t)
	ctx := context.Background()

	v, err := s.SchemaVersion(ctx)
	if err != nil || v != 1 {
		t.Fatalf("schema version = %d, err=%v; want 1", v, err)
	}

	f := createFixture(t, s, "N1000AA", true, time.Now().UTC())
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if v, err := reopened.SchemaVersion(ctx); err != nil || v != 1 {
		t.Fatalf("reopened schema version = %d, err=%v", v, err)
	}
	got, err := reopened.Queries().GetWork(ctx, f.work.ID)
	if err != nil {
		t.Fatalf("get work after restart: %v", err)
	}
	if got.OriginalTitle != f.work.OriginalTitle {
		t.Fatalf("title = %q", got.OriginalTitle)
	}
}

func TestTransactionRollbackLeavesNoRows(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	w := &domain.Work{ID: id.New("work"), OriginalTitle: "rollback", CreatedAt: now, UpdatedAt: now}

	wantErr := errors.New("boom")
	err := s.InTx(ctx, func(q *store.Queries) error {
		if err := q.CreateWork(ctx, w); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want boom", err)
	}
	if _, err := s.Queries().GetWork(ctx, w.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetWork err = %v, want ErrNotFound", err)
	}
}

func TestDueWatches(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	due := createFixture(t, s, "N2000AA", true, now.Add(-time.Hour))
	_ = createFixture(t, s, "N2000BB", true, now.Add(time.Hour))
	_ = createFixture(t, s, "N2000CC", false, now.Add(-time.Hour))

	watches, err := s.Queries().ListDueWatches(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 1 || watches[0].ID != due.watch.ID {
		t.Fatalf("due watches = %+v, want only %s", watches, due.watch.ID)
	}
}

func TestEventDedupe(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	f := createFixture(t, s, "N3000AA", true, time.Now().UTC())
	now := time.Now().UTC().Truncate(time.Second)

	event := &domain.Event{
		ID:                    id.New("evt"),
		Type:                  domain.EventReleaseAdded,
		PublicationID:         f.publication.ID,
		WatchID:               f.watch.ID,
		SourceID:              f.source.ID,
		ExternalWorkReference: "ncode:N3000AA",
		ReleaseIDs:            []string{"N3000AA/1"},
		Payload:               json.RawMessage(`{"schema_version":"1.0"}`),
		DedupeKey:             "dedupe-1",
		SchemaVersion:         "1.0",
		DetectedAt:            now,
		CreatedAt:             now,
	}
	created, err := s.Queries().InsertEvent(ctx, event)
	if err != nil || !created {
		t.Fatalf("first insert created=%v err=%v", created, err)
	}

	second := *event
	second.ID = id.New("evt")
	created, err = s.Queries().InsertEvent(ctx, &second)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("duplicate dedupe key inserted a second event")
	}
	count, err := s.Queries().CountEvents(ctx, f.publication.ID)
	if err != nil || count != 1 {
		t.Fatalf("event count = %d err=%v", count, err)
	}

	events, err := s.Queries().ListEvents(ctx, store.EventFilter{PublicationID: f.publication.ID, Limit: 10})
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %d err=%v", len(events), err)
	}
	if string(events[0].Payload) != `{"schema_version":"1.0"}` {
		t.Fatalf("payload = %s", events[0].Payload)
	}
}

func TestDeliveryDedupe(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	f := createFixture(t, s, "N4000AA", true, time.Now().UTC())
	now := time.Now().UTC().Truncate(time.Second)
	event := &domain.Event{
		ID:                    id.New("evt"),
		Type:                  domain.EventReleaseAdded,
		PublicationID:         f.publication.ID,
		WatchID:               f.watch.ID,
		SourceID:              f.source.ID,
		ExternalWorkReference: "ncode:N4000AA",
		ReleaseIDs:            []string{"N4000AA/1"},
		Payload:               json.RawMessage(`{}`),
		DedupeKey:             "delivery-dedupe",
		SchemaVersion:         "1.0",
		DetectedAt:            now,
		CreatedAt:             now,
	}
	if _, err := s.Queries().InsertEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	d := &domain.NotificationDelivery{
		ID:            id.New("del"),
		EventID:       event.ID,
		Channel:       "telegram",
		Status:        domain.DeliveryPending,
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	created, err := s.Queries().InsertDelivery(ctx, d)
	if err != nil || !created {
		t.Fatalf("first delivery created=%v err=%v", created, err)
	}
	second := *d
	second.ID = id.New("del")
	created, err = s.Queries().InsertDelivery(ctx, &second)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("duplicate event/channel inserted a second delivery")
	}
}

func TestReleaseBaselineAndRemoval(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	f := createFixture(t, s, "N5000AA", true, time.Now().UTC())
	now := time.Now().UTC().Truncate(time.Second)
	release := &domain.Release{
		ID:            id.New("rel"),
		PublicationID: f.publication.ID,
		ExternalID:    "N5000AA/1",
		Kind:          domain.ReleaseChapter,
		ContentHash:   "hash-1",
		IsBaseline:    true,
		FirstSeenAt:   now,
		LastSeenAt:    now,
	}
	if err := s.Queries().InsertRelease(ctx, release); err != nil {
		t.Fatal(err)
	}
	active, err := s.Queries().ListReleasesByPublication(ctx, f.publication.ID, false, 10)
	if err != nil || len(active) != 1 || !active[0].IsBaseline {
		t.Fatalf("active = %+v err=%v", active, err)
	}
	if err := s.Queries().MarkReleaseRemoved(ctx, release.ID, now); err != nil {
		t.Fatal(err)
	}
	active, err = s.Queries().ListReleasesByPublication(ctx, f.publication.ID, false, 10)
	if err != nil || len(active) != 0 {
		t.Fatalf("active after removal = %+v err=%v", active, err)
	}
	all, err := s.Queries().ListReleasesByPublication(ctx, f.publication.ID, true, 10)
	if err != nil || len(all) != 1 || all[0].RemovedAt == nil {
		t.Fatalf("all after removal = %+v err=%v", all, err)
	}
}

func TestSettingRoundTrip(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.Queries().SetSetting(ctx, "default_interval_seconds", "86400", now); err != nil {
		t.Fatal(err)
	}
	got, err := s.Queries().GetSetting(ctx, "default_interval_seconds")
	if err != nil || got.Value != "86400" {
		t.Fatalf("setting = %+v err=%v", got, err)
	}
	if err := s.Queries().SetSetting(ctx, "default_interval_seconds", "21600", now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Queries().GetSetting(ctx, "default_interval_seconds")
	if got.Value != "21600" {
		t.Fatalf("setting = %q", got.Value)
	}
}
