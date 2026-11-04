package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/lock"
	"github.com/HaidaDaniel/release-watcher/internal/notifier"
)

type telegramFixture struct {
	mu         sync.Mutex
	messages   []string
	requests   int
	failStatus int
	failCount  int
	retryAfter int
	server     *httptest.Server
}

func newTelegramFixture(t *testing.T) *telegramFixture {
	t.Helper()
	f := &telegramFixture{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		if !strings.Contains(r.URL.Path, "/botTEST_TOKEN/sendMessage") {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if f.failCount > 0 {
			f.failCount--
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.failStatus)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":false,"error_code":400,"description":"fixture failure","parameters":{"retry_after":%d}}`, f.retryAfter)))
			return
		}
		f.messages = append(f.messages, r.Form.Get("text"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *telegramFixture) notifier() *notifier.Telegram {
	return notifier.NewTelegram(f.server.URL, "TEST_TOKEN", "chat-1", time.Second)
}

func (f *telegramFixture) snapshot() ([]string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.messages...), f.requests
}

func newNotifyEnv(t *testing.T, n *naroFixture, tg *telegramFixture) *testEnv {
	env := newTestEnv(t, n)
	env.svc.SetNotifier(tg.notifier())
	return env
}

func TestTelegramGroupedMessageAndDedupe(t *testing.T) {
	n := newNaroFixture(t, []string{"One", "Two", "Three"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	added := addNaro(t, env, nil)

	n.setChapters([]string{"One", "Two", "Three", "Four", "Five"})
	scan := scanID(t, env, added.WatchID)
	if scan.Notifications.Sent != 1 || scan.Notifications.Pending != 0 || scan.Notifications.Failed != 0 {
		t.Fatalf("notifications = %+v", scan.Notifications)
	}
	messages, requests := tg.snapshot()
	if len(messages) != 1 || requests != 1 {
		t.Fatalf("messages=%d requests=%d", len(messages), requests)
	}
	msg := messages[0]
	for _, want := range []string{"появились 2 главы", "4–5", "Открыть оригинал"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}

	scanID(t, env, added.WatchID)
	messages, requests = tg.snapshot()
	if len(messages) != 1 || requests != 1 {
		t.Fatalf("replay sent duplicate: messages=%d requests=%d", len(messages), requests)
	}
	counts, err := env.svc.FlushDeliveries(ctx)
	if err != nil || counts.Sent != 1 || counts.Pending != 0 {
		t.Fatalf("flush = %+v err=%v", counts, err)
	}
}

func TestTelegramFailureRetry(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addNaro(t, env, nil)

	tg.mu.Lock()
	tg.failStatus = http.StatusInternalServerError
	tg.failCount = 1
	tg.mu.Unlock()
	n.setChapters([]string{"One", "Two"})
	scan := scanID(t, env, added.WatchID)
	if scan.Notifications.Failed != 1 {
		t.Fatalf("failed notifications = %+v", scan.Notifications)
	}
	messages, _ := tg.snapshot()
	if len(messages) != 0 {
		t.Fatal("failure still delivered a message")
	}

	clock = clock.Add(2 * time.Minute)
	counts, appErr := env.svc.FlushDeliveries(ctx)
	if appErr != nil {
		t.Fatal(appErr)
	}
	if counts.Sent != 1 || counts.Failed != 0 {
		t.Fatalf("retry counts = %+v", counts)
	}
	messages, _ = tg.snapshot()
	if len(messages) != 1 {
		t.Fatalf("retry messages = %d", len(messages))
	}
}

func TestTelegramPermanentFailureIsNotRetried(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addNaro(t, env, nil)

	tg.mu.Lock()
	tg.failStatus = http.StatusBadRequest
	tg.failCount = 100
	tg.mu.Unlock()
	n.setChapters([]string{"One", "Two"})
	scanID(t, env, added.WatchID)
	_, before := tg.snapshot()

	clock = clock.Add(365 * 24 * time.Hour)
	if _, appErr := env.svc.FlushDeliveries(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	_, after := tg.snapshot()
	if after != before {
		t.Fatalf("permanent failure retried: before=%d after=%d", before, after)
	}
}

func TestEnglishMessageFormatting(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	env.svc.SetLanguage("en")
	added := addNaro(t, env, nil)
	n.setChapters([]string{"One", "Two"})
	scanID(t, env, added.WatchID)
	messages, _ := tg.snapshot()
	if len(messages) != 1 || !strings.Contains(messages[0], "1 new chapter") || !strings.Contains(messages[0], "Open original") {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestTelegramNonJSON5xxAnd429AreRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	}))
	defer server.Close()
	tg := notifier.NewTelegram(server.URL, "TEST_TOKEN", "chat", time.Second)
	err := tg.Send(context.Background(), "hello")
	var nErr *notifier.Error
	if !errors.As(err, &nErr) || !nErr.Retryable {
		t.Fatalf("502 non-json err = %#v", err)
	}

	server429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("slow down"))
	}))
	defer server429.Close()
	tg429 := notifier.NewTelegram(server429.URL, "TEST_TOKEN", "chat", time.Second)
	err = tg429.Send(context.Background(), "hello")
	if !errors.As(err, &nErr) || !nErr.Retryable {
		t.Fatalf("429 non-json err = %#v", err)
	}
}

