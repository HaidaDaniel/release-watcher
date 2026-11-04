// Package notifier contains optional event delivery channels.
package notifier

import (
	"context"
	"time"
)

// Notifier sends one already-formatted message.
type Notifier interface {
	Name() string
	Send(ctx context.Context, text string) error
}

// Error is a typed delivery error. Error() deliberately excludes causes and
// request URLs so a bot token can never appear in a log line.
type Error struct {
	Code       string
	Message    string
	Retryable  bool
	RetryAfter time.Duration
	StatusCode int
	Cause      error
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func (e *Error) Unwrap() error { return e.Cause }
