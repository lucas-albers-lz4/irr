package image

import (
	"testing"
)

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		name           string
		imageRef       string
		expectedResult *ImageReference
		expectError    bool
	}{
		{
			name:     "standard image with registry",
			imageRef: "docker.io/nginx:1.23",
			expectedResult: &ImageReference{
				Registry:   "docker.io",
				Repository: "nginx",
				Tag:        "1.23",
			},
			expectError: false,
		},
		{
			name:     "image with nested path",
			imageRef: "quay.io/prometheus/node-exporter:v1.3.1",
			expectedResult: &ImageReference{
				Registry:   "quay.io",
				Repository: "prometheus/node-exporter",
				Tag:        "v1.3.1",
			},
			expectError: false,
		},
		{
			name:     "image with implicit docker.io registry",
			imageRef: "nginx:latest",
			expectedResult: &ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "latest",
			},
			expectError: false,
		},
		{
			name:     "image with digest",
			imageRef: "docker.io/nginx@sha256:1234567890123456789012345678901234567890123456789012345678901234",
			expectedResult: &ImageReference{
				Registry:   "docker.io",
				Repository: "nginx",
				Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
			expectError: false,
		},
		{
			name:        "invalid image reference",
			imageRef:    "invalid::image:format",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseImageReference(tc.imageRef)

			// Check for expected error
			if tc.expectError && err == nil {
				t.Errorf("Expected error, but got nil")
				return
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
				return
			}

			// Skip further checks if we expected an error
			if tc.expectError {
				return
			}

			// Check reference fields
			if result.Registry != tc.expectedResult.Registry {
				t.Errorf("Registry mismatch: expected %s, got %s", tc.expectedResult.Registry, result.Registry)
			}
			if result.Repository != tc.expectedResult.Repository {
				t.Errorf("Repository mismatch: expected %s, got %s", tc.expectedResult.Repository, result.Repository)
			}
			if tc.expectedResult.Digest != "" {
				if result.Digest != tc.expectedResult.Digest {
					t.Errorf("Digest mismatch: expected %s, got %s", tc.expectedResult.Digest, result.Digest)
				}
			} else {
				if result.Tag != tc.expectedResult.Tag {
					t.Errorf("Tag mismatch: expected %s, got %s", tc.expectedResult.Tag, result.Tag)
				}
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
