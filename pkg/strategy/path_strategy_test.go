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
		input          *image.ImageReference
		targetRegistry string
		expected       string
	}{
		{
			name: "standard image with registry",
			input: &image.ImageReference{
				Registry:   "docker.io",
				Repository: "nginx",
				Tag:        "1.23",
			},
			targetRegistry: "",
			expected:       "dockerio/nginx",
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
			name: "registry with port",
			input: &image.ImageReference{
				Registry:   "registry.example.com:5000",
				Repository: "app/frontend",
				Tag:        "v2",
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
	mappings := &registry.RegistryMappings{
		Mappings: []registry.RegistryMapping{
			{
				Source: "quay.io",
				Target: "quay",
			},
		},
	}

	tests := []struct {
		name         string
		strategyName string
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "valid strategy",
			strategyName: "prefix-source-registry",
			wantErr:      false,
		},
		{
			name:         "unknown strategy",
			strategyName: "unknown",
			wantErr:      true,
			errMsg:       "unknown path strategy: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := GetStrategy(tt.strategyName, mappings)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				assert.Nil(t, strategy)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, strategy)
			}
		})
	}
}

func TestPrefixSourceRegistryStrategy_GeneratePath(t *testing.T) {
	mappings := &registry.RegistryMappings{
		Mappings: []registry.RegistryMapping{
			{
				Source: "quay.io",
				Target: "quay",
			},
		},
	}

	tests := []struct {
		name           string
		targetRegistry string
		imgRef         *image.ImageReference
		want           string
	}{
		{
			name:           "simple repository",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "quay.io",
				Repository: "org/repo",
			},
			want: "quay/org/repo",
		},
		{
			name:           "repository with dots",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "docker.io",
				Repository: "org/repo",
			},
			want: "dockerio/org/repo",
		},
		{
			name:           "repository with port",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "localhost:5000",
				Repository: "org/repo",
			},
			want: "localhost/org/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PrefixSourceRegistryStrategy{
				Mappings: mappings,
			}
			got, err := s.GeneratePath(tt.imgRef, tt.targetRegistry, mappings)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrefixSourceRegistryStrategy_GeneratePath_WithMappings(t *testing.T) {
	mappings := &registry.RegistryMappings{
		Mappings: []registry.RegistryMapping{
			{
				Source: "quay.io",
				Target: "quay",
			},
		},
	}

	tests := []struct {
		name           string
		targetRegistry string
		imgRef         *image.ImageReference
		want           string
	}{
		{
			name:           "with custom mapping",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "quay.io",
				Repository: "jetstack/cert-manager-controller",
			},
			want: "quay/jetstack/cert-manager-controller",
		},
		{
			name:           "without custom mapping",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
			},
			want: "dockerio/library/nginx",
		},
		{
			name:           "with digest",
			targetRegistry: "",
			imgRef: &image.ImageReference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Digest:     "sha256:1234567890123456789012345678901234567890123456789012345678901234",
			},
			want: "dockerio/library/nginx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PrefixSourceRegistryStrategy{
				Mappings: mappings,
			}
			got, err := s.GeneratePath(tt.imgRef, tt.targetRegistry, mappings)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
