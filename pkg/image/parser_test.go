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
			name:          "invalid image reference",
			input:         "not:a:valid:image",
			expectedRef:   nil,
			expectedErr:   true,
			errorContains: "invalid image reference",
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
