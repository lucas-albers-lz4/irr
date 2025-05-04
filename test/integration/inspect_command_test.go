//go:build integration

// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	// Use constants for file permissions instead of hardcoded values for consistency and maintainability
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	output, _ /*stderr*/, err := harness.ExecuteIRRWithStderr(nil, false, args...)
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
	output, _ /*stderr*/, err := harness.ExecuteIRRWithStderr(nil, false, args...)
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
	_, _ /*stderr*/, err := harness.ExecuteIRRWithStderr(nil, false, args...)
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

	chartPath := harness.GetTestdataPath("charts/parent-test")
	require.NotEqual(t, "", chartPath, "parent-test chart not found")

	harness.SetupChart(chartPath)
	args := []string{
		"inspect",
		"--chart-path", harness.chartPath,
		"--output-format", "json", // Use JSON for easier parsing in test
		"--context-aware", // Enable context-aware mode
	}

	// Execute with context-aware flag enabled for this test
	stdout, stderr, err := harness.ExecuteIRRWithStderr(nil, true, args...)
	require.NoError(t, err, "Inspect command failed. Stderr: %s", stderr)

	// --- Start: Updated JSON parsing and assertions ---
	// Define structs matching the JSON output structure
	type ImageInfoOutput struct {
		Registry         string `json:"registry"`
		Repository       string `json:"repository"`
		Tag              string `json:"tag,omitempty"`
		Source           string `json:"source"` // Path from originating values file
		OriginalRegistry string `json:"originalRegistry,omitempty"`
		ValuePath        string `json:"valuePath,omitempty"` // Merged value path
	}
	type ChartInfoOutput struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	type ImageAnalysisOutput struct {
		Chart  ChartInfoOutput   `json:"chart"`
		Images []ImageInfoOutput `json:"images"` // Expecting the processed Images list
		// ImagePatterns might also be present, but we focus on Images for verification
	}

	// Parse the JSON output
	var analysisResult ImageAnalysisOutput
	err = json.Unmarshal([]byte(stdout), &analysisResult) // Use json.Unmarshal
	require.NoError(t, err, "Failed to unmarshal inspect output JSON: %s", stdout)

	// Verify chart info
	assert.Equal(t, "parent-test", analysisResult.Chart.Name)

	// Assert specific images and their properties
	foundParentImage := false
	foundChildImage := false
	foundAnotherChildPromImage := false
	foundAnotherChildQuayImage := false

	for _, img := range analysisResult.Images {
		t.Logf("Found Image: Registry=%s, Repository=%s, Tag=%s, Source=%s, ValuePath=%s, OriginalRegistry=%s",
			img.Registry, img.Repository, img.Tag, img.Source, img.ValuePath, img.OriginalRegistry)

		// Check based on ValuePath (merged path) or a combination
		switch img.ValuePath { // Assuming ValuePath holds the merged path like "parentImage"
		case "parentImage": // Top-level image defined with explicit registry
			foundParentImage = true
			assert.Equal(t, "docker.io", img.Registry, "parentImage registry")
			assert.Equal(t, "parent/app", img.Repository, "parentImage repository")
			assert.Equal(t, "v1.0.0", img.Tag, "parentImage tag")
			assert.Equal(t, "values.yaml", img.Source, "parentImage source path") // Expect file path
			assert.Empty(t, img.OriginalRegistry, "parentImage original registry should be empty (same as parsed)")

		case "child.image": // Child image, uses default registry, overridden tag
			foundChildImage = true
			assert.Equal(t, "docker.io", img.Registry, "child.image registry")
			assert.Equal(t, "library/nginx", img.Repository, "child.image repository") // Expect normalized 'library/' prefix
			assert.Equal(t, "1.23", img.Tag, "child.image tag (overridden)")
			// Source reflects origin of final value override (tag from parent values)
			assert.Equal(t, "values.yaml", img.Source, "child.image source path") // Expect file path of final override
			assert.Empty(t, img.OriginalRegistry, "child.image original registry should be empty (default)")

		case "another-child.monitoring.prometheusImage": // Nested subchart image, default registry
			foundAnotherChildPromImage = true
			assert.Equal(t, "docker.io", img.Registry, "prometheusImage registry")
			assert.Equal(t, "prom/prometheus", img.Repository, "prometheusImage repository")
			assert.Equal(t, "v2.40.0", img.Tag, "prometheusImage tag")
			assert.Equal(t, "values.yaml", img.Source, "prometheusImage source path") // Expect file path (overridden in parent)
			assert.Empty(t, img.OriginalRegistry, "prometheusImage original registry should be empty (default)")

		case "another-child.quayImage.image": // Nested subchart image, explicit QUAY.IO registry
			foundAnotherChildQuayImage = true
			assert.Equal(t, "quay.io", img.Registry, "quayImage registry")
			assert.Equal(t, "prometheus/node-exporter", img.Repository, "quayImage repository")
			assert.Equal(t, "v1.5.0", img.Tag, "quayImage tag")
			assert.Equal(t, "values.yaml", img.Source, "quayImage source path") // Expect file path (overridden in parent)
			// This is the key check for context-aware analysis:
			// The final parsed registry is quay.io, and the original was also explicitly quay.io.
			// So, OriginalRegistry should be EMPTY because it doesn't *differ* from the parsed registry.
			// If the context analyzer logic for OriginalRegistry were different (e.g., always populate if subchart),
			// this assertion would change.
			assert.Empty(t, img.OriginalRegistry, "quayImage original registry should be empty (same as parsed)")
		}
	}

	assert.True(t, foundParentImage, "parentImage not found in images list")
	assert.True(t, foundChildImage, "child.image not found in images list")
	assert.True(t, foundAnotherChildPromImage, "another-child.monitoring.prometheusImage not found in images list")
	assert.True(t, foundAnotherChildQuayImage, "another-child.quayImage.image not found in images list")
	// --- End: Updated JSON parsing and assertions ---
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
	_, _ /*stderr*/, err := harness.ExecuteIRRWithStderr(nil, false, args...)
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
		stdout, _, err := h.ExecuteIRRWithStderr(nil, false, "inspect", "--chart-path", chartDir, "--output-format", "yaml")
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
		stdout, _, err := h.ExecuteIRRWithStderr(nil, false, "inspect", "--chart-path", chartDir, "--output-format", "yaml")
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
	stdout, stderr, err := h.ExecuteIRRWithStderr(env, false, args...)

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

