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
	for _, want := range []string{"OnCalendar=*-*-* 04:15:00", "Persistent=true", "RandomizedDelaySec=15m", "WantedBy=timers.target"} {
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
	for _, want := range []string{"release-watcher.service", "release-watcher.timer", "release-watcher doctor", "loginctl enable-linger"} {
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
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("systemd-analyze verify: %v\n%s", err, output)
		}
		if strings.Contains(string(output), "ignoring:") {
			t.Fatalf("systemd-analyze warnings:\n%s", output)
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
		"ExecStart=\"" + filepath.Join(strings.ReplaceAll(prefix, "%", "%%"), "bin", "release-watcher") + "\" scan --due --json",
		"EnvironmentFile=-" + filepath.Join(configHome, "release-watcher", "env"),
		"Environment=\"RELEASE_WATCHER_STATE_DIR=" + stateDir + "\"",
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

func TestInstallScriptHonorsXDGDataAndSpecialPaths(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	prefix := filepath.Join(tmp, "pre fix&|%x")
	configHome := filepath.Join(tmp, "config")
	dataHome := filepath.Join(tmp, "xdg data")
	fakeBin := filepath.Join(tmp, "fake release-watcher")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"), fakeBin)
	cmd.Env = append(os.Environ(),
		"HOME="+tmp,
		"PREFIX="+prefix,
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+dataHome,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install script: %v\n%s", err, output)
	}
	service, err := os.ReadFile(filepath.Join(configHome, "systemd", "user", "release-watcher.service"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(service)
	for _, want := range []string{
		"ExecStart=\"" + filepath.Join(strings.ReplaceAll(prefix, "%", "%%"), "bin", "release-watcher") + "\" scan --due --json",
		"Environment=\"RELEASE_WATCHER_STATE_DIR=" + filepath.Join(strings.ReplaceAll(dataHome, "%", "%%"), "release-watcher") + "\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered service missing %q:\n%s", want, text)
		}
	}
}

func TestSystemLevelUnitsAndPackagingScripts(t *testing.T) {
	root := repoRoot(t)
	servicePath := filepath.Join(root, "deploy", "systemd", "system", "release-watcher.service")
	timerPath := filepath.Join(root, "deploy", "systemd", "system", "release-watcher.timer")
	service, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatal(err)
	}
	timer, err := os.ReadFile(timerPath)
	if err != nil {
		t.Fatal(err)
	}
	serviceText := string(service)
	for _, want := range []string{
		"User=release-watcher",
		"Group=release-watcher",
		"ExecStart=/opt/release-watcher/current/release-watcher scan --due --json",
		"StateDirectory=release-watcher",
		"MemoryAccounting=yes",
		"MemoryHigh=256M",
		"MemoryMax=512M",
		"MemorySwapMax=128M",
		"CPUAccounting=yes",
		"CPUQuota=40%",
		"CPUWeight=50",
		"TasksAccounting=yes",
		"TasksMax=64",
		"IOAccounting=yes",
		"IOWeight=50",
		"Nice=10",
		"IOSchedulingClass=idle",
		"TimeoutStartSec=15min",
		"LimitNOFILE=4096",
		"NoNewPrivileges=yes",
		"PrivateTmp=yes",
		"ProtectSystem=strict",
		"ProtectHome=yes",
		"RestrictSUIDSGID=yes",
		"RestrictRealtime=yes",
	} {
		if !strings.Contains(serviceText, want) {
			t.Fatalf("system service missing %q", want)
		}
	}
	for _, want := range []string{
		"OnCalendar=*-*-* 04:15:00",
		"Persistent=true",
		"RandomizedDelaySec=15m",
		"WantedBy=timers.target",
	} {
		if !strings.Contains(string(timer), want) {
			t.Fatalf("system timer missing %q", want)
		}
	}
	for _, script := range []string{"scripts/package-release.sh", "scripts/install-system.sh", "scripts/rollback-system.sh"} {
		cmd := exec.Command("bash", "-n", filepath.Join(root, script))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v\n%s", script, err, output)
		}
	}

	if _, err := exec.LookPath("systemd-analyze"); err == nil {
		tmp := t.TempDir()
		fakeBin := filepath.Join(tmp, "release-watcher")
		if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		serviceCopy := strings.ReplaceAll(serviceText, "/opt/release-watcher/current/release-watcher", fakeBin)
		serviceCopy = strings.ReplaceAll(serviceCopy, "User=release-watcher", "User=root")
		serviceCopy = strings.ReplaceAll(serviceCopy, "Group=release-watcher", "Group=root")
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
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("systemd-analyze verify: %v\n%s", err, output)
		}
		if strings.Contains(string(output), "ignoring:") {
			t.Fatalf("systemd-analyze warnings:\n%s", output)
		}
	}
}
