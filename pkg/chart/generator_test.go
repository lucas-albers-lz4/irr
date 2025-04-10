package chart

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	helmchart "helm.sh/helm/v3/pkg/chart"

	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
)

// MockPathStrategy implements the strategy.PathStrategy interface for testing
type MockPathStrategy struct{}

func (m *MockPathStrategy) GeneratePath(ref *image.Reference, _ string) (string, error) {
	if ref == nil {
		return "", errors.New("mock strategy received nil reference")
	}
	// Return mockpath/{repository} format as expected by tests
	return fmt.Sprintf("mockpath/%s", ref.Repository), nil
}

// MockChartLoader implements the analysis.ChartLoader interface for testing
type MockChartLoader struct {
	chart *helmchart.Chart
	err   error
}

func (m *MockChartLoader) Load(_ string) (*helmchart.Chart, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.chart, nil
}

func TestNewGenerator(t *testing.T) {
	strategy := &MockPathStrategy{}
	loader := &MockChartLoader{} // Use mock loader
	// Use chart.NewGenerator from the actual package
	gen := NewGenerator("path", "target", []string{"source"}, []string{}, strategy, nil, false, 80, loader, []string(nil), []string(nil), []string(nil))
	assert.NotNil(t, gen)
}

func TestGenerator_Generate_Simple(t *testing.T) {
	// Use the implemented mocks
	mockLoader := &MockChartLoader{
		chart: &helmchart.Chart{
			Metadata: &helmchart.Metadata{Name: "test-chart"},
			Values: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "source.registry.com",
					"repository": "library/nginx",
					"tag":        "latest",
				},
			},
		},
	}
	mockStrategy := &MockPathStrategy{}

	g := NewGenerator(
		"test-chart",
		"target.registry.com",
		[]string{"source.registry.com"},
		[]string{},
		mockStrategy,
		nil,
		false,
		0,
		mockLoader,
		nil, nil, nil,
	)

	result, err := g.Generate()
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the expected overrides map structure
	expectedOverrides := override.File{
		ChartPath: "test-chart",
		Overrides: map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "target.registry.com",
				"repository": "mockpath/library/nginx",
				"tag":        "latest",
			},
		},
		Unsupported: []override.UnsupportedStructure{},
	}

	assert.Equal(t, expectedOverrides.ChartPath, result.ChartPath)
	assert.Equal(t, expectedOverrides.Overrides, result.Overrides)
	assert.Equal(t, expectedOverrides.Unsupported, result.Unsupported)
}

func TestGenerator_Generate_ThresholdMet(t *testing.T) {
	// Setup mocks similar to TestGenerator_Generate_Simple, but with data
	// that results in multiple images to test threshold logic.
	mockLoader := &MockChartLoader{
		chart: &helmchart.Chart{
			Metadata: &helmchart.Metadata{Name: "test-chart"},
			Values: map[string]interface{}{
				"image": map[string]interface{}{ // Image 1 (Map)
					"registry":   "source.registry.com",
					"repository": "library/nginx",
					"tag":        "latest",
				},
				"sidecar": map[string]interface{}{
					"image": map[string]interface{}{ // Image 2 (Map) - Nested
						"registry":   "another.source.com",
						"repository": "utils/busybox",
						"tag":        "1.2.3",
					},
				},
				"ignoredImage": "ignored.registry.com/ignored/image:tag", // Will be filtered
			},
		},
	}
	mockStrategy := &MockPathStrategy{} // Will prepend "mockpath/"

	g := NewGenerator(
		"test-chart",
		"target.registry.com",
		[]string{"source.registry.com", "another.source.com"}, // Allow both sources
		[]string{"ignored.registry.com"},                      // Exclude this one
		mockStrategy,
		nil, false,
		80, // Threshold 80% - Should pass (2/2 eligible images processed)
		mockLoader,
		nil, nil, nil,
	)

	result, err := g.Generate()
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the expected overrides for both images
	expectedOverrides := override.File{
		Overrides: map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "target.registry.com",
				"repository": "mockpath/library/nginx", // mockpath/ + library/nginx
				"tag":        "latest",
			},
			"sidecar": map[string]interface{}{ // Nested structure preserved
				"image": map[string]interface{}{
					"registry":   "target.registry.com",    // Default target registry
					"repository": "mockpath/utils/busybox", // mockpath/ + utils/busybox
					"tag":        "1.2.3",
				},
			},
		},
		Unsupported: []override.UnsupportedStructure{},
	}
	assert.Equal(t, expectedOverrides.Overrides, result.Overrides)
	assert.Empty(t, result.Unsupported) // Ensure no unsupported structures reported
}

