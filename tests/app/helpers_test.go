package app_test

import (
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
	"github.com/HaidaDaniel/release-watcher/internal/source"
	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

type naroFixture struct {
	mu          sync.Mutex
	title       string
	chapters    []string
	end         int
	failAPI     bool
	failCatalog bool
	server      *httptest.Server
}

func newNaroFixture(t *testing.T, chapters []string) *naroFixture {
	t.Helper()
	f := &naroFixture{title: "テスト作品", chapters: append([]string{}, chapters...), end: 1}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/novelapi/api/":
			if f.failAPI {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			payload := []any{
				map[string]any{"allcount": 1},
				map[string]any{
					"title":          f.title,
					"ncode":          "N2166FE",
					"general_all_no": len(f.chapters),
					"general_lastup": "2023-08-01 22:25:09",
					"novel_type":     1,
					"end":            f.end,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		case strings.HasPrefix(r.URL.Path, "/n2166fe/"):
			if f.failCatalog {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(naroCatalog(f.chapters)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func naroCatalog(chapters []string) string {
	var b strings.Builder
	b.WriteString(`<div class="p-eplist">`)
	for i, title := range chapters {
		fmt.Fprintf(&b, `<div class="p-eplist__sublist"><a href="/n2166fe/%d/" class="p-eplist__subtitle">%s</a><div class="p-eplist__update">2023/08/01 22:25</div></div>`, i+1, title)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func (f *naroFixture) setChapters(chapters []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chapters = append([]string{}, chapters...)
}

func (f *naroFixture) setFailAPI(value bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failAPI = value
}

func (f *naroFixture) setFailCatalog(value bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failCatalog = value
}

type rssEntry struct {
	ID    string
	Title string
	Date  string
}

type rssFixture struct {
	mu      sync.Mutex
	title   string
	entries []rssEntry
	fail    bool
	server  *httptest.Server
}

func newRSSFixture(t *testing.T, entries []rssEntry) *rssFixture {
	t.Helper()
	f := &rssFixture{title: "Fixture RSS", entries: append([]rssEntry{}, entries...)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/feed.xml" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		var b strings.Builder
		fmt.Fprintf(&b, `<?xml version="1.0"?><rss version="2.0"><channel><title>%s</title><link>%s</link>`, f.title, f.server.URL)
		for _, entry := range f.entries {
			fmt.Fprintf(&b, `<item><title>%s</title><link>https://example.test/novel/%s</link><guid>%s</guid><pubDate>%s</pubDate></item>`, entry.Title, entry.ID, entry.ID, entry.Date)
		}
		b.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *rssFixture) setEntries(entries []rssEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append([]rssEntry{}, entries...)
}

func (f *rssFixture) setFail(value bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = value
}

type testEnv struct {
	svc   *app.Service
	store *store.Store
	cfg   config.Config
	naro  *naroFixture
}

func newTestEnv(t *testing.T, naro *naroFixture) *testEnv {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		StateDir:          dir,
		DBPath:            filepath.Join(dir, "release-watcher.db"),
		LockPath:          filepath.Join(dir, "scan.lock"),
		DefaultInterval:   time.Hour,
		HTTPTimeout:       2 * time.Second,
		UserAgent:         "release-watcher-test",
		Lang:              "ru",
		LogFormat:         "text",
		LogLevel:          "error",
		MaxDeliveryTries:  3,
		MaxRedirects:      3,
		RateInterval:      0,
		MaxRetries:        0,
		MaxBodyBytes:      1 << 20,
		AllowPrivateHosts: true,
	}
	client := httpx.New(httpx.Config{
		UserAgent:         cfg.UserAgent,
		Timeout:           cfg.HTTPTimeout,
		MaxRedirects:      cfg.MaxRedirects,
		MaxRetries:        cfg.MaxRetries,
		RateInterval:      cfg.RateInterval,
		MaxBodyBytes:      cfg.MaxBodyBytes,
		AllowPrivateHosts: cfg.AllowPrivateHosts,
	})
	naroAdapter := source.NewNaro(client)
	naroAdapter.APIBase = "https://api.syosetu.com/novelapi/api/"
	naroAdapter.CatalogBase = "https://ncode.syosetu.com/"
	if naro != nil {
		naroAdapter.APIBase = naro.server.URL + "/novelapi/api/"
		naroAdapter.CatalogBase = naro.server.URL + "/"
	}
	reg := source.NewRegistryWithAdapters(naroAdapter, source.NewFeed(client))
	st, err := store.Open(t.Context(), cfg.DBPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := app.New(cfg, st, reg, logger)
	t.Cleanup(func() { _ = st.Close() })
	return &testEnv{svc: svc, store: st, cfg: cfg, naro: naro}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

const naroURL = "https://ncode.syosetu.com/n2166fe/"

func rssEntries(ids ...string) []rssEntry {
	out := make([]rssEntry, 0, len(ids))
	for _, id := range ids {
		day := 1
		if len(id) > 0 {
			day = int(id[0]-'A') + 1
			if day < 1 || day > 28 {
				day = 1
			}
		}
		out = append(out, rssEntry{
			ID:    id,
			Title: "Entry " + id,
			Date:  time.Date(2026, 11, day, 10, 0, 0, 0, time.UTC).Format(time.RFC1123Z),
		})
	}
	return out
}

func (f *naroFixture) setMeta(title string, end int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.title = title
	f.end = end
}

func openServiceForConfig(t *testing.T, cfg config.Config, naro *naroFixture) *testEnv {
	t.Helper()
	client := httpx.New(httpx.Config{
		UserAgent: cfg.UserAgent, Timeout: cfg.HTTPTimeout, MaxRedirects: cfg.MaxRedirects,
		MaxRetries: cfg.MaxRetries, RateInterval: cfg.RateInterval, MaxBodyBytes: cfg.MaxBodyBytes,
		AllowPrivateHosts: cfg.AllowPrivateHosts,
	})
	naroAdapter := source.NewNaro(client)
	if naro != nil {
		naroAdapter.APIBase = naro.server.URL + "/novelapi/api/"
		naroAdapter.CatalogBase = naro.server.URL + "/"
	}
	reg := source.NewRegistryWithAdapters(naroAdapter, source.NewFeed(client))
	st, err := store.Open(t.Context(), cfg.DBPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := app.New(cfg, st, reg, logger)
	t.Cleanup(func() { _ = st.Close() })
	return &testEnv{svc: svc, store: st, cfg: cfg, naro: naro}
}
