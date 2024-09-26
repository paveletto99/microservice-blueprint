// Package logging sets up and configures logging.
package logging

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"log/slog"
)

// contextKey is a private string type to prevent collisions in the context map.
type contextKey string

// loggerKey points to the value in the context where the logger is stored.
const loggerKey = contextKey("logger")

var (
	// defaultLogger is the default logger. It is initialized once per package
	// include upon calling DefaultLogger.
	defaultLogger     *slog.Logger
	defaultLoggerOnce sync.Once
)

// custom levels
const (
	LevelEmergency = slog.Level(-1)
	LevelAlert     = slog.Level(1)
	LevelCritical  = slog.Level(2)
	LevelPanic     = slog.Level(4)
	LevelNotice    = slog.Level(5)
	LevelFatal     = slog.Level(16)
)

// NewLogger creates a new logger with the given configuration.
func NewLogger(level string, development bool) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: levelToSlogLevel(level),
	}

	if development {
		handler = slog.NewTextHandler(os.Stderr, opts)
		// Customize the handler for development if needed
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
		// Customize the handler for production if needed
	}

	logger := slog.New(handler)

	return logger
}

// NewLoggerFromEnv creates a new logger from the environment. It consumes
// LOG_LEVEL for determining the level and LOG_MODE for determining the output
// parameters.
func NewLoggerFromEnv() *slog.Logger {
	level := os.Getenv("LOG_LEVEL")
	development := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_MODE"))) == "development"
	return NewLogger(level, development)
}

// DefaultLogger returns the default logger for the package.
func DefaultLogger() *slog.Logger {
	defaultLoggerOnce.Do(func() {
		defaultLogger = NewLoggerFromEnv()
	})
	return defaultLogger
}

// WithLogger creates a new context with the provided logger attached.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext returns the logger stored in the context. If no such logger
// exists, a default logger is returned.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return DefaultLogger()
}

func FromNamedContext(ctx context.Context, key contextKey) *slog.Logger {
	if logger, ok := ctx.Value(key).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return DefaultLogger()
}

const (
	levelDebug     = "DEBUG"
	levelInfo      = "INFO"
	levelWarning   = "WARNING"
	levelError     = "ERROR"
	levelNotice    = "NOTICE"    // Normal but significant conditions.
	levelCritical  = "CRITICAL"  // Critical conditions.
	levelAlert     = "ALERT"     // Action must be taken immediately.
	levelEmergency = "EMERGENCY" // System is unusable. Immediate attention required.
	levelFatal     = "FATAL"     // Fatal-level messages causing termination.
)

// levelToSlogLevel converts the given string to the appropriate slog level value.
func levelToSlogLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case levelDebug:
		return slog.LevelDebug
	case levelInfo:
		return slog.LevelInfo
	case levelWarning:
		return slog.LevelWarn
	case levelError:
		return slog.LevelError
	case levelCritical:
		return LevelCritical // slog doesn't have Critical, custom mapping
	case levelAlert:
		return LevelAlert // slog doesn't have Alert, custom mapping
	case levelEmergency:
		return LevelEmergency // slog doesn't have Emergency, custom mapping
	case levelFatal:
		return LevelFatal // slog doesn't have Fatal, custom mapping
	case levelNotice:
		return LevelNotice // slog doesn't have Notice, custom mapping
	}
	return slog.LevelWarn
}

// Example of a time encoder if you need custom time formatting
func timeEncoder(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

// You can extend this package with helper functions if needed for structured logging.
// For example:

func Debug(ctx context.Context, msg string, args ...any) {
	logger := FromContext(ctx)
	logger.Debug(msg, args...)
}

func Info(ctx context.Context, msg string, args ...any) {
	logger := FromContext(ctx)
	logger.Info(msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	logger := FromContext(ctx)
	logger.Warn(msg, args...)
}

func Error(ctx context.Context, msg string, args ...any) {
	logger := FromContext(ctx)
	logger.Error(msg, args...)
}
