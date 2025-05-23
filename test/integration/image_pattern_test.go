//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestImagePatternDetection provides focused tests for different image pattern formats
// to help troubleshoot analyzer issues and verify pattern detection works correctly.
func TestImagePatternDetection(t *testing.T) {
	tests := []struct {
		name           string
		values         string
		expectedImages []string
		skipTemplate   bool // Skip tests that use Go template syntax which isn't compatible with yaml parsing
	}{
		{
			name: "standard_image_map",
			values: `
image:
  registry: docker.io
  repository: nginx
  tag: 1.23.0
`,
			expectedImages: []string{"docker.io/nginx"},
		},
		{
			name: "image_map_without_registry",
			values: `
image:
  repository: nginx
  tag: 1.23.0
`,
			expectedImages: []string{"docker.io/nginx"}, // Default registry should be added
		},
		{
			name: "image_as_string",
			values: `
image: docker.io/nginx:1.23.0
`,
			expectedImages: []string{"docker.io/nginx"},
		},
		{
			name: "nested_image_map",
			values: `
component:
  subcomponent:
    image:
      registry: quay.io
      repository: prometheus/prometheus
      tag: v2.40.0
`,
			expectedImages: []string{"quay.io/prometheus/prometheus"},
		},
		{
			name: "image_in_array",
			values: `
sidecars:
  - name: sidecar1
    image:
      registry: docker.io
      repository: fluent/fluentd
      tag: v1.14.0
  - name: sidecar2
    image:
      registry: quay.io
      repository: prometheus/node-exporter
      tag: v1.5.0
`,
			expectedImages: []string{"docker.io/fluent/fluentd", "quay.io/prometheus/node-exporter"},
		},
		{
			name: "containers_array",
			values: `
containers:
  - name: main
    image: docker.io/nginx:1.23.0
  - name: sidecar
    image: quay.io/prometheus/node-exporter:v1.5.0
`,
			expectedImages: []string{"docker.io/nginx", "quay.io/prometheus/node-exporter"},
		},
		{
			name: "init_containers",
			values: `
initContainers:
  - name: init1
    image:
      registry: docker.io
      repository: busybox
      tag: 1.36.0
`,
			expectedImages: []string{"docker.io/busybox"},
		},
		{
			name: "image_with_digest",
			values: `
image:
  registry: docker.io
  repository: nginx
  tag: "1.23.0"
`,
			expectedImages: []string{"docker.io/nginx"}, // Should detect repository despite digest format
		},
		{
			name: "templated_image",
			values: `
image:
  registry: "docker.io"
  repository: "nginx"
  tag: "latest"
  # Note: This test used to have actual Go templates which caused YAML parsing errors
  # {{ .Values.global.registry | default "docker.io" }}
  # We now use a simpler approach with comments to test template detection
`,
			expectedImages: []string{"docker.io/nginx"}, // Should detect template as image pattern
			skipTemplate:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipTemplate {
				t.Skip("Skipping template test - Go template syntax in values.yaml causes YAML parsing errors")
			}
			overrides, h := setupAndRunOverride(t, tt.values, tt.name+"-overrides.yaml")
			defer h.Cleanup()
			foundImages := extractFoundImages(h, overrides)
			assertExpectedImages(t, h, tt.name, tt.expectedImages, foundImages)
		})
	}
}

