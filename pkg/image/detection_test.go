package image

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// comparePaths compares two paths represented as string slices.
func comparePaths(p1, p2 []string) int {
	len1, len2 := len(p1), len(p2)
	minLen := len1
	if len2 < minLen {
		minLen = len2
	}
	for i := 0; i < minLen; i++ {
		if p1[i] != p2[i] {
			if p1[i] < p2[i] {
				return -1
			}
			return 1
		}
	}
	if len1 < len2 {
		return -1
	}
	if len1 > len2 {
		return 1
	}
	return 0
}

// SortDetectedImages sorts a slice of DetectedImage primarily by path, then by pattern.
func SortDetectedImages(images []DetectedImage) {
	sort.SliceStable(images, func(i, j int) bool {
		pathComparison := comparePaths(images[i].Path, images[j].Path)
		if pathComparison != 0 {
			return pathComparison < 0
		}
		// If paths are equal, sort by Pattern
		return images[i].Pattern < images[j].Pattern
	})
}

// SortUnsupportedImages sorts a slice of UnsupportedImage by location path.
func SortUnsupportedImages(images []UnsupportedImage) {
	sort.SliceStable(images, func(i, j int) bool {
		return comparePaths(images[i].Location, images[j].Location) < 0
	})
}

// assertDetectedImages compares two slices of DetectedImage field by field.
// It assumes the slices have been sorted beforehand.
func assertDetectedImages(t *testing.T, expected, actual []DetectedImage, checkOriginal bool) {
	t.Helper()

	// Sort both slices by path string for deterministic comparison
	SortDetectedImages(expected)
	SortDetectedImages(actual)

	assert.Equal(t, len(expected), len(actual), "Detected image count mismatch")
	if len(expected) == len(actual) {
		for i := range actual {
			assert.Equal(t, expected[i].Reference.Registry, actual[i].Reference.Registry, fmt.Sprintf("detected[%d] Registry mismatch", i))
			assert.Equal(t, expected[i].Reference.Repository, actual[i].Reference.Repository, fmt.Sprintf("detected[%d] Repository mismatch", i))
			assert.Equal(t, expected[i].Reference.Tag, actual[i].Reference.Tag, fmt.Sprintf("detected[%d] Tag mismatch", i))
			assert.Equal(t, expected[i].Reference.Digest, actual[i].Reference.Digest, fmt.Sprintf("detected[%d] Digest mismatch", i))
			assert.Equal(t, expected[i].Path, actual[i].Path, fmt.Sprintf("detected[%d] path mismatch", i))
			assert.Equal(t, expected[i].Pattern, actual[i].Pattern, fmt.Sprintf("detected[%d] pattern mismatch", i))
			if checkOriginal {
				assert.Equal(t, expected[i].Original, actual[i].Original, fmt.Sprintf("detected[%d] original mismatch", i))
			}
		}
	} else {
		assert.Equal(t, expected, actual, "Detected images mismatch (fallback diff)") // Fallback for detailed diff on length mismatch
	}
}

// assertUnsupportedImages compares two slices of UnsupportedImage.
// It assumes the slices have been sorted beforehand.
func assertUnsupportedImages(t *testing.T, expected, actual []UnsupportedImage) {
	t.Helper()
	assert.Equal(t, len(expected), len(actual), "Unsupported image count mismatch")
	if len(expected) == len(actual) {
		for i := range actual {
			assert.Equal(t, expected[i].Location, actual[i].Location, fmt.Sprintf("unsupported[%d] location mismatch", i))
			assert.Equal(t, expected[i].Type, actual[i].Type, fmt.Sprintf("unsupported[%d] type mismatch", i))
			if expected[i].Error != nil {
				assert.Error(t, actual[i].Error, fmt.Sprintf("unsupported[%d] should have error", i))
				assert.Equal(t, expected[i].Error.Error(), actual[i].Error.Error(), fmt.Sprintf("unsupported[%d] error string mismatch", i))
			} else {
				assert.NoError(t, actual[i].Error, fmt.Sprintf("unsupported[%d] should not have error", i))
			}
		}
	} // No else needed, length assertion covers mismatch
}

