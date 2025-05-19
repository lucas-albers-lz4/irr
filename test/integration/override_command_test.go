//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestOverrideFallbackTriggeredAndSucceeds tests the basic functionality
// of the fallback chart.
func TestOverrideFallbackTriggeredAndSucceeds(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Define the chart path
	chartPath := h.GetTestdataPath("charts/fallback-test")
	if chartPath == "" {
		t.Skip("fallback-test chart not found, skipping test")
	}

	// Run a simplified test using direct override command with --chart-path
	stdout, stderr, err := h.ExecuteIRRWithStderr(nil, false,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--log-level", "debug",
	)
	require.NoError(t, err, "override command should succeed")
	t.Logf("Stderr output: %s", stderr)

	// Verify the content contains expected overrides
	assert.Contains(t, stdout, "registry: my-target-registry.com", "Output should include the target registry")
	assert.Contains(t, stdout, "repository: docker.io/library/nginx", "Output should include the image repository")
	assert.Contains(t, stdout, "tag: latest", "Output should include the image tag")
}

// TestOverrideParentChart verifies override generation for a chart with subcharts.
func TestOverrideParentChart(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/parent-test")
	require.NotEqual(t, "", chartPath, "parent-test chart not found")

	h.SetupChart(chartPath)
	h.SetRegistries("test-registry.io/my-project", []string{"docker.io", "quay.io", "custom-repo"})

	outputFileName := "override-parent-test.yaml"
	outputFilePath := filepath.Join(h.tempDir, outputFileName)

	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", outputFilePath,
		// "--debug", // Uncomment for detailed generator logs if needed
	}

	// Execute with context-aware flag enabled for this test
	stdout, stderr, err := h.ExecuteIRRWithStderr(nil, true, args...)
	// Log stderr unconditionally for debugging, especially on failure
	t.Logf("[DEBUG] Stderr from ExecuteIRRWithStderr:\n%s", stderr)
	require.NoError(t, err, "Command failed. Stderr: %s\nStdout: %s", stderr, stdout) // Improved error message

	// Verify the output file exists and parse it
	require.FileExists(t, outputFilePath, "Output file should exist")
	content, err := os.ReadFile(outputFilePath) // #nosec G304
	require.NoError(t, err, "Should be able to read output file")

	var overrideData map[string]interface{}
	err = yaml.Unmarshal(content, &overrideData)
	require.NoError(t, err, "Failed to unmarshal override YAML from %s", outputFilePath)

	// <<< ADD DEBUG LOGGING HERE >>>
	t.Logf("[DEBUG TEST] Parsed Override Data:\n%#v\n", overrideData)

	// --- Verification ---
	// We need helper functions or direct checks to verify nested map values
	// based on the source paths.

	// Helper function to get nested value
	getNestedValue := func(data map[string]interface{}, path string) (interface{}, bool) {
		parts := strings.Split(path, ".")
		current := interface{}(data)
		for _, part := range parts {
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil, false // Not a map, cannot traverse further
			}
			value, exists := m[part]
			if !exists {
				return nil, false // Key not found
			}
			current = value
		}
		return current, true
	}

	// Verify parent image override structure (parentImage key)
	parentImageMap, ok := getNestedValue(overrideData, "parentImage")
	assert.True(t, ok, "Should have override map for parentImage")
	parentImageMapTyped, ok := parentImageMap.(map[string]interface{})
	require.True(t, ok, "parentImage override should be a map")

	parentImageRegistry, ok := parentImageMapTyped["registry"].(string)
	assert.True(t, ok, "parentImage map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", parentImageRegistry, "Parent image registry override incorrect")

	parentImageRepo, ok := parentImageMapTyped["repository"].(string)
	assert.True(t, ok, "parentImage map should have repository string")
	// Path strategy preserves registry with dots
	assert.Contains(t, parentImageRepo, "docker.io/parent/app", "Parent image repo override incorrect")

	parentImageTag, ok := parentImageMapTyped["tag"].(string)
	assert.True(t, ok, "parentImage map should have tag string")
	assert.Equal(t, "v1.0.0", parentImageTag, "Parent image tag override incorrect")

	// Verify parent app image override structure (parentAppImage key - formerly 'image')
	parentAppImageMap, ok := getNestedValue(overrideData, "parentAppImage")
	assert.True(t, ok, "Should have override map for parentAppImage")
	parentAppImageMapTyped, ok := parentAppImageMap.(map[string]interface{})
	require.True(t, ok, "parentAppImage override should be a map")

	parentAppImageRegistry, ok := parentAppImageMapTyped["registry"].(string)
	assert.True(t, ok, "parentAppImage map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", parentAppImageRegistry, "Parent app image registry override incorrect")

	parentAppImageRepo, ok := parentAppImageMapTyped["repository"].(string)
	assert.True(t, ok, "parentAppImage map should have repository string")
	// Path strategy preserves registry with dots
	assert.Contains(t, parentAppImageRepo, "docker.io/parent/app", "Parent app image repo override incorrect")

	parentAppImageTag, ok := parentAppImageMapTyped["tag"].(string)
	assert.True(t, ok, "parentAppImage map should have tag string")
	// NOTE: The original value was 'latest', analyzer should preserve this.
	assert.Equal(t, "latest", parentAppImageTag, "Parent app image tag override incorrect - should be 'latest'")

	// Verify child image override structure
	childImageMap, ok := getNestedValue(overrideData, "child.image")
	assert.True(t, ok, "Should have override map for child.image")
	childImageMapTyped, ok := childImageMap.(map[string]interface{})
	require.True(t, ok, "child.image override should be a map")

	childImageRegistry, ok := childImageMapTyped["registry"].(string)
	assert.True(t, ok, "child.image map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", childImageRegistry, "Child image registry override incorrect")

	childImageRepo, ok := childImageMapTyped["repository"].(string)
	assert.True(t, ok, "child.image map should have repository string")
	// Path strategy preserves registry with dots and adds library/
	assert.Contains(t, childImageRepo, "docker.io/library/nginx", "Child image repo override incorrect")

	childImageTag, ok := childImageMapTyped["tag"].(string)
	assert.True(t, ok, "child.image map should have tag string")
	// NOTE: The analyzer/generator uses the value found ('1.23') from parent chart's override of child values
	assert.Equal(t, "1.23", childImageTag, "Child image tag override incorrect - generator should use parent override value")

	// Verify another-child image override structure
	anotherChildImageMap, ok := getNestedValue(overrideData, "another-child.image")
	assert.True(t, ok, "Should have override map for another-child.image")
	anotherChildImageMapTyped, ok := anotherChildImageMap.(map[string]interface{})
	require.True(t, ok, "another-child.image override should be a map")

	anotherChildImageRegistry, ok := anotherChildImageMapTyped["registry"].(string)
	assert.True(t, ok, "another-child.image map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", anotherChildImageRegistry, "Another-child image registry override incorrect")

	anotherChildImageRepo, ok := anotherChildImageMapTyped["repository"].(string)
	assert.True(t, ok, "another-child.image map should have repository string")
	assert.Contains(t, anotherChildImageRepo, "custom-repo/custom-image", "Another-child image repo override incorrect") // Check combined repo

	anotherChildImageTag, ok := anotherChildImageMapTyped["tag"].(string)
	assert.True(t, ok, "another-child.image map should have tag string")
	assert.Equal(t, "stable", anotherChildImageTag, "Another-child image tag override incorrect")

	// Verify deeply nested image override structure (prometheusImage)
	promImageMap, ok := getNestedValue(overrideData, "another-child.monitoring.prometheusImage")
	assert.True(t, ok, "Should have override map for another-child.monitoring.prometheusImage")
	promImageMapTyped, ok := promImageMap.(map[string]interface{})
	require.True(t, ok, "prometheusImage override should be a map")

	promImageRegistry, ok := promImageMapTyped["registry"].(string)
	assert.True(t, ok, "prometheusImage map should have registry string")
	assert.Equal(t, "test-registry.io/my-project", promImageRegistry, "Prometheus image registry override incorrect")

	promImageRepo, ok := promImageMapTyped["repository"].(string)
	assert.True(t, ok, "prometheusImage map should have repository string")
	// Path strategy preserves registry with dots
	assert.Contains(t, promImageRepo, "docker.io/prom/prometheus", "Prometheus image repo override incorrect")

	promImageTag, ok := promImageMapTyped["tag"].(string)
	assert.True(t, ok, "prometheusImage map should have tag string")
	assert.Equal(t, "v2.40.0", promImageTag, "Prometheus image tag override incorrect")
}

