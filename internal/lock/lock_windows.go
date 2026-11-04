//go:build windows

package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Windows is not a supported deployment target for the MVP. This fallback uses
// exclusive file creation and removes the file on release.
func Acquire(ctx context.Context, path string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock: create lock directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if os.IsExist(err) {
		return nil, ErrLocked
	}
	if err != nil {
		return nil, fmt.Errorf("lock: acquire: %w", err)
	}
	return func() {
		_ = f.Close()
		_ = os.Remove(path)
	}, nil
}
