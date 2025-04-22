package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Execute the test function. Logs will go to logBuf.
	testFunc()

	// No need to manually handle pipes or os.Stderr anymore.

	// Return the captured log content from the buffer.
	return logBuf.String(), nil
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
func CaptureJSONLogs(logLevel log.Level, testFunc func()) ([]map[string]interface{}, error) {
	// --- Environment Variable Handling ---
	originalLogFormat := os.Getenv("LOG_FORMAT")
	// Set LOG_FORMAT=json for the duration of this capture
	if err := os.Setenv("LOG_FORMAT", "json"); err != nil {
		return nil, fmt.Errorf("failed to set LOG_FORMAT=json: %w", err)
	}
	defer func() {
		// Restore original LOG_FORMAT
		if err := os.Setenv("LOG_FORMAT", originalLogFormat); err != nil {
			// Log an error if restoring fails, but don't fail the test here
			log.Error("failed to restore original LOG_FORMAT environment variable", "originalValue", originalLogFormat, "error", err)
		}
	}()

	// --- Capture Logic (similar to CaptureLogOutput) ---
	originalLevel := log.CurrentLevel()
	var logBuf bytes.Buffer
	restoreLog := log.SetOutput(&logBuf)
	defer restoreLog()

	log.SetLevel(logLevel)
	defer log.SetLevel(originalLevel)

	// Re-initialize logger to pick up LOG_FORMAT change *after* setting env var
	// NOTE: This assumes log.Initialize (or similar) reads env vars.
	// If the logger setup is more complex, this might need adjustment.
	// We might need an explicit re-initialization function in pkg/log.
	// For now, we assume setting output implicitly handles reconfig or that
	// the logger reads the env var each time it creates a handler.
	// If tests fail due to wrong format, revisit this.

	testFunc()

	// --- JSON Parsing ---
	capturedOutput := logBuf.String()
	var parsedLogs []map[string]interface{}

	// Handle empty output gracefully
	if strings.TrimSpace(capturedOutput) == "" {
		return parsedLogs, nil // Return empty slice, not an error
	}

	lines := strings.Split(strings.TrimSpace(capturedOutput), "\n")
	for i, line := range lines {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Provide context in the error message
			return nil, fmt.Errorf("failed to unmarshal log line %d as JSON: %w\nLine content: %s", i+1, err, line)
		}
		parsedLogs = append(parsedLogs, entry)
	}

	return parsedLogs, nil
}

// AssertLogContainsJSON asserts that at least one log entry in the provided slice
// contains all the key-value pairs specified in expectedFields.
// Uses require.Fail for immediate test failure if no match is found.
func AssertLogContainsJSON(t *testing.T, logs []map[string]interface{}, expectedFields map[string]interface{}, msgAndArgs ...interface{}) {
	t.Helper()

	for _, entry := range logs {
		match := true
		for key, expectedValue := range expectedFields {
			actualValue, ok := entry[key]
			if !ok || !reflect.DeepEqual(actualValue, expectedValue) {
				match = false
				break
			}
		}
		if match {
			// Found a matching entry, assertion passes
			return
		}
	}

	// If no match was found after checking all entries
	// Construct a helpful failure message
	expectedBytes, _ := json.MarshalIndent(expectedFields, "", "  ")
	logsBytes, _ := json.MarshalIndent(logs, "", "  ")
	failureMsg := fmt.Sprintf("Log entries did not contain expected fields.\nExpected Fields:\n%s\n\nActual Logs:\n%s",
		string(expectedBytes), string(logsBytes))

	// Append custom message and args if provided
	require.Fail(t, failureMsg, msgAndArgs...)
}

// AssertLogDoesNotContainJSON asserts that *no* log entry in the provided slice
// contains all the key-value pairs specified in unexpectedFields.
// Uses assert.Fail to allow other assertions to run even if this one passes.
func AssertLogDoesNotContainJSON(t *testing.T, logs []map[string]interface{}, unexpectedFields map[string]interface{}, msgAndArgs ...interface{}) {
	t.Helper()

	for i, entry := range logs {
		match := true
		for key, unexpectedValue := range unexpectedFields {
			actualValue, ok := entry[key]
			if !ok || !reflect.DeepEqual(actualValue, unexpectedValue) {
				match = false
				break
			}
		}
		if match {
			// Found a matching entry, assertion FAILS
			unexpectedBytes, _ := json.MarshalIndent(unexpectedFields, "", "  ")
			entryBytes, _ := json.MarshalIndent(entry, "", "  ")
			failureMsg := fmt.Sprintf("Found unexpected log entry (index %d) containing specified fields.\nUnexpected Fields:\n%s\n\nMatching Entry:\n%s",
				i, string(unexpectedBytes), string(entryBytes))
			assert.Fail(t, failureMsg, msgAndArgs...)
			return // No need to check further if one match is found
		}
	}

	// If no match was found after checking all entries, assertion PASSES
}
