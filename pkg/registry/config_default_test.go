package registry

import (
	"path/filepath"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// testConfigDir is the directory used for test configuration files
	testConfigDir = "/test"

	// testDirPerm is the permission used for test directories
	testDirPerm = 0o755

	// testFilePerm is the permission used for test files
	testFilePerm = 0o644
)

// TestLoadConfigWithFSDirect tests the LoadConfigWithFS function directly
func TestLoadConfigWithFSDirect(t *testing.T) {
	// Save the original DefaultFS
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	fs := fileutil.NewAferoFS(memFs)

	// Create test file path
	configFile := filepath.Join(testConfigDir, "config-withfs.yaml")

	// Create test content for structured config
	content := `registries:
  mappings:
    - source: docker.io
      target: registry.example.com/docker-mirror
    - source: quay.io
      target: registry.example.com/quay-mirror
  defaultTarget: registry.example.com/default
`

	// Set up the filesystem
	require.NoError(t, memFs.MkdirAll(testConfigDir, testDirPerm))
	require.NoError(t, afero.WriteFile(memFs, configFile, []byte(content), testFilePerm))

	// Test LoadConfigWithFS function
	config, err := LoadConfigWithFS(fs, configFile, true)

	// Check the results
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Len(t, config.Registries.Mappings, 2)

	// Find the docker.io mapping
	var dockerMapping, quayMapping *RegMapping
	for i := range config.Registries.Mappings {
		mapping := &config.Registries.Mappings[i]
		switch mapping.Source {
		case "docker.io":
			dockerMapping = mapping
		case "quay.io":
			quayMapping = mapping
		}
	}

	require.NotNil(t, dockerMapping, "docker.io mapping not found")
	require.NotNil(t, quayMapping, "quay.io mapping not found")
	assert.Equal(t, "registry.example.com/docker-mirror", dockerMapping.Target)
	assert.Equal(t, "registry.example.com/quay-mirror", quayMapping.Target)
}

// TestLoadConfigDefault tests the LoadConfigDefault function directly
func TestLoadConfigDefault(t *testing.T) {
	// Save the original DefaultFS
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	fs := fileutil.NewAferoFS(memFs)
	DefaultFS = fs // Set our mock FS as the default

	// Create test file path
	configFile := filepath.Join(testConfigDir, "config-default-direct.yaml")

	// Create test content
	content := `registries:
  mappings:
    - source: gcr.io
      target: registry.example.com/gcr-mirror
    - source: ghcr.io
      target: registry.example.com/github-mirror
  defaultTarget: registry.example.com/default
`

	// Set up the filesystem
	require.NoError(t, memFs.MkdirAll(testConfigDir, testDirPerm))
	require.NoError(t, afero.WriteFile(memFs, configFile, []byte(content), testFilePerm))

	// Test LoadConfigDefault function
	config, err := LoadConfigDefault(configFile, true)

	// Check the results
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Len(t, config.Registries.Mappings, 2)

	// Find the gcr.io and ghcr.io mappings
	var gcrMapping, ghcrMapping *RegMapping
	for i := range config.Registries.Mappings {
		mapping := &config.Registries.Mappings[i]
		switch mapping.Source {
		case "gcr.io":
			gcrMapping = mapping
		case "ghcr.io":
			ghcrMapping = mapping
		}
	}

	require.NotNil(t, gcrMapping, "gcr.io mapping not found")
	require.NotNil(t, ghcrMapping, "ghcr.io mapping not found")
	assert.Equal(t, "registry.example.com/gcr-mirror", gcrMapping.Target)
	assert.Equal(t, "registry.example.com/github-mirror", ghcrMapping.Target)
}

// TestLoadConfigDefaultError tests error handling in LoadConfigDefault
func TestLoadConfigDefaultError(t *testing.T) {
	// Save the original DefaultFS
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	fs := fileutil.NewAferoFS(memFs)
	DefaultFS = fs // Set our mock FS as the default

	// Test with non-existent file
	config, err := LoadConfigDefault("nonexistent.yaml", true)
	assert.Error(t, err)
	assert.Nil(t, config)
}

