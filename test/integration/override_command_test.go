//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestOverrideFallbackTriggeredAndSucceeds tests the basic functionality
// of the fallback chart.
func TestOverrideFallbackTriggeredAndSucceeds(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Define the chart path
	chartPath := h.GetTestdataPath("charts/fallback-test")
	if chartPath == "" {
		t.Skip("fallback-test chart not found, skipping test")
	}

	// Run a simplified test using direct override command with --chart-path
	stdout, stderr, err := h.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--log-level", "debug",
	)
	require.NoError(t, err, "override command should succeed")
	t.Logf("Stderr output: %s", stderr)

	// Verify the content contains expected overrides
	assert.Contains(t, stdout, "registry: my-target-registry.com", "Output should include the target registry")
	assert.Contains(t, stdout, "repository: docker.io/library/nginx", "Output should include the image repository")
	assert.Contains(t, stdout, "tag: latest", "Output should include the image tag")
}

// TestOverrideParentChart verifies override generation for a chart with subcharts.
func TestOverrideParentChart(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the existing parent-test chart
	parentTestChartPath := testutil.GetChartPath(t, "parent-test")
	_, err := os.Stat(parentTestChartPath)
	require.NoError(t, err, "parent-test chart should exist")

	// Run the override command on the parent-test chart
	outputFile := filepath.Join(harness.tempDir, "parent-override.yaml")
	args := []string{
		"override",
		"--chart-path", parentTestChartPath,
		"--output-file", outputFile,
		"--target-registry", "test-registry.io/my-project", // Example target
		"--log-level=error",
	}
	_, _, err = harness.ExecuteIRRWithStderr(nil, args...)
	require.NoError(t, err)

	// Verify the output file exists and parse it
	require.FileExists(t, outputFile, "Output file should exist")
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	var overrideData map[string]interface{}
	err = yaml.Unmarshal(content, &overrideData)
	require.NoError(t, err, "Failed to unmarshal override output YAML")

	// --- Verification ---
	// We need helper functions or direct checks to verify nested map values
	// based on the source paths.

	// Helper function to get nested value
	getNestedValue := func(data map[string]interface{}, path string) (interface{}, bool) {
		parts := strings.Split(path, ".")
		current := interface{}(data)
		for _, part := range parts {
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil, false // Not a map, cannot traverse further
			}
			value, exists := m[part]
			if !exists {
				return nil, false // Key not found
			}
			current = value
		}
		return current, true
	}

	// Verify parent image override structure
	parentImageRepo, ok := getNestedValue(overrideData, "image.repository")
	assert.True(t, ok, "Should have override for parent image.repository")
	assert.Contains(t, parentImageRepo.(string), "test-registry.io/my-project", "Parent image repo override incorrect")
	assert.Contains(t, parentImageRepo.(string), "nginx", "Parent image repo override incorrect")

	parentImageTag, ok := getNestedValue(overrideData, "image.tag")
	assert.True(t, ok, "Should have override for parent image.tag")
	assert.Equal(t, "1.23", parentImageTag, "Parent image tag override incorrect")

	// Verify child image override structure
	childImageRepo, ok := getNestedValue(overrideData, "child.image.repository")
	assert.True(t, ok, "Should have override for child.image.repository")
	assert.Contains(t, childImageRepo.(string), "test-registry.io/my-project", "Child image repo override incorrect")
	assert.Contains(t, childImageRepo.(string), "redis", "Child image repo override incorrect")

	childImageTag, ok := getNestedValue(overrideData, "child.image.tag")
	assert.True(t, ok, "Should have override for child.image.tag")
	assert.Equal(t, "7.0", childImageTag, "Child image tag override incorrect")

	// Verify another-child image override structure
	anotherChildImageRepo, ok := getNestedValue(overrideData, "another-child.image.repository")
	assert.True(t, ok, "Should have override for another-child.image.repository")
	assert.Contains(t, anotherChildImageRepo.(string), "test-registry.io/my-project", "Another-child image repo override incorrect")
	assert.Contains(t, anotherChildImageRepo.(string), "custom-repo/custom-image", "Another-child image repo override incorrect")

	anotherChildImageTag, ok := getNestedValue(overrideData, "another-child.image.tag")
	assert.True(t, ok, "Should have override for another-child.image.tag")
	assert.Equal(t, "stable", anotherChildImageTag, "Another-child image tag override incorrect")

	// Verify deeply nested image override structure
	promImageRepo, ok := getNestedValue(overrideData, "another-child.monitoring.prometheusImage.repository")
	assert.True(t, ok, "Should have override for another-child.monitoring.prometheusImage.repository")
	assert.Contains(t, promImageRepo.(string), "test-registry.io/my-project", "Prometheus image repo override incorrect")
	assert.Contains(t, promImageRepo.(string), "prom/prometheus", "Prometheus image repo override incorrect")

	promImageTag, ok := getNestedValue(overrideData, "another-child.monitoring.prometheusImage.tag")
	assert.True(t, ok, "Should have override for another-child.monitoring.prometheusImage.tag")
	assert.Equal(t, "v2.40.0", promImageTag, "Prometheus image tag override incorrect")
}
