package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/HaidaDaniel/release-watcher/migrations"
)

// InspectSchema opens the database read-only and returns its migration version.
// It never creates, migrates or writes to the database.
func InspectSchema(ctx context.Context, path string) (int, error) {
	if path == "" {
		return 0, errors.New("store: empty database path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	if _, err := os.Stat(abs); err != nil {
		return 0, err
	}
	if _, err := os.Stat(abs + "-wal"); err == nil {
		tmpDir, err := os.MkdirTemp("", "release-watcher-inspect-*")
		if err != nil {
			return 0, fmt.Errorf("store: create inspection directory: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		tmpDB := filepath.Join(tmpDir, "inspect.db")
		if err := copyFile(abs, tmpDB); err != nil {
			return 0, fmt.Errorf("store: copy database for inspection: %w", err)
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if _, statErr := os.Stat(abs + suffix); statErr == nil {
				if err := copyFile(abs+suffix, tmpDB+suffix); err != nil {
					return 0, fmt.Errorf("store: copy database sidecar for inspection: %w", err)
				}
			}
		}
		return inspectPath(ctx, tmpDB)
	}
	return inspectPath(ctx, abs)
}

func inspectPath(ctx context.Context, abs string) (int, error) {
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	if _, err := os.Stat(abs + "-wal"); os.IsNotExist(err) {
		q.Set("immutable", "1")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return 0, fmt.Errorf("store: open read-only: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return 0, fmt.Errorf("store: ping read-only: %w", err)
	}
	var present int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`,
	).Scan(&present); err != nil {
		return 0, fmt.Errorf("store: inspect schema: %w", err)
	}
	if present == 0 {
		return 0, nil
	}
	var version int
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
	).Scan(&version); err != nil {
		return 0, fmt.Errorf("store: read schema version: %w", err)
	}
	return version, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := in.Seek(0, 0); err != nil {
		_ = out.Close()
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// KnownSchemaVersion returns the highest migration embedded in this binary.
func KnownSchemaVersion() (int, error) {
	all, err := migrations.All()
	if err != nil {
		return 0, err
	}
	version := 0
	for _, m := range all {
		if m.Version > version {
			version = m.Version
		}
	}
	return version, nil
}

// VacuumInto writes a consistent copy of the live database to a new file.
func (s *Store) VacuumInto(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("store: empty backup path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("store: create backup directory: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("store: backup target already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	escaped := strings.ReplaceAll(path, "'", "''")
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return fmt.Errorf("store: vacuum into backup: %w", err)
	}
	return os.Chmod(path, 0o600)
}
