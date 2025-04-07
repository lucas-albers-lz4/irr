// Package log provides a simple leveled logger.
package log

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
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
	return debug.Enabled || currentLevel <= LevelDebug
}

// SetLevel sets the logging level
func SetLevel(level Level) {
	currentLevel = level
	if level == LevelDebug {
		debug.Init(true)
	}
	fmt.Fprintf(os.Stderr, "Log level set to %s\n", level)
}

// Debugf logs a debug message if debug logging is enabled
func Debugf(format string, args ...interface{}) {
	if IsDebugEnabled() {
		if debug.Enabled {
			// Use the debug package's formatting if it's enabled
			log.Printf("[DEBUG] "+format, args...)
		} else {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
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
