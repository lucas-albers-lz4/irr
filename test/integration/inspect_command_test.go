// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	output, stderr, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", chartPath,
		"--output-format", "yaml",
	)
	require.NoError(t, err, "Inspect command should succeed: %s", stderr)

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
	output, stderr, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", chartPath,
		"--source-registries", "docker.io",
		"--output-format", "yaml",
	)
	require.NoError(t, err, "Inspect command should succeed with source registry filter: %s", stderr)

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

	_, stderr, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", chartPath,
		"--output-file", outputFile,
		"--output-format", "yaml",
	)
	require.NoError(t, err, "Inspect command should succeed with output to file: %s", stderr)

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
	require.NoError(t, os.MkdirAll(chartDir, 0o750))
	parentChartYaml := `apiVersion: v2
name: parent-chart
version: 0.1.0
dependencies:
- name: child
  version: 0.1.0
  repository: ""`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(parentChartYaml), 0o600))

	// Create parents values.yaml with image reference
	parentValuesYaml := `image:
  repository: nginx
  tag: "1.23"`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(parentValuesYaml), 0o600))

	// Create subchart structure
	require.NoError(t, os.MkdirAll(subchartDir, 0o750))
	childChartYaml := `apiVersion: v2
name: child
version: 0.1.0`
	require.NoError(t, os.WriteFile(filepath.Join(subchartDir, "Chart.yaml"), []byte(childChartYaml), 0o600))

	// Create subchart values file with image reference
	childValuesYaml := `image:
  repository: redis
  tag: "7.0"`
	require.NoError(t, os.WriteFile(filepath.Join(subchartDir, "values.yaml"), []byte(childValuesYaml), 0o600))

	// Create subchart templates directory and deployment file
	require.NoError(t, os.MkdirAll(filepath.Join(subchartDir, "templates"), 0o750))
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
	require.NoError(t, os.WriteFile(filepath.Join(subchartDir, "templates", "deployment.yaml"), []byte(childDeploymentYaml), 0o600))

	// Create parent templates directory and deployment file
	require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "templates"), 0o750))
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
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(parentDeploymentYaml), 0o600))

	// Set the chart path in the harness
	harness.chartPath = chartDir

	// Run the inspect command on the parent chart
	output, stderr, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", chartDir,
		"--output-format", "yaml",
	)
	require.NoError(t, err, "Inspect command should succeed with parent chart: %s", stderr)

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
	_, stderr, err := harness.ExecuteIRRWithStderr(
		"inspect",
		"--chart-path", harness.chartPath,
		"--generate-config-skeleton",
		"--output-file", skeletonFile,
		"--output-format", "yaml",
	)
	require.NoError(t, err, "Inspect command should succeed with generate-config-skeleton option: %s", stderr)

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
	err := os.MkdirAll(templatesDir, 0o750)
	require.NoError(t, err)

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: pattern-test
version: 0.1.0`
	err = os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0o600)
	require.NoError(t, err)

	// Create values.yaml with digest image
	valuesYaml := `
image:
  repository: quay.io/prometheus/prometheus
  tag: "v2.45.0@sha256:2c6c2a0e0d2d0a4d9b36c598c6d4310c0eb9b5aa0f6b3d4554be3c8f7a8c8f8"
`
	err = os.WriteFile(valuesFile, []byte(valuesYaml), 0o600)
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
	err = os.WriteFile(filepath.Join(templatesDir, "deployment.yaml"), []byte(deploymentYaml), 0o600)
	require.NoError(t, err)

	t.Run("image_with_digest", func(t *testing.T) {
		// Run inspect command on chart
		stdout, _, err := h.ExecuteIRRWithStderr("inspect", "--chart-path", chartDir, "--output-format", "yaml")
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
	err := os.MkdirAll(templatesDir, 0o750)
	require.NoError(t, err)

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: advanced-pattern-test
version: 0.1.0`
	err = os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0o600)
	require.NoError(t, err)

	// Create values.yaml with complex image structures
	valuesYaml := `
images:
  registry: docker.io
  repository: library/nginx
  tag: 1.19.0

templateImage: '{{ .Values.images.registry }}/{{ .Values.images.repository }}:{{ .Values.images.tag }}'
`
	err = os.WriteFile(valuesFile, []byte(valuesYaml), 0o600)
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
	err = os.WriteFile(filepath.Join(templatesDir, "deployment.yaml"), []byte(deploymentYaml), 0o600)
	require.NoError(t, err)

	t.Run("template_string_image_references", func(t *testing.T) {
		// Run inspect command on chart
		stdout, _, err := h.ExecuteIRRWithStderr("inspect", "--chart-path", chartDir, "--output-format", "yaml")
		require.NoError(t, err, "Inspect command should succeed for template string image references")

		// Check general content
		assert.Contains(t, stdout, "chart:")
		assert.Contains(t, stdout, "name: advanced-pattern-test")
		assert.Contains(t, stdout, "docker.io")
		assert.Contains(t, stdout, "library")
	})
}
