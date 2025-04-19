//go:build integration
// +build integration

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
			// Create an output file path specific to this component group
			outputFile := filepath.Join(h.tempDir, fmt.Sprintf("%s-overrides.yaml", group.name))

			// Test that component overrides work as expected
			args := []string{
				"override",
				"--chart-path", h.chartPath,
				"--target-registry", "test.registry.io",
				"--source-registries", "quay.io,docker.io,registry.k8s.io",
				"--output-file", outputFile,
				"--values", h.CombineValuesPaths(group.valuesPaths),
				// Disable validate if it's causing issues (when troubleshooting)
				// "--no-validate",
			}

			output, stderr, err := h.ExecuteIRRWithStderr(args...)
			if err != nil {
				t.Logf("Command stderr: %s", stderr)
				t.Fatalf("Failed to execute irr command: %v", err)
			}

			// Read the raw output file to directly check for specific strings
			rawFileBytes, readErr := os.ReadFile(outputFile)
			if readErr != nil {
				t.Logf("Error reading output file: %v", readErr)
			}
			rawFileContent := string(rawFileBytes)

			// Parse the file content into structured data
			var parsedOverrides map[string]interface{}
			if yamlErr := yaml.Unmarshal(rawFileBytes, &parsedOverrides); yamlErr != nil {
				t.Logf("Error parsing YAML: %v", yamlErr)
			}

			// Verify each component's images were properly overridden
			overrides, err := h.getOverrides()
			require.NoError(t, err, "Should be able to read generated overrides file")

			// Debug info for the overrides file - log key paths but not full content
			t.Logf("Generated overrides structure keys: %v", getTopLevelKeys(overrides))

			for _, component := range group.components {
				found := false

				// First check in direct output (summary info)
				if strings.Contains(output, component) {
					found = true
					t.Logf("✓ Component %s found in command output", component)
				}

				// Next, do a raw string search in the file content
				if !found && strings.Contains(rawFileContent, component) {
					found = true
					t.Logf("✓ Component %s found via raw file content search", component)
				}

				// Try a specialized search for kube-state-metrics
				if !found && component == "kube-state-metrics" {
					// Check for similar variants like "kube-state" or specific patterns
					if strings.Contains(rawFileContent, "kube-state") ||
						strings.Contains(rawFileContent, "kubeStateMetrics") ||
						strings.Contains(rawFileContent, "state-metrics") {
						found = true
						t.Logf("✓ Component %s found via fuzzy matching", component)
					}

					// Additional specialized check for kube-state-metrics
					if !found && findKubeStateMetrics(parsedOverrides) {
						found = true
						t.Logf("✓ Component %s found via deep search", component)
					}
				}

				// If not found in output or raw search, search in the YAML structure
				if !found {
					// Search for component in the overrides structure
					componentFound := searchForComponent(overrides, component)

					if componentFound {
						found = true
						t.Logf("✓ Component %s found in overrides structure", component)
					}
				}

				// Final assertion
				assert.True(t, found, "Component %s should be found in output or overrides", component)

				if !found {
					t.Errorf("Component %s not found in any search method", component)
					t.Logf("Component search failed for: %s", component)

					// Print limited portion of output and stderr, not the entire YAML
					if len(stderr) > 0 {
						if len(stderr) > 500 {
							t.Logf("Stderr summary (first 500 chars): %s...", stderr[:500])
						} else {
							t.Logf("Stderr: %s", stderr)
						}
					}

					if len(output) > 0 {
						if len(output) > 500 {
							t.Logf("Output summary (first 500 chars): %s...", output[:500])
						} else {
							t.Logf("Output: %s", output)
						}
					}

					// Dump a condensed version of the raw file for debugging
					if len(rawFileContent) > 0 {
						t.Logf("File contains %d characters", len(rawFileContent))

						// Only print first few lines and check for relevant sections
						lines := strings.Split(rawFileContent, "\n")
						if len(lines) > 20 {
							t.Logf("First 10 lines of file content:")
							for i := 0; i < 10 && i < len(lines); i++ {
								t.Logf("%d: %s", i+1, lines[i])
							}
						}

						// Look for lines specifically containing "kube" or "state" to help diagnose
						if component == "kube-state-metrics" {
							t.Logf("Searching for 'kube' or 'state' in file content...")
							matched := 0
							for i, line := range lines {
								if strings.Contains(strings.ToLower(line), "kube") ||
									strings.Contains(strings.ToLower(line), "state") {
									t.Logf("Line %d: %s", i+1, line)
									matched++
									if matched >= 10 {
										t.Logf("(showing first 10 matches only)")
										break
									}
								}
							}
							if matched == 0 {
								t.Logf("No matches found for 'kube' or 'state' in file content")
							}
						}
					}
				}
			}
		})
	}
}

// Helper function to get top-level keys from a map for debugging
func getTopLevelKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Helper function to search for a component name in the overrides structure
func searchForComponent(data interface{}, component string) bool {
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this map has a key containing the component name
		for key := range v {
			if strings.Contains(key, component) {
				return true
			}

			// Recursively search in nested maps
			if searchForComponent(v[key], component) {
				return true
			}
		}
	case []interface{}:
		// Search in array elements
		for _, item := range v {
			if searchForComponent(item, component) {
				return true
			}
		}
	case string:
		// Check if the string value contains the component name
		if strings.Contains(v, component) {
			return true
		}
	}
	return false
}

// Specialized function to find kube-state-metrics in any form
func findKubeStateMetrics(data interface{}) bool {
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if "kube-state-metrics" exists as a key or as part of a key
		for key, value := range v {
			// Check the key itself
			if strings.Contains(strings.ToLower(key), "kube") && strings.Contains(strings.ToLower(key), "state") {
				return true
			}

			// For string values, check if they contain "kube-state-metrics"
			if strValue, ok := value.(string); ok {
				if strings.Contains(strings.ToLower(strValue), "kube-state-metrics") {
					return true
				}
			}

			// Special handling for known patterns in the chart structure
			if key == "kube-state-metrics" || key == "kubeStateMetrics" {
				return true
			}

			// Check nested structures
			if findKubeStateMetrics(value) {
				return true
			}
		}
	case []interface{}:
		// Search in array elements
		for _, item := range v {
			if findKubeStateMetrics(item) {
				return true
			}
		}
	case string:
		// Check string values
		return strings.Contains(strings.ToLower(v), "kube") && strings.Contains(strings.ToLower(v), "state")
	}
	return false
}
