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
			input:         "not:a:valid:image",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "invalid image reference",
		},
		{
			name:          "empty string",
			input:         "",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "empty image reference",
		},
		{
			name:          "invalid digest format",
			input:         "docker.io/library/nginx@notadigest",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "invalid digest format",
		},
		{
			name:          "invalid tag format",
			input:         "docker.io/library/nginx:invalid/tag",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "invalid tag format",
		},
		{
			name:          "invalid registry name",
			input:         "invalid.registry.with.too.many.parts/app:latest",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "invalid registry name",
		},
		{
			name:          "non-string input",
			input:         123,
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "input must be a string",
		},
		{
			name:          "nil input",
			input:         nil,
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "input must be a string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := ParseImageReference(tc.input)

			if tc.expectedErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				return
			}

			assert.NoError(t, err)
			if tc.expectedRef != nil {
				assert.Equal(t, tc.expectedRef.Registry, ref.Registry, "Registry mismatch")
				assert.Equal(t, tc.expectedRef.Repository, ref.Repository, "Repository mismatch")
				assert.Equal(t, tc.expectedRef.Tag, ref.Tag, "Tag mismatch")
				assert.Equal(t, tc.expectedRef.Digest, ref.Digest, "Digest mismatch")
			} else {
				assert.Nil(t, ref)
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
		name        string
		input       map[string]interface{}
		expected    *ImageReference
		expectedErr bool
	}{
		{
			name: "repository with tag",
			input: map[string]interface{}{
				"repository": "nginx",
				"tag":        "1.21.0",
			},
			expected: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "1.21.0",
			},
		},
		{
			name: "repository with registry and tag",
			input: map[string]interface{}{
				"repository": "quay.io/company/app",
				"tag":        "v2.3.4",
			},
			expected: &ImageReference{
				Registry:   "quay.io",
				Repository: "company/app",
				Tag:        "v2.3.4",
			},
		},
		{
			name: "repository with digest",
			input: map[string]interface{}{
				"repository": "nginx",
				"digest":     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
			expected: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
		},
		{
			name: "repository without tag or digest",
			input: map[string]interface{}{
				"repository": "busybox",
			},
			expected: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/busybox",
			},
		},
		{
			name: "repository with non-string tag",
			input: map[string]interface{}{
				"repository": "app",
				"tag":        123, // Non-string tag
			},
			expected: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/app",
				// Tag should not be set since it's not a string
			},
		},
		{
			name: "missing repository",
			input: map[string]interface{}{
				"tag": "latest",
			},
			expected: nil, // Should return nil when repository is missing
		},
		{
			name: "non-string repository",
			input: map[string]interface{}{
				"repository": 123, // Non-string repository
				"tag":        "latest",
			},
			expected: nil, // Should return nil for non-string repository
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := parseImageMap(tc.input)

			if tc.expectedErr {
				assert.Error(t, err)
				return
			}

			if tc.expected == nil {
				assert.Nil(t, ref)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expected.Registry, ref.Registry, "Registry mismatch")
			assert.Equal(t, tc.expected.Repository, ref.Repository, "Repository mismatch")
			assert.Equal(t, tc.expected.Tag, ref.Tag, "Tag mismatch")
			assert.Equal(t, tc.expected.Digest, ref.Digest, "Digest mismatch")
		})
	}
}
