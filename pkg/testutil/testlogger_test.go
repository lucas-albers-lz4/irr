package testutil

import (
	"fmt"
	"testing"

	"github.com/lalbers/irr/pkg/log"
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
