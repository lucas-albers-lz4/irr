package image

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

// Helper function to sort DetectedImage slices by path for consistent comparison
func sortDetectedImages(images []DetectedImage) {
	sort.Slice(images, func(i, j int) bool {
		pathI := strings.Join(images[i].Path, "/")
		pathJ := strings.Join(images[j].Path, "/")
		if pathI != pathJ {
			return pathI < pathJ
		}
		// If paths are equal, sort by Pattern for stability
		return images[i].Pattern < images[j].Pattern
	})
}

// Helper function to sort UnsupportedImage slices by path for consistent comparison
func sortUnsupportedImages(images []UnsupportedImage) {
	sort.Slice(images, func(i, j int) bool {
		pathI := strings.Join(images[i].Location, "/")
		pathJ := strings.Join(images[j].Location, "/")
		if pathI != pathJ {
			return pathI < pathJ
		}
		// If paths are equal, sort by Type for stability
		return images[i].Type < images[j].Type
	})
}

func TestImageDetector(t *testing.T) {
	type testCase struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}

	testCases := []testCase{
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
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
		{
			name: "partial_image_map_with_global_registry",
			values: map[string]interface{}{
				"global": map[string]interface{}{
					"registry": "my-registry.example.com",
				},
				"image": map[string]interface{}{
					"repository": "app",
					"tag":        "v1.0",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "app", Tag: "v1.0"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "app", "tag": "v1.0"},
				},
			},
		},
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
					Reference: &ImageReference{Registry: "quay.io", Repository: "prometheus/node-exporter", Tag: "v1.3.1"},
					Path:      []string{"spec", "template", "spec", "containers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "quay.io/prometheus/node-exporter:v1.3.1",
				},
			},
		},
		{
			name: "image_with_template_variables",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "{{ .Chart.AppVersion }}",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Tag: "{{ .Chart.AppVersion }}"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "{{ .Chart.AppVersion }}"},
				},
			},
		},
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
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
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
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"containers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "nginx:1.23",
				},
				{
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/fluentd", Tag: "v1.14"},
					Path:      []string{"containers", "[1]", "image"},
					Pattern:   PatternString,
					Original:  "fluentd:v1.14",
				},
			},
		},
		{
			name: "digest-based_references",
			values: map[string]interface{}{
				"image": "docker.io/nginx@sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Digest: "sha256:1234567890123456789012345678901234567890123456789012345678901234"},
					Path:      []string{"image"},
					Pattern:   PatternString,
					Original:  "docker.io/nginx@sha256:1234567890123456789012345678901234567890123456789012345678901234",
				},
			},
		},
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
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(&DetectionContext{
				SourceRegistries: []string{"docker.io", "quay.io", "my-registry.example.com"},
			})
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported) // Assuming these tests should not produce unsupported images

			// Sort both slices for consistent comparison
			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)

			assert.Len(t, gotDetected, len(tc.wantDetected), "number of detected images mismatch")

			if len(gotDetected) == len(tc.wantDetected) {
				for i := range gotDetected {
					assert.Equal(t, tc.wantDetected[i].Path, gotDetected[i].Path, "path mismatch")
					assert.Equal(t, tc.wantDetected[i].Pattern, gotDetected[i].Pattern, "pattern mismatch")
					if assert.NotNil(t, gotDetected[i].Reference) && assert.NotNil(t, tc.wantDetected[i].Reference) {
						assert.Equal(t, tc.wantDetected[i].Reference.Registry, gotDetected[i].Reference.Registry, "registry mismatch")
						assert.Equal(t, tc.wantDetected[i].Reference.Repository, gotDetected[i].Reference.Repository, "repository mismatch")
						assert.Equal(t, tc.wantDetected[i].Reference.Tag, gotDetected[i].Reference.Tag, "tag mismatch")
						assert.Equal(t, tc.wantDetected[i].Reference.Digest, gotDetected[i].Reference.Digest, "digest mismatch")
					}
				}
			} else {
				// Use reflect.DeepEqual for a detailed diff if lengths differ
				assert.Equal(t, tc.wantDetected, gotDetected, "Mismatch in detected images")
			}
		})
	}
}

