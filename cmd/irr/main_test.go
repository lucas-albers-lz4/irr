package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsRunningAsHelmPlugin(t *testing.T) {
	// Save original environment variables and restore them after tests
	originalHelmPluginName := os.Getenv("HELM_PLUGIN_NAME")
	originalHelmPluginDir := os.Getenv("HELM_PLUGIN_DIR")
	defer func() {
		err := os.Setenv("HELM_PLUGIN_NAME", originalHelmPluginName)
		require.NoError(t, err, "Failed to restore HELM_PLUGIN_NAME")
		err = os.Setenv("HELM_PLUGIN_DIR", originalHelmPluginDir)
		require.NoError(t, err, "Failed to restore HELM_PLUGIN_DIR")
	}()

	tests := []struct {
		name     string
		setup    func(t *testing.T)
		expected bool
	}{
		{
			name: "Neither env var set",
			setup: func(t *testing.T) {
				err := os.Unsetenv("HELM_PLUGIN_NAME")
				require.NoError(t, err, "Failed to unset HELM_PLUGIN_NAME")
				err = os.Unsetenv("HELM_PLUGIN_DIR")
				require.NoError(t, err, "Failed to unset HELM_PLUGIN_DIR")
			},
			expected: false,
		},
		{
			name: "Only HELM_PLUGIN_NAME set",
			setup: func(t *testing.T) {
				err := os.Setenv("HELM_PLUGIN_NAME", "irr")
				require.NoError(t, err, "Failed to set HELM_PLUGIN_NAME")
				err = os.Unsetenv("HELM_PLUGIN_DIR")
				require.NoError(t, err, "Failed to unset HELM_PLUGIN_DIR")
			},
			expected: true,
		},
		{
			name: "Only HELM_PLUGIN_DIR set",
			setup: func(t *testing.T) {
				err := os.Unsetenv("HELM_PLUGIN_NAME")
				require.NoError(t, err, "Failed to unset HELM_PLUGIN_NAME")
				err = os.Setenv("HELM_PLUGIN_DIR", "/some/path")
				require.NoError(t, err, "Failed to set HELM_PLUGIN_DIR")
			},
			expected: true,
		},
		{
			name: "Both env vars set",
			setup: func(t *testing.T) {
				err := os.Setenv("HELM_PLUGIN_NAME", "irr")
				require.NoError(t, err, "Failed to set HELM_PLUGIN_NAME")
				err = os.Setenv("HELM_PLUGIN_DIR", "/some/path")
				require.NoError(t, err, "Failed to set HELM_PLUGIN_DIR")
			},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup the environment for this test case
			tc.setup(t)

			// Call the function
			result := isRunningAsHelmPlugin()

			// Verify the result
			assert.Equal(t, tc.expected, result, "isRunningAsHelmPlugin() result mismatch")
		})
	}
}

func TestParseIrrDebugEnvVar(t *testing.T) {
	// Save original environment variable and restore it after the tests
	originalDebug := os.Getenv("IRR_DEBUG")
	defer func() {
		err := os.Setenv("IRR_DEBUG", originalDebug)
		require.NoError(t, err, "Failed to restore IRR_DEBUG")
	}()

	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{
			name:     "Environment variable not set",
			envValue: "",
			expected: false,
		},
		{
			name:     "Environment variable set to '1'",
			envValue: "1",
			expected: true,
		},
		{
			name:     "Environment variable set to 'true'",
			envValue: "true",
			expected: true,
		},
		{
			name:     "Environment variable set to 'TRUE'",
			envValue: "TRUE",
			expected: true,
		},
		{
			name:     "Environment variable set to 'yes'",
			envValue: "yes",
			expected: true,
		},
		{
			name:     "Environment variable set to 'YES'",
			envValue: "YES",
			expected: true,
		},
		{
			name:     "Environment variable set to other value",
			envValue: "other",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup the environment for this test case
			if tc.envValue == "" {
				err := os.Unsetenv("IRR_DEBUG")
				require.NoError(t, err, "Failed to unset IRR_DEBUG")
			} else {
				err := os.Setenv("IRR_DEBUG", tc.envValue)
				require.NoError(t, err, "Failed to set IRR_DEBUG")
			}

			// Call the function
			result := parseIrrDebugEnvVar()

			// Verify the result
			assert.Equal(t, tc.expected, result, "parseIrrDebugEnvVar() result mismatch")
		})
	}
}
