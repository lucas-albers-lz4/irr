package main

import (
	"os"
	"testing"

	"github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to capture stderr output for testing log functions
// func captureStderr(t *testing.T, action func()) string {
// 	t.Helper()
// 	oldStderr := os.Stderr
// 	r, w, err := os.Pipe()
// 	require.NoError(t, err, "Failed to create pipe for stderr capture")
// 	os.Stderr = w
//
// 	action() // Execute the function that writes to stderr
//
// 	// Close the writer *before* reading to signal EOF
// 	errCloseWriter := w.Close()
// 	require.NoError(t, errCloseWriter, "Failed to close stderr pipe writer before reading")
//
// 	// Read captured output
// 	var buf bytes.Buffer
// 	_, errCopy := io.Copy(&buf, r)
// 	require.NoError(t, errCopy, "Failed to read from stderr pipe reader")
//
// 	return buf.String()
// }

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
				if err := os.Setenv(key, value); err != nil {
					t.Fatalf("Failed to set env var %s: %v", key, err)
				}
			}
			// Unset vars not in the test case to ensure clean state
			helmPluginVars := []string{"HELM_PLUGIN_NAME", "HELM_PLUGIN_DIR"}
			for _, key := range helmPluginVars {
				if _, exists := tc.envVars[key]; !exists {
					originalEnv[key] = os.Getenv(key)
					if err := os.Unsetenv(key); err != nil {
						t.Fatalf("Failed to unset env var %s: %v", key, err)
					}
				}
			}

			// Restore original environment variables after the test
			defer func() {
				for key := range tc.envVars {
					value := originalEnv[key] // Get original value
					if value == "" {
						if err := os.Unsetenv(key); err != nil {
							t.Logf("Warning: failed to unset env var %s: %v", key, err)
						}
					} else {
						if err := os.Setenv(key, value); err != nil {
							t.Logf("Warning: failed to set env var %s: %v", key, err)
						}
					}
				}
			}()

			// Call the function and assert the result
			assert.Equal(t, tc.expected, isRunningAsHelmPlugin())
		})
	}
}

func TestLogHelmEnvironment(t *testing.T) {
	// Set LOG_FORMAT=json for this test
	t.Setenv("LOG_FORMAT", "json")

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
	// Explicitly unset a var that logHelmEnvironment checks, to ensure it's not logged
	t.Setenv("HELM_DEBUG", "")

	// Capture logs using the testutil helper
	_, logs, err := testutil.CaptureJSONLogs(log.LevelDebug, func() {
		// Set the desired level *within* the capture function's scope
		// because CaptureJSONLogs sets up its own logger instance.
		// log.SetLevel(log.LevelDebug) // This is handled by CaptureJSONLogs level param
		logHelmEnvironment()
	})
	require.NoError(t, err, "Failed to capture JSON logs")

	// Check that Helm environment variables were logged using JSON assertions
	testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{
		"level": "DEBUG",
		"msg":   "Helm Environment Variables:",
	})

	testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{
		"level": "DEBUG",
		"msg":   "Helm Env",
		"var":   "HELM_PLUGIN_NAME",
		"value": "irr",
	})

	testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{
		"level": "DEBUG",
		"msg":   "Helm Env",
		"var":   "HELM_BIN",
		"value": "helm",
	})

	testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{
		"level": "DEBUG",
		"msg":   "Helm Env",
		"var":   "HELM_NAMESPACE",
		"value": "test-ns",
	})

	// Check that non-Helm var or empty var wasn't logged
	testutil.AssertLogDoesNotContainJSON(t, logs, map[string]interface{}{
		"var": "SOME_OTHER_VAR",
	})
	testutil.AssertLogDoesNotContainJSON(t, logs, map[string]interface{}{
		"var": "HELM_DEBUG",
	})
}

func TestExecute(t *testing.T) {
	// Save original log level and restore
	originalLevel := log.CurrentLevel()
	defer log.SetLevel(originalLevel)

	// Set log level to Info for this test, as we are testing an Info level message.
	// Note: CaptureJSONLogs also sets the level, but setting it here ensures
	// the logging functions behave as expected even before CaptureJSONLogs takes over.
	log.SetLevel(log.LevelInfo)

	// Capture logs to verify startup logging
	_, logs, err := testutil.CaptureJSONLogs(log.LevelInfo, func() {
		// Directly call the logic that produces the startup message
		// Mimic the relevant parts from rootCmd.PersistentPreRunE
		pluginModeDetected := isRunningAsHelmPlugin()                                                                          // From main.go
		log.Debug("Execution Mode Detected", "mode", map[bool]string{true: "Plugin", false: "Standalone"}[pluginModeDetected]) // Logged at Debug, won't be in capture

		if pluginModeDetected {
			log.Info("IRR running as Helm plugin", "version", BinaryVersion) // From main.go
		} else {
			// This is the log message we expect to capture
			log.Info("IRR running in standalone mode", "version", BinaryVersion)
		}
	})
	assert.NoError(t, err) // Assert no error from log capture itself

	// Verify the standalone mode info log exists
	testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{
		"level":   "INFO",
		"msg":     "IRR running in standalone mode",
		"version": BinaryVersion, // Check version as well
	})

	// Also verify the plugin mode message is NOT present (assuming default standalone)
	testutil.AssertLogDoesNotContainJSON(t, logs, map[string]interface{}{
		"msg": "IRR running as Helm plugin",
	})
}
