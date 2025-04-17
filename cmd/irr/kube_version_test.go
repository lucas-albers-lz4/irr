package main

import (
	"fmt"
	"testing"

	"github.com/lalbers/irr/internal/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Constants for repeated test values
const (
	testChartPath   = "/path/to/chart"
	testReleaseName = "test-release"
	testNamespace   = "test-namespace"
)

// directTemplateMock is a helper function to mock helm.Template with a callback
func directTemplateMock(t *testing.T, callback func(*helm.TemplateOptions)) func() {
	t.Helper()
	original := helm.HelmTemplateFunc
	// Add debug info for the test
	t.Logf("Setting up directTemplateMock")

	helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
		// Call the callback with the options received
		callback(options)
		// Return successful result with non-empty content
		t.Logf("Mock helm.Template called, returning successful result")
		return &helm.CommandResult{
			Success: true,
			Stdout:  "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: value\n",
		}, nil
	}

	return func() {
		t.Logf("Cleanup directTemplateMock called")
		helm.HelmTemplateFunc = original
	}
}

func TestKubeVersionInValidateChartWithFiles(t *testing.T) {
	// Test data
	chartPath := testChartPath
	releaseName := testReleaseName
	namespace := testNamespace
	valuesFiles := []string{"/path/to/values.yaml"}
	strict := false

	// Test case 1: Default Kubernetes version
	t.Run("Default Kubernetes Version", func(t *testing.T) {
		expectedVersion := DefaultKubernetesVersion
		var captured *helm.TemplateOptions

		// Mock helm.Template, capture options
		cleanup := directTemplateMock(t, func(options *helm.TemplateOptions) {
			t.Logf("Captured options in callback")
			captured = options
		})
		defer cleanup()

		// Call the function - Our mock returns success and non-empty content
		t.Logf("About to call validateChartWithFiles")
		result, err := validateChartWithFiles(chartPath, releaseName, namespace, valuesFiles, strict, expectedVersion)
		t.Logf("validateChartWithFiles returned, err=%v, result length=%d", err, len(result))
		require.NoError(t, err)
		require.NotEmpty(t, result, "Expected non-empty template result")

		// Verify KubeVersion was passed correctly
		require.NotNil(t, captured, "Template options should have been captured")
		assert.Equal(t, expectedVersion, captured.KubeVersion, "KubeVersion should match the input")
	})

	// Test case 2: Custom Kubernetes version
	t.Run("Custom Kubernetes Version", func(t *testing.T) {
		expectedVersion := "1.29.5"
		var captured *helm.TemplateOptions

		// Mock helm.Template, capture options
		cleanup := directTemplateMock(t, func(options *helm.TemplateOptions) {
			t.Logf("Captured options in callback")
			captured = options
		})
		defer cleanup()

		// Call the function - Our mock returns success and non-empty content
		t.Logf("About to call validateChartWithFiles")
		result, err := validateChartWithFiles(chartPath, releaseName, namespace, valuesFiles, strict, expectedVersion)
		t.Logf("validateChartWithFiles returned, err=%v, result length=%d", err, len(result))
		require.NoError(t, err)
		require.NotEmpty(t, result, "Expected non-empty template result")

		// Verify KubeVersion was passed correctly
		require.NotNil(t, captured, "Template options should have been captured")
		assert.Equal(t, expectedVersion, captured.KubeVersion, "KubeVersion should match the input")
	})
}

func TestKubeVersionPassthrough(t *testing.T) {
	// Save original HelmTemplateFunc and restore it after the test
	originalHelmTemplateFunc := helm.HelmTemplateFunc
	defer func() { helm.HelmTemplateFunc = originalHelmTemplateFunc }()

	tests := []struct {
		name           string
		inputVersion   string
		expectedOutput string
		expectError    bool
		errorMsg       string
		strict         bool // Add strict flag to control behavior
	}{
		{
			name:           "Default Kubernetes Version",
			inputVersion:   DefaultKubernetesVersion,
			expectedOutput: DefaultKubernetesVersion,
			expectError:    false,
			strict:         false,
		},
		{
			name:           "Custom Kubernetes Version",
			inputVersion:   "1.29.5",
			expectedOutput: "1.29.5",
			expectError:    false,
			strict:         false,
		},
		{
			name:           "Invalid Kubernetes Version",
			inputVersion:   "not-a-version",
			expectedOutput: "not-a-version",
			expectError:    true,
			errorMsg:       "invalid Kubernetes version",
			strict:         true, // Set strict=true to ensure error is returned
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock function for helm.Template
			var capturedOptions *helm.TemplateOptions
			if tc.expectError {
				helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
					// Store a copy of the options before returning the error
					// This fixes an issue where the options weren't being captured correctly
					capturedOptions = &helm.TemplateOptions{
						ChartPath:   options.ChartPath,
						ReleaseName: options.ReleaseName,
						ValuesFiles: options.ValuesFiles,
						Namespace:   options.Namespace,
						KubeVersion: options.KubeVersion,
						SetValues:   options.SetValues,
					}
					return &helm.CommandResult{
						Success: false,
						Stderr:  "Error: invalid kubernetes version",
					}, fmt.Errorf("invalid Kubernetes version %q: some error", options.KubeVersion)
				}
			} else {
				helm.HelmTemplateFunc = func(options *helm.TemplateOptions) (*helm.CommandResult, error) {
					capturedOptions = options
					return &helm.CommandResult{
						Success: true,
						Stdout:  "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: value\n",
					}, nil
				}
			}

			// Call the function directly
			chartPath := testChartPath
			releaseName := testReleaseName
			namespace := testNamespace
			valuesFiles := []string{"/path/to/values.yaml"}
			strict := tc.strict // Use the test case's strict value

			result, err := validateChartWithFiles(chartPath, releaseName, namespace, valuesFiles, strict, tc.inputVersion)

			// Assertions
			if tc.expectError {
				require.Error(t, err, "Expected an error for invalid Kubernetes version")
				// When strict=true, validateChartWithFiles returns an exitcodes.ExitCodeError
				// So we need to check the underlying error message
				errString := err.Error()
				assert.Contains(t, errString, tc.errorMsg, "Error message should contain expected text")
				assert.Empty(t, result, "Result should be empty when there is an error")
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, result, "Expected non-empty template result")
			}

			// Verify the KubeVersion was correctly passed to the HelmTemplateFunc
			require.NotNil(t, capturedOptions, "Template options should have been captured")
			assert.Equal(t, tc.expectedOutput, capturedOptions.KubeVersion)
		})
	}
}
