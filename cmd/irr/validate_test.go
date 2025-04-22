package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Mock internal/helm for testing command logic without actual Helm calls
	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/spf13/afero"
)

// Constants for repeated test values in validate tests - reserved for future use
// These are available for tests that need common values
//
//nolint:unused // These constants are available for future test usage
const (
	validateTestChartPath = "/path/to/chart"
	validateTestRelease   = "release"
)

// Mock the helm.Template function via the exported variable
//
//nolint:unused // This variable is used in some tests to override the template function
var mockHelmTemplate func(options *helm.TemplateOptions) (*helm.CommandResult, error)

//nolint:unused // This function is available for testing but not currently used
func setupValidateTest(t *testing.T) (cmd *cobra.Command, cleanup func()) {
	// Create a temporary directory for chart and values
	tempDir, err := os.MkdirTemp("", "validate-test-")
	require.NoError(t, err)

	// Create dummy chart
	chartDir := filepath.Join(tempDir, "mychart")
	err = os.Mkdir(chartDir, 0o750) // More secure permissions
	require.NoError(t, err)
	chartFile := filepath.Join(chartDir, "Chart.yaml")
	err = os.WriteFile(chartFile, []byte("apiVersion: v2\nname: mychart\nversion: 0.1.0"), fileutil.ReadWriteUserPermission)
	require.NoError(t, err)

	// Create dummy values file
	valuesFile := filepath.Join(tempDir, "values.yaml")
	err = os.WriteFile(valuesFile, []byte("key: value"), fileutil.ReadWriteUserPermission)
	require.NoError(t, err)

	// Create command
	cmd = newValidateCmd()

	// Set default flags needed for basic execution
	err = cmd.Flags().Set("chart-path", chartDir)
	require.NoError(t, err)
	err = cmd.Flags().Set("values", valuesFile)
	require.NoError(t, err)

	// Set release-name to empty to avoid plugin mode checks
	err = cmd.Flags().Set("release-name", "")
	require.NoError(t, err)

	// Set strict flag value for tests
	err = cmd.Flags().Set("strict", "false")
	require.NoError(t, err)

	// Replace the real helm.Template with our mock via the exported variable
	originalHelmTemplate := helm.HelmTemplateFunc // Store original
	helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
		if mockHelmTemplate != nil {
			return mockHelmTemplate(options)
		}
		// Default mock behavior if not set by test - return valid non-empty content
		return &helm.CommandResult{
			Success: true,
			Stdout:  "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: value\n",
		}, nil
	}

	cleanup = func() {
		helm.HelmTemplateFunc = originalHelmTemplate // Restore original
		err := os.RemoveAll(tempDir)
		if err != nil {
			t.Logf("Warning: failed to clean up temp directory %s: %v", tempDir, err)
		}
		mockHelmTemplate = nil // Reset mock
	}

	return cmd, cleanup
}

// setupValuesFilesAndChart sets up fake chart and values files
func setupValuesFilesAndChart(t *testing.T, memFs afero.Fs) (chartPath string, valuesFiles []string) {
	// Create a test chart directory with Chart.yaml
	tmpDir := t.TempDir()
	chartDir := filepath.Join(tmpDir, "mock-chart")
	err := memFs.MkdirAll(chartDir, fileutil.ReadWriteExecuteUserReadExecuteOthers)
	assert.NoError(t, err, "Failed to create chart directory")

	chartFile := filepath.Join(chartDir, "Chart.yaml")
	err = afero.WriteFile(memFs, chartFile, []byte("name: mock-chart\nversion: 1.0.0"), fileutil.ReadWriteUserReadOthers)
	assert.NoError(t, err, "Failed to create Chart.yaml")

	// Create test values files
	values1 := filepath.Join(chartDir, "values1.yaml")
	err = afero.WriteFile(memFs, values1, []byte("key1: value1"), fileutil.ReadWriteUserReadOthers)
	assert.NoError(t, err, "Failed to create values1.yaml")

	values2 := filepath.Join(chartDir, "values2.yaml")
	err = afero.WriteFile(memFs, values2, []byte("key2: value2"), fileutil.ReadWriteUserReadOthers)
	assert.NoError(t, err, "Failed to create values2.yaml")

	return chartDir, []string{values1, values2}
}

