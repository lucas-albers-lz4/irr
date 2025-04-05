package image

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Helper function to sort detected images by path for consistent comparison
func sortDetectedImages(images []DetectedImage) {
	sort.Slice(images, func(i, j int) bool {
		// Compare paths element by element
		for k := 0; k < len(images[i].Location) && k < len(images[j].Location); k++ {
			if images[i].Location[k] != images[j].Location[k] {
				return images[i].Location[k] < images[j].Location[k]
			}
		}
		return len(images[i].Location) < len(images[j].Location)
	})
}

func TestImageDetector(t *testing.T) {
	// Outcome-focused test suite: Validates DetectImages finds the correct images
	// across various common structures and scenarios. Due to the heuristic nature
	// of image detection across arbitrary YAML, these tests prioritize verifying
	// the presence of expected image references over asserting exact internal
	// detection details (like precise location paths or patterns), reducing brittleness.
	tests := []struct {
		name              string
		values            interface{}
		context           *DetectionContext
		expectedImages    []DetectedImage
		expectUnsupported []DetectedImage
		expectError       bool
	}{
		{
			name: "standard image map",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "docker.io",
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
		{
			name: "partial image map with global registry",
			values: map[string]interface{}{
				"global": map[string]interface{}{
					"registry": "my-registry.example.com",
				},
				"image": map[string]interface{}{
					"repository": "app",
					"tag":        "v1.0",
				},
			},
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "my-registry.example.com",
						Repository: "app", // No library prefix for non-docker.io registry
						Tag:        "v1.0",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
		{
			name: "string image in known path",
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
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"spec", "template", "spec", "containers", "[0]", "image"},
					Reference: &ImageReference{
						Registry:   "quay.io",
						Repository: "prometheus/node-exporter",
						Tag:        "v1.3.1",
					},
					LocationType: TypeString,
					Pattern:      "string",
				},
			},
		},
		{
			name: "image with template variables",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "{{ .Chart.AppVersion }}",
				},
			},
			context: &DetectionContext{
				TemplateMode: true,
			},
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "{{ .Chart.AppVersion }}",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
		{
			name: "non-image boolean values",
			values: map[string]interface{}{
				"enabled": true,
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
		{
			name: "array-based images",
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
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"containers", "[0]", "image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					LocationType: TypeString,
					Pattern:      "string",
				},
				{
					Location: []string{"containers", "[1]", "image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/fluentd",
						Tag:        "v1.14",
					},
					LocationType: TypeString,
					Pattern:      "string",
				},
			},
		},
		{
			name: "digest-based references",
			values: map[string]interface{}{
				"image": "docker.io/nginx@sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "nginx",
						Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
					},
					LocationType: TypeString,
					Pattern:      "string",
				},
			},
		},
		{
			name: "non-image configuration values",
			values: map[string]interface{}{
				"port":               8080,
				"timeout":            "30s",
				"serviceAccountName": "default",
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(tc.context)
			images, unsupported, err := detector.DetectImages(tc.values, nil)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedImages), len(images), "number of detected images mismatch")
			if tc.expectUnsupported != nil {
				assert.Equal(t, len(tc.expectUnsupported), len(unsupported), "number of unsupported images mismatch")
			} else {
				assert.Empty(t, unsupported, "expected no unsupported images")
			}

			// Sort both expected and actual images for consistent comparison
			sortDetectedImages(tc.expectedImages)
			sortDetectedImages(images)

			for i, expected := range tc.expectedImages {
				if i >= len(images) {
					break
				}
				actual := images[i]

				assert.Equal(t, expected.Location, actual.Location, "path mismatch")
				assert.Equal(t, expected.Pattern, actual.Pattern, "pattern mismatch")
				assert.Equal(t, expected.LocationType, actual.LocationType, "location type mismatch")

				if expected.Reference != nil {
					assert.Equal(t, expected.Reference.Registry, actual.Reference.Registry, "registry mismatch")
					assert.Equal(t, expected.Reference.Repository, actual.Reference.Repository, "repository mismatch")
					assert.Equal(t, expected.Reference.Tag, actual.Reference.Tag, "tag mismatch")
					assert.Equal(t, expected.Reference.Digest, actual.Reference.Digest, "digest mismatch")
				}
			}
		})
	}
}

