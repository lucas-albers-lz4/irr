package registry

import (
	"path/filepath"
	"testing"

	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// testTmpDir is the temporary directory used for testing
	testTmpDir = "/test"

	// testDirPerms is the directory permissions used for tests
	testDirPerms = 0o755

	// testFilePerms is the file permissions used for tests
	testFilePerms = 0o644

	// expectedMappingsEntries is the expected number of mappings entries
	expectedMappingsEntries = 3
)

// TestLoadMappingsDefault tests the LoadMappingsDefault function for loading registry mappings
func TestLoadMappingsDefault(t *testing.T) {
	// Save the original DefaultFS
	origFS := DefaultFS
	defer func() { DefaultFS = origFS }()

	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	fs := fileutil.NewAferoFS(memFs)
	DefaultFS = fs // Set our mock FS as the default

	// Create test file path
	mappingsFile := filepath.Join(testTmpDir, "mappings-default.yaml")

	// Create test content and setup test filesystem
	content := createTestMappingsContent()
	setupTestFilesystem(t, memFs, mappingsFile, content)

	// Call LoadMappingsDefault with the test file
	mappings, err := LoadMappingsDefault(mappingsFile, true)

	// Check the results
	if assert.NoError(t, err) {
		verifyMappingsContent(t, mappings)
	}

	// Test with a non-existent file
	nonExistentFile := filepath.Join(testTmpDir, "nonexistent.yaml")
	mappings, err = LoadMappingsDefault(nonExistentFile, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mappings file does not exist")
	assert.Nil(t, mappings)

	// Test with an invalid file
	invalidFile := filepath.Join(testTmpDir, "invalid.yaml")
	require.NoError(t, afero.WriteFile(memFs, invalidFile, []byte("invalid content"), testFilePerms))
	_, err = LoadMappingsDefault(invalidFile, true)
	assert.Error(t, err)
}

// TestLoadMappingsWithFSWrapper tests the LoadMappingsWithFS wrapper function that uses the filesystem abstraction
func TestLoadMappingsWithFSWrapper(t *testing.T) {
	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	fs := fileutil.NewAferoFS(memFs)

	// Save the original DefaultFS and GetAferoFS function so we can restore it later
	origFS := DefaultFS
	DefaultFS = fs // Set our mock FS as the default

	// Restore the original DefaultFS
	defer func() { DefaultFS = origFS }()

	// Create test file path
	mappingsFile := filepath.Join(testTmpDir, "mappings-withfs.yaml")

	// Create test content and setup test filesystem
	content := createTestMappingsContent()
	setupTestFilesystem(t, memFs, mappingsFile, content)

	// Call LoadMappingsWithFS with the test file
	mappings, err := LoadMappingsWithFS(fs, mappingsFile, true)

	// Check the results
	if assert.NoError(t, err) {
		verifyMappingsContent(t, mappings)
	}

	// Test with a non-existent file
	nonExistentFile := filepath.Join(testTmpDir, "nonexistent.yaml")
	mappings, err = LoadMappingsWithFS(fs, nonExistentFile, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mappings file does not exist")
	assert.Nil(t, mappings)
}

// Helper functions

// createTestMappingsContent creates the test content for mappings files
func createTestMappingsContent() string {
	return `mappings:
  - source: docker.io
    target: registry.example.com/docker
  - source: quay.io
    target: registry.example.com/quay
  - source: gcr.io
    target: registry.example.com/gcr
`
}

// setupTestFilesystem sets up the test filesystem with the provided content
func setupTestFilesystem(t *testing.T, memFs afero.Fs, filePath, content string) {
	// Create parent directory if needed
	dir := filepath.Dir(filePath)
	require.NoError(t, memFs.MkdirAll(dir, testDirPerms))

	// Write test file
	require.NoError(t, afero.WriteFile(memFs, filePath, []byte(content), testFilePerms))
}

// verifyMappingsContent verifies the content of mappings against expected values
func verifyMappingsContent(t *testing.T, mappings *Mappings) {
	require.NotNil(t, mappings)
	assert.Len(t, mappings.Entries, expectedMappingsEntries)
	assert.Equal(t, "docker.io", mappings.Entries[0].Source)
	assert.Equal(t, "registry.example.com/docker", mappings.Entries[0].Target)
	assert.Equal(t, "quay.io", mappings.Entries[1].Source)
	assert.Equal(t, "registry.example.com/quay", mappings.Entries[1].Target)
	assert.Equal(t, "gcr.io", mappings.Entries[2].Source)
	assert.Equal(t, "registry.example.com/gcr", mappings.Entries[2].Target)
}