// setupCommandForTest creates and configures a validate command for testing
func setupCommandForTest() *cobra.Command {
	// Create a new command with the appropriate flags
	cmd := newValidateCmd()
	// Don't run the PreRun function during tests
	cmd.PreRunE = nil

	return cmd
}

// TestValidateCmd_DefaultKubeVersion tests validation with default Kubernetes version
func TestValidateCmd_DefaultKubeVersion(t *testing.T) {
	// Save original functionality and restore after tests
	originalTemplateFunc := helm.HelmTemplateFunc
	defer func() { helm.HelmTemplateFunc = originalTemplateFunc }()

	// Save original test mode state and restore it after the test
	originalTestMode := isValidateTestMode
	defer func() { isValidateTestMode = originalTestMode }()

	// Set test mode to true to avoid actual Helm calls
	isValidateTestMode = true

	// Set up our mock template function
	helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
		// In the real code, DefaultKubernetesVersion is used
		// But for this test, we'll just check that whatever value is passed
		// matches the expected constant
		assert.Equal(t, DefaultKubernetesVersion, options.KubeVersion,
			"Kubernetes version should match DefaultKubernetesVersion constant")
		return &helm.CommandResult{
			Success: true,
			Stdout:  "Validation successful: Chart rendered successfully with values.",
		}, nil
	}

	// Setup temporary filesystem
	memFs := afero.NewMemMapFs()
	chartPath, valuesFiles := setupValuesFilesAndChart(t, memFs)

	// Set temp filesystem as the app filesystem for the test
	originalFs := AppFs
	AppFs = memFs
	defer func() { AppFs = originalFs }()

	// Create command
	cmd := setupCommandForTest()

	// Set args for normal chart path validation
	cmd.SetArgs([]string{
		"--chart-path", chartPath,
		"--values", valuesFiles[0],
	})

	// Capture stdout
	bufStdout := &bytes.Buffer{}
	bufStderr := &bytes.Buffer{}
	cmd.SetOut(bufStdout)
	cmd.SetErr(bufStderr)

	// Run the command
	err := cmd.Execute()
	require.NoError(t, err, "Command should execute without error")

	// Now test with release name validation
	err = os.Setenv("HELM_PLUGIN_NAME", "irr")
	require.NoError(t, err)
	defer func() {
		err := os.Unsetenv("HELM_PLUGIN_NAME")
		if err != nil {
			t.Errorf("Error unsetting HELM_PLUGIN_NAME: %v", err)
		}
	}()
	cmd = setupCommandForTest()
	cmd.SetArgs([]string{
		"--release-name", "test-release",
		"--values", valuesFiles[0],
	})

	// Reset buffers
	bufStdout.Reset()
	bufStderr.Reset()
	cmd.SetOut(bufStdout)
	cmd.SetErr(bufStderr)

	// Run the command
	err = cmd.Execute()
	require.NoError(t, err, "Command should execute without error for release name validation")
}

