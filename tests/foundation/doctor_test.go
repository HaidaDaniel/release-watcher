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

func TestDoctorScheduleCadenceParsing(t *testing.T) {
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

	cases := []struct {
		name    string
		unit    string
		dropIn  string
		want    string
		message string
	}{
		{name: "daily", unit: "OnCalendar=*-*-* 04:15:00", want: "warn", message: "24h"},
		{name: "daily alias", unit: "OnCalendar=daily", want: "warn", message: "24h"},
		{name: "six hour step", unit: "OnCalendar=*-*-* 00/6:00:00", want: "ok", message: "6h"},
		{name: "twelve hour step", unit: "OnCalendar=*-*-* 00/12:00:00", want: "warn", message: "12h"},
		{name: "hourly", unit: "OnCalendar=hourly", want: "ok", message: "1h"},
		{name: "five minute step", unit: "OnCalendar=*-*-* *:0/5:00", want: "ok", message: "5m"},
		{name: "weekly", unit: "OnCalendar=Mon *-*-* 04:15:00", want: "warn", message: "168h"},
		{name: "weekday range is conservative", unit: "OnCalendar=Mon..Fri *-*-* 04:15:00", want: "warn", message: "cannot determine"},
		{name: "weekday with step is conservative", unit: "OnCalendar=Mon *-*-* 00/6:00:00", want: "warn", message: "cannot determine"},
		{name: "monthly date is conservative", unit: "OnCalendar=*-*-01 04:15:00", want: "warn", message: "cannot determine"},
		{name: "drop-in six hour", unit: "OnCalendar=*-*-* 04:15:00", dropIn: "OnCalendar=*-*-* 00/6:00:00", want: "ok", message: "6h"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.RemoveAll(filepath.Join(timerDir, "release-watcher.timer.d")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(timerDir, "release-watcher.timer"), []byte("[Timer]\n"+tc.unit+"\nPersistent=true\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if tc.dropIn != "" {
				dropDir := filepath.Join(timerDir, "release-watcher.timer.d")
				if err := os.MkdirAll(dropDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dropDir, "10-cadence.conf"), []byte("[Timer]\n"+tc.dropIn+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			status, message := doctorSchedule(t, env)
			if status != tc.want {
				t.Fatalf("schedule status = %q, want %q (%s)", status, tc.want, message)
			}
			if !strings.Contains(message, tc.message) {
				t.Fatalf("schedule message = %q, want it to mention %q", message, tc.message)
			}
		})
	}
}

func doctorSchedule(t *testing.T, env map[string]string) (string, string) {
	t.Helper()
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
		if check.ID == "schedule" {
			return check.Status, check.Message
		}
	}
	t.Fatal("doctor did not report a schedule check")
	return "", ""
}
