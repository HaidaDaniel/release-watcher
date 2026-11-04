package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/HaidaDaniel/release-watcher/internal/cli"
)

type feedServer struct {
	mu      sync.Mutex
	entries []string
	fail    bool
	server  *httptest.Server
}

func newFeedServer(t *testing.T, entries ...string) *feedServer {
	t.Helper()
	f := &feedServer{entries: append([]string{}, entries...)}
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
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel><title>CLI Feed</title>`)
		for _, id := range f.entries {
			b.WriteString(`<item><title>Entry ` + id + `</title><link>https://example.test/` + id + `</link><guid>` + id + `</guid><pubDate>Mon, 02 Nov 2026 10:00:00 GMT</pubDate></item>`)
		}
		b.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *feedServer) set(entries ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append([]string{}, entries...)
}

func (f *feedServer) setFail(value bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = value
}

func runCLI(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	getenv := func(key string) string { return env[key] }
	code := cli.Run(context.Background(), args, &stdout, &stderr, getenv)
	return code, stdout.String(), stderr.String()
}

func decodeEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
	return got
}

func testEnv(t *testing.T) map[string]string {
	dir := t.TempDir()
	return map[string]string{
		"RELEASE_WATCHER_STATE_DIR":           dir,
		"RELEASE_WATCHER_DB":                  filepath.Join(dir, "release-watcher.db"),
		"RELEASE_WATCHER_ALLOW_PRIVATE_HOSTS": "true",
		"RELEASE_WATCHER_RATE_INTERVAL":       "0s",
		"RELEASE_WATCHER_MAX_RETRIES":         "0",
		"RELEASE_WATCHER_LOG_LEVEL":           "error",
	}
}

func dataMap(t *testing.T, got map[string]any) map[string]any {
	t.Helper()
	if got["ok"] != true {
		t.Fatalf("not ok: %v", got)
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %v", got["data"])
	}
	return data
}

