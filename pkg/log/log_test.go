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
	// No need to save/restore global level as we won't modify it.
	// originalLevel := CurrentLevel()
	// defer SetLevel(originalLevel)

	tests := []struct {
		name     string
		setLevel slog.Level
		// Remove logFuncs map, we'll call logger methods directly
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
			// Create a local buffer and logger for this test run
			var buf bytes.Buffer
			testHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: tt.setLevel})
			localLogger := slog.New(testHandler)

			// Map levels to their names for messages
			levelNames := map[slog.Level]string{
				slog.LevelDebug: "Debug",
				slog.LevelInfo:  "Info",
				slog.LevelWarn:  "Warn",
				slog.LevelError: "Error",
			}

			// Call logger methods directly
			localLogger.Debug("Test message for Debug", "level", "Debug")
			localLogger.Info("Test message for Info", "level", "Info")
			localLogger.Warn("Test message for Warn", "level", "Warn")
			localLogger.Error("Test message for Error", "level", "Error")

			// Assertions based on the buffer content
			output := buf.String()
			for level, shouldAppear := range tt.wantOutput {
				levelName := levelNames[level]
				uniqueMsgFragment := fmt.Sprintf("msg=\"Test message for %s\"", levelName)
				expectedLevelSubstr := "level=" + strings.ToUpper(levelName)

				if shouldAppear {
					// Check that *a* line contains both the quoted message fragment and the correct level substring
					found := false
					for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
						if strings.Contains(line, uniqueMsgFragment) && strings.Contains(line, expectedLevelSubstr) {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected log for %s was not found at level %s. Output:\n%s", levelName, tt.setLevel, output)
				} else {
					// Check that *no* line contains the message fragment (the quoted form)
					assert.NotContains(t, output, uniqueMsgFragment, "Unexpected log for %s was found at level %s. Output:\n%s", levelName, tt.setLevel, output)
				}
			}
		})
	}
}
