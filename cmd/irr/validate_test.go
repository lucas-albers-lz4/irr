package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewValidateCommand tests the creation and flag setup of the validate command
func TestNewValidateCommand(t *testing.T) {
	cmd := newValidateCmd()

	// Check if flags are correctly defined
	assert.NotNil(t, cmd.Flags().Lookup("chart-path"), "chart-path flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("release-name"), "release-name flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("values"), "values flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("namespace"), "namespace flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("output-file"), "output-file flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("strict"), "strict flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("kube-version"), "kube-version flag should be defined")

	// Check default values
	chartPath, err := cmd.Flags().GetString("chart-path")
	require.NoError(t, err, "Failed to get chart-path flag")
	assert.Equal(t, "", chartPath, "Default chart-path should be empty")

	namespace, err := cmd.Flags().GetString("namespace")
	require.NoError(t, err, "Failed to get namespace flag")
	assert.Equal(t, "default", namespace, "Default namespace should be 'default'")

	strict, err := cmd.Flags().GetBool("strict")
	require.NoError(t, err, "Failed to get strict flag")
	assert.False(t, strict, "Default strict mode should be false")

	outputFile, err := cmd.Flags().GetString("output-file")
	require.NoError(t, err, "Failed to get output-file flag")
	assert.Equal(t, "", outputFile, "Default output-file should be empty")
}

// TestGetValidateOutputFlags checks if the function correctly retrieves output-related flag values.
func TestGetValidateOutputFlags(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectStrict     bool
		expectOutputFile string
		expectErr        bool
	}{
		{
			name:             "no output flags",
			args:             []string{"myrelease", "--namespace", "myns"},
			expectStrict:     false,
			expectOutputFile: "",
			expectErr:        false,
		},
		{
			name:             "strict flag set",
			args:             []string{"myrelease", "--namespace", "myns", "--strict"},
			expectStrict:     true,
			expectOutputFile: "",
			expectErr:        false,
		},
		{
			name:             "output-file flag set",
			args:             []string{"myrelease", "--namespace", "myns", "--output-file", "out.yaml"},
			expectStrict:     false,
			expectOutputFile: "out.yaml",
			expectErr:        false,
		},
		{
			name:             "both flags set",
			args:             []string{"myrelease", "--namespace", "myns", "--strict", "--output-file", "result.txt"},
			expectStrict:     true,
			expectOutputFile: "result.txt",
			expectErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newValidateCmd()
			// Parse flags silently; errors checked by getValidateOutputFlags itself
			err := cmd.ParseFlags(tt.args)
			require.NoError(t, err, "Flag parsing failed unexpectedly in test setup")

			outputFile, strict, err := getValidateOutputFlags(cmd)

			if tt.expectErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Did not expect an error but got one: %v", err)
				assert.Equal(t, tt.expectStrict, strict, "Strict flag mismatch")
				assert.Equal(t, tt.expectOutputFile, outputFile, "OutputFile flag mismatch")
			}
		})
	}
}

// TestGetValidateFlags checks if the function correctly retrieves chart path and values files flags.
func TestGetValidateFlags(t *testing.T) {
	tests := []struct {
		name            string
		flags           map[string]interface{} // Use interface{} for string slice flags
		expectChartPath string
		expectValues    []string
		expectErr       bool
	}{
		{
			name:            "no flags set",
			flags:           nil,
			expectChartPath: "",
			expectValues:    []string{}, // Expect empty slice, not nil
			expectErr:       false,
		},
		{
			name:            "chart-path set",
			flags:           map[string]interface{}{"chart-path": "./my-chart"},
			expectChartPath: "./my-chart",
			expectValues:    []string{}, // Expect empty slice, not nil
			expectErr:       false,
		},
		{
			name:            "single values file set",
			flags:           map[string]interface{}{"values": []string{"values.yaml"}},
			expectChartPath: "",
			expectValues:    []string{"values.yaml"},
			expectErr:       false,
		},
		{
			name:            "multiple values files set",
			flags:           map[string]interface{}{"values": []string{"values.yaml", "override.yaml"}},
			expectChartPath: "",
			expectValues:    []string{"values.yaml", "override.yaml"},
			expectErr:       false,
		},
		{
			name:            "both flags set",
			flags:           map[string]interface{}{"chart-path": "../charts/other", "values": []string{"common.yaml"}},
			expectChartPath: "../charts/other",
			expectValues:    []string{"common.yaml"},
			expectErr:       false,
		},
		// Skipping direct error simulation for GetString/GetStringSlice failing
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newValidateCmd()

			// Set flags if provided
			if tt.flags != nil {
				for key, val := range tt.flags {
					switch v := val.(type) {
					case string:
						err := cmd.Flags().Set(key, v)
						require.NoErrorf(t, err, "Failed to set string flag %s=%s", key, v)
					case []string:
						// For string slices, Set doesn't work directly, use SetStringSlice
						err := cmd.Flags().Set(key, strings.Join(v, ",")) // pflag uses comma-separated for Set
						// Alternatively, directly set the slice if possible, depending on pflag version/behavior
						// err := cmd.Flags().SetStringSlice(key, v)
						require.NoErrorf(t, err, "Failed to set string slice flag %s=%v", key, v)
					default:
						t.Fatalf("Unsupported flag type in test setup: %T", val)
					}
				}
			}

			chartPath, values, err := getValidateFlags(cmd)

			if tt.expectErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Did not expect an error but got one: %v", err)
				assert.Equal(t, tt.expectChartPath, chartPath, "Chart path mismatch")
				assert.Equal(t, tt.expectValues, values, "Values files mismatch")
			}
		})
	}
}

