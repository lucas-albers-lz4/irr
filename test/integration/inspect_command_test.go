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

func TestInspectCommand(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Run the inspect command on the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")
	output, _, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", chartPath,
	)
	require.NoError(t, err, "Inspect command should succeed")

	// Verify the output contains expected sections
	assert.Contains(t, output, "chart:", "Output should include chart section")
	assert.Contains(t, output, "images:", "Output should include images section")
	assert.Contains(t, output, "docker.io/nginx", "Output should include the nginx image")
}

func TestInspectWithSourceRegistryFilter(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Run the inspect command with a specific source registry filter
	chartPath := harness.GetTestdataPath("charts/minimal-test")
	output, _, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", chartPath,
		"--source-registries", "docker.io",
	)
	require.NoError(t, err, "Inspect command should succeed with source registry filter")

	// Verify only docker.io images are included
	assert.Contains(t, output, "docker.io/nginx", "Output should include the nginx image")
	assert.NotContains(t, output, "k8s.gcr.io", "Output should not include k8s.gcr.io images")
}

func TestInspectOutputToFile(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Run the inspect command and output to a file
	chartPath := harness.GetTestdataPath("charts/minimal-test")
	outputFile := filepath.Join(harness.tempDir, "inspect-output.yaml")

	_, _, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", chartPath,
		"--output-file", outputFile,
	)
	require.NoError(t, err, "Inspect command should succeed with output to file")

	// Verify the file exists and has content
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")
	assert.NotEmpty(t, content, "Output file should not be empty")

	// Check for expected content in the file
	contentStr := string(content)
	assert.Contains(t, contentStr, "chart:", "Output file should include chart section")
	assert.Contains(t, contentStr, "images:", "Output file should include images section")
}

func TestInspectParentChart(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Run the inspect command on a parent chart with subcharts
	chartPath := harness.GetTestdataPath("charts/parent-test")
	if chartPath == "" {
		t.Skip("parent-test chart not found, skipping test")
	}

	output, _, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", chartPath,
	)
	require.NoError(t, err, "Inspect command should succeed with parent chart")

	// Verify the output contains both parent and subchart information
	assert.Contains(t, output, "parent-test", "Output should include parent chart name")
	assert.Contains(t, output, "child.", "Output should include subchart information")
}

func TestInspectGenerateConfigSkeleton(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use setupMinimalTestChart instead of CreateMinimalTestChart
	setupMinimalTestChart(t, harness)
	// Use proper constant reference
	skeletonFile := filepath.Join(harness.tempDir, "irr-config.yaml")

	// Run the inspect command with generate-config-skeleton option
	_, _, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", harness.chartPath,
		"--generate-config-skeleton",
		"--output-file", skeletonFile,
	)
	require.NoError(t, err, "Inspect command should succeed with generate-config-skeleton option")

	// Check the generated file
	content, err := os.ReadFile(skeletonFile) // #nosec G304
	require.NoError(t, err, "Should be able to read config skeleton file")

	// Verify the output contains the skeleton configuration
	contentStr := string(content)
	assert.Contains(t, contentStr, "mappings:", "Config skeleton should include mappings section")
	assert.Contains(t, contentStr, "docker.io", "Config skeleton should include docker.io registry")
}

func TestInspectInvalidChartPath(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Run the inspect command with an invalid chart path
	invalidPath := filepath.Join(harness.tempDir, "nonexistent-chart")
	_, stderr, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", invalidPath,
	)

	// Verify the command fails with an appropriate error
	require.Error(t, err, "Inspect command should fail with invalid chart path")
	assert.Contains(t, stderr, "no such file or directory", "Error message should indicate file not found")
}

// --- Phase 9.1 Test ---

