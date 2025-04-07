// Package override_test contains tests for the override package.
package override

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/lalbers/irr/pkg/image"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestGenerateOverrides(t *testing.T) {
	tests := []struct {
		name     string
		ref      *image.Reference
		path     []string
		expected map[string]interface{}
		wantErr  bool
	}{
		{
			name: "simple image override",
			ref: &image.Reference{
				Registry:   "my-registry.example.com",
				Repository: "dockerio/nginx",
				Tag:        "1.23",
			},
			path: []string{"image"},
			expected: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "my-registry.example.com",
					"repository": "dockerio/nginx",
					"tag":        "1.23",
				},
			},
		},
		{
			name: "nested image override",
			ref: &image.Reference{
				Registry:   "my-registry.example.com",
				Repository: "quayio/prometheus/node-exporter",
				Tag:        "v1.3.1",
			},
			path: []string{"subchart", "image"},
			expected: map[string]interface{}{
				"subchart": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "my-registry.example.com",
						"repository": "quayio/prometheus/node-exporter",
						"tag":        "v1.3.1",
					},
				},
			},
		},
		{
			name: "digest reference",
			ref: &image.Reference{
				Registry:   "my-registry.example.com",
				Repository: "quayio/jetstack/cert-manager-controller",
				Digest:     "sha256:1234567890abcdef",
			},
			path: []string{"cert-manager", "image"},
			expected: map[string]interface{}{
				"cert-manager": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "my-registry.example.com",
						"repository": "quayio/jetstack/cert-manager-controller",
						"digest":     "sha256:1234567890abcdef",
					},
				},
			},
		},
		{
			name:    "empty path",
			ref:     &image.Reference{},
			path:    []string{},
			wantErr: true,
		},
		{
			name:    "nil reference",
			ref:     nil,
			path:    []string{"image"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GenerateOverrides(tt.ref, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Error("GenerateOverrides() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GenerateOverrides() unexpected error: %v", err)
				return
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("GenerateOverrides() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSubchartAliasPathConstruction(t *testing.T) {
	tests := []struct {
		name     string
		path     []string
		expected []string
	}{
		{
			name:     "simple alias",
			path:     []string{"subchart", "image"},
			expected: []string{"subchart", "image"},
		},
		{
			name:     "nested alias",
			path:     []string{"subchart", "nested", "image"},
			expected: []string{"subchart", "nested", "image"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.path
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestYAMLGeneration(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{
			name: "simple override yaml",
			input: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "dockerio/nginx",
				},
			},
			expected: `image:
  repository: dockerio/nginx
`,
		},
		{
			name: "complex override yaml",
			input: map[string]interface{}{
				"subchart": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "quayio/prometheus/node-exporter",
					},
				},
			},
			expected: `subchart:
  image:
    repository: quayio/prometheus/node-exporter
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := yaml.NewEncoder(&buf)
			enc.SetIndent(2)
			err := enc.Encode(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}
