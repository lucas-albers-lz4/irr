// Package strategy_test contains tests for the strategy package.
package strategy

import (
	"fmt"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrefixSourceRegistryStrategy(t *testing.T) {
	strategy := NewPrefixSourceRegistryStrategy(nil)
	targetRegistry := "ignored-target-registry"

	testCases := []struct {
		name           string
		originalImage  string
		expectedPath   string
		expectedError  bool
		targetRegistry string
	}{
		{
			name:          "standard image with registry",
			originalImage: "docker.io/library/nginx:latest",
			expectedPath:  "docker.io/library/nginx",
		},
		{
			name:          "standard image without registry (defaults to docker hub)",
			originalImage: "nginx:latest",
			expectedPath:  "docker.io/library/nginx",
		},
		{
			name:          "nested path",
			originalImage: "quay.io/prometheus/node-exporter:v1",
			expectedPath:  "quay.io/prometheus/node-exporter",
		},
		{
			name:          "digest reference",
			originalImage: "gcr.io/google-containers/pause@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedPath:  "gcr.io/google-containers/pause",
		},
		{
			name:          "registry with port",
			originalImage: "registry.example.com:5000/app/frontend:stable",
			expectedPath:  "registry.example.com/app/frontend",
		},
		{
			name:          "empty image string",
			originalImage: "",
			expectedError: true,
		},
		{
			name:          "invalid image string (double colon)",
			originalImage: ":::",
			expectedError: true,
		},
		{
			name:           "no target registry provided",
			originalImage:  "docker.io/library/redis:alpine",
			targetRegistry: "",
			expectedPath:   "docker.io/library/redis",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			imgRef, err := image.ParseImageReference(tc.originalImage)
			if err != nil && !tc.expectedError {
				t.Fatalf("Failed to parse test image reference '%s': %v", tc.originalImage, err)
			}

			currentTargetRegistry := targetRegistry
			if tc.targetRegistry != "" || tc.name == "no target registry provided" {
				currentTargetRegistry = tc.targetRegistry
			}

			var generatedPath string
			var genErr error
			if imgRef != nil {
				generatedPath, genErr = strategy.GeneratePath(imgRef, currentTargetRegistry)
			}

			if tc.expectedError {
				assert.True(t, err != nil || genErr != nil, "Expected an error from parsing or generation, but got none (parse err: %v, gen err: %v)", err, genErr)
			} else {
				require.NoError(t, err, "Parsing image reference failed unexpectedly")
				require.NoError(t, genErr, "GeneratePath returned an unexpected error")
				assert.Equal(t, tc.expectedPath, generatedPath)
			}
		})
	}
}

func TestGetStrategy(t *testing.T) {
	testCases := []struct {
		name          string
		strategyName  string
		mappings      *registry.Mappings
		expectedType  interface{}
		expectedError bool
	}{
		{
			name:         "prefix-source-registry",
			strategyName: "prefix-source-registry",
			mappings:     &registry.Mappings{},
			expectedType: &PrefixSourceRegistryStrategy{},
		},
		{
			name:         "flat",
			strategyName: "flat",
			mappings:     nil,
			expectedType: &FlatStrategy{},
		},
		{
			name:          "unknown",
			strategyName:  "unknown",
			mappings:      nil,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			strategy, err := GetStrategy(tc.strategyName, tc.mappings)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, strategy)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, strategy)
				assert.IsType(t, tc.expectedType, strategy)
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
			want: "quay.io-org-repo",
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
			want: "registry.k8s.io-ingress-nginx-controller",
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
			want: "gcr.io-google-containers-kubernetes-dashboard",
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

func TestFlatStrategy(t *testing.T) {
	strategy := NewFlatStrategy()
	targetRegistry := "target.registry"

	testCases := []struct {
		name          string
		originalImage string
		expectedPath  string
		expectedError bool
	}{
		{
			name:          "standard docker hub image",
			originalImage: "nginx:latest",
			expectedPath:  "docker.io-library-nginx",
		},
		{
			name:          "docker hub library image",
			originalImage: "docker.io/library/redis:alpine",
			expectedPath:  "docker.io-library-redis",
		},
		{
			name:          "nested path image",
			originalImage: "quay.io/prometheus/node-exporter:v1",
			expectedPath:  "quay.io-prometheus-node-exporter",
		},
		{
			name:          "registry with port",
			originalImage: "myregistry.com:5000/app/backend:v2",
			expectedPath:  "myregistry.com-app-backend",
		},
		{
			name:          "digest reference",
			originalImage: "gcr.io/distroless/static@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expectedPath:  "gcr.io-distroless-static",
		},
		{
			name:          "empty image string",
			originalImage: "",
			expectedError: true,
		},
		{
			name:          "invalid image string (ambiguous tag/repo)",
			originalImage: "docker.io/repo:tag@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			imgRef, err := image.ParseImageReference(tc.originalImage)
			if err != nil && !tc.expectedError {
				t.Fatalf("Failed to parse test image reference '%s': %v", tc.originalImage, err)
			}

			var generatedPath string
			var genErr error
			if imgRef != nil {
				generatedPath, genErr = strategy.GeneratePath(imgRef, targetRegistry)
			}

			if tc.expectedError {
				assert.True(t, err != nil || genErr != nil, "Expected an error from parsing or generation, but got none (parse err: %v, gen err: %v)", err, genErr)
			} else {
				require.NoError(t, err, "Parsing image reference failed unexpectedly")
				require.NoError(t, genErr, "GeneratePath returned an unexpected error")
				assert.Equal(t, tc.expectedPath, generatedPath)
			}
		})
	}
}
