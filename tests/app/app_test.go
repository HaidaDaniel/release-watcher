package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/lock"
)

type eventDoc struct {
	SchemaVersion string    `json:"schema_version"`
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	ReleaseIDs    []string  `json:"release_ids"`
	DetectedAt    time.Time `json:"detected_at"`
	CurrentCount  *int64    `json:"current_count"`
	Work          struct {
		OriginalTitle string `json:"original_title"`
	} `json:"work"`
}

func decodeEvents(t *testing.T, result *app.EventsResult) []eventDoc {
	t.Helper()
	out := make([]eventDoc, 0, len(result.Events))
	for _, raw := range result.Events {
		var doc eventDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		out = append(out, doc)
	}
	return out
}

func addNaro(t *testing.T, env *testEnv, interval *time.Duration) *app.AddResult {
	t.Helper()
	result, err := env.svc.Add(context.Background(), app.AddInput{URL: naroURL, Interval: interval})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	return result
}

func scanID(t *testing.T, env *testEnv, id string) *app.ScanResult {
	t.Helper()
	result, err := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "id", ID: id})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return result
}

func TestNaroFirstAddBaselineAndDuplicate(t *testing.T) {
	n := newNaroFixture(t, []string{"One", "Two", "Three"})
	env := newTestEnv(t, n)
	ctx := context.Background()

	added := addNaro(t, env, nil)
	if !added.Created || added.BaselineReleases != 3 || added.ExternalWorkReference != "ncode:N2166FE" {
		t.Fatalf("add = %+v", added)
	}
	events, err := env.svc.Events(ctx, app.EventQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Events) != 0 {
		t.Fatalf("baseline emitted %d events", len(events.Events))
	}
	show, err := env.svc.Show(ctx, added.WatchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(show.Releases) != 3 {
		t.Fatalf("releases = %d", len(show.Releases))
	}
	for _, release := range show.Releases {
		if !release.IsBaseline {
			t.Fatalf("release %s is not baseline", release.ExternalID)
		}
	}

	again := addNaro(t, env, nil)
	if again.Created || again.WatchID != added.WatchID || again.Reason != "already_exists" {
		t.Fatalf("duplicate add = %+v", again)
	}
	show, _ = env.svc.Show(ctx, added.WatchID)
	if len(show.Releases) != 3 {
		t.Fatalf("duplicate add changed releases: %d", len(show.Releases))
	}
}

func TestNaroNewChaptersAndReplay(t *testing.T) {
	n := newNaroFixture(t, []string{"One", "Two", "Three"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)

	n.setChapters([]string{"One", "Two", "Three", "Four"})
	first := scanID(t, env, added.WatchID)
	if first.EventsCreated != 1 || first.Failed != 0 {
		t.Fatalf("first scan = %+v", first)
	}
	events, _ := env.svc.Events(ctx, app.EventQuery{})
	docs := decodeEvents(t, events)
	if len(docs) != 1 || docs[0].EventType != "release.added" || len(docs[0].ReleaseIDs) != 1 || docs[0].ReleaseIDs[0] != "N2166FE/4" {
		t.Fatalf("events = %+v", docs)
	}
	if docs[0].SchemaVersion != "1.0" || docs[0].Work.OriginalTitle != "テスト作品" {
		t.Fatalf("event metadata = %+v", docs[0])
	}

	replay := scanID(t, env, added.WatchID)
	if replay.EventsCreated != 0 {
		t.Fatalf("replay created %d events", replay.EventsCreated)
	}
	events, _ = env.svc.Events(ctx, app.EventQuery{})
	if len(events.Events) != 1 {
		t.Fatalf("replay duplicated events: %d", len(events.Events))
	}

	n.setChapters([]string{"One", "Two", "Three", "Four", "Five", "Six"})
	multi := scanID(t, env, added.WatchID)
	if multi.EventsCreated != 1 {
		t.Fatalf("multi scan = %+v", multi)
	}
	events, _ = env.svc.Events(ctx, app.EventQuery{})
	docs = decodeEvents(t, events)
	if len(docs) != 2 || len(docs[1].ReleaseIDs) != 2 {
		t.Fatalf("multi events = %+v", docs)
	}
}

func TestNaroFailureAndRecovery(t *testing.T) {
	n := newNaroFixture(t, []string{"One", "Two", "Three"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)

	n.setFailAPI(true)
	failed := scanID(t, env, added.WatchID)
	if failed.Failed != 1 || failed.Results[0].Error == nil || failed.Results[0].Error.Code != "fetch_failed" {
		t.Fatalf("failed scan = %+v", failed)
	}
	show, _ := env.svc.Show(ctx, added.WatchID)
	if show.Watch.LastErrorCategory == nil || *show.Watch.LastErrorCategory != "fetch_failed" {
		t.Fatalf("last error = %v", show.Watch.LastErrorCategory)
	}
	if len(show.Scans) == 0 || show.Scans[0].Result != "error" {
		t.Fatalf("scan history = %+v", show.Scans)
	}

	n.setFailAPI(false)
	n.setChapters([]string{"One", "Two", "Three", "Four"})
	recovered := scanID(t, env, added.WatchID)
	if recovered.Succeeded != 1 || recovered.EventsCreated != 1 {
		t.Fatalf("recovery = %+v", recovered)
	}
	show, _ = env.svc.Show(ctx, added.WatchID)
	if show.Watch.LastErrorCategory != nil {
		t.Fatalf("error not cleared: %v", *show.Watch.LastErrorCategory)
	}
}

func TestRSSSlidingWindowDoesNotRemove(t *testing.T) {
	r := newRSSFixture(t, rssEntries("A", "B"))
	env := newTestEnv(t, nil)
	ctx := context.Background()
	added, err := env.svc.Add(ctx, app.AddInput{URL: r.server.URL + "/feed.xml"})
	if err != nil {
		t.Fatal(err)
	}
	if added.BaselineReleases != 2 {
		t.Fatalf("baseline = %d", added.BaselineReleases)
	}

	r.setEntries(rssEntries("B", "C"))
	scan := scanID(t, env, added.WatchID)
	if scan.EventsCreated != 1 {
		t.Fatalf("scan = %+v", scan)
	}
	events, _ := env.svc.Events(ctx, app.EventQuery{})
	docs := decodeEvents(t, events)
	if len(docs) != 1 || docs[0].EventType != "release.added" || len(docs[0].ReleaseIDs) != 1 || docs[0].ReleaseIDs[0] != "C" {
		t.Fatalf("events = %+v", docs)
	}
	show, _ := env.svc.Show(ctx, added.WatchID)
	var foundA bool
	for _, release := range show.Releases {
		if release.ExternalID == "A" {
			foundA = true
			if release.RemovedAt != nil {
				t.Fatal("sliding-window feed removed old entry A")
			}
		}
	}
	if !foundA {
		t.Fatal("old entry A disappeared from stored releases")
	}

	r.setEntries(rssEntries("A", "B", "C"))
	replay := scanID(t, env, added.WatchID)
	if replay.EventsCreated != 0 {
		t.Fatalf("replay = %+v", replay)
	}
}

func TestCountOnlyReconcilesSilently(t *testing.T) {
	n := newNaroFixture(t, []string{"One", "Two", "Three"})
	n.setFailCatalog(true)
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)
	if added.Mode != string(domain.ModeCountOnly) || added.BaselineReleases != 3 {
		t.Fatalf("add = %+v", added)
	}

	n.setFailCatalog(false)
	n.setChapters([]string{"One", "Two", "Three", "Four"})
	reconcile := scanID(t, env, added.WatchID)
	if reconcile.EventsCreated != 0 || reconcile.Results[0].Outcome != "baseline" {
		t.Fatalf("reconcile = %+v", reconcile)
	}
	show, _ := env.svc.Show(ctx, added.WatchID)
	if show.Watch.Mode != string(domain.ModeCatalog) || len(show.Releases) != 4 {
		t.Fatalf("show = mode %q releases %d", show.Watch.Mode, len(show.Releases))
	}

	n.setChapters([]string{"One", "Two", "Three", "Four", "Five"})
	next := scanID(t, env, added.WatchID)
	if next.EventsCreated != 1 {
		t.Fatalf("next scan = %+v", next)
	}
}

func TestDueIntervalPauseResumeRemove(t *testing.T) {
	n := newNaroFixture(t, []string{"One", "Two", "Three"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	interval := 6 * time.Hour
	added := addNaro(t, env, &interval)

	due, err := env.svc.Scan(ctx, app.ScanOptions{Mode: "due"})
	if err != nil {
		t.Fatal(err)
	}
	if due.Scanned != 0 {
		t.Fatalf("watch scanned before due: %+v", due)
	}

	clock = clock.Add(7 * time.Hour)
	n.setChapters([]string{"One", "Two", "Three", "Four"})
	due, _ = env.svc.Scan(ctx, app.ScanOptions{Mode: "due"})
	if due.Scanned != 1 || due.EventsCreated != 1 {
		t.Fatalf("due scan = %+v", due)
	}

	paused, err := env.svc.Pause(ctx, added.WatchID)
	if err != nil || paused.Enabled {
		t.Fatalf("pause = %+v err=%v", paused, err)
	}
	clock = clock.Add(7 * time.Hour)
	n.setChapters([]string{"One", "Two", "Three", "Four", "Five"})
	due, _ = env.svc.Scan(ctx, app.ScanOptions{Mode: "due"})
	if due.Scanned != 0 {
		t.Fatalf("paused watch scanned: %+v", due)
	}

	resumed, err := env.svc.Resume(ctx, added.WatchID)
	if err != nil || !resumed.Enabled {
		t.Fatalf("resume = %+v err=%v", resumed, err)
	}
	due, _ = env.svc.Scan(ctx, app.ScanOptions{Mode: "due"})
	if due.Scanned != 1 || due.EventsCreated != 1 {
		t.Fatalf("resume scan = %+v", due)
	}

	changed, err := env.svc.SetInterval(ctx, added.WatchID, 12*time.Hour)
	if err != nil || changed.IntervalSeconds != 43200 {
		t.Fatalf("interval = %+v err=%v", changed, err)
	}

	removed, err := env.svc.Remove(ctx, added.WatchID)
	if err != nil || !removed.Removed {
		t.Fatalf("remove = %+v err=%v", removed, err)
	}
	list, _ := env.svc.List(ctx)
	if len(list.Watches) != 0 {
		t.Fatalf("watches after remove = %+v", list.Watches)
	}
	events, _ := env.svc.Events(ctx, app.EventQuery{})
	if len(events.Events) != 2 {
		t.Fatalf("events after remove = %d, want preserved history", len(events.Events))
	}
	show, showErr := env.svc.Show(ctx, added.WatchID)
	if showErr != nil || show.Watch.RemovedAt == nil || len(show.Releases) == 0 {
		t.Fatalf("history after remove = %+v err=%v", show, showErr)
	}
	if _, err := env.svc.Purge(ctx, added.WatchID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	events, _ = env.svc.Events(ctx, app.EventQuery{})
	if len(events.Events) != 0 {
		t.Fatalf("events after purge = %d", len(events.Events))
	}
}

func TestGlobalIntervalUpdatesDefaultWatches(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	r := newRSSFixture(t, rssEntries("A"))
	env := newTestEnv(t, n)
	ctx := context.Background()
	naroAdded := addNaro(t, env, nil)
	rssAdded, err := env.svc.Add(ctx, app.AddInput{URL: r.server.URL + "/feed.xml"})
	if err != nil {
		t.Fatal(err)
	}
	custom := 2 * time.Hour
	if _, appErr := env.svc.SetInterval(ctx, naroAdded.WatchID, custom); appErr != nil {
		t.Fatal(appErr)
	}
	if _, appErr := env.svc.SetGlobalInterval(ctx, 6*time.Hour); appErr != nil {
		t.Fatal(appErr)
	}
	naroShow, _ := env.svc.Show(ctx, naroAdded.WatchID)
	if naroShow.Watch.IntervalSeconds != int64(custom/time.Second) {
		t.Fatalf("custom interval changed: %d", naroShow.Watch.IntervalSeconds)
	}
	rssShow, _ := env.svc.Show(ctx, rssAdded.WatchID)
	if rssShow.Watch.IntervalSeconds != int64((6*time.Hour)/time.Second) {
		t.Fatalf("default watch not updated: %d", rssShow.Watch.IntervalSeconds)
	}
}

func TestScanLockPreventsOverlap(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	release, err := lock.Acquire(ctx, env.cfg.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, appErr := env.svc.Scan(ctx, app.ScanOptions{Mode: "all"}); appErr == nil || appErr.Code != "locked" {
		t.Fatalf("scan with held lock = %+v", appErr)
	}
	release()
	result, appErr := env.svc.Scan(ctx, app.ScanOptions{Mode: "all"})
	if appErr != nil || result.Scanned != 0 {
		t.Fatalf("scan after release = %+v err=%v", result, appErr)
	}
}

func TestRestartKeepsState(t *testing.T) {
	n := newNaroFixture(t, []string{"One", "Two"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)
	n.setChapters([]string{"One", "Two", "Three"})
	scanID(t, env, added.WatchID)
	if err := env.store.Close(); err != nil {
		t.Fatal(err)
	}

	restarted := openServiceForConfig(t, env.cfg, n)
	list, err := restarted.svc.List(ctx)
	if err != nil || len(list.Watches) != 1 {
		t.Fatalf("list after restart = %+v err=%v", list, err)
	}
	show, err := restarted.svc.Show(ctx, added.WatchID)
	if err != nil || len(show.Releases) != 3 {
		t.Fatalf("show after restart = %+v err=%v", show, err)
	}
	events, _ := restarted.svc.Events(ctx, app.EventQuery{})
	if len(events.Events) != 1 {
		t.Fatalf("events after restart = %d", len(events.Events))
	}
}

func TestEventPagination(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)
	for _, chapters := range [][]string{
		{"One", "Two"},
		{"One", "Two", "Three"},
		{"One", "Two", "Three", "Four"},
	} {
		n.setChapters(chapters)
		scanID(t, env, added.WatchID)
	}
	first, err := env.svc.Events(ctx, app.EventQuery{Limit: 1})
	if err != nil || len(first.Events) != 1 || first.NextCursor == "" {
		t.Fatalf("first page = %+v err=%v", first, err)
	}
	second, err := env.svc.Events(ctx, app.EventQuery{Limit: 1, After: first.NextCursor})
	if err != nil || len(second.Events) != 1 || second.NextCursor == first.NextCursor {
		t.Fatalf("second page = %+v err=%v", second, err)
	}
	third, _ := env.svc.Events(ctx, app.EventQuery{Limit: 1, After: second.NextCursor})
	if len(third.Events) != 1 {
		t.Fatalf("third page = %+v", third)
	}
}

func TestMetadataAndStatusChanges(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)
	n.setMeta("新しい題名", 0)
	scanID(t, env, added.WatchID)
	events, _ := env.svc.Events(ctx, app.EventQuery{})
	docs := decodeEvents(t, events)
	types := map[string]bool{}
	for _, doc := range docs {
		types[doc.EventType] = true
	}
	if !types["publication.metadata_changed"] || !types["publication.status_changed"] {
		t.Fatalf("event types = %v", types)
	}
	show, _ := env.svc.Show(ctx, added.WatchID)
	if show.Work.OriginalTitle != "新しい題名" || show.Publication.Status != "completed" {
		t.Fatalf("show = %+v / %+v", show.Work, show.Publication)
	}
}

func TestInvalidAndUnknownSources(t *testing.T) {
	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>not a feed</body></html>`))
	}))
	defer htmlServer.Close()
	env := newTestEnv(t, nil)
	ctx := context.Background()

	if _, err := env.svc.Add(ctx, app.AddInput{URL: "ftp://example.test/x"}); err == nil || err.Code != "invalid_url" {
		t.Fatalf("ftp add = %+v", err)
	}
	if _, err := env.svc.Add(ctx, app.AddInput{URL: htmlServer.URL}); err == nil || err.Code != "unsupported_source" {
		t.Fatalf("html add = %+v", err)
	}
	if _, err := env.svc.Show(ctx, "missing"); err == nil || err.Code != "not_found" {
		t.Fatalf("missing show = %+v", err)
	}
	if _, err := env.svc.Scan(ctx, app.ScanOptions{Mode: "id", ID: "missing"}); err == nil || err.Code != "not_found" {
		t.Fatalf("missing scan = %+v", err)
	}
	if _, err := env.svc.SetInterval(ctx, "missing", time.Hour); err == nil || err.Code != "not_found" {
		t.Fatalf("missing interval = %+v", err)
	}
	if _, err := env.svc.SetInterval(ctx, "missing", time.Second); err == nil || err.Code != "invalid_argument" {
		t.Fatalf("invalid interval = %+v", err)
	}
}

func TestScanIsolatesWatchErrors(t *testing.T) {
	good := newRSSFixture(t, rssEntries("A"))
	bad := newRSSFixture(t, rssEntries("B"))
	env := newTestEnv(t, nil)
	ctx := context.Background()
	goodAdd, err := env.svc.Add(ctx, app.AddInput{URL: good.server.URL + "/feed.xml"})
	if err != nil {
		t.Fatal(err)
	}
	badAdd, err := env.svc.Add(ctx, app.AddInput{URL: bad.server.URL + "/feed.xml"})
	if err != nil {
		t.Fatal(err)
	}
	_ = goodAdd
	_ = badAdd
	bad.setFail(true)
	result, appErr := env.svc.Scan(ctx, app.ScanOptions{Mode: "all"})
	if appErr != nil {
		t.Fatal(appErr)
	}
	if result.Scanned != 2 || result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("scan = %+v", result)
	}
}

func TestDuplicateAddWorksWhenSourceUnavailable(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)
	n.setFailAPI(true)
	again, err := env.svc.Add(ctx, app.AddInput{URL: naroURL})
	if err != nil {
		t.Fatalf("duplicate add failed while source down: %v", err)
	}
	if again.Created || again.WatchID != added.WatchID || again.Reason != "already_exists" {
		t.Fatalf("duplicate add = %+v", again)
	}
}

func TestMetadataEventPayloadUsesNewTitle(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)
	n.setMeta("新しい題名", 1)
	scanID(t, env, added.WatchID)
	events, _ := env.svc.Events(ctx, app.EventQuery{})
	for _, doc := range decodeEvents(t, events) {
		if doc.EventType == "publication.metadata_changed" && doc.Work.OriginalTitle != "新しい題名" {
			t.Fatalf("metadata event title = %q", doc.Work.OriginalTitle)
		}
	}
}

func TestNaroStatusTransitionsOngoingCompletedOngoing(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addNaro(t, env, nil)

	show, _ := env.svc.Show(ctx, added.WatchID)
	if show.Publication.Status != "ongoing" {
		t.Fatalf("initial status = %q, want ongoing (end=1)", show.Publication.Status)
	}

	n.setMeta("テスト作品", 0)
	clock = clock.Add(time.Minute)
	scanID(t, env, added.WatchID)
	show, _ = env.svc.Show(ctx, added.WatchID)
	if show.Publication.Status != "completed" {
		t.Fatalf("after end=0 status = %q, want completed", show.Publication.Status)
	}

	n.setMeta("テスト作品", 1)
	clock = clock.Add(time.Minute)
	scanID(t, env, added.WatchID)
	show, _ = env.svc.Show(ctx, added.WatchID)
	if show.Publication.Status != "ongoing" {
		t.Fatalf("after end=1 status = %q, want ongoing", show.Publication.Status)
	}

	var transitions []string
	for _, doc := range decodeEvents(t, mustEvents(t, env)) {
		if doc.EventType == "publication.status_changed" {
			transitions = append(transitions, doc.EventID)
		}
	}
	if len(transitions) != 2 {
		t.Fatalf("status transitions = %d, want 2", len(transitions))
	}
}

func mustEvents(t *testing.T, env *testEnv) *app.EventsResult {
	t.Helper()
	events, err := env.svc.Events(context.Background(), app.EventQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func TestRepeatedStatusTransitionsAreNotDropped(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addNaro(t, env, nil)
	for _, end := range []int{0, 1, 0} {
		clock = clock.Add(time.Minute)
		n.setMeta("テスト作品", end)
		scanID(t, env, added.WatchID)
	}
	events, _ := env.svc.Events(ctx, app.EventQuery{Limit: 100})
	count := 0
	for _, doc := range decodeEvents(t, events) {
		if doc.EventType == "publication.status_changed" {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("status events = %d, want 3", count)
	}
}

func TestEmptyRSSWindowDoesNotEmitCountEvent(t *testing.T) {
	r := newRSSFixture(t, rssEntries("A", "B", "C"))
	env := newTestEnv(t, nil)
	ctx := context.Background()
	added, err := env.svc.Add(ctx, app.AddInput{URL: r.server.URL + "/feed.xml"})
	if err != nil {
		t.Fatal(err)
	}
	r.setEntries(nil)
	scan := scanID(t, env, added.WatchID)
	if scan.EventsCreated != 0 {
		t.Fatalf("empty feed emitted events: %+v", scan)
	}
	events, _ := env.svc.Events(ctx, app.EventQuery{})
	if len(events.Events) != 0 {
		t.Fatalf("empty feed events = %d", len(events.Events))
	}
}

func TestShowReturnsRecentEvents(t *testing.T) {
	n := newNaroFixture(t, []string{"0"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)
	chapters := []string{"0"}
	for i := 1; i <= 25; i++ {
		chapters = append(chapters, "Entry")
		n.setChapters(chapters)
		scanID(t, env, added.WatchID)
	}
	show, err := env.svc.Show(ctx, added.WatchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(show.Events) != 20 {
		t.Fatalf("show events = %d", len(show.Events))
	}
	var newest eventDoc
	if err := json.Unmarshal(show.Events[0], &newest); err != nil {
		t.Fatal(err)
	}
	if len(newest.ReleaseIDs) != 1 || newest.ReleaseIDs[0] != "N2166FE/26" {
		t.Fatalf("show newest event = %+v", newest.ReleaseIDs)
	}
}

func TestAddCollapsesDuplicateFeedEntries(t *testing.T) {
	r := newRSSFixture(t, []rssEntry{
		{ID: "A", Title: "Entry A", Date: "Mon, 02 Nov 2026 10:00:00 GMT"},
		{ID: "A", Title: "Entry A", Date: "Mon, 02 Nov 2026 10:00:00 GMT"},
	})
	env := newTestEnv(t, nil)
	ctx := context.Background()
	added, err := env.svc.Add(ctx, app.AddInput{URL: r.server.URL + "/feed.xml"})
	if err != nil {
		t.Fatalf("add with duplicate entries: %v", err)
	}
	if added.BaselineReleases != 1 {
		t.Fatalf("baseline = %d", added.BaselineReleases)
	}
	show, _ := env.svc.Show(ctx, added.WatchID)
	if len(show.Releases) != 1 {
		t.Fatalf("releases = %d", len(show.Releases))
	}
}

func TestRemovePreservesHistoryAndReAddRestores(t *testing.T) {
	n := newNaroFixture(t, []string{"One", "Two", "Three"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)

	n.setChapters([]string{"One", "Two", "Three", "Four"})
	scanID(t, env, added.WatchID)

	removed, err := env.svc.Remove(ctx, added.WatchID)
	if err != nil || !removed.Removed || !removed.HistoryPreserved {
		t.Fatalf("remove = %+v err=%v", removed, err)
	}
	list, _ := env.svc.List(ctx)
	if len(list.Watches) != 0 {
		t.Fatalf("list after remove = %+v", list.Watches)
	}
	events, _ := env.svc.Events(ctx, app.EventQuery{})
	if len(events.Events) != 1 {
		t.Fatalf("events after remove = %d", len(events.Events))
	}
	show, showErr := env.svc.Show(ctx, added.WatchID)
	if showErr != nil || show.Watch.RemovedAt == nil || len(show.Releases) != 4 {
		t.Fatalf("show after remove = %+v err=%v", show, showErr)
	}
	if _, scanErr := env.svc.Scan(ctx, app.ScanOptions{Mode: "id", ID: added.WatchID}); scanErr == nil || scanErr.Code != "invalid_argument" {
		t.Fatalf("scan removed watch = %+v", scanErr)
	}

	restored, addErr := env.svc.Add(ctx, app.AddInput{URL: naroURL})
	if addErr != nil {
		t.Fatal(addErr)
	}
	if restored.Created || restored.Reason != "restored" || restored.WatchID != added.WatchID {
		t.Fatalf("re-add = %+v", restored)
	}
	show, _ = env.svc.Show(ctx, added.WatchID)
	if show.Watch.RemovedAt != nil || len(show.Releases) != 4 {
		t.Fatalf("restored show = %+v", show)
	}
	events, _ = env.svc.Events(ctx, app.EventQuery{})
	if len(events.Events) != 1 {
		t.Fatalf("re-add replayed old chapters: %d events", len(events.Events))
	}
	replay := scanID(t, env, added.WatchID)
	if replay.EventsCreated != 0 {
		t.Fatalf("replay after restore = %+v", replay)
	}

	n.setChapters([]string{"One", "Two", "Three", "Four", "Five"})
	fresh := scanID(t, env, added.WatchID)
	if fresh.EventsCreated != 1 {
		t.Fatalf("new chapter after restore = %+v", fresh)
	}
	show, _ = env.svc.Show(ctx, added.WatchID)
	if len(show.Releases) != 5 {
		t.Fatalf("releases after restore = %d", len(show.Releases))
	}
	ids := map[string]int{}
	for _, release := range show.Releases {
		ids[release.ExternalID]++
	}
	for externalID, count := range ids {
		if count != 1 {
			t.Fatalf("duplicate release %s x%d", externalID, count)
		}
	}
}