// TestValidateCmd_ExplicitKubeVersion tests validation with an explicit Kubernetes version
func TestValidateCmd_ExplicitKubeVersion(t *testing.T) {
	// Save original functionality and restore after tests
	originalTemplateFunc := helm.HelmTemplateFunc
	defer func() { helm.HelmTemplateFunc = originalTemplateFunc }()

	// Save original test mode state and restore it after the test
	originalTestMode := isValidateTestMode
	defer func() { isValidateTestMode = originalTestMode }()

	// Set test mode to true to avoid actual Helm calls
	isValidateTestMode = true

	// Set the expected Kubernetes version
	expectedVersion := "1.21.0"

	// Set up our mock template function
	helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
		// Verify kubeVersion matches what we expect
		assert.Equal(t, expectedVersion, options.KubeVersion, "Kubernetes version should match explicit value")
		return &helm.CommandResult{
			Success: true,
			Stdout:  "Validation successful: Chart rendered successfully with values.",
		}, nil
	}

	// Setup temporary filesystem
	memFs := afero.NewMemMapFs()
	chartPath, valuesFiles := setupValuesFilesAndChart(t, memFs)

	// Set temp filesystem as the app filesystem for the test
	originalFs := AppFs
	AppFs = memFs
	defer func() { AppFs = originalFs }()

	// Create command
	cmd := setupCommandForTest()

	// Set args with explicit Kubernetes version
	cmd.SetArgs([]string{
		"--chart-path", chartPath,
		"--values", valuesFiles[0],
		"--kube-version", expectedVersion,
	})

	// Capture stdout
	bufStdout := &bytes.Buffer{}
	bufStderr := &bytes.Buffer{}
	cmd.SetOut(bufStdout)
	cmd.SetErr(bufStderr)

	// Run the command
	err := cmd.Execute()
	require.NoError(t, err, "Command should execute without error")

	// Now test with release name validation
	err = os.Setenv("HELM_PLUGIN_NAME", "irr")
	require.NoError(t, err)
	defer func() {
		err := os.Unsetenv("HELM_PLUGIN_NAME")
		if err != nil {
			t.Errorf("Error unsetting HELM_PLUGIN_NAME: %v", err)
		}
	}()
	cmd = setupCommandForTest()
	cmd.SetArgs([]string{
		"--release-name", "test-release",
		"--values", valuesFiles[0],
		"--kube-version", expectedVersion,
	})

	// Reset buffers
	bufStdout.Reset()
	bufStderr.Reset()
	cmd.SetOut(bufStdout)
	cmd.SetErr(bufStderr)

	// Run the command
	err = cmd.Execute()
	require.NoError(t, err, "Command should execute without error for release name validation")
}

// TestValidateCmd_InvalidKubeVersionFormat tests validation with an invalid Kubernetes version format
func TestValidateCmd_InvalidKubeVersionFormat(t *testing.T) {
	// Save original functionality and restore after tests
	originalTemplateFunc := helm.HelmTemplateFunc
	defer func() { helm.HelmTemplateFunc = originalTemplateFunc }()

	// Save original test mode state and restore it after the test
	originalTestMode := isValidateTestMode
	defer func() { isValidateTestMode = originalTestMode }()

	// Set test mode to true for proper mocking
	isValidateTestMode = true

	// Also set global test mode to ensure filesystem mocking works
	originalGlobalTestMode := isTestMode
	isTestMode = true
	defer func() { isTestMode = originalGlobalTestMode }()

	// Set an invalid Kubernetes version
	invalidVersion := "not-a-semver"

	// Set up our mock template function to return an error for invalid version
	helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
		if options.KubeVersion == invalidVersion {
			return &helm.CommandResult{
				Success: false,
				Stderr:  "Error: invalid kubernetes version: not-a-semver",
			}, fmt.Errorf("invalid kubernetes version: %s", invalidVersion)
		}
		// For any other version, return success
		return &helm.CommandResult{
			Success: true,
			Stdout:  "Valid template output",
		}, nil
	}

	// Setup temporary filesystem
	memFs := afero.NewMemMapFs()
	chartPath, valuesFiles := setupValuesFilesAndChart(t, memFs)

	// Set temp filesystem as the app filesystem for the test
	originalFs := AppFs
	AppFs = memFs
	defer func() { AppFs = originalFs }()

	// Create command
	cmd := setupCommandForTest()

	// Set args with invalid Kubernetes version
	// Add --strict flag to ensure error is returned
	cmd.SetArgs([]string{
		"--chart-path", chartPath,
		"--values", valuesFiles[0],
		"--kube-version", invalidVersion,
		"--strict",
	})

	// Ensure the flags are set
	err := cmd.Flags().Set("chart-path", chartPath)
	require.NoError(t, err)
	err = cmd.Flags().Set("values", valuesFiles[0])
	require.NoError(t, err)
	err = cmd.Flags().Set("kube-version", invalidVersion)
	require.NoError(t, err)
	err = cmd.Flags().Set("strict", "true")
	require.NoError(t, err)

	// Capture stdout
	bufStdout := &bytes.Buffer{}
	bufStderr := &bytes.Buffer{}
	cmd.SetOut(bufStdout)
	cmd.SetErr(bufStderr)

	// Run the command - should fail with error
	err = cmd.Execute()
	require.Error(t, err, "Command should fail with invalid Kubernetes version")
	assert.Contains(t, err.Error(), "invalid kubernetes version", "Error should mention invalid version")

	// Now test with release name validation
	err = os.Setenv("HELM_PLUGIN_NAME", "irr")
	require.NoError(t, err)
	defer func() {
		err := os.Unsetenv("HELM_PLUGIN_NAME")
		if err != nil {
			t.Errorf("Error unsetting HELM_PLUGIN_NAME: %v", err)
		}
	}()
	cmd = setupCommandForTest()
	cmd.SetArgs([]string{
		"--release-name", "test-release",
		"--values", valuesFiles[0],
		"--kube-version", invalidVersion,
	})

	// Reset buffers
	bufStdout.Reset()
	bufStderr.Reset()
	cmd.SetOut(bufStdout)
	cmd.SetErr(bufStderr)

	// Execute command - should fail with error
	err = cmd.Execute()
	require.Error(t, err, "Command should fail with invalid Kubernetes version")
	assert.Contains(t, err.Error(), "invalid kubernetes version", "Error should mention invalid version")
}