// TestOverrideAlias verifies that overrides use the subchart alias
// when --context-aware is enabled.
func TestOverrideAlias(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	chartPath := harness.GetTestdataPath("charts/minimal-alias-test")
	require.NotEqual(t, "", chartPath, "minimal-alias-test chart not found")

	harness.SetupChart(chartPath) // Copies chart to temp dir
	overrideFilePath := filepath.Join(harness.tempDir, "override-alias-test.yaml")

	// Define arguments for irr override
	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", "docker.io", // busybox defaults to docker.io
		"--context-aware",
		"--output-file", overrideFilePath,
	}

	// Execute the command
	// Use ExecuteIRRWithStderr which returns stdout and stderr separately
	// Pass nil for env, false for useContextAware (as we add it manually)
	stdout, stderr, err := harness.ExecuteIRRWithStderr(nil, false, args...)
	log.Debug("Command output", "stdout", stdout, "stderr", stderr) // Log output for debugging
	t.Logf("Captured Stderr:\n%s", stderr)                          // Explicitly log captured stderr
	require.NoError(t, err, "irr override command failed")

	// Read the generated override file using os.ReadFile
	overrideData, err := os.ReadFile(overrideFilePath)
	require.NoError(t, err, "Failed to read generated override file")
	log.Debug("Generated override file content", "content", string(overrideData))

	// Unmarshal the YAML data
	var overrides map[string]interface{}
	err = yaml.Unmarshal(overrideData, &overrides)
	require.NoError(t, err, "Failed to unmarshal override YAML")

	// Log the unmarshalled map for debugging
	log.Debug("Unmarshalled overrides map", "map", overrides)

	// *** Add extra debugging before assertion ***
	t.Logf("DEBUG: Type of overrides map: %T", overrides)
	t.Logf("DEBUG: Keys in overrides map:")
	mapKeys := []string{}
	for k := range overrides {
		mapKeyStr := fmt.Sprintf("%v", k) // Handle potential non-string keys
		mapKeys = append(mapKeys, mapKeyStr)
		t.Logf("  - Key: '%s' (Type: %T)", mapKeyStr, k)
	}
	log.Debug("Extracted map keys", "keys", mapKeys)
	// *** End extra debugging ***

	// Assertions
	// 1. Check if the top-level alias key exists
	aliasValue, aliasExists := overrides["theAlias"]
	assert.True(t, aliasExists, "'theAlias' key should exist at the top level")
	aliasMap, aliasIsMap := aliasValue.(map[string]interface{}) // Try string keys first
	if !aliasIsMap {
		// Fallback: Try interface keys if string assertion fails
		aliasInterfaceMap, aliasIsInterfaceMap := aliasValue.(map[interface{}]interface{})
		require.True(t, aliasIsInterfaceMap, "'theAlias' value should be a map (either string or interface keys)")
		// Convert interface map to string map for easier access (assuming keys are strings)
		aliasMap = make(map[string]interface{})
		for k, v := range aliasInterfaceMap {
			if keyStr, ok := k.(string); ok {
				aliasMap[keyStr] = v
			} else {
				t.Fatalf("Key '%v' under 'theAlias' is not a string", k)
			}
		}
		aliasIsMap = true // Mark as map now
	}
	require.True(t, aliasIsMap, "Value under 'theAlias' is not a map")

	// 2. Check the nested 'image' map within aliasMap
	imageValue, imageExists := aliasMap["image"]
	assert.True(t, imageExists, "'theAlias.image' key should exist")
	imageMap, imageIsMap := imageValue.(map[string]interface{}) // Try string keys first
	if !imageIsMap {
		imageInterfaceMap, imageIsInterfaceMap := imageValue.(map[interface{}]interface{})
		require.True(t, imageIsInterfaceMap, "'theAlias.image' value should be a map")
		imageMap = make(map[string]interface{})
		for k, v := range imageInterfaceMap {
			if keyStr, ok := k.(string); ok {
				imageMap[keyStr] = v
			} else {
				t.Fatalf("Key '%v' under 'theAlias.image' is not a string", k)
			}
		}
		imageIsMap = true
	}
	require.True(t, imageIsMap, "Value under 'theAlias.image' is not a map")

	// 3. Check registry
	registry, ok := imageMap["registry"].(string)
	assert.True(t, ok, "'theAlias.image.registry' should be a string")
	assert.Equal(t, "test.registry.io", registry, "'theAlias.image.registry' value incorrect")

	// 4. Check repository (using PrefixSourceRegistry strategy)
	repository, ok := imageMap["repository"].(string)
	assert.True(t, ok, "'theAlias.image.repository' should be a string")
	assert.Equal(t, "docker.io/library/busybox", repository, "'theAlias.image.repository' value incorrect") // Expect full path including library/

	// 5. Check tag
	tag, ok := imageMap["tag"].(string)
	assert.True(t, ok, "'theAlias.image.tag' should be a string")
	assert.Equal(t, "1.0", tag, "'theAlias.image.tag' value incorrect")

	// 6. (Optional but good) Check pullPolicy (should be preserved/defaulted)
	pullPolicy, ok := imageMap["pullPolicy"].(string)
	assert.True(t, ok, "'theAlias.image.pullPolicy' should exist")
	assert.Equal(t, "IfNotPresent", pullPolicy, "'theAlias.image.pullPolicy' value incorrect")
}

