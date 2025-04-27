// Package log_test contains tests for the log package.
package log

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		levelStr string
		want     Level
		wantErr  bool
	}{
		{name: "debug", levelStr: "DEBUG", want: LevelDebug, wantErr: false},
		{name: "lowercase debug", levelStr: "debug", want: LevelDebug, wantErr: false},
		{name: "mixed case debug", levelStr: "Debug", want: LevelDebug, wantErr: false},
		{name: "info", levelStr: "INFO", want: LevelInfo, wantErr: false},
		{name: "warn", levelStr: "WARN", want: LevelWarn, wantErr: false},
		{name: "warning", levelStr: "WARNING", want: LevelWarn, wantErr: false},
		{name: "error", levelStr: "ERROR", want: LevelError, wantErr: false},
		{name: "invalid", levelStr: "INVALID", want: LevelInfo, wantErr: true},
		{name: "empty", levelStr: "", want: LevelInfo, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLevel(tt.levelStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseLevel() = %v, want %v", got, tt.want)
			}

			// Test error wrapping for invalid cases
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidLogLevel) {
					t.Errorf("ParseLevel() error not wrapping ErrInvalidLogLevel: %v", err)
				}
				if !strings.Contains(err.Error(), tt.levelStr) {
					t.Errorf("ParseLevel() error message should contain the invalid level string '%s': %v", tt.levelStr, err)
				}
			}

			// Add tests for the String() method within TestParseLevel
			t.Run("String method", func(t *testing.T) {
				assert.Equal(t, "DEBUG", LevelDebug.String(), "String() for LevelDebug")
				assert.Equal(t, "INFO", LevelInfo.String(), "String() for LevelInfo")
				assert.Equal(t, "WARN", LevelWarn.String(), "String() for LevelWarn")
				assert.Equal(t, "ERROR", LevelError.String(), "String() for LevelError")
				assert.Equal(t, "UNKNOWN", Level(99).String(), "String() for unknown level")
			})
		})
	}
}

func TestSetAndCurrentLevel(t *testing.T) {
	// Save the original level to restore after test
	originalLevel := CurrentLevel() // Get current level using public API
	defer func() {
		SetLevel(originalLevel) // Restore using public API
	}()

	// Test with both slog.Level and the custom Level types, since SetLevel supports both
	testLevels := []struct {
		name   string
		level  interface{} // Use interface{} to hold either type
		expect slog.Level  // The expected underlying slog.Level after setting
	}{ // Test both slog.Level and custom Level inputs
		{"slog.LevelDebug", slog.LevelDebug, slog.LevelDebug},
		{"LevelDebug", LevelDebug, slog.LevelDebug},
		{"slog.LevelInfo", slog.LevelInfo, slog.LevelInfo},
		{"LevelInfo", LevelInfo, slog.LevelInfo},
		{"slog.LevelWarn", slog.LevelWarn, slog.LevelWarn},
		{"LevelWarn", LevelWarn, slog.LevelWarn},
		{"slog.LevelError", slog.LevelError, slog.LevelError},
		{"LevelError", LevelError, slog.LevelError},
	}

	for _, tt := range testLevels {
		t.Run(tt.name, func(t *testing.T) {
			oldStderr := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}
			os.Stderr = w // Capture stderr

			SetLevel(tt.level) // Call the public SetLevel function

			if err := w.Close(); err != nil {
				t.Logf("Warning: failed to close pipe writer: %v", err) // Log warning, not fatal
			}
			os.Stderr = oldStderr // Restore stderr

			// Read the output but don't use it directly for assertion anymore
			_, err = io.ReadAll(r)
			if err != nil {
				t.Fatalf("Failed to read from pipe: %v", err)
			}

			// Verify the level was set correctly using the public CurrentLevel()
			assert.Equal(t, tt.expect, CurrentLevel(), "CurrentLevel() returned incorrect value")
		})
	}
}