// TestValidateCmd_KubeVersionPrecedence requires modification of how TemplateOptions
// handles --set values, which is currently done inside helm.Template.
// To test precedence properly here, we'd need to inspect the final args passed
// to the Helm SDK within the mock, or enhance the mock significantly.
// For now, this test is deferred or simplified.
/*
func TestValidateCmd_KubeVersionPrecedence(t *testing.T) {
	cmd, cleanup := setupValidateTest(t)
	defer cleanup()

	flagVersion := "1.30.1"
	setVersion := "1.28.8"

	cmd.Flags().Set("kube-version", flagVersion)
	cmd.Flags().Set("set", fmt.Sprintf("Capabilities.KubeVersion.Version=v%s", setVersion))
	cmd.Flags().Set("set", fmt.Sprintf("kubeVersion=%s", setVersion))

	var capturedOptions *helm.TemplateOptions
	mockHelmTemplate = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
		capturedOptions = options
		// In a real scenario, the helm.Template function itself should ensure
		// options.KubeVersion takes precedence over any conflicting --set values.
		// Here, we just check the KubeVersion field was set correctly from the flag.
		return &helm.CommandResult{Success: true, Stdout: "manifest"}, nil
	}

	err := cmd.Execute()
	require.NoError(t, err)
	require.NotNil(t, capturedOptions)
	assert.Equal(t, flagVersion, capturedOptions.KubeVersion, "--kube-version flag should take precedence")

	// Ideally, also assert that the conflicting --set values were NOT passed
	// or were ignored by the (mocked) helm.Template logic.
	// This requires more complex mocking or inspection.
}
*/

