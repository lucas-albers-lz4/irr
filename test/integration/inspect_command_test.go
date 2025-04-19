// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	assert.Contains(t, output, "imagePatterns:", "Output should include image patterns section")
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
	assert.Contains(t, contentStr, "imagePatterns:", "Output file should include image patterns section")
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
	assert.Contains(t, stderr, "chart path not found", "Error should mention chart path")
}

func TestInspectOutputFormat(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Run the inspect command with JSON output format
	chartPath := harness.GetTestdataPath("charts/minimal-test")
	output, _, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", chartPath,
		"--output-format", "json",
	)
	require.NoError(t, err, "Inspect command should succeed with JSON output format")

	// Verify the output is valid JSON
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "Output should be valid JSON")

	// Check for expected JSON structure
	chart, ok := result["chart"].(map[string]interface{})
	require.True(t, ok, "JSON should contain chart object")
	assert.Equal(t, "minimal-test", chart["name"], "Chart name should be correct")

	_, ok = result["imagePatterns"]
	require.True(t, ok, "JSON should contain imagePatterns array")
}
