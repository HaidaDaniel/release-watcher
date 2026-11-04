package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/config"
	"github.com/HaidaDaniel/release-watcher/internal/notifier"
	"github.com/HaidaDaniel/release-watcher/internal/source"
	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

const naroURL = "https://ncode.syosetu.com/n2166fe/"

type naroServer struct {
	mu          sync.Mutex
	title       string
	chapters    []string
	failAPI     bool
	failCatalog bool
	server      *httptest.Server
}

func newNaroServer(t *testing.T, chapters []string) *naroServer {
	t.Helper()
	n := &naroServer{title: "E2E 作品", chapters: append([]string{}, chapters...)}
	n.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.mu.Lock()
		defer n.mu.Unlock()
		switch {
		case r.URL.Path == "/novelapi/api/":
			if n.failAPI {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			payload := []any{
				map[string]any{"allcount": 1},
				map[string]any{
					"title":          n.title,
					"ncode":          "N2166FE",
					"general_all_no": len(n.chapters),
					"general_lastup": "2023-08-01 22:25:09",
					"novel_type":     1,
					"end":            1,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		case strings.HasPrefix(r.URL.Path, "/n2166fe/"):
			if n.failCatalog {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			page := 1
			if raw := r.URL.Query().Get("p"); raw != "" {
				_, _ = fmt.Sscanf(raw, "%d", &page)
			}
			start := (page - 1) * 100
			if start >= len(n.chapters) {
				_, _ = w.Write([]byte(`<div class="p-eplist"></div>`))
				return
			}
			end := start + 100
			if end > len(n.chapters) {
				end = len(n.chapters)
			}
			var b strings.Builder
			b.WriteString(`<div class="p-eplist">`)
			for i := start; i < end; i++ {
				fmt.Fprintf(&b, `<div class="p-eplist__sublist"><a href="/n2166fe/%d/" class="p-eplist__subtitle">%s</a><div class="p-eplist__update">2023/08/01 22:25</div></div>`, i+1, n.chapters[i])
			}
			b.WriteString(`</div>`)
			_, _ = w.Write([]byte(b.String()))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(n.server.Close)
	return n
}

func (n *naroServer) setChapters(chapters []string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.chapters = append([]string{}, chapters...)
}

func (n *naroServer) setFailAPI(value bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failAPI = value
}

type telegramServer struct {
	mu       sync.Mutex
	fail     bool
	messages []string
	requests int
	server   *httptest.Server
}

func newTelegramServer(t *testing.T) *telegramServer {
	t.Helper()
	tg := &telegramServer{}
	tg.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tg.mu.Lock()
		defer tg.mu.Unlock()
		tg.requests++
		if !strings.Contains(r.URL.Path, "/botTEST_TOKEN/sendMessage") {
			http.NotFound(w, r)
			return
		}
		if tg.fail {
			http.Error(w, "telegram down", http.StatusInternalServerError)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		tg.messages = append(tg.messages, r.Form.Get("text"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(tg.server.Close)
	return tg
}

func (tg *telegramServer) setFail(value bool) {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.fail = value
}

func (tg *telegramServer) snapshot() ([]string, int) {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	return append([]string{}, tg.messages...), tg.requests
}

func naroChapters(count int) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = fmt.Sprintf("Chapter %d", i+1)
	}
	return out
}

func newService(t *testing.T, n *naroServer, tg *telegramServer) (*app.Service, *store.Store, config.Config) {
	t.Helper()
	dir := t.TempDir()
	return newServiceWithConfig(t, n, tg, config.Config{
		StateDir:          dir,
		DBPath:            filepath.Join(dir, "release-watcher.db"),
		LockPath:          filepath.Join(dir, "scan.lock"),
		DefaultInterval:   time.Hour,
		HTTPTimeout:       5 * time.Second,
		UserAgent:         "release-watcher-e2e",
		Lang:              "ru",
		LogFormat:         "text",
		LogLevel:          "error",
		MaxDeliveryTries:  3,
		MaxRedirects:      3,
		RateInterval:      0,
		MaxRetries:        0,
		MaxBodyBytes:      1 << 20,
		AllowPrivateHosts: true,
	})
}

func newServiceWithConfig(t *testing.T, n *naroServer, tg *telegramServer, cfg config.Config) (*app.Service, *store.Store, config.Config) {
	t.Helper()
	if tg != nil {
		cfg.TelegramToken = "TEST_TOKEN"
		cfg.TelegramChatID = "chat"
		cfg.TelegramAPIBase = tg.server.URL
	}
	client := httpx.New(httpx.Config{
		UserAgent: cfg.UserAgent, Timeout: cfg.HTTPTimeout, MaxRedirects: cfg.MaxRedirects,
		MaxRetries: cfg.MaxRetries, RateInterval: cfg.RateInterval, MaxBodyBytes: cfg.MaxBodyBytes,
		AllowPrivateHosts: cfg.AllowPrivateHosts,
	})
	naro := source.NewNaro(client)
	naro.APIBase = n.server.URL + "/novelapi/api/"
	naro.CatalogBase = n.server.URL + "/"
	reg := source.NewRegistryWithAdapters(naro, source.NewFeed(client))
	st, err := store.Open(t.Context(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	logger := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := app.New(cfg, st, reg, logger)
	if tg != nil {
		svc.SetNotifier(notifier.NewTelegram(cfg.TelegramAPIBase, cfg.TelegramToken, cfg.TelegramChatID, cfg.HTTPTimeout))
	}
	return svc, st, cfg
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

func scan(t *testing.T, svc *app.Service, id string) *app.ScanResult {
	t.Helper()
	result, err := svc.Scan(context.Background(), app.ScanOptions{Mode: "id", ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func events(t *testing.T, svc *app.Service) []json.RawMessage {
	t.Helper()
	result, err := svc.Events(context.Background(), app.EventQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return result.Events
}

func TestNaroEndToEndWithTelegramOutbox(t *testing.T) {
	n := newNaroServer(t, naroChapters(210))
	tg := newTelegramServer(t)
	svc, st, cfg := newService(t, n, tg)
	ctx := context.Background()
	clock := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return clock })

	added, addErr := svc.Add(ctx, app.AddInput{URL: naroURL})
	if addErr != nil {
		t.Fatal(addErr)
	}
	if !added.Created || added.BaselineReleases != 210 {
		t.Fatalf("add = %+v", added)
	}
	if got := events(t, svc); len(got) != 0 {
		t.Fatalf("baseline events = %d", len(got))
	}

	unchanged := scan(t, svc, added.WatchID)
	if unchanged.EventsCreated != 0 || unchanged.Notifications.Sent != 0 {
		t.Fatalf("unchanged scan = %+v", unchanged)
	}

	n.setChapters(naroChapters(212))
	first := scan(t, svc, added.WatchID)
	if first.EventsCreated != 1 || first.Notifications.Sent != 1 || first.Notifications.Pending != 0 {
		t.Fatalf("two-chapter scan = %+v", first)
	}
	msgs, _ := tg.snapshot()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "2 главы") || !strings.Contains(msgs[0], "211–212") {
		t.Fatalf("telegram messages = %q", msgs)
	}
	got := events(t, svc)
	if len(got) != 1 {
		t.Fatalf("events = %d", len(got))
	}
	var doc struct {
		EventType  string   `json:"event_type"`
		ReleaseIDs []string `json:"release_ids"`
	}
	if err := json.Unmarshal(got[0], &doc); err != nil {
		t.Fatal(err)
	}
	if doc.EventType != "release.added" || strings.Join(doc.ReleaseIDs, ",") != "N2166FE/211,N2166FE/212" {
		t.Fatalf("event = %+v", doc)
	}

	replay := scan(t, svc, added.WatchID)
	if replay.EventsCreated != 0 {
		t.Fatalf("replay = %+v", replay)
	}
	if msgs, _ = tg.snapshot(); len(msgs) != 1 {
		t.Fatalf("replay sent duplicate messages: %d", len(msgs))
	}

	n.setChapters(naroChapters(214))
	tg.setFail(true)
	failed := scan(t, svc, added.WatchID)
	if failed.EventsCreated != 1 || failed.Notifications.Failed != 1 {
		t.Fatalf("telegram failure scan = %+v", failed)
	}
	if got = events(t, svc); len(got) != 2 {
		t.Fatalf("events after failure = %d", len(got))
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	svc2, _, _ := newServiceWithConfig(t, n, tg, cfg)
	svc2.SetNow(func() time.Time { return clock.Add(2 * time.Minute) })
	tg.setFail(false)
	due := mustScan(t, svc2, app.ScanOptions{Mode: "due"})
	if due.Scanned != 0 {
		t.Fatalf("expected no due watches, got %+v", due)
	}
	if due.Notifications.Sent != 2 || due.Notifications.Failed != 0 {
		t.Fatalf("retry on scan --due = %+v", due)
	}
	if msgs, _ = tg.snapshot(); len(msgs) != 2 || !strings.Contains(msgs[1], "2 главы") {
		t.Fatalf("retried messages = %q", msgs)
	}
	if cfg.TelegramToken != "TEST_TOKEN" {
		t.Fatal("test config lost Telegram token")
	}
}

func mustScan(t *testing.T, svc *app.Service, opts app.ScanOptions) *app.ScanResult {
	t.Helper()
	result, err := svc.Scan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type rssServer struct {
	mu      sync.Mutex
	entries []string
	server  *httptest.Server
}

func newRSSServer(t *testing.T, entries []string) *rssServer {
	t.Helper()
	r := &rssServer{entries: append([]string{}, entries...)}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		defer r.mu.Unlock()
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel><title>E2E Feed</title>`)
		for _, id := range r.entries {
			fmt.Fprintf(&b, `<item><title>Entry %s</title><link>https://example.test/%s</link><guid>%s</guid><pubDate>Mon, 02 Nov 2026 10:00:00 GMT</pubDate></item>`, id, id, id)
		}
		b.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *rssServer) set(entries ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append([]string{}, entries...)
}

func TestScanIsolatesUnavailableSourceFromUpdatingSource(t *testing.T) {
	n := newNaroServer(t, naroChapters(2))
	tg := newTelegramServer(t)
	svc, _, _ := newService(t, n, tg)
	ctx := context.Background()
	clock := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return clock })

	naroAdded, err := svc.Add(ctx, app.AddInput{URL: naroURL})
	if err != nil {
		t.Fatal(err)
	}
	feed := newRSSServer(t, []string{"one"})
	rssAdded, addErr := svc.Add(ctx, app.AddInput{URL: feed.server.URL + "/feed.xml"})
	if addErr != nil {
		t.Fatal(addErr)
	}

	n.setFailAPI(true)
	feed.set("one", "two")
	result := mustScan(t, svc, app.ScanOptions{Mode: "all"})
	if result.Scanned != 2 || result.Succeeded != 1 || result.Failed != 1 || result.EventsCreated != 1 {
		t.Fatalf("isolated scan = %+v", result)
	}
	outcomes := map[string]string{}
	for _, watchResult := range result.Results {
		outcomes[watchResult.WatchID] = watchResult.Outcome
	}
	if outcomes[naroAdded.WatchID] != "error" || outcomes[rssAdded.WatchID] != "ok" {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	got := events(t, svc)
	if len(got) != 1 {
		t.Fatalf("events = %d", len(got))
	}
	var doc struct {
		ReleaseIDs []string `json:"release_ids"`
	}
	if err := json.Unmarshal(got[0], &doc); err != nil {
		t.Fatal(err)
	}
	if strings.Join(doc.ReleaseIDs, ",") != "two" {
		t.Fatalf("event release ids = %v", doc.ReleaseIDs)
	}
}
