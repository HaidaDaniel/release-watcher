package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/config"
	rwmcp "github.com/HaidaDaniel/release-watcher/internal/mcp"
	"github.com/HaidaDaniel/release-watcher/internal/source"
	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type feedFixture struct {
	mu      sync.Mutex
	entries []string
	server  *httptest.Server
}

func newFeedFixture(t *testing.T, entries ...string) *feedFixture {
	t.Helper()
	f := &feedFixture{entries: append([]string{}, entries...)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel><title>MCP Fixture</title>`)
		for i, id := range f.entries {
			fmt.Fprintf(&b, `<item><title>Entry %s</title><link>https://example.test/%s</link><guid>%s</guid><pubDate>%s</pubDate></item>`,
				id, id, id, time.Date(2026, 11, 1+i, 10, 0, 0, 0, time.UTC).Format(time.RFC1123Z))
		}
		b.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *feedFixture) set(entries ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append([]string{}, entries...)
}

type mcpEnv struct {
	server *httptest.Server
	svc    *app.Service
	feed   *feedFixture
}

func newEnv(t *testing.T, write bool) *mcpEnv {
	t.Helper()
	return newEnvWith(t, write, nil)
}

func newEnvWith(t *testing.T, write bool, allowedHosts []string) *mcpEnv {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		StateDir:          dir,
		DBPath:            filepath.Join(dir, "release-watcher.db"),
		LockPath:          filepath.Join(dir, "scan.lock"),
		DefaultInterval:   24 * time.Hour,
		Location:          time.UTC,
		HTTPTimeout:       2 * time.Second,
		UserAgent:         "release-watcher-mcp-test",
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
	feed := newFeedFixture(t, "A")
	client := httpx.New(httpx.Config{
		UserAgent: cfg.UserAgent, Timeout: cfg.HTTPTimeout, MaxRedirects: cfg.MaxRedirects,
		MaxRetries: cfg.MaxRetries, RateInterval: cfg.RateInterval, MaxBodyBytes: cfg.MaxBodyBytes,
		AllowPrivateHosts: cfg.AllowPrivateHosts,
	})
	reg := source.NewRegistryWithAdapters(source.NewNaro(client), source.NewFeed(client))
	st, err := store.Open(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := app.New(cfg, st, reg, logger)
	server, err := rwmcp.New(svc, rwmcp.Config{
		Token:         testToken,
		WriteEnabled:  write,
		AllowedHosts:  allowedHosts,
		MaxBodyBytes:  64 << 10,
		MaxConcurrent: 4,
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &mcpEnv{server: ts, svc: svc, feed: feed}
}

type authTransport struct{ token string }

func (a authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+a.token)
	return http.DefaultTransport.RoundTrip(clone)
}

func connect(t *testing.T, endpoint, token string) *sdkmcp.ClientSession {
	t.Helper()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &sdkmcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           &http.Client{Transport: authTransport{token: token}},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func call(t *testing.T, session *sdkmcp.ClientSession, name string, args any) *sdkmcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return result
}

func decode(t *testing.T, result *sdkmcp.CallToolResult, target any) {
	t.Helper()
	for _, content := range result.Content {
		if text, ok := content.(*sdkmcp.TextContent); ok {
			if err := json.Unmarshal([]byte(text.Text), target); err != nil {
				t.Fatalf("decode %q: %v", text.Text, err)
			}
			return
		}
	}
	t.Fatal("result has no text content")
}

func toolNames(t *testing.T, session *sdkmcp.ClientSession) map[string]bool {
	t.Helper()
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, tool := range result.Tools {
		out[tool.Name] = true
	}
	return out
}

func TestMCPReadToolsAndMutations(t *testing.T) {
	env := newEnv(t, true)
	session := connect(t, env.server.URL+"/mcp", testToken)
	if version := session.InitializeResult().ProtocolVersion; version == "" {
		t.Fatal("empty negotiated protocol version")
	}
	names := toolNames(t, session)
	for _, name := range []string{"list_watches", "get_watch", "list_events", "add_watch", "pause_watch", "resume_watch", "set_watch_interval", "remove_watch"} {
		if !names[name] {
			t.Fatalf("missing tool %s in %v", name, names)
		}
	}

	var list struct {
		Watches   []app.WatchView `json:"watches"`
		Total     int             `json:"total"`
		Truncated bool            `json:"truncated"`
	}
	decode(t, call(t, session, "list_watches", map[string]any{}), &list)
	if len(list.Watches) != 0 || list.Total != 0 {
		t.Fatalf("initial watches = %+v", list)
	}

	var added app.AddResult
	decode(t, call(t, session, "add_watch", map[string]any{"url": env.feed.server.URL + "/feed.xml", "interval": "24h"}), &added)
	if !added.Created || added.BaselineReleases != 1 {
		t.Fatalf("add = %+v", added)
	}
	var again app.AddResult
	decode(t, call(t, session, "add_watch", map[string]any{"url": env.feed.server.URL + "/feed.xml"}), &again)
	if again.Created || again.Reason != "already_exists" || again.WatchID != added.WatchID {
		t.Fatalf("duplicate add = %+v", again)
	}

	decode(t, call(t, session, "list_watches", map[string]any{}), &list)
	if len(list.Watches) != 1 || list.Watches[0].WatchID != added.WatchID {
		t.Fatalf("watches = %+v", list)
	}

	var shown app.ShowResult
	decode(t, call(t, session, "get_watch", map[string]any{"watch_id": added.WatchID}), &shown)
	if shown.Watch.WatchID != added.WatchID || len(shown.Releases) != 1 {
		t.Fatalf("show = %+v", shown)
	}

	env.feed.set("A", "B")
	if _, appErr := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "all", NoNotify: true}); appErr != nil {
		t.Fatal(appErr)
	}
	var events app.EventsResult
	decode(t, call(t, session, "list_events", map[string]any{"limit": 10}), &events)
	if len(events.Events) != 1 || events.NextCursor == "" {
		t.Fatalf("events = %+v", events)
	}
	var eventDoc struct {
		EventType  string   `json:"event_type"`
		ReleaseIDs []string `json:"release_ids"`
	}
	if err := json.Unmarshal(events.Events[0], &eventDoc); err != nil {
		t.Fatal(err)
	}
	if eventDoc.EventType != "release.added" || strings.Join(eventDoc.ReleaseIDs, ",") != "B" {
		t.Fatalf("event = %+v", eventDoc)
	}

	var paused app.WatchView
	decode(t, call(t, session, "pause_watch", map[string]any{"watch_id": added.WatchID}), &paused)
	if paused.Enabled {
		t.Fatalf("pause = %+v", paused)
	}
	var resumed app.WatchView
	decode(t, call(t, session, "resume_watch", map[string]any{"watch_id": added.WatchID}), &resumed)
	if !resumed.Enabled {
		t.Fatalf("resume = %+v", resumed)
	}
	var changed app.WatchView
	decode(t, call(t, session, "set_watch_interval", map[string]any{"watch_id": added.WatchID, "interval": "48h"}), &changed)
	if changed.IntervalSeconds != 172800 {
		t.Fatalf("interval = %+v", changed)
	}

	var removed app.RemoveResult
	decode(t, call(t, session, "remove_watch", map[string]any{"watch_id": added.WatchID}), &removed)
	if !removed.Removed || !removed.HistoryPreserved {
		t.Fatalf("remove = %+v", removed)
	}
	decode(t, call(t, session, "list_watches", map[string]any{}), &list)
	if len(list.Watches) != 0 {
		t.Fatalf("watches after remove = %+v", list)
	}
	decode(t, call(t, session, "get_watch", map[string]any{"watch_id": added.WatchID}), &shown)
	if shown.Watch.RemovedAt == nil || len(shown.Releases) != 2 {
		t.Fatalf("history after remove = %+v", shown)
	}
}

