// Package log provides a simple leveled logger built on top of the standard library's slog package.
//
// By default, it configures a global logger writing JSON (or text if LOG_FORMAT=text)
// to os.Stderr. The log level is controlled globally via SetLevel() and can be
// initialized from environment variables or command-line flags (typically done in main).
//
// Use the SetOutput() function to redirect log output, primarily for testing purposes.
// It replaces the default os.Stderr writer and returns a function to restore it.
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
	logger *slog.Logger
	// currentLevel           = slog.LevelInfo // Replaced by globalLeveler
	globalLeveler           = &slog.LevelVar{} // Use LevelVar for dynamic level changes
	outputWriter  io.Writer = os.Stderr
	// ErrInvalidLogLevel indicates an invalid log level string was provided.
	ErrInvalidLogLevel = fmt.Errorf("invalid log level")
	// includeTimestampsForTest is a flag used by test helpers (like testutil.CaptureJSONLogs)
	// to temporarily force timestamp inclusion during log capture, overriding the default behavior.
	includeTimestampsForTest bool // Defaults to false
)

// init initializes the logging package with a default level.
// The final level is determined later in cmd/irr/root.go based on flags/env vars.
func init() {
	// Start with a sensible default (INFO).
	// The actual level will be set definitively by PersistentPreRunE in root.go
	// before any significant command logic runs.
	initialLevel := slog.LevelInfo
	globalLeveler.Set(initialLevel)

	// Configure the logger with initial settings (using the default level)
	configureLogger()
}

// configureLogger sets up the logger using the current global state
// (outputWriter and globalLeveler). It does not read environment variables itself.
func configureLogger() {
	// Determine log format
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))
	var handler slog.Handler

	// Prepare common options, using the dynamic LevelVar for the level
	opts := &slog.HandlerOptions{Level: globalLeveler}

	// Default to JSON unless LOG_FORMAT is explicitly "text"
	if format == "text" {
		// Text handler: Timestamps are included by default, no ReplaceAttr needed initially.
		// If specific text format changes are needed later, they would go here.
		handler = slog.NewTextHandler(outputWriter, opts)
	} else {
		// JSON handler: Conditionally remove the time attribute based on the test flag.
		opts.ReplaceAttr = func(_ []string, a slog.Attr) slog.Attr {
			// Remove the time attribute ONLY if the test flag is NOT set.
			if !includeTimestampsForTest && a.Key == slog.TimeKey {
				return slog.Attr{} // Remove the time attribute
			}
			return a // Keep other attributes (or time attribute if flag is true)
		}
		handler = slog.NewJSONHandler(outputWriter, opts)
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

// SetLevel allows changing the log level at runtime using the global LevelVar.
func SetLevel(level interface{}) {
	var targetSlogLevel slog.Level
	switch v := level.(type) {
	case slog.Level:
		targetSlogLevel = v
	case Level:
		targetSlogLevel = slog.Level(v)
	default:
		panic(fmt.Sprintf("SetLevel: unsupported level type %T", level))
	}
	// Update the dynamic level
	globalLeveler.Set(targetSlogLevel)
	// DO NOT call configureLogger() here anymore
}

// CurrentLevel returns the current slog.Level from the LevelVar
func CurrentLevel() slog.Level {
	return globalLeveler.Level()
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

// SetTestModeWithTimestamps controls whether timestamps are included in JSON logs.
// This is intended ONLY for use by test helpers (e.g., testutil.CaptureJSONLogs)
// to ensure timestamps are present for assertions, even if the default is to omit them.
func SetTestModeWithTimestamps(enabled bool) {
	includeTimestampsForTest = enabled
	// Reconfigure the logger immediately to apply the change
	// This is important because the test helper might call this *before*
	// calling SetOutput or SetLevel, which also reconfigure.
	configureLogger()
}