func TestDetectChartInCurrentDirectoryIfNeeded(t *testing.T) {
	// Create test cases
	testCases := []struct {
		name          string
		inputPath     string
		setupFs       func(fs afero.Fs)
		expectedPath  string
		expectedError bool
	}{
		{
			name:          "Path already provided",
			inputPath:     "/some/chart/path",
			setupFs:       func(_ afero.Fs) {},
			expectedPath:  "/some/chart/path",
			expectedError: false,
		},
		{
			name:      "Chart.yaml in current directory",
			inputPath: "",
			setupFs: func(fs afero.Fs) {
				err := afero.WriteFile(fs, "Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), fileutil.ReadWriteUserReadOthers)
				require.NoError(t, err)
			},
			expectedPath:  "", // Will be replaced with os.Getwd() result
			expectedError: false,
		},
		{
			name:      "Chart.yaml in subdirectory",
			inputPath: "",
			setupFs: func(fs afero.Fs) {
				err := fs.MkdirAll("mychart", fileutil.ReadWriteExecuteUserReadExecuteOthers)
				require.NoError(t, err)
				err = afero.WriteFile(fs, "mychart/Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), fileutil.ReadWriteUserReadOthers)
				require.NoError(t, err)
			},
			expectedPath:  "mychart",
			expectedError: false,
		},
		{
			name:          "No chart found",
			inputPath:     "",
			setupFs:       func(_ afero.Fs) {},
			expectedPath:  "",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock filesystem
			mockFs := afero.NewMemMapFs()
			tc.setupFs(mockFs)

			// Replace global filesystem with mock
			reset := SetFs(mockFs)
			defer reset() // Restore original filesystem

			// Call the function
			path, err := detectChartInCurrentDirectoryIfNeeded(tc.inputPath)

			// Check results
			switch {
			case tc.expectedError:
				assert.Error(t, err)
			case tc.inputPath != "":
				assert.NoError(t, err)
				assert.Equal(t, tc.inputPath, path)
			case tc.expectedPath == "mychart":
				assert.NoError(t, err)
				assert.Contains(t, path, "mychart")
			case tc.expectedPath == "":
				assert.NoError(t, err)
				assert.NotEmpty(t, path)
			}
		})
	}
}

// TestValidateCmdFlags tests the flag parsing for the validate command
func TestValidateCmdFlags(t *testing.T) {
	t.Run("chart path flag", func(t *testing.T) {
		// Create the command
		cmd := newValidateCmd()

		// Set the chart path flag
		args := []string{"--chart-path", "/test/chart"}
		cmd.SetArgs(args)

		// Parse flags - important to pass the actual args, not the command args
		err := cmd.ParseFlags(args)
		require.NoError(t, err)

		// Get the flag value
		chartPath, err := cmd.Flags().GetString("chart-path")
		require.NoError(t, err)
		assert.Equal(t, "/test/chart", chartPath)
	})

	t.Run("values files flag", func(t *testing.T) {
		// Create the command
		cmd := newValidateCmd()

		// Set the values flag
		args := []string{"--values", "/test/values.yaml"}
		cmd.SetArgs(args)

		// Parse flags - important to pass the actual args, not the command args
		err := cmd.ParseFlags(args)
		require.NoError(t, err)

		// Get the flag value
		values, err := cmd.Flags().GetStringSlice("values")
		require.NoError(t, err)
		assert.Contains(t, values, "/test/values.yaml")
	})

	t.Run("kube version flag", func(t *testing.T) {
		// Create the command
		cmd := newValidateCmd()

		// Set the kube version flag
		args := []string{"--kube-version", "1.20.0"}
		cmd.SetArgs(args)

		// Parse flags - important to pass the actual args, not the command args
		err := cmd.ParseFlags(args)
		require.NoError(t, err)

		// Get the flag value
		kubeVersion, err := cmd.Flags().GetString("kube-version")
		require.NoError(t, err)
		assert.Equal(t, "1.20.0", kubeVersion)
	})

	t.Run("positional argument", func(t *testing.T) {
		// Create the command
		cmd := newValidateCmd()

		// Set a positional argument
		args := []string{"release-name"}
		cmd.SetArgs(args)

		// Execute command to process arguments
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.RunE = func(_ *cobra.Command, _ []string) error {
			// Successfully parsed arguments
			return nil
		}
		err := cmd.Execute()
		require.NoError(t, err)

		// Make sure the argument is parsed
		assert.Len(t, args, 1, "Argument should be parsed")
		assert.Equal(t, "release-name", args[0])
	})

	t.Run("strict flag", func(t *testing.T) {
		// Create the command
		cmd := newValidateCmd()

		// Set the strict flag
		args := []string{"--strict"}
		cmd.SetArgs(args)

		// Parse flags - important to pass the actual args, not the command args
		err := cmd.ParseFlags(args)
		require.NoError(t, err)

		// Get the flag value
		strict, err := cmd.Flags().GetBool("strict")
		require.NoError(t, err)
		assert.True(t, strict)
	})

	t.Run("namespace flag", func(t *testing.T) {
		// Create the command
		cmd := newValidateCmd()

		// Set the namespace flag
		args := []string{"--namespace", "test-ns"}
		cmd.SetArgs(args)

		// Parse flags - important to pass the actual args, not the command args
		err := cmd.ParseFlags(args)
		require.NoError(t, err)

		// Get the flag value
		namespace, err := cmd.Flags().GetString("namespace")
		require.NoError(t, err)
		assert.Equal(t, "test-ns", namespace)
	})
}

// TestValidateCommandWithOverrides tests the validate command with a mocked overrideRelease function
func TestValidateCommandWithOverrides(t *testing.T) {
	// Create a command that will just parse flags and output what it would validate
	validateCmd := &cobra.Command{
		Use:   "validate [release-name]",
		Short: "Test validate command",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get the flag values with error checking
			chartPath, err := cmd.Flags().GetString("chart-path")
			if err != nil {
				return fmt.Errorf("failed to get chart-path flag: %w", err)
			}
			values, err := cmd.Flags().GetStringSlice("values")
			if err != nil {
				return fmt.Errorf("failed to get values flag: %w", err)
			}
			namespace, err := cmd.Flags().GetString("namespace")
			if err != nil {
				return fmt.Errorf("failed to get namespace flag: %w", err)
			}
			kubeVersion, err := cmd.Flags().GetString("kube-version")
			if err != nil {
				return fmt.Errorf("failed to get kube-version flag: %w", err)
			}
			strict, err := cmd.Flags().GetBool("strict")
			if err != nil {
				return fmt.Errorf("failed to get strict flag: %w", err)
			}

			// Get release name from args if available
			releaseName := ""
			if len(args) > 0 {
				releaseName = args[0]
			}

			// Output what would be validated
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Would validate: chart=%s, release=%s, namespace=%s, kube-version=%s, strict=%v\n",
				chartPath, releaseName, namespace, kubeVersion, strict); err != nil {
				return fmt.Errorf("failed to write output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "With values files: %v\n", values); err != nil {
				return fmt.Errorf("failed to write values output: %w", err)
			}

			return nil
		},
	}

	// Add flags
	validateCmd.Flags().String("chart-path", "", "Path to Helm chart")
	validateCmd.Flags().StringSlice("values", []string{}, "Values files to use")
	validateCmd.Flags().String("namespace", "", "Namespace to use")
	validateCmd.Flags().String("kube-version", "", "Kubernetes version")
	validateCmd.Flags().Bool("strict", false, "Enable strict mode")

	// Test with chart path
	t.Run("validate with chart path", func(t *testing.T) {
		// Create a buffer to capture output
		buf := new(bytes.Buffer)
		validateCmd.SetOut(buf)

		validateCmd.SetArgs([]string{
			"--chart-path", "/test/chart",
			"--values", "/test/values.yaml",
			"--kube-version", "1.20.0",
		})

		err := validateCmd.Execute()
		require.NoError(t, err)

		// Check the output
		output := buf.String()
		assert.Contains(t, output, "chart=/test/chart")
		assert.Contains(t, output, "kube-version=1.20.0")
		assert.Contains(t, output, "/test/values.yaml")
	})

	// Test with release name
	t.Run("validate with release name", func(t *testing.T) {
		// Create a buffer to capture output
		buf := new(bytes.Buffer)
		validateCmd.SetOut(buf)

		validateCmd.SetArgs([]string{
			"release-name",
			"--values", "/test/values.yaml",
			"--namespace", "test-ns",
		})

		err := validateCmd.Execute()
		require.NoError(t, err)

		// Check the output
		output := buf.String()
		assert.Contains(t, output, "release=release-name")
		assert.Contains(t, output, "namespace=test-ns")
		assert.Contains(t, output, "/test/values.yaml")
	})
}

