// Package strategy_test contains tests for the strategy package.
package strategy

import (
	"testing"

	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/stretchr/testify/assert"
)

func TestPrefixSourceRegistryStrategy(t *testing.T) {
	strategy := &PrefixSourceRegistryStrategy{}

	tests := []struct {
		name           string
		input          *image.Reference
		targetRegistry string
		expected       string
	}{
		{
			name: "standard image with registry",
			input: &image.Reference{
				Registry:   "docker.io",
				Repository: "nginx",
				Tag:        "latest",
			},
			targetRegistry: "",
			expected:       "dockerio/library/nginx",
		},
		{
			name: "nested path",
			input: &image.Reference{
				Registry:   "quay.io",
				Repository: "prometheus/node-exporter",
				Tag:        "v1.3.1",
			},
			targetRegistry: "",
			expected:       "quayio/prometheus/node-exporter",
		},
		{
			name: "digest reference",
			input: &image.Reference{
				Registry:   "gcr.io",
				Repository: "google-containers/pause",
				Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
			targetRegistry: "",
			expected:       "gcrio/google-containers/pause",
		},
		{
			name: "registry_with_port",
			input: &image.Reference{
				Registry:   "registry.example.com:5000",
				Repository: "app/frontend",
				Tag:        "v1.2.0",
			},
			targetRegistry: "",
			expected:       "registryexamplecom/app/frontend",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := strategy.GeneratePath(tc.input, tc.targetRegistry)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetStrategy(t *testing.T) {
	tests := []struct {
		name         string
		strategyName string
		mappings     *registry.Mappings
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "valid strategy",
			strategyName: "prefix-source-registry",
			mappings:     nil,
			wantErr:      false,
		},
		{
			name:         "unknown strategy",
			strategyName: "unknown",
			mappings:     nil,
			wantErr:      true,
			errMsg:       "unknown path strategy: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := GetStrategy(tt.strategyName, tt.mappings)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, strategy)
			}
		})
	}
}

func TestPrefixSourceRegistryStrategy_GeneratePath(t *testing.T) {
	tests := []struct {
		name           string
		targetRegistry string
		imgRef         *image.Reference
		want           string
	}{
		{
			name:           "simple_repository",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "quay.io",
				Repository: "org/repo",
				Tag:        "latest",
			},
			want: "quayio/org/repo",
		},
		{
			name:           "repository_with_dots",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "registry.k8s.io",
				Repository: "pause",
				Tag:        "3.9",
			},
			want: "registryk8sio/pause",
		},
		{
			name:           "repository_with_port",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "localhost:5000",
				Repository: "myimage",
				Tag:        "dev",
			},
			want: "localhost/myimage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PrefixSourceRegistryStrategy{}
			got, err := s.GeneratePath(tt.imgRef, tt.targetRegistry)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrefixSourceRegistryStrategy_GeneratePath_WithMappings(t *testing.T) {
	tests := []struct {
		name           string
		targetRegistry string
		imgRef         *image.Reference
		want           string
		mapping        *registry.Mappings
	}{
		{
			name:           "with_custom_mapping",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "quay.io",
				Repository: "jetstack/cert-manager-controller",
				Tag:        "v1.5.3",
			},
			mapping: &registry.Mappings{
				Entries: []registry.Mapping{
					{Source: "quay.io", Target: "custom.registry.local/proxy"},
				},
			},
			want: "quayio/jetstack/cert-manager-controller",
		},
		{
			name:           "without custom mapping",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "docker.io",
				Repository: "library/nginx",
			},
			mapping: nil,
			want:    "dockerio/library/nginx",
		},
		{
			name:           "with digest",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
			mapping: nil,
			want:    "dockerio/library/nginx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PrefixSourceRegistryStrategy{}
			got, err := s.GeneratePath(tt.imgRef, tt.targetRegistry)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
