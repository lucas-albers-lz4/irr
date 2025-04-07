package strategy

import (
	"testing"

	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registrymapping"
	"github.com/stretchr/testify/assert"
)

func TestPrefixSourceRegistryStrategy(t *testing.T) {
	strategy := &PrefixSourceRegistryStrategy{}

	tests := []struct {
		name           string
		input          *image.ImageReference
		targetRegistry string
		expected       string
	}{
		{
			name: "standard image with registry",
			input: &image.ImageReference{
				Registry:   "docker.io",
				Repository: "nginx",
				Tag:        "latest",
			},
			targetRegistry: "",
			expected:       "dockerio/library/nginx",
		},
		{
			name: "nested path",
			input: &image.ImageReference{
				Registry:   "quay.io",
				Repository: "prometheus/node-exporter",
				Tag:        "v1.3.1",
			},
			targetRegistry: "",
			expected:       "quayio/prometheus/node-exporter",
		},
		{
			name: "digest reference",
			input: &image.ImageReference{
				Registry:   "gcr.io",
				Repository: "google-containers/pause",
				Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
			targetRegistry: "",
			expected:       "gcrio/google-containers/pause",
		},
		{
			name: "registry_with_port",
			input: &image.ImageReference{
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
			result, err := strategy.GeneratePath(tc.input, tc.targetRegistry, nil)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetStrategy(t *testing.T) {
	tests := []struct {
		name         string
		strategyName string
		mappings     *registrymapping.RegistryMappings
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
		imgRef         *image.ImageReference
		want           string
	}{
		{
			name:           "simple_repository",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "quay.io",
				Repository: "org/repo",
				Tag:        "latest",
			},
			want: "quayio/org/repo",
		},
		{
			name:           "repository_with_dots",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "registry.k8s.io",
				Repository: "pause",
				Tag:        "3.9",
			},
			want: "registryk8sio/pause",
		},
		{
			name:           "repository_with_port",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "localhost:5000",
				Repository: "myimage",
				Tag:        "dev",
			},
			want: "localhost/myimage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PrefixSourceRegistryStrategy{Mappings: nil}
			got, err := s.GeneratePath(tt.imgRef, tt.targetRegistry, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrefixSourceRegistryStrategy_GeneratePath_WithMappings(t *testing.T) {
	tests := []struct {
		name           string
		targetRegistry string
		imgRef         *image.ImageReference
		want           string
		mapping        *registrymapping.RegistryMappings
	}{
		{
			name:           "with_custom_mapping",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "quay.io",
				Repository: "jetstack/cert-manager-controller",
				Tag:        "v1.5.3",
			},
			mapping: &registrymapping.RegistryMappings{
				Mappings: []registrymapping.RegistryMapping{
					{Source: "quay.io", Target: "custom.registry.local/proxy"},
				},
			},
			want: "customregistrylocal/proxy/jetstack/cert-manager-controller",
		},
		{
			name:           "without custom mapping",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
			},
			mapping: nil,
			want:    "dockerio/library/nginx",
		},
		{
			name:           "with digest",
			targetRegistry: "",
			imgRef: &image.ImageReference{
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
			s := &PrefixSourceRegistryStrategy{
				Mappings: tt.mapping,
			}
			got, err := s.GeneratePath(tt.imgRef, tt.targetRegistry, tt.mapping)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