func TestImageDetector_DetectImages_EdgeCases(t *testing.T) {
	testCases := map[string]struct {
		name                     string
		input                    interface{}
		expected                 []DetectedImage
		strict                   bool
		expectedError            bool
		expectedUnsupportedCount int
		expectedUnsupported      []UnsupportedImage // Expect non-nil empty slice if count is 0
	}{
		"nil_values": {
			input:                    nil,
			expected:                 []DetectedImage{},
			strict:                   false,
			expectedUnsupportedCount: 0,
			expectedUnsupported:      []UnsupportedImage{}, // Expect empty slice
		},
		"empty_map": {
			input:                    map[string]interface{}{},
			expected:                 []DetectedImage{},
			strict:                   false,
			expectedUnsupportedCount: 0,
			expectedUnsupported:      []UnsupportedImage{}, // Expect empty slice
		},
		"invalid_type_in_image_map": {
			input: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": 123, // Invalid type
					"tag":        "1.23",
				},
			},
			expected:                 []DetectedImage{},
			strict:                   true,
			expectedError:            false, // Should be handled as unsupported
			expectedUnsupportedCount: 1,
			expectedUnsupported: []UnsupportedImage{
				{Location: []string{"image"}, Type: UnsupportedTypeMap, Error: ErrInvalidImageMapRepo},
			},
		},
		"deeply_nested_valid_image": {
			input: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": map[string]interface{}{
							"image": map[string]interface{}{
								"repository": "nginx",
								"tag":        "1.23",
							},
						},
					},
				},
			},
			expected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"a", "b", "c", "image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
			strict:                   false,
			expectedUnsupportedCount: 0,
			expectedUnsupported:      []UnsupportedImage{}, // Expect empty slice
		},
		"mixed_valid_and_invalid_images": {
			input: map[string]interface{}{
				"valid": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "nginx",
						"tag":        "1.23",
					},
				},
				"invalid": map[string]interface{}{
					"image": "not:a:valid:image", // Invalid string format
				},
			},
			expected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"valid", "image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
			strict:                   true,
			expectedError:            false,
			expectedUnsupportedCount: 1,
			expectedUnsupported: []UnsupportedImage{
				// Assuming ParseImageReference fails with ErrInvalidRepoName for "not:a:valid:image"
				{Location: []string{"invalid", "image"}, Type: UnsupportedTypeString, Error: ErrInvalidRepoName},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := &DetectionContext{Strict: tc.strict, SourceRegistries: []string{defaultRegistry}}
			detector := NewImageDetector(ctx)
			detected, unsupported, err := detector.DetectImages(tc.input, nil)

			if tc.expectedError {
				assert.Error(t, err)
				return // Stop checks if a general error was expected
			}
			assert.NoError(t, err)

			// Sort slices for consistent comparison
			sortDetectedImages(detected)
			sortDetectedImages(tc.expected)
			sortUnsupportedImages(unsupported)
			sortUnsupportedImages(tc.expectedUnsupported)

			assert.Equal(t, tc.expected, detected)

			// Check unsupported images
			assert.Len(t, unsupported, tc.expectedUnsupportedCount)
			if len(unsupported) == tc.expectedUnsupportedCount && tc.expectedUnsupportedCount > 0 {
				assert.Equal(t, tc.expectedUnsupported, unsupported, "Unsupported images mismatch")
			} else if tc.expectedUnsupportedCount == 0 {
				assert.Empty(t, unsupported, "Expected no unsupported images")
			}
		})
	}
}

