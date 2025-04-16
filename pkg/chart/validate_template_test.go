package chart

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// TestNginxValues is a simple YAML with nginx image configuration
	TestNginxValues = `
image:
  repository: nginx
  tag: latest
`
)

// TestValidateHelmTemplateInternal tests the validateHelmTemplateInternal function
func TestValidateHelmTemplateInternal(t *testing.T) {
	t.Run("Valid Template", func(t *testing.T) {
		// Create a temporary chart directory with a simple Chart.yaml and values.yaml
		chartYaml := `
apiVersion: v2
name: validate-test
version: 1.0.0
`
		valuesYaml := TestNginxValues
		chartDir := createTempChartDir(t, "validate-test", chartYaml, valuesYaml)

		// Create a simple override to use for validation
		overrideYaml := []byte(`
image:
  repository: custom-nginx
  tag: 1.19.3
`)

		// Test validation with the override
		err := validateHelmTemplateInternal(chartDir, overrideYaml)
		assert.NoError(t, err, "Template validation should succeed with valid override")
	})

	t.Run("Invalid Template", func(t *testing.T) {
		// This test will need to create a chart with invalid templates
		// For example, a template with syntax errors or invalid references

		// Create a chart directory with an intentionally malformed template
		chartYaml := `
apiVersion: v2
name: invalid-template-test
version: 1.0.0
`
		valuesYaml := TestNginxValues
		chartDir := createTempChartDir(t, "invalid-template-test", chartYaml, valuesYaml)

		// Create an invalid template file with syntax errors
		templatePath := filepath.Join(chartDir, "templates", "invalid.yaml")
		invalidTemplate := `
apiVersion: v1
kind: Pod
metadata:
  name: {{ .Values.doesNotExist.required "missing value" }}
spec:
  containers:
  - name: {{ .invalidSyntax? }}
    image: {{ .Values.image.repository }}:{{ .Values.image.tag }}
`
		err := os.WriteFile(templatePath, []byte(invalidTemplate), FilePermissions)
		require.NoError(t, err, "Failed to write invalid template file")

		// Test validation with a simple override
		overrideYaml := []byte(`
image:
  repository: custom-nginx
  tag: 1.19.3
`)

		// Expect validation to fail
		err = validateHelmTemplateInternal(chartDir, overrideYaml)
		assert.Error(t, err, "Template validation should fail with invalid template")
		assert.Contains(t, err.Error(), "helm template rendering failed", "Error should indicate template rendering failure")
	})

	t.Run("Invalid Override", func(t *testing.T) {
		// Create a simple chart
		chartYaml := `
apiVersion: v2
name: override-test
version: 1.0.0
`
		valuesYaml := TestNginxValues
		chartDir := createTempChartDir(t, "override-test", chartYaml, valuesYaml)

		// Create an invalid YAML override
		invalidOverride := []byte(`
image:
  repository: "nginx
  tag: 1.19.3
`)

		// Test validation with invalid override
		err := validateHelmTemplateInternal(chartDir, invalidOverride)
		assert.Error(t, err, "Template validation should fail with invalid override YAML")
		// The error could be about YAML parsing or template rendering
		assert.Contains(t, err.Error(), "failed to read values", "Error should indicate values file issue")
	})

	t.Run("Empty Chart Path", func(t *testing.T) {
		// Test with empty chart path
		err := validateHelmTemplateInternal("", []byte("image: nginx"))
		assert.Error(t, err, "Template validation should fail with empty chart path")
	})

	t.Run("Empty Override", func(t *testing.T) {
		// Create a simple chart
		chartYaml := `
apiVersion: v2
name: empty-override-test
version: 1.0.0
`
		valuesYaml := TestNginxValues
		chartDir := createTempChartDir(t, "empty-override-test", chartYaml, valuesYaml)

		// Test with empty override
		err := validateHelmTemplateInternal(chartDir, []byte{})
		assert.NoError(t, err, "Template validation should succeed with empty override")
	})

	// Test with valid YAML using a simple Helm chart
	t.Run("valid values", func(t *testing.T) {
		// Create a temporary file for the values
		tempDir := t.TempDir()
		valuesPath := filepath.Join(tempDir, "values.yaml")
		err := os.WriteFile(valuesPath, []byte(TestNginxValues), FilePermissions)
		require.NoError(t, err)

		// ... rest of the test ...
	})
}

func TestReadValuesFile(t *testing.T) {
	// Create a temp file with valid YAML
	tempDir := t.TempDir()
	valuesPath := filepath.Join(tempDir, "values.yaml")
	err := os.WriteFile(valuesPath, []byte(TestNginxValues), FilePermissions)
	require.NoError(t, err)

	// ... rest of the test ...
}

func TestValidateHelmTemplate(t *testing.T) {
	// Test with valid Helm chart and values
	t.Run("successful validation", func(t *testing.T) {
		// Create a temporary file for values.yaml
		tempDir := t.TempDir()
		valuesPath := filepath.Join(tempDir, "values.yaml")
		err := os.WriteFile(valuesPath, []byte(TestNginxValues), FilePermissions)
		require.NoError(t, err)

		// ... rest of the test ...
	})

	// ... existing code ...
}