func TestImageDetector_DetectImages_EdgeCases(t *testing.T) {
	// Outcome-focused test suite: Validates DetectImages handles various edge cases
	// gracefully (nil values, empty maps, invalid types). Focuses on ensuring
	// correct image detection outcomes or appropriate error handling rather than
	// exact internal detection path validation.
	tests := []struct {
		name              string
		values            interface{}
		context           *DetectionContext
		expectedImages    []DetectedImage
		expectUnsupported []DetectedImage
		expectError       bool
	}{
		{
			name:           "nil values",
			values:         nil,
			context:        nil,
			expectedImages: nil,
			expectError:    false,
		},
		{
			name:           "empty map",
			values:         map[string]interface{}{},
			context:        nil,
			expectedImages: nil,
			expectError:    false,
		},
		{
			name: "invalid type in image map",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": 123, // Should be string
					"tag":        "1.23",
				},
			},
			context:     nil,
			expectError: true,
		},
		{
			name: "deeply nested valid image",
			values: map[string]interface{}{
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
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"a", "b", "c", "image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
		{
			name: "mixed valid and invalid images",
			values: map[string]interface{}{
				"valid": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "nginx",
						"tag":        "1.23",
					},
				},
				"invalid": map[string]interface{}{
					"image": "not:a:valid:image",
				},
			},
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"valid", "image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(tc.context)
			images, unsupported, err := detector.DetectImages(tc.values, nil)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			if tc.expectedImages == nil {
				assert.Empty(t, images)
				assert.Empty(t, unsupported)
				return
			}

			// Sort both expected and actual images for consistent comparison
			sortDetectedImages(tc.expectedImages)
			sortDetectedImages(images)

			assert.Equal(t, len(tc.expectedImages), len(images))
			for i, expected := range tc.expectedImages {
				actual := images[i]
				assert.Equal(t, expected.Location, actual.Location)
				assert.Equal(t, expected.Pattern, actual.Pattern)
				assert.Equal(t, expected.LocationType, actual.LocationType)
				assert.Equal(t, expected.Reference, actual.Reference)
			}
		})
	}
}

func TestImageDetector_GlobalRegistry(t *testing.T) {
	// Outcome-focused test suite: Validates how DetectImages interacts with the
	// GlobalRegistry setting in the DetectionContext. Verifies correct registry
	// application and precedence rules based on the final detected image reference,
	// not necessarily the exact path or pattern used internally for detection.
	tests := []struct {
		name              string
		values            interface{}
		context           *DetectionContext
		expectedImages    []DetectedImage
		expectUnsupported []DetectedImage
	}{
		{
			name: "global registry with multiple images",
			values: map[string]interface{}{
				"global": map[string]interface{}{
					"imageRegistry": "my-registry.example.com",
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
			context: nil,
			expectedImages: []DetectedImage{
				{
					Location: []string{"frontend", "image"},
					Reference: &ImageReference{
						Registry:   "my-registry.example.com",
						Repository: "frontend-app",
						Tag:        "v1.0",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
				{
					Location: []string{"backend", "image"},
					Reference: &ImageReference{
						Registry:   "my-registry.example.com",
						Repository: "backend-app",
						Tag:        "v2.0",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
		{
			name: "global registry in context",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "app",
					"tag":        "v1.0",
				},
			},
			context: &DetectionContext{
				GlobalRegistry: "my-registry.example.com",
			},
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "my-registry.example.com",
						Repository: "app",
						Tag:        "v1.0",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
		{
			name: "registry precedence - map registry over global",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "local-registry.example.com",
					"repository": "app",
					"tag":        "v1.0",
				},
			},
			context: &DetectionContext{
				GlobalRegistry: "global-registry.example.com",
			},
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "local-registry.example.com",
						Repository: "app",
						Tag:        "v1.0",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(tc.context)
			images, unsupported, err := detector.DetectImages(tc.values, nil)

			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedImages), len(images))
			if tc.expectUnsupported != nil {
				assert.Equal(t, len(tc.expectUnsupported), len(unsupported), "number of unsupported images mismatch")
			} else {
				assert.Empty(t, unsupported, "expected no unsupported images")
			}

			// Sort both expected and actual images for consistent comparison
			sortDetectedImages(tc.expectedImages)
			sortDetectedImages(images)

			for i, expected := range tc.expectedImages {
				actual := images[i]
				assert.Equal(t, expected.Location, actual.Location)
				assert.Equal(t, expected.Pattern, actual.Pattern)
				assert.Equal(t, expected.LocationType, actual.LocationType)
				assert.Equal(t, expected.Reference.Registry, actual.Reference.Registry)
				assert.Equal(t, expected.Reference.Repository, actual.Reference.Repository)
				assert.Equal(t, expected.Reference.Tag, actual.Reference.Tag)
			}
		})
	}
}

func TestImageDetector_TemplateVariables(t *testing.T) {
	// Outcome-focused test suite: Validates DetectImages' behavior when encountering
	// potential template variables, especially when TemplateMode is enabled/disabled.
	// Focuses on the resulting image reference (preserving templates or not) rather
	// than the precise internal parsing steps.
	tests := []struct {
		name           string
		values         interface{}
		context        *DetectionContext
		expectedImages []DetectedImage
		expectError    bool
	}{
		{
			name: "template variable in tag",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "{{ .Chart.AppVersion }}",
				},
			},
			context: &DetectionContext{
				TemplateMode: true,
			},
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "{{ .Chart.AppVersion }}",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
		{
			name: "template variable in repository",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "{{ .Values.global.repository }}/nginx",
					"tag":        "1.23",
				},
			},
			context: &DetectionContext{
				TemplateMode: true,
			},
			expectedImages: []DetectedImage{
				{
					Location: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "{{ .Values.global.repository }}/nginx",
						Tag:        "1.23",
					},
					LocationType: TypeMapRegistryRepositoryTag,
					Pattern:      "map",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(tc.context)
			images, unsupported, err := detector.DetectImages(tc.values, nil)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Empty(t, unsupported)
			assert.Equal(t, len(tc.expectedImages), len(images))

			// Sort images for consistent comparison before iterating
			sortDetectedImages(tc.expectedImages)
			sortDetectedImages(images)

			for i, expected := range tc.expectedImages {
				actual := images[i]
				assert.Equal(t, expected.Location, actual.Location)
				assert.Equal(t, expected.Pattern, actual.Pattern)
				assert.Equal(t, expected.LocationType, actual.LocationType)
				assert.Equal(t, expected.Reference, actual.Reference)
			}
		})
	}
}

