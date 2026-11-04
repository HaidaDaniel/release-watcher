package foundation

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func runBuild(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(filepath.Join(repoRoot(t), "scripts", "build.sh"), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestBuildScriptRejectsInvalidRFC3339(t *testing.T) {
	for _, builtAt := range []string{
		"2026-11-04",
		"2026-11-04T12:00:00+24:00",
		"2026-11-04T12:00:00+23:60",
		"2026-11-04T12:00:00+00:60",
	} {
		t.Run(strings.NewReplacer(":", "_", "+", "plus").Replace(builtAt), func(t *testing.T) {
			_, stderr, err := runBuild(t,
				"--mode", "release",
				"--version", "0.1.0",
				"--commit", strings.Repeat("a", 40),
				"--built-at", builtAt,
				"--out-dir", t.TempDir(),
				"--allow-dirty",
			)
			if err == nil {
				t.Fatalf("expected %q to be rejected", builtAt)
			}
			if !strings.Contains(stderr, "RFC3339") {
				t.Fatalf("stderr = %q, want RFC3339 diagnostic", stderr)
			}
		})
	}
}

func TestBuildScriptRejectsDevVersionInReleaseMode(t *testing.T) {
	_, stderr, err := runBuild(t,
		"--mode", "release",
		"--version", "0.1.0-dev",
		"--commit", strings.Repeat("a", 40),
		"--built-at", "2026-11-04T12:00:00Z",
		"--out-dir", t.TempDir(),
		"--allow-dirty",
	)
	if err == nil {
		t.Fatal("expected dev version to be rejected in release mode")
	}
	if !strings.Contains(stderr, "non-dev") {
		t.Fatalf("stderr = %q, want non-dev diagnostic", stderr)
	}
}

func TestReleaseBuildIdentity(t *testing.T) {
	outDir := t.TempDir()
	commit := strings.Repeat("1", 40)
	_, stderr, err := runBuild(t,
		"--mode", "release",
		"--version", "0.1.0",
		"--commit", commit,
		"--built-at", "2026-11-04T12:00:00Z",
		"--out-dir", outDir,
		"--allow-dirty",
	)
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, stderr)
	}
	binary := filepath.Join(outDir, "release-watcher-linux-amd64")
	cmd := exec.Command(binary, "version", "--json")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("version failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	data := got["data"].(map[string]any)
	if data["version"] != "0.1.0" || data["commit"] != commit || data["build_mode"] != "release" || data["built_at"] != "2026-11-04T12:00:00Z" {
		t.Fatalf("unexpected identity: %v", data)
	}
}
