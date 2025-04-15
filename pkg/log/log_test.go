// Package log_test contains tests for the log package.
package log

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
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
		{name: "invalid", levelStr: "INVALID", want: currentLevel, wantErr: true},
		{name: "empty", levelStr: "", want: currentLevel, wantErr: true},
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

func TestLevelStringRepresentation(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{level: LevelDebug, want: "DEBUG"},
		{level: LevelInfo, want: "INFO"},
		{level: LevelWarn, want: "WARN"},
		{level: LevelError, want: "ERROR"},
		{level: Level(99), want: "UNKNOWN"}, // Test out of range value
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("Level.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetAndCurrentLevel(t *testing.T) {
	// Save the original level to restore after test
	originalLevel := currentLevel
	defer func() {
		currentLevel = originalLevel
	}()

	levels := []Level{LevelDebug, LevelInfo, LevelWarn, LevelError}
	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			// Redirect stderr to capture output
			oldStderr := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}
			os.Stderr = w

			SetLevel(level)

			// Close the writer to flush the output
			if err := w.Close(); err != nil {
				t.Logf("Failed to close pipe writer: %v", err)
			}
			os.Stderr = oldStderr

			// Read the captured output
			out, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("Failed to read from pipe: %v", err)
			}

			if currentLevel != level {
				t.Errorf("SetLevel() didn't set level correctly, got %v, want %v", currentLevel, level)
			}

			if CurrentLevel() != level {
				t.Errorf("CurrentLevel() = %v, want %v", CurrentLevel(), level)
			}

			// Check that the level was logged to stderr
			if !strings.Contains(string(out), level.String()) {
				t.Errorf("SetLevel() didn't log correctly: %s", string(out))
			}
		})
	}
}

func TestLevelBasedFiltering(t *testing.T) {
	// Save the original level to restore after test
	originalLevel := currentLevel
	defer func() {
		currentLevel = originalLevel
	}()

	tests := []struct {
		name       string
		setLevel   Level
		logFuncs   map[string]func(string, ...interface{})
		wantOutput map[string]bool // which log functions should produce output
	}{
		{
			name:     "debug level shows all logs",
			setLevel: LevelDebug,
			logFuncs: map[string]func(string, ...interface{}){
				"Debugf": Debugf,
				"Infof":  Infof,
				"Warnf":  Warnf,
				"Errorf": Errorf,
			},
			wantOutput: map[string]bool{
				"Debugf": true,
				"Infof":  true,
				"Warnf":  true,
				"Errorf": true,
			},
		},
		{
			name:     "info level hides debug logs",
			setLevel: LevelInfo,
			logFuncs: map[string]func(string, ...interface{}){
				"Debugf": Debugf,
				"Infof":  Infof,
				"Warnf":  Warnf,
				"Errorf": Errorf,
			},
			wantOutput: map[string]bool{
				"Debugf": false,
				"Infof":  true,
				"Warnf":  true,
				"Errorf": true,
			},
		},
		{
			name:     "warn level hides debug and info logs",
			setLevel: LevelWarn,
			logFuncs: map[string]func(string, ...interface{}){
				"Debugf": Debugf,
				"Infof":  Infof,
				"Warnf":  Warnf,
				"Errorf": Errorf,
			},
			wantOutput: map[string]bool{
				"Debugf": false,
				"Infof":  false,
				"Warnf":  true,
				"Errorf": true,
			},
		},
		{
			name:     "error level shows only error logs",
			setLevel: LevelError,
			logFuncs: map[string]func(string, ...interface{}){
				"Debugf": Debugf,
				"Infof":  Infof,
				"Warnf":  Warnf,
				"Errorf": Errorf,
			},
			wantOutput: map[string]bool{
				"Debugf": false,
				"Infof":  false,
				"Warnf":  false,
				"Errorf": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetLevel(tt.setLevel)

			for funcName, logFunc := range tt.logFuncs {
				// Redirect stderr to capture output
				oldStderr := os.Stderr
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatalf("Failed to create pipe: %v", err)
				}
				os.Stderr = w

				// Call the log function with a test message
				logFunc("Test message from %s", funcName)

				// Close the writer to flush the output
				if err := w.Close(); err != nil {
					t.Logf("Failed to close pipe writer: %v", err)
				}
				os.Stderr = oldStderr

				// Read the captured output
				out, err := io.ReadAll(r)
				if err != nil {
					t.Fatalf("Failed to read from pipe: %v", err)
				}
				hasOutput := len(out) > 0

				if hasOutput != tt.wantOutput[funcName] {
					if tt.wantOutput[funcName] {
						t.Errorf("%s() didn't produce output when it should have at level %v", funcName, tt.setLevel)
					} else {
						t.Errorf("%s() produced output when it shouldn't have at level %v: %s", funcName, tt.setLevel, string(out))
					}
				}
			}
		})
	}
}

func TestIsDebugEnabled(t *testing.T) {
	// Save the original level to restore after test
	originalLevel := currentLevel
	defer func() {
		currentLevel = originalLevel
	}()

	tests := []struct {
		name     string
		setLevel Level
		want     bool
	}{
		{name: "debug level enables debug", setLevel: LevelDebug, want: true},
		{name: "info level disables debug", setLevel: LevelInfo, want: false},
		{name: "warn level disables debug", setLevel: LevelWarn, want: false},
		{name: "error level disables debug", setLevel: LevelError, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetLevel(tt.setLevel)
			if got := IsDebugEnabled(); got != tt.want {
				t.Errorf("IsDebugEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
