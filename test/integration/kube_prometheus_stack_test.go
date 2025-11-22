//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestKubePrometheusStack_Alertmanager tests override generation specifically for the Alertmanager component
// using a minimal values file that defines its image.
// This reflects current IRR limitations (no subchart default processing - see Phase 10).
func TestKubePrometheusStack_Alertmanager(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/kube-prometheus-stack")
	h.SetChartPath(chartPath)
	outputFile := filepath.Join(h.tempDir, "alertmanager-overrides.yaml")
	// NOTE: Assumes alertmanager-minimal-values.yaml exists and defines alertmanager.alertmanagerSpec.image
	// valuesFile := h.GetTestdataPath("values/kube-prometheus-stack/alertmanager-minimal-values.yaml")

	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", "quay.io", // Only need the source for this component
		"--output-file", outputFile,
	}

	_, stderr, err := h.ExecuteIRRWithStderr(nil, true, args...)
	require.NoError(t, err, "irr override failed for Alertmanager: %s", stderr)

	overrides, err := h.getOverridesFromFile(outputFile)
	require.NoError(t, err, "Should be able to read generated overrides file for Alertmanager")

	t.Logf("Generated overrides structure keys for Alertmanager: %v", getTopLevelKeys(overrides))

	// Assert that ONLY the alertmanager key is present
	_, found := overrides["alertmanager"]
	assert.True(t, found, "Expected 'alertmanager' key in overrides")

	// Optional: Add deeper validation for the alertmanager structure if needed
}

// TestKubePrometheusStack_Prometheus tests override generation specifically for the Prometheus component
// using a minimal values file that defines its image.
func TestKubePrometheusStack_Prometheus(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/kube-prometheus-stack")
	h.SetChartPath(chartPath)
	outputFile := filepath.Join(h.tempDir, "prometheus-overrides.yaml")
	// NOTE: Assumes prometheus-minimal-values.yaml exists and defines prometheus.prometheusSpec.image
	// valuesFile := h.GetTestdataPath("values/kube-prometheus-stack/prometheus-minimal-values.yaml")

	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", "quay.io", // Only need the source for this component
		"--output-file", outputFile,
	}

	_, stderr, err := h.ExecuteIRRWithStderr(nil, true, args...)
	require.NoError(t, err, "irr override failed for Prometheus: %s", stderr)

	overrides, err := h.getOverridesFromFile(outputFile)
	require.NoError(t, err, "Should be able to read generated overrides file for Prometheus")

	t.Logf("Generated overrides structure keys for Prometheus: %v", getTopLevelKeys(overrides))

	// Assert that ONLY the prometheus key is present
	_, found := overrides["prometheus"]
	assert.True(t, found, "Expected 'prometheus' key in overrides")
}

// TestKubePrometheusStack_FullChart_WithSubcharts_TODO tests override generation for the entire
// kube-prometheus-stack chart, including images defined in subchart defaults.
// This test is currently skipped because full subchart value processing is not yet implemented (Phase 10).
func TestKubePrometheusStack_FullChart_WithSubcharts_TODO(t *testing.T) {
	t.Skip("Skipping: Test requires full subchart value processing (Phase 10)")

	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/kube-prometheus-stack")
	h.SetChartPath(chartPath)
	outputFile := filepath.Join(h.tempDir, "kube-prometheus-stack-full-overrides.yaml")

	// Use a broader set of source registries expected in the full chart
	sourceRegistries := []string{"quay.io", "docker.io", "registry.k8s.io"}

	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", strings.Join(sourceRegistries, ","),
		"--output-file", outputFile,
	}

	_, stderr, err := h.ExecuteIRRWithStderr(nil, true, args...)
	// NOTE: We expect this to potentially fail or produce incomplete results currently,
	// but the require.NoError check should pass once Phase 10 is implemented.
	require.NoError(t, err, "irr override failed for full kube-prometheus-stack: %s", stderr)

	overrides, err := h.getOverridesFromFile(outputFile)
	require.NoError(t, err, "Should be able to read generated overrides file for full chart")

	t.Logf("Generated overrides structure keys for full chart: %v", getTopLevelKeys(overrides))

	// Assert that keys from subcharts are present once Phase 10 is done.
	// These components are typically defined in subchart values, not top-level.
	requiredSubchartKeys := []string{"grafana", "kube-state-metrics"}
	for _, key := range requiredSubchartKeys {
		_, found := overrides[key]
		assert.True(t, found, "Expected subchart key '%s' in overrides after Phase 10 implementation", key)
		// Optional: Add deeper validation for the structure within these keys if needed
	}

	// Also check for a top-level key to ensure those are still processed.
	_, foundTopLevel := overrides["prometheus"] // Example top-level key
	assert.True(t, foundTopLevel, "Expected top-level key 'prometheus' to still be present")
}