func TestCLIJSONVerticalSlice(t *testing.T) {
	feed := newFeedServer(t, "A", "B")
	env := testEnv(t)
	url := feed.server.URL + "/feed.xml"

	code, stdout, stderr := runCLI(t, env, "add", url, "--json")
	if code != 0 {
		t.Fatalf("add exit %d stderr=%s", code, stderr)
	}
	addData := dataMap(t, decodeEnvelope(t, stdout))
	watchID, _ := addData["watch_id"].(string)
	if watchID == "" || addData["created"] != true {
		t.Fatalf("add data = %v", addData)
	}
	if addData["last_published_at"] == nil || addData["work_id"] == nil || addData["status_source"] == nil {
		t.Fatalf("add data missing documented fields: %v", addData)
	}

	code, stdout, _ = runCLI(t, env, "events", "--json")
	if code != 0 {
		t.Fatalf("events exit %d", code)
	}
	if events := dataMap(t, decodeEnvelope(t, stdout))["events"].([]any); len(events) != 0 {
		t.Fatalf("baseline events = %v", events)
	}

	code, stdout, _ = runCLI(t, env, "list", "--json")
	if code != 0 {
		t.Fatalf("list exit %d", code)
	}
	if watches := dataMap(t, decodeEnvelope(t, stdout))["watches"].([]any); len(watches) != 1 {
		t.Fatalf("watches = %v", watches)
	}

	code, stdout, _ = runCLI(t, env, "show", watchID, "--json")
	if code != 0 {
		t.Fatalf("show exit %d", code)
	}
	showData := dataMap(t, decodeEnvelope(t, stdout))
	if releases := showData["releases"].([]any); len(releases) != 2 {
		t.Fatalf("releases = %v", releases)
	}

	feed.set("A", "B", "C")
	code, stdout, _ = runCLI(t, env, "scan", "--id", watchID, "--json")
	if code != 0 {
		t.Fatalf("scan exit %d", code)
	}
	scanData := dataMap(t, decodeEnvelope(t, stdout))
	if scanData["events_created"] != float64(1) {
		t.Fatalf("scan = %v", scanData)
	}

	code, stdout, _ = runCLI(t, env, "events", "--json")
	if code != 0 {
		t.Fatalf("events exit %d", code)
	}
	eventsData := dataMap(t, decodeEnvelope(t, stdout))
	events := eventsData["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("events = %v", events)
	}
	event := events[0].(map[string]any)
	if event["schema_version"] != "1.0" || event["event_type"] != "release.added" {
		t.Fatalf("event = %v", event)
	}
	cursor, _ := eventsData["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing next cursor")
	}

	code, stdout, _ = runCLI(t, env, "events", "--json", "--after", cursor)
	if code != 0 {
		t.Fatalf("events after exit %d", code)
	}
	if len(dataMap(t, decodeEnvelope(t, stdout))["events"].([]any)) != 0 {
		t.Fatal("after cursor returned duplicate events")
	}

	code, stdout, _ = runCLI(t, env, "pause", watchID, "--json")
	if code != 0 || dataMap(t, decodeEnvelope(t, stdout))["enabled"] != false {
		t.Fatalf("pause = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "resume", watchID, "--json")
	if code != 0 || dataMap(t, decodeEnvelope(t, stdout))["enabled"] != true {
		t.Fatalf("resume = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "interval", watchID, "6h", "--json")
	if code != 0 || dataMap(t, decodeEnvelope(t, stdout))["interval_seconds"] != float64(21600) {
		t.Fatalf("interval = %s", stdout)
	}

	code, stdout, _ = runCLI(t, env, "remove", watchID, "--yes", "--json")
	if code != 0 || dataMap(t, decodeEnvelope(t, stdout))["removed"] != true {
		t.Fatalf("remove = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "list", "--json")
	if code != 0 || len(dataMap(t, decodeEnvelope(t, stdout))["watches"].([]any)) != 0 {
		t.Fatalf("list after remove = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "events", "--json")
	if code != 0 || len(dataMap(t, decodeEnvelope(t, stdout))["events"].([]any)) != 0 {
		t.Fatalf("events after remove = %s", stdout)
	}
}

func TestCLIErrorsAreJSONAndExitCodes(t *testing.T) {
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not a feed</html>`))
	}))
	defer html.Close()
	env := testEnv(t)

	code, stdout, _ := runCLI(t, env, "add", html.URL, "--json")
	if code != cli.ExitUsage {
		t.Fatalf("exit = %d", code)
	}
	got := decodeEnvelope(t, stdout)
	errShape := got["error"].(map[string]any)
	if got["ok"] != false || errShape["code"] != "unsupported_source" {
		t.Fatalf("error = %v", got)
	}

	code, stdout, _ = runCLI(t, env, "show", "missing", "--json")
	if code != cli.ExitError {
		t.Fatalf("exit = %d", code)
	}
	if decodeEnvelope(t, stdout)["error"].(map[string]any)["code"] != "not_found" {
		t.Fatalf("show error = %s", stdout)
	}

	code, _, _ = runCLI(t, env, "remove", "missing")
	if code != cli.ExitUsage {
		t.Fatalf("remove without --yes exit = %d", code)
	}
}

func TestCLIPartialScanExitCode(t *testing.T) {
	good := newFeedServer(t, "A")
	bad := newFeedServer(t, "B")
	env := testEnv(t)
	code, stdout, _ := runCLI(t, env, "add", good.server.URL+"/feed.xml", "--json")
	if code != 0 {
		t.Fatalf("add good = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "add", bad.server.URL+"/feed.xml", "--json")
	if code != 0 {
		t.Fatalf("add bad = %s", stdout)
	}
	bad.setFail(true)
	code, stdout, _ = runCLI(t, env, "scan", "--all", "--json")
	if code != cli.ExitPartial {
		t.Fatalf("exit = %d, want %d", code, cli.ExitPartial)
	}
	data := dataMap(t, decodeEnvelope(t, stdout))
	if data["scanned"] != float64(2) || data["succeeded"] != float64(1) || data["failed"] != float64(1) {
		t.Fatalf("scan = %v", data)
	}
}

func TestCLIHumanLanguage(t *testing.T) {
	feed := newFeedServer(t, "A")
	env := testEnv(t)
	code, stdout, _ := runCLI(t, env, "add", feed.server.URL+"/feed.xml")
	if code != 0 || !strings.Contains(stdout, "Добавлено") {
		t.Fatalf("ru add = %d %q", code, stdout)
	}
	code, stdout, _ = runCLI(t, env, "--lang", "en", "list")
	if code != 0 || !strings.Contains(stdout, "ID") {
		t.Fatalf("en list = %d %q", code, stdout)
	}
}