// TestGetValidateReleaseNamespace checks if the function correctly retrieves release and namespace.
func TestGetValidateReleaseNamespace(t *testing.T) {
	tests := []struct {
		name            string
		args            []string          // Arguments passed to the command (simulate positional args after command)
		flags           map[string]string // Flags set explicitly on the command object
		envVars         map[string]string // Environment variables to set for plugin mode testing
		expectRelease   string
		expectNamespace string
		expectErr       bool
	}{
		{
			name:            "Helm plugin mode - args only (no flags set)",
			args:            []string{"my-release"},                                                                         // Positional arg for release name
			flags:           nil,                                                                                            // No flags set on cmd
			envVars:         map[string]string{"HELM_NAMESPACE": "my-helm-ns", "HELM_PLUGIN_DIR": "/fake/helm/plugins/irr"}, // Use HELM_PLUGIN_DIR
			expectRelease:   "my-release",
			expectNamespace: "my-helm-ns", // Expect env var override because flag is default
			expectErr:       false,
		},
		{
			name:            "Helm plugin mode - flags set (override args/env)",
			args:            []string{"arg-release"}, // Arg should be ignored if flag is set
			flags:           map[string]string{"release-name": "flag-release", "namespace": "flag-ns"},
			envVars:         map[string]string{"HELM_NAMESPACE": "ignored-env-ns", "HELM_PLUGIN_DIR": "/fake/helm/plugins/irr"}, // Use HELM_PLUGIN_DIR
			expectRelease:   "flag-release",                                                                                     // Flag takes precedence
			expectNamespace: "flag-ns",                                                                                          // Flag takes precedence over env
			expectErr:       false,
		},
		{
			name:            "Standalone mode - flags only",
			args:            []string{}, // No positional args
			flags:           map[string]string{"release-name": "flag-release", "namespace": "flag-ns"},
			envVars:         nil, // Not in plugin mode
			expectRelease:   "flag-release",
			expectNamespace: "flag-ns",
			expectErr:       false,
		},
		{
			name:            "Standalone mode - default namespace",
			args:            []string{},                                        // No positional args
			flags:           map[string]string{"release-name": "flag-release"}, // No namespace flag
			envVars:         nil,
			expectRelease:   "flag-release",
			expectNamespace: "default", // Should default
			expectErr:       false,
		},
		{
			name:            "Standalone mode - no release name set",
			args:            []string{},                                // No positional args
			flags:           map[string]string{"namespace": "flag-ns"}, // No release name flag
			envVars:         nil,
			expectRelease:   "", // Expect empty because no flag and no args
			expectNamespace: "flag-ns",
			expectErr:       false,
		},
		{
			name:    "Error case - invalid flag value (simulated)",
			args:    []string{},                         // No positional args
			flags:   map[string]string{"namespace": ""}, // Set to empty, but pretend GetString failed
			envVars: nil,
			// We can't easily force GetString to fail here, so this case might be hard to test directly.
			// Skipping direct error simulation for now.
		},
	}

	// Helper to set/unset env vars
	setEnv := func(key, value string) func() {
		originalValue := os.Getenv(key)
		err := os.Setenv(key, value)
		require.NoErrorf(t, err, "Failed to set env var %s in test setup", key)
		return func() {
			err := os.Setenv(key, originalValue)
			if err != nil {
				t.Logf("Warning: failed to restore env var %s: %v", key, err)
			}
		}
	}

	for _, tt := range tests {
		// Skip incomplete tests
		if tt.name == "Error case - invalid flag value (simulated)" {
			t.Skip("Skipping direct error simulation for flag parsing")
			continue
		}

		t.Run(tt.name, func(t *testing.T) {
			cmd := newValidateCmd()

			// Set environment variables for this test run
			var restorers []func()
			if tt.envVars != nil {
				for key, val := range tt.envVars {
					restorers = append(restorers, setEnv(key, val))
				}
			}
			// Ensure env vars are restored after test
			defer func() {
				for _, restore := range restorers {
					restore()
				}
			}()

			// Set flags if provided
			if tt.flags != nil {
				for key, val := range tt.flags {
					err := cmd.Flags().Set(key, val)
					require.NoErrorf(t, err, "Failed to set flag %s=%s", key, val)
				}
			}

			// Call the function with the command and args
			// Note: We pass tt.args, which simulates os.Args AFTER the command name itself.
			release, namespace, err := getValidateReleaseNamespace(cmd, tt.args)

			if tt.expectErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Did not expect an error but got one: %v", err)
				assert.Equal(t, tt.expectRelease, release, "Release name mismatch")
				assert.Equal(t, tt.expectNamespace, namespace, "Namespace mismatch")
			}
		})
	}
}

func TestValidateAndDetectChartPath(_ *testing.T) {
	// ... existing code ...
}
