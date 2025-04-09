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
	// Create a temporary mappings file
	// **NOTE:** Updated fixture to match map[string]string format expected by LoadMappings.
	content := `
 "quay.io": "my-registry.example.com/quay-mirror"
 "docker.io": "my-registry.example.com/docker-mirror"
 `
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "mappings.yaml")
	err := os.WriteFile(tmpFile, []byte(content), 0o600)
	require.NoError(t, err)

	// Create an empty temporary file
	emptyTmpFile := filepath.Join(tmpDir, "empty.yaml")
	// #nosec G304 // Testing file operations with temporary files is safe
	emptyFile, err := os.Create(emptyTmpFile)
	require.NoError(t, err)
	if err := emptyFile.Close(); err != nil { // Close immediately after creation
		t.Logf("Warning: failed to close empty temp file: %v", err)
	}

	// Create a temporary file with invalid YAML
	invalidYAMLContent := `"key: value" : missing_quote`
	invalidTmpFile := filepath.Join(tmpDir, "invalid.yaml")
	err = os.WriteFile(invalidTmpFile, []byte(invalidYAMLContent), 0o600)
	require.NoError(t, err)

	// Create a temporary file with an invalid extension
	invalidExtTmpFile := filepath.Join(tmpDir, "mappings.txt")
	// #nosec G304 // Testing file operations with temporary files is safe
	invalidExtFile, err := os.Create(invalidExtTmpFile)
	require.NoError(t, err)
	if err := invalidExtFile.Close(); err != nil {
		t.Logf("Warning: failed to close invalid extension temp file: %v", err)
	}

	// Create a temporary directory
	tmpSubDir := filepath.Join(tmpDir, "subdir")
	err = os.MkdirAll(tmpSubDir, 0o750)
	require.NoError(t, err)

	tests := []struct {
		name          string
		path          string
		wantMappings  *Mappings // Use consolidated Mappings type
		wantErr       bool
		errorContains string
	}{
		{
			name: "valid mappings file",
			path: tmpFile,
			wantMappings: &Mappings{ // Use consolidated Mappings type
				Entries: []Mapping{ // Use .Entries
					{Source: "quay.io", Target: "my-registry.example.com/quay-mirror"},
					{Source: "docker.io", Target: "my-registry.example.com/docker-mirror"},
				},
			},
			wantErr:       false, // Should succeed now
			errorContains: "",
		},
		{
			name:          "nonexistent file",
			path:          "nonexistent.yaml",
			wantErr:       true,
			errorContains: "mappings file does not exist", // Check specific error text from WrapMappingFileNotExist
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
			errorContains: "is a directory",
		},
		{
			name:          "invalid path traversal",
			path:          "../../../etc/passwd", // Example invalid path
			wantErr:       true,
			errorContains: "path must be within the current working directory tree",
		},
		{
			name: "empty path",
			path: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env variable to skip CWD check in LoadMappings during testing
			t.Setenv("IRR_TESTING", "true")

			// Call the consolidated LoadMappings function
			got, err := LoadMappings(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				return
			}
			require.NoError(t, err)
			if tt.path == "" {
				assert.Nil(t, got) // Correct expectation for empty path
			} else {
				// Use ElementsMatch because the order from map iteration is not guaranteed
				assert.ElementsMatch(t, tt.wantMappings.Entries, got.Entries) // Use .Entries
			}
		})
	}
}

// Test function names updated to match consolidated functions
func TestGetTargetRegistry(t *testing.T) {
	mappings := &Mappings{ // Use consolidated Mappings type
		Entries: []Mapping{ // Use .Entries
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