// TestLoadStructuredConfigDefaultDirect tests the LoadStructuredConfigDefault function directly
func TestLoadStructuredConfigDefaultDirect(t *testing.T) {
	// Save the original DefaultFS
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	fs := fileutil.NewAferoFS(memFs)
	DefaultFS = fs // Set our mock FS as the default

	// Create test file path
	configFile := filepath.Join(testConfigDir, "structured-default-direct.yaml")

	// Create test content
	content := `registries:
  mappings:
    - source: ecr.amazonaws.com
      target: registry.example.com/aws-mirror
    - source: registry.k8s.io
      target: registry.example.com/k8s-mirror
  defaultTarget: registry.example.com/fallback
  strictMode: true
version: "1"
`

	// Set up the filesystem
	require.NoError(t, memFs.MkdirAll(testConfigDir, testDirPerm))
	require.NoError(t, afero.WriteFile(memFs, configFile, []byte(content), testFilePerm))

	// Test LoadStructuredConfigDefault function
	config, err := LoadStructuredConfigDefault(configFile, true)

	// Check the results
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "1", config.Version)
	assert.Len(t, config.Registries.Mappings, 2)
	assert.Equal(t, "ecr.amazonaws.com", config.Registries.Mappings[0].Source)
	assert.Equal(t, "registry.example.com/aws-mirror", config.Registries.Mappings[0].Target)
	assert.Equal(t, "registry.k8s.io", config.Registries.Mappings[1].Source)
	assert.Equal(t, "registry.example.com/k8s-mirror", config.Registries.Mappings[1].Target)
	assert.Equal(t, "registry.example.com/fallback", config.Registries.DefaultTarget)
	assert.True(t, config.Registries.StrictMode)
}

// TestLoadConfigDefaultViaStructured tests the LoadConfigDefault function indirectly
// by testing its underlying implementation (LoadStructuredConfig + conversion)
func TestLoadConfigDefaultViaStructured(t *testing.T) {
	// Save the original DefaultFS
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	fs := fileutil.NewAferoFS(memFs)
	DefaultFS = fs // Set our mock FS as the default

	// Create test file path
	configFile := filepath.Join(testConfigDir, "config-default.yaml")

	// Create test content
	content := `registries:
  mappings:
    - source: gcr.io
      target: registry.example.com/gcr-mirror
    - source: ghcr.io
      target: registry.example.com/github-mirror
  defaultTarget: registry.example.com/default
`

	// Set up the filesystem
	require.NoError(t, memFs.MkdirAll(testConfigDir, testDirPerm))
	require.NoError(t, afero.WriteFile(memFs, configFile, []byte(content), testFilePerm))

	// Test LoadStructuredConfig directly
	config, err := LoadStructuredConfig(memFs, configFile, true)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Check the results
	require.NotNil(t, config)
	assert.Len(t, config.Registries.Mappings, 2)

	// Find the gcr.io and ghcr.io mappings
	var gcrMapping, ghcrMapping *RegMapping
	for i := range config.Registries.Mappings {
		mapping := &config.Registries.Mappings[i]
		switch mapping.Source {
		case "gcr.io":
			gcrMapping = mapping
		case "ghcr.io":
			ghcrMapping = mapping
		}
	}

	require.NotNil(t, gcrMapping, "gcr.io mapping not found")
	require.NotNil(t, ghcrMapping, "ghcr.io mapping not found")
	assert.Equal(t, "registry.example.com/gcr-mirror", gcrMapping.Target)
	assert.Equal(t, "registry.example.com/github-mirror", ghcrMapping.Target)
}

// TestLoadStructuredConfigDefaultViaUnderlying tests the LoadStructuredConfigDefault function
// indirectly by testing its underlying implementation (LoadStructuredConfig)
func TestLoadStructuredConfigDefaultViaUnderlying(t *testing.T) {
	// Save the original DefaultFS
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Create a memory-backed filesystem for testing
	memFs := afero.NewMemMapFs()
	fs := fileutil.NewAferoFS(memFs)
	DefaultFS = fs // Set our mock FS as the default

	// Create test file path
	configFile := filepath.Join(testConfigDir, "structured-default.yaml")

	// Create test content
	content := `registries:
  mappings:
    - source: ecr.amazonaws.com
      target: registry.example.com/aws-mirror
    - source: registry.k8s.io
      target: registry.example.com/k8s-mirror
  defaultTarget: registry.example.com/fallback
  strictMode: true
version: "1"
`

	// Set up the filesystem
	require.NoError(t, memFs.MkdirAll(testConfigDir, testDirPerm))
	require.NoError(t, afero.WriteFile(memFs, configFile, []byte(content), testFilePerm))

	// Test LoadStructuredConfig directly
	config, err := LoadStructuredConfig(memFs, configFile, true)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Check the results
	assert.Equal(t, "1", config.Version)
	assert.Len(t, config.Registries.Mappings, 2)
	assert.Equal(t, "ecr.amazonaws.com", config.Registries.Mappings[0].Source)
	assert.Equal(t, "registry.example.com/aws-mirror", config.Registries.Mappings[0].Target)
	assert.Equal(t, "registry.k8s.io", config.Registries.Mappings[1].Source)
	assert.Equal(t, "registry.example.com/k8s-mirror", config.Registries.Mappings[1].Target)
	assert.Equal(t, "registry.example.com/fallback", config.Registries.DefaultTarget)
	assert.True(t, config.Registries.StrictMode)
}