// TestImageDetector_StandardMaps tests standard image map detection
func TestImageDetector_StandardMaps(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "standard_image_map",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
		{
			name: "partial_image_map_with_global_registry",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "app",
					"tag":        "latest",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{
						Registry:   "docker.io",
						Repository: "library/app",
						Tag:        "latest",
						Path:       []string{"image"},
					},
					Path:    []string{"image"},
					Pattern: PatternMap,
					Original: map[string]interface{}{
						"repository": "app",
						"tag":        "latest",
					},
				},
			},
		},
	}

	runImageDetectorTests(t, tests)
}

// TestImageDetector_ContainerPaths tests image detection in container paths
func TestImageDetector_ContainerPaths(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "string_image_in_known_path",
			values: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"image": "quay.io/prometheus/node-exporter:v1.3.1",
								},
							},
						},
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "quay.io", Repository: "prometheus/node-exporter", Tag: "v1.3.1"},
					Path:      []string{"spec", "template", "spec", "containers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "quay.io/prometheus/node-exporter:v1.3.1",
				},
			},
		},
	}

	runImageDetectorTests(t, tests)
}

// TestImageDetector_DigestReferences tests digest-based image references
func TestImageDetector_DigestReferences(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "digest-based_references",
			values: map[string]interface{}{
				"image": "docker.io/nginx@sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
					},
					Path:     []string{"image"},
					Pattern:  PatternString,
					Original: "docker.io/nginx@sha256:1234567890123456789012345678901234567890123456789012345678901234",
				},
			},
		},
	}

	runImageDetectorTests(t, tests)
}

// TestImageDetector_MixedConfiguration tests detection with mixed configuration values
func TestImageDetector_MixedConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "non-image_configuration_values",
			values: map[string]interface{}{
				"port":               8080,
				"timeout":            "30s",
				"serviceAccountName": "default",
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
	}

	runImageDetectorTests(t, tests)
}

// Helper function to run common test logic
func runImageDetectorTests(t *testing.T, tests []struct {
	name         string
	values       interface{}
	wantDetected []DetectedImage
}) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &DetectionContext{
				SourceRegistries: []string{"docker.io", "quay.io", "my-registry.example.com"},
			}

			// Set GlobalRegistry for specific tests
			if tc.name == "partial_image_map_with_global_registry" {
				ctx.GlobalRegistry = "docker.io"
			}

			detector := NewDetector(*ctx)
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported)

			// Sort both slices for consistent comparison
			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)

			assertDetectedImages(t, tc.wantDetected, gotDetected, true)
		})
	}
}

// TestImageDetector_StringDetection tests string-based image reference detection
func TestImageDetector_StringDetection(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "string_image_in_known_path",
			values: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"image": "quay.io/prometheus/node-exporter:v1.3.1",
								},
							},
						},
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "quay.io", Repository: "prometheus/node-exporter", Tag: "v1.3.1"},
					Path:      []string{"spec", "template", "spec", "containers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "quay.io/prometheus/node-exporter:v1.3.1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Provide context with the relevant source registry for this test
			ctx := DetectionContext{
				SourceRegistries: []string{"quay.io"},
			}
			detector := NewDetector(ctx)
			detected, unsupported, err := detector.DetectImages(tt.values, nil)
			require.NoError(t, err)
			require.Empty(t, unsupported)

			SortDetectedImages(detected)
			SortDetectedImages(tt.wantDetected)
			assertDetectedImages(t, tt.wantDetected, detected, true)
		})
	}
}

// TestImageDetector_MixedValues tests detection with mixed value types
func TestImageDetector_MixedValues(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "non-image_boolean_values",
			values: map[string]interface{}{
				"enabled": true,
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Context needed for default registry (docker.io)
			ctx := DetectionContext{
				SourceRegistries: []string{"docker.io"},
			}
			detector := NewDetector(ctx)
			detected, unsupported, err := detector.DetectImages(tt.values, nil)
			require.NoError(t, err)
			require.Empty(t, unsupported)

			SortDetectedImages(detected)
			SortDetectedImages(tt.wantDetected)
			assertDetectedImages(t, tt.wantDetected, detected, true)
		})
	}
}

