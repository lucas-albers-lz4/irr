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
				"state-metrics",
				"kubeStateMetrics",
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

				// Check if the component exists as a top-level key in the overrides
				if _, ok := overrides[component]; ok {
					found = true
					t.Logf("✓ Component %s found as top-level key in overrides", component)
				} else {
					// Fallback: Search deeply if not found at top level (might still be needed for complex structures)
					// We can simplify this later if the normalization proves robust for all cases.
					if searchForComponent(overrides, component) {
						found = true
						t.Logf("✓ Component %s found via deep search in overrides structure", component)
					}
				}

				// Final assertion
				assert.True(t, found, "Component [%s] in group [%s] should be found in the generated overrides structure", component, group.name)

				// Add specific validation for kube-state-metrics structure if found
				if component == "kube-state-metrics" && found {
					ksmBlock, ok := overrides["kube-state-metrics"].(map[string]interface{})
					assert.True(t, ok, "kube-state-metrics override should be a map")
					imageBlock, ok := ksmBlock["image"].(map[string]interface{})
					assert.True(t, ok, "kube-state-metrics.image override should be a map")
					assert.Contains(t, imageBlock, "registry", "kube-state-metrics.image should contain registry")
					assert.Contains(t, imageBlock, "repository", "kube-state-metrics.image should contain repository")
					// Check that tag OR digest exists
					_, hasTag := imageBlock["tag"]
					_, hasDigest := imageBlock["digest"]
					assert.True(t, hasTag || hasDigest, "kube-state-metrics.image should contain tag or digest")
					t.Logf("✓ kube-state-metrics structure validated: %v", imageBlock)
				}

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

// findKubeStateMetrics performs a deep search for kube-state-metrics in the override structure
func findKubeStateMetrics(data interface{}) bool {
	if data == nil {
		return false
	}

	// Common name patterns for kube-state-metrics
	ksmPatterns := []string{
		"kube-state-metrics",
		"kubestatemetrics",
		"state-metrics",
		"kubestateMetrics",
		"k8s/kube-state-metrics",
	}

	switch v := data.(type) {
	case map[string]interface{}:
		// Direct key check for known patterns
		for key, value := range v {
			keyLower := strings.ToLower(key)

			// Check against established patterns
			for _, pattern := range ksmPatterns {
				if strings.Contains(keyLower, pattern) {
					return true
				}
			}

			// Generic check for "kube" + "state" combinations
			if strings.Contains(keyLower, "kube") && strings.Contains(keyLower, "state") {
				return true
			}

			// For repository fields, do extra checks
			if key == "repository" {
				if strValue, ok := value.(string); ok {
					strValueLower := strings.ToLower(strValue)

					// Check for known patterns in repository values
					for _, pattern := range ksmPatterns {
						if strings.Contains(strValueLower, pattern) {
							return true
						}
					}

					// Generic check for "kube" + "state" combinations
					if strings.Contains(strValueLower, "kube") && strings.Contains(strValueLower, "state") {
						return true
					}
				}
			}

			// For any string values, check for kube-state-metrics references
			if strValue, ok := value.(string); ok {
				strValueLower := strings.ToLower(strValue)

				// Check for known patterns in string values
				for _, pattern := range ksmPatterns {
					if strings.Contains(strValueLower, pattern) {
						return true
					}
				}

				// Generic check for "kube" + "state" combinations
				if strings.Contains(strValueLower, "kube") && strings.Contains(strValueLower, "state") {
					return true
				}
			}

			// Recurse into nested structures
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
		// Check string values directly
		strLower := strings.ToLower(v)

		// Check for known patterns in the string
		for _, pattern := range ksmPatterns {
			if strings.Contains(strLower, pattern) {
				return true
			}
		}

		// Generic check for "kube" + "state" combinations
		return strings.Contains(strLower, "kube") && strings.Contains(strLower, "state")
	}
	return false
}
