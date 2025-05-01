//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	stdout, stderr, err := h.ExecuteIRRWithStderr(nil, false,
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
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/parent-test")
	require.NotEqual(t, "", chartPath, "parent-test chart not found")

	h.SetupChart(chartPath)
	h.SetRegistries("test-registry.io/my-project", []string{"docker.io", "quay.io", "custom-repo"})

	outputFileName := "override-parent-test.yaml"
	outputFilePath := filepath.Join(h.tempDir, outputFileName)

	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", outputFilePath,
		// "--debug", // Uncomment for detailed generator logs if needed
	}

	// Execute with context-aware flag enabled for this test
	stdout, stderr, err := h.ExecuteIRRWithStderr(nil, true, args...)
	// Log stderr unconditionally for debugging, especially on failure
	t.Logf("[DEBUG] Stderr from ExecuteIRRWithStderr:\n%s", stderr)
	require.NoError(t, err, "Command failed. Stderr: %s\nStdout: %s", stderr, stdout) // Improved error message

	// Verify the output file exists and parse it
	require.FileExists(t, outputFilePath, "Output file should exist")
	content, err := os.ReadFile(outputFilePath) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	var overrideData map[string]interface{}
	err = yaml.Unmarshal(content, &overrideData)
	require.NoError(t, err, "Failed to unmarshal override YAML from %s", outputFilePath)

	// <<< ADD DEBUG LOGGING HERE >>>
	t.Logf("[DEBUG TEST] Parsed Override Data:\n%#v\n", overrideData)

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

	// Verify parent image override structure (parentImage key)
	parentImageMap, ok := getNestedValue(overrideData, "parentImage")
	assert.True(t, ok, "Should have override map for parentImage")
	parentImageMapTyped, ok := parentImageMap.(map[string]interface{})
	require.True(t, ok, "parentImage override should be a map")

	parentImageRegistry, ok := parentImageMapTyped["registry"].(string)
	assert.True(t, ok, "parentImage map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", parentImageRegistry, "Parent image registry override incorrect")

	parentImageRepo, ok := parentImageMapTyped["repository"].(string)
	assert.True(t, ok, "parentImage map should have repository string")
	// NOTE: Path strategy should include source registry path
	assert.Contains(t, parentImageRepo, "docker.io/parent/app", "Parent image repo override incorrect")

	parentImageTag, ok := parentImageMapTyped["tag"].(string)
	assert.True(t, ok, "parentImage map should have tag string")
	assert.Equal(t, "v1.0.0", parentImageTag, "Parent image tag override incorrect")

	// Verify parent app image override structure (parentAppImage key - formerly 'image')
	parentAppImageMap, ok := getNestedValue(overrideData, "parentAppImage")
	assert.True(t, ok, "Should have override map for parentAppImage")
	parentAppImageMapTyped, ok := parentAppImageMap.(map[string]interface{})
	require.True(t, ok, "parentAppImage override should be a map")

	parentAppImageRegistry, ok := parentAppImageMapTyped["registry"].(string)
	assert.True(t, ok, "parentAppImage map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", parentAppImageRegistry, "Parent app image registry override incorrect")

	parentAppImageRepo, ok := parentAppImageMapTyped["repository"].(string)
	assert.True(t, ok, "parentAppImage map should have repository string")
	// NOTE: Path strategy should include source registry path
	assert.Contains(t, parentAppImageRepo, "docker.io/parent/app", "Parent app image repo override incorrect")

	parentAppImageTag, ok := parentAppImageMapTyped["tag"].(string)
	assert.True(t, ok, "parentAppImage map should have tag string")
	// NOTE: The original value was 'latest', analyzer should preserve this.
	assert.Equal(t, "latest", parentAppImageTag, "Parent app image tag override incorrect - should be 'latest'")

	// Verify child image override structure
	childImageMap, ok := getNestedValue(overrideData, "child.image")
	assert.True(t, ok, "Should have override map for child.image")
	childImageMapTyped, ok := childImageMap.(map[string]interface{})
	require.True(t, ok, "child.image override should be a map")

	childImageRegistry, ok := childImageMapTyped["registry"].(string)
	assert.True(t, ok, "child.image map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", childImageRegistry, "Child image registry override incorrect")

	childImageRepo, ok := childImageMapTyped["repository"].(string)
	assert.True(t, ok, "child.image map should have repository string")
	assert.Contains(t, childImageRepo, "docker.io/library/nginx", "Child image repo override incorrect") // Check combined repo

	childImageTag, ok := childImageMapTyped["tag"].(string)
	assert.True(t, ok, "child.image map should have tag string")
	// NOTE: The analyzer/generator uses the value found ('latest') from child/values.yaml
	assert.Equal(t, "latest", childImageTag, "Child image tag override incorrect - generator uses found value")

	// Verify another-child image override structure
	anotherChildImageMap, ok := getNestedValue(overrideData, "another-child.image")
	assert.True(t, ok, "Should have override map for another-child.image")
	anotherChildImageMapTyped, ok := anotherChildImageMap.(map[string]interface{})
	require.True(t, ok, "another-child.image override should be a map")

	anotherChildImageRegistry, ok := anotherChildImageMapTyped["registry"].(string)
	assert.True(t, ok, "another-child.image map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", anotherChildImageRegistry, "Another-child image registry override incorrect")

	anotherChildImageRepo, ok := anotherChildImageMapTyped["repository"].(string)
	assert.True(t, ok, "another-child.image map should have repository string")
	assert.Contains(t, anotherChildImageRepo, "custom-repo/custom-image", "Another-child image repo override incorrect") // Check combined repo

	anotherChildImageTag, ok := anotherChildImageMapTyped["tag"].(string)
	assert.True(t, ok, "another-child.image map should have tag string")
	assert.Equal(t, "stable", anotherChildImageTag, "Another-child image tag override incorrect")

	// Verify deeply nested image override structure (prometheusImage)
	promImageMap, ok := getNestedValue(overrideData, "another-child.monitoring.prometheusImage")
	assert.True(t, ok, "Should have override map for another-child.monitoring.prometheusImage")
	promImageMapTyped, ok := promImageMap.(map[string]interface{})
	require.True(t, ok, "prometheusImage override should be a map")

	promImageRegistry, ok := promImageMapTyped["registry"].(string)
	assert.True(t, ok, "prometheusImage map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", promImageRegistry, "Prometheus image registry override incorrect")

	promImageRepo, ok := promImageMapTyped["repository"].(string)
	assert.True(t, ok, "prometheusImage map should have repository string")
	assert.Contains(t, promImageRepo, "quayio/prometheus/prometheus", "Prometheus image repo override incorrect") // Check combined repo

	promImageTag, ok := promImageMapTyped["tag"].(string)
	assert.True(t, ok, "prometheusImage map should have tag string")
	assert.Equal(t, "v2.40.0", promImageTag, "Prometheus image tag override incorrect")
}
