// Package registry_test contains tests for the registry package.
package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test function names updated to match consolidated functions
func TestLoadMappings(t *testing.T) {
	// Create a temporary mappings file with new format
	newFormatContent := `mappings:
  - source: quay.io
    target: my-registry.example.com/quay-mirror
  - source: docker.io
    target: my-registry.example.com/docker-mirror`
	tmpDir := t.TempDir()
	newFormatFile := filepath.Join(tmpDir, "new-format.yaml")
	err := os.WriteFile(newFormatFile, []byte(newFormatContent), 0o600)
	require.NoError(t, err)

	// Create a temporary file with old format
	oldFormatContent := `quay.io: my-registry.example.com/quay-mirror
docker.io: my-registry.example.com/docker-mirror`
	oldFormatFile := filepath.Join(tmpDir, "old-format.yaml")
	err = os.WriteFile(oldFormatFile, []byte(oldFormatContent), 0o600)
	require.NoError(t, err)

	// Create an empty temporary file
	emptyTmpFile := filepath.Join(tmpDir, "empty.yaml")
	emptyFile, err := os.Create(emptyTmpFile)
	require.NoError(t, err)
	if err := emptyFile.Close(); err != nil {
		t.Logf("Warning: failed to close empty temp file: %v", err)
	}

	// Create a temporary file with invalid YAML
	invalidYAMLContent := `mappings: [invalid yaml`
	invalidTmpFile := filepath.Join(tmpDir, "invalid.yaml")
	err = os.WriteFile(invalidTmpFile, []byte(invalidYAMLContent), 0o600)
	require.NoError(t, err)

	// Create a temporary file with an invalid extension
	invalidExtTmpFile := filepath.Join(tmpDir, "mappings.txt")
	invalidExtFile, err := os.Create(invalidExtTmpFile)
	require.NoError(t, err)
	if err := invalidExtFile.Close(); err != nil {
		t.Logf("Warning: failed to close invalid extension temp file: %v", err)
	}

	// Create a temporary directory
	tmpSubDir := filepath.Join(tmpDir, "subdir")
	err = os.MkdirAll(tmpSubDir, 0o750)
	require.NoError(t, err)

	// Create a temporary file in the directory
	tmpDirFile := filepath.Join(tmpSubDir, "mappings.yaml")
	err = os.WriteFile(tmpDirFile, []byte(""), 0o600)
	require.NoError(t, err)

	expectedMappings := &Mappings{
		Entries: []Mapping{
			{Source: "quay.io", Target: "my-registry.example.com/quay-mirror"},
			{Source: "docker.io", Target: "my-registry.example.com/docker-mirror"},
		},
	}

	tests := []struct {
		name          string
		path          string
		wantMappings  *Mappings
		wantErr       bool
		errorContains string
	}{
		{
			name:          "valid mappings file (new format)",
			path:          newFormatFile,
			wantMappings:  expectedMappings,
			wantErr:       false,
			errorContains: "",
		},
		{
			name:          "valid mappings file (old format)",
			path:          oldFormatFile,
			wantMappings:  expectedMappings,
			wantErr:       false,
			errorContains: "",
		},
		{
			name:          "nonexistent file",
			path:          "nonexistent.yaml",
			wantErr:       true,
			errorContains: "mappings file does not exist",
		},
		{
			name:          "empty file",
			path:          emptyTmpFile,
			wantErr:       true,
			errorContains: "mappings file is empty",
		},
		{
			name:          "invalid yaml format",
			path:          invalidTmpFile,
			wantErr:       true,
			errorContains: "failed to parse mappings file",
		},
		{
			name:          "invalid file extension",
			path:          invalidExtTmpFile,
			wantErr:       true,
			errorContains: "mappings file path must end with .yaml or .yml",
		},
		{
			name:          "path is a directory",
			path:          tmpSubDir,
			wantErr:       true,
			errorContains: "failed to read mappings file",
		},
		{
			name:          "invalid path traversal",
			path:          "../../../etc/passwd.yaml",
			wantErr:       true,
			errorContains: "mappings file path '../../../etc/passwd.yaml' must be within the current working directory tree",
		},
		{
			name: "empty path",
			path: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment for path traversal testing
			if tt.name == "invalid path traversal" {
				t.Setenv("IRR_ALLOW_PATH_TRAVERSAL", "false")
			} else {
				t.Setenv("IRR_ALLOW_PATH_TRAVERSAL", "true")
			}

			// Call the consolidated LoadMappings function
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
				assert.ElementsMatch(t, tt.wantMappings.Entries, got.Entries)
			}
		})
	}
}

// Test function names updated to match consolidated functions
func TestGetTargetRegistry(t *testing.T) {
	mappings := &Mappings{ // Use consolidated Mappings type
		Entries: []Mapping{
			{Source: "quay.io", Target: "my-registry.example.com/quay-mirror"},
			{Source: "docker.io", Target: "my-registry.example.com/docker-mirror"},
		},
	}

	tests := []struct {
		name     string
		mappings *Mappings // Use consolidated Mappings type
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
			name:     "existing mapping - normalization docker.io",
			mappings: mappings,                          // docker.io maps to my-registry.example.com/docker-mirror
			source:   "index.docker.io/library/myimage", // Should normalize to docker.io
			want:     "my-registry.example.com/docker-mirror",
		},
		{
			name:     "existing mapping - normalization with CR",
			mappings: mappings,    // quay.io maps to my-registry.example.com/quay-mirror
			source:   "quay.io\r", // Carriage return should be stripped
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
		{
			name:     "empty mappings list",
			mappings: &Mappings{Entries: []Mapping{}}, // Use .Entries
			source:   "quay.io",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the consolidated GetTargetRegistry method
			got := tt.mappings.GetTargetRegistry(tt.source)
			assert.Equal(t, tt.want, got)
		})
	}
}