// TestImageDetector_ArrayBasedImages tests detection in array structures
func TestImageDetector_ArrayBasedImages(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "array-based_images",
			values: map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "nginx:1.23",
					},
					map[string]interface{}{
						"name":  "sidecar",
						"image": "fluentd:v1.14",
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"containers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "nginx:1.23",
				},
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/fluentd", Tag: "v1.14"},
					Path:      []string{"containers", "[1]", "image"},
					Pattern:   PatternString,
					Original:  "fluentd:v1.14",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Context needed for default registry (docker.io)
			ctx := DetectionContext{
				SourceRegistries: []string{"docker.io"},
			}
			detector := NewDetector(ctx)
			detected, unsupported, err := detector.DetectImages(tt.values, nil)
			require.NoError(t, err)
			require.Empty(t, unsupported)

			SortDetectedImages(detected)
			SortDetectedImages(tt.wantDetected)
			assertDetectedImages(t, tt.wantDetected, detected, true)
		})
	}
}

// TestImageDetector_EmptyInputs tests detection behavior with empty or nil inputs
func TestImageDetector_EmptyInputs(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name:         "nil_input",
			values:       nil,
			wantDetected: []DetectedImage{},
		},
		{
			name:         "empty_map",
			values:       map[string]interface{}{},
			wantDetected: []DetectedImage{},
		},
		{
			name: "empty_nested_map",
			values: map[string]interface{}{
				"image": map[string]interface{}{},
			},
			wantDetected: []DetectedImage{},
		},
	}

	runImageDetectorTests(t, tests)
}

// TestImageDetector_InvalidTypes tests detection behavior with invalid value types
func TestImageDetector_InvalidTypes(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "invalid_type_in_map",
			values: map[string]interface{}{
				"image": 42,
			},
			wantDetected: []DetectedImage{},
		},
		{
			name: "invalid_repository_type",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": true,
					"tag":        "1.0",
				},
			},
			wantDetected: []DetectedImage{},
		},
		{
			name: "invalid_tag_type",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        []string{"1.0"},
				},
			},
			wantDetected: []DetectedImage{},
		},
	}

	runImageDetectorTests(t, tests)
}

// TestImageDetector_DeeplyNestedImages tests detection in deeply nested structures
func TestImageDetector_DeeplyNestedImages(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "deeply_nested_map",
			values: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"image": map[string]interface{}{
								"repository": "nginx",
								"tag":        "1.23",
							},
						},
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"level1", "level2", "level3", "image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
	}

	runImageDetectorTests(t, tests)
}

// TestImageDetector_MixedValidityImages tests detection with a mix of valid and invalid images
func TestImageDetector_MixedValidityImages(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "mixed_valid_and_invalid",
			values: map[string]interface{}{
				"valid": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "nginx",
						"tag":        "1.23",
					},
				},
				"invalid": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": 42,
						"tag":        true,
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"valid", "image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
	}

	runImageDetectorTests(t, tests)
}

// TestImageDetector_GlobalRegistryBasic tests basic global registry functionality
func TestImageDetector_GlobalRegistryBasic(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "basic_global_registry",
			values: map[string]interface{}{
				"global": map[string]interface{}{
					"registry": "my-registry.example.com",
				},
				"image": map[string]interface{}{
					"repository": "app",
					"tag":        "1.0",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "my-registry.example.com", Repository: "app", Tag: "1.0"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "app", "tag": "1.0"},
				},
			},
		},
	}

	ctx := &DetectionContext{
		GlobalRegistry:   "my-registry.example.com",
		SourceRegistries: []string{"docker.io", "my-registry.example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewDetector(*ctx)
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported)

			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)
			assertDetectedImages(t, tc.wantDetected, gotDetected, true)
		})
	}
}

// TestImageDetector_GlobalRegistryOverride tests global registry override behavior
func TestImageDetector_GlobalRegistryOverride(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "explicit_registry_override",
			values: map[string]interface{}{
				"global": map[string]interface{}{
					"registry": "my-registry.example.com",
				},
				"image": map[string]interface{}{
					"registry":   "quay.io",
					"repository": "app",
					"tag":        "1.0",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "quay.io", Repository: "app", Tag: "1.0"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"registry": "quay.io", "repository": "app", "tag": "1.0"},
				},
			},
		},
	}

	ctx := &DetectionContext{
		GlobalRegistry:   "my-registry.example.com",
		SourceRegistries: []string{"docker.io", "my-registry.example.com", "quay.io"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewDetector(*ctx)
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported)

			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)
			assertDetectedImages(t, tc.wantDetected, gotDetected, true)
		})
	}
}

