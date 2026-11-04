package app

import (
	"errors"
	"fmt"

	"github.com/HaidaDaniel/release-watcher/internal/lock"
	"github.com/HaidaDaniel/release-watcher/internal/source"
	"github.com/HaidaDaniel/release-watcher/internal/store"
)

// Error is an application error with a stable machine code.
type Error struct {
	Code    string
	Message string
	Details map[string]any
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func newError(code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func mapError(err error) *Error {
	if err == nil {
		return nil
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	var sourceErr *source.Error
	if errors.As(err, &sourceErr) {
		return &Error{Code: sourceErr.Code, Message: sourceErr.Message, Cause: sourceErr.Cause}
	}
	if errors.Is(err, lock.ErrLocked) {
		return &Error{Code: "locked", Message: "another release-watcher operation is already running", Cause: err}
	}
	if errors.Is(err, store.ErrNotFound) {
		return &Error{Code: "not_found", Message: "requested record was not found", Cause: err}
	}
	return &Error{Code: "internal_error", Message: "internal error", Cause: err}
}
