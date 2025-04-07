// Package debug provides simple debugging utilities, mainly controlled by an environment variable.
package debug

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Package debug provides simple conditional debugging output.
// This package implements a simple but effective debugging system that can be
// enabled at runtime to help diagnose issues and track program flow.

// Key concepts:
// - Debug State: Global enabled/disabled state
// - Debug Prefix: Prefix added to all debug messages
// - Function Tracking: Enter/exit logging for functions
// - Value Dumping: Pretty printing of complex values

// Debug message format:
// [DEBUG] message
// [DEBUG] → Entering function
// [DEBUG] ← Exiting function
// [DEBUG] Label: value

// @llm-helper This package uses stderr for debug output
// @llm-helper Debug state is controlled by a global flag
// @llm-helper Messages are prefixed for easy filtering

var (
	// Enabled controls whether debug logging is active.
	Enabled bool
	// startTime stores the time when Init was called.
	startTime time.Time

	// debugPrefix was removed as it was unused
)

func init() {
	debugEnv := os.Getenv("IRR_DEBUG")
	if debugEnv != "" {
		var err error
		Enabled, err = strconv.ParseBool(debugEnv)
		if err != nil {
			// Default to false if parsing fails, maybe log this?
			Enabled = false
			fmt.Fprintf(os.Stderr, "Warning: Invalid boolean value for IRR_DEBUG: %s. Defaulting to false.\n", debugEnv)
		}
		// Assignment to debugPrefix removed
	} else {
		Enabled = false
	}
}

// Init initializes the debug package, checking the IRR_DEBUG environment variable.
func Init(forceEnable bool) {
	if forceEnable {
		Enabled = true
	} else {
		debugEnv := os.Getenv("IRR_DEBUG")
		// Handle error from strconv.ParseBool
		var err error
		Enabled, err = strconv.ParseBool(debugEnv)
		if err != nil {
			// Default to false if parsing fails
			Enabled = false
			fmt.Fprintf(os.Stderr, "Warning: Invalid boolean value for IRR_DEBUG: %s. Defaulting to false.\n", debugEnv)
		}
	}
	startTime = time.Now()
	if Enabled {
		Printf("Debug logging enabled at %s", startTime.Format(time.RFC3339))
	}
}

// Printf logs a formatted debug message if debug logging is enabled.
func Printf(format string, args ...interface{}) {
	if Enabled {
		prefix := fmt.Sprintf("[DEBUG +%v] ", time.Since(startTime).Round(time.Millisecond))
		fmt.Fprintf(os.Stderr, prefix+format+"\n", args...)
	}
}

// Println logs a debug message followed by a newline if debug logging is enabled.
func Println(args ...interface{}) {
	if Enabled {
		prefix := fmt.Sprintf("[DEBUG +%v] ", time.Since(startTime).Round(time.Millisecond))
		fmt.Fprint(os.Stderr, prefix)
		fmt.Fprintln(os.Stderr, args...)
	}
}

// DumpValue logs the value of a variable in JSON format if debug logging is enabled.
func DumpValue(name string, value interface{}) {
	if Enabled {
		prefix := fmt.Sprintf("[DEBUG +%v] ", time.Since(startTime).Round(time.Millisecond))
		jsonData, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%sDump %s: Error marshalling to JSON: %v\n", prefix, name, err)
			return
		}
		fmt.Fprintf(os.Stderr, "%sDump %s:\n%s\n", prefix, name, string(jsonData))
	}
}

// FunctionEnter logs the entry into a function.
func FunctionEnter(name string) {
	Printf("Enter: %s", name)
}

// FunctionExit logs the exit from a function.
func FunctionExit(name string) {
	Printf("Exit: %s", name)
}

// SetPrefix was removed as debugPrefix was unused

// DumpYAML was removed