// TestOverrideDeepNesting verifies that overrides work correctly with deeply nested values
// when --context-aware is enabled.
func TestOverrideDeepNesting(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	chartPath := harness.GetTestdataPath("charts/deep-nesting-test")
	require.NotEqual(t, "", chartPath, "deep-nesting-test chart not found")

	harness.SetupChart(chartPath) // Copies chart to temp dir
	overrideFilePath := filepath.Join(harness.tempDir, "override-deep-nesting-test.yaml")

	// *** Run Analysis Separately ***
	analysisArgs := []string{
		"--context-aware",                                                    // Ensure context-aware is used for analysis
		"--source-registries", "docker.io,quay.io,ghcr.io,mcr.microsoft.com", // Match source registries
	}
	analysisResult, analysisErr := harness.ExecuteAnalysisOnly(harness.chartPath, analysisArgs...)
	require.NoError(t, analysisErr, "Analysis command failed")
	require.NotNil(t, analysisResult, "Analysis result is nil")

	// *** Log Detected Paths ***
	t.Log("--- Detected Image Paths from Analysis ---")
	foundProblemPath := false
	for _, pattern := range analysisResult.ImagePatterns {
		t.Logf("Path: %s, Value: %s, Type: %s", pattern.Path, pattern.Value, pattern.Type)
		if strings.HasSuffix(pattern.Path, ".repository") {
			t.Logf("*** POTENTIAL PROBLEM: Path ends in .repository: %s", pattern.Path)
			foundProblemPath = true
		}
	}
	t.Log("-----------------------------------------")
	require.False(t, foundProblemPath, "Found image patterns with paths ending in .repository")
	// *** End Logging ***

	// Define arguments for irr override
	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", "docker.io,quay.io,ghcr.io,mcr.microsoft.com",
		"--context-aware",
		"--output-file", overrideFilePath,
	}

	// Execute the command
	stdout, stderr, err := harness.ExecuteIRRWithStderr(nil, false, args...)
	log.Debug("Command output", "stdout", stdout, "stderr", stderr) // Log output for debugging
	require.NoError(t, err, "irr override command failed")

	// Read the generated override file
	overrideData, err := os.ReadFile(overrideFilePath)
	require.NoError(t, err, "Failed to read generated override file")
	log.Debug("Generated override file content", "content", string(overrideData))

	// *** Add logging ***
	t.Logf("DEBUG: Raw override YAML:\n%s", string(overrideData))
	// *** End logging ***

	// Unmarshal the YAML data
	var overrides map[string]interface{}
	err = yaml.Unmarshal(overrideData, &overrides)
	require.NoError(t, err, "Failed to unmarshal override YAML")

	// Helper function to get nested values
	getNestedImageMap := func(path string) map[string]interface{} {
		parts := strings.Split(path, ".")
		current := overrides

		for i, part := range parts {
			// Handle array indices
			if strings.Contains(part, "[") {
				idx := part[strings.Index(part, "[")+1 : strings.Index(part, "]")]
				idxNum, _ := strconv.Atoi(idx)
				part = part[:strings.Index(part, "[")]

				value, exists := current[part]
				if !exists {
					t.Fatalf("Array parent %s not found at index %d of path %s", part, i, path)
					return nil
				}

				arr, ok := value.([]interface{})
				if !ok {
					t.Fatalf("Expected array at %s, got %T", part, value)
					return nil
				}

				if idxNum >= len(arr) {
					t.Fatalf("Array index %d out of bounds for %s (len=%d)", idxNum, part, len(arr))
					return nil
				}

				// Check if we're at final element
				if i == len(parts)-1 {
					imgMap, ok := arr[idxNum].(map[string]interface{})
					if !ok {
						t.Fatalf("Expected map at %s[%d], got %T", part, idxNum, arr[idxNum])
						return nil
					}
					// *** Add logging ***
					t.Logf("DEBUG (getNestedImageMap): Returning map for array path %s[%d]: %#v", part, idxNum, imgMap)
					// *** End logging ***
					return imgMap
				}

				// If at intermediate element, grab the map and continue
				nextMap, ok := arr[idxNum].(map[string]interface{})
				if !ok {
					t.Fatalf("Expected map at %s[%d], got %T", part, idxNum, arr[idxNum])
					return nil
				}
				current = nextMap
				continue
			}

			// Handle regular path components
			if i == len(parts)-1 {
				// Last component should be the image map
				imgMap, ok := current[part].(map[string]interface{})
				if !ok {
					t.Fatalf("Expected map at %s, got %T", path, current[part])
					return nil
				}
				// *** Add logging ***
				t.Logf("DEBUG (getNestedImageMap): Returning map for path %s: %#v", path, imgMap)
				// *** End logging ***
				return imgMap
			}

			nextMap, ok := current[part].(map[string]interface{})
			if !ok {
				t.Fatalf("Path component %s not found or not a map in path %s", part, path)
				return nil
			}
			current = nextMap
		}

		t.Fatalf("Should not reach here - path: %s", path)
		return nil
	}

	// Verify deeply nested image
	deepImageMap := getNestedImageMap("level1.level2.level3.level4.level5.image")
	require.NotNil(t, deepImageMap, "Deep image map not found")
	assert.Equal(t, "test.registry.io", deepImageMap["registry"], "Deep image registry incorrect")
	assert.Contains(t, deepImageMap["repository"].(string), "docker.io/deepnest/extreme-depth", "Deep image repository incorrect")
	assert.Equal(t, "v1.2.3", deepImageMap["tag"], "Deep image tag incorrect")

	// Verify array nested images
	frontendMainImageMap := getNestedImageMap("services.frontend.containers[0].image")
	require.NotNil(t, frontendMainImageMap, "Frontend main image map not found")
	assert.Equal(t, "test.registry.io", frontendMainImageMap["registry"], "Frontend image registry incorrect")
	assert.Contains(t, frontendMainImageMap["repository"].(string), "quay.io/frontend/webapp", "Frontend image repository incorrect")
	assert.Equal(t, "stable", frontendMainImageMap["tag"], "Frontend image tag incorrect")

	// Verify subchart image with nested paths
	subchartImageMap := getNestedImageMap("minimal-child.nestedStruct.deeperImage")
	require.NotNil(t, subchartImageMap, "Subchart image map not found")
	assert.Equal(t, "test.registry.io", subchartImageMap["registry"], "Subchart image registry incorrect")

	// *** Add logging for debugging ***
	t.Logf("DEBUG: Subchart image map content: %#v", subchartImageMap)
	if repoValue, ok := subchartImageMap["repository"]; ok {
		t.Logf("DEBUG: Type of repository value: %T", repoValue)
	}
	// *** End logging ***

	assert.Contains(t, subchartImageMap["repository"].(string), "docker.io/subchart/deep-image", "Subchart image repository incorrect")
	assert.Equal(t, "alpha", subchartImageMap["tag"], "Subchart image tag incorrect")
}

