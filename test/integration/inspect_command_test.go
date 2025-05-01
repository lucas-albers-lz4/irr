//go:build integration

// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	// Use constants for file permissions instead of hardcoded values for consistency and maintainability
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestInspectCommand(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Set up a minimal test chart
	setupMinimalTestChart(t, harness)
	chartPath := harness.chartPath

	// Run the inspect command on the minimal-test chart
	args := []string{
		"inspect",
		"--chart-path", chartPath,
		"--output-format", "yaml",
		"--log-level=error",
	}
	output, _ /*stderr*/, err := harness.ExecuteIRRWithStderr(nil, args...)
	require.NoError(t, err)

	// Verify the output contains expected sections
	assert.Contains(t, output, "chart:", "Output should include chart section")
	assert.Contains(t, output, "imagePatterns:", "Output should include image patterns section")
	// Verify the nginx image is detected in some form
	assert.True(t,
		strings.Contains(output, "nginx") ||
			strings.Contains(output, "library/nginx") ||
			strings.Contains(output, "docker.io/nginx") ||
			strings.Contains(output, "docker.io/library/nginx"),
		"Output should include the nginx image")
}

func TestInspectWithSourceRegistryFilter(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Set up a minimal test chart
	setupMinimalTestChart(t, harness)
	chartPath := harness.chartPath

	// Run the inspect command with a specific source registry filter
	args := []string{
		"inspect",
		"--chart-path", chartPath,
		"--source-registries", "docker.io",
		"--output-format", "yaml",
		"--log-level=error",
	}
	output, _ /*stderr*/, err := harness.ExecuteIRRWithStderr(nil, args...)
	require.NoError(t, err)

	// Verify output - should detect the nginx image whether it has a docker.io prefix or not
	assert.True(t,
		strings.Contains(output, "nginx") ||
			strings.Contains(output, "library/nginx") ||
			strings.Contains(output, "docker.io/nginx") ||
			strings.Contains(output, "docker.io/library/nginx"),
		"Output should include the nginx image")
}

func TestInspectOutputToFile(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Set up a minimal test chart
	setupMinimalTestChart(t, harness)
	chartPath := harness.chartPath

	outputFile := filepath.Join(harness.tempDir, "inspect-output.yaml")

	args := []string{
		"inspect",
		"--chart-path", chartPath,
		"--output-file", outputFile,
		"--output-format", "yaml",
		"--log-level=error",
	}
	_, _ /*stderr*/, err := harness.ExecuteIRRWithStderr(nil, args...)
	require.NoError(t, err)

	// Verify the file exists and has content
	require.FileExists(t, outputFile, "Output file should exist")
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

	// Use the existing parent-test chart for better coverage
	parentTestChartPath := testutil.GetChartPath("parent-test") // Use testutil helper

	// Ensure the chart path exists before proceeding
	_, err := os.Stat(parentTestChartPath)
	require.NoError(t, err, "parent-test chart should exist")

	// Run the inspect command on the parent-test chart
	outputFile := filepath.Join(harness.tempDir, "parent-inspect.yaml")
	args := []string{
		"inspect",
		"--chart-path", parentTestChartPath,
		"--output-format", "yaml",
		"--output-file", outputFile, // Output to file for easier parsing
	}
	_, _ /*stderr*/, err = harness.ExecuteIRRWithStderr(map[string]string{"LOG_LEVEL": "debug"}, args...)
	require.NoError(t, err)

	// Verify the output file exists and parse it
	require.FileExists(t, outputFile, "Output file should exist")
	content, err := os.ReadFile(outputFile) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	// --- START Diagnostic Logging (Phase 9.4.2) ---
	t.Logf("Generated Inspect File Content:\n---\n%s\n---", string(content))
	// --- END Diagnostic Logging ---

	// Define a struct to unmarshal the relevant parts of the inspect output
	// Updated struct to match the actual ImageInfo fields in the output
	type ImageAnalysisOutput struct {
		Images []struct {
			Registry   string `yaml:"registry"`
			Repository string `yaml:"repository"`
			Tag        string `yaml:"tag,omitempty"`
			Digest     string `yaml:"digest,omitempty"`
			SourcePath string `yaml:"source"` // Match the 'source' key used in ImageInfo
		} `yaml:"images"`
	}

	var analysisResult ImageAnalysisOutput
	err = yaml.Unmarshal(content, &analysisResult)
	require.NoError(t, err, "Failed to unmarshal inspect output YAML")

	// Verify expected images and their source paths based on Analyzer output (Phase 9.3/9.4)
	expectedImages := map[string]string{ // reference -> expected sourcePath(s)
		// Values map directly to analyzer log output (IMAGE APPEND entries)
		// Using the parsed/normalized reference string as the key.
		"parent/nginx:latest":                      "image",
		"prom/prometheus:v2.0.0":                   "child.image",
		"docker.io/bitnami/nginx:latest":           "child.extraImage; global.extraImage", // Found in two places
		"quay.io/coreos/kube-state-metrics:v2.5.0": "another-child.image",
		"quay.io/prometheus/node-exporter:latest":  "another-child.monitoring.image; global.monitoring.image", // Found in two places
		"docker.io/parent/app:v1.0.0":              "global.parentImage",
		// Note: The test setup might not have all the images listed in the original assertion.
		// These are the ones confirmed by the analyzer logs for parent-test chart.
	}

	foundImages := make(map[string]string)
	for _, img := range analysisResult.Images { // img now has Registry, Repository, Tag, Digest, SourcePath fields
		// --- START Reverted Key Reconstruction (Phase 9.4.2 v3) ---
		// Reconstruct the reference string reliably using image.Reference logic
		// Create a temporary ref to utilize its String() method for canonical representation
		tempRef := image.Reference{
			Registry:   img.Registry, // Use fields directly from the unmarshaled struct
			Repository: img.Repository,
			Tag:        img.Tag,
			Digest:     img.Digest,
		}
		refKey := tempRef.String() // Get canonical string representation
		// --- END Reverted Key Reconstruction ---

		// Use the canonical reference string as the key and SourcePath as the value
		foundImages[refKey] = img.SourcePath
	}

	// --- DEBUGGING ---
	t.Logf("Expected images map: %v", expectedImages)
	t.Logf("Found images map: %v", foundImages) // Re-enable debug log
	// --- END DEBUGGING ---

	for ref, expectedPath := range expectedImages {
		actualPath, ok := foundImages[ref]
		assert.True(t, ok, "Expected image not found in inspect output: %s", ref)
		if ok {
			// Split paths by semicolon and sort for comparison
			expectedPaths := strings.Split(expectedPath, "; ")
			actualPaths := strings.Split(actualPath, "; ")
			sort.Strings(expectedPaths)
			sort.Strings(actualPaths)
			assert.Equal(t, expectedPaths, actualPaths, "Incorrect sourcePath(s) for image: %s", ref)
		}
	}

	// Assert the total number of unique images found matches the expectation
	assert.Len(t, analysisResult.Images, len(expectedImages), "Unexpected number of unique images found")
}