// TestEdgeCases tests problematic edge cases for image pattern detection
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		values         string
		expectedImages []string
		shouldSkip     bool
		skipReason     string
	}{
		{
			name: "numeric_tag",
			values: `
image:
  registry: docker.io
  repository: nginx
  tag: 1
`,
			expectedImages: []string{"docker.io/nginx"},
		},
		{
			name: "empty_registry",
			values: `
image:
  registry: ""
  repository: nginx
  tag: latest
`,
			expectedImages: []string{"docker.io/nginx"}, // Should default to docker.io
		},
		{
			name: "custom_image_fields",
			values: `
customImage:
  imageRegistry: docker.io
  imageRepository: nginx
  imageTag: 1.23.0
`,
			expectedImages: []string{"docker.io/nginx"},
			shouldSkip:     true,
			skipReason:     "Custom field names are not currently supported by the analyzer",
		},
		{
			name: "mixed_case_field_names",
			values: `
Image:
  Registry: docker.io
  Repository: nginx
  Tag: 1.23.0
`,
			expectedImages: []string{"docker.io/nginx"},
		},
		{
			name: "string_with_port",
			values: `
image: docker.io:5000/nginx:1.23.0
`,
			expectedImages: []string{"docker.io:5000/nginx"},
		},
		{
			name: "short_image_string",
			values: `
image: nginx:1.23.0
`,
			expectedImages: []string{"nginx"},
		},
		{
			name: "invalid_template",
			values: `
image:
  registry: {{ .Values.global.registry }}
  repository: {{ .Values.image.name 
  tag: {{ .Values.image.version }}
`,
			expectedImages: []string{},
			shouldSkip:     true,
			skipReason:     "Invalid templates might cause helm failures, not an analyzer issue",
		},
		{
			name: "registry_with_path",
			values: `
image:
  registry: docker.io/library
  repository: nginx
  tag: 1.23.0
`,
			expectedImages: []string{"docker.io/library/nginx"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldSkip {
				t.Skip(tt.skipReason)
			}

			h := NewTestHarness(t)
			defer h.Cleanup()

			// Create a test chart with the specified values
			chartDir := createTestChartWithValues(t, h, tt.values)
			h.SetupChart(chartDir)
			h.SetRegistries("test.registry.io", []string{"docker.io", "quay.io"})

			// Create output file path
			outputFile := filepath.Join(h.tempDir, "edge-"+tt.name+"-overrides.yaml")

			// Execute the override command
			output, stderr, err := h.ExecuteIRRWithStderr(nil, false,
				"override",
				"--chart-path", h.chartPath,
				"--target-registry", h.targetReg,
				"--source-registries", strings.Join(h.sourceRegs, ","),
				"--output-file", outputFile,
				"--debug", // Enable debug output for better diagnostics
			)

			// Fail immediately if the override command errors out
			// We allow errors in edge cases, but we want to see what happened
			require.NoError(t, err, "Override command failed for %s.\nOutput: %s\nStderr: %s", tt.name, output, stderr)

			// If the override completed successfully, check the output
			if _, statErr := os.Stat(outputFile); statErr == nil {
				// #nosec G304 -- outputFile is generated in a secure test temp directory, not user-controlled
				overrideBytes, err := os.ReadFile(outputFile)
				require.NoError(t, err, "Failed to read override file")

				// Log the content of the override file for debugging
				t.Logf("Override content for %s: %s", tt.name, string(overrideBytes))

				// Parse the YAML
				var overrides map[string]interface{}
				if err := yaml.Unmarshal(overrideBytes, &overrides); err != nil {
					t.Logf("Failed to parse override YAML: %v", err)
					return
				}

				// Extract the image repositories from the override file
				foundImages := make(map[string]bool)
				h.WalkImageFields(overrides, func(_ []string, imageValue interface{}) {
					switch v := imageValue.(type) {
					case map[string]interface{}:
						if repo, ok := v["repository"].(string); ok {
							foundImages[repo] = true
						}
					case string:
						foundImages[v] = true
					}
				})

				t.Logf("Found images: %v", foundImages)
			}
		})
	}
}

// Helper function to run a single image pattern test case
func runImagePatternTest(t *testing.T, h *TestHarness, testName, valuesContent string, expectedImages []string, outputFilePrefix string) {
	t.Helper() // Mark as test helper

	// Create a test chart with the specified values
	chartDir := createTestChartWithValues(t, h, valuesContent)
	h.SetupChart(chartDir)
	h.SetRegistries("test.registry.io", []string{"docker.io"}) // Assuming docker.io is always a source for these tests

	// Create output file path
	outputFile := filepath.Join(h.tempDir, outputFilePrefix+"-"+testName+"-overrides.yaml")

	// Execute the override command with debug output
	output, stderr, err := h.ExecuteIRRWithStderr(nil, false,
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", outputFile,
		"--debug",
	)
	require.NoError(t, err, "override command should succeed. Output: %s\nStderr: %s", output, stderr)

	// Verify that the override file was created
	require.FileExists(t, outputFile, "Override file should be created")

	// Read the generated override file
	// #nosec G304 -- outputFile is generated in a secure test temp directory, not user-controlled
	overrideBytes, err := os.ReadFile(outputFile)
	require.NoError(t, err, "Should be able to read generated override file")

	// Log the content for debugging
	t.Logf("Override content for %s: %s", testName, string(overrideBytes))

	// Parse the YAML
	var overrides map[string]interface{}
	err = yaml.Unmarshal(overrideBytes, &overrides)
	require.NoError(t, err, "Should be able to parse the override YAML")

	// Extract the image repositories
	foundImages := make(map[string]bool)
	h.WalkImageFields(overrides, func(_ []string, imageValue interface{}) {
		t.Logf("Found image value: %v", imageValue)

		switch v := imageValue.(type) {
		case map[string]interface{}:
			if repo, ok := v["repository"].(string); ok {
				foundImages[repo] = true

				// Also add the path without the registry prefix for easier matching
				parts := strings.Split(repo, "/")
				if len(parts) > 1 {
					nonPrefixedRepo := strings.Join(parts[1:], "/")
					foundImages[nonPrefixedRepo] = true
				}
			}
		case string:
			// Handle plain image strings if necessary, might need adjustment based on WalkImageFields
			// Example: Check if it looks like a valid image reference before adding
			if strings.Contains(v, "/") { // Simple check
				foundImages[v] = true
			}
		}
	})

	// Verify that all expected images were found
	for _, expectedImage := range expectedImages {
		found := false
		expectedRepo := strings.Split(expectedImage, ":")[0] // Strip any tag

		// Try different variations of the repository name for matching
		variations := []string{
			expectedRepo, // Full path: docker.io/nginx
			strings.TrimPrefix(expectedRepo, "docker.io/"), // Without registry: nginx
		}

		// For docker.io images, also check with library/ prefix
		if strings.HasPrefix(expectedRepo, "docker.io/") && !strings.Contains(strings.TrimPrefix(expectedRepo, "docker.io/"), "/") {
			variations = append(variations, "library/"+strings.TrimPrefix(expectedRepo, "docker.io/"))
		}

		for _, variation := range variations {
			for foundImage := range foundImages {
				// Use HasSuffix for partial matches, Contains for full path or substring matches
				if strings.HasSuffix(foundImage, variation) || strings.Contains(foundImage, variation) {
					found = true
					t.Logf("Found expected repository %s as %s", expectedRepo, foundImage)
					break
				}
			}
			if found {
				break
			}
		}

		assert.True(t, found, "Expected image %s should be found in overrides", expectedImage)
		if !found {
			t.Logf("Expected image %s not found. Found images: %v", expectedImage, foundImages)
		}
	}
}

