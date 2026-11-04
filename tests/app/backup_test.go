package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/HaidaDaniel/release-watcher/internal/app"
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
	manifest := map[string]any{
		"format": "release-watcher-backup", "format_version": 1,
		"schema_version": schema, "app_version": "test",
		"database_sha256": sha256Hex(dbData), "database_bytes": len(dbData),
	}
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
