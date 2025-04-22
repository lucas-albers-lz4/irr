// Package log provides a simple leveled logger.
//
// Note: This logger writes directly to os.Stderr using fmt.Fprintf.
// It does not use the standard library's log.SetOutput redirection mechanism.
// To capture logs in tests, redirect os.Stderr directly.
package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Log level constants matching slog and environment variable values.
const (
	levelDebugStr = "DEBUG"
	levelInfoStr  = "INFO"
	levelWarnStr  = "WARN"
	levelErrorStr = "ERROR"
)

var (
	logger       *slog.Logger
	currentLevel           = slog.LevelInfo
	outputWriter io.Writer = os.Stderr
	// ErrInvalidLogLevel indicates an invalid log level string was provided.
	ErrInvalidLogLevel = fmt.Errorf("invalid log level")
)

// init initializes the logging package based on environment variables.
func init() {
	// Determine initial log level from environment
	initialLevel := slog.LevelInfo // Default
	if levelStr := os.Getenv("LOG_LEVEL"); levelStr != "" {
		switch strings.ToUpper(levelStr) {
		case levelDebugStr:
			initialLevel = slog.LevelDebug
		case levelInfoStr:
			initialLevel = slog.LevelInfo
		case levelWarnStr, "WARNING": // Accept both forms
			initialLevel = slog.LevelWarn
		case levelErrorStr:
			initialLevel = slog.LevelError
		}
	}
	// IRR_DEBUG=1 also enables debug unless LOG_LEVEL already set it higher
	if os.Getenv("IRR_DEBUG") == "1" && initialLevel > slog.LevelDebug {
		initialLevel = slog.LevelDebug
	}
	currentLevel = initialLevel // Set the global level determined by environment

	// Configure the logger with initial settings
	configureLogger()
}

// configureLogger sets up the logger using the current global state
// (currentLevel and outputWriter). It does not read environment variables itself.
func configureLogger() {
	// Determine log format
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))
	var handler slog.Handler
	// Default to JSON unless LOG_FORMAT is explicitly "text"
	if format == "text" {
		handler = slog.NewTextHandler(outputWriter, &slog.HandlerOptions{Level: currentLevel})
	} else {
		// Default to JSON if LOG_FORMAT is empty/unset or explicitly "json" (or anything else)
		handler = slog.NewJSONHandler(outputWriter, &slog.HandlerOptions{Level: currentLevel})
	}
	logger = slog.New(handler)
}

// SetOutput changes the output destination for the logger.
// It returns a function that can be called to restore the original output writer.
// This is primarily intended for testing.
func SetOutput(w io.Writer) (restore func()) {
	originalWriter := outputWriter
	outputWriter = w
	configureLogger() // Re-configure logger with the new writer
	return func() {
		outputWriter = originalWriter
		configureLogger() // Restore original writer and re-configure
	}
}

// Debug logs a debug message with optional key-value pairs
func Debug(msg string, args ...any) {
	logger.Debug(msg, args...)
}

// Info logs an info message with optional key-value pairs
func Info(msg string, args ...any) {
	logger.Info(msg, args...)
}

// Warn logs a warning message with optional key-value pairs
func Warn(msg string, args ...any) {
	logger.Warn(msg, args...)
}

// Error logs an error message with optional key-value pairs
func Error(msg string, args ...any) {
	logger.Error(msg, args...)
}

// Logger returns the underlying slog.Logger
func Logger() *slog.Logger {
	return logger
}

// SetLevel allows changing the log level at runtime
func SetLevel(level interface{}) {
	switch v := level.(type) {
	case slog.Level:
		currentLevel = v
	case Level:
		currentLevel = slog.Level(v)
	default:
		panic(fmt.Sprintf("SetLevel: unsupported level type %T", level))
	}
	configureLogger() // Re-configure logger with the new level
}

// CurrentLevel returns the current slog.Level
func CurrentLevel() slog.Level {
	return currentLevel
}

// Level is a log level type compatible with slog.Level, for test and testutil compatibility
// and to provide a stable API for the rest of the codebase.
type Level int8

// Log level definitions.
const (
	// LevelDebug defines the debug log level.
	LevelDebug Level = Level(slog.LevelDebug)
	// LevelInfo defines the info log level.
	LevelInfo Level = Level(slog.LevelInfo)
	// LevelWarn defines the warn log level.
	LevelWarn Level = Level(slog.LevelWarn)
	// LevelError defines the error log level.
	LevelError Level = Level(slog.LevelError)
)

// String returns the string representation of the log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a string and returns the corresponding Level.
func ParseLevel(levelStr string) (Level, error) {
	switch strings.ToUpper(levelStr) {
	case levelDebugStr:
		return LevelDebug, nil
	case levelInfoStr:
		return LevelInfo, nil
	case levelWarnStr, "WARNING": // Accept both forms
		return LevelWarn, nil
	case levelErrorStr:
		return LevelError, nil
	default:
		// Return default level (Info) on parse error, fixing gosec G115 warning.
		return LevelInfo, fmt.Errorf("%w: %s", ErrInvalidLogLevel, levelStr)
	}
}
