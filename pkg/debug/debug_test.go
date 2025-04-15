// Package debug_test contains tests for the debug package.
package debug

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInitWithMockedEnvironment(t *testing.T) {
	// Save original state to restore later
	origEnabled := Enabled
	origStartTime := startTime
	origShowDebugEnvWarnings := ShowDebugEnvWarnings
	defer func() {
		Enabled = origEnabled
		startTime = origStartTime
		ShowDebugEnvWarnings = origShowDebugEnvWarnings
		if err := os.Unsetenv("IRR_DEBUG"); err != nil {
			t.Logf("Failed to unset IRR_DEBUG: %v", err)
		}
	}()

	tests := []struct {
		name          string
		envValue      string
		forceEnable   bool
		expectEnabled bool
		shouldLog     bool
	}{
		{
			name:          "force enable overrides env var",
			envValue:      "false",
			forceEnable:   true,
			expectEnabled: true,
			shouldLog:     true,
		},
		{
			name:          "env var true enables debug",
			envValue:      "true",
			forceEnable:   false,
			expectEnabled: true,
			shouldLog:     true,
		},
		{
			name:          "env var false disables debug",
			envValue:      "false",
			forceEnable:   false,
			expectEnabled: false,
			shouldLog:     false,
		},
		{
			name:          "invalid env var defaults to false",
			envValue:      "not-a-bool",
			forceEnable:   false,
			expectEnabled: false,
			shouldLog:     false,
		},
		{
			name:          "unset env var defaults to false",
			envValue:      "",
			forceEnable:   false,
			expectEnabled: false,
			shouldLog:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			if tt.envValue == "" {
				if err := os.Unsetenv("IRR_DEBUG"); err != nil {
					t.Fatalf("Failed to unset IRR_DEBUG: %v", err)
				}
			} else {
				if err := os.Setenv("IRR_DEBUG", tt.envValue); err != nil {
					t.Fatalf("Failed to set IRR_DEBUG: %v", err)
				}
			}

			// Reset global state
			Enabled = false
			startTime = time.Time{}
			ShowDebugEnvWarnings = false

			// Capture stderr
			oldStderr := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}
			os.Stderr = w

			// Call the function under test
			Init(tt.forceEnable)

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
			output := string(out)

			// Check if Enabled state is set correctly
			if Enabled != tt.expectEnabled {
				t.Errorf("Expected Enabled to be %v, but got %v", tt.expectEnabled, Enabled)
			}

			// Verify if startTime was set
			if startTime.IsZero() {
				t.Error("startTime was not set")
			}

			// Check if debug message was logged or not
			hasOutput := output != ""
			containsDebugEnabled := strings.Contains(output, "Debug logging enabled")

			if tt.shouldLog && !containsDebugEnabled {
				t.Errorf("Expected debug enabled message, but got none")
			}
			if !tt.shouldLog && hasOutput {
				t.Errorf("Expected no output, but got: %s", output)
			}
		})
	}
}

func TestDebugStateToggling(t *testing.T) {
	// Save original state to restore later
	origEnabled := Enabled
	origStartTime := startTime
	defer func() {
		Enabled = origEnabled
		startTime = origStartTime
	}()

	// Test toggling through direct assignment
	testCases := []bool{true, false, true}
	for i, state := range testCases {
		t.Run(fmt.Sprintf("toggle-%d", i), func(t *testing.T) {
			Enabled = state
			if Enabled != state {
				t.Errorf("Failed to set debug state to %v", state)
			}
		})
	}

	// Test toggle through Init function
	cases := []struct {
		force  bool
		envVal string
		want   bool
	}{
		{force: true, envVal: "", want: true},
		{force: false, envVal: "true", want: true},
		{force: false, envVal: "false", want: false},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("init-toggle-%d", i), func(t *testing.T) {
			// Setup
			if c.envVal == "" {
				if err := os.Unsetenv("IRR_DEBUG"); err != nil {
					t.Fatalf("Failed to unset IRR_DEBUG: %v", err)
				}
			} else {
				if err := os.Setenv("IRR_DEBUG", c.envVal); err != nil {
					t.Fatalf("Failed to set IRR_DEBUG: %v", err)
				}
			}
			defer func() {
				if err := os.Unsetenv("IRR_DEBUG"); err != nil {
					t.Logf("Failed to unset IRR_DEBUG in defer: %v", err)
				}
			}()

			// Reset state
			Enabled = !c.want // set to opposite of expected to ensure change
			startTime = time.Time{}

			// Capture stderr to prevent output during test
			oldStderr := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}
			defer func() {
				if err := w.Close(); err != nil {
					t.Logf("Failed to close pipe writer: %v", err)
				}
				os.Stderr = oldStderr
				if _, err := io.ReadAll(r); err != nil {
					t.Logf("Failed to read pipe: %v", err)
				}
			}()
			os.Stderr = w

			// Call function
			Init(c.force)

			// Verify
			if Enabled != c.want {
				t.Errorf("Init(%v) with IRR_DEBUG=%q: want Enabled=%v, got %v",
					c.force, c.envVal, c.want, Enabled)
			}
		})
	}
}