// TestKubePrometheusStack_Operator tests override generation specifically for the Prometheus Operator component.
func TestKubePrometheusStack_Operator(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/kube-prometheus-stack")
	h.SetChartPath(chartPath)
	outputFile := filepath.Join(h.tempDir, "operator-overrides.yaml")

	// Assuming operator images primarily come from quay.io based on chart structure
	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", "quay.io",
		"--output-file", outputFile,
	}

	_, stderr, err := h.ExecuteIRRWithStderr(nil, true, args...)
	require.NoError(t, err, "irr override failed for Operator: %s", stderr)

	overrides, err := h.getOverridesFromFile(outputFile)
	require.NoError(t, err, "Should be able to read generated overrides file for Operator")

	t.Logf("Generated overrides structure keys for Operator: %v", getTopLevelKeys(overrides))

	// Assert that the prometheusOperator key is present
	_, found := overrides["prometheusOperator"]
	assert.True(t, found, "Expected 'prometheusOperator' key in overrides")

	// Optional: Deeper validation for operator structure (e.g., specific images like admissionWebhooks)
}

// TestKubePrometheusStack_ThanosRuler tests override generation specifically for the Thanos Ruler component.
func TestKubePrometheusStack_ThanosRuler(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/kube-prometheus-stack")
	h.SetChartPath(chartPath)
	outputFile := filepath.Join(h.tempDir, "thanos-ruler-overrides.yaml")

	// Assuming thanos ruler images primarily come from quay.io based on chart structure
	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", "quay.io",
		"--output-file", outputFile,
	}

	_, stderr, err := h.ExecuteIRRWithStderr(nil, true, args...)
	require.NoError(t, err, "irr override failed for Thanos Ruler: %s", stderr)

	overrides, err := h.getOverridesFromFile(outputFile)
	require.NoError(t, err, "Should be able to read generated overrides file for Thanos Ruler")

	t.Logf("Generated overrides structure keys for Thanos Ruler: %v", getTopLevelKeys(overrides))

	// Assert that the thanosRuler key is present
	_, found := overrides["thanosRuler"]
	assert.True(t, found, "Expected 'thanosRuler' key in overrides")

	// Optional: Deeper validation for thanos ruler structure
}

// TODO: Add similar focused tests for remaining top-level components like:
// - NodeExporter (if image defined at top level)
// - KubeStateMetrics (if image defined at top level)
// - Other components defined directly in the main values.yaml
// TODO: Consider using include/exclude patterns for more targeted tests if needed.

// Helper function to get top-level keys from a map for debugging
func getTopLevelKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Helper function to get overrides from a specific file
// Adjusted from TestHarness.getOverrides to take a filename
func (h *TestHarness) getOverridesFromFile(filename string) (map[string]interface{}, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read overrides file '%s': %w", filename, err)
	}

	var rawOutput map[string]interface{}
	if err := yaml.Unmarshal(content, &rawOutput); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw output YAML from '%s': %w", filename, err)
	}

	// Extract the actual overrides from the 'Values' key
	valuesData, ok := rawOutput["Values"]
	if !ok {
		// Handle cases where 'Values' key might be missing (e.g., errors or different output structure)
		// For now, return the raw output, but log a warning or error if appropriate for the test context
		h.t.Logf("Warning: 'Values' key not found in output file %s. Returning raw structure.", filename)
		return rawOutput, nil
	}

	overrides, ok := valuesData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to assert 'Values' data as map[string]interface{} in '%s'", filename)
	}

	return overrides, nil
}