// TestOverrideGlobals verifies that overrides correctly handle global variables
// when --context-aware is enabled.
func TestOverrideGlobals(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	chartPath := harness.GetTestdataPath("charts/global-test")
	require.NotEqual(t, "", chartPath, "global-test chart not found")

	harness.SetupChart(chartPath) // Copies chart to temp dir
	overrideFilePath := filepath.Join(harness.tempDir, "override-globals-test.yaml")

	// Define arguments for irr override
	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", "docker.io,quay.io,ghcr.io,mcr.microsoft.com",
		"--context-aware",
		"--output-file", overrideFilePath,
	}

	// Execute the command
	stdout, stderr, err := harness.ExecuteIRRWithStderr(nil, false, args...)
	log.Debug("Command output", "stdout", stdout, "stderr", stderr) // Log output for debugging
	require.NoError(t, err, "irr override command failed")

	// Read the generated override file
	overrideData, err := os.ReadFile(overrideFilePath)
	require.NoError(t, err, "Failed to read generated override file")
	log.Debug("Generated override file content", "content", string(overrideData))

	// Unmarshal the YAML data
	var overrides map[string]interface{}
	err = yaml.Unmarshal(overrideData, &overrides)
	require.NoError(t, err, "Failed to unmarshal override YAML")

	// Helper function to get image map from override structure
	getImageMap := func(path string) map[string]interface{} {
		parts := strings.Split(path, ".")
		current := overrides

		for i, part := range parts {
			if i == len(parts)-1 {
				// Last component should be the image map
				imgMap, ok := current[part].(map[string]interface{})
				if !ok {
					t.Fatalf("Expected map at %s, got %T", path, current[part])
					return nil
				}
				return imgMap
			}

			nextMap, ok := current[part].(map[string]interface{})
			if !ok {
				t.Fatalf("Path component %s not found or not a map in path %s", part, path)
				return nil
			}
			current = nextMap
		}

		return nil
	}

	// 1. Test global.image override - should be overridden with source registry=quay.io
	globalImageMap := getImageMap("global.image")
	require.NotNil(t, globalImageMap, "global.image override not found")
	assert.Equal(t, "test.registry.io", globalImageMap["registry"], "global.image registry should be test.registry.io")
	assert.Contains(t, globalImageMap["repository"].(string), "quay.io/organization/shared-app", "global.image repository incorrect")
	assert.Equal(t, "1.0.0", globalImageMap["tag"], "global.image tag incorrect")

	// 2. Test global imageRegistry override
	globalSection, ok := overrides["global"].(map[string]interface{})
	require.True(t, ok, "global section not found or not a map")
	assert.Equal(t, "test.registry.io", globalSection["imageRegistry"], "global.imageRegistry should be test.registry.io")

	// 3. Test parentImage - uses global.imageRegistry
	parentImageMap := getImageMap("parentImage")
	require.NotNil(t, parentImageMap, "parentImage override not found")
	assert.Equal(t, "test.registry.io", parentImageMap["registry"], "parentImage registry should be test.registry.io")
	assert.Contains(t, parentImageMap["repository"].(string), "docker.io/parent/app", "parentImage repository incorrect")
	assert.Equal(t, "v1.0.0", parentImageMap["tag"], "parentImage tag incorrect")

	// 4. Test explicitImage - has its own registry (ghcr.io)
	explicitImageMap := getImageMap("explicitImage")
	require.NotNil(t, explicitImageMap, "explicitImage override not found")
	assert.Equal(t, "test.registry.io", explicitImageMap["registry"], "explicitImage registry should be test.registry.io")
	assert.Contains(t, explicitImageMap["repository"].(string), "ghcr.io/explicit/component", "explicitImage repository incorrect")
	assert.Equal(t, "latest", explicitImageMap["tag"], "explicitImage tag incorrect")

	// 5. Test subchart image with global registry
	subchartImageMap := getImageMap("minimal-child.image")
	require.NotNil(t, subchartImageMap, "minimal-child.image override not found")
	assert.Equal(t, "test.registry.io", subchartImageMap["registry"], "minimal-child.image registry should be test.registry.io")
	assert.Contains(t, subchartImageMap["repository"].(string), "docker.io/custom/subchart-image", "minimal-child.image repository incorrect")
	assert.Equal(t, "v2.3.4", subchartImageMap["tag"], "minimal-child.image tag incorrect")

	// 6. Test subchart standalone image with explicit registry
	standaloneImageMap := getImageMap("minimal-child.standaloneImage")
	require.NotNil(t, standaloneImageMap, "minimal-child.standaloneImage override not found")
	assert.Equal(t, "test.registry.io", standaloneImageMap["registry"], "minimal-child.standaloneImage registry should be test.registry.io")
	assert.Contains(t, standaloneImageMap["repository"].(string), "mcr.microsoft.com/standalone/component", "minimal-child.standaloneImage repository incorrect")
	assert.Equal(t, "20.04", standaloneImageMap["tag"], "minimal-child.standaloneImage tag incorrect")
}

