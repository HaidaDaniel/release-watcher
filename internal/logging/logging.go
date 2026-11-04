package logging

import (
	"io"
	"log/slog"
	"strings"
)

// New returns a text or JSON logger. The logger never receives secrets from
// callers; configuration values are not logged by this package.
func New(format, level string, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	if strings.EqualFold(format, "json") {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
