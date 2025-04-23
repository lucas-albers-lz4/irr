package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/log"
	"github.com/stretchr/testify/assert"
)

// CaptureLogOutput redirects log output using log.SetOutput during test execution
// and returns captured content. It properly restores the original output and log level
// after the test function completes.
// Example usage:
//
//	output, err := testutil.CaptureLogOutput(log.LevelDebug, func() {
//	    // Code that generates log output
//	    log.Info("This will be captured")
//	})
//	require.NoError(t, err)
//	assert.Contains(t, output, "This will be captured")
func CaptureLogOutput(logLevel log.Level, testFunc func()) (string, error) {
	// Save original log level
	originalLevel := log.CurrentLevel()

	// --- Use log.SetOutput --- Start
	var logBuf bytes.Buffer
	restoreLog := log.SetOutput(&logBuf)
	// Defer the restore function from log.SetOutput
	defer restoreLog()
	// --- Use log.SetOutput --- End

	// Set the desired log level for the test *after* setting the output buffer
	// because log.SetOutput might reconfigure based on environment variables initially.
	log.SetLevel(logLevel)

	// Ensure the original level is restored *after* the test function runs
	// and *after* the log output is restored by the previous defer.
	defer log.SetLevel(originalLevel)

	// Execute the test function, recovering from panics.
	var panicErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr = fmt.Errorf("panic during log capture: %v", r)
			}
		}()
		testFunc()
	}()

	// No need to manually handle pipes or os.Stderr anymore.

	// Return the captured log content from the buffer and any panic error.
	return logBuf.String(), panicErr
}

// ContainsLog checks if the log output contains the specified message
func ContainsLog(output, message string) bool {
	return strings.Contains(output, message)
}

// CaptureJSONLogs captures log output specifically in JSON format.
// It temporarily sets the LOG_FORMAT environment variable to "json",
// captures the log output during the execution of testFunc,
// parses each line as JSON, and returns a slice of maps.
// It restores the original LOG_FORMAT afterwards.
// The logLevel parameter controls the minimum level of logs captured.
// It returns the raw captured output string, the parsed logs, and any error.
func CaptureJSONLogs(logLevel log.Level, testFunc func()) (logOutput string, parsedLogs []map[string]interface{}, err error) {
	// --- Environment Variable Handling ---
	originalLogFormat := os.Getenv("LOG_FORMAT")
	if setErr := os.Setenv("LOG_FORMAT", "json"); setErr != nil {
		err = fmt.Errorf("failed to set LOG_FORMAT=json: %w", setErr)
		return
	}
	defer func() {
		if restoreErr := os.Setenv("LOG_FORMAT", originalLogFormat); restoreErr != nil {
			log.Error("failed to restore original LOG_FORMAT environment variable", "originalValue", originalLogFormat, "error", restoreErr)
		}
	}()

	// --- Enable Timestamps for Test Capture ---
	log.SetTestModeWithTimestamps(true)
	// Ensure the test mode is disabled AFTER the capture setup is restored
	defer log.SetTestModeWithTimestamps(false)

	// --- Capture Logic (Original Version + Level Setting) ---
	originalLevel := log.CurrentLevel()
	var logBuf bytes.Buffer

	// SetOutput will trigger configureLogger, which now respects the test mode flag
	restoreLog := log.SetOutput(&logBuf)
	// restoreLog (returned by SetOutput) restores the original writer and reconfigures.
	// The defer below ensures test mode is off AFTER restoreLog runs.
	defer restoreLog()

	// SetLevel also triggers configureLogger, ensuring level is set correctly with test mode active.
	log.SetLevel(logLevel)
	// Restore original level AFTER test func runs and AFTER restoreLog runs.
	defer log.SetLevel(originalLevel)

	// Execute the test function, recovering from panics.
	var panicErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr = fmt.Errorf("panic during log capture: %v", r)
			}
		}()
		testFunc()
	}()

	// Populate logOutput before returning on panic or empty buffer
	logOutput = logBuf.String()

	// If the test function panicked, return the panic error immediately.
	if panicErr != nil {
		err = panicErr
		return
	}

	// --- JSON Parsing ---
	if strings.TrimSpace(logOutput) == "" {
		return
	}

	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	var parseErr error
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]interface{}
		if unmarshalErr := json.Unmarshal([]byte(line), &entry); unmarshalErr != nil {
			parseErr = fmt.Errorf("failed to unmarshal log line %d as JSON: %w\nLine content: %s", i+1, unmarshalErr, line)
			break
		}
		parsedLogs = append(parsedLogs, entry)
	}

	err = parseErr
	return
}

