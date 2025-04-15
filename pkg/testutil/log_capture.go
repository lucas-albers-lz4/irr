package testutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lalbers/irr/pkg/log"
)

// CaptureLogOutput redirects log output during test execution and returns captured content.
// It properly restores the original output after the test function completes.
// Example usage:
//
//	output := testutil.CaptureLogOutput(log.LevelDebug, func() {
//	    // Code that generates log output
//	    log.Infof("This will be captured")
//	})
//	assert.Contains(t, output, "This will be captured")
func CaptureLogOutput(logLevel log.Level, testFunc func()) (string, error) {
	// Save original stderr and log level
	originalStderr := os.Stderr
	originalLevel := log.CurrentLevel()

	// Create pipe to capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("failed to create pipe: %w", err)
	}

	// Redirect stderr to the pipe
	os.Stderr = w

	// Set log level for the test
	log.SetLevel(logLevel)

	// Execute the test function with the redirected stderr
	testFunc()

	// Close the writer and restore the original stderr
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("failed to close pipe writer: %w", err)
	}
	os.Stderr = originalStderr

	// Read the captured output
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return "", fmt.Errorf("failed to copy from pipe: %w", err)
	}

	// Restore original stderr and log level
	log.SetLevel(originalLevel)

	return buf.String(), nil
}

// ContainsLog checks if the log output contains the specified message
func ContainsLog(output, message string) bool {
	return strings.Contains(output, message)
}