func TestImageDetector_GlobalRegistry(t *testing.T) {
	testCases := []struct {
		name         string
		values       interface{}
		globalReg    string
		wantDetected []DetectedImage
	}{
		{
			name: "global_registry_with_multiple_images",
			values: map[string]interface{}{
				"global": map[string]interface{}{
					"imageRegistry": "my-registry.example.com", // Note: Key differs from context
				},
				"frontend": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "frontend-app",
						"tag":        "v1.0",
					},
				},
				"backend": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "backend-app",
						"tag":        "v2.0",
					},
				},
			},
			globalReg: "my-registry.example.com", // Set in context
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: "my-registry.example.com", Repository: "frontend-app", Tag: "v1.0"},
					Path:      []string{"frontend", "image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "frontend-app", "tag": "v1.0"},
				},
				{
					Reference: &ImageReference{Registry: "my-registry.example.com", Repository: "backend-app", Tag: "v2.0"},
					Path:      []string{"backend", "image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "backend-app", "tag": "v2.0"},
				},
			},
		},
		{
			name: "global_registry_in_context",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			globalReg: "global.registry.com",
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: "global.registry.com", Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "1.23"},
				},
			},
		},
		{
			name: "registry_precedence_-_map_registry_over_global",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "specific.registry.com",
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			globalReg: "global.registry.com",
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: "specific.registry.com", Repository: "nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"registry": "specific.registry.com", "repository": "nginx", "tag": "1.23"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(&DetectionContext{
				GlobalRegistry: tc.globalReg,
				// Add source/exclude registries if relevant for these tests
			})
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported)

			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)

			assert.Equal(t, tc.wantDetected, gotDetected)
		})
	}
}

func TestImageDetector_TemplateVariables(t *testing.T) {
	testCases := []struct {
		name            string
		values          interface{}
		strict          bool // Add strict mode testing if needed
		wantDetected    []DetectedImage
		wantUnsupported []UnsupportedImage
	}{
		{
			name: "template_variable_in_tag",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "{{ .Chart.AppVersion }}",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "library/nginx", Tag: "{{ .Chart.AppVersion }}"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "{{ .Chart.AppVersion }}"},
				},
			},
		},
		{
			name: "template_variable_in_repository",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "{{ .Values.global.repository }}/nginx",
					"tag":        "1.23",
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "{{ .Values.global.repository }}/nginx", Tag: "1.23"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "{{ .Values.global.repository }}/nginx", "tag": "1.23"},
				},
			},
		},
		// Add more test cases, e.g., template in registry, strict mode behavior
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(&DetectionContext{
				Strict: tc.strict,
				// TemplateMode might be relevant if validation changes based on it
			})
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err) // Assuming template vars themselves don't cause errors

			// Compare detected images
			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)
			assert.Equal(t, tc.wantDetected, gotDetected, "Detected images mismatch")

			// Compare unsupported images (adjust expectations based on strict mode)
			SortUnsupportedImages(gotUnsupported)
			SortUnsupportedImages(tc.wantUnsupported)
			assert.Equal(t, tc.wantUnsupported, gotUnsupported, "Unsupported images mismatch")
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
								map[string]interface{}{"name": "main", "image": "nginx:1.23"},
								map[string]interface{}{"name": "sidecar", "image": "fluentd:v1.14"},
							},
						},
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "library/nginx", Tag: "1.23"},
					Path:      []string{"spec", "template", "spec", "containers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "nginx:1.23",
				},
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "library/fluentd", Tag: "v1.14"},
					Path:      []string{"spec", "template", "spec", "containers", "[1]", "image"},
					Pattern:   PatternString,
					Original:  "fluentd:v1.14",
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
								map[string]interface{}{"name": "init", "image": "busybox:1.35"},
							},
						},
					},
				},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "library/busybox", Tag: "1.35"},
					Path:      []string{"spec", "template", "spec", "initContainers", "[0]", "image"},
					Pattern:   PatternString,
					Original:  "busybox:1.35",
				},
			},
		},
		// Add more cases if needed, e.g., empty arrays, different nesting
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(&DetectionContext{})
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported)

			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)

			assert.Equal(t, tc.wantDetected, gotDetected)
		})
	}
}

