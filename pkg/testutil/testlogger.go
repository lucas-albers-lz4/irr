package testutil

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/log"
)

var (
	// originalOut holds the original standard output
	// originalOut *os.File // No longer needed for log capture
	// originalErr holds the original standard error
	// originalErr *os.File // No longer needed for log capture
	// mutex protects concurrent access to logger state
	mutex sync.Mutex
)

// SuppressLogging redirects all logging output to /dev/null during test execution
// Call the returned function to restore original logging
func SuppressLogging() func() {
	mutex.Lock()
	defer mutex.Unlock()

	// Save original logger output writer
	var buf bytes.Buffer              // Temporary buffer, not actually used for suppression
	restoreLog := log.SetOutput(&buf) // Get restore func first

	// Now set output to io.Discard (preferred over /dev/null)
	log.SetOutput(io.Discard)

	return func() {
		mutex.Lock()
		defer mutex.Unlock()
		// Restore the original logger output
		restoreLog()
	}
}

// CaptureLogging captures log output using log.SetOutput.
// Call the returned function to restore original logging and retrieve the captured output.
// Note: This only captures logs written via the pkg/log functions.
// It does NOT capture direct writes to os.Stdout or os.Stderr.
func CaptureLogging() func() string {
	mutex.Lock()
	// No defer mutex.Unlock() here, restore function will handle it

	var logBuf bytes.Buffer
	logRestore := log.SetOutput(&logBuf)

	// Return the restore function
	return func() string {
		// Ensure unlock happens after restoration and buffer read
		defer mutex.Unlock()

		// Restore the original logger output
		logRestore()

		// Return the captured log content
		return logBuf.String()
	}
}

// UseTestLogger sets up a logger that only prints output if the test fails
// This should be called at the beginning of a test with defer t.Cleanup(restore)
func UseTestLogger(t *testing.T) func() {
	t.Helper()

	// Only capture output on non-verbose test runs
	if !testing.Verbose() {
		// Use the new CaptureLogging which only captures log output
		restoreAndGetLogs := CaptureLogging()

		t.Cleanup(func() {
			capturedLogs := restoreAndGetLogs() // Call restore func to get logs
			// Only print the output if the test fails
			if t.Failed() {
				t.Logf("Log output captured during test:\n%s", capturedLogs)
				// No separate stdout/stderr capture now
				// t.Logf("Standard error captured during test:\n%s", errOutput)
			}
		})
	}

	// Return a no-op function, as cleanup handles restoration
	return func() {}
}