// TestOverridePluginMode_SingleReleaseNamespace verifies that 'irr override' works in Helm plugin mode for a single release/namespace.
func TestOverridePluginMode_SingleReleaseNamespace(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// 1. Install a test chart as a release in a test namespace
	chartPath := h.GetTestdataPath("charts/fallback-test")
	releaseName := "test-release"
	namespace := "test-ns"
	_, err := h.ExecuteHelm("install", releaseName, chartPath, "--namespace", namespace, "--create-namespace")
	require.NoError(t, err, "Helm install should succeed")
	// Clean up the helm release after the test
	defer func() {
		if err := h.UninstallHelmRelease(releaseName, namespace); err != nil {
			t.Logf("Warning: Failed to uninstall release %s in namespace %s: %v", releaseName, namespace, err)
		}
	}()

	// 2. Simulate plugin mode: set HELM_PLUGIN_NAME/HELM_PLUGIN_DIR
	env := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_PLUGIN_DIR":  "/fake/plugins/irr",
		"HELM_NAMESPACE":   namespace, // Optional, for completeness
	}

	// 3. Run irr override in plugin mode
	stdout, stderr, err := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--log-level", "debug",
	)
	require.NoError(t, err, "override (plugin mode) should succeed. Stderr: %s", stderr)

	// 4. Assert output contains expected registry/repo/tag
	assert.Contains(t, stdout, "registry: my-target-registry.com", "Output should include the target registry")
	assert.Contains(t, stdout, "repository: docker.io/library/nginx", "Output should include the image repository")
	assert.Contains(t, stdout, "tag: latest", "Output should include the image tag")
}