// TestImageDetector_GlobalRegistryMultiImage tests global registry with multiple images
func TestImageDetector_GlobalRegistryMultiImage(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "multiple_images_with_global",
			values: map[string]interface{}{
				"global": map[string]interface{}{
					"registry": "my-registry.example.com",
				},
				"app": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "app",
						"tag":        "1.0",
					},
				},
				"sidecar": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "quay.io",
						"repository": "helper",
						"tag":        "latest",
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "my-registry.example.com", Repository: "app", Tag: "1.0"},
					Path:      []string{"app", "image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "app", "tag": "1.0"},
				},
				{
					Reference: &Reference{Registry: "quay.io", Repository: "helper", Tag: "latest"},
					Path:      []string{"sidecar", "image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"registry": "quay.io", "repository": "helper", "tag": "latest"},
				},
			},
		},
	}

	ctx := &DetectionContext{
		GlobalRegistry:   "my-registry.example.com",
		SourceRegistries: []string{"docker.io", "my-registry.example.com", "quay.io"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewDetector(*ctx)
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported)

			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)
			assertDetectedImages(t, tc.wantDetected, gotDetected, true)
		})
	}
}

func TestImageDetector_TemplateVariables(t *testing.T) {
	// Helper function to check the common unsupported error structure
	checkTemplateVariableUnsupportedError := func(t *testing.T, unsupported []UnsupportedImage) {
		t.Helper()
		require.Len(t, unsupported, 1)
		assert.ErrorIs(t, unsupported[0].Error, ErrTemplateVariableDetected, "Error should be ErrTemplateVariableDetected")
		assert.Equal(t, []string{"image"}, unsupported[0].Location, "Unsupported location path mismatch")
		assert.Equal(t, UnsupportedTypeTemplateMap, unsupported[0].Type, "Unsupported type mismatch")
	}

	tests := []struct {
		name                     string
		values                   map[string]interface{}
		context                  DetectionContext
		expectedCount            int                                                // Expected DETECTED count
		expectedUnsupportedCount int                                                // Expected UNSUPPORTED count
		checkError               func(t *testing.T, unsupported []UnsupportedImage) // Function to check unsupported details
	}{
		{
			name: "template variable in tag",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "{{ .Chart.AppVersion }}",
				},
			},
			context: DetectionContext{
				SourceRegistries: []string{"docker.io"}, // Assume docker.io for nginx
				Strict:           true,                  // Enable strict mode
			},
			expectedCount:            0,                                     // Should NOT be detected in strict mode
			expectedUnsupportedCount: 1,                                     // Should be marked as unsupported
			checkError:               checkTemplateVariableUnsupportedError, // Use helper
		},
		{
			name: "template variable in repository",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "{{ .Values.global.repository }}/nginx",
					"tag":        "1.23",
				},
			},
			context: DetectionContext{
				SourceRegistries: []string{"docker.io"}, // Assume docker.io
				Strict:           true,                  // Enable strict mode
			},
			expectedCount:            0,                                     // Should NOT be detected in strict mode
			expectedUnsupportedCount: 1,                                     // Should be marked as unsupported
			checkError:               checkTemplateVariableUnsupportedError, // Use helper
		},
		// Add more test cases for templates in strings, different strictness levels etc.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewDetector(tt.context)
			detected, unsupported, err := detector.DetectImages(tt.values, nil) // Start path is nil

			require.NoError(t, err, "DetectImages should not return a top-level error")

			assert.Len(t, detected, tt.expectedCount, "Detected image count mismatch")
			// Use helper assertions if needed
			// assertDetectedImagesMatch(t, tt.expectedDetected, detected, "Detected images mismatch (fallback diff)")

			assert.Len(t, unsupported, tt.expectedUnsupportedCount, "Unsupported image count mismatch")
			if tt.checkError != nil {
				tt.checkError(t, unsupported)
			}
			// Use helper assertions if needed
			// assertUnsupportedMatches(t, tt.expectedUnsupported, unsupported, "Unsupported images mismatch (fallback diff)")
		})
	}
}