func TestOutputFunctions(t *testing.T) {
	// Save original state to restore later
	origEnabled := Enabled
	origStartTime := startTime
	defer func() {
		Enabled = origEnabled
		startTime = origStartTime
	}()

	// Enable debug and set a fixed start time for predictable output
	Enabled = true
	startTime = time.Now()

	tests := []struct {
		name     string
		fn       func()
		expected []string // Fragments that should be in the output
	}{
		{
			name: "Printf",
			fn: func() {
				Printf("Test message with %s", "formatting")
			},
			expected: []string{"[DEBUG", "Test message with formatting"},
		},
		{
			name: "Println",
			fn: func() {
				Println("Test", "println", "message")
			},
			expected: []string{"[DEBUG", "Test println message"},
		},
		{
			name: "DumpValue",
			fn: func() {
				DumpValue("testMap", map[string]string{"key": "value"})
			},
			expected: []string{"[DEBUG", "Dump testMap:", `"key": "value"`},
		},
		{
			name: "FunctionEnter",
			fn: func() {
				FunctionEnter("TestFunction")
			},
			expected: []string{"[DEBUG", "Enter: TestFunction"},
		},
		{
			name: "FunctionExit",
			fn: func() {
				FunctionExit("TestFunction")
			},
			expected: []string{"[DEBUG", "Exit: TestFunction"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stderr
			oldStderr := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}
			os.Stderr = w

			// Call the function under test
			tt.fn()

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
			output := string(out)

			// Check for expected fragments in the output
			for _, fragment := range tt.expected {
				if !strings.Contains(output, fragment) {
					t.Errorf("Expected output to contain %q, but got: %s", fragment, output)
				}
			}
		})
	}

	// Test with debug disabled
	t.Run("disabled", func(t *testing.T) {
		// Disable debug
		Enabled = false

		// Capture stderr
		oldStderr := os.Stderr
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("Failed to create pipe: %v", err)
		}
		os.Stderr = w

		// Call all output functions
		Printf("This should not appear")
		Println("This should not appear either")
		DumpValue("noOutput", map[string]string{"key": "value"})
		FunctionEnter("NoOutputFunction")
		FunctionExit("NoOutputFunction")

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
		output := string(out)

		// There should be no output
		if output != "" {
			t.Errorf("Expected no output when debug is disabled, but got: %s", output)
		}
	})
}

func TestEnableDebugEnvVarWarnings(t *testing.T) {
	// Save original state to restore later
	origShowDebugEnvWarnings := ShowDebugEnvWarnings
	origEnabled := Enabled
	defer func() {
		ShowDebugEnvWarnings = origShowDebugEnvWarnings
		Enabled = origEnabled
		if err := os.Unsetenv("IRR_DEBUG"); err != nil {
			t.Logf("Failed to unset IRR_DEBUG: %v", err)
		}
	}()

	// Set an invalid debug value
	if err := os.Setenv("IRR_DEBUG", "invalid-value"); err != nil {
		t.Fatalf("Failed to set IRR_DEBUG: %v", err)
	}

	// Test without warnings enabled
	ShowDebugEnvWarnings = false
	Enabled = true // Reset to ensure we see the change

	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Call Init with invalid env var
	Init(false)

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
	output := string(out)

	// Should be no warning
	if strings.Contains(output, "Warning: Invalid boolean value") {
		t.Errorf("Expected no warning, but got: %s", output)
	}

	// Now test with warnings enabled
	ShowDebugEnvWarnings = true
	Enabled = true // Reset to ensure we see the change

	// Capture stderr again
	r, w, err = os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Call Init with invalid env var
	Init(false)

	// Close the writer to flush the output
	if err := w.Close(); err != nil {
		t.Logf("Failed to close pipe writer: %v", err)
	}
	os.Stderr = oldStderr

	// Read the captured output
	out, err = io.ReadAll(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	output = string(out)

	// Should see a warning
	if !strings.Contains(output, "Warning: Invalid boolean value") {
		t.Errorf("Expected warning about invalid IRR_DEBUG value, but got: %s", output)
	}

	// Verify that EnableDebugEnvVarWarnings works
	ShowDebugEnvWarnings = false
	EnableDebugEnvVarWarnings()
	if !ShowDebugEnvWarnings {
		t.Error("EnableDebugEnvVarWarnings didn't set ShowDebugEnvWarnings to true")
	}
}