func TestGenerator_Generate_ThresholdNotMet(t *testing.T) {
	// Similar setup to ThresholdMet, but force a path generation error for one image
	mockLoader := &MockChartLoader{
		chart: &helmchart.Chart{
			Metadata: &helmchart.Metadata{Name: "test-chart"},
			Values: map[string]interface{}{
				"image1": "source.registry.com/library/nginx:latest", // String type
				"image2": "source.registry.com/library/redis:stable", // String type - will cause error
			},
		},
	}
	// Mock strategy that errors for redis
	mockStrategy := &MockPathStrategyWithError{ErrorImageRepo: "library/redis"}

	g := NewGenerator(
		"test-chart", "target.registry.com",
		[]string{"source.registry.com"}, []string{},
		mockStrategy, // Use the erroring strategy
		nil, false,
		100, // Threshold 100% - Should fail (1/2 processed)
		mockLoader,
		nil, nil, nil,
	)

	result, err := g.Generate()
	// Expect a ThresholdError because only 1 out of 2 eligible images could be processed
	require.Error(t, err)
	assert.Nil(t, result) // No result file on threshold error
	// Use errors.As to check for the specific *ThresholdError type
	var thresholdErr *ThresholdError
	require.ErrorAs(t, err, &thresholdErr, "Error should be of type ThresholdError")
	// Adjust the expected error message to match the actual format from ThresholdError.Error()
	expectedErrMsgPart1 := "processing failed: success rate 50% below threshold 100% (1/2 eligible images processed)"
	expectedErrMsgPart2 := "path generation failed for 'source.registry.com/library/redis:stable': assert.AnError general error for testing"
	assert.Contains(t, err.Error(), expectedErrMsgPart1, "Error message should contain the rate and threshold info")
	assert.Contains(t, err.Error(), expectedErrMsgPart2, "Error message should contain the specific path generation failure")

	// Optional: Check underlying errors if ThresholdError wraps them
	// require.Len(t, thresholdErr.WrappedErrs, 1)
}

// Helper Mock Strategy for error testing
type MockPathStrategyWithError struct {
	ErrorImageRepo string // If ref.Repository matches this, return error
}

func (m *MockPathStrategyWithError) GeneratePath(ref *image.Reference, _ string) (string, error) {
	if ref.Repository == m.ErrorImageRepo {
		return "", assert.AnError // Return a generic error
	}
	// Otherwise, behave like the normal mock strategy
	return "mockpath/" + ref.Repository + ":" + ref.Tag, nil
}

// Test case for when strict mode finds unsupported patterns (like templates)
func TestGenerator_Generate_StrictModeViolation(t *testing.T) {
	chartPath := "/test/strict"
	// Mock loader returns a chart with a templated image value
	mockLoader := &MockChartLoader{
		chart: &helmchart.Chart{
			Metadata: &helmchart.Metadata{Name: "test-strict"},
			Values: map[string]interface{}{
				"templatedImage": "{{ .Values.repo }}/myimage:{{ .Values.tag }}", // Unsupported
				"normalImage":    "docker.io/library/nginx:stable",               // Supported
			},
		},
	}
	mockStrategy := &MockPathStrategy{}

	// Enable strict mode
	g := NewGenerator(
		chartPath,
		"target.registry.com",
		[]string{"docker.io"}, // Source registry
		[]string{},            // No exclusions
		mockStrategy,
		nil,  // No mappings
		true, // Strict mode ON
		0,    // Threshold (irrelevant when strict fails)
		mockLoader,
		nil, nil, nil,
	)

	result, err := g.Generate()
	// Use require.ErrorIs for specific error type checking
	require.ErrorIs(t, err, ErrUnsupportedStructure)
	assert.Nil(t, result) // Ensure result is nil on error
}

