//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestKubePrometheusStack runs override tests for the full kube-prometheus-stack chart,
// validating that images in both the parent and subcharts are correctly rewritten.
func TestKubePrometheusStack(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Path to test chart
	chartPath := h.GetTestdataPath("charts/kube-prometheus-stack")
	// No longer setting chart path globally in harness, pass directly to command

	// Define expected image paths and their simplified repository prefixes after override
	// These paths are based on the structure found in the chart's values/templates.
	expectedImagePaths := map[string]string{
		// Parent chart images (if any directly defined) - adjust if needed
		// "someParent.image": "test.registry.io/parent/image",

		// Subchart images (using default aliases)
		"alertmanager.alertmanagerSpec.image.repository":                "test.registry.io/prometheus-alertmanager/alertmanager",
		"grafana.image.repository":                                      "test.registry.io/grafana/grafana", // Check grafana subchart path
		"grafana.initChownData.image.repository":                        "test.registry.io/busybox",         // Example of init container image
		"kube-state-metrics.image.repository":                           "test.registry.io/kube-state-metrics/kube-state-metrics",
		"prometheus-node-exporter.image.repository":                     "test.registry.io/prometheus/node-exporter",
		"prometheus-operator.image.repository":                          "test.registry.io/prometheus-operator/prometheus-operator",
		"prometheus-operator.prometheusConfigReloader.image.repository": "test.registry.io/prometheus-operator/prometheus-config-reloader",
		"prometheus.prometheusSpec.image.repository":                    "test.registry.io/prometheus/prometheus", // Prometheus itself
		// "prometheus-operator.thanosImage.repository": Needs check if thanos sidecar is enabled by default / in test values
		// Add more paths as needed based on default enabled components
	}

	// Output file path for the full chart override
	outputFile := filepath.Join(h.tempDir, "kube-prometheus-stack-full-overrides.yaml")

	// Run irr override for the whole chart
	// Include --values pointing to the chart's default values.yaml for Helm rendering
	defaultValuesPath := filepath.Join(chartPath, "values.yaml")
	args := []string{
		"override",
		"--chart-path", chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", "quay.io,docker.io,registry.k8s.io,ghcr.io", // Added ghcr.io commonly used
		"--output-file", outputFile,
		"--values", defaultValuesPath, // Use default values for rendering
		"--no-validate",     // Disable internal validation to inspect output
		"--log-level=debug", // Enable debug logging for troubleshooting
	}

	output, stderr, err := h.ExecuteIRRWithStderr(args...)
	t.Logf("irr override command output:\n%s", output)
	t.Logf("irr override command stderr:\n%s", stderr)
	require.NoError(t, err, "irr override command failed")

	// Read the generated overrides file
	rawFileBytes, readErr := os.ReadFile(outputFile)
	require.NoError(t, readErr, "Failed to read output override file: %s", outputFile)
	rawFileContent := string(rawFileBytes)
	t.Logf("Generated Override File Content:\n%s", rawFileContent)

	// Parse the overrides file
	var parsedOverrides map[string]interface{}
	yamlErr := yaml.Unmarshal(rawFileBytes, &parsedOverrides)
	require.NoError(t, yamlErr, "Failed to parse generated YAML overrides")
	require.NotNil(t, parsedOverrides, "Parsed overrides should not be nil")

	// --- Validate Specific Image Paths ---
	validationFailed := false
	for path, expectedRepoPrefix := range expectedImagePaths {
		pathKeys := strings.Split(path, ".")
		// Get the value at the specified path (should be the repository string)
		repoValueRaw := walkPath(parsedOverrides, pathKeys...)

		// Assert that the path exists and the value is a string
		repoValue, ok := repoValueRaw.(string)
		if assert.True(t, ok, "Path [%s] not found or not a string in overrides", path) {
			// Assert that the repository string starts with the expected rewritten prefix
			assert.True(t, strings.HasPrefix(repoValue, expectedRepoPrefix),
				"Path [%s]: Expected repository prefix '%s', but got '%s'", path, expectedRepoPrefix, repoValue)
		} else {
			validationFailed = true
		}

		// Additionally, check if the corresponding tag exists at the same level
		// e.g., if path is "grafana.image.repository", check "grafana.image.tag"
		if len(pathKeys) > 0 {
			tagPathKeys := append(pathKeys[:len(pathKeys)-1], "tag")
			tagValueRaw := walkPath(parsedOverrides, tagPathKeys...)
			_, tagOk := tagValueRaw.(string)
			// We just check existence and type, not the specific tag value (unless needed)
			if !assert.True(t, tagOk, "Path [%s]: Corresponding tag path [%s] not found or not a string", path, strings.Join(tagPathKeys, ".")) {
				validationFailed = true
			}
		}
	}

	// If any specific validation failed, log additional debug info
	if validationFailed {
		t.Logf("One or more path validations failed. Dumping parsed overrides structure keys: %v", getTopLevelKeys(parsedOverrides))
		// Optionally dump more details if needed
	}

	// --- Validate Inspect Command Output ---
	t.Log("Validating 'irr inspect' output...")

	// Define expected inspect paths and their origins
	expectedInspectPatterns := map[string]string{
		"alertmanager.alertmanagerSpec.image.repository": "alertmanager",
		"grafana.image.repository":                       "grafana",
		"grafana.initChownData.image.repository":         "grafana", // Check origin for init containers
		"kube-state-metrics.image.repository":            "kube-state-metrics",
		"prometheus-node-exporter.image.repository":      "prometheus-node-exporter",
		"prometheus-operator.image.repository":           "prometheus-operator",
		"prometheus.prometheusSpec.image.repository":     "prometheus",
		// Add more as needed
	}

	inspectArgs := []string{
		"inspect",
		"--chart-path", chartPath,
		"--values", defaultValuesPath,
		"--output-format=json", // Request JSON for easier parsing
		"--log-level=debug",
	}

	inspectOutputStr, inspectStderr, inspectErr := h.ExecuteIRRWithStderr(inspectArgs...)
	t.Logf("irr inspect command output:\n%s", inspectOutputStr)
	t.Logf("irr inspect command stderr:\n%s", inspectStderr)
	require.NoError(t, inspectErr, "irr inspect command failed")

	var inspectAnalysis struct {
		Images []struct {
			Path   string `json:"path"`
			Origin string `json:"origin"`
			Value  string `json:"value"` // We can check the image string too
			Type   string `json:"type"`
		} `json:"images"`
	}

	unmarshalErr := json.Unmarshal([]byte(inspectOutputStr), &inspectAnalysis)
	require.NoError(t, unmarshalErr, "Failed to unmarshal inspect JSON output")

	// --- Debug: Print specific pattern --- >
	for _, img := range inspectAnalysis.Images {
		if strings.HasPrefix(img.Path, "prometheus-node-exporter.image") {
			t.Logf("DEBUG: Found prometheus-node-exporter pattern: Path='%s', Origin='%s', Value='%s', Type='%s'",
				img.Path, img.Origin, img.Value, img.Type)
		}
	}
	// < --- End Debug ---

	// Create a map for quick lookup of found patterns by path
	foundInspectPatterns := make(map[string]struct {
		Origin string
		Value  string
		Type   string
	})
	for _, img := range inspectAnalysis.Images {
		foundInspectPatterns[img.Path] = struct {
			Origin string
			Value  string
			Type   string
		}{Origin: img.Origin, Value: img.Value, Type: img.Type}
	}

	inspectValidationFailed := false
	for expectedPath, expectedOrigin := range expectedInspectPatterns {
		foundPattern, found := foundInspectPatterns[expectedPath]
		if assert.True(t, found, "Inspect: Expected path [%s] not found in analysis results", expectedPath) {
			assert.Equal(t, expectedOrigin, foundPattern.Origin,
				"Inspect: Path [%s]: Expected origin '%s', but got '%s'", expectedPath, expectedOrigin, foundPattern.Origin)
			// Optionally: Add checks for foundPattern.Value or foundPattern.Type
		} else {
			inspectValidationFailed = true
		}
	}

	/* // Temporarily disable inspect validation as override is the focus
	if inspectValidationFailed {
		t.Logf("One or more inspect path validations failed. Dumping found paths: %v", getMapKeys(foundInspectPatterns))
	}
	*/
}

// --- Validation Helper Functions ---

// walkPath navigates a nested map structure using a slice of keys.
// Returns the value found at the end of the path, or nil if the path is invalid.
func walkPath(data interface{}, keys ...string) interface{} {
	current := data
	for _, key := range keys {
		mapCurrent, ok := current.(map[string]interface{})
		if !ok {
			return nil // Path segment is not a map or doesn't exist
		}
		value, exists := mapCurrent[key]
		if !exists {
			return nil // Key not found
		}
		current = value
	}
	return current
}

// getTopLevelKeys returns a slice of top-level keys from a map for debugging
func getTopLevelKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getMapKeys returns a slice of keys from any map[string]T for debugging
func getMapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
