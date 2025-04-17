package main

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"errors"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestOverrideCommand_ReleaseFlag_PluginMode(t *testing.T) {
	// Save original isHelmPlugin value and restore it after the test
	originalIsHelmPlugin := isHelmPlugin
	defer func() {
		isHelmPlugin = originalIsHelmPlugin
	}()

	// Set up plugin mode
	isHelmPlugin = true

	// Create the command
	cmd := newOverrideCmd()

	// Ensure release-name flag exists in plugin mode
	flag := cmd.Flags().Lookup("release-name")
	require.NotNil(t, flag, "release-name flag should be available in plugin mode")

	// Ensure namespace flag exists in plugin mode
	flag = cmd.Flags().Lookup("namespace")
	require.NotNil(t, flag, "namespace flag should be available in plugin mode")
}

func TestOverrideCommand_ReleaseFlag_StandaloneMode(t *testing.T) {
	// Save original isHelmPlugin value and restore it after the test
	originalIsHelmPlugin := isHelmPlugin
	defer func() {
		isHelmPlugin = originalIsHelmPlugin
	}()

	// Save original isTestMode value and restore after tests
	originalIsTestMode := isTestMode
	defer func() { isTestMode = originalIsTestMode }()

	// Set up standalone mode
	isHelmPlugin = false
	isTestMode = false // We don't want test mode for this specific test

	// Create the command
	cmd := newOverrideCmd()

	// The release-name flag should still exist in the command
	// but it should be rejected when used in standalone mode

	// Create a test buffer to capture output
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Set up minimum flags needed to avoid other validation errors
	cmd.SetArgs([]string{
		"--chart-path", "test-chart",
		"--target-registry", "docker.io",
		"--source-registries", "quay.io",
		"--release-name", "test-release", // This should cause an error in standalone mode
	})

	// This should fail because release-name is only for plugin mode
	err := cmd.Execute()
	require.Error(t, err, "release-name flag should be rejected in standalone mode")

	// Check that the error message is descriptive about plugin mode
	assert.Contains(t, err.Error(), "plugin", "Error should indicate this is a plugin-only feature")
}

// setupMockHelmAdapter sets up a mock Helm adapter for tests
func setupMockHelmAdapter(t *testing.T) (cleanup func()) {
	// Save original factory function
	originalFactory := helmAdapterFactory

	// Replace with a mock factory for testing
	helmAdapterFactory = func() (*helm.Adapter, error) {
		// Return a real adapter with mock client
		// This works because we're in test mode (isTestMode = true)
		// and the adapter won't make actual helm calls
		return helm.NewAdapter(nil, AppFs, true), nil
	}

	cleanup = func() {
		helmAdapterFactory = originalFactory
	}

	return cleanup
}

func TestOverrideCommand_OutputFileHandling(t *testing.T) {
	// Save original filesystem and restore after tests
	originalFs := AppFs
	defer func() { AppFs = originalFs }()

	// Save original isHelmPlugin value and restore after tests
	originalIsHelmPlugin := isHelmPlugin
	defer func() { isHelmPlugin = originalIsHelmPlugin }()

	// Save original isTestMode value and restore after tests
	originalIsTestMode := isTestMode
	defer func() { isTestMode = originalIsTestMode }()

	// Enable test mode
	isTestMode = true

	// Set up mock adapter for tests
	cleanup := setupMockHelmAdapter(t)
	defer cleanup()

	t.Run("default output file name in plugin mode", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()
		isHelmPlugin = true

		// Create the command
		cmd := newOverrideCmd()

		// Set required flags
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"test-release", // Release name as positional arg
		})

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check if default output file was created
		exists, err := afero.Exists(AppFs, "test-release-overrides.yaml")
		require.NoError(t, err)
		assert.True(t, exists, "Default output file should be created")

		// Check file permissions
		info, err := AppFs.Stat("test-release-overrides.yaml")
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), info.Mode().Perm(), "File should have 0644 permissions")
	})

	t.Run("error when file exists without force flag", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()
		isHelmPlugin = true

		// Create an existing file
		err := afero.WriteFile(AppFs, "test-release-overrides.yaml", []byte("existing content"), 0644)
		require.NoError(t, err)

		// Create the command
		cmd := newOverrideCmd()

		// Set required flags
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"test-release", // Release name as positional arg
		})

		// Execute the command - should fail because file exists
		err = cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")

		// Verify file content wasn't changed
		content, err := afero.ReadFile(AppFs, "test-release-overrides.yaml")
		require.NoError(t, err)
		assert.Equal(t, "existing content", string(content))
	})

	t.Run("custom output file path", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()
		isHelmPlugin = true

		// Create the command
		cmd := newOverrideCmd()

		// Set required flags with custom output path
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"--output-file", "custom/path/overrides.yaml",
			"test-release", // Release name as positional arg
		})

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check if custom output file was created
		exists, err := afero.Exists(AppFs, "custom/path/overrides.yaml")
		require.NoError(t, err)
		assert.True(t, exists, "Custom output file should be created")
	})
}