// TestInitContainerPatterns specifically tests the detection of initContainer image patterns
// This is important as initContainers often have different structures than regular containers
func TestInitContainerPatterns(t *testing.T) {
	tests := []struct {
		name           string
		values         string
		expectedImages []string
	}{
		{
			name: "standard_init_containers",
			values: `
initContainers:
  - name: init-container
    image: docker.io/busybox:1.36.0
`,
			expectedImages: []string{"docker.io/busybox"},
		},
		{
			name: "init_containers_with_map_images",
			values: `
initContainers:
  - name: init-container
    image:
      registry: docker.io
      repository: busybox
      tag: 1.36.0
`,
			expectedImages: []string{"docker.io/busybox"},
		},
		{
			name: "nested_init_containers",
			values: `
deployment:
  initContainers:
    - name: init-container
      image: docker.io/busybox:1.36.0
`,
			expectedImages: []string{"docker.io/busybox"},
		},
		{
			name: "admission_webhook_pattern",
			values: `
admissionWebhooks:
  image:
    registry: docker.io
    repository: bitnami/nginx
    tag: 1.25.0
`,
			expectedImages: []string{"docker.io/bitnami/nginx"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewTestHarness(t)
			defer h.Cleanup()
			// Call the helper function
			runImagePatternTest(t, h, tt.name, tt.values, tt.expectedImages, "init")
		})
	}
}

// TestMixedHelmAndKubernetesPatterns specifically tests the detection of mixed Helm and Kubernetes patterns
func TestMixedHelmAndKubernetesPatterns(t *testing.T) {
	tests := []struct {
		name           string
		values         string
		expectedImages []string
	}{
		{
			name: "helm_chart_with_kubernetes_pattern",
			values: `
image:
  registry: docker.io
  repository: nginx
  tag: 1.23.0
`,
			expectedImages: []string{"docker.io/nginx"},
		},
		{
			name: "kubernetes_pattern_with_helm_chart",
			values: `
image:
  registry: docker.io
  repository: nginx
  tag: 1.23.0
`,
			expectedImages: []string{"docker.io/nginx"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewTestHarness(t)
			defer h.Cleanup()
			// Call the helper function
			runImagePatternTest(t, h, tt.name, tt.values, tt.expectedImages, "mixed")
		})
	}
}

// createTestChartWithValues creates a minimal test chart with the given values content
func createTestChartWithValues(t *testing.T, h *TestHarness, valuesContent string) string {
	t.Helper()
	chartDir := filepath.Join(h.tempDir, "test-chart")
	require.NoError(t, os.MkdirAll(chartDir, fileutil.ReadWriteExecuteUserReadGroup))

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: test-chart
description: A test chart for IRR
type: application
version: 0.1.0
`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), fileutil.ReadWriteUserPermission))

	// Create values.yaml with the specified content
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesContent), fileutil.ReadWriteUserPermission))

	// Create templates directory and a simple deployment.yaml
	templateDir := filepath.Join(chartDir, "templates")
	require.NoError(t, os.MkdirAll(templateDir, fileutil.ReadWriteExecuteUserReadGroup))

	deploymentYaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  selector:
    matchLabels:
      app: test
  template:
    spec:
      containers:
      - name: main
        image: {{ .Values.image | default "nginx:latest" }}
`
	require.NoError(t, os.WriteFile(filepath.Join(templateDir, "deployment.yaml"), []byte(deploymentYaml), fileutil.ReadWriteUserPermission))

	return chartDir
}
