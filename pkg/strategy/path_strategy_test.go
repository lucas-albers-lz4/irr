// Package strategy_test contains tests for the strategy package.
package strategy

import (
	"fmt"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/stretchr/testify/assert"
)

func TestPrefixSourceRegistryStrategy(t *testing.T) {
	tests := []struct {
		name          string
		inputRef      *image.Reference
		expectedPath  string // Expect base path WITHOUT prefix
		expectedError bool
	}{
		{
			name: "standard image with registry",
			inputRef: &image.Reference{
				Original:   "docker.io/library/nginx:latest",
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "latest",
			},
			expectedPath:  "library/nginx", // Corrected: Base path only
			expectedError: false,
		},
		{
			name: "nested path",
			inputRef: &image.Reference{
				Original:   "quay.io/prometheus/node-exporter:v1.3.1",
				Registry:   "quay.io",
				Repository: "prometheus/node-exporter",
				Tag:        "v1.3.1",
			},
			expectedPath:  "prometheus/node-exporter", // Corrected: Base path only
			expectedError: false,
		},
		{
			name: "digest reference",
			inputRef: &image.Reference{
				Original:   "gcr.io/google-containers/pause@sha256:12345",
				Registry:   "gcr.io",
				Repository: "google-containers/pause",
				Digest:     "sha256:12345",
			},
			expectedPath:  "google-containers/pause", // Corrected: Base path only
			expectedError: false,
		},
		{
			name: "registry with port",
			inputRef: &image.Reference{
				Original:   "registry.example.com:5000/app/frontend:stable",
				Registry:   "registry.example.com:5000",
				Repository: "app/frontend",
				Tag:        "stable",
			},
			expectedPath:  "app/frontend", // Corrected: Base path only
			expectedError: false,
		},
	}

	strategy := NewPrefixSourceRegistryStrategy()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualPath, err := strategy.GeneratePath(tt.inputRef, "ignored-target-registry") // Target registry doesn't affect this strategy's output
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, actualPath)
			}
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
				Repository: "library/nginx",
				Tag:        "stable",
			},
			want: "docker.io-library-nginx",
		},
		{
			name:           "docker_hub_official_image",
			targetRegistry: "",
			imgRef: &image.Reference{
				Registry:   "docker.io",
				Repository: "nginx",
				Tag:        "latest",
			},
			want: "docker.io-library-nginx",
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