// AssertLogContainsJSON checks if any log entry in the captured logs (as a slice of maps)
// contains all the key-value pairs present in the expectedLog map.
func AssertLogContainsJSON(t *testing.T, logs []map[string]interface{}, expectedLog map[string]interface{}) {
	t.Helper()
	found := false
	for _, logEntry := range logs {
		if containsAll(logEntry, expectedLog) {
			found = true
			break
		}
	}
	if !found {
		// Log the captured logs and the expected log for easier debugging
		var logBuffer bytes.Buffer
		encoder := json.NewEncoder(&logBuffer)
		encoder.SetIndent("", "  ") // Pretty print the JSON
		for _, entry := range logs {
			_ = encoder.Encode(entry) //nolint:errcheck // Ignore error for test helper
		}

		expectedLogJSON, _ := json.MarshalIndent(expectedLog, "", "  ") //nolint:errcheck // Ignore error for test helper

		assert.Fail(t, "Expected log entry not found",
			"Expected log containing:\n%s\n\nActual captured logs:\n%s",
			string(expectedLogJSON), logBuffer.String())
	}
}

// AssertLogDoesNotContainJSON checks if no log entry in the captured logs (as a slice of maps)
// contains all the key-value pairs present in the unexpectedLog map.
func AssertLogDoesNotContainJSON(t *testing.T, logs []map[string]interface{}, unexpectedLog map[string]interface{}) {
	t.Helper()
	found := false
	var foundEntry map[string]interface{}
	for _, logEntry := range logs {
		if containsAll(logEntry, unexpectedLog) {
			found = true
			foundEntry = logEntry
			break
		}
	}
	if found {
		// Log the found entry and the unexpected log for easier debugging
		foundEntryJSON, _ := json.MarshalIndent(foundEntry, "", "  ")       //nolint:errcheck // Ignore error for test helper
		unexpectedLogJSON, _ := json.MarshalIndent(unexpectedLog, "", "  ") //nolint:errcheck // Ignore error for test helper

		assert.Fail(t, "Unexpected log entry found",
			"Found log entry:\n%s\n\nUnexpected log containing:\n%s",
			string(foundEntryJSON), string(unexpectedLogJSON))
	}
}

// containsAll checks if the actual map contains all key-value pairs from the expected map.
// It performs a deep comparison for nested maps and slices if necessary,
// but primarily focuses on top-level key matching for structured logs.
func containsAll(actual, expected map[string]interface{}) bool {
	for key, expectedValue := range expected {
		actualValue, ok := actual[key]
		if !ok {
			return false // Key not found in actual log entry
		}

		// Simple comparison for basic types (string, number, bool)
		// Convert numbers to float64 for consistent comparison
		switch actualVal := actualValue.(type) {
		case float64:
			switch expectedVal := expectedValue.(type) {
			case float64:
				if actualVal != expectedVal {
					return false
				}
			case int:
				// Allow comparison between float64 (from JSON) and int (from test)
				if actualVal != float64(expectedVal) {
					return false
				}
			case int64: // Allow comparison with int64 as well
				if actualVal != float64(expectedVal) {
					return false
				}
			default:
				// Type mismatch (e.g., comparing float64 with string)
				return false
			}
		// Add cases for other numeric types if needed, e.g., actualValue being int/int64
		default:
			// Non-float actual value, use direct comparison
			if actualValue != expectedValue {
				// Use assert.ObjectsAreEqual for potentially more complex types if needed,
				// but direct comparison covers most common log field types.
				// For now, a simple direct comparison should suffice for typical log fields.
				// if !assert.ObjectsAreEqual(actualValue, expectedValue) { // If more complex comparison is needed
				return false
				// }
			}
		}
	}
	return true // All expected key-value pairs were found and matched
}
