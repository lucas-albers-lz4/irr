// Package log provides a simple leveled logger.
package log

import (
	"fmt"
	"os"
	"strings"
)

// Level represents the logging level
type Level int

const (
	// LevelDebug enables debug level logging
	LevelDebug Level = iota
	// LevelWarn enables warning level logging
	LevelWarn
	// LevelError enables error level logging
	LevelError
)

var (
	// currentLevel is the current logging level
	currentLevel = LevelWarn
)

// init initializes the logging package
func init() {
	// Check for LOG_LEVEL environment variable
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		switch strings.ToUpper(level) {
		case "DEBUG":
			currentLevel = LevelDebug
		case "WARN":
			currentLevel = LevelWarn
		case "ERROR":
			currentLevel = LevelError
		}
	}
}

// IsDebugEnabled returns whether debug logging is enabled
func IsDebugEnabled() bool {
	// Only controlled by LOG_LEVEL
	return currentLevel <= LevelDebug
}

// SetLevel sets the logging level
func SetLevel(level Level) {
	currentLevel = level
	// Removed debug.Init(true) - decouple from debug package
	fmt.Fprintf(os.Stderr, "Log level set to %s\n", level)
}

// Debugf logs a debug message if debug logging is enabled
func Debugf(format string, args ...interface{}) {
	// Check if debug level is enabled via LOG_LEVEL
	if IsDebugEnabled() {
		// Always write to stderr for consistency
		fmt.Fprintf(os.Stderr, "[LOG_DEBUG] "+format+"\n", args...)
	}
}

// Warnf logs a warning message
func Warnf(format string, args ...interface{}) {
	if currentLevel <= LevelWarn {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// Errorf logs an error message
func Errorf(format string, args ...interface{}) {
	if currentLevel <= LevelError {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// String returns the string representation of the log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
