//go:build !windows

package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Acquire takes an exclusive, non-blocking file lock. The returned function
// releases it. A held lock returns ErrLocked instead of waiting.
func Acquire(ctx context.Context, path string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock: create lock directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: open lock file: %w", err)
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
		_ = f.Close()
		return nil, ErrLocked
	}
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock: acquire: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