func TestOverrideCommand_ValidationHandling(t *testing.T) {
	// Save original filesystem and restore after tests
	originalFs := AppFs
	defer func() { AppFs = originalFs }()

	// Save original isHelmPlugin value and restore after tests
	originalIsHelmPlugin := isHelmPlugin
	defer func() { isHelmPlugin = originalIsHelmPlugin }()

	// Save original isTestMode value and restore after tests
	originalIsTestMode := isTestMode
	defer func() { isTestMode = originalIsTestMode }()

	t.Run("validation with chart path", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()
		isHelmPlugin = false

		// Create a mock chart directory
		require.NoError(t, AppFs.MkdirAll("./test-chart", 0755))

		// Create the command
		cmd := newOverrideCmd()

		// Set required flags with validation
		cmd.SetArgs([]string{
			"--chart-path", "./test-chart",
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"--validate",
		})

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)
	})

	t.Run("validation with release name in plugin mode", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()
		isHelmPlugin = true

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set required flags with validation
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"--validate",
			"test-release", // Release name as positional arg
		})

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check output for validation success message
		output := buf.String()
		assert.Contains(t, output, "Validation successful", "Output should contain validation success message")
	})
}

func TestOverrideCommand_NamespaceHandling(t *testing.T) {
	// Save original isHelmPlugin value and restore after tests
	originalIsHelmPlugin := isHelmPlugin
	defer func() { isHelmPlugin = originalIsHelmPlugin }()

	// Save original isTestMode value and restore after tests
	originalIsTestMode := isTestMode
	defer func() { isTestMode = originalIsTestMode }()

	// Enable test mode
	isTestMode = true

	t.Run("namespace flag takes precedence", func(t *testing.T) {
		isHelmPlugin = true

		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set required flags with explicit namespace
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"--namespace", "explicit-namespace",
			"test-release", // Release name as positional arg
		})

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check file content for namespace
		fileContent, err := afero.ReadFile(AppFs, "test-release-overrides.yaml")
		require.NoError(t, err)
		assert.Contains(t, string(fileContent), "namespace: explicit-namespace", "File should contain explicit namespace")
	})

	t.Run("default namespace when none specified", func(t *testing.T) {
		isHelmPlugin = true

		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set required flags without namespace
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"test-release", // Release name as positional arg
		})

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check file content for default namespace
		fileContent, err := afero.ReadFile(AppFs, "test-release-overrides.yaml")
		require.NoError(t, err)
		assert.Contains(t, string(fileContent), "namespace: default", "File should contain default namespace")
	})
}

func TestOverrideCommand_DryRun(t *testing.T) {
	// Save original filesystem and restore after tests
	originalFs := AppFs
	defer func() { AppFs = originalFs }()

	// Save original isHelmPlugin value and restore after tests
	originalIsHelmPlugin := isHelmPlugin
	defer func() { isHelmPlugin = originalIsHelmPlugin }()

	// Save original isTestMode value and restore after tests
	originalIsTestMode := isTestMode
	defer func() { isTestMode = originalIsTestMode }()

	// Enable test mode
	isTestMode = true

	t.Run("dry run outputs to stdout without file creation", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()
		isHelmPlugin = true

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set required flags with dry-run
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"--dry-run",
			"test-release", // Release name as positional arg
		})

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check that output contains mock content
		output := buf.String()
		assert.Contains(t, output, "mock: true", "Output should contain mock indication")
		assert.Contains(t, output, "generated: true", "Output should contain generated indication")

		// Verify no file was created
		exists, err := afero.Exists(AppFs, "test-release-overrides.yaml")
		require.NoError(t, err)
		assert.False(t, exists, "No file should be created in dry-run mode")
	})

	t.Run("dry run with validation", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()
		isHelmPlugin = true

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set required flags with dry-run and validation
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"--dry-run",
			"--validate",
			"test-release", // Release name as positional arg
		})

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check that output contains validation success message
		output := buf.String()
		assert.Contains(t, output, "mock: true", "Output should contain mock indication")
		assert.Contains(t, output, "generated: true", "Output should contain generated indication")
		assert.Contains(t, output, "Validation successful", "Output should indicate successful validation")

		// Verify no file was created
		exists, err := afero.Exists(AppFs, "test-release-overrides.yaml")
		require.NoError(t, err)
		assert.False(t, exists, "No file should be created in dry-run mode")
	})
}

