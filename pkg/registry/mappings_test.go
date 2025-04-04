package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadMappings(t *testing.T) {
	// Create a temporary mappings file
	content := `mappings:
  - source: "quay.io"
    target: "my-registry.example.com/quay-mirror"
  - source: "docker.io"
    target: "my-registry.example.com/docker-mirror"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "mappings.yaml")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	tests := []struct {
		name          string
		path          string
		wantMappings  *RegistryMappings
		wantErr       bool
		errorContains string
	}{
		{
			name: "valid mappings file",
			path: tmpFile,
			wantMappings: &RegistryMappings{
				Mappings: []RegistryMapping{
					{Source: "quay.io", Target: "my-registry.example.com/quay-mirror"},
					{Source: "docker.io", Target: "my-registry.example.com/docker-mirror"},
				},
			},
		},
		{
			name:          "nonexistent file",
			path:          "nonexistent.yaml",
			wantErr:       true,
			errorContains: "failed to read mappings file",
		},
		{
			name: "empty path",
			path: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadMappings(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				return
			}
			require.NoError(t, err)
			if tt.path == "" {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tt.wantMappings, got)
			}
		})
	}
}

func TestGetTargetRegistry(t *testing.T) {
	mappings := &RegistryMappings{
		Mappings: []RegistryMapping{
			{Source: "quay.io", Target: "my-registry.example.com/quay-mirror"},
			{Source: "docker.io", Target: "my-registry.example.com/docker-mirror"},
		},
	}

	tests := []struct {
		name     string
		mappings *RegistryMappings
		source   string
		want     string
	}{
		{
			name:     "existing mapping",
			mappings: mappings,
			source:   "quay.io",
			want:     "my-registry.example.com/quay-mirror",
		},
		{
			name:     "non-existent mapping",
			mappings: mappings,
			source:   "gcr.io",
			want:     "",
		},
		{
			name:     "nil mappings",
			mappings: nil,
			source:   "quay.io",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.mappings.GetTargetRegistry(tt.source)
			assert.Equal(t, tt.want, got)
		})
	}
}
