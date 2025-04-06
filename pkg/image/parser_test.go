package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		name          string
		input         interface{}
		expectedRef   *ImageReference
		expectedErr   bool
		errorContains string
	}{
		{
			name:  "standard image with registry",
			input: "quay.io/nginx:1.20.0",
			expectedRef: &ImageReference{
				Registry:   "quay.io",
				Repository: "nginx",
				Tag:        "1.20.0",
			},
		},
		{
			name:  "image with nested path",
			input: "docker.io/org/suborg/app:v1.2.3",
			expectedRef: &ImageReference{
				Registry:   "docker.io",
				Repository: "org/suborg/app",
				Tag:        "v1.2.3",
			},
		},
		{
			name:  "image with implicit docker.io registry",
			input: "nginx:latest",
			expectedRef: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "latest",
			},
		},
		{
			name:  "image with digest",
			input: "alpine@sha256:1234567890123456789012345678901234567890123456789012345678901234",
			expectedRef: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/alpine",
				Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
		},
		{
			name:  "image with port in registry",
			input: "localhost:5000/app:latest",
			expectedRef: &ImageReference{
				Registry:   "localhost:5000",
				Repository: "app",
				Tag:        "latest",
			},
		},
		{
			name:  "image with both tag and digest",
			input: "docker.io/library/nginx:1.21.0@sha256:1234567890123456789012345678901234567890123456789012345678901234",
			expectedRef: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "1.21.0",
				Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
		},
		{
			name:          "invalid image reference",
			input:         "invalid:image:format::",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "invalid tag format",
		},
		{
			name:          "empty string",
			input:         "",
			expectedRef:   nil,
			expectedErr:   false,
			errorContains: "",
		},
		{
			name:          "standard image with registry, tag, and nested path",
			input:         "my-registry.com:5000/org/nested/path/image:v1.2.3",
			expectedRef:   &ImageReference{Registry: "my-registry.com:5000", Repository: "org/nested/path/image", Tag: "v1.2.3"},
			expectedErr:   false,
			errorContains: "",
		},
		{
			name:          "invalid digest format",
			input:         "image@sha256:invalid",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "invalid digest format",
		},
		{
			name:          "invalid tag format",
			input:         "image:!invalidTag",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "invalid tag format",
		},
		{
			name:          "invalid registry name",
			input:         "InvalidRegistry/image:tag",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "invalid image reference format",
		},
		{
			name:          "non-string input",
			input:         123,
			expectedRef:   nil,
			expectedErr:   false,
			errorContains: "",
		},
		{
			name:          "nil input",
			input:         nil,
			expectedRef:   nil,
			expectedErr:   false,
			errorContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseImageReference(tt.input)

			if tt.expectedErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			if !tt.expectedErr {
				assert.Equal(t, tt.expectedRef, ref)
			}
		})
	}
}

func TestIsSourceRegistry(t *testing.T) {
	testRef := &ImageReference{
		Registry:   "docker.io",
		Repository: "nginx",
		Tag:        "latest",
	}

	sourceRegistries := []string{"docker.io", "quay.io", "gcr.io"}
	excludeRegistries := []string{"internal.registry.example.com"}

	// Should be included
	if !IsSourceRegistry(testRef, sourceRegistries, excludeRegistries) {
		t.Errorf("IsSourceRegistry should return true for docker.io when it's in the source list")
	}

	// Change to non-source registry
	testRef.Registry = "k8s.gcr.io"
	if IsSourceRegistry(testRef, sourceRegistries, excludeRegistries) {
		t.Errorf("IsSourceRegistry should return false for k8s.gcr.io when it's not in the source list")
	}

	// Change to excluded registry
	testRef.Registry = "internal.registry.example.com"
	if IsSourceRegistry(testRef, sourceRegistries, excludeRegistries) {
		t.Errorf("IsSourceRegistry should return false for excluded registry")
	}
}

func TestNormalizeRegistry(t *testing.T) {
	tests := []struct {
		registry string
		expected string
	}{
		{"docker.io", "docker.io"},
		{"index.docker.io", "docker.io"},
		{"DOCKER.IO", "docker.io"},
		{"quay.io", "quay.io"},
		{"k8s.gcr.io", "k8s.gcr.io"},
		{"registry:5000", "registry"},
		{"internal-registry.example.com:5000", "internal-registry.example.com"},
		{"registry.example.com/", "registry.example.com"},
		{"REGISTRY.EXAMPLE.COM", "registry.example.com"},
	}

	for _, tc := range tests {
		result := NormalizeRegistry(tc.registry)
		if result != tc.expected {
			t.Errorf("NormalizeRegistry(%s): expected %s, got %s", tc.registry, tc.expected, result)
		}
	}
}

func TestSanitizeRegistryForPath(t *testing.T) {
	tests := []struct {
		registry string
		expected string
	}{
		{"docker.io", "dockerio"},
		{"quay.io", "quayio"},
		{"k8s.gcr.io", "k8sgcrio"},
		{"registry:5000", "registry"},
		{"internal-registry.example.com:5000", "internal-registryexamplecom"},
	}

	for _, tc := range tests {
		result := SanitizeRegistryForPath(tc.registry)
		if result != tc.expected {
			t.Errorf("SanitizeRegistryForPath(%s): expected %s, got %s", tc.registry, tc.expected, result)
		}
	}
}

func TestParseImageMap(t *testing.T) {
	tests := []struct {
		name          string
		input         map[string]interface{}
		expectedRef   *ImageReference
		expectedErr   bool
		errorContains string
	}{
		{
			name:        "repository_with_tag",
			input:       map[string]interface{}{"repository": "library/nginx", "tag": "1.21.0"},
			expectedRef: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Tag: "1.21.0"},
			expectedErr: false,
		},
		{
			name:        "repository_with_registry_and_tag",
			input:       map[string]interface{}{"registry": "quay.io", "repository": "company/app", "tag": "v2.3.4"},
			expectedRef: &ImageReference{Registry: "quay.io", Repository: "company/app", Tag: "v2.3.4"},
			expectedErr: false,
		},
		{
			name:        "repository_with_digest",
			input:       map[string]interface{}{"repository": "nginx", "digest": "sha256:1234567890123456789012345678901234567890123456789012345678901234"},
			expectedRef: &ImageReference{Registry: "docker.io", Repository: "library/nginx", Digest: "sha256:1234567890123456789012345678901234567890123456789012345678901234"},
			expectedErr: false,
		},
		{
			name:        "repository_without_tag_or_digest",
			input:       map[string]interface{}{"repository": "image"},
			expectedRef: &ImageReference{Registry: "docker.io", Repository: "library/image"},
			expectedErr: false,
		},
		{
			name:          "repository_with_non-string_tag",
			input:         map[string]interface{}{"repository": "image", "tag": 123},
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: ErrInvalidImageMapTagType.Error(),
		},
		{
			name:          "missing_repository",
			input:         map[string]interface{}{"tag": "latest"},
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: ErrInvalidImageMapRepo.Error(),
		},
		{
			name:          "non-string_repository",
			input:         map[string]interface{}{"repository": false, "tag": "latest"},
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: ErrInvalidImageMapRepo.Error(),
		},
		{
			name:          "non-string_registry",
			input:         map[string]interface{}{"registry": 123, "repository": "image", "tag": "latest"},
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: ErrInvalidImageMapRegistryType.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseImageMap(tt.input, "")

			if tt.expectedErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedRef, ref)
		})
	}
}
