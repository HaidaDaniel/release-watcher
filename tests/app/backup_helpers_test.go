package app_test

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/config"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func testConfigForRestore(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	return config.Config{
		StateDir: dir, DBPath: filepath.Join(dir, "release-watcher.db"),
		LockPath: filepath.Join(dir, "scan.lock"), DefaultInterval: time.Hour,
		MaxDeliveryTries: 3,
	}
}