func TestTelegramForbiddenAndInvalidResponse(t *testing.T) {
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":403,"description":"forbidden"}`))
	}))
	defer forbidden.Close()
	err := notifier.NewTelegram(forbidden.URL, "TEST_TOKEN", "chat", time.Second).Send(context.Background(), "hello")
	var nErr *notifier.Error
	if !errors.As(err, &nErr) || nErr.Retryable || nErr.Code != notifier.CodeTelegramForbidden {
		t.Fatalf("403 err = %#v", err)
	}

	invalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer invalid.Close()
	err = notifier.NewTelegram(invalid.URL, "TEST_TOKEN", "chat", time.Second).Send(context.Background(), "hello")
	if !errors.As(err, &nErr) || nErr.Retryable || nErr.Code != notifier.CodeTelegramInvalid {
		t.Fatalf("invalid err = %#v", err)
	}
}

func TestTelegram429RetryAfter(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addNaro(t, env, nil)

	tg.mu.Lock()
	tg.failStatus = http.StatusTooManyRequests
	tg.failCount = 1
	tg.retryAfter = 120
	tg.mu.Unlock()
	n.setChapters([]string{"One", "Two"})
	scanID(t, env, added.WatchID)
	_, before := tg.snapshot()

	clock = clock.Add(time.Minute)
	if _, appErr := env.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	_, afterShort := tg.snapshot()
	if afterShort != before {
		t.Fatal("retry_after was ignored")
	}
	clock = clock.Add(2 * time.Minute)
	if _, appErr := env.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	messages, _ := tg.snapshot()
	if len(messages) != 1 {
		t.Fatalf("messages = %d", len(messages))
	}
}

func TestFailedDeliveryPersistsAcrossRestart(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	ctx := context.Background()
	added := addNaro(t, env, nil)
	tg.mu.Lock()
	tg.failStatus = http.StatusInternalServerError
	tg.failCount = 1
	tg.mu.Unlock()
	n.setChapters([]string{"One", "Two"})
	scanID(t, env, added.WatchID)
	if err := env.store.Close(); err != nil {
		t.Fatal(err)
	}

	restarted := openServiceForConfig(t, env.cfg, n)
	restarted.svc.SetNotifier(tg.notifier())
	restarted.svc.SetNow(func() time.Time { return clock.Add(2 * time.Minute) })
	if _, appErr := restarted.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	messages, _ := tg.snapshot()
	if len(messages) != 1 {
		t.Fatalf("messages after restart = %d", len(messages))
	}
}

func TestNotifierErrorDoesNotLeakToken(t *testing.T) {
	tg := notifier.NewTelegram("http://127.0.0.1:1", "SECRET_TOKEN", "chat", 100*time.Millisecond)
	err := tg.Send(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected send failure")
	}
	if strings.Contains(err.Error(), "SECRET_TOKEN") {
		t.Fatalf("token leaked in error: %v", err)
	}
}

func TestNotifyUsesScanLock(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	release, err := lock.Acquire(ctx, env.cfg.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, appErr := env.svc.Notify(ctx); appErr == nil || appErr.Code != "locked" {
		t.Fatalf("notify with held lock = %+v", appErr)
	}
	release()
	if _, appErr := env.svc.Notify(ctx); appErr != nil {
		t.Fatalf("notify after release = %+v", appErr)
	}
}

func TestTelegramMaxAttemptsBounded(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	env.svc.SetNow(func() time.Time { return clock })
	added := addNaro(t, env, nil)

	tg.mu.Lock()
	tg.failStatus = http.StatusInternalServerError
	tg.failCount = 100
	tg.mu.Unlock()
	n.setChapters([]string{"One", "Two"})
	scanID(t, env, added.WatchID)

	clock = clock.Add(2 * time.Minute)
	if _, appErr := env.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	clock = clock.Add(3 * time.Minute)
	if _, appErr := env.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	_, afterMaxRequests := tg.snapshot()
	if afterMaxRequests != 3 {
		t.Fatalf("attempts = %d, want 3", afterMaxRequests)
	}
	clock = clock.Add(365 * 24 * time.Hour)
	if _, appErr := env.svc.Notify(ctx); appErr != nil {
		t.Fatal(appErr)
	}
	_, afterYearRequests := tg.snapshot()
	if afterYearRequests != afterMaxRequests {
		t.Fatalf("retried after max attempts: %d -> %d", afterMaxRequests, afterYearRequests)
	}
}

func TestDeliveryLastErrorDoesNotContainToken(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	tg := newTelegramFixture(t)
	env := newNotifyEnv(t, n, tg)
	ctx := context.Background()
	added := addNaro(t, env, nil)
	tg.mu.Lock()
	tg.failStatus = http.StatusInternalServerError
	tg.failCount = 1
	tg.mu.Unlock()
	n.setChapters([]string{"One", "Two"})
	scanID(t, env, added.WatchID)
	events, _ := env.svc.Events(ctx, app.EventQuery{})
	docs := decodeEvents(t, events)
	if len(docs) != 1 {
		t.Fatalf("events = %d", len(docs))
	}
	deliveries, err := env.store.Queries().ListDeliveriesByEvent(ctx, docs[0].EventID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("deliveries = %+v err=%v", deliveries, err)
	}
	if strings.Contains(deliveries[0].LastError, "TEST_TOKEN") {
		t.Fatalf("token leaked in last_error: %q", deliveries[0].LastError)
	}
}
