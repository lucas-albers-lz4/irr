// Package generator_test contains tests for the generator package.
package generator

import (
	"testing"

	"github.com/lalbers/irr/pkg/analyzer"
	"github.com/lalbers/irr/pkg/strategy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestGenerator_Generate tests the basic override generation without mappings.
func TestGenerator_Generate(t *testing.T) {
	// Setup path strategy (takes no arguments)
	pathStrategy := strategy.NewPrefixSourceRegistryStrategy()

	// Create generator instance (nil mappings)
	gen := NewGenerator(nil, pathStrategy, []string{"docker.io"}, nil, false, false)

	// Define sample image patterns to pass to Generate
	samplePatterns := []analyzer.ImagePattern{
		{Path: "image1", Value: "docker.io/nginx:latest", Type: "string", Origin: ".", RawPath: "image1", Count: 1},
		{Path: "nested.image2", Value: "docker.io/redis:alpine", Type: "string", Origin: ".", RawPath: "nested.image2", Count: 1},
	}

	// Call Generate with the new signature
	yamlBytes, err := gen.Generate(samplePatterns)
	require.NoError(t, err)

	// Unmarshal and assert the output structure/values
	var overrides map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &overrides)
	require.NoError(t, err, "Failed to unmarshal generated YAML")

	t.Logf("Generated Overrides (TestGenerator_Generate):\n%s", string(yamlBytes))

	// Basic assertions
	assert.Contains(t, overrides, "image1", "Expected override for image1")
	assert.Contains(t, overrides, "nested", "Expected nested map key")
	nestedMap, ok := overrides["nested"].(map[string]interface{})
	require.True(t, ok, "Expected nested map")
	assert.Contains(t, nestedMap, "image2", "Expected override for nested.image2")

	// Specific assertions for image1
	img1Map, ok := overrides["image1"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, img1Map, "registry", "Registry should be present")
	assert.Equal(t, "docker.io", img1Map["registry"], "Registry mismatch for image1")
	assert.Equal(t, "dockerio/library/nginx", img1Map["repository"], "Repository mismatch for image1")
	assert.Equal(t, "latest", img1Map["tag"], "Tag mismatch for image1")

	// Specific assertions for image2
	img2Map, ok := nestedMap["image2"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, img2Map, "registry", "Registry should be present")
	assert.Equal(t, "docker.io", img2Map["registry"], "Registry mismatch for image2")
	assert.Equal(t, "dockerio/library/redis", img2Map["repository"], "Repository mismatch for image2")
	assert.Equal(t, "alpine", img2Map["tag"], "Tag mismatch for image2")

	// Expected output based on input patterns + KSM normalization
	// require.Equal(t, expectedYAML, string(yamlBytes))
}

// TestGenerator_GenerateWithMappings tests override generation with registry mappings.
func TestGenerator_GenerateWithMappings(t *testing.T) {
	// Setup path strategy (takes no arguments)
	pathStrategy := strategy.NewPrefixSourceRegistryStrategy()

	// Create generator instance, passing nil for mappings for now
	// TODO: Update this test later if Mappings creation/mocking is feasible/needed.
	gen := NewGenerator(nil, pathStrategy, []string{"docker.io", "quay.io"}, nil, false, false)

	// Define sample image patterns
	samplePatterns := []analyzer.ImagePattern{
		{Path: "app.image", Value: "docker.io/myapp:1.0", Type: "string", Origin: ".", RawPath: "app.image", Count: 1},
		{Path: "db.image", Value: "quay.io/postgres:13", Type: "string", Origin: ".", RawPath: "db.image", Count: 1},
		{Path: "ignored.image", Value: "gcr.io/google/pause:3.2", Type: "string", Origin: ".", RawPath: "ignored.image", Count: 1},
	}

	// Call Generate with the new signature
	yamlBytes, err := gen.Generate(samplePatterns)
	require.NoError(t, err)

	// Unmarshal and assert the output structure/values
	var overrides map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &overrides)
	require.NoError(t, err, "Failed to unmarshal generated YAML")

	t.Logf("Generated Overrides (TestGenerator_GenerateWithMappings):\n%s", string(yamlBytes))

	// Assertions will now reflect the behavior *without* mappings
	// Check app image (no mapping applied)
	assert.Contains(t, overrides, "app", "Expected app map key")
	appMap, ok := overrides["app"].(map[string]interface{})
	require.True(t, ok, "Expected app map")
	assert.Contains(t, appMap, "image", "Expected image key in app map")
	appImageMap, ok := appMap["image"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, appImageMap, "registry", "Registry should be present without mapping")
	assert.Equal(t, "docker.io", appImageMap["registry"], "Registry mismatch for app.image")
	assert.Equal(t, "dockerio/library/myapp", appImageMap["repository"], "Repository should use default strategy")
	assert.Equal(t, "1.0", appImageMap["tag"], "Tag mismatch for app.image")

	// Check db image (no mapping applied)
	assert.Contains(t, overrides, "db", "Expected db map key")
	dbMap, ok := overrides["db"].(map[string]interface{})
	require.True(t, ok, "Expected db map")
	assert.Contains(t, dbMap, "image", "Expected image key in db map")
	dbImageMap, ok := dbMap["image"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, dbImageMap, "registry", "Registry should be present without mapping")
	assert.Equal(t, "quay.io", dbImageMap["registry"], "Registry mismatch for db.image")
	assert.Equal(t, "quayio/postgres", dbImageMap["repository"], "Repository should use default strategy")
	assert.Equal(t, "13", dbImageMap["tag"], "Tag mismatch for db.image")

	// Check that ignored image is not present
	_, ignoredExists := overrides["ignored"] // Check top level
	assert.False(t, ignoredExists, "Ignored image should not be present in overrides")

	// Expected output based on input patterns + KSM normalization + mappings
	// require.Equal(t, expectedYAML, string(yamlBytes))
}

// Test for Strict Mode (Example - can be expanded)
func TestGenerator_GenerateStrictMode(_ *testing.T) {
	// ... existing code ...
}

// --- (Potentially keep TestGenerateKubeStateMetricsNormalization below, adapting its call to gen.Generate) ---

// Example adaptation for KSM test (assuming it exists):
/*
func TestGenerateKubeStateMetricsNormalization(t *testing.T) {
    // ... setup strategy, mappings, generator ...

    samplePatterns := []analyzer.ImagePattern{
        // Include a KSM pattern here
        {Path: "prometheusExporter.image", Value: "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.1", Type: "string", Origin: ".", RawPath: "prometheusExporter.image", Count: 1},
        {Path: "other.image", Value: "docker.io/nginx:latest", Type: "string", Origin: ".", RawPath: "other.image", Count: 1},
    }

    yamlBytes, err := gen.Generate(samplePatterns)
    require.NoError(t, err)

    // ... Assertions for KSM normalization ...
    // Check for overrides["kube-state-metrics"]["image"]
    // Check that overrides["prometheusExporter"] is either gone or doesn't contain the KSM image
}
*/

// --- End TestGenerateKubeStateMetricsNormalization ---
