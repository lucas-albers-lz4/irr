package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to capture stderr output for testing log functions
func captureStderr(t *testing.T, action func()) string {
	t.Helper()
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err, "Failed to create pipe for stderr capture")
	os.Stderr = w

	// Restore stderr in a deferred function
	defer func() {
		os.Stderr = oldStderr
		errClose := w.Close()
		if errClose != nil {
			// Log error during cleanup but don't fail the test here
			t.Logf("Warning: error closing stderr pipe writer: %v", errClose)
		}
	}()

	action() // Execute the function that writes to stderr

	// Close the writer *before* reading to signal EOF
	errCloseWriter := w.Close()
	require.NoError(t, errCloseWriter, "Failed to close stderr pipe writer before reading")

	// Read captured output
	var buf bytes.Buffer
	_, errCopy := io.Copy(&buf, r)
	require.NoError(t, errCopy, "Failed to read from stderr pipe reader")

	return buf.String()
}

func TestIsRunningAsHelmPlugin(t *testing.T) {
	testCases := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "No Helm env vars set",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name:     "HELM_PLUGIN_NAME set",
			envVars:  map[string]string{"HELM_PLUGIN_NAME": "irr"},
			expected: true,
		},
		{
			name:     "HELM_PLUGIN_DIR set",
			envVars:  map[string]string{"HELM_PLUGIN_DIR": "/path/to/plugins/irr"},
			expected: true,
		},
		{
			name:     "Both Helm env vars set",
			envVars:  map[string]string{"HELM_PLUGIN_NAME": "irr", "HELM_PLUGIN_DIR": "/path"},
			expected: true,
		},
		{
			name:     "Irrelevant env vars set",
			envVars:  map[string]string{"OTHER_VAR": "value"},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set environment variables for the test case
			originalEnv := make(map[string]string)
			for key, value := range tc.envVars {
				originalEnv[key] = os.Getenv(key)
				os.Setenv(key, value)
			}
			// Unset vars not in the test case to ensure clean state
			helmPluginVars := []string{"HELM_PLUGIN_NAME", "HELM_PLUGIN_DIR"}
			for _, key := range helmPluginVars {
				if _, exists := tc.envVars[key]; !exists {
					originalEnv[key] = os.Getenv(key)
					os.Unsetenv(key)
				}
			}

			// Restore original environment variables after the test
			defer func() {
				for key, value := range originalEnv {
					if value == "" {
						os.Unsetenv(key)
					} else {
						os.Setenv(key, value)
					}
				}
			}()

			// Call the function and assert the result
			assert.Equal(t, tc.expected, isRunningAsHelmPlugin())
		})
	}
}

func TestParseIrrDebugEnvVar(t *testing.T) {
	testCases := []struct {
		name     string
		envValue *string // Use pointer to differentiate between unset and empty string
		expected bool
	}{
		{"Unset", nil, false},
		{"EmptyString", stringPtr(""), false},
		{"TrueLowercase", stringPtr("true"), true},
		{"TrueUppercase", stringPtr("TRUE"), true},
		{"TrueMixedcase", stringPtr("True"), true},
		{"Number1", stringPtr("1"), true},
		{"YesLowercase", stringPtr("yes"), true},
		{"FalseLowercase", stringPtr("false"), false},
		{"Number0", stringPtr("0"), false},
		{"NoLowercase", stringPtr("no"), false},
		{"OtherString", stringPtr("other"), false},
	}

	originalEnv := os.Getenv("IRR_DEBUG")
	defer os.Setenv("IRR_DEBUG", originalEnv)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envValue == nil {
				os.Unsetenv("IRR_DEBUG")
			} else {
				os.Setenv("IRR_DEBUG", *tc.envValue)
			}
			assert.Equal(t, tc.expected, parseIrrDebugEnvVar())
		})
	}
}

// Helper for TestParseIrrDebugEnvVar
func stringPtr(s string) *string {
	return &s
}

func TestLogHelmEnvironment(t *testing.T) {
	// Set debug level for this test
	originalLevel := log.CurrentLevel()
	log.SetLevel(log.LevelDebug)
	defer log.SetLevel(originalLevel)

	// Set some Helm environment variables using t.Setenv for proper test cleanup
	testEnvVars := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_BIN":         "helm",
		"HELM_NAMESPACE":   "test-ns",
		"SOME_OTHER_VAR":   "ignore_me", // Should not be logged
	}
	for k, v := range testEnvVars {
		t.Setenv(k, v)
	}
	// Unset one variable to ensure it's not logged if empty
	t.Setenv("HELM_DEBUG", "")

	// Capture logs
	stopCapture := testutil.CaptureLogging()

	// Call the function
	logHelmEnvironment()

	// Get captured logs
	_, capturedStderr := stopCapture()

	// Assertions
	assert.Contains(t, capturedStderr, "Helm Environment Variables:")
	assert.Contains(t, capturedStderr, "HELM_PLUGIN_NAME=irr")
	assert.Contains(t, capturedStderr, "HELM_BIN=helm")
	assert.Contains(t, capturedStderr, "HELM_NAMESPACE=test-ns")
	assert.NotContains(t, capturedStderr, "SOME_OTHER_VAR") // Verify non-Helm vars are not logged
	assert.NotContains(t, capturedStderr, "HELM_DEBUG=")    // Verify empty vars are not logged
}
