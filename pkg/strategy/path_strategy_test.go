// Package strategy_test contains tests for the strategy package.
package strategy

import (
	"fmt"
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

// TestPrefixSourceRegistryStrategy_GeneratePath_InputVariations tests the GeneratePath method
// with various image reference inputs, focusing on how the source registry and repository
// are combined and sanitized into the resulting path part.
// Note: This test does not involve registry mappings, as GeneratePath itself doesn't handle them.
func TestPrefixSourceRegistryStrategy_GeneratePath_InputVariations(t *testing.T) {
	tests := []struct {
		name           string
		targetRegistry string // Kept for signature, but not used by this strategy's path logic
		imgRef         *image.Reference
		want           string
		// mapping field removed as it's not used by GeneratePath
	}{
		{
			name:           "quay_repo",
			targetRegistry: "", // Example target registry (unused in path calculation)
			imgRef: &image.Reference{
				Registry:   "quay.io",
				Repository: "jetstack/cert-manager-controller",
				Tag:        "v1.5.3",
			},
			want: "quayio/jetstack/cert-manager-controller", // Sanitized source registry prefix
		},
		{
			name:           "dockerhub_library_repo",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "latest",
			},
			want: "dockerio/library/nginx",
		},
		{
			name:           "dockerhub_official_repo_no_library_prefix",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "docker.io",
				Repository: "nginx", // Implicitly library/nginx
				Tag:        "stable",
			},
			want: "dockerio/library/nginx", // Expect library/ to be added
		},
		{
			name:           "repo_with_digest",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Digest:     "sha256:abcdef123456",
			},
			want: "dockerio/library/nginx",
		},
		{
			name:           "registry_with_port_and_dots",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "private.registry.example.com:5000",
				Repository: "my-app/backend",
				Tag:        "1.2.3",
			},
			want: "privateregistryexamplecom/my-app/backend", // Port removed, dots removed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PrefixSourceRegistryStrategy{}
			// Pass targetRegistry, even though it's not used by this strategy for path generation
			got, err := s.GeneratePath(tt.imgRef, tt.targetRegistry)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFlatStrategy_GeneratePath tests the GeneratePath method of the FlatStrategy
func TestFlatStrategy_GeneratePath(t *testing.T) {
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
			want: "quayio-org-repo",
		},
		{
			name:           "nested_path",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "docker.io",
				Repository: "library/nginx/stable",
				Tag:        "1.21",
			},
			want: "dockerio-library-nginx-stable",
		},
		{
			name:           "docker_hub_official_image",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "docker.io",
				Repository: "nginx",
				Tag:        "latest",
			},
			want: "dockerio-library-nginx",
		},
		{
			name:           "repository_with_dots",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "registry.k8s.io",
				Repository: "ingress-nginx/controller",
				Tag:        "v1.2.0",
			},
			want: "registryk8sio-ingress-nginx-controller",
		},
		{
			name:           "repository_with_port",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "localhost:5000",
				Repository: "my/local/image",
				Tag:        "dev",
			},
			want: "localhost-my-local-image",
		},
		{
			name:           "deeply_nested_path",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "gcr.io",
				Repository: "google-containers/kubernetes/dashboard",
				Tag:        "v2.0.0",
			},
			want: "gcrio-google-containers-kubernetes-dashboard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &FlatStrategy{}
			got, err := s.GeneratePath(tt.imgRef, tt.targetRegistry)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetStrategy_WithFlatStrategy(t *testing.T) {
	tests := []struct {
		name         string
		strategyName string
		mappings     *registry.Mappings
		wantErr      bool
		strategyType string
	}{
		{
			name:         "prefix-source-registry strategy",
			strategyName: "prefix-source-registry",
			mappings:     nil,
			wantErr:      false,
			strategyType: "*strategy.PrefixSourceRegistryStrategy",
		},
		{
			name:         "flat strategy",
			strategyName: "flat",
			mappings:     nil,
			wantErr:      false,
			strategyType: "*strategy.FlatStrategy",
		},
		{
			name:         "unknown strategy",
			strategyName: "unknown",
			mappings:     nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := GetStrategy(tt.strategyName, tt.mappings)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, strategy)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, strategy)
				assert.Equal(t, tt.strategyType, fmt.Sprintf("%T", strategy))
			}
		})
	}
}
