// Package exitcodes provides centralized exit code definitions and error handling for the IRR tool.
// Exit codes are organized in ranges to categorize different types of failures:
//
//	0:     Success
//	1-9:   Input/Configuration Errors (e.g., missing flags, invalid config)
//	10-19: Chart Processing Errors (e.g., unsupported structures, parsing failures)
//	20-29: Runtime Errors (e.g., I/O errors, system failures)
package exitcodes

import (
	"errors"
	"fmt"
)

// Exit code constants organized by category
const (
	// Success (0)
	ExitSuccess = 0

	// Input/Configuration Errors (1-9)
	ExitMissingRequiredFlag     = 1 // Required command flag not provided
	ExitInputConfigurationError = 2 // General configuration error
	ExitCodeInvalidStrategy     = 3 // Invalid path strategy specified
	ExitChartNotFound           = 4 // Chart or values file not found

	// Chart Processing Errors (10-19)
	ExitChartParsingError     = 10 // Failed to parse or load chart
	ExitImageProcessingError  = 11 // Failed to process image references
	ExitUnsupportedStructure  = 12 // Unsupported structure found (e.g., templates in strict mode)
	ExitThresholdError        = 13 // Failed to meet processing success threshold
	ExitChartLoadFailed       = 14 // Failed to load chart
	ExitChartProcessingFailed = 15 // Failed to process chart
	ExitHelmCommandFailed     = 16 // Helm command execution failed

	// Runtime Errors (20-29)
	ExitGeneralRuntimeError = 20 // General runtime/system error
	ExitIOError             = 21 // IO operation error
)

// ExitCodeError wraps an error with an exit code for consistent error handling.
// This type is used throughout the codebase to propagate both error details
// and the appropriate exit code up the call stack.
type ExitCodeError struct {
	Code int   // Exit code to return
	Err  error // Underlying error
}

func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("exit code %d: %v", e.Code, e.Err)
}

func (e *ExitCodeError) Unwrap() error {
	return e.Err
}

// IsExitCodeError checks if an error is an ExitCodeError and returns its code.
// Returns false and 0 if the error is not an ExitCodeError.
func IsExitCodeError(err error) (int, bool) {
	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code, true
	}
	return 0, false
}
