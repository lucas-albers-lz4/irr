//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestKubePrometheusStack runs component-group tests for the kube-prometheus-stack chart.
// This implements the component-group testing approach described in docs/TESTING-COMPLEX-CHARTS.md
func TestKubePrometheusStack(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Path to test chart
	chartPath := h.GetTestdataPath("charts/kube-prometheus-stack")
	h.SetChartPath(chartPath)

	// Define component groups to test
	componentGroups := []struct {
		name        string
		components  []string
		valuesPaths []string
	}{
		{
			name: "alertmanager",
			components: []string{
				"alertmanager",
			},
			valuesPaths: []string{
				"values/kube-prometheus-stack/alertmanager-values.yaml",
			},
		},
		{
			name: "grafana",
			components: []string{
				"grafana",
			},
			valuesPaths: []string{
				"values/kube-prometheus-stack/grafana-values.yaml",
			},
		},
		{
			name: "prometheus",
			components: []string{
				"prometheus",
			},
			valuesPaths: []string{
				"values/kube-prometheus-stack/prometheus-values.yaml",
			},
		},
		{
			name: "exporters",
			components: []string{
				"prometheus-node-exporter",
				"kube-state-metrics",
			},
			valuesPaths: []string{
				"values/kube-prometheus-stack/exporters-values.yaml",
			},
		},
		{
			name: "operator",
			components: []string{
				"prometheus-operator",
			},
			valuesPaths: []string{
				"values/kube-prometheus-stack/operator-values.yaml",
			},
		},
		{
			name: "thanos",
			components: []string{
				"thanos-ruler",
			},
			valuesPaths: []string{
				"values/kube-prometheus-stack/thanos-values.yaml",
			},
		},
	}

	// Run tests for each component group
	for _, group := range componentGroups {
		t.Run(group.name, func(t *testing.T) {
			// Test that component overrides work as expected
			output, err := h.ExecuteIRR("override", "-f", "testOutput.yaml", "--values", h.CombineValuesPaths(group.valuesPaths))
			if assert.NoError(t, err) {
				// Verify each component's images were properly overridden
				for _, component := range group.components {
					assert.Contains(t, output, component, "Output should contain information about the %s component", component)
				}
			}
		})
	}
}
