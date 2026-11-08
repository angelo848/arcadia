// Package logging builds the structured slog loggers shared by the agent and the desktop UI.
// Logs are written as JSON to a size-rotated file.
package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Rotation policy for log files.
const (
	// MaxSizeMB is the size in megabytes at which the current log file is rotated.
	MaxSizeMB = 10
	// MaxBackups is the number of rotated log files kept on disk.
	MaxBackups = 3
)

// Options configures a logger created by New.
type Options struct {
	Dir   string // Paths.Logs
	File  string // "agent.log" | "desktop.log"
	Level slog.Level
}

// New returns a JSON slog logger writing to Dir/File with size-based rotation,
// plus a Closer that closes the underlying log file. Dir is created if missing.
func New(opts Options) (*slog.Logger, io.Closer, error) {
	if opts.Dir == "" {
		return nil, nil, errors.New("logging: new: empty log directory")
	}
	if opts.File == "" {
		return nil, nil, errors.New("logging: new: empty log file name")
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("logging: create log dir %q: %w", opts.Dir, err)
	}
	// Note: lumberjack v2.2.1 starts a cleanup goroutine (millRun) on first write
	// that Close does not stop. It is third-party, one per logger, and lives until
	// the process exits; we accept it rather than work around the library.
	rot := newRotator(opts)
	h := slog.NewJSONHandler(rot, &slog.HandlerOptions{Level: opts.Level})
	return slog.New(h), rot, nil
}

// newRotator builds the rotating file writer for opts.
func newRotator(opts Options) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   filepath.Join(opts.Dir, opts.File),
		MaxSize:    MaxSizeMB,
		MaxBackups: MaxBackups,
	}
}

// ParseLevel maps a lowercase level name to an slog.Level.
// The empty string maps to Info; accepted values are debug, info, warn and error.
func ParseLevel(s string) (slog.Level, error) {
	switch s {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("logging: invalid log level %q (accepted: debug, info, warn, error)", s)
	}
}
