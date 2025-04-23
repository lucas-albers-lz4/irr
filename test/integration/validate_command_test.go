// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCommand(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")
	valuesPath := harness.GetTestdataPath("charts/minimal-test/values.yaml")
	overridesPath := filepath.Join(harness.tempDir, "overrides.yaml") // Define path for generated overrides

	// First, generate overrides using the override command
	_, _, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", overridesPath,
	)
	require.NoError(t, err, "override command should succeed")

	// Run the validate command
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", valuesPath,
		"--values", overridesPath,
	)
	require.NoError(t, err, "validate command should succeed")

	// Verify the output contains a success message
	assert.Contains(t, stderr, "Validation successful", "Output should include validation success message")
}

func TestValidateWithInvalidValues(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Create an invalid values file (invalid YAML)
	invalidValuesFile := filepath.Join(harness.tempDir, "invalid-values.yaml")
	// Create a severely malformed YAML file that will definitely fail validation
	err := os.WriteFile(invalidValuesFile, []byte(`
image:
  repository: "nginx
  tag: "1.21.0
  This is not valid YAML at all
  - broken: array
`), fileutil.ReadWriteUserPermission)
	require.NoError(t, err, "Should be able to create invalid values file")

	// Run the validate command with invalid values
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", invalidValuesFile,
	)

	// Expect an error from YAML parsing
	require.Error(t, err, "validate command should fail with invalid values file")
	assert.Contains(t, stderr, "error", "Error should indicate validation failure")
}

func TestValidateWithMultipleValuesFiles(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Create first values file
	valuesFile1 := filepath.Join(harness.tempDir, "values1.yaml")
	err := os.WriteFile(valuesFile1, []byte("global:\n  imageRegistry: test-registry.local"), fileutil.ReadWriteUserPermission)
	require.NoError(t, err, "Should be able to create first values file")

	// Create second values file
	valuesFile2 := filepath.Join(harness.tempDir, "values2.yaml")
	err = os.WriteFile(valuesFile2, []byte("nginx:\n  tag: latest"), fileutil.ReadWriteUserPermission)
	require.NoError(t, err, "Should be able to create second values file")

	// Run the validate command with multiple values files
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", valuesFile1,
		"--values", valuesFile2,
	)
	require.NoError(t, err, "validate command should succeed with multiple values files")

	// Verify the output contains a success message
	assert.Contains(t, stderr, "Validation successful", "Output should include validation success message")
}

func TestValidateWithOutputFile(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")
	valuesPath := harness.GetTestdataPath("charts/minimal-test/values.yaml")
	overridesPath := filepath.Join(harness.tempDir, "overrides.yaml") // Define path for generated overrides

	// First, generate overrides using the override command
	_, _, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", overridesPath,
	)
	require.NoError(t, err, "override command should succeed")

	// Output file for validation results
	outputFile := filepath.Join(harness.tempDir, "validate-output.txt")

	// Run the validate command with output file
	_, _, err = harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", valuesPath,
		"--values", overridesPath,
		"--output-file", outputFile,
	)
	require.NoError(t, err, "validate command should succeed with output file")

	// Verify the file exists and is not empty
	fileInfo, err := os.Stat(outputFile)
	require.NoError(t, err, "Output file should exist")
	assert.Greater(t, fileInfo.Size(), int64(0), "Output file should not be empty")

	// Read the file content
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	// Verify the output contains valid YAML with deployment info
	assert.Contains(t, string(content), "kind: Deployment", "Output should include Kubernetes manifests")
}

func TestValidateWithNonExistentChart(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use a non-existent chart path
	chartPath := filepath.Join(harness.tempDir, "non-existent-chart")

	// Run the validate command with a non-existent chart
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
	)

	// Expect an error about the chart not existing
	require.Error(t, err, "validate command should fail with non-existent chart")
	assert.Contains(t, stderr, "chart path not found", "Error should mention chart path")
}

func TestValidateWithMissingValuesFile(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Use a non-existent values file
	valuesFile := filepath.Join(harness.tempDir, "non-existent-values.yaml")

	// Run the validate command with a missing values file
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", valuesFile,
	)

	// Expect an error about the values file not existing
	require.Error(t, err, "validate command should fail with missing values file")
	assert.Contains(t, stderr, "values file not found", "Error should mention values file")
}

func TestValidateWithStrictFlag(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Generate overrides file path
	overridesFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Use a chart with unsupported structures that would cause strict mode to fail
	chartPath := harness.GetTestdataPath("charts/unsupported-test")

	// First, generate overrides using the override command
	_, _, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", overridesFile,
		"--strict", // Added to test strict mode failure with unsupported structures
	)
	require.Error(t, err, "override command should fail for unsupported structures")

	// The validation part is unreachable as override fails first.
}
