//go:build windows

package store

import "context"

// Windows is not a supported deployment target for the MVP. Open retries
// migration errors instead of using a file lock.
func acquireFileLock(ctx context.Context, path string) (func(), error) {
	return func() {}, nil
}