func TestMCPReadOnlyMode(t *testing.T) {
	env := newEnv(t, false)
	session := connect(t, env.server.URL+"/mcp", testToken)
	names := toolNames(t, session)
	for _, name := range []string{"add_watch", "pause_watch", "resume_watch", "set_watch_interval", "remove_watch"} {
		if names[name] {
			t.Fatalf("read-only server advertised %s", name)
		}
	}
	if _, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "add_watch", Arguments: map[string]any{"url": env.feed.server.URL + "/feed.xml"}}); err == nil {
		t.Fatal("read-only server executed add_watch")
	}
}

func TestMCPAuthenticationAndHostOrigin(t *testing.T) {
	env := newEnv(t, true)
	post := func(token, origin, host string) int {
		req, err := http.NewRequest(http.MethodPost, env.server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if host != "" {
			req.Host = host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if got := post("", "", ""); got != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d", got)
	}
	if got := post("wrong", "", ""); got != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d", got)
	}
	if got := post(testToken, "http://evil.example", ""); got != http.StatusForbidden {
		t.Fatalf("bad origin status = %d", got)
	}
	if got := post(testToken, "", "evil.example"); got != http.StatusForbidden {
		t.Fatalf("bad host status = %d", got)
	}
}

func TestMCPPaginationAndCancellation(t *testing.T) {
	env := newEnv(t, true)
	session := connect(t, env.server.URL+"/mcp", testToken)
	var added app.AddResult
	decode(t, call(t, session, "add_watch", map[string]any{"url": env.feed.server.URL + "/feed.xml", "interval": "24h"}), &added)
	env.feed.set("A", "B")
	if _, appErr := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "all", NoNotify: true}); appErr != nil {
		t.Fatal(appErr)
	}
	env.feed.set("A", "B", "C")
	if _, appErr := env.svc.Scan(context.Background(), app.ScanOptions{Mode: "all", NoNotify: true}); appErr != nil {
		t.Fatal(appErr)
	}
	var first app.EventsResult
	decode(t, call(t, session, "list_events", map[string]any{"limit": 1}), &first)
	if len(first.Events) != 1 || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	var second app.EventsResult
	decode(t, call(t, session, "list_events", map[string]any{"limit": 1, "after": first.NextCursor}), &second)
	if len(second.Events) != 1 || second.NextCursor == first.NextCursor {
		t.Fatalf("second page = %+v", second)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "list_watches", Arguments: map[string]any{}}); err == nil {
		t.Fatal("canceled call succeeded")
	}
}