// TestOverridePluginMode_NonexistentReleaseOrNamespace verifies that 'irr override' works in Helm plugin mode for a nonexistent release and/or namespace.
func TestOverridePluginMode_NonexistentReleaseOrNamespace(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	releaseName := "nonexistent-release"
	namespace := "nonexistent-ns"
	env := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_PLUGIN_DIR":  "/fake/plugins/irr",
		"HELM_NAMESPACE":   namespace,
	}

	stdout, stderr, err := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--log-level", "debug",
	)
	assert.Error(t, err, "Should error for nonexistent release/namespace")
	assert.Contains(t, stderr+stdout, "not found", "Error message should mention not found")
}

// TestOverridePluginMode_ReleaseWithNoImages verifies that 'irr override' works in Helm plugin mode for a release with no images.
func TestOverridePluginMode_ReleaseWithNoImages(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Use a minimal chart with no images (assume test-data/charts/no-values-test exists)
	chartPath := h.GetTestdataPath("charts/no-values-test")
	releaseName := "no-images-release"
	namespace := "no-images-ns"
	_, err := h.ExecuteHelm("install", releaseName, chartPath, "--namespace", namespace, "--create-namespace")
	require.NoError(t, err, "Helm install should succeed")
	// Clean up the helm release after the test
	defer func() {
		if err := h.UninstallHelmRelease(releaseName, namespace); err != nil {
			t.Logf("Warning: Failed to uninstall release %s in namespace %s: %v", releaseName, namespace, err)
		}
	}()

	env := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_PLUGIN_DIR":  "/fake/plugins/irr",
		"HELM_NAMESPACE":   namespace,
	}

	stdout, stderr, err := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--log-level", "debug",
	)
	assert.NoError(t, err, "Should not error for release with no images. Stderr: %s", stderr)

	// Check if stdout is effectively empty (common empty YAML representations) or if stderr contains a relevant warning.
	// An empty override often results in "{}\n" or "null\n".
	trimmedStdout := strings.TrimSpace(stdout)
	isEmptyYamlOutput := trimmedStdout == "{}" || trimmedStdout == "null" || trimmedStdout == ""
	isNoImagesWarning := strings.Contains(stderr, "no images found that require override") || // A more general warning
		strings.Contains(stderr, "Analysis result is nil") || // Specific warning from current code
		strings.Contains(stderr, "chart has no values/images") || // Part of the specific warning
		strings.Contains(stderr, "No image patterns found") || // Another possible warning
		strings.Contains(stderr, "No images found in chart that match criteria") // Yet another

	assert.True(t, isEmptyYamlOutput || isNoImagesWarning,
		fmt.Sprintf("Output should be empty YAML ('{}', 'null', or '') or stderr should contain a 'no images' warning.\nStdout: %s\nStderr: %s", stdout, stderr))
}

