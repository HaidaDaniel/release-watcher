package install_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	mcp, err := os.ReadFile(filepath.Join(root, "deploy", "systemd", "system", "release-watcher-mcp.service"))
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
	mcpText := string(mcp)
	for _, want := range []string{
		"User=release-watcher",
		"ExecStart=/opt/release-watcher/current/release-watcher mcp serve",
		"EnvironmentFile=-/etc/release-watcher/mcp.env",
		"MemoryHigh=128M",
		"MemoryMax=256M",
		"MemorySwapMax=128M",
		"CPUQuota=25%",
		"TasksMax=64",
		"IOWeight=50",
		"Restart=on-failure",
		"ReadWritePaths=/var/lib/release-watcher",
	} {
		if !strings.Contains(mcpText, want) {
			t.Fatalf("MCP service missing %q", want)
		}
	}
	for _, script := range []string{"scripts/package-release.sh", "scripts/install-system.sh", "scripts/rollback-system.sh", "scripts/install-mcp-env.sh"} {
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
		mcpCopy := strings.ReplaceAll(mcpText, "/opt/release-watcher/current/release-watcher", fakeBin)
		mcpCopy = strings.ReplaceAll(mcpCopy, "User=release-watcher", "User=root")
		mcpCopy = strings.ReplaceAll(mcpCopy, "Group=release-watcher", "Group=root")
		if err := os.WriteFile(filepath.Join(tmp, "release-watcher-mcp.service"), []byte(mcpCopy), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "release-watcher.timer"), timer, 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("systemd-analyze", "verify",
			filepath.Join(tmp, "release-watcher.service"),
			filepath.Join(tmp, "release-watcher.timer"),
			filepath.Join(tmp, "release-watcher-mcp.service"),
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

func TestPruneBackupsKeepsVerifiedAndNewest(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	names := []string{"b1.rwbackup", "b2.rwbackup", "b3.rwbackup", "b4.rwbackup", "b5.rwbackup"}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, base.Add(time.Duration(i)*time.Hour), base.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	verified := filepath.Join(dir, "b1.rwbackup")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "prune-backups.sh"),
		"--dir", dir, "--keep", "2", "--verified", verified, "--apply")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prune-backups: %v\n%s", err, output)
	}
	for _, name := range []string{"b1.rwbackup", "b4.rwbackup", "b5.rwbackup"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should be kept: %v", name, err)
		}
	}
	for _, name := range []string{"b2.rwbackup", "b3.rwbackup"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should be removed", name)
		}
	}
}

func TestPruneBackupsCanonicalizesVerifiedPath(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(t.TempDir(), "backups with space")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 4; i++ {
		name := filepath.Join(dir, fmt.Sprintf("b%d.rwbackup", i))
		if err := os.WriteFile(name, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(name, base.Add(time.Duration(i)*time.Hour), base.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "verified-link.rwbackup")
	if err := os.Symlink("b1.rwbackup", link); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "prune-backups.sh"),
		"--dir", dir, "--keep", "2", "--verified", link, "--apply")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prune-backups: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(dir, "b1.rwbackup")); err != nil {
		t.Fatalf("verified target was deleted: %v", err)
	}
}

func TestSystemInstallerVerifiesBeforeSwitchAndHasRollback(t *testing.T) {
	root := repoRoot(t)
	script, err := os.ReadFile(filepath.Join(root, "scripts", "install-system.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	versionIdx := strings.Index(text, `"$staging/release-watcher" version --json`)
	verifyIdx := strings.Index(text, `systemd-analyze verify "$verify_units/release-watcher.service" "$verify_units/release-watcher.timer" "$verify_units/release-watcher-mcp.service"`)
	switchIdx := strings.LastIndex(text, `mv -Tf "$tmp_link" "$root/current"`)
	if versionIdx < 0 || verifyIdx < 0 || switchIdx < 0 {
		t.Fatalf("installer is missing a verification or switch step")
	}
	if versionIdx > switchIdx {
		t.Fatal("installer switches current before verifying the binary")
	}
	if verifyIdx > switchIdx {
		t.Fatal("installer switches current before verifying units")
	}
	for _, want := range []string{
		"trap cleanup EXIT",
		"restored previous current/units",
		"previous_service",
		"previous_timer",
		"previous_current",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("installer rollback missing %q", want)
		}
	}
}