func TestInspectGenerateConfigSkeleton(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Set up a minimal test chart
	setupMinimalTestChart(t, harness)
	skeletonFile := filepath.Join(harness.tempDir, "irr-config.yaml")

	// Run the inspect command with generate-config-skeleton option
	args := []string{
		"inspect",
		"--chart-path", harness.chartPath,
		"--generate-config-skeleton",
		"--output-file", skeletonFile,
		"--output-format", "yaml",
		"--log-level=error",
	}
	_, _ /*stderr*/, err := harness.ExecuteIRRWithStderr(nil, args...)
	require.NoError(t, err)

	// Check that the file exists
	require.FileExists(t, skeletonFile, "Config skeleton file should exist")

	// Check the generated file
	content, err := os.ReadFile(skeletonFile) // #nosec G304
	require.NoError(t, err, "Should be able to read config skeleton file")

	// Verify the output contains the skeleton configuration
	contentStr := string(content)
	// Either mappings or registries.mappings should be present (depending on whether
	// structured or legacy format is used)
	assert.True(t,
		strings.Contains(contentStr, "mappings:") ||
			strings.Contains(contentStr, "registries:"),
		"Config skeleton should include a mappings or registries section")
}

// TestImagePatternProcessing tests inspecting images with various patterns
func TestImagePatternProcessing(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Set up a chart directory with values.yaml and deployment.yaml
	chartDir := filepath.Join(h.tempDir, "pattern-test")
	valuesFile := filepath.Join(chartDir, "values.yaml")
	templatesDir := filepath.Join(chartDir, "templates")

	// Create templates directory
	err := os.MkdirAll(templatesDir, fileutil.ReadWriteExecuteUserReadGroup)
	require.NoError(t, err)

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: pattern-test
version: 0.1.0`
	err = os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), fileutil.ReadWriteUserPermission)
	require.NoError(t, err)

	// Create values.yaml with digest image
	valuesYaml := `
image:
  repository: quay.io/prometheus/prometheus
  tag: "v2.45.0@sha256:2c6c2a0e0d2d0a4d9b36c598c6d4310c0eb9b5aa0f6b3d4554be3c8f7a8c8f8"
`
	err = os.WriteFile(valuesFile, []byte(valuesYaml), fileutil.ReadWriteUserPermission)
	require.NoError(t, err)

	// Create deployment.yaml with image reference
	deploymentYaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: digest-test
spec:
  selector:
    matchLabels:
      app: digest-test
  template:
    metadata:
      labels:
        app: digest-test
    spec:
      containers:
      - name: main
        image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
`
	err = os.WriteFile(filepath.Join(templatesDir, "deployment.yaml"), []byte(deploymentYaml), fileutil.ReadWriteUserPermission)
	require.NoError(t, err)

	t.Run("image_with_digest", func(t *testing.T) {
		// Run inspect command on chart
		stdout, _, err := h.ExecuteIRRWithStderr(nil, "inspect", "--chart-path", chartDir, "--output-format", "yaml")
		require.NoError(t, err, "Inspect command should succeed for image with digest")

		// Check general content
		assert.Contains(t, stdout, "chart:")
		assert.Contains(t, stdout, "name: pattern-test")
		assert.Contains(t, stdout, "version: 0.1.0")
		assert.Contains(t, stdout, "quay.io/prometheus/prometheus")
		assert.Contains(t, stdout, "sha256:2c6c2a0e0d2d0a4d9b36c598c6d4310c0eb9b5aa0f6b3d4554be3c8f7a8c8f8")
	})
}

