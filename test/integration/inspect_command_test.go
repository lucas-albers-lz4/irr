// Package integration contains integration tests for the irr CLI tool.
package integration

import (
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

	// Create a parent chart with a subchart
	chartDir := filepath.Join(harness.tempDir, "parent-chart")
	subchartDir := filepath.Join(chartDir, "charts", "child")

	// Create parent chart structure
	require.NoError(t, os.MkdirAll(chartDir, fileutil.ReadWriteExecuteUserReadGroup))
	parentChartYaml := `apiVersion: v2
name: parent-chart
version: 0.1.0
dependencies:
- name: child
  version: 0.1.0
  repository: ""`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(parentChartYaml), fileutil.ReadWriteUserPermission))

	// Create parents values.yaml with image reference
	parentValuesYaml := `image:
  repository: nginx
  tag: "1.23"`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(parentValuesYaml), fileutil.ReadWriteUserPermission))

	// Create subchart structure
	require.NoError(t, os.MkdirAll(subchartDir, fileutil.ReadWriteExecuteUserReadGroup))
	childChartYaml := `apiVersion: v2
name: child
version: 0.1.0`
	require.NoError(t, os.WriteFile(filepath.Join(subchartDir, "Chart.yaml"), []byte(childChartYaml), fileutil.ReadWriteUserPermission))

	// Create subchart values file with image reference
	childValuesYaml := `image:
  repository: redis
  tag: "7.0"`
	require.NoError(t, os.WriteFile(filepath.Join(subchartDir, "values.yaml"), []byte(childValuesYaml), fileutil.ReadWriteUserPermission))

	// Create subchart templates directory and deployment file
	require.NoError(t, os.MkdirAll(filepath.Join(subchartDir, "templates"), fileutil.ReadWriteExecuteUserReadGroup))
	childDeploymentYaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: child
spec:
  template:
    spec:
      containers:
      - name: redis
        image: {{ .Values.image.repository }}:{{ .Values.image.tag }}`
	require.NoError(t, os.WriteFile(filepath.Join(subchartDir, "templates", "deployment.yaml"), []byte(childDeploymentYaml), fileutil.ReadWriteUserPermission))

	// Create parent templates directory and deployment file
	require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "templates"), fileutil.ReadWriteExecuteUserReadGroup))
	parentDeploymentYaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: parent
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: {{ .Values.image.repository }}:{{ .Values.image.tag }}`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(parentDeploymentYaml), fileutil.ReadWriteUserPermission))

	// Set the chart path in the harness
	harness.chartPath = chartDir

	// Run the inspect command on the parent chart
	args := []string{
		"inspect",
		"--chart-path", chartDir,
		"--output-format", "yaml",
		"--log-level=error",
	}
	output, _ /*stderr*/, err := harness.ExecuteIRRWithStderr(nil, args...)
	require.NoError(t, err)

	// Verify the output contains parent chart information (should find the nginx image)
	assert.Contains(t, output, "parent-chart", "Output should include parent chart name")
	assert.True(t,
		strings.Contains(output, "nginx") ||
			strings.Contains(output, "library/nginx") ||
			strings.Contains(output, "docker.io/nginx") ||
			strings.Contains(output, "docker.io/library/nginx"),
		"Output should include parent chart's nginx image")

	// Subchart images might not be detected without additional processing
	// (which is a known limitation mentioned in Phase 9 of TODO.md)
	// So don't assert on child chart's redis image
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