func TestImageDetector_ContainerArrays(t *testing.T) {
	testCases := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "pod_template_containers",
			values: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "app",
									"image": "nginx:1.23",
								},
								map[string]interface{}{
									"name":  "sidecar",
									"image": "fluentd:v1.14",
								},
							},
						},
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{
						Original:   "nginx:1.23",
						Registry:   defaultRegistry,
						Repository: "library/nginx",
						Tag:        "1.23",
						Path:       []string{"spec", "template", "spec", "containers", "[0]", "image"},
					},
					Path:     []string{"spec", "template", "spec", "containers", "[0]", "image"},
					Pattern:  PatternString,
					Original: "nginx:1.23",
				},
				{
					Reference: &Reference{
						Original:   "fluentd:v1.14",
						Registry:   defaultRegistry,
						Repository: "library/fluentd",
						Tag:        "v1.14",
						Path:       []string{"spec", "template", "spec", "containers", "[1]", "image"},
					},
					Path:     []string{"spec", "template", "spec", "containers", "[1]", "image"},
					Pattern:  PatternString,
					Original: "fluentd:v1.14",
				},
			},
		},
		{
			name: "init_containers",
			values: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"initContainers": []interface{}{
								map[string]interface{}{
									"name":  "init",
									"image": "busybox:1.35",
								},
							},
						},
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{
						Original:   "busybox:1.35",
						Registry:   defaultRegistry,
						Repository: "library/busybox",
						Tag:        "1.35",
						Path:       []string{"spec", "template", "spec", "initContainers", "[0]", "image"},
					},
					Path:     []string{"spec", "template", "spec", "initContainers", "[0]", "image"},
					Pattern:  PatternString,
					Original: "busybox:1.35",
				},
			},
		},
		// Add more cases if needed, e.g., empty arrays, different nesting
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &DetectionContext{
				SourceRegistries: []string{defaultRegistry},
				TemplateMode:     true, // Enable template mode for container array tests
			}
			detector := NewDetector(*ctx)
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported)

			// Use capitalized sorting functions that handle array indices properly
			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)

			// Compare Detected Images field by field using helper (checking Original field)
			assertDetectedImages(t, tc.wantDetected, gotDetected, true)
		})
	}
}

// TestDetectImages_BasicDetection tests basic image detection scenarios
func TestDetectImages_BasicDetection(t *testing.T) {
	values := map[string]interface{}{
		"simpleImage": "nginx:1.19", // docker.io source
		"imageMap": map[string]interface{}{ // quay.io source
			"registry":   "quay.io",
			"repository": "org/app",
			"tag":        "v1.2.3",
		},
		"nestedImages": map[string]interface{}{
			"frontend": map[string]interface{}{ // docker.io source
				"image": "docker.io/frontend:latest",
			},
			"backend": map[string]interface{}{ // docker.io source (implicit)
				"image": map[string]interface{}{
					"repository": "backend",
					"tag":        "v1",
				},
			},
		},
		"excludedImage":  "private.registry.io/internal/app:latest", // excluded
		"nonSourceImage": "k8s.gcr.io/pause:3.1",                    // not source
	}

	wantDetected := []DetectedImage{
		{ // simpleImage
			Reference: &Reference{
				Original:   "nginx:1.19",
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "1.19",
				Path:       []string{"simpleImage"},
			},
			Path:     []string{"simpleImage"},
			Pattern:  PatternString,
			Original: "nginx:1.19",
		},
		{ // imageMap
			Reference: &Reference{
				Original:   "quay.io/org/app:v1.2.3",
				Registry:   "quay.io",
				Repository: "org/app",
				Tag:        "v1.2.3",
				Path:       []string{"imageMap"},
			},
			Path:    []string{"imageMap"},
			Pattern: PatternMap,
			Original: map[string]interface{}{
				"registry":   "quay.io",
				"repository": "org/app",
				"tag":        "v1.2.3",
			},
		},
		{ // nestedImages.frontend.image
			Reference: &Reference{
				Original:   "docker.io/frontend:latest",
				Registry:   "docker.io",
				Repository: "library/frontend",
				Tag:        "latest",
				Path:       []string{"nestedImages", "frontend", "image"},
			},
			Path:     []string{"nestedImages", "frontend", "image"},
			Pattern:  PatternString,
			Original: "docker.io/frontend:latest",
		},
		{ // nestedImages.backend.image
			Reference: &Reference{
				Original:   "backend:v1",
				Registry:   "docker.io",
				Repository: "library/backend",
				Tag:        "v1",
				Path:       []string{"nestedImages", "backend", "image"},
			},
			Path:    []string{"nestedImages", "backend", "image"},
			Pattern: PatternMap,
			Original: map[string]interface{}{
				"repository": "backend",
				"tag":        "v1",
			},
		},
	}

	// Context needed for multiple registries
	ctx := DetectionContext{
		SourceRegistries: []string{"docker.io", "quay.io"},
	}
	detector := NewDetector(ctx)
	gotDetected, gotUnsupported, err := detector.DetectImages(values, nil)
	require.NoError(t, err)

	// Use custom assertion helper if available, otherwise basic length check
	assertDetectedImages(t, wantDetected, gotDetected, true) // Check original values too
	assertUnsupportedImages(t, gotUnsupported, gotUnsupported)
}

