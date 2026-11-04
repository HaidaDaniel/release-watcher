package install_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initSecretRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	script, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "check-secrets.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "check-secrets.sh"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func runSecretScan(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{filepath.Join(dir, "scripts", "check-secrets.sh")}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestSecretScannerDoesNotPrintMatchedSecret(t *testing.T) {
	dir := initSecretRepo(t)
	secret := "123456789:" + strings.Repeat("A", 35)
	if err := os.WriteFile(filepath.Join(dir, "telegram.env"), []byte("RELEASE_WATCHER_TELEGRAM_BOT_TOKEN="+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "telegram.env")

	out, err := runSecretScan(t, dir)
	if err == nil {
		t.Fatalf("scanner accepted a staged token:\n%s", out)
	}
	if !strings.Contains(out, "telegram.env") || !strings.Contains(out, "telegram-bot-token") {
		t.Fatalf("scanner did not report file/type:\n%s", out)
	}
	if strings.Contains(out, secret) || strings.Contains(out, strings.Repeat("A", 35)) {
		t.Fatalf("scanner leaked the matched secret:\n%s", out)
	}
}

func TestSecretScannerScansStagedAdditionsOnlyByDefault(t *testing.T) {
	dir := initSecretRepo(t)
	secret := "AKIA" + "IOSFODNN7EXAMPLE"
	if err := os.WriteFile(filepath.Join(dir, "credentials.txt"), []byte("key="+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runSecretScan(t, dir)
	if err != nil {
		t.Fatalf("unstaged secret blocked default staged scan:\n%s", out)
	}
	out, err = runSecretScan(t, dir, "--working")
	if err == nil {
		t.Fatalf("--working did not scan the working tree:\n%s", out)
	}
	if strings.Contains(out, secret) {
		t.Fatalf("scanner leaked the matched secret:\n%s", out)
	}
}

func TestSecretScannerAllowsSafeStagedFile(t *testing.T) {
	dir := initSecretRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("public documentation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")

	out, err := runSecretScan(t, dir)
	if err != nil {
		t.Fatalf("safe staged file failed: %v\n%s", err, out)
	}
}

func TestVerifyPushFailsClearlyWithoutPnpm(t *testing.T) {
	root := repoRoot(t)
	bin := t.TempDir()
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found")
	}
	if err := os.Symlink(goPath, filepath.Join(bin, "go")); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "verify-push.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+bin)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("verify-push succeeded without pnpm:\n%s", out)
	}
	if !strings.Contains(string(out), "pnpm is required") {
		t.Fatalf("missing clear pnpm error:\n%s", out)
	}
	if strings.Contains(string(out), "skipping APK lint") {
		t.Fatalf("verify-push still silently skips APK lint:\n%s", out)
	}
}
