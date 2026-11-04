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
	if err != nil || v != 3 {
		t.Fatalf("schema version = %d, err=%v; want 3", v, err)
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
	if v, err := reopened.SchemaVersion(ctx); err != nil || v != 3 {
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

func TestDeleteObservationRemovesOwnedData(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	f := createFixture(t, s, "N6000AA", true, time.Now().UTC())
	now := time.Now().UTC().Truncate(time.Second)

	release := &domain.Release{
		ID: id.New("rel"), PublicationID: f.publication.ID, ExternalID: "N6000AA/1",
		Kind: domain.ReleaseChapter, ContentHash: "h", FirstSeenAt: now, LastSeenAt: now,
	}
	if err := s.Queries().InsertRelease(ctx, release); err != nil {
		t.Fatal(err)
	}
	event := &domain.Event{
		ID: id.New("evt"), Type: domain.EventReleaseAdded, PublicationID: f.publication.ID,
		WatchID: f.watch.ID, SourceID: f.source.ID, ExternalWorkReference: "ncode:N6000AA",
		ReleaseIDs: []string{"N6000AA/1"}, Payload: json.RawMessage(`{"x":1}`),
		DedupeKey: "remove-1", SchemaVersion: "1.0", DetectedAt: now, CreatedAt: now,
	}
	if _, err := s.Queries().InsertEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	delivery := &domain.NotificationDelivery{
		ID: id.New("del"), EventID: event.ID, Channel: "telegram", Status: domain.DeliveryPending,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.Queries().InsertDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	run := &domain.ScanRun{
		ID: id.New("scan"), WatchID: f.watch.ID, PublicationID: f.publication.ID,
		StartedAt: now, FinishedAt: now, Result: domain.ScanOK, EventsCreated: 1, DurationMillis: 5,
	}
	if err := s.Queries().InsertScanRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteObservation(ctx, f.watch.ID); err != nil {
		t.Fatalf("delete observation: %v", err)
	}
	if _, err := s.Queries().GetWatch(ctx, f.watch.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("watch still exists: %v", err)
	}
	if _, err := s.Queries().GetPublication(ctx, f.publication.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("publication still exists: %v", err)
	}
	if _, err := s.Queries().GetWork(ctx, f.work.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("work still exists: %v", err)
	}
	if _, err := s.Queries().GetSource(ctx, f.source.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("source still exists: %v", err)
	}
	all, err := s.Queries().ListReleasesByPublication(ctx, f.publication.ID, true, 10)
	if err != nil || len(all) != 0 {
		t.Fatalf("releases remain: %+v err=%v", all, err)
	}
	events, err := s.Queries().ListEvents(ctx, store.EventFilter{PublicationID: f.publication.ID})
	if err != nil || len(events) != 0 {
		t.Fatalf("events remain: %+v err=%v", events, err)
	}
	deliveries, err := s.Queries().ListDeliveriesByEvent(ctx, event.ID)
	if err != nil || len(deliveries) != 0 {
		t.Fatalf("deliveries remain: %+v err=%v", deliveries, err)
	}
	runs, err := s.Queries().ListScanRunsByWatch(ctx, f.watch.ID, 10)
	if err != nil || len(runs) != 0 {
		t.Fatalf("scan runs remain: %+v err=%v", runs, err)
	}
}

func TestDeleteObservationRetainsSharedWorkAndSource(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	f := createFixture(t, s, "N7000AA", true, time.Now().UTC())
	now := time.Now().UTC().Truncate(time.Second)

	pub2 := &domain.Publication{
		ID: id.New("pub"), WorkID: f.work.ID, SourceID: f.source.ID,
		Type: domain.PublicationLN, OriginalURL: "https://example.test/ln/N7000AA",
		Status: domain.StatusUnknown, CreatedAt: now, UpdatedAt: now,
	}
	watch2 := &domain.Watch{
		ID: id.New("watch"), PublicationID: pub2.ID, Enabled: true, IntervalSeconds: 86400,
		Mode: domain.ModeCatalog, NextCheckAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.InTx(ctx, func(q *store.Queries) error {
		if err := q.CreatePublication(ctx, pub2); err != nil {
			return err
		}
		return q.CreateWatch(ctx, watch2)
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteObservation(ctx, f.watch.ID); err != nil {
		t.Fatalf("delete first observation: %v", err)
	}
	if _, err := s.Queries().GetWork(ctx, f.work.ID); err != nil {
		t.Fatalf("shared work removed: %v", err)
	}
	if _, err := s.Queries().GetSource(ctx, f.source.ID); err != nil {
		t.Fatalf("shared source removed: %v", err)
	}
	if _, err := s.Queries().GetPublication(ctx, pub2.ID); err != nil {
		t.Fatalf("second publication removed: %v", err)
	}
	if _, err := s.Queries().GetWatch(ctx, watch2.ID); err != nil {
		t.Fatalf("second watch removed: %v", err)
	}

	if err := s.DeleteObservation(ctx, watch2.ID); err != nil {
		t.Fatalf("delete second observation: %v", err)
	}
	if _, err := s.Queries().GetWork(ctx, f.work.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unreferenced work remains: %v", err)
	}
	if _, err := s.Queries().GetSource(ctx, f.source.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unreferenced source remains: %v", err)
	}
}

func TestNilSlicesArePersistedAsEmptyArrays(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	source := &domain.Source{ID: id.New("src"), Kind: domain.SourceRSS, URI: "https://example.test/feed", ExternalID: "feed-1", FieldsAvailable: nil, CreatedAt: now, UpdatedAt: now}
	if err := s.Queries().CreateSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	got, err := s.Queries().GetSource(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FieldsAvailable == nil || len(got.FieldsAvailable) != 0 {
		t.Fatalf("fields = %#v, want non-nil empty slice", got.FieldsAvailable)
	}

	work := &domain.Work{ID: id.New("work"), OriginalTitle: "nil-slice", CreatedAt: now, UpdatedAt: now}
	pub := &domain.Publication{ID: id.New("pub"), WorkID: work.ID, SourceID: source.ID, Type: domain.PublicationUnknown, OriginalURL: "https://example.test/feed", Status: domain.StatusUnknown, CreatedAt: now, UpdatedAt: now}
	watch := &domain.Watch{ID: id.New("watch"), PublicationID: pub.ID, Enabled: true, IntervalSeconds: 86400, Mode: domain.ModeCatalog, NextCheckAt: now, CreatedAt: now, UpdatedAt: now}
	if err := s.InTx(ctx, func(q *store.Queries) error {
		if err := q.CreateWork(ctx, work); err != nil {
			return err
		}
		if err := q.CreatePublication(ctx, pub); err != nil {
			return err
		}
		return q.CreateWatch(ctx, watch)
	}); err != nil {
		t.Fatal(err)
	}
	event := &domain.Event{ID: id.New("evt"), Type: domain.EventReleaseAdded, PublicationID: pub.ID, WatchID: watch.ID, SourceID: source.ID, ExternalWorkReference: "feed-1", ReleaseIDs: nil, Payload: json.RawMessage(`{}`), DedupeKey: "nil-slice", SchemaVersion: "1.0", DetectedAt: now, CreatedAt: now}
	if _, err := s.Queries().InsertEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	gotEvent, err := s.Queries().GetEvent(ctx, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotEvent.ReleaseIDs == nil || len(gotEvent.ReleaseIDs) != 0 {
		t.Fatalf("release ids = %#v, want non-nil empty slice", gotEvent.ReleaseIDs)
	}
}

func TestInvalidEnumsAndPayloadRejected(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.Queries().CreateSource(ctx, &domain.Source{ID: id.New("src"), Kind: "bogus", URI: "x", ExternalID: "x", CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("invalid source kind accepted")
	}
	if err := s.Queries().CreatePublication(ctx, &domain.Publication{ID: id.New("pub"), WorkID: "w", SourceID: "s", Type: "bogus", OriginalURL: "x", Status: domain.StatusUnknown, CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("invalid publication type accepted")
	}
	if err := s.Queries().CreateWatch(ctx, &domain.Watch{ID: id.New("watch"), PublicationID: "p", Enabled: true, IntervalSeconds: 1, Mode: "bogus", NextCheckAt: now, CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("invalid watch mode accepted")
	}
	if err := s.Queries().InsertRelease(ctx, &domain.Release{ID: id.New("rel"), PublicationID: "p", ExternalID: "x", Kind: "bogus", ContentHash: "h", FirstSeenAt: now, LastSeenAt: now}); err == nil {
		t.Fatal("invalid release kind accepted")
	}
	if err := s.Queries().InsertScanRun(ctx, &domain.ScanRun{ID: id.New("scan"), WatchID: "w", PublicationID: "p", StartedAt: now, FinishedAt: now, Result: "bogus"}); err == nil {
		t.Fatal("invalid scan result accepted")
	}
	if _, err := s.Queries().InsertEvent(ctx, &domain.Event{ID: id.New("evt"), Type: "bogus", DedupeKey: "x", SchemaVersion: "1.0", Payload: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("invalid event type accepted")
	}
	if _, err := s.Queries().InsertEvent(ctx, &domain.Event{ID: id.New("evt"), Type: domain.EventReleaseAdded, DedupeKey: "x", SchemaVersion: "1.0", Payload: nil}); err == nil {
		t.Fatal("empty event payload accepted")
	}
	if _, err := s.Queries().InsertDelivery(ctx, &domain.NotificationDelivery{ID: id.New("del"), EventID: "e", Channel: "telegram", Status: "bogus", NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("invalid delivery status accepted")
	}
	if err := s.Queries().UpdateWatchSchedule(ctx, "missing", 0, now, now); err == nil {
		t.Fatal("non-positive watch interval accepted")
	}
}

func TestUnknownEventCursorReturnsNotFound(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	_, err := s.Queries().ListEvents(ctx, store.EventFilter{After: "evt_missing", Limit: 10})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestConcurrentOpenRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	const workers = 8
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			st, err := store.Open(context.Background(), path)
			if err == nil {
				err = st.Close()
			}
			errs <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent open failed: %v", err)
		}
	}
}

func TestListPublicationsDoesNotDeadlock(t *testing.T) {
	s, _ := openStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	createFixture(t, s, "N8000AA", true, time.Now().UTC())
	createFixture(t, s, "N8000BB", true, time.Now().UTC())
	pubs, err := s.Queries().ListPublications(ctx)
	if err != nil {
		t.Fatalf("list publications: %v", err)
	}
	if len(pubs) != 2 {
		t.Fatalf("publications = %d, want 3", len(pubs))
	}
	watches, err := s.Queries().ListWatches(ctx)
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(watches) != 2 {
		t.Fatalf("watches = %d, want 3", len(watches))
	}
}

func TestUnwatchPreservesHistoryAndRestoreReusesRows(t *testing.T) {
	s, _ := openStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	f := createFixture(t, s, "N8000AA", true, now)

	release := &domain.Release{
		ID: id.New("rel"), PublicationID: f.publication.ID, ExternalID: "N8000AA/1",
		Kind: domain.ReleaseChapter, ContentHash: "h", FirstSeenAt: now, LastSeenAt: now,
	}
	if err := s.Queries().InsertRelease(ctx, release); err != nil {
		t.Fatal(err)
	}
	event := &domain.Event{
		ID: id.New("evt"), Type: domain.EventReleaseAdded, PublicationID: f.publication.ID,
		WatchID: f.watch.ID, SourceID: f.source.ID, ExternalWorkReference: "ncode:N8000AA",
		ReleaseIDs: []string{"N8000AA/1"}, Payload: json.RawMessage(`{"x":1}`),
		DedupeKey: "unwatch-1", SchemaVersion: "1.0", DetectedAt: now, CreatedAt: now,
	}
	if _, err := s.Queries().InsertEvent(ctx, event); err != nil {
		t.Fatal(err)
	}

	if err := s.Queries().MarkWatchRemoved(ctx, f.watch.ID, now); err != nil {
		t.Fatal(err)
	}
	watch, err := s.Queries().GetWatch(ctx, f.watch.ID)
	if err != nil || watch.RemovedAt == nil || watch.Enabled {
		t.Fatalf("removed watch = %+v err=%v", watch, err)
	}
	watches, err := s.Queries().ListWatches(ctx)
	if err != nil || len(watches) != 0 {
		t.Fatalf("list after unwatch = %+v err=%v", watches, err)
	}
	events, err := s.Queries().ListEvents(ctx, store.EventFilter{WatchID: f.watch.ID})
	if err != nil || len(events) != 1 || events[0].ID != event.ID {
		t.Fatalf("events after unwatch = %+v err=%v", events, err)
	}
	if _, err := s.Queries().GetPublication(ctx, f.publication.ID); err != nil {
		t.Fatalf("publication after unwatch: %v", err)
	}

	if err := s.Queries().RestoreWatch(ctx, f.watch.ID, now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	watch, err = s.Queries().GetWatch(ctx, f.watch.ID)
	if err != nil || watch.RemovedAt != nil || !watch.Enabled {
		t.Fatalf("restored watch = %+v err=%v", watch, err)
	}
	watches, _ = s.Queries().ListWatches(ctx)
	if len(watches) != 1 || watches[0].ID != f.watch.ID {
		t.Fatalf("list after restore = %+v", watches)
	}
	events, _ = s.Queries().ListEvents(ctx, store.EventFilter{WatchID: f.watch.ID})
	if len(events) != 1 {
		t.Fatalf("events after restore = %d", len(events))
	}
}