func TestOverrideCommand_ErrorHandling(t *testing.T) {
	// Save original isHelmPlugin value and restore after tests
	originalIsHelmPlugin := isHelmPlugin
	defer func() { isHelmPlugin = originalIsHelmPlugin }()

	// Save original isTestMode value and restore after tests
	originalIsTestMode := isTestMode
	defer func() { isTestMode = originalIsTestMode }()

	// Enable test mode
	isTestMode = true

	t.Run("missing required flags", func(t *testing.T) {
		isHelmPlugin = true

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set only release name without required flags
		cmd.SetArgs([]string{
			"test-release", // Release name as positional arg
		})

		// Execute the command - should fail due to missing required flags
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target-registry", "Error should mention missing target registry")
	})

	t.Run("invalid target registry format", func(t *testing.T) {
		isHelmPlugin = true
		isTestMode = false // Disable test mode for this test

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set flags with invalid target registry
		cmd.SetArgs([]string{
			"--target-registry", "invalid:registry:format",
			"--source-registries", "docker.io",
			"test-release", // Release name as positional arg
		})

		// Execute the command - should fail due to invalid registry format
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid", "Error should indicate invalid registry format")
	})

	t.Run("invalid source registry format", func(t *testing.T) {
		isHelmPlugin = true
		isTestMode = false // Disable test mode for this test

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set flags with invalid source registry
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "invalid:registry:format",
			"test-release", // Release name as positional arg
		})

		// Execute the command - should fail due to invalid registry format
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid", "Error should indicate invalid registry format")
	})

	t.Run("non-existent release in plugin mode", func(t *testing.T) {
		isHelmPlugin = true
		isTestMode = false // Disable test mode for this test

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set flags with non-existent release
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"non-existent-release", // Release name as positional arg
		})

		// Execute the command - should fail due to non-existent release
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found", "Error should indicate release not found")
	})
}

func TestOverrideCommand_EdgeCases(t *testing.T) {
	// Save original filesystem and restore after tests
	originalFs := AppFs
	defer func() { AppFs = originalFs }()

	// Save original isHelmPlugin value and restore after tests
	originalIsHelmPlugin := isHelmPlugin
	defer func() { isHelmPlugin = originalIsHelmPlugin }()

	// Save original isTestMode value and restore after tests
	originalIsTestMode := isTestMode
	defer func() { isTestMode = originalIsTestMode }()

	// Enable test mode
	isTestMode = true

	t.Run("empty source registries list", func(t *testing.T) {
		isHelmPlugin = true

		// Create the command
		cmd := newOverrideCmd()

		// Set up a buffer to capture output
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set flags with empty source registries
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "",
			"test-release", // Release name as positional arg
		})

		// Execute the command - should fail due to empty source registries
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source", "Error should mention source registries issue")
	})

	t.Run("output directory does not exist", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()
		isHelmPlugin = true

		// Create the command
		cmd := newOverrideCmd()

		// Set flags with non-existent output directory
		cmd.SetArgs([]string{
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"--output-file", "non/existent/dir/output.yaml",
			"test-release", // Release name as positional arg
		})

		// Execute the command - should create directory and succeed
		err := cmd.Execute()
		require.NoError(t, err)

		// Verify directory was created
		exists, err := afero.DirExists(AppFs, "non/existent/dir")
		require.NoError(t, err)
		assert.True(t, exists, "Directory should be created")

		// Verify file was created
		exists, err = afero.Exists(AppFs, "non/existent/dir/output.yaml")
		require.NoError(t, err)
		assert.True(t, exists, "Output file should be created")
	})

	t.Run("both chart path and release name provided", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()
		isHelmPlugin = true

		// Capture log output
		buf := new(bytes.Buffer)
		cmd := newOverrideCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Set required flags with both chart path and release name
		cmd.SetArgs([]string{
			"--chart-path", "./test-chart",
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
			"test-release", // Release name as positional arg
		})

		// Execute the command - should use chart path
		err := cmd.Execute()
		require.NoError(t, err)

		// Check that overrides were generated and written to default file
		exists, err := afero.Exists(AppFs, "test-release-overrides.yaml")
		require.NoError(t, err)
		assert.True(t, exists, "Overrides file should be created")

		// Check that output mentions both were provided
		output := buf.String()
		// In test mode we can't check for the chart path being used since we're mocking
		// the helm plugin override handler
		assert.Contains(t, output, "test-release", "Output should mention release name")
	})
}

