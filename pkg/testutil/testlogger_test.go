package testutil

import (
	"bytes"
	"fmt"
	"testing"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUseTestLogger(t *testing.T) {
	t.Run("Runs without error", func(t *testing.T) {
		// This test mainly ensures the function runs without panicking.
		// Verifying the t.Cleanup behavior and output on failure is complex
		// and better suited for integration or manual testing.
		restore := UseTestLogger(t)
		assert.NotNil(t, restore, "UseTestLogger should return a restore function")
		// We can call the restore function to ensure it doesn't panic, although it does nothing in the current implementation.
		restore()
	})
}

// TestCaptureLogging verifies that CaptureLogOutput correctly captures log messages.
func TestCaptureLogging(t *testing.T) {
	t.Run("Captures Info Logs", func(t *testing.T) {
		// Set LOG_FORMAT to text for this test
		t.Setenv("LOG_FORMAT", "text")

		logMsg := "This is an info message for capture"
		debugMsg := "This debug message should NOT be captured"

		// Capture logs at Info level
		output, err := CaptureLogOutput(log.LevelInfo, func() {
			log.Debug(debugMsg)                                        // Should not appear in output
			log.Info(logMsg)                                           // Should appear in output
			fmt.Println("This goes to stdout, not captured by logger") // Verify stdout is unaffected
		})

		// Assert results
		require.NoError(t, err, "CaptureLogOutput failed")
		assert.Contains(t, output, logMsg, "Captured output should contain the info message")
		assert.NotContains(t, output, debugMsg, "Captured output should NOT contain the debug message")
		assert.Contains(t, output, `level=INFO`, "Log entry should have INFO level (text format)") // Check text format
		// Add more assertions as needed, e.g., checking specific key-value pairs if using structured logs

		// Note: We don't check stdout directly here, as CaptureLogOutput only captures the logger's output.
	})

	t.Run("Captures Debug Logs", func(t *testing.T) {
		// Set LOG_FORMAT to text for this test
		t.Setenv("LOG_FORMAT", "text")

		debugMsg := "This is a debug message"

		// Capture logs at Debug level
		output, err := CaptureLogOutput(log.LevelDebug, func() {
			log.Debug(debugMsg)
		})

		// Assert results
		require.NoError(t, err, "CaptureLogOutput failed")
		assert.Contains(t, output, debugMsg, "Captured output should contain the debug message")
		assert.Contains(t, output, `level=DEBUG`, "Log entry should have DEBUG level (text format)") // Check text format
	})

	// Add more test cases if needed, e.g., for Warn, Error levels, different formats, etc.
}

// TestSuppressLogging verifies that logging is suppressed and restored correctly.
func TestSuppressLogging(t *testing.T) {
	// 1. Setup a buffer to capture potential log output for the test duration.
	var testBuf bytes.Buffer
	restoreOutput := log.SetOutput(&testBuf)
	defer restoreOutput() // Ensure original output is restored after the test.

	// Log something initially to ensure the buffer capture works.
	log.Info("Initial message before suppression")
	initialContent := testBuf.String()
	assert.NotEmpty(t, initialContent, "Should have logged initial message")
	testBuf.Reset() // Clear buffer for the suppression test itself.

	// 2. Suppress logging
	restoreSuppression := SuppressLogging()

	// 3. Log while suppressed
	log.Warn("This message should be suppressed") // Use Warn for variety
	assert.Empty(t, testBuf.String(), "Log buffer should be empty while logging is suppressed")

	// 4. Restore logging
	restoreSuppression()

	// 5. Log after restoration
	log.Error("This message should appear after restoration") // Use Error for variety
	afterRestoreContent := testBuf.String()
	assert.NotEmpty(t, afterRestoreContent, "Should have logged message after restoration")
	assert.Contains(t, afterRestoreContent, "This message should appear after restoration",
		"Log buffer should contain the message logged after restoration")
	assert.NotContains(t, afterRestoreContent, "This message should be suppressed",
		"Log buffer should NOT contain the message logged during suppression")
}
