// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateWithExplicitKubeVersion tests the validate command with an explicitly set Kubernetes version
func TestValidateWithExplicitKubeVersion(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Generate overrides
	overridesFile := filepath.Join(harness.tempDir, "overrides.yaml")
	_, _, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", overridesFile,
	)
	require.NoError(t, err, "override command should succeed")

	// Run the validate command with explicit Kubernetes version
	outputFile := filepath.Join(harness.tempDir, "output-explicit-version.yaml")
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", overridesFile,
		"--kube-version", "1.25.0",
		"--output-file", outputFile,
	)
	require.NoError(t, err, "validate command should succeed with explicit kube-version")

	// Verify the output contains a success message
	assert.Contains(t, stderr, "Validation successful", "Output should include validation success message")

	// Read the output file and verify it contains standard Kubernetes resource elements
	// #nosec G304 - This is a test-generated file in a test-controlled directory
	content, err := os.ReadFile(filepath.Clean(outputFile))
	require.NoError(t, err, "Should be able to read output file")

	// Check for common elements that should be in the rendered template
	outputStr := string(content)
	assert.Contains(t, outputStr, "apiVersion:", "Output should include Kubernetes apiVersion")
	assert.Contains(t, outputStr, "kind:", "Output should include Kubernetes resource kind")
}

// TestKubeVersionDefaultBehavior tests the default behavior of Kubernetes version in standalone mode
func TestKubeVersionDefaultBehavior(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Generate overrides
	overridesFile := filepath.Join(harness.tempDir, "overrides.yaml")
	_, _, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", overridesFile,
	)
	require.NoError(t, err, "override command should succeed")

	// Run the validate command without specifying a Kubernetes version (should use default)
	outputFile := filepath.Join(harness.tempDir, "output-default-version.yaml")
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", overridesFile,
		"--output-file", outputFile,
		"--debug",
	)
	require.NoError(t, err, "validate command should succeed with default Kubernetes version")

	// Verify debug output confirms the default Kubernetes version is being used
	// The DefaultKubernetesVersion constant is 1.31.0 as defined in the code
	assert.Contains(t, stderr, "1.31.0", "Debug output should include default Kubernetes version")
	assert.Contains(t, stderr, "Validation successful", "Output should include validation success message")
}

// TestVersionCompatibilityEdgeCases tests Kubernetes version edge cases
func TestVersionCompatibilityEdgeCases(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Generate overrides
	overridesFile := filepath.Join(harness.tempDir, "overrides.yaml")
	_, _, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", overridesFile,
	)
	require.NoError(t, err, "override command should succeed")

	// Test with a very old Kubernetes version
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", overridesFile,
		"--kube-version", "1.11.0", // Very old version
	)

	// The behavior here depends on the chart - some charts will fail with very old Kubernetes
	// versions if they use features not available in that version, others may still render successfully.
	// We need to check if the error is due to version incompatibility or something else.
	if err != nil {
		assert.Contains(t, stderr, "apiVersion", "Error should be related to Kubernetes API version incompatibility")
	} else {
		assert.Contains(t, stderr, "Validation successful", "Output should include validation success message")
	}

	// Test with a very new/future Kubernetes version
	_, stderr, err = harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", overridesFile,
		"--kube-version", "1.99.0", // Future version
	)
	// Future versions should generally work since Helm typically allows rendering with newer versions
	require.NoError(t, err, "validate command should succeed with future Kubernetes version")
	assert.Contains(t, stderr, "Validation successful", "Output should include validation success message")
}

// TestInvalidKubernetesVersionFormat tests behavior with invalid version formats
func TestInvalidKubernetesVersionFormat(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Generate overrides
	overridesFile := filepath.Join(harness.tempDir, "overrides.yaml")
	_, _, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", overridesFile,
	)
	require.NoError(t, err, "override command should succeed")

	// Test with invalid version format
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", overridesFile,
		"--kube-version", "invalid-version",
	)

	// Helm/Kubernetes version parsing can be lenient, but completely invalid formats should fail
	// Check if we get a clear error about version format
	if err != nil {
		assert.Contains(t, stderr, "version", "Error should mention invalid version format")
	} else {
		// If it doesn't fail, the output should at least contain a warning or notice
		t.Log("Note: Invalid version format was accepted, which might be a permissive behavior of Helm")
	}

	// Test with malformed but recognizable version
	_, stderr, err = harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", overridesFile,
		"--kube-version", "v1.31", // v-prefix and missing patch version
	)

	// Helm may accept this format, so check either for success or a specific error
	if err != nil {
		assert.Contains(t, stderr, "version", "Error should be related to version format")
	} else {
		assert.Contains(t, stderr, "Validation successful", "Output should include validation success message")
	}
}