func TestLevelBasedFiltering(t *testing.T) {
	// Save the original level and output to restore after test
	originalLevel := CurrentLevel()
	var buf bytes.Buffer
	restoreOutput := SetOutput(&buf) // Capture output from the global logger
	defer restoreOutput()
	defer SetLevel(originalLevel) // Restore level last

	tests := []struct {
		name       string
		setLevel   slog.Level
		wantOutput map[slog.Level]bool // Map level to expected output status
	}{
		{
			name:     "debug level shows all logs",
			setLevel: slog.LevelDebug,
			wantOutput: map[slog.Level]bool{
				slog.LevelDebug: true,
				slog.LevelInfo:  true,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:     "info level hides debug logs",
			setLevel: slog.LevelInfo,
			wantOutput: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  true,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:     "warn level hides debug and info logs",
			setLevel: slog.LevelWarn,
			wantOutput: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  false,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:     "error level shows only error logs",
			setLevel: slog.LevelError,
			wantOutput: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  false,
				slog.LevelWarn:  false,
				slog.LevelError: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the global log level
			SetLevel(tt.setLevel)
			buf.Reset() // Clear buffer for this test case

			// Map levels to their names for messages
			levelNames := map[slog.Level]string{
				slog.LevelDebug: "Debug",
				slog.LevelInfo:  "Info",
				slog.LevelWarn:  "Warn",
				slog.LevelError: "Error",
			}
			// Map levels to the corresponding package-level logging function
			logFuncs := map[slog.Level]func(msg string, args ...any){
				slog.LevelDebug: Debug,
				slog.LevelInfo:  Info,
				slog.LevelWarn:  Warn,
				slog.LevelError: Error,
			}

			// Call the package-level logging functions
			for level, name := range levelNames {
				logFunc := logFuncs[level]
				logFunc(fmt.Sprintf("Test message for %s", name), "level", name)
			}

			// Assertions based on the buffer content
			output := buf.String()
			for level, shouldAppear := range tt.wantOutput {
				levelName := levelNames[level]
				// Use a unique part of the message for checking presence/absence
				// Assuming JSON format for simplicity in assertion (default)
				// Use %q for the message string to handle potential special characters
				uniqueMsgFragment := fmt.Sprintf(`"msg":%q`, fmt.Sprintf("Test message for %s", levelName))
				expectedLevelSubstr := fmt.Sprintf(`"level":%q`, strings.ToUpper(levelName))

				if shouldAppear {
					// Check that *a* line contains both the message fragment and the correct level substring
					found := false
					for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
						if strings.Contains(line, uniqueMsgFragment) && strings.Contains(line, expectedLevelSubstr) {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected log for %s was not found at level %s. Output:\n%s", levelName, tt.setLevel, output)
				} else {
					// Check that *no* line contains the specific message fragment
					assert.NotContains(t, output, uniqueMsgFragment, "Unexpected log for %s was found at level %s. Output:\n%s", levelName, tt.setLevel, output)
				}
			}
		})
	}
}

// TestDebug ensures Debug logs are written correctly and respect the level.
func TestDebug(t *testing.T) {
	originalLevel := CurrentLevel()
	var buf bytes.Buffer
	restoreOutput := SetOutput(&buf)
	defer restoreOutput()
	defer SetLevel(originalLevel) // Ensure level is restored regardless of test path

	t.Run("with key-value pairs", func(t *testing.T) {
		// Case 1: Debug level set, debug log should appear
		SetLevel(LevelDebug)
		buf.Reset() // Clear buffer before logging
		Debug("debug message kv", "key", "value")
		output := buf.String()
		assert.Contains(t, output, `"level":"DEBUG"`, "Log output should contain level:DEBUG")
		assert.Contains(t, output, `"msg":"debug message kv"`, "Log output should contain the message")
		assert.Contains(t, output, `"key":"value"`, "Log output should contain key:value pair")

		// Case 2: Info level set, debug log should NOT appear
		SetLevel(LevelInfo)
		buf.Reset() // Clear buffer before logging
		Debug("another debug message kv")
		assert.Empty(t, buf.String(), "Debug log should not appear when level is INFO")
	})

	t.Run("message only", func(t *testing.T) {
		// Case 1: Debug level set, debug log should appear
		SetLevel(LevelDebug)
		buf.Reset() // Clear buffer before logging
		Debug("debug message only")
		output := buf.String()
		assert.Contains(t, output, `"level":"DEBUG"`, "Log output should contain level:DEBUG")
		assert.Contains(t, output, `"msg":"debug message only"`, "Log output should contain the message")
		assert.NotContains(t, output, `"key"`, "Log output should not contain key-value pair") // Check no extra args appear

		// Case 2: Info level set, debug log should NOT appear
		SetLevel(LevelInfo)
		buf.Reset() // Clear buffer before logging
		Debug("another debug message only")
		assert.Empty(t, buf.String(), "Debug log should not appear when level is INFO")
	})
}

// TestInfoWarnError tests the Info, Warn, and Error log functions.
func TestInfoWarnError(t *testing.T) {
	originalLevel := CurrentLevel()
	defer SetLevel(originalLevel) // Ensure level is restored

	var buf bytes.Buffer
	restoreOutput := SetOutput(&buf)
	defer restoreOutput()

	tests := []struct {
		name        string
		levelToSet  Level
		logFunc     func(msg string, args ...any)
		levelString string // e.g., "INFO", "WARN", "ERROR"
		shouldLog   bool   // Whether the message should be logged at the set level
	}{
		{"Info logs at Info level", LevelInfo, Info, "INFO", true},
		{"Info logs at Debug level", LevelDebug, Info, "INFO", true},
		{"Info does not log at Warn level", LevelWarn, Info, "INFO", false},
		{"Warn logs at Warn level", LevelWarn, Warn, "WARN", true},
		{"Warn logs at Info level", LevelInfo, Warn, "WARN", true},
		{"Warn logs at Debug level", LevelDebug, Warn, "WARN", true},
		{"Warn does not log at Error level", LevelError, Warn, "WARN", false},
		{"Error logs at Error level", LevelError, Error, "ERROR", true},
		{"Error logs at Warn level", LevelWarn, Error, "ERROR", true},
		{"Error logs at Info level", LevelInfo, Error, "ERROR", true},
		{"Error logs at Debug level", LevelDebug, Error, "ERROR", true},
	}

	for _, tt := range tests {
		t.Run(tt.name+" with key-value", func(t *testing.T) {
			SetLevel(tt.levelToSet)
			buf.Reset()
			msg := fmt.Sprintf("test message for %s kv", tt.levelString)
			key := "testkey"
			val := tt.levelString
			tt.logFunc(msg, key, val)

			output := buf.String()
			if tt.shouldLog {
				assert.Contains(t, output, fmt.Sprintf(`"level":%q`, tt.levelString), "Log output should contain correct level")
				assert.Contains(t, output, fmt.Sprintf(`"msg":%q`, msg), "Log output should contain message")
				assert.Contains(t, output, fmt.Sprintf(`%q:%q`, key, val), "Log output should contain key-value pair")
			} else {
				assert.Empty(t, output, "Log output should be empty when level is too high")
			}
		})

		t.Run(tt.name+" message only", func(t *testing.T) {
			SetLevel(tt.levelToSet)
			buf.Reset()
			msg := fmt.Sprintf("test message for %s only", tt.levelString)
			tt.logFunc(msg) // Call with only the message

			output := buf.String()
			if tt.shouldLog {
				assert.Contains(t, output, fmt.Sprintf(`"level":%q`, tt.levelString), "Log output should contain correct level")
				assert.Contains(t, output, fmt.Sprintf(`"msg":%q`, msg), "Log output should contain message")
				// Ensure no unexpected key-value pair is present
				assert.NotContains(t, output, `"testkey"`, "Log output should not contain key-value pair for message-only log")
			} else {
				assert.Empty(t, output, "Log output should be empty when level is too high")
			}
		})
	}
}

// TestSetOutput verifies that SetOutput correctly changes the log output destination
// and that the restore function restores the original output.
func TestSetOutput(t *testing.T) {
	// 1. Setup: Save original state and create a buffer
	originalLevel := CurrentLevel()
	originalWriter := outputWriter // Directly access the package variable for comparison
	defer SetLevel(originalLevel)  // Restore level
	defer func() {                 // Ensure original writer is restored even if test panics
		outputWriter = originalWriter
		configureLogger()
	}()

	var buf1 bytes.Buffer

	// 2. Change Output
	restore := SetOutput(&buf1)
	assert.NotSame(t, originalWriter, outputWriter, "outputWriter should have changed after SetOutput")
	assert.Same(t, &buf1, outputWriter, "outputWriter should be the buffer we set")

	// 3. Log to the new output and verify
	SetLevel(LevelInfo)
	Info("message to buffer 1")
	assert.Contains(t, buf1.String(), `"msg":"message to buffer 1"`, "Log message should go to the new buffer")

	// 4. Restore Output
	restore()
	assert.Same(t, originalWriter, outputWriter, "outputWriter should be restored to the original")

	// 5. Log again and verify it doesn't go to the buffer
	buf1.Reset() // Clear the buffer
	Info("message after restore")
	assert.Empty(t, buf1.String(), "Log message should not go to the buffer after restoring output")

	// Optional: Test changing to another buffer
	var buf2 bytes.Buffer
	restore2 := SetOutput(&buf2)
	assert.Same(t, &buf2, outputWriter, "outputWriter should be the second buffer")
	Info("message to buffer 2")
	assert.Contains(t, buf2.String(), `"msg":"message to buffer 2"`, "Log message should go to the second buffer")
	assert.Empty(t, buf1.String(), "Log message should not go to the first buffer when output is changed again")
	restore2() // Restore back to original (os.Stderr)
	assert.Same(t, originalWriter, outputWriter, "outputWriter should be restored to the original again")
}

// TestLogger checks if the Logger function returns a non-nil logger instance.
func TestLogger(t *testing.T) {
	assert.NotNil(t, Logger(), "Logger() should return a non-nil logger instance")
	// We could potentially check the type or configuration, but non-nil is a good start.
}

// TestSetTestModeWithTimestamps verifies that timestamps are correctly included or excluded
// from JSON logs based on the flag setting.
func TestSetTestModeWithTimestamps(t *testing.T) {
	originalLevel := CurrentLevel()
	var buf bytes.Buffer
	restoreOutput := SetOutput(&buf)
	originalTestMode := includeTimestampsForTest // Save original test mode flag
	defer restoreOutput()
	defer SetLevel(originalLevel)
	defer func() { // Ensure test mode flag is restored
		SetTestModeWithTimestamps(originalTestMode)
	}()

	// Ensure logger is JSON format for this test
	originalFormat := os.Getenv("LOG_FORMAT")
	err := os.Setenv("LOG_FORMAT", "json")
	require.NoError(t, err, "Failed to set LOG_FORMAT=json for test")
	configureLogger() // Reconfigure with JSON format
	defer func() {    // Restore original format
		err := os.Setenv("LOG_FORMAT", originalFormat)
		if err != nil {
			t.Logf("Warning: failed to restore LOG_FORMAT: %v", err)
		}
		configureLogger()
	}()

	// Case 1: Timestamps Enabled
	SetTestModeWithTimestamps(true)
	buf.Reset()
	Info("message with timestamp")
	output := buf.String()
	assert.Contains(t, output, `"time":"`, "JSON log should contain time field when enabled")
	assert.Contains(t, output, `"msg":"message with timestamp"`, "JSON log should contain message")

	// Case 2: Timestamps Disabled (Default behavior might be no timestamps)
	SetTestModeWithTimestamps(false)
	buf.Reset()
	Info("message without timestamp")
	output = buf.String()
	assert.NotContains(t, output, `"time":"`, "JSON log should NOT contain time field when disabled")
	assert.Contains(t, output, `"msg":"message without timestamp"`, "JSON log should contain message")
}