// TestOverridePluginMode_ExcludedRegistries verifies that 'irr override' works in Helm plugin mode for a release with excluded registries.
func TestOverridePluginMode_ExcludedRegistries(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Use a chart with images from multiple registries (assume test-data/charts/global-test exists)
	chartPath := h.GetTestdataPath("charts/global-test")
	releaseName := "excluded-registries-release"
	namespace := "excluded-registries-ns"
	_, err := h.ExecuteHelm("install", releaseName, chartPath, "--namespace", namespace, "--create-namespace")
	require.NoError(t, err, "Helm install should succeed")
	// Clean up the helm release after the test
	defer func() {
		if err := h.UninstallHelmRelease(releaseName, namespace); err != nil {
			t.Logf("Warning: Failed to uninstall release %s in namespace %s: %v", releaseName, namespace, err)
		}
	}()

	env := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_PLUGIN_DIR":  "/fake/plugins/irr",
		"HELM_NAMESPACE":   namespace,
	}

	stdout, stderr, err := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io,quay.io,ghcr.io,mcr.microsoft.com",
		"--exclude-registries", "quay.io,ghcr.io",
		"--log-level", "debug",
	)
	require.NoError(t, err, "override (plugin mode) should succeed. Stderr: %s", stderr)

	// Assert that images from excluded registries are not present in the output
	assert.NotContains(t, stdout, "quay.io", "Output should not contain excluded registry quay.io")
	assert.NotContains(t, stdout, "ghcr.io", "Output should not contain excluded registry ghcr.io")
	// Assert that images from included registries are present
	assert.Contains(t, stdout, "docker.io", "Output should contain included registry docker.io")
	assert.Contains(t, stdout, "mcr.microsoft.com", "Output should contain included registry mcr.microsoft.com")
}

// TestOverridePluginMode_StrictModeUnsupportedStructure verifies that 'irr override' works in Helm plugin mode for a release with an unsupported image structure.
func TestOverridePluginMode_StrictModeUnsupportedStructure(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Use a chart with an unsupported image structure (assume test-data/charts/unsupported-test exists)
	chartPath := h.GetTestdataPath("charts/unsupported-test")
	releaseName := "strict-unsupported-release"
	namespace := "strict-unsupported-ns"
	_, err := h.ExecuteHelm("install", releaseName, chartPath, "--namespace", namespace, "--create-namespace")
	require.NoError(t, err, "Helm install should succeed")
	// Clean up the helm release after the test
	defer func() {
		if err := h.UninstallHelmRelease(releaseName, namespace); err != nil {
			t.Logf("Warning: Failed to uninstall release %s in namespace %s: %v", releaseName, namespace, err)
		}
	}()

	env := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_PLUGIN_DIR":  "/fake/plugins/irr",
		"HELM_NAMESPACE":   namespace,
	}

	stdout, stderr, err := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--strict",
		"--log-level", "debug",
	)
	assert.Error(t, err, "Should error in strict mode for unsupported structure")
	assert.Contains(t, stderr+stdout, "unsupported", "Error message should mention unsupported structure")
}

// TestOverridePluginMode_RegistryMappingFile verifies that 'irr override' works in Helm plugin mode for a release with a registry mapping file.
func TestOverridePluginMode_RegistryMappingFile(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Use a chart with a known source registry (assume test-data/charts/fallback-test exists)
	chartPath := h.GetTestdataPath("charts/fallback-test")
	releaseName := "mappingfile-release"
	namespace := "mappingfile-ns"
	_, err := h.ExecuteHelm("install", releaseName, chartPath, "--namespace", namespace, "--create-namespace")
	require.NoError(t, err, "Helm install should succeed")
	// Clean up the helm release after the test
	defer func() {
		if err := h.UninstallHelmRelease(releaseName, namespace); err != nil {
			t.Logf("Warning: Failed to uninstall release %s in namespace %s: %v", releaseName, namespace, err)
		}
	}()

	// Create a custom registry mapping file
	mappingFile := h.GetTempFilePath("custom-mapping.yaml")
	mappingContent := `version: "1.0"
registries:
  mappings:
    - source: "docker.io"
      target: "custom.registry.io/mirror"
      enabled: true
`
	require.NoError(t, os.WriteFile(mappingFile, []byte(mappingContent), 0600), "Failed to write mapping file")

	env := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_PLUGIN_DIR":  "/fake/plugins/irr",
		"HELM_NAMESPACE":   namespace,
	}

	stdout, stderr, err := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--registry-file", mappingFile,
		"--target-registry", "should-not-be-used.io",
		"--source-registries", "docker.io",
		"--log-level", "debug",
	)
	require.NoError(t, err, "override (plugin mode) with mapping file should succeed. Stderr: %s", stderr)
	assert.Contains(t, stdout, "registry: custom.registry.io", "Output should use registry from mapping file")
	assert.Contains(t, stdout, "repository: mirror/library/nginx", "Output should use mirror path from mapping file")
	assert.NotContains(t, stdout, "should-not-be-used.io", "Output should not use CLI target registry when mapping file is present")
}

