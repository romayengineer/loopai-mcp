package proto

import (
	"io"
	"log/slog"
	"os"
)

// ParseLogLevel converts a log level string to slog.Level.
// Accepted values: "debug", "info", "warn", "error" (case-sensitive).
// Returns slog.LevelInfo for unrecognized values.
func ParseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// SetLogDefault configures the default slog logger with the given level,
// writing structured text output to os.Stderr.
func SetLogDefault(level slog.Level) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}

// SetLogDefaultWithWriter is like SetLogDefault but allows specifying the
// output writer (useful for tests).
func SetLogDefaultWithWriter(w io.Writer, level slog.Level) {
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
}