// TestDetectImages_StrictMode tests image detection in strict mode
func TestDetectImages_StrictMode(t *testing.T) {
	values := map[string]interface{}{
		"appImage":            "docker.io/library/nginx:1.23",    // Valid, known path (.image), source registry
		"sidecarImage":        "other.registry/app:1.0",          // Valid, known path (.image), non-source registry
		"initImage":           "invalid-image-string",            // Invalid, known path (.image)
		"someOtherConfigKey":  "docker.io/library/alpine:latest", // Valid, unknown path
		"templatedValueImage": "{{ .Values.image.tag }}",         // Templated string, known path (.image)
	}

	expectedDetected := []DetectedImage{
		{
			Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
			Path:      []string{"appImage"},
			Pattern:   PatternString,
			Original:  "docker.io/library/nginx:1.23",
		},
	}
	expectedUnsupported := []UnsupportedImage{
		{
			Location: []string{"initImage"},
			Type:     UnsupportedTypeStringParseError,
			Error:    fmt.Errorf("strict mode: string at known image path [initImage] was skipped (likely invalid format)"),
		},
		{
			Location: []string{"sidecarImage"},
			Type:     UnsupportedTypeNonSourceImage,
			Error:    fmt.Errorf("strict mode: string at path [sidecarImage] is not from a configured source registry"),
		},
		{
			Location: []string{"templatedValueImage"},
			Type:     UnsupportedTypeTemplateString,
			Error:    fmt.Errorf("strict mode: template variable detected in string at path [templatedValueImage]"),
		},
	}

	// Run with strict mode and docker.io as source
	ctx := DetectionContext{
		Strict:           true,
		SourceRegistries: []string{"docker.io"},
	}
	detector := NewDetector(ctx)
	detected, unsupported, err := detector.DetectImages(values, nil)

	require.NoError(t, err)
	SortDetectedImages(detected)
	SortDetectedImages(expectedDetected)
	SortUnsupportedImages(unsupported)
	SortUnsupportedImages(expectedUnsupported)

	assertDetectedImages(t, expectedDetected, detected, true)
	assertUnsupportedImages(t, expectedUnsupported, unsupported)
}

// TestDetectImages_EmptyValues tests detection with empty inputs
func TestDetectImages_EmptyValues(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name:         "nil_values",
			values:       nil,
			wantDetected: []DetectedImage{},
		},
		{
			name:         "empty_map",
			values:       map[string]interface{}{},
			wantDetected: []DetectedImage{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewDetector(DetectionContext{})
			gotDetected, gotUnsupported, err := detector.DetectImages(tt.values, nil)
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported)
			assertDetectedImages(t, tt.wantDetected, gotDetected, true)
		})
	}
}

