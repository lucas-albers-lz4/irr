package testutil

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestSuppressLogging(t *testing.T) {
	t.Run("Redirects and restores stdout/stderr", func(t *testing.T) {
		// Capture original stdout/stderr for comparison after restore
		originalStdout := os.Stdout
		originalStderr := os.Stderr

		restore := SuppressLogging()
		assert.NotNil(t, restore, "SuppressLogging should return a restore function")

		// Check if stdout/stderr were redirected (cannot easily verify they point to /dev/null)
		assert.NotEqual(t, originalStdout, os.Stdout, "os.Stdout should be redirected")
		assert.NotEqual(t, originalStderr, os.Stderr, "os.Stderr should be redirected")

		// Attempt to print something (should go to /dev/null)
		fmt.Println("This should be suppressed (stdout)")
		fmt.Fprintln(os.Stderr, "This should be suppressed (stderr)")

		// Restore original streams
		restore()

		// Verify that stdout/stderr are restored
		assert.Equal(t, originalStdout, os.Stdout, "os.Stdout should be restored")
		assert.Equal(t, originalStderr, os.Stderr, "os.Stderr should be restored")

		// Verify we can print again (optional, but good check)
		fmt.Println("Logging restored (stdout)")
		fmt.Fprintln(os.Stderr, "Logging restored (stderr)")
	})
}

func TestCaptureLogging(t *testing.T) {
	t.Run("Captures and restores stdout/stderr", func(t *testing.T) {
		// Capture original stdout/stderr for comparison after restore
		originalStdout := os.Stdout
		originalStderr := os.Stderr

		restoreAndGetOutput := CaptureLogging()
		assert.NotNil(t, restoreAndGetOutput, "CaptureLogging should return a restore function")

		// Check if stdout/stderr were redirected
		assert.NotEqual(t, originalStdout, os.Stdout, "os.Stdout should be redirected")
		assert.NotEqual(t, originalStderr, os.Stderr, "os.Stderr should be redirected")

		// Print messages to be captured
		stdoutMsg := "Message for stdout"
		stderrMsg := "Message for stderr"
		fmt.Println(stdoutMsg)
		fmt.Fprintln(os.Stderr, stderrMsg)

		// Restore original streams and get captured output
		capturedOut, capturedErr := restoreAndGetOutput()

		// Verify that stdout/stderr are restored
		assert.Equal(t, originalStdout, os.Stdout, "os.Stdout should be restored")
		assert.Equal(t, originalStderr, os.Stderr, "os.Stderr should be restored")

		// Verify captured output (trimming whitespace as Println might add extra newline)
		assert.Equal(t, stdoutMsg, strings.TrimSpace(capturedOut), "Captured stdout mismatch")
		assert.Equal(t, stderrMsg, strings.TrimSpace(capturedErr), "Captured stderr mismatch")

		// Verify we can print again (optional)
		fmt.Println("Logging restored after capture (stdout)")
		fmt.Fprintln(os.Stderr, "Logging restored after capture (stderr)")
	})
}
