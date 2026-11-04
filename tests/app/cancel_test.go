package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/domain"
	"github.com/HaidaDaniel/release-watcher/internal/lock"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

func deliveriesForWatch(t *testing.T, env *testEnv, watchID string) map[string]domain.DeliveryStatus {
	t.Helper()
	events, err := env.store.Queries().ListEvents(context.Background(), store.EventFilter{WatchID: watchID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]domain.DeliveryStatus{}
	for _, event := range events {
		deliveries, err := env.store.Queries().ListDeliveriesByEvent(context.Background(), event.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, delivery := range deliveries {
			out[event.ID] = delivery.Status
		}
	}
	return out
}

func TestRemoveCancelsPendingAndFailedDeliveries(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addNaro(t, env, nil)

	n.setChapters([]string{"One", "Two"})
	scanID(t, env, added.WatchID)

	tg.mu.Lock()
	tg.failStatus = http.StatusInternalServerError
	tg.failCount = 100
	tg.mu.Unlock()
	n.setChapters([]string{"One", "Two", "Three"})
	scanID(t, env, added.WatchID)

	n.setChapters([]string{"One", "Two", "Three", "Four"})
	if _, appErr := env.svc.Scan(ctx, app.ScanOptions{Mode: "id", ID: added.WatchID, NoNotify: true}); appErr != nil {
		t.Fatal(appErr)
	}

	statuses := deliveriesForWatch(t, env, added.WatchID)
	counts := map[domain.DeliveryStatus]int{}
	for _, status := range statuses {
		counts[status]++
	}
	if counts[domain.DeliverySent] != 1 || counts[domain.DeliveryPending] != 1 || counts[domain.DeliveryFailed] != 1 {
		t.Fatalf("before remove statuses = %+v", counts)
	}

	removed, appErr := env.svc.Remove(ctx, added.WatchID)
	if appErr != nil {
		t.Fatal(appErr)
	}
	if removed.CanceledDeliveries != 2 {
		t.Fatalf("canceled deliveries = %d, want 2", removed.CanceledDeliveries)
	}
	statuses = deliveriesForWatch(t, env, added.WatchID)
	counts = map[domain.DeliveryStatus]int{}
	for _, status := range statuses {
		counts[status]++
	}
	if counts[domain.DeliveryCanceled] != 2 || counts[domain.DeliverySent] != 1 || counts[domain.DeliveryPending] != 0 || counts[domain.DeliveryFailed] != 0 {
		t.Fatalf("after remove statuses = %+v", counts)
	}

	tg.mu.Lock()
	tg.failCount = 0
	tg.mu.Unlock()
	clock = clock.Add(2 * time.Minute)
	countsAfterNotify, appErr := env.svc.Notify(ctx)
	if appErr != nil {
		t.Fatal(appErr)
	}
	if countsAfterNotify.Sent != 1 || countsAfterNotify.Canceled != 2 {
		t.Fatalf("notify after remove = %+v", countsAfterNotify)
	}
	messages, _ := tg.snapshot()
	if len(messages) != 1 {
		t.Fatalf("canceled deliveries were sent: %d messages", len(messages))
	}

	restored, appErr := env.svc.Add(ctx, app.AddInput{URL: naroURL})
	if appErr != nil {
		t.Fatal(appErr)
	}
	if restored.Reason != "restored" {
		t.Fatalf("re-add = %+v", restored)
	}
	if _, appErr := env.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	if messages, _ = tg.snapshot(); len(messages) != 1 {
		t.Fatalf("re-add sent old canceled deliveries: %d", len(messages))
	}

	if err := env.store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := openServiceForConfig(t, env.cfg, n)
	restarted.svc.SetNotifier(tg.notifier())
	restarted.svc.SetNow(func() time.Time { return clock.Add(2 * time.Minute) })
	if _, appErr := restarted.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	if messages, _ = tg.snapshot(); len(messages) != 1 {
		t.Fatalf("restart sent canceled deliveries: %d", len(messages))
	}
	statuses = deliveriesForWatch(t, restarted, restored.WatchID)
	counts = map[domain.DeliveryStatus]int{}
	for _, status := range statuses {
		counts[status]++
	}
	if counts[domain.DeliveryCanceled] != 2 || counts[domain.DeliverySent] != 1 {
		t.Fatalf("statuses after restart = %+v", counts)
	}

	n.setChapters([]string{"One", "Two", "Three", "Four", "Five"})
	fresh := scanID(t, restarted, restored.WatchID)
	if fresh.EventsCreated != 1 {
		t.Fatalf("new event after restore = %+v", fresh)
	}
	messages, _ = tg.snapshot()
	if len(messages) != 2 {
		t.Fatalf("new event not delivered: %d messages", len(messages))
	}
}

func TestPauseDoesNotCancelDeliveries(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addNaro(t, env, nil)

	n.setChapters([]string{"One", "Two"})
	if _, appErr := env.svc.Scan(ctx, app.ScanOptions{Mode: "id", ID: added.WatchID, NoNotify: true}); appErr != nil {
		t.Fatal(appErr)
	}
	if _, appErr := env.svc.Pause(ctx, added.WatchID); appErr != nil {
		t.Fatal(appErr)
	}
	if _, appErr := env.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	messages, _ := tg.snapshot()
	if len(messages) != 1 {
		t.Fatalf("pause canceled a pending delivery: %d messages", len(messages))
	}
}

func TestRemoveCancelsOnlyOwnDeliveries(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	r := newRSSFixture(t, rssEntries("A"))
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	added := addNaro(t, env, nil)
	rssAdded, err := env.svc.Add(ctx, app.AddInput{URL: r.server.URL + "/feed.xml"})
	if err != nil {
		t.Fatal(err)
	}

	n.setChapters([]string{"One", "Two"})
	r.setEntries(rssEntries("A", "B"))
	if _, appErr := env.svc.Scan(ctx, app.ScanOptions{Mode: "all", NoNotify: true}); appErr != nil {
		t.Fatal(appErr)
	}
	if _, appErr := env.svc.Remove(ctx, added.WatchID); appErr != nil {
		t.Fatal(appErr)
	}
	if _, appErr := env.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	messages, _ := tg.snapshot()
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want only the remaining watch", len(messages))
	}
	statuses := deliveriesForWatch(t, env, rssAdded.WatchID)
	for eventID, status := range statuses {
		if status != domain.DeliverySent {
			t.Fatalf("other watch delivery %s = %s", eventID, status)
		}
	}
}

func TestRemoveAndPurgeUseScanLock(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)

	release, err := lock.Acquire(ctx, env.cfg.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, appErr := env.svc.Remove(ctx, added.WatchID); appErr == nil || appErr.Code != "locked" {
		t.Fatalf("remove with held lock = %+v", appErr)
	}
	if _, appErr := env.svc.Purge(ctx, added.WatchID); appErr == nil || appErr.Code != "locked" {
		t.Fatalf("purge with held lock = %+v", appErr)
	}
	release()

	if _, appErr := env.svc.Remove(ctx, added.WatchID); appErr != nil {
		t.Fatalf("remove after release = %+v", appErr)
	}
}