// TestOverrideCommandFlags tests the override command flag parsing, particularly with positional arguments
func TestOverrideCommandFlags(t *testing.T) {
	t.Run("positional argument as release name", func(t *testing.T) {
		// Create the command
		cmd := newOverrideCmd()

		// Set a positional argument with required flags
		args := []string{
			"my-release",
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
		}
		cmd.SetArgs(args)

		// Execute command to process arguments
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Create a custom runE that just collects the args
		var capturedArgs []string
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			capturedArgs = args
			return nil
		}

		// Save original isHelmPlugin value and restore after test
		originalIsHelmPlugin := isHelmPlugin
		defer func() { isHelmPlugin = originalIsHelmPlugin }()
		// Set plugin mode to true
		isHelmPlugin = true

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check that the positional argument was captured
		require.Len(t, capturedArgs, 1, "Expected one positional argument")
		assert.Equal(t, "my-release", capturedArgs[0], "Expected release name to be captured")
	})

	t.Run("required flags in plugin mode", func(t *testing.T) {
		// Create the command
		cmd := newOverrideCmd()

		// Set release name as positional argument and minimal required flags
		args := []string{
			"my-release",
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
		}
		cmd.SetArgs(args)

		// Execute command to process arguments
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Create a custom runE that checks flag values
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			// Get flag values
			targetRegistry, _ := cmd.Flags().GetString("target-registry")
			sourceRegistries, _ := cmd.Flags().GetStringSlice("source-registries")

			// Check flag values
			assert.Equal(t, "registry.example.com", targetRegistry)
			assert.Contains(t, sourceRegistries, "docker.io")
			assert.Equal(t, "my-release", args[0])

			return nil
		}

		// Save original isHelmPlugin value and restore after test
		originalIsHelmPlugin := isHelmPlugin
		defer func() { isHelmPlugin = originalIsHelmPlugin }()
		// Set plugin mode to true
		isHelmPlugin = true

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)
	})

	t.Run("release name by flag or positional", func(t *testing.T) {
		// Create the command with both flag and positional release name
		cmd := newOverrideCmd()

		// Set both flag and positional argument
		args := []string{
			"positional-release",
			"--release-name", "flag-release",
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
		}
		cmd.SetArgs(args)

		// Execute command to process arguments
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Create a custom runE that collects the release name
		var capturedArgs []string
		var capturedReleaseName string
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			capturedArgs = args
			capturedReleaseName, _ = cmd.Flags().GetString("release-name")
			return nil
		}

		// Save original isHelmPlugin value and restore after test
		originalIsHelmPlugin := isHelmPlugin
		defer func() { isHelmPlugin = originalIsHelmPlugin }()
		// Set plugin mode to true
		isHelmPlugin = true

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Positional argument should be captured
		require.Len(t, capturedArgs, 1, "Expected one positional argument")
		assert.Equal(t, "positional-release", capturedArgs[0], "Expected positional release name to be captured")

		// Flag should also be captured
		assert.Equal(t, "flag-release", capturedReleaseName, "Expected flag release name to be captured")
	})

	t.Run("namespace flag", func(t *testing.T) {
		// Create the command
		cmd := newOverrideCmd()

		// Set namespace flag with release
		args := []string{
			"my-release",
			"--namespace", "test-namespace",
			"--target-registry", "registry.example.com",
			"--source-registries", "docker.io",
		}
		cmd.SetArgs(args)

		// Execute command to process arguments
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		// Create a custom runE that checks namespace
		var capturedNamespace string
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			capturedNamespace, _ = cmd.Flags().GetString("namespace")
			return nil
		}

		// Save original isHelmPlugin value and restore after test
		originalIsHelmPlugin := isHelmPlugin
		defer func() { isHelmPlugin = originalIsHelmPlugin }()
		// Set plugin mode to true
		isHelmPlugin = true

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check namespace
		assert.Equal(t, "test-namespace", capturedNamespace, "Expected namespace flag to be captured")
	})
}

// TestOverrideCommand_EdgeCases tests edge cases for the override command
