package install_test

import (
	"os"
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

func TestSystemdUnitsAndInstallScript(t *testing.T) {
	root := repoRoot(t)
	service, err := os.ReadFile(filepath.Join(root, "deploy", "systemd", "release-watcher.service"))
	if err != nil {
		t.Fatal(err)
	}
	timer, err := os.ReadFile(filepath.Join(root, "deploy", "systemd", "release-watcher.timer"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"scan --due", "EnvironmentFile=-%h/.config/release-watcher/env", "TimeoutStartSec=30min"} {
		if !strings.Contains(string(service), want) {
			t.Fatalf("service missing %q", want)
		}
	}
	for _, want := range []string{"OnCalendar=hourly", "Persistent=true", "WantedBy=timers.target"} {
		if !strings.Contains(string(timer), want) {
			t.Fatalf("timer missing %q", want)
		}
	}

	for _, script := range []string{"scripts/install.sh", "scripts/build.sh"} {
		cmd := exec.Command("bash", "-n", filepath.Join(root, script))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v\n%s", script, err, output)
		}
	}
	installScript, err := os.ReadFile(filepath.Join(root, "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"release-watcher.service", "release-watcher.timer", "release-watcher doctor"} {
		if !strings.Contains(string(installScript), want) {
			t.Fatalf("install script missing %q", want)
		}
	}

	if _, err := exec.LookPath("systemd-analyze"); err == nil {
		tmp := t.TempDir()
		fakeBin := filepath.Join(tmp, "release-watcher")
		if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		serviceCopy := strings.ReplaceAll(string(service), "%h/.local/bin/release-watcher", fakeBin)
		if err := os.WriteFile(filepath.Join(tmp, "release-watcher.service"), []byte(serviceCopy), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "release-watcher.timer"), timer, 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("systemd-analyze", "verify",
			filepath.Join(tmp, "release-watcher.service"),
			filepath.Join(tmp, "release-watcher.timer"),
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("systemd-analyze verify: %v\n%s", err, output)
		}
	}
}
