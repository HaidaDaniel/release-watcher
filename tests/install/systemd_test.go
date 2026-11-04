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

func TestInstallScriptRendersCustomPaths(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	prefix := filepath.Join(tmp, "prefix")
	configHome := filepath.Join(tmp, "config")
	stateDir := filepath.Join(tmp, "state")
	fakeBin := filepath.Join(tmp, "fake-release-watcher")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"), fakeBin)
	cmd.Env = append(os.Environ(),
		"HOME="+tmp,
		"PREFIX="+prefix,
		"XDG_CONFIG_HOME="+configHome,
		"RELEASE_WATCHER_STATE_DIR="+stateDir,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install script: %v\n%s", err, output)
	}
	service, err := os.ReadFile(filepath.Join(configHome, "systemd", "user", "release-watcher.service"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ExecStart=" + filepath.Join(prefix, "bin", "release-watcher") + " scan --due --json",
		"EnvironmentFile=-" + filepath.Join(configHome, "release-watcher", "env"),
		"Environment=RELEASE_WATCHER_STATE_DIR=" + stateDir,
	} {
		if !strings.Contains(string(service), want) {
			t.Fatalf("rendered service missing %q:\n%s", want, service)
		}
	}
	if _, err := os.Stat(filepath.Join(prefix, "bin", "release-watcher")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(configHome, "release-watcher", "env")); err != nil {
		t.Fatal(err)
	}
}