func TestGenerator_Generate_Mappings(t *testing.T) {
	chartPath := "/test/chart-map" // Path doesn't matter much with mock loader

	// Create mock mappings directly using the struct literal
	mappings := &registry.Mappings{
		Entries: []registry.Mapping{
			{Source: "source.registry.com", Target: "mapped-target.example.com"},
			{Source: "another.source.com", Target: "another-mapped.example.com"},
		},
	}

	// Mock loader returns chart with images from different source registries
	mockLoader := &MockChartLoader{
		chart: &helmchart.Chart{
			Metadata: &helmchart.Metadata{Name: "test-chart-map"},
			Values: map[string]interface{}{
				"imageOne":   "source.registry.com/library/nginx:stable",   // Will use first mapping
				"imageTwo":   "another.source.com/utils/prometheus:latest", // Will use second mapping
				"imageThree": "unmapped.source.com/app/backend:v1",         // Will use default target registry
			},
		},
	}
	mockStrategy := &MockPathStrategy{} // Simple "mockpath/" strategy

	gen := NewGenerator(
		chartPath,
		"default-target.registry.com", // Default target registry
		[]string{"source.registry.com", "another.source.com", "unmapped.source.com"}, // Source registries
		[]string{}, // No exclusions
		mockStrategy,
		mappings, // Provide the mappings
		false,    // Strict mode off
		0,        // Threshold
		mockLoader,
		nil, nil, nil,
	)

	overrideFile, err := gen.Generate()
	require.NoError(t, err)
	require.NotNil(t, overrideFile)
	// Expect overrides for all three images
	require.Len(t, overrideFile.Overrides, 3)

	// Check imageOne override (Uses mapping: source -> mapped-target)
	// Since original was string, expect string override
	imgOneOverride, ok := overrideFile.Overrides["imageOne"].(string)
	require.True(t, ok, "Override for imageOne should be a string")
	// Expected: mapped-target.example.com + / + mockpath/library/nginx:stable
	assert.Equal(t, "mapped-target.example.com/mockpath/library/nginx:stable", imgOneOverride)

	// Check imageTwo override (Uses mapping: another -> another-mapped)
	imgTwoOverride, ok := overrideFile.Overrides["imageTwo"].(string)
	require.True(t, ok, "Override for imageTwo should be a string")
	// Expected: another-mapped.example.com + / + mockpath/utils/prometheus:latest
	assert.Equal(t, "another-mapped.example.com/mockpath/utils/prometheus:latest", imgTwoOverride)

	// Check imageThree override (No mapping, uses default target registry)
	imgThreeOverride, ok := overrideFile.Overrides["imageThree"].(string)
	require.True(t, ok, "Override for imageThree should be a string")
	// Expected: default-target.registry.com + / + mockpath/app/backend:v1
	assert.Equal(t, "default-target.registry.com/mockpath/app/backend:v1", imgThreeOverride)
}

// Remove tests for deleted functions
func TestProcessChartForOverrides_Removed(t *testing.T) {
	t.Skip("Test for removed function processChartForOverrides")
}

func TestMergeOverrides(t *testing.T) {
	t.Skip("Test for removed function mergeOverrides")
}

func TestExtractSubtree(t *testing.T) {
	t.Skip("Functionality removed or under refactoring")
}

// MockImageDetector for testing
type MockImageDetector struct {
	DetectedImages []image.DetectedImage
	Unsupported    []image.UnsupportedImage
	Error          error
}

func (m *MockImageDetector) DetectImages(_ interface{}, _ []string) ([]image.DetectedImage, []image.UnsupportedImage, error) {
	return m.DetectedImages, m.Unsupported, m.Error
}

func TestProcessChartForOverrides(t *testing.T) {
	t.Skip("Functionality removed or under refactoring")
}

func TestGenerateOverrides_Integration(t *testing.T) {
	// Setup mocks for a more integrated test
	mockLoader := &MockChartLoader{
		chart: &helmchart.Chart{
			Metadata: &helmchart.Metadata{Name: "test-integration"},
			Values: map[string]interface{}{
				"image": "source.test/nginx:tag", // Use .test TLD
				"sidecar": map[string]interface{}{ // Nested string image
					"image": "source.test/helper:tag", // Use .test TLD
				},
				"unused": "ignored.com/image:latest", // Should be ignored
			},
		},
	}
	// Use the actual PrefixSourceRegistryStrategy
	pathStrategy, err := strategy.GetStrategy("prefix-source-registry", nil)
	require.NoError(t, err)

	g := NewGenerator(
		"test-integration",
		"target.registry.com",
		[]string{"source.test"}, // Update source registry
		[]string{"ignored.com"},
		pathStrategy,
		nil, false, 0, // No mappings, non-strict, no threshold
		mockLoader,
		nil, nil, nil,
	)

	result, err := g.Generate()
	require.NoError(t, err)
	require.NotNil(t, result)

	// Assertions
	assert.Len(t, result.Overrides, 2, "Should have 2 overrides (image, sidecar.image)")

	// Check top-level image override (string type)
	imgOverride, ok := result.Overrides["image"].(string)
	assert.True(t, ok, "'image' override should be a string")
	if ok {
		// Expected: target.registry.com/sourcetest/nginx:tag
		expectedImage := "target.registry.com/sourcetest/nginx:tag"
		assert.Equal(t, expectedImage, imgOverride)
	}

	// Check nested sidecar override (map type, as string was converted)
	sidecarOverride, ok := result.Overrides["sidecar"].(map[string]interface{})
	require.True(t, ok, "'sidecar' override should be a map")

	sidecarImageOverride, ok := sidecarOverride["image"].(string)
	assert.True(t, ok, "'sidecar.image' override should resolve to a string")
	if ok {
		// Expected: target.registry.com/sourcetest/helper:tag
		expectedSidecarImage := "target.registry.com/sourcetest/helper:tag"
		assert.Equal(t, expectedSidecarImage, sidecarImageOverride)
	}

	// Ensure the unused image wasn't included
	_, unusedExists := result.Overrides["unused"]
	assert.False(t, unusedExists, "'unused' key should not be present in overrides")
}

// Helper function to create a temporary Helm chart directory
// ... rest of the file ...