func TestInspectCommand_SubchartDiscrepancyWarning(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use a chart known to have subcharts with default images not in the parent values
	// (e.g., kube-prometheus-stack or a similar complex chart fixture)
	// Using complex-chart as it seems designed for this kind of testing.
	// Correct the chart path to use the actual kube-prometheus-stack fixture
	chartPath := harness.GetTestdataPath("charts/kube-prometheus-stack")
	if chartPath == "" { // Check if GetTestdataPath handles non-existence
		t.Skipf("Skipping test, chart directory not found or GetTestdataPath failed for: charts/kube-prometheus-stack")
	}
	harness.SetChartPath(chartPath)

	tests := []struct {
		name          string
		args          []string
		wantWarning   bool
		expectWarning bool // Whether a warning is genuinely expected based on the chart
	}{
		{
			name: "Warning enabled (default), mismatch expected",
			// Base args for inspect with chart path
			args:          []string{"inspect", "--chart-path", chartPath},
			wantWarning:   true,
			expectWarning: true, // Assume complex-chart WILL have a mismatch
		},
		{
			name:          "Warning enabled explicitly, mismatch expected",
			args:          []string{"inspect", "--chart-path", chartPath, "--warn-subchart-discrepancy=true"},
			wantWarning:   true,
			expectWarning: true,
		},
		{
			name:          "Warning disabled, mismatch expected but ignored",
			args:          []string{"inspect", "--chart-path", chartPath, "--warn-subchart-discrepancy=false"},
			wantWarning:   false,
			expectWarning: true,
		},
		// Optional: Add a case with a simple chart where counts match
		// {
		// 	name: "Warning enabled, no mismatch expected (simple chart)",
		// 	chartKey: "charts/simple-chart", // Use a simple chart here
		// 	args: []string{"inspect"}, // Harness will add chart path
		// 	wantWarning:   false,
		// 	expectWarning: false,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute command using harness, capturing both stdout and stderr
			stdout, stderr, err := harness.ExecuteIRRWithStderr(tt.args...)

			// Check for command execution errors (not the warning itself)
			// Allow exit code 0 even if warnings are present in stderr
			assert.NoError(t, err, "Command execution failed unexpectedly")
			assert.NotEmpty(t, stdout, "Expected stdout output from inspect")

			// Check for the specific warning message in stderr
			warningMsg := "Image count mismatch: Analyzer found"
			if tt.wantWarning {
				if tt.expectWarning {
					assert.True(t, strings.Contains(stderr, warningMsg), "Expected subchart discrepancy warning in stderr")
				} else {
					assert.False(t, strings.Contains(stderr, warningMsg), "Did NOT expect subchart discrepancy warning in stderr for this chart")
				}
			} else { // wantWarning == false
				assert.False(t, strings.Contains(stderr, warningMsg), "Did NOT expect subchart discrepancy warning when disabled")
			}
		})
	}
}

// --- End Phase 9.1 Test ---

func TestInspectCommand_GenerateConfigSkeleton(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart fixture
	// Update to use the actual minimal-test chart fixture
	chartPath := harness.GetTestdataPath("charts/minimal-test")
	if chartPath == "" {
		t.Skipf("Skipping test, chart directory not found or GetTestdataPath failed for: charts/minimal-test")
	}

	// Define the expected skeleton file path within the harness temp directory
	// Use the same constant value as defined in inspect.go (since it's not exported)
	const defaultConfigSkeletonFilename = "irr-config.yaml"
	// Correction: The command writes the default skeleton to the CWD, not necessarily tempDir.
	// The test harness executes commands likely from the project root or test dir.
	// We'll check for the default filename relative to the assumed execution CWD.
	// Adjust path to check relative to test execution directory (test/integration)
	skeletonFile := filepath.Join("..", "..", defaultConfigSkeletonFilename)

	// Ensure the potentially existing skeleton file from previous runs is removed
	// Ignore error explicitly if it's because the file doesn't exist
	if err := os.Remove(skeletonFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("Failed to remove pre-existing skeleton file: %v", err)
	}
	defer func() {
		if err := os.Remove(skeletonFile); err != nil && !os.IsNotExist(err) { // Cleanup after test
			t.Logf("Warning: Failed to clean up skeleton file: %v", err) // Use Logf in defer
		}
	}()

	// Run the inspect command with --generate-config-skeleton
	args := []string{
		"inspect",
		"--chart-path", chartPath,
		"--generate-config-skeleton",
	}
	_, stderr, err := harness.ExecuteIRRWithStderr(args...)

	// Check for command execution success
	require.NoError(t, err, "Inspect command with skeleton generation failed: %s", stderr)

	// Verify the skeleton file was created in the expected location (CWD)
	require.FileExists(t, skeletonFile, "Config skeleton file should be created in CWD")

	// Read the skeleton file content
	skeletonBytes, err := os.ReadFile(skeletonFile) // #nosec G304 - Test file in CWD
	require.NoError(t, err, "Failed to read config skeleton file")
	skeletonContent := string(skeletonBytes)

	// Verify the content looks like a valid registry config skeleton
	// Basic checks:
	assert.Contains(t, skeletonContent, "version:", "Skeleton should contain version field")
	assert.Contains(t, skeletonContent, "registries:", "Skeleton should contain registries block")
	assert.Contains(t, skeletonContent, "mappings:", "Skeleton should contain mappings block")
	// Check for non-docker.io image found in minimal-test (e.g., quay.io)
	assert.Contains(t, skeletonContent, "quay.io", "Skeleton should contain detected source registry quay.io")
	// Ensure docker.io is NOT included, as per skeleton generation logic
	assert.NotContains(t, skeletonContent, "docker.io", "Skeleton should NOT contain docker.io")
	assert.Contains(t, skeletonContent, "Quay.io Container Registry", "Skeleton should contain description for quay.io") // Check description added in fix
}