func TestMCPAllowedHostNormalization(t *testing.T) {
	env := newEnvWith(t, true, []string{"private.example.com:443"})
	req, err := http.NewRequest(http.MethodPost, env.server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Host = "private.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("configured allowed host was rejected")
	}
}

func TestMCPOversizedBodyAndValidation(t *testing.T) {
	env := newEnv(t, true)
	post := func(body string) int {
		req, err := http.NewRequest(http.MethodPost, env.server.URL+"/mcp", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(strings.Repeat("x", 70<<10)); got != http.StatusRequestEntityTooLarge && got != http.StatusBadRequest {
		t.Fatalf("oversized body status = %d", got)
	}
	session := connect(t, env.server.URL+"/mcp", testToken)
	result, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "list_events", Arguments: map[string]any{"limit": 1000}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("invalid limit was accepted")
	}
	result, err = session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "add_watch", Arguments: map[string]any{"url": env.feed.server.URL + "/feed.xml", "interval": "not-a-duration"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("invalid interval was accepted")
	}
}

func TestMCPPropagatesRequestCancellation(t *testing.T) {
	env := newEnv(t, true)
	started := make(chan struct{})
	canceled := make(chan struct{})
	var once sync.Once
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(5 * time.Second):
		}
	}))
	defer slow.Close()
	session := connect(t, env.server.URL+"/mcp", testToken)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "add_watch", Arguments: map[string]any{"url": slow.URL + "/feed.xml"}})
		errCh <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("source request did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("server-side source request was not canceled in flight")
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("canceled tool call returned success")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled tool call did not return")
	}
}
