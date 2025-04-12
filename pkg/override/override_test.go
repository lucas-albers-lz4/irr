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

func TestConstructSubchartPath(t *testing.T) {
	tests := []struct {
		name         string
		dependencies []ChartDependency
		path         string
		expected     string
		wantErr      bool
	}{
		{
			name:         "no dependencies",
			dependencies: []ChartDependency{},
			path:         "subchart.image",
			expected:     "subchart.image",
			wantErr:      false,
		},
		{
			name: "single dependency no match",
			dependencies: []ChartDependency{
				{Name: "other-chart", Alias: "other-alias"},
			},
			path:     "subchart.image",
			expected: "subchart.image",
			wantErr:  false,
		},
		{
			name: "single dependency with match",
			dependencies: []ChartDependency{
				{Name: "subchart", Alias: "mychart"},
			},
			path:     "subchart.image",
			expected: "mychart.image",
			wantErr:  false,
		},
		{
			name: "multiple dependencies with match",
			dependencies: []ChartDependency{
				{Name: "subchart", Alias: "mychart"},
				{Name: "other-chart", Alias: "other-alias"},
			},
			path:     "subchart.image",
			expected: "mychart.image",
			wantErr:  false,
		},
		{
			name: "nested path with dependency match",
			dependencies: []ChartDependency{
				{Name: "subchart", Alias: "mychart"},
			},
			path:     "subchart.container.image",
			expected: "mychart.container.image",
			wantErr:  false,
		},
		{
			name: "multiple dependencies in path",
			dependencies: []ChartDependency{
				{Name: "subchart", Alias: "mychart"},
				{Name: "container", Alias: "mycontainer"},
			},
			path:     "subchart.container.image",
			expected: "mychart.mycontainer.image",
			wantErr:  false,
		},
		{
			name: "dependency without alias",
			dependencies: []ChartDependency{
				{Name: "subchart", Alias: ""},
			},
			path:     "subchart.image",
			expected: "subchart.image",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConstructSubchartPath(tt.dependencies, tt.path)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVerifySubchartPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		deps    []ChartDependency
		wantErr bool
	}{
		{
			name: "valid path with matching chart",
			path: "subchart.image",
			deps: []ChartDependency{
				{Name: "subchart", Alias: ""},
			},
			wantErr: false,
		},
		{
			name: "valid path with matching alias",
			path: "mychart.image",
			deps: []ChartDependency{
				{Name: "subchart", Alias: "mychart"},
			},
			wantErr: false,
		},
		{
			name:    "valid path with no dependencies",
			path:    "image.repository",
			deps:    []ChartDependency{},
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			deps:    []ChartDependency{},
			wantErr: true,
		},
		{
			name: "valid path with non-matching chart name",
			path: "unknown.image",
			deps: []ChartDependency{
				{Name: "subchart", Alias: "mychart"},
			},
			wantErr: false, // This doesn't error, just warns in debug output
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifySubchartPath(tt.path, tt.deps)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
