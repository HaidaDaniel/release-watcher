package foundation

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HaidaDaniel/release-watcher/internal/cli"
)

func run(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	getenv := func(key string) string { return env[key] }
	code := cli.Run(context.Background(), args, &stdout, &stderr, getenv)
	return code, stdout.String(), stderr.String()
}

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
	return got
}

func TestVersionJSON(t *testing.T) {
	code, stdout, stderr := run(t, nil, "version", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr)
	}
	got := decode(t, stdout)
	if got["schema_version"] != "1.0" || got["ok"] != true {
		t.Fatalf("unexpected envelope: %v", got)
	}
	data := got["data"].(map[string]any)
	if data["name"] != "release-watcher" {
		t.Fatalf("name = %v", data["name"])
	}
	for _, field := range []string{"version", "commit", "built_at", "build_mode", "go_version"} {
		if _, ok := data[field]; !ok {
			t.Fatalf("missing field %q", field)
		}
	}
}

func TestHelpLanguages(t *testing.T) {
	code, stdout, _ := run(t, nil, "--lang", "ru", "help")
	if code != 0 || !strings.Contains(stdout, "Release Watcher") {
		t.Fatalf("ru help failed: code=%d out=%q", code, stdout)
	}
	code, stdout, _ = run(t, nil, "--lang", "en", "help")
	if code != 0 || !strings.Contains(stdout, "Usage:") {
		t.Fatalf("en help failed: code=%d out=%q", code, stdout)
	}
}

func TestUnknownCommandJSON(t *testing.T) {
	code, stdout, _ := run(t, nil, "nope", "--json")
	if code != cli.ExitUsage {
		t.Fatalf("exit = %d, want %d", code, cli.ExitUsage)
	}
	got := decode(t, stdout)
	if got["ok"] != false {
		t.Fatalf("ok = %v", got["ok"])
	}
	errShape := got["error"].(map[string]any)
	if errShape["code"] != "usage" {
		t.Fatalf("code = %v", errShape["code"])
	}
}

func TestDoctorJSON(t *testing.T) {
	state := t.TempDir()
	code, stdout, stderr := run(t, map[string]string{
		"RELEASE_WATCHER_STATE_DIR": state,
	}, "doctor", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr)
	}
	got := decode(t, stdout)
	data := got["data"].(map[string]any)
	if data["healthy"] != true {
		t.Fatalf("healthy = %v", data["healthy"])
	}
	checks := data["checks"].([]any)
	if len(checks) == 0 {
		t.Fatal("no checks")
	}
}

func TestArrayJSONIsNotHTMLEscaped(t *testing.T) {
	code, stdout, _ := run(t, nil, "version", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(stdout, "\\u003c") {
		t.Fatalf("unexpected HTML escaping: %s", stdout)
	}
}