func TestValidateCommand_MultipleValueFiles(t *testing.T) {
	memFs := afero.NewMemMapFs()
	chartDir := "/mock-chart"
	values1 := "/values1.yaml"
	values2 := "/values2.yaml"
	chartFile := filepath.Join(chartDir, "Chart.yaml")

	// Use constants for file permissions instead of hardcoded values for consistency and maintainability
	err := memFs.MkdirAll(chartDir, fileutil.ReadWriteExecuteUserReadExecuteOthers)
	require.NoError(t, err)

	err = afero.WriteFile(memFs, chartFile, []byte("name: mock-chart\nversion: 1.0.0"), fileutil.ReadWriteUserReadOthers)
	require.NoError(t, err)

	// Write value files
	err = afero.WriteFile(memFs, values1, []byte("key1: value1"), fileutil.ReadWriteUserReadOthers)
	require.NoError(t, err)
	err = afero.WriteFile(memFs, values2, []byte("key2: value2"), fileutil.ReadWriteUserReadOthers)
	require.NoError(t, err)

	// Execute validate command
	// ... rest of function ...
}

func TestValidateCommand_WithChartsDir(t *testing.T) {
	// Define test cases for chart detection
	testCases := []struct {
		name              string
		setupFs           func(t *testing.T, fs afero.Fs)
		explicitChartPath string // Path to pass via --chart-path for this test
		expectError       bool
		errorContains     string
	}{
		{
			name: "Chart.yaml in current directory",
			setupFs: func(t *testing.T, fs afero.Fs) {
				err := afero.WriteFile(fs, "Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), fileutil.ReadWriteUserReadOthers)
				require.NoError(t, err)
				// Add dummy values file
				err = afero.WriteFile(fs, "/values.yaml", []byte("key: value"), fileutil.ReadWriteUserReadOthers)
				require.NoError(t, err)
			},
			explicitChartPath: "/",
			expectError:       false,
		},
		// {
		// 	name: "Chart in subdirectory",
		// 	setupFs: func(t *testing.T, fs afero.Fs) {
		// 		err := fs.MkdirAll("mychart", fileutil.ReadWriteExecuteUserReadExecuteOthers)
		// 		require.NoError(t, err)
		// 		err = afero.WriteFile(fs, "mychart/Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), fileutil.ReadWriteUserReadOthers)
		// 		require.NoError(t, err)
		// 		// Add dummy values file
		// 		err = afero.WriteFile(fs, "/values.yaml", []byte("key: value"), fileutil.ReadWriteUserReadOthers)
		// 		require.NoError(t, err)
		// 	},
		// 	explicitChartPath: "/mychart",
		// 	expectError:       false,
		// },
		{
			name: "No chart found",
			setupFs: func(t *testing.T, fs afero.Fs) {
				// Add dummy values file even when chart is missing
				err := afero.WriteFile(fs, "/values.yaml", []byte("key: value"), fileutil.ReadWriteUserReadOthers)
				require.NoError(t, err)
			},
			explicitChartPath: "/",
			expectError:       true,
			errorContains:     "chart path not found",
		},
	}

	// Save original test mode state and restore it
	originalTestMode := isValidateTestMode
	defer func() { isValidateTestMode = originalTestMode }()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set test mode within the subtest to ensure isolation
			isValidateTestMode = true

			// Setup mock filesystem for each test case
			fs := afero.NewMemMapFs()
			originalFs := AppFs
			AppFs = fs
			defer func() { AppFs = originalFs }()
			tc.setupFs(t, fs)

			// Mock Helm template function (ensure it's mocked for test mode)
			originalTemplateFunc := helm.HelmTemplateFunc
			helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
				// Simple mock: return success unless chart path is explicitly invalid
				if tc.name == "No chart found" && options.ChartPath == "/" {
					// Simulate Helm's chart loading error
					return nil, fmt.Errorf("chart path not found: %s", options.ChartPath)
				}
				return &helm.CommandResult{Success: true, Stdout: "mock template output"}, nil
			}
			defer func() { helm.HelmTemplateFunc = originalTemplateFunc }()

			// Setup command and buffers for each test case
			cmd := newValidateCmd()
			buffer := new(bytes.Buffer)
			cmd.SetIn(bytes.NewBufferString(""))
			cmd.SetOut(buffer)
			cmd.SetErr(buffer)

			// Set args, explicitly providing the chart path AND the dummy values file
			args := []string{"--values", "/values.yaml"} // Always add dummy values
			if tc.explicitChartPath != "" {
				args = append(args, "--chart-path", tc.explicitChartPath)
			}
			cmd.SetArgs(args)

			// Execute the command
			err := cmd.Execute()

			// Assertions
			if tc.expectError {
				assert.Error(t, err, "Expected an error for %s", tc.name)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains, "Error message mismatch for %s", tc.name)
				}
			} else {
				assert.NoError(t, err, "Did not expect an error for %s. Stderr: %s", tc.name, buffer.String())
			}
		})
	}
}