// TestAdvancedImagePatterns tests complex image reference patterns used in templates
func TestAdvancedImagePatterns(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Set up a chart directory with values.yaml and deployment.yaml
	chartDir := filepath.Join(h.tempDir, "advanced-pattern-test")
	valuesFile := filepath.Join(chartDir, "values.yaml")
	templatesDir := filepath.Join(chartDir, "templates")

	// Create templates directory
	err := os.MkdirAll(templatesDir, fileutil.ReadWriteExecuteUserReadGroup)
	require.NoError(t, err)

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: advanced-pattern-test
version: 0.1.0`
	err = os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), fileutil.ReadWriteUserPermission)
	require.NoError(t, err)

	// Create values.yaml with complex image structures
	valuesYaml := `
images:
  registry: docker.io
  repository: library/nginx
  tag: 1.19.0

templateImage: '{{ .Values.images.registry }}/{{ .Values.images.repository }}:{{ .Values.images.tag }}'
`
	err = os.WriteFile(valuesFile, []byte(valuesYaml), fileutil.ReadWriteUserPermission)
	require.NoError(t, err)

	// Create deployment.yaml with template string image reference
	deploymentYaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: template-test
spec:
  selector:
    matchLabels:
      app: template-test
  template:
    metadata:
      labels:
        app: template-test
    spec:
      containers:
      - name: main
        image: "{{ .Values.templateImage }}"
      - name: separate
        image: "{{ .Values.images.registry }}/{{ .Values.images.repository }}:{{ .Values.images.tag }}"
`
	err = os.WriteFile(filepath.Join(templatesDir, "deployment.yaml"), []byte(deploymentYaml), fileutil.ReadWriteUserPermission)
	require.NoError(t, err)

	t.Run("template_string_image_references", func(t *testing.T) {
		// Run inspect command on chart
		stdout, _, err := h.ExecuteIRRWithStderr(nil, "inspect", "--chart-path", chartDir, "--output-format", "yaml")
		require.NoError(t, err, "Inspect command should succeed for template string image references")

		// Check general content
		assert.Contains(t, stdout, "chart:")
		assert.Contains(t, stdout, "name: advanced-pattern-test")
		assert.Contains(t, stdout, "docker.io")
		assert.Contains(t, stdout, "library")
	})
}

// TestInspectCommand_HelmMode simulates running inspect as a Helm plugin.
func TestInspectCommand_HelmMode(t *testing.T) {
	releaseName := "inspect-helm-mode-release"
	namespace := "helm-mode-ns"

	h := NewTestHarness(t)
	defer h.Cleanup()

	// Define environment variables to simulate Helm plugin environment
	env := map[string]string{
		"HELM_BIN":        "helm",
		"HELM_PLUGIN_DIR": "/fake/plugins/irr",
		"HELM_NAMESPACE":  namespace, // Set the namespace env var too
		// Add other relevant Helm env vars if needed for the specific code path
	}

	// Test: Run inspect command with release name and namespace args
	// We expect this to FAIL because there's no real Tiller/Kubernetes to get the release from.
	// However, we want to verify it TRIED to run in plugin mode.
	args := []string{
		"inspect",
		releaseName,
		"--namespace", namespace, // Also pass flag, though env var might take precedence depending on logic
		"--log-level=debug", // Enable debug logs to check behavior
	}
	stdout, stderr, err := h.ExecuteIRRWithStderr(env, args...)

	// Assertions
	require.Error(t, err, "irr inspect in Helm mode should fail without a real Helm environment")
	assert.Contains(t, stderr, "IRR running as Helm plugin", "Stderr should contain log indicating Helm plugin mode")
	assert.Contains(t, stderr, "Running inspect in Helm plugin mode for release", "Stderr should contain log attempting Helm release inspection")
	// Check for a specific error related to fetching the release (might vary)
	// Check if stderr contains either the "release not found" error (common locally) or the "cluster unreachable" error (common in CI)
	assert.True(t,
		strings.Contains(stderr, "release: not found") || strings.Contains(stderr, "Kubernetes cluster unreachable"),
		"Stderr should indicate failure to get the Helm release (either 'release: not found' or 'Kubernetes cluster unreachable')",
	)

	// Stdout might be empty or contain partial info depending on where the error occurred
	_ = stdout // Avoid unused variable error
}
