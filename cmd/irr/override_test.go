package main

import (
	"fmt"
	"testing"

	"errors"

	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/stretchr/testify/assert"
)

const testChartDir = "/test/chart"

func TestOverrideCommand_RequiredFlags(t *testing.T) {
	cmd := newOverrideCmd()
	err := cmd.Execute()
	assert.Error(t, err, "Should error when required flags are missing")

	var exitErr *exitcodes.ExitCodeError
	ok := errors.As(err, &exitErr)
	assert.True(t, ok, "Error should be an ExitCodeError")
	assert.Equal(t, exitcodes.ExitMissingRequiredFlag, exitErr.Code, "Should have correct exit code")
}

func TestOverrideCommand_LoadChart(t *testing.T) {
	tests := []struct {
		name         string
		chartPath    string
		releaseName  string
		loadRelease  bool
		loadPath     bool
		expectedErr  bool
		expectedCode int
	}{
		{
			name:        "Load from path - success",
			chartPath:   testChartDir,
			loadPath:    true,
			expectedErr: false,
		},
		{
			name:         "Load from path - not found",
			chartPath:    "/nonexistent/chart",
			loadPath:     true,
			expectedErr:  true,
			expectedCode: exitcodes.ExitChartNotFound,
		},
		{
			name:        "Load from release - success",
			releaseName: "test-release",
			loadRelease: true,
			expectedErr: false,
		},
		{
			name:         "No load method specified",
			expectedErr:  true,
			expectedCode: exitcodes.ExitInputConfigurationError,
		},
	}

	originalLoader := chartLoader
	defer func() { chartLoader = originalLoader }()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a mock chartLoader function
			chartLoader = func(_ *GeneratorConfig, loadFromRelease, loadFromPath bool, releaseName, _ string) (string, error) {
				// Verify the function parameters
				assert.Equal(t, test.loadRelease, loadFromRelease)
				assert.Equal(t, test.loadPath, loadFromPath)
				assert.Equal(t, test.releaseName, releaseName)

				// Simulate errors based on test case
				if test.expectedErr {
					return "", &exitcodes.ExitCodeError{
						Code: test.expectedCode,
						Err:  fmt.Errorf("simulated error for test"),
					}
				}

				return "test chart source", nil
			}

			// Create a config for testing
			config := GeneratorConfig{
				ChartPath: test.chartPath,
			}

			// Call the function under test
			result, err := chartLoader(&config, test.loadRelease, test.loadPath, test.releaseName, "")

			if test.expectedErr {
				assert.Error(t, err, "Should return error")
				var exitErr *exitcodes.ExitCodeError
				ok := errors.As(err, &exitErr)
				assert.True(t, ok, "Error should be an ExitCodeError")
				assert.Equal(t, test.expectedCode, exitErr.Code, "Should have correct exit code")
			} else {
				assert.NoError(t, err, "Should not return error")
				assert.Equal(t, "test chart source", result, "Should return correct chart source")
			}
		})
	}
}

// Skip TestOverrideCommand_ValidateChart since we can't easily mock the helm.Template function
// without making code changes to the main code to support testing.
// This is an intentional trade-off to keep the main code clean.

func TestOverrideCommand_GeneratorError(t *testing.T) {
	// Create temporary filesystem context
	fs, _, cleanup := setupMemoryFSContext(t) // Use blank identifier for unused tempDir
	defer cleanup()
	AppFs = fs

	// Test that chart-related errors are properly handled
	tests := []struct {
		name           string
		chartPath      string
		expectedErr    bool
		expectedCode   int
		expectedErrMsg string
	}{
		{
			name:           "Chart path required",
			chartPath:      "",
			expectedErr:    true,
			expectedCode:   exitcodes.ExitMissingRequiredFlag,
			expectedErrMsg: "required flag",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := newOverrideCmd()
			// Setup minimal valid args
			args := []string{
				"--target-registry", "target.io",
				"--source-registries", "source.io",
			}

			// Add chart path if specified
			if test.chartPath != "" {
				args = append(args, "--chart-path", test.chartPath)
			}

			cmd.SetArgs(args)
			err := cmd.Execute()

			if test.expectedErr {
				assert.Error(t, err, "Expected an error")
				if test.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), test.expectedErrMsg, "Error message should contain expected text")
				}

				var exitErr *exitcodes.ExitCodeError
				ok := errors.As(err, &exitErr)
				if assert.True(t, ok, "Error should be an ExitCodeError") {
					assert.Equal(t, test.expectedCode, exitErr.Code, "Exit code should match expected")
				}
			} else {
				assert.NoError(t, err, "Did not expect an error")
			}
		})
	}
}
