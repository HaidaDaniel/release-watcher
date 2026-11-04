package foundation

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HaidaDaniel/release-watcher/internal/cli"
)

func doctorCheckStatus(t *testing.T, stateDir string) map[string]string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := func(key string) string {
		if key == "RELEASE_WATCHER_STATE_DIR" {
			return stateDir
		}
		return ""
	}
	code := cli.Run(context.Background(), []string{"doctor", "--json"}, &stdout, &stderr, env)
	if code != 0 {
		t.Fatalf("doctor exit %d: %s", code, stderr.String())
	}
	var got struct {
		Data struct {
			Checks []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	out := map[string]string{}
	for _, c := range got.Data.Checks {
		out[c.ID] = c.Status
	}
	return out
}

func TestDoctorPathsAreIndependentOfWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	state := t.TempDir()
	rootStatus := doctorCheckStatus(t, state)
	docs := filepath.Join(repoRoot(t), "docs")
	if err := os.Chdir(docs); err != nil {
		t.Fatal(err)
	}
	subStatus := doctorCheckStatus(t, state)

	for _, id := range []string{"hooks", "ci"} {
		if rootStatus[id] != subStatus[id] {
			t.Fatalf("%s status differs by cwd: root=%q subdir=%q", id, rootStatus[id], subStatus[id])
		}
	}
}

func TestDoctorWarnsWhenTimerIsSlowerThanWatchInterval(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Fixture</title><item><title>One</title><link>https://example.test/1</link><guid>1</guid><pubDate>Mon, 02 Nov 2026 10:00:00 GMT</pubDate></item></channel></rss>`))
	}))
	defer feed.Close()

	state := t.TempDir()
	configHome := t.TempDir()
	env := map[string]string{
		"RELEASE_WATCHER_STATE_DIR":           state,
		"RELEASE_WATCHER_ALLOW_PRIVATE_HOSTS": "true",
		"RELEASE_WATCHER_RATE_INTERVAL":       "0s",
		"RELEASE_WATCHER_MAX_RETRIES":         "0",
		"RELEASE_WATCHER_LOG_LEVEL":           "error",
		"XDG_CONFIG_HOME":                     configHome,
	}
	if code, stdout, stderr := run(t, env, "add", feed.URL+"/feed.xml", "--interval", "6h", "--json"); code != 0 {
		t.Fatalf("add exit %d: %s %s", code, stdout, stderr)
	}
	timerDir := filepath.Join(configHome, "systemd", "user")
	if err := os.MkdirAll(timerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(timerDir, "release-watcher.timer"), []byte("[Timer]\nOnCalendar=*-*-* 04:15:00\nPersistent=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run(t, env, "doctor", "--json")
	if code != 0 {
		t.Fatalf("doctor exit %d: %s", code, stderr)
	}
	var got struct {
		Data struct {
			Checks []struct {
				ID      string `json:"id"`
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	for _, check := range got.Data.Checks {
		if check.ID != "schedule" {
			continue
		}
		if check.Status != "warn" {
			t.Fatalf("schedule status = %q, want warn", check.Status)
		}
		if !strings.Contains(check.Message, "24h") || !strings.Contains(check.Message, "6h") {
			t.Fatalf("schedule message = %q", check.Message)
		}
		return
	}
	t.Fatal("doctor did not report a schedule check")
}
