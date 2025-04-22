// Package registry_test contains tests for the registry package.
package registry

import (
	"path/filepath"
	"testing"

	"errors"

	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadMappings tests the LoadMappings function with different inputs
func TestLoadMappings(t *testing.T) {
	// Create a memory-backed filesystem for testing
	fs := afero.NewMemMapFs()
	tmpDir := TestTmpDir

	// Create test file paths
	newFormatFile := filepath.Join(tmpDir, "new-format.yaml")
	emptyTmpFile := filepath.Join(tmpDir, "empty.yaml")
	invalidTmpFile := filepath.Join(tmpDir, "invalid.yaml")
	invalidExtTmpFile := filepath.Join(tmpDir, "mappings.txt")
	tmpSubDir := filepath.Join(tmpDir, "subdir")
	tmpDirFile := filepath.Join(tmpSubDir, "mappings.yaml")

	// Create test content
	newFormatContent := `mappings:
  - source: quay.io
    target: my-registry.example.com/quay-mirror
  - source: docker.io
    target: my-registry.example.com/docker-mirror`

	invalidYAMLContent := `mappings: [invalid yaml`

	// Set up the memory filesystem
	require.NoError(t, fs.MkdirAll(tmpDir, fileutil.ReadWriteExecuteUserReadExecuteOthers))
	require.NoError(t, fs.MkdirAll(tmpSubDir, fileutil.ReadWriteExecuteUserReadExecuteOthers))

	// Write test files
	require.NoError(t, afero.WriteFile(fs, newFormatFile, []byte(newFormatContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, emptyTmpFile, []byte(""), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, invalidTmpFile, []byte(invalidYAMLContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, invalidExtTmpFile, []byte(""), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, tmpDirFile, []byte(""), fileutil.ReadWriteUserReadOthers))

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
			name:          "valid_mappings_file_(new_format)",
			path:          newFormatFile,
			wantMappings:  expectedMappings,
			wantErr:       false,
			errorContains: "",
		},
		{
			name:          "nonexistent_file",
			path:          "/path/does/not/exist.yaml",
			wantMappings:  nil,
			wantErr:       true,
			errorContains: "mappings file does not exist",
		},
		{
			name:          "empty_file",
			path:          emptyTmpFile,
			wantErr:       true,
			errorContains: "mappings file is empty",
		},
		{
			name:          "invalid_yaml_format",
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
			name:          "empty path",
			path:          "",
			wantErr:       true,
			errorContains: "mappings file path is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment for path traversal testing
			// REMOVED: Environment variable was not used by LoadMappings
			// if tt.name == "invalid path traversal" {
			// 	t.Setenv("IRR_ALLOW_PATH_TRAVERSAL", "false")
			// } else {
			// 	t.Setenv("IRR_ALLOW_PATH_TRAVERSAL", "true")
			// }

			// Determine if CWD restriction should be skipped for this test case.
			// Only the 'invalid path traversal' test needs the check active (skip=false).
			skipCheck := tt.name != "invalid path traversal"

			// Call the consolidated LoadMappings function with the test filesystem
			got, err := LoadMappings(fs, tt.path, skipCheck)
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

// TestNonexistentFileMappingError tests that the correct error type is returned for nonexistent files
func TestNonexistentFileMappingError(t *testing.T) {
	// Create a memory-backed filesystem for testing
	fs := afero.NewMemMapFs()

	// Try to load a nonexistent file
	nonexistentPath := "/path/does/not/exist.yaml"
	_, err := LoadMappings(fs, nonexistentPath, true)

	// Check that the error is of the correct type
	require.Error(t, err)
	var mappingErr *ErrMappingFileNotExist
	ok := errors.As(err, &mappingErr)
	require.True(t, ok, "Error should be of type ErrMappingFileNotExist, got %T", err)

	// Check the error message
	assert.Contains(t, err.Error(), "mappings file does not exist")

	// Check that we can unwrap the original error
	unwrappedErr := errors.Unwrap(err)
	require.NotNil(t, unwrappedErr, "Unwrapped error should not be nil")
}
