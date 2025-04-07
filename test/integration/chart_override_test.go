// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"os"
	"testing"

	"github.com/lalbers/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

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
				// Validate WordPress container override structure
				image, ok := overrides["image"].(map[string]interface{})
				require.True(t, ok, "image key should be a map")

				assert.Equal(t, "my-registry.example.com", image["registry"], "registry should be set correctly")
				repository, ok := image["repository"].(string)
				require.True(t, ok, "repository should be a string")
				assert.Contains(t, repository, "dockerio/wordpress", "repository should be transformed correctly")
				assert.NotEmpty(t, image["tag"], "tag should be present")

				// Validate MariaDB container override structure
				mariadb, ok := overrides["mariadb"].(map[string]interface{})
				require.True(t, ok, "mariadb key should be a map")

				mariaImage, ok := mariadb["image"].(map[string]interface{})
				require.True(t, ok, "mariadb.image key should be a map")

				assert.Equal(t, "my-registry.example.com", mariaImage["registry"], "mariadb registry should be set correctly")
				repository, ok = mariaImage["repository"].(string)
				require.True(t, ok, "mariadb repository should be a string")
				assert.Contains(t, repository, "dockerio/mariadb", "mariadb repository should be transformed correctly")
				assert.NotEmpty(t, mariaImage["tag"], "mariadb tag should be present")
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
				// Validate cert-manager container override structure
				image, ok := overrides["image"].(map[string]interface{})
				require.True(t, ok, "image key should be a map")

				assert.Equal(t, "my-registry.example.com", image["registry"], "registry should be set correctly")
				repository, ok := image["repository"].(string)
				require.True(t, ok, "repository should be a string")
				assert.Contains(t, repository, "quayio/jetstack/cert-manager-controller", "repository should be transformed correctly")
				assert.NotEmpty(t, image["tag"], "tag should be present")

				// Validate webhook container override structure
				webhook, ok := overrides["webhook"].(map[string]interface{})
				require.True(t, ok, "webhook key should be a map")

				webhookImage, ok := webhook["image"].(map[string]interface{})
				require.True(t, ok, "webhook.image key should be a map")

				assert.Equal(t, "my-registry.example.com", webhookImage["registry"], "webhook registry should be set correctly")
				repository, ok = webhookImage["repository"].(string)
				require.True(t, ok, "webhook repository should be a string")
				assert.Contains(t, repository, "quayio/jetstack/cert-manager-webhook", "webhook repository should be transformed correctly")
				assert.NotEmpty(t, webhookImage["tag"], "webhook tag should be present")
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
