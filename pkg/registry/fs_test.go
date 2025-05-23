package registry

import (
	"path/filepath"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Constants for registry sources used in tests
const (
	quaySource   = "quay.io"
	dockerSource = "docker.io"
)

func TestSetFS(t *testing.T) {
	// Save original FS
	originalFS := DefaultFS

	// Create a mock filesystem
	mockFS := fileutil.NewAferoFS(afero.NewMemMapFs())

	// Set the mock filesystem
	cleanup := SetFS(mockFS)

	// Verify the FS was changed
	assert.NotEqual(t, originalFS, DefaultFS)

	// Run cleanup function
	cleanup()

	// Verify the original FS was restored
	assert.Equal(t, originalFS, DefaultFS)
}

func TestLoadMappingsWithFS(t *testing.T) {
	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	mockFS := fileutil.NewAferoFS(memFs)
	tmpDir := TestTmpDir

	// Create test file path
	mappingsFile := filepath.Join(tmpDir, "mappings.yaml")

	// Create test content
	content := `
registries:
  mappings:
    - source: quay.io
      target: registry.example.com/quay-mirror
    - source: docker.io
      target: registry.example.com/docker-mirror
`

	// Set up the mock filesystem
	require.NoError(t, memFs.MkdirAll(tmpDir, fileutil.ReadWriteExecuteUserReadExecuteOthers))
	require.NoError(t, afero.WriteFile(memFs, mappingsFile, []byte(content), fileutil.ReadWriteUserPermission))

	// Now our GetAferoFS works correctly with the mock filesystem
	// So LoadMappingsWithFS should succeed
	mappings, err := LoadMappingsWithFS(mockFS, mappingsFile, true)
	require.NoError(t, err)
	require.NotNil(t, mappings)
	assert.Len(t, mappings.Entries, 2)
	// Use ElementsMatch as map iteration order isn't guaranteed
	expectedEntries := []Mapping{
		{Source: quaySource, Target: "registry.example.com/quay-mirror"},
		{Source: dockerSource, Target: "registry.example.com/docker-mirror"},
	}
	assert.ElementsMatch(t, expectedEntries, mappings.Entries)

	// Also test the original LoadMappings to ensure compatibility
	mappingsViaOriginal, err := LoadMappings(memFs, mappingsFile, true)
	require.NoError(t, err)
	require.NotNil(t, mappingsViaOriginal)
	assert.Len(t, mappingsViaOriginal.Entries, 2)
}

func TestLoadConfigWithFS(t *testing.T) {
	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	mockFS := fileutil.NewAferoFS(memFs)
	tmpDir := TestTmpDir

	// Create test file path
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Create test content
	content := `registries:
  mappings:
    - source: quay.io
      target: registry.example.com/quay-mirror
    - source: docker.io
      target: registry.example.com/docker-mirror
  defaultTarget: registry.example.com/default
  strictMode: false
`

	// Set up the mock filesystem
	require.NoError(t, memFs.MkdirAll(tmpDir, fileutil.ReadWriteExecuteUserReadExecuteOthers))
	require.NoError(t, afero.WriteFile(memFs, configFile, []byte(content), fileutil.ReadWriteUserReadOthers))

	// Now our GetAferoFS works correctly with the mock filesystem
	// So LoadStructuredConfigWithFS should succeed
	config, err := LoadStructuredConfigWithFS(mockFS, configFile, true)
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Len(t, config.Registries.Mappings, 2)
	assert.Equal(t, "registry.example.com/default", config.Registries.DefaultTarget)

	// Also test the original LoadStructuredConfig to ensure compatibility
	configViaOriginal, err := LoadStructuredConfig(memFs, configFile, true)
	require.NoError(t, err)
	require.NotNil(t, configViaOriginal)
	assert.Len(t, configViaOriginal.Registries.Mappings, 2)

	// Test that we can find both mappings
	var quayMapping, dockerMapping *RegMapping
	for i := range config.Registries.Mappings {
		mapping := &config.Registries.Mappings[i]
		switch mapping.Source {
		case quaySource:
			quayMapping = mapping
		case dockerSource:
			dockerMapping = mapping
		}
	}
	require.NotNil(t, quayMapping, quaySource+" mapping not found")
	require.NotNil(t, dockerMapping, dockerSource+" mapping not found")
	assert.Equal(t, "registry.example.com/quay-mirror", quayMapping.Target)
	assert.Equal(t, "registry.example.com/docker-mirror", dockerMapping.Target)
}

func TestGetAferoFS(t *testing.T) {
	// Test with nil input
	fs := GetAferoFS(nil)
	assert.NotNil(t, fs, "Should return a non-nil filesystem even with nil input")

	// Test with actual fileutil.FS instance
	mockFS := fileutil.NewAferoFS(afero.NewMemMapFs())
	fs = GetAferoFS(mockFS)
	assert.NotNil(t, fs, "Should return a non-nil filesystem with fileutil.FS input")

	// Basic functionality test - memory filesystem should work fine for this
	err := fs.MkdirAll("/test", fileutil.ReadWriteExecuteUserReadExecuteOthers)
	assert.NoError(t, err, "Should be able to create directory")
}
