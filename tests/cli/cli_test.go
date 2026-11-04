package cli_test

import (
	"bytes"
	"context"
	"database/sql"
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
	if code != 0 {
		t.Fatalf("remove = %s", stdout)
	}
	removeData := dataMap(t, decodeEnvelope(t, stdout))
	if removeData["removed"] != true || removeData["history_preserved"] != true {
		t.Fatalf("remove = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "list", "--json")
	if code != 0 || len(dataMap(t, decodeEnvelope(t, stdout))["watches"].([]any)) != 0 {
		t.Fatalf("list after remove = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "events", "--json")
	if code != 0 || len(dataMap(t, decodeEnvelope(t, stdout))["events"].([]any)) != 1 {
		t.Fatalf("events after remove = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "purge", watchID, "--yes", "--json")
	if code != 0 || dataMap(t, decodeEnvelope(t, stdout))["purged"] != true {
		t.Fatalf("purge = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "events", "--json")
	if code != 0 || len(dataMap(t, decodeEnvelope(t, stdout))["events"].([]any)) != 0 {
		t.Fatalf("events after purge = %s", stdout)
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

type telegramServer struct {
	mu       sync.Mutex
	messages []string
	server   *httptest.Server
}

func newTelegramServer(t *testing.T) *telegramServer {
	t.Helper()
	f := &telegramServer{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = r.ParseForm()
		f.messages = append(f.messages, r.Form.Get("text"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *telegramServer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

func TestCLINotifyAndNoNotify(t *testing.T) {
	feed := newFeedServer(t, "A")
	tg := newTelegramServer(t)
	env := testEnv(t)
	env["RELEASE_WATCHER_TELEGRAM_BOT_TOKEN"] = "TEST_TOKEN"
	env["RELEASE_WATCHER_TELEGRAM_CHAT_ID"] = "chat-1"
	env["RELEASE_WATCHER_TELEGRAM_API_BASE"] = tg.server.URL

	code, stdout, _ := runCLI(t, env, "add", feed.server.URL+"/feed.xml", "--json")
	if code != 0 {
		t.Fatalf("add = %s", stdout)
	}
	watchID := dataMap(t, decodeEnvelope(t, stdout))["watch_id"].(string)
	feed.set("A", "B")

	code, stdout, _ = runCLI(t, env, "scan", "--id", watchID, "--no-notify", "--json")
	if code != 0 {
		t.Fatalf("scan no-notify = %s", stdout)
	}
	if tg.count() != 0 {
		t.Fatal("--no-notify still sent Telegram")
	}
	scanData := dataMap(t, decodeEnvelope(t, stdout))
	notifications := scanData["notifications"].(map[string]any)
	if notifications["sent"] != float64(0) || notifications["pending"] != float64(1) {
		t.Fatalf("notifications = %v", notifications)
	}

	code, stdout, _ = runCLI(t, env, "notify", "--json")
	if code != 0 {
		t.Fatalf("notify = %s", stdout)
	}
	counts := dataMap(t, decodeEnvelope(t, stdout))
	if counts["sent"] != float64(1) || counts["pending"] != float64(0) {
		t.Fatalf("counts = %v", counts)
	}
	if tg.count() != 1 {
		t.Fatalf("messages = %d", tg.count())
	}
	code, _, _ = runCLI(t, env, "notify", "--json")
	if code != 0 || tg.count() != 1 {
		t.Fatalf("duplicate notify: code=%d messages=%d", code, tg.count())
	}

	code, stdout, _ = runCLI(t, env, "scan", "--id", watchID, "--json")
	if code != 0 {
		t.Fatalf("scan = %s", stdout)
	}
	counts = dataMap(t, decodeEnvelope(t, stdout))["notifications"].(map[string]any)
	if counts["sent"] != float64(1) || tg.count() != 1 {
		t.Fatalf("automatic notifications = %v messages=%d", counts, tg.count())
	}
}

func TestCLINotifyRequiresConfiguration(t *testing.T) {
	env := testEnv(t)
	code, stdout, _ := runCLI(t, env, "notify", "--json")
	if code != cli.ExitError {
		t.Fatalf("exit = %d", code)
	}
	if decodeEnvelope(t, stdout)["error"].(map[string]any)["code"] != "config_error" {
		t.Fatalf("error = %s", stdout)
	}
}

func TestCLIBackupRestoreAndDoctor(t *testing.T) {
	feed := newFeedServer(t, "A")
	env := testEnv(t)
	code, stdout, _ := runCLI(t, env, "add", feed.server.URL+"/feed.xml", "--json")
	if code != 0 {
		t.Fatalf("add = %s", stdout)
	}
	watchID := dataMap(t, decodeEnvelope(t, stdout))["watch_id"].(string)

	backupPath := filepath.Join(t.TempDir(), "state.rwbackup")
	code, stdout, _ = runCLI(t, env, "backup", "--out", backupPath, "--json")
	if code != 0 {
		t.Fatalf("backup = %s", stdout)
	}
	if dataMap(t, decodeEnvelope(t, stdout))["schema_version"] != float64(2) {
		t.Fatalf("backup data = %s", stdout)
	}

	feed.set("A", "B")
	code, stdout, _ = runCLI(t, env, "scan", "--id", watchID, "--json")
	if code != 0 {
		t.Fatalf("scan = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "events", "--json")
	if len(dataMap(t, decodeEnvelope(t, stdout))["events"].([]any)) != 1 {
		t.Fatalf("events before restore = %s", stdout)
	}

	code, stdout, _ = runCLI(t, env, "restore", "--in", backupPath, "--json")
	if code != cli.ExitError {
		t.Fatalf("restore without force exit = %d", code)
	}
	if decodeEnvelope(t, stdout)["error"].(map[string]any)["code"] != "already_exists" {
		t.Fatalf("restore error = %s", stdout)
	}

	code, stdout, _ = runCLI(t, env, "restore", "--in", backupPath, "--force", "--json")
	if code != 0 {
		t.Fatalf("restore = %s", stdout)
	}
	code, stdout, _ = runCLI(t, env, "events", "--json")
	if len(dataMap(t, decodeEnvelope(t, stdout))["events"].([]any)) != 0 {
		t.Fatalf("events after restore = %s", stdout)
	}

	code, stdout, _ = runCLI(t, env, "doctor", "--json")
	if code != 0 {
		t.Fatalf("doctor = %s", stdout)
	}
	checks := dataMap(t, decodeEnvelope(t, stdout))["checks"].([]any)
	found := false
	for _, raw := range checks {
		check := raw.(map[string]any)
		if check["id"] == "schema" && check["status"] == "ok" {
			found = true
		}
	}
	if !found {
		t.Fatalf("doctor schema check missing: %s", stdout)
	}
}

func TestCLIDoctorReportsNewerSchemaWithoutMigrating(t *testing.T) {
	env := testEnv(t)
	dbPath := env["RELEASE_WATCHER_DB"]
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (999, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	code, stdout, _ := runCLI(t, env, "doctor", "--json")
	if code != cli.ExitError {
		t.Fatalf("doctor exit = %d", code)
	}
	checks := dataMap(t, decodeEnvelope(t, stdout))["checks"].([]any)
	found := false
	for _, raw := range checks {
		check := raw.(map[string]any)
		if check["id"] == "schema" && check["status"] == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("doctor did not report newer schema: %s", stdout)
	}
	// Doctor must not migrate or alter the injected version.
	version, err := func() (int, error) {
		db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
		if err != nil {
			return 0, err
		}
		defer db.Close()
		var v int
		err = db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v)
		return v, err
	}()
	if err != nil || version != 999 {
		t.Fatalf("schema changed by doctor: version=%d err=%v", version, err)
	}
}