// TestOverridePluginMode_OutputFileAndDryRun verifies that 'irr override' works in Helm plugin mode for a release with output file and dry run behavior.
func TestOverridePluginMode_OutputFileAndDryRun(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/fallback-test")
	releaseName := "outputfile-release"
	namespace := "outputfile-ns"
	_, err := h.ExecuteHelm("install", releaseName, chartPath, "--namespace", namespace, "--create-namespace")
	require.NoError(t, err, "Helm install should succeed")
	// Clean up the helm release after the test
	defer func() {
		if err := h.UninstallHelmRelease(releaseName, namespace); err != nil {
			t.Logf("Warning: Failed to uninstall release %s in namespace %s: %v", releaseName, namespace, err)
		}
	}()

	outputFile := h.GetTempFilePath("plugin-output.yaml")
	env := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_PLUGIN_DIR":  "/fake/plugins/irr",
		"HELM_NAMESPACE":   namespace,
	}

	// 1. Run with --output-file (should write file)
	_, stderr, err := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
		"--log-level", "debug",
	)
	require.NoError(t, err, "override (plugin mode) with output file should succeed. Stderr: %s", stderr)
	content, readErr := os.ReadFile(outputFile)
	require.NoError(t, readErr, "Should be able to read output file")
	assert.Contains(t, string(content), "my-target-registry.com", "Output file should contain target registry")

	// 2. Run again with same --output-file (should error)
	_, stderr2, err2 := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--output-file", outputFile,
		"--log-level", "debug",
	)
	assert.Error(t, err2, "Should error if output file already exists")
	assert.Contains(t, stderr2, "already exists", "Error should mention file already exists")

	// 3. Run with --dry-run (should print to stdout, not write file)
	dryRunFile := h.GetTempFilePath("plugin-dryrun.yaml")
	stdout3, stderr3, err3 := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--output-file", dryRunFile,
		"--dry-run",
		"--log-level", "debug",
	)
	assert.NoError(t, err3, "Dry run should not error. Stderr: %s", stderr3)
	assert.Contains(t, stdout3, "my-target-registry.com", "Dry run output should contain target registry")
	_, errStat := os.Stat(dryRunFile)
	assert.True(t, os.IsNotExist(errStat), "Dry run should not write output file")
}

// TestOverridePluginMode_ComplexValueStructures verifies that 'irr override' works in Helm plugin mode for a release with complex value structures.
func TestOverridePluginMode_ComplexValueStructures(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/deep-nesting-test")
	releaseName := "complex-struct-release"
	namespace := "complex-struct-ns"
	_, err := h.ExecuteHelm("install", releaseName, chartPath, "--namespace", namespace, "--create-namespace")
	require.NoError(t, err, "Helm install should succeed")
	// Clean up the helm release after the test
	defer func() {
		if err := h.UninstallHelmRelease(releaseName, namespace); err != nil {
			t.Logf("Warning: Failed to uninstall release %s in namespace %s: %v", releaseName, namespace, err)
		}
	}()

	env := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_PLUGIN_DIR":  "/fake/plugins/irr",
		"HELM_NAMESPACE":   namespace,
	}

	stdout, stderr, err := h.ExecuteIRRWithStderr(env, true, // useContextAware=true
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io,quay.io",
		"--log-level", "debug",
	)
	require.NoError(t, err, "override (plugin mode) with complex value structures should succeed. Stderr: %s", stderr)
	// Assert that deeply nested and array images are present in the output
	assert.Contains(t, stdout, "docker.io/deepnest/extreme-depth", "Output should contain deeply nested image")
	assert.Contains(t, stdout, "quay.io/frontend/webapp", "Output should contain array image")
	assert.Contains(t, stdout, "docker.io/subchart/deep-image", "Output should contain subchart nested image")
}

// TestOverridePluginMode_OutputFormatJSON verifies that 'irr override' works in Helm plugin mode for a release with --output-format json.
func TestOverridePluginMode_OutputFormatJSON(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	chartPath := h.GetTestdataPath("charts/fallback-test")
	releaseName := "jsonfmt-release"
	namespace := "jsonfmt-ns"
	_, err := h.ExecuteHelm("install", releaseName, chartPath, "--namespace", namespace, "--create-namespace")
	require.NoError(t, err, "Helm install should succeed")
	// Clean up the helm release after the test
	defer func() {
		if err := h.UninstallHelmRelease(releaseName, namespace); err != nil {
			t.Logf("Warning: Failed to uninstall release %s in namespace %s: %v", releaseName, namespace, err)
		}
	}()

	env := map[string]string{
		"HELM_PLUGIN_NAME": "irr",
		"HELM_PLUGIN_DIR":  "/fake/plugins/irr",
		"HELM_NAMESPACE":   namespace,
	}

	stdout, stderr, err := h.ExecuteIRRWithStderr(env, false,
		"override",
		releaseName,
		"-n", namespace,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--output-format", "json",
		"--log-level", "debug",
	)
	require.NoError(t, err, "override (plugin mode) with --output-format json should succeed. Stderr: %s", stderr)
	assert.Contains(t, stdout, "my-target-registry.com", "JSON output should contain target registry")
	assert.Contains(t, stdout, "docker.io/library/nginx", "JSON output should contain image repository")
	assert.Contains(t, stdout, "latest", "JSON output should contain image tag")
	// Check that output is valid JSON
	var js map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &js)
	assert.NoError(t, err, "Output should be valid JSON")
}

// Add other test functions below if needed...
