package debug

import (
	"fmt"
	"os"
	"strings"
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
	// IsEnabled indicates whether debug logging is enabled
	IsEnabled bool

	// debugPrefix is prepended to all debug messages
	// @llm-helper This prefix helps identify debug output
	// @llm-helper Can be customized using SetPrefix
	debugPrefix = "[DEBUG] "
)

// Init initializes the debug package with the given configuration.
// @param enabled: Whether to enable debug logging
// @llm-helper This must be called before using other functions
// @llm-helper Sets the global enabled state
func Init(enabled bool) {
	IsEnabled = enabled
}

// Printf prints a debug message if debug logging is enabled.
// @param format: Printf-style format string
// @param args: Arguments for format string
// @llm-helper This function is similar to fmt.Printf
// @llm-helper Only outputs if debugging is enabled
func Printf(format string, args ...interface{}) {
	if IsEnabled {
		fmt.Fprintf(os.Stderr, debugPrefix+format+"\n", args...)
	}
}

// Println prints a debug message if debug logging is enabled.
// @param args: Values to print
// @llm-helper This function is similar to fmt.Println
// @llm-helper Only outputs if debugging is enabled
func Println(args ...interface{}) {
	if IsEnabled {
		fmt.Fprintln(os.Stderr, debugPrefix+fmt.Sprint(args...))
	}
}

// FunctionEnter logs entry into a function if debug logging is enabled.
// @param funcName: Name of the function being entered
// @llm-helper This function helps track program flow
// @llm-helper Uses → arrow to indicate entry
func FunctionEnter(funcName string) {
	if IsEnabled {
		fmt.Fprintf(os.Stderr, "%s→ Entering %s\n", debugPrefix, funcName)
	}
}

// FunctionExit logs exit from a function if debug logging is enabled.
// @param funcName: Name of the function being exited
// @llm-helper This function helps track program flow
// @llm-helper Uses ← arrow to indicate exit
func FunctionExit(funcName string) {
	if IsEnabled {
		fmt.Fprintf(os.Stderr, "%s← Exiting %s\n", debugPrefix, funcName)
	}
}

// DumpValue dumps a value with a label if debug logging is enabled.
// @param label: Label for the value being dumped
// @param value: Value to dump
// @llm-helper This function pretty prints complex values
// @llm-helper Useful for inspecting data structures
func DumpValue(label string, value interface{}) {
	if IsEnabled {
		fmt.Fprintf(os.Stderr, "%s%s: %+v\n", debugPrefix, label, value)
	}
}

// SetPrefix sets a custom prefix for debug messages.
// @param prefix: New prefix to use
// @llm-helper This function customizes message format
// @llm-helper Ensures prefix ends with a space
func SetPrefix(prefix string) {
	if !strings.HasSuffix(prefix, " ") {
		prefix += " "
	}
	debugPrefix = prefix
}

// DumpYAML was removed