// TestInspectAlias verifies that inspect correctly detects images in aliased subcharts
// when --context-aware is enabled.
func TestInspectAlias(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	chartPath := harness.GetTestdataPath("charts/minimal-alias-test")
	require.NotEqual(t, "", chartPath, "minimal-alias-test chart not found")

	harness.SetupChart(chartPath) // Copies chart to temp dir

	// Define arguments for irr inspect - use JSON format for easier parsing
	args := []string{
		"inspect",
		"--chart-path", harness.chartPath,
		"--output-format", "json",
		"--context-aware", // Enable context-aware mode
	}

	// Execute the command
	stdout, stderr, err := harness.ExecuteIRRWithStderr(nil, true, args...)
	t.Logf("Command output - stdout: %s, stderr: %s", stdout, stderr) // Log both outputs for debugging
	require.NoError(t, err, "irr inspect command failed")

	// Parse the JSON output
	type ImageInfo struct {
		Registry       string `json:"registry,omitempty"`
		Repository     string `json:"repository"`
		Tag            string `json:"tag,omitempty"`
		Source         string `json:"source,omitempty"`         // Source file
		ValuePath      string `json:"valuePath,omitempty"`      // Path in merged values
		SourceRegistry string `json:"sourceRegistry,omitempty"` // Detected registry
	}

	type ChartInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	type InspectResult struct {
		Chart  ChartInfo   `json:"chart"`
		Images []ImageInfo `json:"images"`
	}

	var inspectResult InspectResult
	err = json.Unmarshal([]byte(stdout), &inspectResult)
	require.NoError(t, err, "Failed to unmarshal inspect JSON output")

	// Verify chart info
	assert.Equal(t, "minimal-alias-test", inspectResult.Chart.Name, "Chart name should be minimal-alias-test")

	// Find the image in the aliased subchart (should be theAlias.image)
	var aliasImage *ImageInfo
	for i, img := range inspectResult.Images {
		if img.ValuePath == "theAlias.image" {
			aliasImage = &inspectResult.Images[i]
			break
		}
	}

	// Verify the image was found
	require.NotNil(t, aliasImage, "Image in aliased subchart (theAlias.image) not found in inspect results")

	// Verify image details
	assert.Equal(t, "docker.io", aliasImage.Registry, "Registry should be docker.io")
	assert.Contains(t, aliasImage.Repository, "busybox", "Repository should contain busybox")
	assert.Equal(t, "1.0", aliasImage.Tag, "Tag should be 1.0")

	// Verify source info (should point to subchart values.yaml)
	assert.Contains(t, aliasImage.Source, "values.yaml", "Source should point to values.yaml file")

	// Additional assertions for the alias-specific behavior
	// The path should use the alias, not the original chart name
	assert.Equal(t, "theAlias.image", aliasImage.ValuePath, "ValuePath should use the alias name (theAlias)")
}
