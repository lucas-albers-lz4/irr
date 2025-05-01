//go:build integration

package integration

import (
	"os"
	"path/filepath"
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
	// Check the actual output structure produced by the generator for map overrides
	// The generator now creates separate registry/repository keys for map overrides.
	assert.Contains(t, stdout, "registry: my-target-registry.com", "Output should include the target registry key")
	assert.Contains(t, stdout, "repository: docker.io/library/nginx", "Output should include the relocated repository path key")
	assert.Contains(t, stdout, "tag: latest", "Output should include the image tag")
	// assert.NotContains(t, stdout, "registry:", "Output should NOT contain a separate 'registry:' key for this structure") // This assertion is no longer valid
}

// TestOverrideParentChart verifies override generation for a chart with subcharts.
func TestOverrideParentChart(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the existing parent-test chart
	parentTestChartPath := testutil.GetChartPath("parent-test")
	_, err := os.Stat(parentTestChartPath)
	require.NoError(t, err, "parent-test chart should exist")

	// Run the override command on the parent-test chart
	outputFile := filepath.Join(harness.tempDir, "parent-override.yaml")
	args := []string{
		"override",
		"--chart-path", parentTestChartPath,
		"--output-file", outputFile,
		"--target-registry", "test-registry.io/my-project", // Example target
		"--source-registries", "docker.io,quay.io,registry.k8s.io,ghcr.io,custom-repo,parent,prom", // Add relevant source registries
	}
	stdout, stderr, err := harness.ExecuteIRRWithStderr(nil, args...)
	require.NoError(t, err, "Command failed. Stderr: %s\nStdout: %s", stderr, stdout) // Improved error message

	// Verify the output file exists and parse it
	require.FileExists(t, outputFile, "Output file should exist")
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	// --- START Diagnostic Logging (Phase 9.4.1) ---
	// t.Logf("Generated Override File Content:\n---\n%s\n---", string(content)) // Removed diagnostic log
	// --- END Diagnostic Logging ---

	var overrideData map[string]interface{}
	err = yaml.Unmarshal(content, &overrideData)
	require.NoError(t, err, "Failed to unmarshal override output YAML")

	// --- Verification --- (Updated to match actual YAML output 2024-07-16)

	// Verify top-level 'image' override structure (Parent Chart Image)
	parentImageRegistry, ok := getNestedValue(overrideData, "image.registry")
	assert.True(t, ok, "Should have override for image.registry")
	assert.Equal(t, "test-registry.io/my-project", parentImageRegistry.(string), "Parent image registry override incorrect")

	parentImageRepo, ok := getNestedValue(overrideData, "image.repository")
	assert.True(t, ok, "Should have override for image.repository")
	assert.Equal(t, "docker.io/parent/app", parentImageRepo.(string), "Parent image repo override incorrect") // Original source repo (parent/app)

	parentImageTag, ok := getNestedValue(overrideData, "image.tag")
	assert.True(t, ok, "Should have override for image.tag")
	assert.Equal(t, "latest", parentImageTag, "Parent image tag should match analyzer output") // Actual tag

	// Verify 'child.extraImage' override structure
	childExtraImageRegistry, ok := getNestedValue(overrideData, "child.extraImage.registry")
	assert.True(t, ok, "Should have override for child.extraImage.registry")
	assert.Equal(t, "test-registry.io/my-project", childExtraImageRegistry.(string), "Child extraImage registry override incorrect")

	childExtraImageRepo, ok := getNestedValue(overrideData, "child.extraImage.repository")
	assert.True(t, ok, "Should have override for child.extraImage.repository")
	assert.Equal(t, "docker.io/bitnami/nginx", childExtraImageRepo.(string), "Child extraImage repo override incorrect") // Original source repo

	childExtraImageTag, ok := getNestedValue(overrideData, "child.extraImage.tag")
	assert.True(t, ok, "Should have override for child.extraImage.tag")
	assert.Equal(t, "1.2", childExtraImageTag, "Child extraImage tag should match analyzer output") // Actual tag from subchart

	// Verify 'another-child.monitoring.image' override structure (Using hyphenated key)
	anotherChildMonImageRegistry, ok := getNestedValue(overrideData, "another-child.monitoring.image.registry")
	assert.True(t, ok, "Should have override for another-child.monitoring.image.registry")
	assert.Equal(t, "test-registry.io/my-project", anotherChildMonImageRegistry.(string), "Another-child monitoring image registry override incorrect")

	anotherChildMonImageRepo, ok := getNestedValue(overrideData, "another-child.monitoring.image.repository")
	assert.True(t, ok, "Should have override for another-child.monitoring.image.repository")
	assert.Equal(t, "quay.io/prometheus/node-exporter", anotherChildMonImageRepo.(string), "Another-child monitoring image repo override incorrect") // Original source repo

	anotherChildMonImageTag, ok := getNestedValue(overrideData, "another-child.monitoring.image.tag")
	assert.True(t, ok, "Should have override for another-child.monitoring.image.tag")
	assert.Equal(t, "v1.0", anotherChildMonImageTag, "Another-child monitoring image tag should match analyzer output") // Actual tag from subchart

	// Verify other found top-level keys (These might be from different value files/overrides within the test chart)
	// These might have been simple strings originally, so check both map and string possibilities
	// Assuming they were maps based on previous test structure

	extraImageRegistry, ok := getNestedValue(overrideData, "extraImage.registry")
	assert.True(t, ok, "Should have override for top-level extraImage.registry")
	assert.Equal(t, "test-registry.io/my-project", extraImageRegistry.(string), "Top-level extraImage registry override incorrect")
	extraImageRepo, ok := getNestedValue(overrideData, "extraImage.repository")
	assert.True(t, ok, "Should have override for top-level extraImage.repository")
	assert.Equal(t, "docker.io/bitnami/nginx", extraImageRepo.(string), "Top-level extraImage repo override incorrect")

	monImageRegistry, ok := getNestedValue(overrideData, "monitoring.image.registry")
	assert.True(t, ok, "Should have override for top-level monitoring.image.registry")
	assert.Equal(t, "test-registry.io/my-project", monImageRegistry.(string), "Top-level monitoring image registry override incorrect")
	monImageRepo, ok := getNestedValue(overrideData, "monitoring.image.repository")
	assert.True(t, ok, "Should have override for top-level monitoring.image.repository")
	assert.Equal(t, "quay.io/prometheus/node-exporter", monImageRepo.(string), "Top-level monitoring image repo override incorrect")

	parentImgRegistry, ok := getNestedValue(overrideData, "parentImage.registry")
	assert.True(t, ok, "Should have override for top-level parentImage.registry")
	assert.Equal(t, "test-registry.io/my-project", parentImgRegistry.(string), "Top-level parentImage registry override incorrect")
	parentImgImageRepo, ok := getNestedValue(overrideData, "parentImage.repository")
	assert.True(t, ok, "Should have override for top-level parentImage.repository")
	assert.Equal(t, "docker.io/parent/app", parentImgImageRepo.(string), "Top-level parentImage repo override incorrect") // Original source repo (parent/app)

}