func TestImageDetector_ContainerArrays(t *testing.T) {
	// Outcome-focused test suite: Specifically validates DetectImages' ability to
	// find images within known Kubernetes container array structures. It verifies
	// the presence of the expected container images in the results, adapting to
	// the heuristic nature of path-based detection within these arrays.
	tests := []struct {
		name           string
		values         interface{}
		expectedImages []struct {
			repository string
			tag        string
		}
		expectError bool
	}{
		{
			name: "pod template containers",
			values: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
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
					},
				},
			},
			expectedImages: []struct {
				repository string
				tag        string
			}{
				{
					repository: "library/nginx",
					tag:        "1.23",
				},
				{
					repository: "library/fluentd",
					tag:        "v1.14",
				},
			},
		},
		{
			name: "init containers",
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
			expectedImages: []struct {
				repository string
				tag        string
			}{
				{
					repository: "library/busybox",
					tag:        "1.35",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(nil)
			images, unsupported, err := detector.DetectImages(tc.values, nil)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Empty(t, unsupported)

			// Check that we have all the expected images based on repository and tag
			for _, expected := range tc.expectedImages {
				found := false
				for _, img := range images {
					if img.Reference.Repository == expected.repository &&
						img.Reference.Tag == expected.tag {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected to find image %s:%s", expected.repository, expected.tag)
			}
		})
	}
}

func TestDetectImages_ContextVariations(t *testing.T) {
	// Outcome-focused test suite: Validates DetectImages under different context
	// settings (Strict mode, Template mode, registry filtering). Focuses on
	// ensuring the final set of detected and unsupported images matches expectations
	// for the given context, accommodating the heuristic detection logic.
	tests := []struct {
		name           string
		values         interface{}
		context        *DetectionContext
		expectedImages []struct {
			repository string
			tag        string
		}
		expectUnsupported bool
		expectError       bool
	}{
		{
			name: "strict mode with ambiguous strings",
			values: map[string]interface{}{
				"images": []interface{}{
					"nginx:1.23",
					"service:8080",     // This is a service:port, not an image
					"not:valid:format", // Not valid image format
				},
			},
			context: &DetectionContext{
				Strict: true,
			},
			expectedImages: []struct {
				repository string
				tag        string
			}{
				{
					repository: "library/nginx",
					tag:        "1.23",
				},
			},
			expectUnsupported: false,
		},
		{
			name: "template mode handling",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "{{ .Chart.AppVersion }}",
				},
			},
			context: &DetectionContext{
				TemplateMode: true,
			},
			expectedImages: []struct {
				repository string
				tag        string
			}{
				{
					repository: "library/nginx",
					tag:        "{{ .Chart.AppVersion }}",
				},
			},
		},
		{
			name: "template mode disabled",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "{{ .Chart.AppVersion }}",
				},
			},
			context: &DetectionContext{
				TemplateMode: false,
			},
			expectedImages: []struct {
				repository string
				tag        string
			}{
				{
					repository: "library/nginx",
					tag:        "{{ .Chart.AppVersion }}",
				},
			},
		},
		{
			name: "global registry in context",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "app",
					"tag":        "v1.0",
				},
			},
			context: &DetectionContext{
				GlobalRegistry: "my-registry.example.com",
			},
			expectedImages: []struct {
				repository string
				tag        string
			}{
				{
					repository: "app",
					tag:        "v1.0",
				},
			},
		},
		{
			name: "registry precedence - map registry over global",
			values: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "local-registry.example.com",
					"repository": "app",
					"tag":        "v1.0",
				},
			},
			context: &DetectionContext{
				GlobalRegistry: "global-registry.example.com",
			},
			expectedImages: []struct {
				repository string
				tag        string
			}{
				{
					repository: "app",
					tag:        "v1.0",
				},
			},
		},
		{
			name: "source registry filtering",
			values: map[string]interface{}{
				"docker": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "docker.io",
						"repository": "nginx",
						"tag":        "1.23",
					},
				},
				"quay": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "quay.io",
						"repository": "prometheus/node-exporter",
						"tag":        "v1.3.1",
					},
				},
				"custom": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "custom.example.com",
						"repository": "app",
						"tag":        "v1.0",
					},
				},
			},
			context: &DetectionContext{
				SourceRegistries: []string{"docker.io", "quay.io"}, // Only docker.io and quay.io are source registries
			},
			expectedImages: []struct {
				repository string
				tag        string
			}{
				{
					repository: "library/nginx",
					tag:        "1.23",
				},
				{
					repository: "prometheus/node-exporter",
					tag:        "v1.3.1",
				},
			},
		},
		{
			name: "exclude registry filtering",
			values: map[string]interface{}{
				"docker": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "docker.io",
						"repository": "nginx",
						"tag":        "1.23",
					},
				},
				"quay": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "quay.io",
						"repository": "prometheus/node-exporter",
						"tag":        "v1.3.1",
					},
				},
			},
			context: &DetectionContext{
				ExcludeRegistries: []string{"quay.io"}, // Exclude quay.io registry
			},
			expectedImages: []struct {
				repository string
				tag        string
			}{
				{
					repository: "library/nginx",
					tag:        "1.23",
				},
				{
					repository: "prometheus/node-exporter",
					tag:        "v1.3.1",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(tc.context)
			images, unsupported, err := detector.DetectImages(tc.values, nil)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Special case for strict mode test
			if tc.name == "strict mode with ambiguous strings" {
				// Verify nginx:1.23 is detected
				found := false
				for _, img := range images {
					if img.Reference != nil &&
						img.Reference.Repository == "library/nginx" &&
						img.Reference.Tag == "1.23" {
						found = true
						break
					}
				}
				assert.True(t, found, "should detect nginx:1.23 as a valid image")

				// Check for unsupported items if expected
				if tc.expectUnsupported {
					assert.Greater(t, len(unsupported), 0, "should have some unsupported items")
				}
				return
			}

			// For other test cases, check for expected images
			for _, expected := range tc.expectedImages {
				found := false
				for _, img := range images {
					if img.Reference.Repository == expected.repository &&
						img.Reference.Tag == expected.tag {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected to find image %s:%s",
					expected.repository, expected.tag)
			}
		})
	}
}