func TestDetectImages(t *testing.T) {
	tests := []struct {
		name              string
		values            interface{}
		startingPath      []string
		sourceRegistries  []string
		excludeRegistries []string
		strict            bool
		wantDetected      []DetectedImage
		wantUnsupported   []UnsupportedImage
	}{
		{
			name: "Basic detection",
			values: map[string]interface{}{
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
			},
			sourceRegistries:  []string{"docker.io", "quay.io"},
			excludeRegistries: []string{"private.registry.io"},
			wantDetected: []DetectedImage{
				{ // simpleImage
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.19"},
					Path:      []string{"simpleImage"}, Pattern: PatternString,
					Original: "nginx:1.19",
				},
				{ // imageMap
					Reference: &ImageReference{Registry: "quay.io", Repository: "org/app", Tag: "v1.2.3"},
					Path:      []string{"imageMap"}, Pattern: PatternMap,
					Original: map[string]interface{}{"registry": "quay.io", "repository": "org/app", "tag": "v1.2.3"},
				},
				{ // nestedImages.frontend.image
					Reference: &ImageReference{Registry: "docker.io", Repository: "frontend", Tag: "latest"},
					Path:      []string{"nestedImages", "frontend", "image"}, Pattern: PatternString,
					Original: "docker.io/frontend:latest",
				},
				{ // nestedImages.backend.image
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/backend", Tag: "v1"},
					Path:      []string{"nestedImages", "backend", "image"}, Pattern: PatternMap,
					Original: map[string]interface{}{"repository": "backend", "tag": "v1"},
				},
			},
		},
		{
			name: "Strict mode",
			values: map[string]interface{}{
				"simpleImage":    "nginx:1.19",                              // Valid, detected
				"invalidMap":     map[string]interface{}{"repository": 123}, // Invalid type, unsupported
				"invalidString":  "not_a_valid_image:tag",                   // Invalid format, unsupported
				"nonSourceImage": "k8s.gcr.io/pause:3.1",                    // Valid format but not source, unsupported
			},
			sourceRegistries: []string{"docker.io"},
			strict:           true,
			wantDetected: []DetectedImage{ // Only the valid source image
				{
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.19"},
					Path:      []string{"simpleImage"}, Pattern: PatternString,
					Original: "nginx:1.19",
				},
			},
			wantUnsupported: []UnsupportedImage{
				{Location: []string{"invalidMap"}, Type: UnsupportedTypeMap, Error: ErrInvalidImageMapRepo},
				// Error for invalidString might depend on ParseImageReference specifics
				{Location: []string{"invalidString"}, Type: UnsupportedTypeString /* Error: ... */},
				// Error for nonSourceImage is nil because format is valid, it's just not a source
				{Location: []string{"nonSourceImage"}, Type: UnsupportedTypeString, Error: nil},
			},
		},
		{
			name:         "Empty values",
			values:       nil,
			wantDetected: []DetectedImage{},
		},
		{
			name:         "Empty map value",
			values:       map[string]interface{}{},
			wantDetected: []DetectedImage{},
		},
		{
			name: "With starting path",
			values: map[string]interface{}{ // Same values as Basic detection
				"simpleImage": "nginx:1.19",
				"imageMap":    map[string]interface{}{"registry": "quay.io", "repository": "org/app", "tag": "v1.2.3"},
				"nestedImages": map[string]interface{}{
					"frontend": map[string]interface{}{"image": "docker.io/frontend:latest"},
					"backend":  map[string]interface{}{"image": map[string]interface{}{"repository": "backend", "tag": "v1"}},
				},
				"excludedImage":  "private.registry.io/internal/app:latest",
				"nonSourceImage": "k8s.gcr.io/pause:3.1",
			},
			startingPath:      []string{"nestedImages"}, // Start search here
			sourceRegistries:  []string{"docker.io", "quay.io"},
			excludeRegistries: []string{"private.registry.io"},
			wantDetected: []DetectedImage{ // Only images under nestedImages
				{
					Reference: &ImageReference{Registry: "docker.io", Repository: "frontend", Tag: "latest"},
					Path:      []string{"nestedImages", "frontend", "image"}, Pattern: PatternString,
					Original: "docker.io/frontend:latest",
				},
				{
					Reference: &ImageReference{Registry: "docker.io", Repository: "library/backend", Tag: "v1"},
					Path:      []string{"nestedImages", "backend", "image"}, Pattern: PatternMap,
					Original: map[string]interface{}{"repository": "backend", "tag": "v1"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: DetectImages is the exported function, not the method
			gotDetected, gotUnsupported, err := DetectImages(tt.values, tt.startingPath, tt.sourceRegistries, tt.excludeRegistries, tt.strict)
			assert.NoError(t, err) // Assuming detection itself doesn't error easily

			// Sort results for comparison
			SortDetectedImages(gotDetected)
			SortDetectedImages(tt.wantDetected)
			SortUnsupportedImages(gotUnsupported)
			SortUnsupportedImages(tt.wantUnsupported)

			assert.Equal(t, tt.wantDetected, gotDetected, "Detected images mismatch")
			// Add more robust error checking for unsupported if needed
			assert.Equal(t, len(tt.wantUnsupported), len(gotUnsupported), "Unsupported images count mismatch")
			// Further checks on unsupported content if necessary
			for i := range gotUnsupported {
				if i < len(tt.wantUnsupported) {
					assert.Equal(t, tt.wantUnsupported[i].Location, gotUnsupported[i].Location, "Unsupported location mismatch")
					assert.Equal(t, tt.wantUnsupported[i].Type, gotUnsupported[i].Type, "Unsupported type mismatch")
					// Error comparison can be tricky, maybe check error type or if it's nil/non-nil
					assert.Equal(t, tt.wantUnsupported[i].Error != nil, gotUnsupported[i].Error != nil, "Unsupported error presence mismatch")
				}
			}

		})
	}
}

func TestTryExtractImageFromString_EdgeCases(t *testing.T) {
	// ... tests for tryExtractImageFromString ...
}

func TestImageDetector_NonImageValues(t *testing.T) {
	testCases := []struct {
		name         string
		values       interface{}
		wantDetected []DetectedImage
	}{
		{
			name: "boolean_and_numeric_values",
			values: map[string]interface{}{
				"enabled": true,
				"port":    8080,
				"timeout": "30s",
				"image":   map[string]interface{}{"repository": "nginx", "tag": "latest"},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "library/nginx", Tag: "latest"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "latest"},
				},
			},
		},
		{
			name: "non-image_configuration_paths",
			values: map[string]interface{}{
				"labels":             map[string]interface{}{"app": "myapp"},
				"annotations":        map[string]interface{}{"prometheus.io/port": "9090"},
				"serviceAccountName": "default",
				"image":              map[string]interface{}{"repository": "nginx", "tag": "latest"},
			},
			wantDetected: []DetectedImage{
				{
					Reference: &ImageReference{Registry: defaultRegistry, Repository: "library/nginx", Tag: "latest"},
					Path:      []string{"image"},
					Pattern:   PatternMap,
					Original:  map[string]interface{}{"repository": "nginx", "tag": "latest"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(&DetectionContext{})
			gotDetected, gotUnsupported, err := detector.DetectImages(tc.values, []string{})
			assert.NoError(t, err)
			assert.Empty(t, gotUnsupported)

			SortDetectedImages(gotDetected)
			SortDetectedImages(tc.wantDetected)
			assert.Equal(t, tc.wantDetected, gotDetected)
		})
	}
}

func TestImageReference_String(t *testing.T) {
	// ... tests for ImageReference.String() ...
}

func TestIsValidImageReference(t *testing.T) {
	// ... tests for IsValidImageReference ...
}
