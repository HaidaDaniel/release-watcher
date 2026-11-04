// Package lock provides a non-blocking process lock for scans.
package lock

import "errors"

// ErrLocked means another process holds the lock.
var ErrLocked = errors.New("lock: another process holds the scan lock")
