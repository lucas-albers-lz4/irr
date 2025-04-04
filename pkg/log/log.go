package log

import (
	"fmt"
	"os"
	"strings"

	"github.com/lalbers/helm-image-override/pkg/debug"
)

// LogLevel represents the logging level
type LogLevel int

const (
	// LevelDebug enables debug level logging
	LevelDebug LogLevel = iota
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
	return debug.IsEnabled || currentLevel <= LevelDebug
}

// SetLevel sets the logging level
func SetLevel(level LogLevel) {
	currentLevel = level
	if level == LevelDebug {
		debug.Init(true)
	}
}

// Debugf logs a debug message if debug logging is enabled
func Debugf(format string, args ...interface{}) {
	if IsDebugEnabled() {
		if debug.IsEnabled {
			// Use the debug package's formatting if it's enabled
			debug.Printf(format, args...)
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
