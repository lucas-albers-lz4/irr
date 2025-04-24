// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/exitcodes"
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

// Test case for --no-validate flag where validation would normally fail
func TestOverrideNoValidateSuccessOnValidationFailure(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the chart specifically designed to fail validation without specific values
	chartPath := harness.GetTestdataPath("charts/validation-fail-test")
	if chartPath == "" {
		t.Skip("validation-fail-test chart not found, skipping test")
	}

	outputFile := filepath.Join(harness.tempDir, "overrides-no-validate.yaml")

	// We need to ensure we use the correct chart path obtained from the harness
	// chartPath = harness.GetTestdataPath("charts/validation-fail-test") <-- Redundant assignment
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath, // Use the chartPath obtained earlier
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io", // Target an image, although this chart has none
		"--output-file", outputFile,
		"--no-validate", // The key flag for this test
	)

	// Expect success because validation is skipped
	require.NoError(t, err, "override command with --no-validate should succeed even if validation would fail: %s", stderr)
	require.FileExists(t, outputFile, "Output file should be created when --no-validate is used")

	// Basic check on content
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err)

	// Since validation-fail-test has no values.yaml, override output should be empty map `{}`
	assert.Equal(t, "{}", strings.TrimSpace(string(content)), "Overrides file should be an empty map '{}' for chart with no values")
}

// Test case for the default behavior (validation runs and potentially fails)
func TestOverrideDefaultValidationFailure(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the chart specifically designed to fail validation without specific values
	chartPath := harness.GetTestdataPath("charts/validation-fail-test")
	if chartPath == "" {
		t.Skip("validation-fail-test chart not found, skipping test")
	}

	outputFile := filepath.Join(harness.tempDir, "overrides-default-validate.yaml")

	// Define args for harness assertion helpers
	args := []string{
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
	}

	// Expect failure because validation runs by default and the chart requires .Values.mandatoryValue
	// Assert that the command fails with the correct Helm error exit code (16)
	harness.AssertExitCode(exitcodes.ExitHelmCommandFailed, args...)

	// Assert that the error message contains the specific Helm template error
	harness.AssertErrorContains("mandatoryValue is required for this chart!", args...)

	// File should not exist if the command fails before writing
	_, statErr := os.Stat(outputFile)
	assert.True(t, os.IsNotExist(statErr), "Output file should not exist if validation fails early")
}

// TestOverrideSimpleChart verifies basic override generation for the simple chart.
func TestOverrideSimpleChart(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the simple chart
	chartPath := harness.GetTestdataPath("charts/simple")
	if chartPath == "" {
		t.Skip("simple chart not found, skipping test")
	}

	// Create output file path
	outputFile := filepath.Join(harness.tempDir, "simple-overrides.yaml")

	// Run the override command
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
	)
	require.NoError(t, err, "override command should succeed for simple chart: %s", stderr)

	// Verify the file exists
	require.FileExists(t, outputFile, "Output file should exist for simple chart")

	// Read the file content
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file for simple chart")

	// Verify the output contains expected overrides
	assert.Contains(t, string(content), "test-registry.local", "Output should include the target registry")
	assert.Contains(t, string(content), "docker.io/library/nginx", "Output should include the transformed repository path for docker.io")
	assert.Contains(t, string(content), "1.21.0", "Output should include the original tag")
}
