package image

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expected      *Reference
		wantErr       bool
		errorContains string
	}{
		{
			name:  "standard image with registry",
			input: "docker.io/library/nginx:1.21.0",
			expected: &Reference{
				Original:   "docker.io/library/nginx:1.21.0",
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "1.21.0",
			},
		},
		{
			name:  "image with nested path",
			input: "quay.io/org/app/component:v1",
			expected: &Reference{
				Original:   "quay.io/org/app/component:v1",
				Registry:   "quay.io",
				Repository: "org/app/component",
				Tag:        "v1",
			},
		},
		{
			name:  "image with implicit docker.io registry",
			input: "nginx:1.21.0",
			expected: &Reference{
				Original:   "nginx:1.21.0",
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "1.21.0",
			},
		},
		{
			name:  "image with digest",
			input: "gcr.io/project/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expected: &Reference{
				Original:   "gcr.io/project/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Registry:   "gcr.io",
				Repository: "project/image",
				Digest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
		{
			name:  "image with port in registry",
			input: "localhost:5000/myimage:latest",
			expected: &Reference{
				Original:   "localhost:5000/myimage:latest",
				Registry:   "localhost:5000",
				Repository: "myimage",
				Tag:        "latest",
			},
		},
		{
			name:          "image_with_both_tag_and_digest",
			input:         "myrepo/myimage:tag@sha256:f6e1a063d1f00c0b9a9e7f1f9a5c4d0d9e6b8b4b3a1e9d5b3b4b3b3b3b3b3b3b",
			wantErr:       true,
			errorContains: "image cannot have both tag and digest specified",
			expected:      nil,
		},
		{
			name:          "invalid image reference",
			input:         "invalid///image::ref",
			wantErr:       true,
			errorContains: "invalid image reference format",
		},
		{
			name:          "empty string",
			input:         "",
			wantErr:       true,
			errorContains: "image reference string cannot be empty",
		},
		{
			name:  "standard image with registry, tag, and nested path",
			input: "docker.io/library/nested/nginx:1.21.0",
			expected: &Reference{
				Original:   "docker.io/library/nested/nginx:1.21.0",
				Registry:   "docker.io",
				Repository: "library/nested/nginx",
				Tag:        "1.21.0",
			},
		},
		{
			name:          "invalid digest format",
			input:         "gcr.io/project/image@invalid-digest",
			wantErr:       true,
			errorContains: "invalid digest format",
		},
		{
			name:          "invalid tag format",
			input:         "gcr.io/project/image:invalid/tag",
			wantErr:       true,
			errorContains: "invalid tag format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseImageReference(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "error message should contain expected text")
				}
				assert.Nil(t, ref, "Reference should be nil on error")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ref, "Reference should not be nil on success")
				if tt.expected != nil {
					if ref != nil {
						ref.Path = nil
					}
					assert.True(t, reflect.DeepEqual(tt.expected, ref), "Mismatch between expected and actual ImageReference.\nExpected: %+v\nActual:   %+v", tt.expected, ref)
				}
			}
		})
	}
}

func TestIsSourceRegistry(t *testing.T) {
	testRef := &Reference{
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