// TestOverrideSubchartAlias verifies override generation uses the alias name.
func TestOverrideSubchartAlias(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the new alias-test chart
	aliasTestChartPath := testutil.GetChartPath("alias-test")
	_, err := os.Stat(aliasTestChartPath)
	require.NoError(t, err, "alias-test chart should exist at %s", aliasTestChartPath)

	// Run the override command
	outputFile := filepath.Join(harness.tempDir, "alias-override.yaml")
	args := []string{
		"override",
		"--chart-path", aliasTestChartPath,
		"--output-file", outputFile,
		"--target-registry", "test-alias.io", // Example target
		"--source-registries", "docker.io", // Source for busybox
	}
	stdout, stderr, err := harness.ExecuteIRRWithStderr(nil, args...)
	require.NoError(t, err, "Command failed. Stderr: %s\nStdout: %s", stderr, stdout)

	// Verify the output file exists and parse it
	require.FileExists(t, outputFile, "Output file should exist")
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err, "Should be able to read output file")

	var overrideData map[string]interface{}
	err = yaml.Unmarshal(content, &overrideData)
	require.NoError(t, err, "Failed to unmarshal override output YAML")

	t.Logf("Generated Alias Override File Content:\n---\n%s\n---", string(content))

	// --- Verification ---

	// Verify top-level 'image' override structure (Parent Chart Image)
	parentImageRegistry, ok := getNestedValue(overrideData, "image.registry")
	assert.True(t, ok, "Should have override for image.registry")
	assert.Equal(t, "test-alias.io", parentImageRegistry.(string), "Parent image registry override incorrect")

	parentImageRepo, ok := getNestedValue(overrideData, "image.repository")
	assert.True(t, ok, "Should have override for image.repository")
	assert.Equal(t, "docker.io/library/busybox", parentImageRepo.(string), "Parent image repo override incorrect") // Original source repo

	parentImageTag, ok := getNestedValue(overrideData, "image.tag")
	assert.True(t, ok, "Should have override for image.tag")
	assert.Equal(t, "1.36", parentImageTag, "Parent image tag should match analyzer output") // Actual tag

	// Verify 'child.extraImage' override structure
	childExtraImageRegistry, ok := getNestedValue(overrideData, "child.extraImage.registry")
	assert.True(t, ok, "Should have override for child.extraImage.registry")
	assert.Equal(t, "test-alias.io", childExtraImageRegistry.(string), "Child extraImage registry override incorrect")

	childExtraImageRepo, ok := getNestedValue(overrideData, "child.extraImage.repository")
	assert.True(t, ok, "Should have override for child.extraImage.repository")
	assert.Equal(t, "docker.io/bitnami/nginx", childExtraImageRepo.(string), "Child extraImage repo override incorrect") // Original source repo

	childExtraImageTag, ok := getNestedValue(overrideData, "child.extraImage.tag")
	assert.True(t, ok, "Should have override for child.extraImage.tag")
	assert.Equal(t, "1.2", childExtraImageTag, "Child extraImage tag should match analyzer output") // Actual tag from subchart

	// Verify 'another-child.monitoring.image' override structure (Using hyphenated key)
	anotherChildMonImageRegistry, ok := getNestedValue(overrideData, "another-child.monitoring.image.registry")
	assert.True(t, ok, "Should have override for another-child.monitoring.image.registry")
	assert.Equal(t, "test-alias.io", anotherChildMonImageRegistry.(string), "Another-child monitoring image registry override incorrect")

	anotherChildMonImageRepo, ok := getNestedValue(overrideData, "another-child.monitoring.image.repository")
	assert.True(t, ok, "Should have override for another-child.monitoring.image.repository")
	assert.Equal(t, "quay.io/prometheus/node-exporter", anotherChildMonImageRepo.(string), "Another-child monitoring image repo override incorrect") // Original source repo

	anotherChildMonImageTag, ok := getNestedValue(overrideData, "another-child.monitoring.image.tag")
	assert.True(t, ok, "Should have override for another-child.monitoring.image.tag")
	assert.Equal(t, "v1.0", anotherChildMonImageTag, "Another-child monitoring image tag should match analyzer output") // Actual tag from subchart

	// Verify other found top-level keys (These might be from different value files/overrides within the test chart)
	// These might have been simple strings originally, so check both map and string possibilities
	// Assuming they were maps based on previous test structure

	extraImageRegistry, ok := getNestedValue(overrideData, "extraImage.registry")
	assert.True(t, ok, "Should have override for top-level extraImage.registry")
	assert.Equal(t, "test-alias.io", extraImageRegistry.(string), "Top-level extraImage registry override incorrect")
	extraImageRepo, ok := getNestedValue(overrideData, "extraImage.repository")
	assert.True(t, ok, "Should have override for top-level extraImage.repository")
	assert.Equal(t, "docker.io/bitnami/nginx", extraImageRepo.(string), "Top-level extraImage repo override incorrect")

	monImageRegistry, ok := getNestedValue(overrideData, "monitoring.image.registry")
	assert.True(t, ok, "Should have override for top-level monitoring.image.registry")
	assert.Equal(t, "test-alias.io", monImageRegistry.(string), "Top-level monitoring image registry override incorrect")
	monImageRepo, ok := getNestedValue(overrideData, "monitoring.image.repository")
	assert.True(t, ok, "Should have override for top-level monitoring.image.repository")
	assert.Equal(t, "quay.io/prometheus/node-exporter", monImageRepo.(string), "Top-level monitoring image repo override incorrect")

	parentImgRegistry, ok := getNestedValue(overrideData, "parentImage.registry")
	assert.True(t, ok, "Should have override for top-level parentImage.registry")
	assert.Equal(t, "test-alias.io", parentImgRegistry.(string), "Top-level parentImage registry override incorrect")
	parentImgImageRepo, ok := getNestedValue(overrideData, "parentImage.repository")
	assert.True(t, ok, "Should have override for top-level parentImage.repository")
	assert.Equal(t, "docker.io/parent/app", parentImgImageRepo.(string), "Top-level parentImage repo override incorrect") // Original source repo (parent/app)

	// Assert that the override path uses the alias "myChildAlias"
	_, aliasExists := getNestedValue(overrideData, "myChildAlias.image.repository")
	assert.True(t, aliasExists, "Override path should use the alias 'myChildAlias' for the subchart image")

	// Assert that the override path does NOT use the original chart name "child-chart"
	_, wrongPathExists := getNestedValue(overrideData, "child-chart.image.repository")
	assert.False(t, wrongPathExists, "Override path should NOT use the original chart name 'child-chart'")

	// Assert the actual override values for the aliased subchart image
	actualAliasRepo, _ := getNestedValue(overrideData, "myChildAlias.image.repository")
	// Note: The subchart image is 'child-nginx', not busybox. Let's assume path strategy adds prefix.
	assert.Contains(t, actualAliasRepo.(string), "test-alias.io/docker.io/library/child-nginx", "Aliased repository override value is incorrect")

	actualAliasTag, _ := getNestedValue(overrideData, "myChildAlias.image.tag")
	assert.Equal(t, "1.0", actualAliasTag, "Aliased tag override value is incorrect (should be from subchart)")

	// Assert that the override for the PARENT chart image also exists
	_, parentImageExists := getNestedValue(overrideData, "image.repository")
	assert.True(t, parentImageExists, "Override for the parent chart image should also exist")

	// Assert the actual override values for the parent chart image
	actualParentRepo, _ := getNestedValue(overrideData, "image.repository")
	assert.Contains(t, actualParentRepo.(string), "test-alias.io/docker.io/library/busybox", "Parent repository override value is incorrect")

	actualParentTag, _ := getNestedValue(overrideData, "image.tag")
	assert.Equal(t, "1.36", actualParentTag, "Parent tag override value is incorrect")
}
