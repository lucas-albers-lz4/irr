package main

import (
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

// Constants for repeated test values in validate tests
const (
	validateTestChartPath = "/path/to/chart"
	validateTestRelease   = "release"
	validateTestNamespace = "default"
)

// Mock the helm.Template function via the exported variable
var mockHelmTemplate func(options *helm.TemplateOptions) (*helm.CommandResult, error)

func setupValidateTest(t *testing.T) (cmd *cobra.Command, cleanup func()) {
	// Save the original isHelmPlugin value to restore later
	originalIsHelmPlugin := isHelmPlugin
	// Set isHelmPlugin to true for tests to allow release-name flag
	isHelmPlugin = true

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
		isHelmPlugin = originalIsHelmPlugin          // Restore original isHelmPlugin value
		err := os.RemoveAll(tempDir)
		if err != nil {
			t.Logf("Warning: failed to clean up temp directory %s: %v", tempDir, err)
		}
		mockHelmTemplate = nil // Reset mock
	}

	return cmd, cleanup
}

func TestValidateCmd_DefaultKubeVersion(t *testing.T) {
	_, cleanup := setupValidateTest(t)
	defer cleanup()

	// Simulate validateChartWithFiles being called directly
	chartPath := validateTestChartPath
	releaseName := validateTestRelease
	namespace := validateTestNamespace
	valuesFiles := []string{"/path/to/values.yaml"}
	strict := false

	// Directly replace HelmTemplateFunc with our mock
	originalHelmTemplate := helm.HelmTemplateFunc
	defer func() { helm.HelmTemplateFunc = originalHelmTemplate }()

	var capturedOptions *helm.TemplateOptions
	helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
		capturedOptions = options
		return &helm.CommandResult{
			Success: true,
			Stdout:  "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: value\n",
		}, nil
	}

	// Call validateChartWithFiles directly instead of cmd.Execute()
	result, err := validateChartWithFiles(chartPath, releaseName, namespace, valuesFiles, strict, DefaultKubernetesVersion)
	require.NoError(t, err)
	require.NotEmpty(t, result, "Expected non-empty template result")

	require.NotNil(t, capturedOptions, "capturedOptions should not be nil")
	assert.Equal(t, DefaultKubernetesVersion, capturedOptions.KubeVersion)
}

func TestValidateCmd_ExplicitKubeVersion(t *testing.T) {
	_, cleanup := setupValidateTest(t)
	defer cleanup()

	expectedVersion := "1.29.5"

	// Simulate validateChartWithFiles being called directly
	chartPath := validateTestChartPath
	releaseName := validateTestRelease
	namespace := validateTestNamespace
	valuesFiles := []string{"/path/to/values.yaml"}
	strict := false

	// Directly replace HelmTemplateFunc with our mock
	originalHelmTemplate := helm.HelmTemplateFunc
	defer func() { helm.HelmTemplateFunc = originalHelmTemplate }()

	var capturedOptions *helm.TemplateOptions
	helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
		capturedOptions = options
		return &helm.CommandResult{
			Success: true,
			Stdout:  "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: value\n",
		}, nil
	}

	// Call validateChartWithFiles directly instead of cmd.Execute()
	result, err := validateChartWithFiles(chartPath, releaseName, namespace, valuesFiles, strict, expectedVersion)
	require.NoError(t, err)
	require.NotEmpty(t, result, "Expected non-empty template result")

	require.NotNil(t, capturedOptions, "capturedOptions should not be nil")
	assert.Equal(t, expectedVersion, capturedOptions.KubeVersion)
}

func TestValidateCmd_InvalidKubeVersionFormat(t *testing.T) {
	_, cleanup := setupValidateTest(t)
	defer cleanup()

	invalidVersion := "not-a-version"

	// Simulate validateChartWithFiles being called directly
	chartPath := validateTestChartPath
	releaseName := validateTestRelease
	namespace := validateTestNamespace
	valuesFiles := []string{"/path/to/values.yaml"}
	strict := true // Set to true to ensure errors are returned instead of swallowed

	// Directly replace HelmTemplateFunc with our mock
	originalHelmTemplate := helm.HelmTemplateFunc
	defer func() { helm.HelmTemplateFunc = originalHelmTemplate }()

	helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
		// Simulate the error that would come from helm.Template parsing the version
		return &helm.CommandResult{
			Success: false,
			Stderr:  "Error: invalid kubernetes version",
		}, fmt.Errorf("invalid Kubernetes version %q: some underlying helm error", options.KubeVersion)
	}

	// Call validateChartWithFiles directly - with strict=true to ensure errors are returned
	result, err := validateChartWithFiles(chartPath, releaseName, namespace, valuesFiles, strict, invalidVersion)
	require.Error(t, err, "Expected an error with invalid Kubernetes version format")
	assert.Contains(t, err.Error(), "invalid Kubernetes version")
	assert.Empty(t, result, "Result should be empty when there is an error")
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
				err := afero.WriteFile(fs, "Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), 0o644)
				require.NoError(t, err)
			},
			expectedPath:  "", // Will be replaced with os.Getwd() result
			expectedError: false,
		},
		{
			name:      "Chart.yaml in subdirectory",
			inputPath: "",
			setupFs: func(fs afero.Fs) {
				err := fs.MkdirAll("mychart", 0o755)
				require.NoError(t, err)
				err = afero.WriteFile(fs, "mychart/Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), 0o644)
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
