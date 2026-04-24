// Package logger provides structured logging for the application.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config for logger.
type Config struct {
	Enabled    bool
	Level      string
	JSONFormat bool
}

// New creates a slog.Logger based on config.
// When Enabled is false, logs are discarded.
func New(cfg Config) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	w := io.Discard
	if cfg.Enabled {
		w = os.Stdout
	}

	var handler slog.Handler
	if cfg.JSONFormat {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	return slog.New(handler)
}
