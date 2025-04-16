package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseImageReferenceEdgeCases tests edge cases for the ParseImageReference function
func TestParseImageReferenceEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		errType  error
		expected *Reference
	}{
		{
			name:    "empty reference",
			input:   "",
			wantErr: true,
			errType: ErrEmptyImageReference,
		},
		{
			name:    "only whitespace",
			input:   "  ",
			wantErr: true,
			errType: ErrInvalidImageReference,
		},
		{
			name:    "invalid characters in repository",
			input:   "registry/repo$name:tag",
			wantErr: true,
			errType: ErrInvalidImageReference,
		},
		{
			name:  "double slash",
			input: "registry//repo:tag",
			expected: &Reference{
				Original:   "registry//repo:tag",
				Registry:   "registry/",
				Repository: "repo:tag",
				Tag:        "latest",
				Detected:   false,
			},
		},
		{
			name:    "double colon",
			input:   "registry/repo::tag",
			wantErr: true,
			errType: ErrInvalidImageReference,
		},
		{
			name:    "double at",
			input:   "registry/repo@@digest",
			wantErr: true,
			errType: ErrInvalidImageReference,
		},
		{
			name:  "localhost registry",
			input: "localhost/repo:tag",
			expected: &Reference{
				Original:   "localhost/repo:tag",
				Registry:   "localhost",
				Repository: "repo",
				Tag:        "tag",
				Detected:   false,
			},
		},
		{
			name:  "IP address as registry",
			input: "127.0.0.1:5000/repo:tag",
			expected: &Reference{
				Original:   "127.0.0.1:5000/repo:tag",
				Registry:   "127.0.0.1",
				Repository: "repo",
				Tag:        "tag",
				Detected:   false,
			},
		},
		{
			name:  "very long but valid repository name",
			input: "docker.io/very-long-namespace/with-multiple-parts/and-more-parts/deep-nesting/component:1.0",
			expected: &Reference{
				Original:   "docker.io/very-long-namespace/with-multiple-parts/and-more-parts/deep-nesting/component:1.0",
				Registry:   "docker.io",
				Repository: "very-long-namespace/with-multiple-parts/and-more-parts/deep-nesting/component",
				Tag:        "1.0",
				Detected:   false,
			},
		},
		{
			name:  "unusual tag characters but valid",
			input: "docker.io/repo:v1.2.3-alpha.1+build.2020.01.01",
			expected: &Reference{
				Original:   "docker.io/repo:v1.2.3-alpha.1+build.2020.01.01",
				Registry:   "docker.io",
				Repository: "repo:v1.2.3-alpha.1+build.2020.01.01",
				Tag:        "latest",
				Detected:   false,
			},
		},
		{
			name:    "both tag and digest",
			input:   "docker.io/repo:tag@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			wantErr: true,
			errType: ErrTagAndDigestPresent,
		},
		{
			name:  "repository with underscore",
			input: "docker.io/my_repo:tag",
			expected: &Reference{
				Original:   "docker.io/my_repo:tag",
				Registry:   "docker.io",
				Repository: "library/my_repo",
				Tag:        "tag",
				Detected:   false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseImageReference(tt.input)

			if tt.wantErr {
				assert.Error(t, err, "Expected error for input: %s", tt.input)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType, "Error should be of expected type")
				}
				assert.Nil(t, ref, "Reference should be nil on error")
			} else {
				require.NoError(t, err, "No error expected for input: %s", tt.input)
				require.NotNil(t, ref, "Reference should not be nil on success")

				assert.Equal(t, tt.expected.Registry, ref.Registry, "Registry mismatch")
				assert.Equal(t, tt.expected.Repository, ref.Repository, "Repository mismatch")
				assert.Equal(t, tt.expected.Tag, ref.Tag, "Tag mismatch")
				assert.Equal(t, tt.expected.Digest, ref.Digest, "Digest mismatch")
				assert.Equal(t, tt.expected.Original, ref.Original, "Original mismatch")
				assert.Equal(t, tt.expected.Detected, ref.Detected, "Detected mismatch")
			}
		})
	}
}

// TestNormalizeImageReference tests edge cases in image reference normalization
func TestNormalizeImageReference(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already normalized reference",
			input:    "docker.io/library/nginx:latest",
			expected: "docker.io/library/nginx:latest",
		},
		{
			name:     "short form without registry",
			input:    "nginx:latest",
			expected: "docker.io/library/nginx:latest",
		},
		{
			name:     "short form without registry and tag",
			input:    "nginx",
			expected: "docker.io/library/nginx:latest",
		},
		{
			name:     "docker registry with implicit library",
			input:    "docker.io/nginx",
			expected: "docker.io/library/nginx:latest",
		},
		{
			name:     "docker registry with explicit library",
			input:    "docker.io/library/nginx",
			expected: "docker.io/library/nginx:latest",
		},
		{
			name:     "docker registry with tag but implicit library",
			input:    "docker.io/nginx:1.19",
			expected: "docker.io/library/nginx:1.19",
		},
		{
			name:     "non-docker registry",
			input:    "quay.io/prometheus/node-exporter:latest",
			expected: "quay.io/prometheus/node-exporter:latest",
		},
		{
			name:     "non-docker registry without tag",
			input:    "quay.io/prometheus/node-exporter",
			expected: "quay.io/prometheus/node-exporter:latest",
		},
		{
			name:     "unusual but valid registry",
			input:    "k8s.gcr.io/kube-apiserver:v1.21.0",
			expected: "k8s.gcr.io/kube-apiserver:v1.21.0",
		},
		{
			name:     "with digest instead of tag",
			input:    "docker.io/library/nginx@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: "docker.io/library/nginx@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the input
			ref, err := ParseImageReference(tt.input)
			require.NoError(t, err, "ParseImageReference should not fail for valid input: %s", tt.input)
			require.NotNil(t, ref, "Reference should not be nil")

			// Check the normalized form
			var normalized string
			if ref.Digest != "" {
				normalized = ref.Registry + "/" + ref.Repository + "@" + ref.Digest
			} else {
				normalized = ref.Registry + "/" + ref.Repository + ":" + ref.Tag
			}

			assert.Equal(t, tt.expected, normalized, "Normalized form should match expected")
		})
	}
}
