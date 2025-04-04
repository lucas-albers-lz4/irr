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
		for k := 0; k < len(images[i].Path) && k < len(images[j].Path); k++ {
			if images[i].Path[k] != images[j].Path[k] {
				return images[i].Path[k] < images[j].Path[k]
			}
		}
		return len(images[i].Path) < len(images[j].Path)
	})
}

func TestImageDetector(t *testing.T) {
	tests := []struct {
		name           string
		values         interface{}
		context        *DetectionContext
		expectedImages []DetectedImage
		expectError    bool
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
					Path: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					Pattern: "map",
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
					Path: []string{"image"},
					Reference: &ImageReference{
						Registry:   "my-registry.example.com",
						Repository: "app", // No library prefix for non-docker.io registry
						Tag:        "v1.0",
					},
					Pattern: "map",
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
					Path: []string{"spec", "template", "spec", "containers", "[0]", "image"},
					Reference: &ImageReference{
						Registry:   "quay.io",
						Repository: "prometheus/node-exporter",
						Tag:        "v1.3.1",
					},
					Pattern: "string",
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
					Path: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "{{ .Chart.AppVersion }}",
					},
					Pattern: "map",
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
					Path: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					Pattern: "map",
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
					Path: []string{"containers", "[0]", "image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					Pattern: "string",
				},
				{
					Path: []string{"containers", "[1]", "image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/fluentd",
						Tag:        "v1.14",
					},
					Pattern: "string",
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
					Path: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
					},
					Pattern: "string",
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
					Path: []string{"image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					Pattern: "map",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(tc.context)
			images, err := detector.DetectImages(tc.values, nil)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedImages), len(images), "number of detected images mismatch")

			// Sort both expected and actual images for consistent comparison
			sortDetectedImages(tc.expectedImages)
			sortDetectedImages(images)

			for i, expected := range tc.expectedImages {
				if i >= len(images) {
					break
				}
				actual := images[i]

				assert.Equal(t, expected.Path, actual.Path, "path mismatch")
				assert.Equal(t, expected.Pattern, actual.Pattern, "pattern mismatch")

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
	tests := []struct {
		name           string
		values         interface{}
		context        *DetectionContext
		expectedImages []DetectedImage
		expectError    bool
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
					Path: []string{"a", "b", "c", "image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					Pattern: "map",
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
					Path: []string{"valid", "image"},
					Reference: &ImageReference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "1.23",
					},
					Pattern: "map",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(tc.context)
			images, err := detector.DetectImages(tc.values, nil)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			if tc.expectedImages == nil {
				assert.Empty(t, images)
				return
			}

			// Sort both expected and actual images for consistent comparison
			sortDetectedImages(tc.expectedImages)
			sortDetectedImages(images)

			assert.Equal(t, len(tc.expectedImages), len(images))
			for i, expected := range tc.expectedImages {
				actual := images[i]
				assert.Equal(t, expected.Path, actual.Path)
				assert.Equal(t, expected.Pattern, actual.Pattern)
				assert.Equal(t, expected.Reference, actual.Reference)
			}
		})
	}
}

func TestImageDetector_GlobalRegistry(t *testing.T) {
	tests := []struct {
		name           string
		values         interface{}
		context        *DetectionContext
		expectedImages []DetectedImage
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
					Path: []string{"frontend", "image"},
					Reference: &ImageReference{
						Registry:   "my-registry.example.com",
						Repository: "frontend-app",
						Tag:        "v1.0",
					},
					Pattern: "map",
				},
				{
					Path: []string{"backend", "image"},
					Reference: &ImageReference{
						Registry:   "my-registry.example.com",
						Repository: "backend-app",
						Tag:        "v2.0",
					},
					Pattern: "map",
				},
			},
		},
		{
			name: "global registry override",
			values: map[string]interface{}{
				"global": map[string]interface{}{
					"imageRegistry": "global-registry.example.com",
				},
				"image": map[string]interface{}{
					"registry":   "local-registry.example.com",
					"repository": "app",
					"tag":        "v1.0",
				},
			},
			context: nil,
			expectedImages: []DetectedImage{
				{
					Path: []string{"image"},
					Reference: &ImageReference{
						Registry:   "local-registry.example.com", // Local registry should take precedence
						Repository: "app",
						Tag:        "v1.0",
					},
					Pattern: "map",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			detector := NewImageDetector(tc.context)
			images, err := detector.DetectImages(tc.values, nil)

			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedImages), len(images))

			// Sort both expected and actual images for consistent comparison
			sortDetectedImages(tc.expectedImages)
			sortDetectedImages(images)

			for i, expected := range tc.expectedImages {
				actual := images[i]
				assert.Equal(t, expected.Path, actual.Path)
				assert.Equal(t, expected.Pattern, actual.Pattern)
				assert.Equal(t, expected.Reference.Registry, actual.Reference.Registry)
				assert.Equal(t, expected.Reference.Repository, actual.Reference.Repository)
				assert.Equal(t, expected.Reference.Tag, actual.Reference.Tag)
			}
		})
	}
}
