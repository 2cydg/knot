package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/term"
)

var (
	// DefaultLogger is the global logger instance.
	DefaultLogger *slog.Logger
)

func init() {
	// Default to a simple text handler writing to stderr before Setup is called.
	DefaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// Setup initializes the global logger.
// If logPath is empty, it logs to stdout.
func Setup(logPath string, level slog.Level, isJSON bool) error {
	var writer io.Writer = os.Stdout

	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}

		// If we're logging to a file and stdout is not a terminal, we assume 
		// stdout is already redirected (e.g., in background daemon mode) 
		// and we avoid double logging by not using MultiWriter.
		if term.IsTerminal(int(os.Stdout.Fd())) {
			writer = io.MultiWriter(os.Stdout, file)
		} else {
			writer = file
		}
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if isJSON {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	DefaultLogger = slog.New(handler)
	slog.SetDefault(DefaultLogger)

	return nil
}

// Info logs at LevelInfo.
func Info(msg string, args ...any) {
	DefaultLogger.Info(msg, args...)
}

// Error logs at LevelError.
func Error(msg string, args ...any) {
	DefaultLogger.Error(msg, args...)
}

// Debug logs at LevelDebug.
func Debug(msg string, args ...any) {
	DefaultLogger.Debug(msg, args...)
}

// Warn logs at LevelWarn.
func Warn(msg string, args ...any) {
	DefaultLogger.Warn(msg, args...)
}
