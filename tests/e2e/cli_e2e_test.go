package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HaidaDaniel/release-watcher/internal/cli"
)

func runCLI(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	getenv := func(key string) string { return env[key] }
	code := cli.Run(context.Background(), args, &stdout, &stderr, getenv)
	return code, stdout.String(), stderr.String()
}

func decodeCLI(t *testing.T, raw string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, raw)
	}
	if got["schema_version"] != "1.0" || got["ok"] != true {
		t.Fatalf("unexpected envelope: %v", got)
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %v", got["data"])
	}
	return data
}

func TestCLIJSONNaroStages(t *testing.T) {
	n := newNaroServer(t, naroChapters(210))
	tg := newTelegramServer(t)
	state := t.TempDir()
	env := map[string]string{
		"RELEASE_WATCHER_STATE_DIR":           state,
		"RELEASE_WATCHER_DB":                  filepath.Join(state, "release-watcher.db"),
		"RELEASE_WATCHER_ALLOW_PRIVATE_HOSTS": "true",
		"RELEASE_WATCHER_RATE_INTERVAL":       "0s",
		"RELEASE_WATCHER_MAX_RETRIES":         "0",
		"RELEASE_WATCHER_LOG_LEVEL":           "error",
		"RELEASE_WATCHER_LANG":                "ru",
		"RELEASE_WATCHER_NARO_API_BASE":       n.server.URL + "/novelapi/api/",
		"RELEASE_WATCHER_NARO_CATALOG_BASE":   n.server.URL + "/",
		"RELEASE_WATCHER_TELEGRAM_API_BASE":   tg.server.URL,
		"RELEASE_WATCHER_TELEGRAM_BOT_TOKEN":  "TEST_TOKEN",
		"RELEASE_WATCHER_TELEGRAM_CHAT_ID":    "chat",
	}

	code, stdout, stderr := runCLI(t, env, "add", naroURL, "--json")
	if code != 0 {
		t.Fatalf("add exit %d stderr=%s", code, stderr)
	}
	addData := decodeCLI(t, stdout)
	if addData["created"] != true || addData["baseline_releases"] != float64(210) {
		t.Fatalf("add data = %v", addData)
	}
	watchID := addData["watch_id"].(string)

	code, stdout, _ = runCLI(t, env, "scan", "--id", watchID, "--json")
	if code != 0 {
		t.Fatalf("unchanged scan exit %d", code)
	}
	if decodeCLI(t, stdout)["events_created"] != float64(0) {
		t.Fatalf("unchanged scan = %s", stdout)
	}

	n.setChapters(naroChapters(212))
	code, stdout, _ = runCLI(t, env, "scan", "--id", watchID, "--json")
	if code != 0 {
		t.Fatalf("two-chapter scan exit %d", code)
	}
	scanData := decodeCLI(t, stdout)
	if scanData["events_created"] != float64(1) {
		t.Fatalf("two-chapter scan = %s", stdout)
	}
	notifications := scanData["notifications"].(map[string]any)
	if notifications["sent"] != float64(1) {
		t.Fatalf("notifications = %v", notifications)
	}
	messages, _ := tg.snapshot()
	if len(messages) != 1 || !strings.Contains(messages[0], "2 главы") || !strings.Contains(messages[0], "211–212") {
		t.Fatalf("messages = %q", messages)
	}

	code, stdout, _ = runCLI(t, env, "events", "--json")
	if code != 0 {
		t.Fatalf("events exit %d", code)
	}
	eventList := decodeCLI(t, stdout)["events"].([]any)
	if len(eventList) != 1 {
		t.Fatalf("events = %v", eventList)
	}
	event := eventList[0].(map[string]any)
	ids := event["release_ids"].([]any)
	if len(ids) != 2 || ids[0] != "N2166FE/211" || ids[1] != "N2166FE/212" {
		t.Fatalf("release ids = %v", ids)
	}

	code, stdout, _ = runCLI(t, env, "scan", "--id", watchID, "--json")
	if code != 0 || decodeCLI(t, stdout)["events_created"] != float64(0) {
		t.Fatalf("replay = %s", stdout)
	}

	code, stdout, _ = runCLI(t, env, "list", "--json")
	if code != 0 {
		t.Fatalf("list exit %d", code)
	}
	if watches := decodeCLI(t, stdout)["watches"].([]any); len(watches) != 1 {
		t.Fatalf("watches = %v", watches)
	}

	code, stdout, _ = runCLI(t, env, "show", watchID, "--json")
	if code != 0 {
		t.Fatalf("show exit %d", code)
	}
	if releases := decodeCLI(t, stdout)["releases"].([]any); len(releases) != 200 {
		t.Fatalf("releases = %d", len(releases))
	}

	code, stdout, _ = runCLI(t, env, "notify", "--json")
	if code != 0 {
		t.Fatalf("notify exit %d", code)
	}
	counts := decodeCLI(t, stdout)
	if counts["sent"] != float64(1) || counts["pending"] != float64(0) || counts["failed"] != float64(0) {
		t.Fatalf("notify counts = %v", counts)
	}
}
