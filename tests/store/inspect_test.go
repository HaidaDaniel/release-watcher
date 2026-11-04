package store_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/store"
)

func TestInspectSchemaAndVacuumInto(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "live.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Queries().SetSetting(ctx, "k", "v", time.Now()); err != nil {
		t.Fatal(err)
	}
	liveInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if liveInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("database permissions = %o", liveInfo.Mode().Perm())
	}
	version, err := store.InspectSchema(ctx, path)
	if err != nil || version != 2 {
		t.Fatalf("inspect live = %d err=%v", version, err)
	}
	known, err := store.KnownSchemaVersion()
	if err != nil || known != 2 {
		t.Fatalf("known = %d err=%v", known, err)
	}
	copyPath := filepath.Join(dir, "copy.db")
	if err := st.VacuumInto(ctx, copyPath); err != nil {
		t.Fatal(err)
	}
	version, err = store.InspectSchema(ctx, copyPath)
	if err != nil || version != 2 {
		t.Fatalf("inspect copy = %d err=%v", version, err)
	}
	info, err := os.Stat(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("copy permissions = %o", info.Mode().Perm())
	}
	if err := st.VacuumInto(ctx, copyPath); err == nil {
		t.Fatal("existing backup target accepted")
	}
	if _, err := store.InspectSchema(ctx, filepath.Join(dir, "missing.db")); err == nil {
		t.Fatal("missing database inspected without error")
	}
}

func TestInspectSchemaDoesNotCreateSidecars(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "closed.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
	version, err := store.InspectSchema(ctx, path)
	if err != nil || version != 2 {
		t.Fatalf("inspect = %d err=%v", version, err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("inspect created sidecar %s", suffix)
		}
	}
}

func TestInspectSchemaWithLiveWALDoesNotModifySourceSidecars(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "live.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Queries().SetSetting(ctx, "k", "v", time.Now()); err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if data, err := os.ReadFile(candidate); err == nil {
			before[candidate] = data
		}
	}
	version, err := store.InspectSchema(ctx, path)
	if err != nil || version != 2 {
		t.Fatalf("inspect live = %d err=%v", version, err)
	}
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if data, err := os.ReadFile(candidate); err == nil {
			if old, ok := before[candidate]; !ok || !bytes.Equal(old, data) {
				t.Fatalf("source file changed by inspect: %s", candidate)
			}
		} else if _, ok := before[candidate]; ok {
			t.Fatalf("source file removed by inspect: %s", candidate)
		}
	}
}
