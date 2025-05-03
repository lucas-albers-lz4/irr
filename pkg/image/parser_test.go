// Package image_test contains tests for the image package, focusing on reference parsing.
package image_test

import (
	"testing"

	"github.com/distribution/reference"
	image "github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validateWithDistributionLib uses the distribution/reference library to validate
// the parsed reference against the canonical implementation
func validateWithDistributionLib(t *testing.T, ref *image.Reference) {
	t.Helper()
	require.NotNil(t, ref, "Reference should not be nil")

	// Reconstruct the canonical reference string
	var refStr string
	if ref.Digest != "" {
		refStr = ref.Registry + "/" + ref.Repository + "@" + ref.Digest
	} else {
		refStr = ref.Registry + "/" + ref.Repository + ":" + ref.Tag
	}

	// Parse with the distribution library for validation
	parsed, err := reference.ParseNormalizedNamed(refStr)
	require.NoError(t, err, "Reconstructed reference should be valid")

	// Verify components match
	assert.Equal(t, reference.Domain(parsed), ref.Registry, "Registry should match")
	assert.Equal(t, reference.Path(parsed), ref.Repository, "Repository should match")

	if tagged, ok := parsed.(reference.Tagged); ok {
		assert.Equal(t, tagged.Tag(), ref.Tag, "Tag should match")
	}

	if digested, ok := parsed.(reference.Digested); ok {
		assert.Equal(t, digested.Digest().String(), ref.Digest, "Digest should match")
	}
}

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expected      *image.Reference
		wantErr       bool
		errorContains string
	}{
		{
			name:  "standard image with registry",
			input: "docker.io/library/nginx:1.21.0",
			expected: &image.Reference{
				Original:   "docker.io/library/nginx:1.21.0",
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "1.21.0",
				Detected:   true,
			},
		},
		{
			name:  "image with nested path",
			input: "quay.io/org/app/component:v1",
			expected: &image.Reference{
				Original:   "quay.io/org/app/component:v1",
				Registry:   "quay.io",
				Repository: "org/app/component",
				Tag:        "v1",
				Detected:   true,
			},
		},
		{
			name:  "image with implicit docker.io registry",
			input: "nginx:1.21.0",
			expected: &image.Reference{
				Original:   "nginx:1.21.0",
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "1.21.0",
				Detected:   true,
			},
		},
		{
			name:  "image with digest",
			input: "gcr.io/project/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expected: &image.Reference{
				Original:   "gcr.io/project/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Registry:   "gcr.io",
				Repository: "project/image",
				Digest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Detected:   true,
			},
		},
		{
			name:  "image with port in registry",
			input: "localhost:5000/myimage:latest",
			expected: &image.Reference{
				Original:   "localhost:5000/myimage:latest",
				Registry:   "localhost",
				Repository: "myimage",
				Tag:        "latest",
				Detected:   true,
			},
		},
		{
			name:          "image_with_both_tag_and_digest",
			input:         "myrepo/myimage:tag@sha256:f6e1a063d1f00c0b9a9e7f1f9a5c4d0d9e6b8b4b3a1e9d5b3b4b3b3b3b3b3b3b",
			wantErr:       true,
			errorContains: image.ErrTagAndDigestPresent.Error(),
			expected:      nil,
		},
		{
			name:          "invalid image reference",
			input:         "invalid///image::ref",
			wantErr:       true,
			errorContains: image.ErrInvalidImageReference.Error(),
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
			expected: &image.Reference{
				Original:   "docker.io/library/nested/nginx:1.21.0",
				Registry:   "docker.io",
				Repository: "library/nested/nginx",
				Tag:        "1.21.0",
				Detected:   true,
			},
		},
		{
			name:          "invalid digest format",
			input:         "gcr.io/project/image@invalid-digest",
			wantErr:       true,
			errorContains: image.ErrInvalidImageReference.Error(),
		},
		{
			name:          "invalid tag format",
			input:         "gcr.io/project/image:invalid/tag",
			wantErr:       true,
			errorContains: image.ErrInvalidImageReference.Error(),
		},
		{
			name:          "invalid repository name",
			input:         "docker.io/Inv@lid Repo/image:tag",
			wantErr:       true,
			errorContains: image.ErrInvalidImageReference.Error(),
		},
		{
			name:  "repository only (implicit latest)",
			input: "busybox",
			expected: &image.Reference{
				Original:   "busybox",
				Registry:   "docker.io",
				Repository: "library/busybox",
				Tag:        "latest",
				Detected:   true,
			},
		},
		{
			name:  "registry and repository only (implicit latest)",
			input: "quay.io/prometheus/node-exporter",
			expected: &image.Reference{
				Original:   "quay.io/prometheus/node-exporter",
				Registry:   "quay.io",
				Repository: "prometheus/node-exporter",
				Tag:        "latest",
				Detected:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := image.ParseImageReference(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "error message should contain expected text")
				}
				assert.Nil(t, ref, "Reference should be nil on error")
			} else {
				require.NoError(t, err)
				require.NotNil(t, ref, "Reference should not be nil on success")

				// Check fields
				if tt.expected != nil {
					assert.Equal(t, tt.expected.Registry, ref.Registry, "Registry mismatch")
					assert.Equal(t, tt.expected.Repository, ref.Repository, "Repository mismatch")
					assert.Equal(t, tt.expected.Tag, ref.Tag, "Tag mismatch")
					assert.Equal(t, tt.expected.Digest, ref.Digest, "Digest mismatch")
					assert.Equal(t, tt.expected.Original, ref.Original, "Original mismatch")
				}

				// Also validate with the distribution/reference library
				validateWithDistributionLib(t, ref)
			}
		})
	}
}

func TestIsSourceRegistry(t *testing.T) {
	testRef := &image.Reference{
		Registry:   "docker.io",
		Repository: "nginx",
		Tag:        "latest",
	}

	sourceRegistries := []string{"docker.io", "quay.io", "gcr.io"}
	excludeRegistries := []string{"internal.registry.example.com"}

	// Should be included
	if !image.IsSourceRegistry(testRef, sourceRegistries, excludeRegistries) {
		t.Errorf("IsSourceRegistry should return true for docker.io when it's in the source list")
	}

	// Change to non-source registry
	testRef.Registry = "k8s.gcr.io"
	if image.IsSourceRegistry(testRef, sourceRegistries, excludeRegistries) {
		t.Errorf("IsSourceRegistry should return false for k8s.gcr.io when it's not in the source list")
	}

	// Change to excluded registry
	testRef.Registry = "internal.registry.example.com"
	if image.IsSourceRegistry(testRef, sourceRegistries, excludeRegistries) {
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
		result := image.NormalizeRegistry(tc.registry)
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
		{"docker.io", "docker.io"},
		{"quay.io", "quay.io"},
		{"k8s.gcr.io", "k8s.gcr.io"},
		{"registry:5000", "registry"},
		{"internal-registry.example.com:5000", "internal-registry.example.com"},
	}

	for _, tc := range tests {
		result := image.SanitizeRegistryForPath(tc.registry)
		if result != tc.expected {
			t.Errorf("SanitizeRegistryForPath(%s): expected %s, got %s", tc.registry, tc.expected, result)
		}
	}
}