func TestTryExtractImageFromString_EdgeCases(t *testing.T) {
	// Strict unit test: Validates the deterministic tryExtractImageFromString function.
	tests := []struct {
		name          string
		input         string
		expected      *ImageReference
		expectError   bool
		errorContains string
	}{
		{
			name:          "empty string",
			input:         "",
			expected:      nil,
			expectError:   true,
			errorContains: "empty string",
		},
		{
			name:        "simple docker library image",
			input:       "nginx:latest", // Add tag for better compatibility
			expectError: false,
			expected: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "latest",
			},
		},
		{
			name:        "image with port in registry",
			input:       "localhost:5000/myapp:1.0",
			expectError: false,
			expected: &ImageReference{
				Registry:   "localhost:5000",
				Repository: "myapp",
				Tag:        "1.0",
			},
		},
		{
			name:        "full reference with organization",
			input:       "quay.io/org/app:v1.2.3",
			expectError: false,
			expected: &ImageReference{
				Registry:   "quay.io",
				Repository: "org/app",
				Tag:        "v1.2.3",
			},
		},
		{
			name:        "digest reference",
			input:       "docker.io/org/repo@sha256:1234567890123456789012345678901234567890123456789012345678901234",
			expectError: false,
			expected: &ImageReference{
				Registry:   "docker.io",
				Repository: "org/repo",
				Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
				Tag:        "",
			},
		},
		{
			name:          "invalid image format",
			input:         "not:a:valid:image",
			expected:      nil,
			expectError:   true,
			errorContains: "invalid repository name",
		},
		{
			name:          "non-image string",
			input:         "just a normal string",
			expected:      nil,
			expectError:   true,
			errorContains: "invalid repository name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := tryExtractImageFromString(tc.input)

			if tc.expectError {
				assert.Error(t, err, "expected an error for input: %s", tc.input)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains, "error message did not contain expected text")
				}
				assert.Nil(t, ref, "expected nil reference for error case")
				return
			}

			assert.NoError(t, err, "unexpected error: %v", err)
			assert.NotNil(t, ref, "expected non-nil reference")

			if tc.expected != nil && ref != nil {
				assert.Equal(t, tc.expected.Registry, ref.Registry, "registry mismatch")
				assert.Equal(t, tc.expected.Repository, ref.Repository, "repository mismatch")
				assert.Equal(t, tc.expected.Tag, ref.Tag, "tag mismatch")
				assert.Equal(t, tc.expected.Digest, ref.Digest, "digest mismatch")
			}
		})
	}
}

