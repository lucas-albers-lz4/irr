package testutil

import (
	"bytes"
	"strings"

	"github.com/lalbers/irr/pkg/log"
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