// TestDetectImages_WithStartingPath tests detection with a starting path
func TestDetectImages_WithStartingPath(t *testing.T) {
	values := map[string]interface{}{
		"simpleImage": "nginx:1.19",
		"imageMap":    map[string]interface{}{"registry": "quay.io", "repository": "org/app", "tag": "v1.2.3"},
		"nestedImages": map[string]interface{}{
			"frontend": map[string]interface{}{"image": "docker.io/frontend:latest"},
			"backend":  map[string]interface{}{"image": map[string]interface{}{"repository": "backend", "tag": "v1"}},
		},
		"excludedImage":  "private.registry.io/internal/app:latest",
		"nonSourceImage": "k8s.gcr.io/pause:3.1",
	}

	wantDetected := []DetectedImage{
		{ // imageMap
			Reference: &Reference{
				Original:   "quay.io/org/app:v1.2.3",
				Registry:   "quay.io",
				Repository: "org/app",
				Tag:        "v1.2.3",
				Path:       []string{"nestedImages", "imageMap"},
			},
			Path:    []string{"nestedImages", "imageMap"},
			Pattern: PatternMap,
			Original: map[string]interface{}{
				"registry":   "quay.io",
				"repository": "org/app",
				"tag":        "v1.2.3",
			},
		},
		{ // nestedImages.backend.image
			Reference: &Reference{
				Original:   "backend:v1",
				Registry:   "docker.io",
				Repository: "library/backend",
				Tag:        "v1",
				Path:       []string{"nestedImages", "nestedImages", "backend", "image"},
			},
			Path:    []string{"nestedImages", "nestedImages", "backend", "image"},
			Pattern: PatternMap,
			Original: map[string]interface{}{
				"repository": "backend",
				"tag":        "v1",
			},
		},
		{ // nestedImages.frontend.image
			Reference: &Reference{
				Original:   "docker.io/frontend:latest",
				Registry:   "docker.io",
				Repository: "library/frontend",
				Tag:        "latest",
				Path:       []string{"nestedImages", "nestedImages", "frontend", "image"},
			},
			Path:     []string{"nestedImages", "nestedImages", "frontend", "image"},
			Pattern:  PatternString,
			Original: "docker.io/frontend:latest",
		},
		{ // simpleImage
			Reference: &Reference{
				Original:   "nginx:1.19",
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "1.19",
				Path:       []string{"nestedImages", "simpleImage"},
			},
			Path:     []string{"nestedImages", "simpleImage"},
			Pattern:  PatternString,
			Original: "nginx:1.19",
		},
	}

	// Context needed for multiple registries
	ctx := DetectionContext{
		SourceRegistries: []string{"docker.io", "quay.io"},
	}
	detector := NewDetector(ctx)
	gotDetected, gotUnsupported, err := detector.DetectImages(values, []string{"nestedImages"}) // Start detection at nestedImages
	require.NoError(t, err)

	SortDetectedImages(gotDetected)
	SortDetectedImages(wantDetected)
	assertDetectedImages(t, wantDetected, gotDetected, true)
	assertUnsupportedImages(t, gotUnsupported, gotUnsupported)
}

// TestImageDetector_NonImageConfiguration tests handling of non-image configuration values
func TestImageDetector_NonImageConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "mixed_configuration_values",
			values: map[string]interface{}{
				"port":               8080,
				"timeout":            "30s",
				"serviceAccountName": "default",
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Context needed for default registry (docker.io)
			ctx := DetectionContext{
				SourceRegistries: []string{"docker.io"},
			}
			detector := NewDetector(ctx)
			detected, unsupported, err := detector.DetectImages(tt.values, nil)
			require.NoError(t, err)
			require.Empty(t, unsupported)

			SortDetectedImages(detected)
			SortDetectedImages(tt.wantDetected)
			assertDetectedImages(t, tt.wantDetected, detected, true)
		})
	}
}

// TestImageDetector_BasicContainerArray tests basic container array detection
func TestImageDetector_BasicContainerArray(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "single_container",
			values: map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": "nginx:1.23",
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"containers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "nginx:1.23",
				},
			},
		},
	}

	runImageDetectorTests(t, tests)
}

// TestImageDetector_MultiContainerArray tests detection in multi-container arrays
func TestImageDetector_MultiContainerArray(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "multiple_containers",
			values: map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": "nginx:1.23",
					},
					map[string]interface{}{
						"name":  "sidecar",
						"image": "fluentd:v1.14",
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"containers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "nginx:1.23",
				},
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/fluentd", Tag: "v1.14"},
					Path:      []string{"containers", "[1]", "image"},
					Pattern:   PatternString,
					Original:  "fluentd:v1.14",
				},
			},
		},
	}

	runImageDetectorTests(t, tests)
}

// TestImageDetector_NestedContainerArray tests detection in nested container arrays
func TestImageDetector_NestedContainerArray(t *testing.T) {
	tests := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "nested_containers",
			values: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "app",
									"image": "nginx:1.23",
								},
							},
							"initContainers": []interface{}{
								map[string]interface{}{
									"name":  "init",
									"image": "busybox:latest",
								},
							},
						},
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"spec", "template", "spec", "containers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "nginx:1.23",
				},
				{
					Reference: &Reference{Registry: "docker.io", Repository: "library/busybox", Tag: "latest"},
					Path:      []string{"spec", "template", "spec", "initContainers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "busybox:latest",
				},
			},
		},
	}

	runImageDetectorTests(t, tests)
}
