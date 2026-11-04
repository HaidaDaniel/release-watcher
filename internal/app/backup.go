package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/buildinfo"
	"github.com/HaidaDaniel/release-watcher/internal/config"
	"github.com/HaidaDaniel/release-watcher/internal/lock"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

const (
	backupFormat        = "release-watcher-backup"
	backupFormatVersion = 1
)

type BackupResult struct {
	Path          string    `json:"path"`
	SchemaVersion int       `json:"schema_version"`
	Bytes         int64     `json:"bytes"`
	CreatedAt     time.Time `json:"created_at"`
}

type RestoreResult struct {
	Path          string `json:"path"`
	SchemaVersion int    `json:"schema_version"`
	Bytes         int64  `json:"bytes"`
}

type backupManifest struct {
	Format         string    `json:"format"`
	FormatVersion  int       `json:"format_version"`
	SchemaVersion  int       `json:"schema_version"`
	AppVersion     string    `json:"app_version"`
	CreatedAt      time.Time `json:"created_at"`
	DatabaseSHA256 string    `json:"database_sha256"`
	DatabaseBytes  int64     `json:"database_bytes"`
}

func (s *Service) Backup(ctx context.Context, out string, force bool) (*BackupResult, *Error) {
	if out == "" {
		return nil, newError("invalid_argument", "backup output path is required", nil)
	}
	release, err := lock.Acquire(ctx, s.cfg.LockPath)
	if err != nil {
		return nil, mapError(err)
	}
	defer release()

	if dangerousBackupTarget(out, s.store.Path(), s.cfg.LockPath) {
		return nil, newError("invalid_argument", "backup output must not be the live database, a sidecar, or the scan lock", nil)
	}

	if _, statErr := os.Stat(out); statErr == nil && !force {
		return nil, newError("already_exists", "backup target already exists; pass --force to replace it", nil)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return nil, newError("storage_error", "cannot inspect backup target", statErr)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return nil, newError("storage_error", "cannot create backup directory", err)
	}
	tmpDir, err := os.MkdirTemp(s.cfg.StateDir, "backup-*")
	if err != nil {
		return nil, newError("storage_error", "cannot create backup staging directory", err)
	}
	defer os.RemoveAll(tmpDir)
	dbCopy := filepath.Join(tmpDir, "release-watcher.db")
	if err := s.store.VacuumInto(ctx, dbCopy); err != nil {
		return nil, newError("storage_error", "cannot create consistent database copy", err)
	}
	data, err := os.ReadFile(dbCopy)
	if err != nil {
		return nil, newError("storage_error", "cannot read database copy", err)
	}
	hash := sha256.Sum256(data)
	schema, err := s.store.SchemaVersion(ctx)
	if err != nil {
		return nil, newError("storage_error", "cannot read schema version", err)
	}
	now := s.now()
	manifest := backupManifest{
		Format:         backupFormat,
		FormatVersion:  backupFormatVersion,
		SchemaVersion:  schema,
		AppVersion:     buildinfo.Get().Version,
		CreatedAt:      now,
		DatabaseSHA256: hex.EncodeToString(hash[:]),
		DatabaseBytes:  int64(len(data)),
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, newError("storage_error", "cannot encode backup manifest", err)
	}

	tmpZip, err := os.CreateTemp(filepath.Dir(out), ".rwbackup-*")
	if err != nil {
		return nil, newError("storage_error", "cannot create backup archive", err)
	}
	tmpName := tmpZip.Name()
	defer os.Remove(tmpName)
	zw := zip.NewWriter(tmpZip)
	mw, err := zw.Create("manifest.json")
	if err == nil {
		_, err = mw.Write(manifestJSON)
	}
	if err == nil {
		var dw io.Writer
		dw, err = zw.Create("release-watcher.db")
		if err == nil {
			_, err = dw.Write(data)
		}
	}
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := tmpZip.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, newError("storage_error", "cannot write backup archive", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return nil, newError("storage_error", "cannot secure backup archive", err)
	}
	if err := os.Rename(tmpName, out); err != nil {
		return nil, newError("storage_error", "cannot move backup archive into place", err)
	}
	return &BackupResult{Path: out, SchemaVersion: schema, Bytes: int64(len(data)), CreatedAt: now}, nil
}

