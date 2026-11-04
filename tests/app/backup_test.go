package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HaidaDaniel/release-watcher/internal/app"
	"github.com/HaidaDaniel/release-watcher/internal/lock"
)

func TestBackupRestoreRoundTrip(t *testing.T) {
	n := newNaroFixture(t, []string{"One", "Two", "Three"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	added := addNaro(t, env, nil)

	backupPath := filepath.Join(t.TempDir(), "state.rwbackup")
	backup, appErr := env.svc.Backup(ctx, backupPath, false)
	if appErr != nil {
		t.Fatal(appErr)
	}
	if backup.SchemaVersion != 1 || backup.Bytes <= 0 {
		t.Fatalf("backup = %+v", backup)
	}
	zr, err := zip.OpenReader(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	_ = zr.Close()
	if !names["manifest.json"] || !names["release-watcher.db"] {
		t.Fatalf("archive names = %v", names)
	}

	n.setChapters([]string{"One", "Two", "Three", "Four"})
	scanID(t, env, added.WatchID)
	events, _ := env.svc.Events(ctx, app.EventQuery{})
	if len(events.Events) != 1 {
		t.Fatalf("events before restore = %d", len(events.Events))
	}
	if err := env.store.Close(); err != nil {
		t.Fatal(err)
	}

	restored, appErr := app.Restore(ctx, env.cfg, backupPath, true)
	if appErr != nil {
		t.Fatal(appErr)
	}
	if restored.SchemaVersion != 1 || restored.Bytes <= 0 {
		t.Fatalf("restore = %+v", restored)
	}

	reopened := openServiceForConfig(t, env.cfg, n)
	show, showErr := reopened.svc.Show(ctx, added.WatchID)
	if showErr != nil {
		t.Fatal(showErr)
	}
	if len(show.Releases) != 3 {
		t.Fatalf("releases after restore = %d", len(show.Releases))
	}
	events, _ = reopened.svc.Events(ctx, app.EventQuery{})
	if len(events.Events) != 0 {
		t.Fatalf("events after restore = %d", len(events.Events))
	}
}

func TestRestoreRequiresForceAndRejectsBadArchives(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	_ = addNaro(t, env, nil)
	backupPath := filepath.Join(t.TempDir(), "state.rwbackup")
	if _, appErr := env.svc.Backup(ctx, backupPath, false); appErr != nil {
		t.Fatal(appErr)
	}
	if _, appErr := app.Restore(ctx, env.cfg, backupPath, false); appErr == nil || appErr.Code != "already_exists" {
		t.Fatalf("restore without force = %+v", appErr)
	}
	badPath := filepath.Join(t.TempDir(), "bad.rwbackup")
	if err := os.WriteFile(badPath, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, appErr := app.Restore(ctx, env.cfg, badPath, true); appErr == nil || appErr.Code != "invalid_backup" {
		t.Fatalf("bad restore = %+v", appErr)
	}
	if _, appErr := env.svc.Backup(ctx, backupPath, false); appErr == nil || appErr.Code != "already_exists" {
		t.Fatalf("backup without force = %+v", appErr)
	}
}

func TestRestoreRejectsNewerSchema(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "newer.db")
	db, err := sql.Open("sqlite", "file:"+source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (999, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "newer.rwbackup")
	writeBackupArchive(t, archive, data, 999)
	cfg := testConfigForRestore(t)
	if _, appErr := app.Restore(ctx, cfg, archive, true); appErr == nil || appErr.Code != "schema_incompatible" {
		t.Fatalf("newer schema restore = %+v", appErr)
	}
}

func writeBackupArchive(t *testing.T, path string, dbData []byte, schema int) {
	t.Helper()
	writeBackupArchiveWithManifest(t, path, map[string]any{
		"format": "release-watcher-backup", "format_version": 1,
		"schema_version": schema, "app_version": "test",
		"database_sha256": sha256Hex(dbData), "database_bytes": len(dbData),
	}, dbData)
}

func writeBackupArchiveWithManifest(t *testing.T, path string, manifest map[string]any, dbData []byte) {
	t.Helper()
	raw, _ := json.Marshal(manifest)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, _ := zw.Create("manifest.json")
	_, _ = mw.Write(raw)
	dw, _ := zw.Create("release-watcher.db")
	_, _ = dw.Write(dbData)
	_ = zw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readBackupDB(t *testing.T, path string) []byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "release-watcher.db" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatal("database entry not found")
	return nil
}

func TestRestoreRejectsMissingHashAndSize(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	_ = addNaro(t, env, nil)
	valid := filepath.Join(t.TempDir(), "valid.rwbackup")
	if _, appErr := env.svc.Backup(ctx, valid, false); appErr != nil {
		t.Fatal(appErr)
	}
	dbData := readBackupDB(t, valid)
	for name, manifest := range map[string]map[string]any{
		"missing-hash": {
			"format": "release-watcher-backup", "format_version": 1,
			"schema_version": 1, "database_bytes": len(dbData),
		},
		"missing-size": {
			"format": "release-watcher-backup", "format_version": 1,
			"schema_version": 1, "database_sha256": sha256Hex(dbData),
		},
		"zero-size": {
			"format": "release-watcher-backup", "format_version": 1,
			"schema_version": 1, "database_sha256": sha256Hex(dbData), "database_bytes": 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name+".rwbackup")
			writeBackupArchiveWithManifest(t, path, manifest, dbData)
			if _, appErr := app.Restore(ctx, env.cfg, path, true); appErr == nil || appErr.Code != "invalid_backup" {
				t.Fatalf("restore = %+v", appErr)
			}
		})
	}
}

func TestBackupPermissionsAndLock(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	_ = addNaro(t, env, nil)
	path := filepath.Join(t.TempDir(), "state.rwbackup")
	if _, appErr := env.svc.Backup(ctx, path, false); appErr != nil {
		t.Fatal(appErr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("backup permissions = %o", info.Mode().Perm())
	}
	release, err := lock.Acquire(ctx, env.cfg.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, appErr := env.svc.Backup(ctx, filepath.Join(t.TempDir(), "locked.rwbackup"), false); appErr == nil || appErr.Code != "locked" {
		t.Fatalf("backup with held lock = %+v", appErr)
	}
	if _, appErr := app.Restore(ctx, env.cfg, path, true); appErr == nil || appErr.Code != "locked" {
		t.Fatalf("restore with held lock = %+v", appErr)
	}
	release()
}

func TestRestoreRejectsMismatchedOrShortHashAndEmptySchema(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	_ = addNaro(t, env, nil)
	valid := filepath.Join(t.TempDir(), "valid.rwbackup")
	if _, appErr := env.svc.Backup(ctx, valid, false); appErr != nil {
		t.Fatal(appErr)
	}
	dbData := readBackupDB(t, valid)
	cases := map[string]map[string]any{
		"short-hash": {
			"format": "release-watcher-backup", "format_version": 1,
			"schema_version": 1, "database_sha256": "abc", "database_bytes": len(dbData),
		},
		"mismatched-hash": {
			"format": "release-watcher-backup", "format_version": 1,
			"schema_version": 1, "database_sha256": strings.Repeat("0", 64), "database_bytes": len(dbData),
		},
	}
	for name, manifest := range cases {
		path := filepath.Join(t.TempDir(), name+".rwbackup")
		writeBackupArchiveWithManifest(t, path, manifest, dbData)
		if _, appErr := app.Restore(ctx, env.cfg, path, true); appErr == nil || appErr.Code != "invalid_backup" {
			t.Fatalf("%s restore = %+v", name, appErr)
		}
	}

	emptySchemaPath := filepath.Join(t.TempDir(), "empty.db")
	db, err := sql.Open("sqlite", "file:"+emptySchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	emptyData, err := os.ReadFile(emptySchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "empty-schema.rwbackup")
	writeBackupArchiveWithManifest(t, archive, map[string]any{
		"format": "release-watcher-backup", "format_version": 1,
		"schema_version": 0, "database_sha256": sha256Hex(emptyData), "database_bytes": len(emptyData),
	}, emptyData)
	if _, appErr := app.Restore(ctx, env.cfg, archive, true); appErr == nil || appErr.Code != "invalid_backup" {
		t.Fatalf("empty schema restore = %+v", appErr)
	}
}

func TestBackupRejectsLiveDatabaseTarget(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	_ = addNaro(t, env, nil)
	for _, target := range []string{env.cfg.DBPath, env.cfg.DBPath + "-wal", env.cfg.LockPath} {
		if _, appErr := env.svc.Backup(ctx, target, true); appErr == nil || appErr.Code != "invalid_argument" {
			t.Fatalf("backup target %s = %+v", target, appErr)
		}
	}
	if _, appErr := env.svc.List(ctx); appErr != nil {
		t.Fatalf("database damaged after rejected backup: %+v", appErr)
	}
}

func TestRestoreCleansSidecarsAndOldDatabase(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	_ = addNaro(t, env, nil)
	archive := filepath.Join(t.TempDir(), "state.rwbackup")
	if _, appErr := env.svc.Backup(ctx, archive, false); appErr != nil {
		t.Fatal(appErr)
	}
	if err := env.store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.WriteFile(env.cfg.DBPath+suffix, []byte("stale sidecar"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, appErr := app.Restore(ctx, env.cfg, archive, true); appErr != nil {
		t.Fatal(appErr)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal", ".pre-restore", ".pre-restore-wal", ".pre-restore-shm", ".pre-restore-journal"} {
		if _, err := os.Stat(env.cfg.DBPath + suffix); !os.IsNotExist(err) {
			t.Fatalf("stale file remains: %s", env.cfg.DBPath+suffix)
		}
	}
	reopened := openServiceForConfig(t, env.cfg, n)
	if _, appErr := reopened.svc.List(ctx); appErr != nil {
		t.Fatalf("database unreadable after restore: %+v", appErr)
	}
}

func TestBackupRejectsSymlinkedAndHardlinkedLiveTargets(t *testing.T) {
	n := newNaroFixture(t, []string{"One"})
	env := newTestEnv(t, n)
	ctx := context.Background()
	_ = addNaro(t, env, nil)

	for _, suffix := range []string{"-shm", "-journal"} {
		if _, appErr := env.svc.Backup(ctx, env.cfg.DBPath+suffix, true); appErr == nil || appErr.Code != "invalid_argument" {
			t.Fatalf("sidecar target %s = %+v", suffix, appErr)
		}
	}
	linkDir := filepath.Join(t.TempDir(), "linked-state")
	if err := os.Symlink(filepath.Dir(env.cfg.DBPath), linkDir); err != nil {
		t.Fatal(err)
	}
	symlinkTarget := filepath.Join(linkDir, filepath.Base(env.cfg.DBPath))
	if _, appErr := env.svc.Backup(ctx, symlinkTarget, true); appErr == nil || appErr.Code != "invalid_argument" {
		t.Fatalf("symlinked parent target = %+v", appErr)
	}
	hardlink := filepath.Join(t.TempDir(), "hardlink.db")
	if err := os.Link(env.cfg.DBPath, hardlink); err != nil {
		t.Fatal(err)
	}
	if _, appErr := env.svc.Backup(ctx, hardlink, true); appErr == nil || appErr.Code != "invalid_argument" {
		t.Fatalf("hardlink target = %+v", appErr)
	}
	if _, appErr := env.svc.List(ctx); appErr != nil {
		t.Fatalf("database damaged: %+v", appErr)
	}
}