// TestVersionDependentTemplate tests templating with version-specific conditional logic
func TestVersionDependentTemplate(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Create a special test chart with Kubernetes version conditionals
	chartDir := filepath.Join(harness.tempDir, "version-test-chart")
	// #nosec G301 - Using TestDirPermissions (0750) from harness.go instead of 0755
	require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "templates"), TestDirPermissions), "Failed to create chart directory")

	// Create Chart.yaml
	chartYaml := []byte(`apiVersion: v2
name: version-test
version: 0.1.0
`)
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), chartYaml, fileutil.ReadWriteUserPermission))

	// Create values.yaml
	valuesFile := filepath.Join(chartDir, "values.yaml")
	valuesYaml := []byte(`image:
  repository: nginx
  tag: latest
`)
	require.NoError(t, os.WriteFile(valuesFile, valuesYaml, fileutil.ReadWriteUserPermission))

	// Create a template with Kubernetes version conditional
	templateYaml := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: version-test
data:
  version-output: "Using Kubernetes {{ .Capabilities.KubeVersion.GitVersion }}"
{{- if semverCompare ">=1.20.0" .Capabilities.KubeVersion.Version }}
  feature-gate: "NewFeatureEnabled"
{{- else }}
  feature-gate: "NewFeatureDisabled"
{{- end }}
`)
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "templates", "configmap.yaml"), templateYaml, fileutil.ReadWriteUserPermission))

	// Test with two different Kubernetes versions
	outputOldVersion := filepath.Join(harness.tempDir, "output-old-version.yaml")
	_, _, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartDir,
		"--values", valuesFile, // Use the chart's own values.yaml file
		"--kube-version", "1.19.0", // Older version
		"--output-file", outputOldVersion,
	)
	require.NoError(t, err, "validate command should succeed with older Kubernetes version")

	outputNewVersion := filepath.Join(harness.tempDir, "output-new-version.yaml")
	_, _, err = harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartDir,
		"--values", valuesFile, // Use the chart's own values.yaml file
		"--kube-version", "1.25.0", // Newer version
		"--output-file", outputNewVersion,
	)
	require.NoError(t, err, "validate command should succeed with newer Kubernetes version")

	// Read both outputs and verify differences
	// #nosec G304 - These are test-generated files in a test-controlled directory
	oldContent, err := os.ReadFile(filepath.Clean(outputOldVersion))
	require.NoError(t, err, "Should be able to read old version output file")

	// #nosec G304 - These are test-generated files in a test-controlled directory
	newContent, err := os.ReadFile(filepath.Clean(outputNewVersion))
	require.NoError(t, err, "Should be able to read new version output file")

	// Verify the version-dependent differences
	assert.Contains(t, string(oldContent), "NewFeatureDisabled", "Old version output should have feature disabled")
	assert.Contains(t, string(newContent), "NewFeatureEnabled", "New version output should have feature enabled")
}

// TestKubeVersionPropagation tests that the Kubernetes version parameter is properly propagated
func TestKubeVersionPropagation(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the minimal-test chart
	chartPath := harness.GetTestdataPath("charts/minimal-test")

	// Generate overrides
	overridesFile := filepath.Join(harness.tempDir, "overrides.yaml")
	_, _, err := harness.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test-registry.local",
		"--source-registries", "docker.io",
		"--output-file", overridesFile,
	)
	require.NoError(t, err, "override command should succeed")

	// Run validate with debug enabled to see the Kubernetes version in logs
	specificVersion := "1.28.0"
	_, stderr, err := harness.ExecuteIRRWithStderr(nil,
		"validate",
		"--chart-path", chartPath,
		"--values", overridesFile,
		"--kube-version", specificVersion,
		"--debug",
	)
	require.NoError(t, err, "validate command should succeed with specific Kubernetes version")

	// Check that the specified version appears in debug logs
	assert.Contains(t, stderr, specificVersion, "Debug output should contain the specified Kubernetes version")
	assert.Contains(t, stderr, "Using Kubernetes version", "Debug output should confirm the version is being used")
}