// Restore replaces the live database from a validated backup archive. It is a
// package function because the live Store must not be open during replacement.
func Restore(ctx context.Context, cfg config.Config, in string, force bool) (*RestoreResult, *Error) {
	if in == "" {
		return nil, newError("invalid_argument", "restore input path is required", nil)
	}
	release, err := lock.Acquire(ctx, cfg.LockPath)
	if err != nil {
		return nil, mapError(err)
	}
	defer release()

	if _, statErr := os.Stat(cfg.DBPath); statErr == nil && !force {
		return nil, newError("already_exists", "database already exists; pass --force to replace it", nil)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return nil, newError("storage_error", "cannot inspect database path", statErr)
	}

	zr, err := zip.OpenReader(in)
	if err != nil {
		return nil, newError("invalid_backup", "cannot open backup archive", err)
	}
	defer zr.Close()
	var (
		manifest     backupManifest
		haveManifest bool
		dbEntry      *zip.File
	)
	for _, f := range zr.File {
		switch f.Name {
		case "manifest.json":
			if f.UncompressedSize64 > 64<<10 {
				return nil, newError("invalid_backup", "backup manifest is too large", nil)
			}
			rc, openErr := f.Open()
			if openErr != nil {
				return nil, newError("invalid_backup", "cannot read backup manifest", openErr)
			}
			decErr := json.NewDecoder(io.LimitReader(rc, 64<<10)).Decode(&manifest)
			_ = rc.Close()
			if decErr != nil {
				return nil, newError("invalid_backup", "backup manifest is invalid", decErr)
			}
			haveManifest = true
		case "release-watcher.db":
			dbEntry = f
		}
	}
	if !haveManifest || manifest.Format != backupFormat || manifest.FormatVersion != backupFormatVersion {
		return nil, newError("invalid_backup", "backup format is not supported", nil)
	}
	if dbEntry == nil {
		return nil, newError("invalid_backup", "backup does not contain a database", nil)
	}
	if manifest.DatabaseBytes <= 0 {
		return nil, newError("invalid_backup", "backup manifest has no valid database size", nil)
	}
	manifestHash, hashErr := hex.DecodeString(manifest.DatabaseSHA256)
	if hashErr != nil || len(manifestHash) != sha256.Size {
		return nil, newError("invalid_backup", "backup manifest has no valid database hash", nil)
	}

	stateDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, newError("storage_error", "cannot create database directory", err)
	}
	tmpDir, err := os.MkdirTemp(stateDir, ".restore-*")
	if err != nil {
		return nil, newError("storage_error", "cannot create restore staging directory", err)
	}
	defer os.RemoveAll(tmpDir)
	tmpDB := filepath.Join(tmpDir, "release-watcher.db")
	rc, err := dbEntry.Open()
	if err != nil {
		return nil, newError("invalid_backup", "cannot open backup database", err)
	}
	out, err := os.OpenFile(tmpDB, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = rc.Close()
		return nil, newError("storage_error", "cannot create restore database", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, hash), rc)
	closeErr := out.Close()
	_ = rc.Close()
	if copyErr != nil || closeErr != nil {
		return nil, newError("invalid_backup", "cannot extract backup database", firstError(copyErr, closeErr))
	}
	if written != manifest.DatabaseBytes {
		return nil, newError("invalid_backup", "backup database size does not match the manifest", nil)
	}
	if hex.EncodeToString(hash.Sum(nil)) != manifest.DatabaseSHA256 {
		return nil, newError("invalid_backup", "backup database hash does not match the manifest", nil)
	}
	version, err := store.InspectSchema(ctx, tmpDB)
	if err != nil {
		return nil, newError("invalid_backup", "cannot inspect backup schema", err)
	}
	if version == 0 {
		return nil, newError("invalid_backup", "backup has no recognizable schema", nil)
	}
	known, err := store.KnownSchemaVersion()
	if err != nil {
		return nil, newError("storage_error", "cannot read known schema version", err)
	}
	if version > known {
		return nil, newError("schema_incompatible", fmt.Sprintf("backup schema %d is newer than this binary (%d)", version, known), nil)
	}

	oldPath := cfg.DBPath + ".pre-restore"
	hadExisting := false
	if _, statErr := os.Stat(cfg.DBPath); statErr == nil {
		hadExisting = true
		_ = os.Remove(oldPath)
		_ = os.Remove(oldPath + "-wal")
		_ = os.Remove(oldPath + "-shm")
		if err := os.Rename(cfg.DBPath, oldPath); err != nil {
			return nil, newError("storage_error", "cannot move the current database aside", err)
		}
		if err := moveSidecars(cfg.DBPath, oldPath); err != nil {
			_ = os.Rename(oldPath, cfg.DBPath)
			_ = moveSidecars(oldPath, cfg.DBPath)
			return nil, newError("storage_error", "cannot move database sidecars aside", err)
		}
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		_ = os.Remove(cfg.DBPath + suffix)
	}
	if err := os.Rename(tmpDB, cfg.DBPath); err != nil {
		if hadExisting {
			_ = os.Rename(oldPath, cfg.DBPath)
			_ = moveSidecars(oldPath, cfg.DBPath)
		}
		return nil, newError("storage_error", "cannot install restored database", err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		_ = os.Remove(cfg.DBPath + suffix)
	}
	if hadExisting {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			_ = os.Remove(oldPath + suffix)
		}
		_ = os.Remove(oldPath)
	}
	return &RestoreResult{Path: cfg.DBPath, SchemaVersion: version, Bytes: written}, nil
}

func firstError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

func moveSidecars(fromBase, toBase string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		src := fromBase + suffix
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.Rename(src, toBase+suffix); err != nil {
			return err
		}
	}
	return nil
}

func dangerousBackupTarget(out, dbPath, lockPath string) bool {
	targets := []string{
		dbPath, dbPath + "-wal", dbPath + "-shm", dbPath + "-journal", lockPath,
	}
	outResolved := resolvedPath(out)
	for _, target := range targets {
		if outResolved == resolvedPath(target) {
			return true
		}
		if sameFile(out, target) {
			return true
		}
	}
	return false
}

func resolvedPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	if parent, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(parent, filepath.Base(abs))
	}
	return filepath.Clean(abs)
}

func sameFile(a, b string) bool {
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	return aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo)
}
