// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Helper function to validate a single image override structure
func validateImageOverride(
	t *testing.T,
	overrides map[string]interface{},
	keyPath []string,
	expectedTargetRegistry,
	expectedRepoSubstring string,
) {
	t.Helper()
	value, err := override.GetValueAtPath(overrides, keyPath)
	require.NoError(t, err, fmt.Sprintf("Failed to get value at path %v", keyPath))

	// Navigate to the image map
	current, ok := value.(map[string]interface{})
	require.True(t, ok, fmt.Sprintf("Expected value at path %v to be a map[string]interface{}, but got %T", keyPath, value))

	var imageMap map[string]interface{}
	for i, key := range keyPath {
		if i == len(keyPath)-1 { // Last element is the image map itself
			imageMap, ok = current[key].(map[string]interface{})
			require.True(t, ok, fmt.Sprintf("%s should be a map", strings.Join(keyPath, ".")))
			break
		}
		next, exists := current[key]
		require.True(t, exists, fmt.Sprintf("key '%s' does not exist in path %v", key, keyPath[:i+1]))
		current, ok = next.(map[string]interface{})
		require.True(t, ok, fmt.Sprintf("%s should be a map", strings.Join(keyPath[:i+1], ".")))
	}

	require.NotNil(t, imageMap, "Image map was not found at path %v", keyPath)

	// Validate registry, repository, and tag
	assert.Equal(t, expectedTargetRegistry, imageMap["registry"], fmt.Sprintf("registry mismatch for %s", strings.Join(keyPath, ".")))
	repository, ok := imageMap["repository"].(string)
	require.True(t, ok, fmt.Sprintf("repository should be a string for %s", strings.Join(keyPath, ".")))
	assert.Contains(t, repository, expectedRepoSubstring, fmt.Sprintf("repository mismatch for %s", strings.Join(keyPath, ".")))
	assert.NotEmpty(t, imageMap["tag"], fmt.Sprintf("tag should be present for %s", strings.Join(keyPath, ".")))
}

func TestChartOverrideStructures(t *testing.T) {
	tests := []struct {
		name           string
		chartPath      string
		targetRegistry string
		sourceRegs     []string
		validateFunc   func(t *testing.T, overrides map[string]interface{})
		skip           bool
		skipReason     string
	}{
		{
			name:           "nginx_image_structure",
			chartPath:      "../charts/nginx",
			targetRegistry: "my-registry.example.com",
			sourceRegs:     []string{"docker.io"},
			validateFunc: func(t *testing.T, overrides map[string]interface{}) {
				// Validate main container override structure
				image, ok := overrides["image"].(map[string]interface{})
				require.True(t, ok, "image key should be a map")

				assert.Equal(t, "my-registry.example.com", image["registry"], "registry should be set correctly")
				repository, ok := image["repository"].(string)
				require.True(t, ok, "repository should be a string")
				assert.Contains(t, repository, "dockerio/nginx", "repository should be transformed correctly")
				assert.NotEmpty(t, image["tag"], "tag should be present")
			},
			skip:       true,
			skipReason: "nginx chart not available in test-data/charts",
		},
		{
			name:           "wordpress_image_structure",
			chartPath:      "../charts/wordpress",
			targetRegistry: "my-registry.example.com",
			sourceRegs:     []string{"docker.io"},
			validateFunc: func(t *testing.T, overrides map[string]interface{}) {
				validateImageOverride(t, overrides, []string{"image"}, "my-registry.example.com", "dockerio/wordpress")
				validateImageOverride(t, overrides, []string{"mariadb", "image"}, "my-registry.example.com", "dockerio/mariadb")
			},
			skip:       true,
			skipReason: "wordpress chart not available in test-data/charts",
		},
		{
			name:           "cert_manager_image_structure",
			chartPath:      testutil.GetChartPath("cert-manager"),
			targetRegistry: "my-registry.example.com",
			sourceRegs:     []string{"quay.io"},
			validateFunc: func(t *testing.T, overrides map[string]interface{}) {
				validateImageOverride(t, overrides, []string{"image"}, "my-registry.example.com", "quayio/jetstack/cert-manager-controller")
				validateImageOverride(t, overrides, []string{"webhook", "image"}, "my-registry.example.com", "quayio/jetstack/cert-manager-webhook")
			},
			skip:       true,
			skipReason: "cert-manager chart validation fails with YAML syntax errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip(tt.skipReason)
			}

			harness := NewTestHarness(t)
			defer harness.Cleanup()

			// Setup chart and registries
			harness.SetupChart(tt.chartPath)
			harness.SetRegistries(tt.targetRegistry, tt.sourceRegs)

			// Generate overrides
			err := harness.GenerateOverrides()
			require.NoError(t, err, "GenerateOverrides should succeed")

			// Read and print the override file contents for debugging
			data, err := os.ReadFile(harness.overridePath)
			require.NoError(t, err, "Failed to read override file")
			t.Logf("Generated override file contents:\n%s", string(data))

			var overrides map[string]interface{}
			err = yaml.Unmarshal(data, &overrides)
			require.NoError(t, err, "should be able to parse override YAML")

			// Run chart-specific validation
			tt.validateFunc(t, overrides)

			// Validate with helm template
			err = harness.ValidateOverrides()
			require.NoError(t, err, "helm template validation should succeed")
		})
	}
}