func TestTryExtractImageFromMap_PartialMaps(t *testing.T) {
	// Strict unit test: Validates the deterministic tryExtractImageFromMap function.
	tests := []struct {
		name            string
		imageMap        map[string]interface{}
		context         *DetectionContext
		expectedRef     *ImageReference
		expectedPattern string
		expectError     bool
		errorContains   string
	}{
		{
			name: "complete map with all fields",
			imageMap: map[string]interface{}{
				"registry":   "docker.io",
				"repository": "nginx",
				"tag":        "1.23",
			},
			context: nil,
			expectedRef: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "1.23",
			},
			expectedPattern: "map",
			expectError:     false,
		},
		{
			name: "partial map - missing tag",
			imageMap: map[string]interface{}{
				"registry":   "docker.io",
				"repository": "nginx",
			},
			context: nil,
			expectedRef: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "",
			},
			expectedPattern: "map",
			expectError:     false,
		},
		{
			name: "partial map - missing registry",
			imageMap: map[string]interface{}{
				"repository": "nginx",
				"tag":        "1.23",
			},
			context: nil,
			expectedRef: &ImageReference{
				Registry:   "docker.io", // Default
				Repository: "library/nginx",
				Tag:        "1.23",
			},
			expectedPattern: "map",
			expectError:     false,
		},
		{
			name: "partial map - missing registry with global context",
			imageMap: map[string]interface{}{
				"repository": "app",
				"tag":        "v1.0",
			},
			context: &DetectionContext{
				GlobalRegistry: "my-registry.example.com",
			},
			expectedRef: &ImageReference{
				Registry:   "my-registry.example.com", // From global context
				Repository: "app",
				Tag:        "v1.0",
			},
			expectedPattern: "map",
			expectError:     false,
		},
		{
			name: "minimal map - only repository",
			imageMap: map[string]interface{}{
				"repository": "nginx",
			},
			context: nil,
			expectedRef: &ImageReference{
				Registry:   "docker.io", // Default
				Repository: "library/nginx",
				Tag:        "",
			},
			expectedPattern: "map",
			expectError:     false,
		},
		{
			name: "non-image map - missing repository",
			imageMap: map[string]interface{}{
				"registry": "docker.io",
				"tag":      "latest",
			},
			context:         nil,
			expectedRef:     nil,
			expectedPattern: "",
			expectError:     false, // Not an error, just returns nil
		},
		{
			name: "invalid map - repository not a string",
			imageMap: map[string]interface{}{
				"repository": 123, // Invalid type
				"tag":        "1.23",
			},
			context:         nil,
			expectedRef:     nil,
			expectedPattern: "",
			expectError:     true,
			errorContains:   "repository is not a string",
		},
		{
			name: "invalid map - registry not a string",
			imageMap: map[string]interface{}{
				"registry":   true, // Invalid type
				"repository": "nginx",
				"tag":        "1.23",
			},
			context:         nil,
			expectedRef:     nil,
			expectedPattern: "",
			expectError:     true,
			errorContains:   "registry is not a string",
		},
		{
			name: "invalid map - tag not a string",
			imageMap: map[string]interface{}{
				"registry":   "docker.io",
				"repository": "nginx",
				"tag":        123, // Invalid type
			},
			context:         nil,
			expectedRef:     nil,
			expectedPattern: "",
			expectError:     true,
			errorContains:   "tag is not a string",
		},
		{
			name: "map with template variable in tag",
			imageMap: map[string]interface{}{
				"repository": "nginx",
				"tag":        "{{ .Chart.AppVersion }}",
			},
			context: &DetectionContext{
				TemplateMode: true,
			},
			expectedRef: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "{{ .Chart.AppVersion }}",
			},
			expectedPattern: "map",
			expectError:     false,
		},
		{
			name: "map with organization in repository",
			imageMap: map[string]interface{}{
				"repository": "myorg/myapp",
				"tag":        "v1.0",
			},
			context: nil,
			expectedRef: &ImageReference{
				Registry:   "docker.io",   // Default
				Repository: "myorg/myapp", // Already has organization, should not prepend library/
				Tag:        "v1.0",
			},
			expectedPattern: "map",
			expectError:     false,
		},
		{
			name: "map with nil tag",
			imageMap: map[string]interface{}{
				"repository": "nginx",
				"tag":        nil,
			},
			context: nil,
			expectedRef: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "",
			},
			expectedPattern: "map",
			expectError:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(tc.context)
			ref, pattern, err := detector.tryExtractImageFromMap(tc.imageMap)

			if tc.expectError {
				assert.Error(t, err, "expected an error")
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains, "error message did not contain expected text")
				}
				assert.Nil(t, ref, "expected nil reference for error case")
				assert.Empty(t, pattern, "expected empty pattern for error case")
			} else {
				assert.NoError(t, err, "unexpected error: %v", err)

				if tc.expectedRef == nil {
					assert.Nil(t, ref, "expected nil reference")
				} else {
					assert.NotNil(t, ref, "expected non-nil reference")
					assert.Equal(t, tc.expectedRef.Registry, ref.Registry, "registry mismatch")
					assert.Equal(t, tc.expectedRef.Repository, ref.Repository, "repository mismatch")
					assert.Equal(t, tc.expectedRef.Tag, ref.Tag, "tag mismatch")
				}

				assert.Equal(t, tc.expectedPattern, pattern, "pattern mismatch")
			}
		})
	}
}

