// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverrideCommand(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command
	_, _, err := harness.ExecuteIRRWithStderr(
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
	)
	require.NoError(t, err, "override command should succeed")

	// Verify the file exists
	_, err = os.Stat(outputFile)
	require.NoError(t, err, "Output file should exist")

	// Read the file content
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	// Verify the output contains expected overrides
	assert.Contains(t, string(content), "test-registry.local/dockerio", "Output should include the relocated image repository")
}

func TestOverrideWithDifferentPathStrategy(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command with explicit path strategy
	_, _, err := harness.ExecuteIRRWithStderr(
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
		"--strategy", "prefix-source-registry",
	)
	require.NoError(t, err, "override command with explicit path strategy should succeed")

	// Verify the output contains expected overrides with the correct path strategy
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err)
	assert.Contains(t, string(content), "test-registry.local/dockerio", "Output should include the relocated image with prefix-source-registry strategy")
}

func TestOverrideWithStrictMode(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the unsupported-test chart which contains unsupported image patterns
	chartPath := harness.GetTestdataPath("charts/unsupported-test")
	if chartPath == "" {
		t.Skip("unsupported-test chart not found, skipping test")
	}

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command with strict mode
	_, stderr, err := harness.ExecuteIRRWithStderr(
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
		"--strict",
	)

	// With strict mode, we expect an error for unsupported image patterns
	require.Error(t, err, "override command with strict mode should fail for unsupported patterns")

	// Check the stderr for the correct error message about unsupported structures
	assert.Contains(t, stderr, "unsupported", "Error should mention unsupported pattern")
}

func TestOverrideDryRun(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command with dry-run
	output, _, err := harness.ExecuteIRRWithStderr(
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
		"--dry-run",
	)
	require.NoError(t, err, "override command with dry-run should succeed")

	// In dry-run mode, the file should not be created
	_, err = os.Stat(outputFile)
	require.Error(t, err, "Output file should not exist in dry-run mode")
	require.True(t, os.IsNotExist(err), "Error should be 'file not exists'")

	// The output should contain the overrides
	assert.Contains(t, output, "test-registry.local/dockerio", "Command output should include the relocated image")
}

func TestOverrideParentChart(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the parent-test chart which includes a subchart
	chartPath := harness.GetTestdataPath("charts/parent-test")

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command
	_, _, err := harness.ExecuteIRRWithStderr(
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
	)
	require.NoError(t, err, "override command should succeed for parent chart")

	// Read the file content
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	// Verify the output contains overrides for both parent and child charts
	assert.Contains(t, string(content), "repository: dockerio/bitnami/nginx", "Output should include the relocated nginx image")
	assert.Contains(t, string(content), "repository: dockerio/parent/app", "Output should include the relocated app image from parent chart")
}

func TestOverrideWithRegistry(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the kube-prometheus-stack chart which has images from multiple registries
	chartPath := harness.GetTestdataPath("charts/kube-prometheus-stack")
	if chartPath == "" {
		t.Skip("kube-prometheus-stack chart not found, skipping test")
	}

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command targeting only quay.io images
	_, _, err := harness.ExecuteIRRWithStderr(
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "quay.io",
		"--output-file", outputFile,
	)
	require.NoError(t, err, "override command should succeed with registry filter")

	// Read the file content
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	// Verify the output contains only quay.io overrides
	assert.Contains(t, string(content), "repository: quayio/", "Output should include the relocated quay.io images")

	// Docker.io images should not be relocated since we only targeted quay.io
	assert.NotContains(t, string(content), "repository: dockerio/", "Output should not include docker.io images")
}

func TestOverrideWithExcludeRegistry(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the parent-test chart which includes images from docker.io
	chartPath := harness.GetTestdataPath("charts/parent-test")

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command with exclude-registries
	_, _, err := harness.ExecuteIRRWithStderr(
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--exclude-registries", "docker.io", // This effectively excludes all images from being processed
		"--output-file", outputFile,
	)
	require.NoError(t, err, "override command should succeed with exclude registries")

	// Read the file content
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	// Since we excluded docker.io, the output should be empty or minimal
	assert.NotContains(t, string(content), "test-registry.local/dockerio", "Output should not include any relocated docker.io images")
}
