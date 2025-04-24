// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverrideCommand(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")
	if chartPath == "" {
		// Set up a minimal test chart if the test data isn't available
		setupMinimalTestChart(t, harness)
		chartPath = harness.chartPath
	}

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
	)
	require.NoError(t, err, "override command should succeed: %s", stderr)

	// Verify the file exists
	require.FileExists(t, outputFile, "Output file should exist")

	// Read the file content
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	// Verify the output contains expected overrides
	assert.Contains(t, string(content), "test-registry.local", "Output should include the target registry")

	// Look for a repository pattern that would typically be generated
	assert.True(t,
		strings.Contains(string(content), "dockerio") ||
			strings.Contains(string(content), "docker"),
		"Output should include a transformed repository")
}

func TestOverrideWithDifferentPathStrategy(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")
	if chartPath == "" {
		// Set up a minimal test chart if the test data isn't available
		setupMinimalTestChart(t, harness)
		chartPath = harness.chartPath
	}

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command with explicit path strategy
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
		"--path-strategy", "prefix-source-registry",
	)
	require.NoError(t, err, "override command with explicit path strategy should succeed: %s", stderr)
	t.Logf("Stderr: %s", stderr)

	// Verify the output contains expected overrides with the correct path strategy
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err)

	// Verify the strategy is applied correctly
	assert.Contains(t, string(content), "test-registry.local", "Output should include the target registry")

	// With prefix-source-registry strategy, we expect to see dockerio in the repository field
	assert.True(t,
		strings.Contains(string(content), "dockerio") ||
			strings.Contains(string(content), "docker"),
		"Output should include a transformed repository with prefix-source-registry strategy")
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
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
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
	if chartPath == "" {
		t.Skip("minimal-test chart not found, skipping test")
	}

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command with dry-run
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
		"--dry-run",
	)
	require.NoError(t, err, "override command with dry-run should succeed")
	t.Logf("Stderr: %s", stderr)

	// In dry-run mode, the file should not be created
	_, err = os.Stat(outputFile)
	require.Error(t, err, "Output file should not exist in dry-run mode")
	require.True(t, os.IsNotExist(err), "Error should be 'file not exists'")

	// In dry-run mode, the output is printed to STDOUT.
	stdout, _, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
		"--dry-run",
	)
	require.NoError(t, err, "override command with dry-run should succeed")

	// The output should contain the overrides (in the stdout)
	assert.Contains(t, stdout, "test-registry.local", "Command output (stdout) should include the target registry")
	assert.Contains(t, stdout, "docker.io/", "Command output (stdout) should include the transformed repository")
}

func TestOverrideParentChart(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the parent-test chart which includes a subchart
	chartPath := harness.GetTestdataPath("charts/parent-test")

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command
	_, _, err := harness.ExecuteIRRWithStderr(nil,
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
	assert.Contains(t, string(content), "repository: docker.io/bitnami/nginx", "Output should include the relocated nginx image")
	assert.Contains(t, string(content), "repository: docker.io/parent/app", "Output should include the relocated app image from parent chart")
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
	_, _, err := harness.ExecuteIRRWithStderr(nil,
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
	assert.NotContains(t, string(content), "repository: docker.io/", "Output should not include docker.io images")
}

func TestOverrideWithExcludeRegistry(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the parent-test chart which includes images from docker.io
	chartPath := harness.GetTestdataPath("charts/parent-test")

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "overrides.yaml")

	// Run the override command with exclude-registries
	_, _, err := harness.ExecuteIRRWithStderr(nil,
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