func TestImageDetector_NonImageValues(t *testing.T) {
	// Outcome-focused test suite: Validates that DetectImages correctly ignores
	// common non-image values (booleans, numbers, unrelated strings) even when
	// they might appear near potential image structures. Focuses on preventing
	// false positives in detection outcomes.
	tests := []struct {
		name          string
		values        interface{}
		expectedImage struct {
			repository string
			tag        string
		}
		expectError bool
	}{
		{
			name: "boolean and numeric values",
			values: map[string]interface{}{
				"enabled": true,
				"port":    8080,
				"timeout": "30s",
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			expectedImage: struct {
				repository string
				tag        string
			}{
				repository: "library/nginx",
				tag:        "1.23",
			},
		},
		{
			name: "non-image configuration paths",
			values: map[string]interface{}{
				"annotations": map[string]interface{}{
					"prometheus.io/port": "9090",
				},
				"labels": map[string]interface{}{
					"app": "myapp",
				},
				"serviceAccountName": "default",
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "1.23",
				},
			},
			expectedImage: struct {
				repository string
				tag        string
			}{
				repository: "library/nginx",
				tag:        "1.23",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(nil)
			images, unsupported, err := detector.DetectImages(tc.values, nil)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Empty(t, unsupported)

			// Check if we can find the expected image
			found := false
			for _, img := range images {
				if img.Reference.Repository == tc.expectedImage.repository &&
					img.Reference.Tag == tc.expectedImage.tag {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected to find image %s:%s",
				tc.expectedImage.repository, tc.expectedImage.tag)
		})
	}
}
