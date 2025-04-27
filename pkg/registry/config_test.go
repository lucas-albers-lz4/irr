package registry

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// TestTmpDir is the temporary directory used for testing
	TestTmpDir = "/tmp"
)

// TestLoadConfig tests the LoadConfig function with different inputs
func TestLoadConfig(t *testing.T) {
	// Create a memory-backed filesystem for testing
	fs := afero.NewMemMapFs()
	tmpDir := TestTmpDir

	// Create test files and directory structure
	testFiles, expectedConfig := setupConfigTestFiles(t, fs, tmpDir)

	tests := []struct {
		name          string
		path          string
		wantConfig    *Config
		wantErr       bool
		errorContains string
	}{
		{
			name:       "valid config file",
			path:       testFiles.validConfigFile,
			wantConfig: expectedConfig,
			wantErr:    false,
		},
		{
			name:          "empty file",
			path:          testFiles.emptyConfigFile,
			wantErr:       true,
			errorContains: "mappings file is empty",
		},
		{
			name:          "nonexistent file",
			path:          "nonexistent.yaml",
			wantErr:       true,
			errorContains: "mappings file does not exist",
		},
		{
			name:          "invalid YAML format",
			path:          testFiles.invalidYamlFile,
			wantErr:       true,
			errorContains: "failed to parse config file",
		},
		{
			name:          "invalid domain",
			path:          testFiles.invalidDomainFile,
			wantErr:       true,
			errorContains: "invalid source registry domain",
		},
		{
			name:          "invalid value (missing slash)",
			path:          testFiles.invalidValueFile,
			wantErr:       true,
			errorContains: "invalid target registry value",
		},
		{
			name:          "invalid file extension",
			path:          testFiles.invalidExtFile,
			wantErr:       true,
			errorContains: "mappings file path must end with .yaml or .yml",
		},
		{
			name:          "path is a directory",
			path:          testFiles.configDir,
			wantErr:       true,
			errorContains: "is a directory, not a file",
		},
		{
			name:          "invalid path traversal",
			path:          "../../../etc/passwd.yaml",
			wantErr:       true,
			errorContains: "mappings file does not exist",
		},
		{
			name:          "empty path",
			path:          "",
			wantConfig:    nil,
			wantErr:       true,
			errorContains: "config file path cannot be empty",
		},
		{
			name:          "duplicate keys",
			path:          testFiles.duplicateKeysFile,
			wantErr:       true,
			errorContains: "duplicate registry key",
		},
		{
			name:          "invalid port number",
			path:          testFiles.invalidPortFile,
			wantErr:       true,
			errorContains: "invalid port number",
		},
		{
			name:          "key too long",
			path:          testFiles.longKeyFile,
			wantErr:       true,
			errorContains: "registry key",
		},
		{
			name:          "value too long",
			path:          testFiles.longValueFile,
			wantErr:       true,
			errorContains: "registry value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine if CWD restriction should be skipped for this test case.
			// Only the 'invalid path traversal' test needs the check active (skip=false).
			skipCheck := tt.name != "invalid path traversal"

			// Call LoadConfig with the test filesystem
			got, err := LoadConfig(fs, tt.path, skipCheck)

			if tt.wantErr {
				require.Error(t, err)
				// Print the actual error for debugging
				t.Logf("Actual error: %q", err.Error())
				assert.Contains(t, err.Error(), tt.errorContains)
				return
			}

			require.NoError(t, err)
			if tt.path == "" {
				assert.Nil(t, got)
			} else {
				// Compare the Config objects
				assert.Equal(t, tt.wantConfig.Registries.Mappings[0].Source, got.Registries.Mappings[0].Source)
				assert.Equal(t, tt.wantConfig.Registries.Mappings[0].Target, got.Registries.Mappings[0].Target)
				assert.Equal(t, tt.wantConfig.Registries.Mappings[1].Source, got.Registries.Mappings[1].Source)
				assert.Equal(t, tt.wantConfig.Registries.Mappings[1].Target, got.Registries.Mappings[1].Target)
				assert.Equal(t, tt.wantConfig.Registries.Mappings[2].Source, got.Registries.Mappings[2].Source)
				assert.Equal(t, tt.wantConfig.Registries.Mappings[2].Target, got.Registries.Mappings[2].Target)
			}
		})
	}
}

// TestFilesConfig contains file paths for config tests
type TestFilesConfig struct {
	validConfigFile   string
	emptyConfigFile   string
	invalidYamlFile   string
	invalidDomainFile string
	invalidValueFile  string
	invalidExtFile    string
	configDir         string
	duplicateKeysFile string
	invalidPortFile   string
	longKeyFile       string
	longValueFile     string
}

// setupConfigTestFiles creates test files for configuration testing
func setupConfigTestFiles(t *testing.T, fs afero.Fs, tmpDir string) (files TestFilesConfig, expectedConfig *Config) {
	// Create test file paths
	files = TestFilesConfig{
		validConfigFile:   filepath.Join(tmpDir, "valid-config.yaml"),
		emptyConfigFile:   filepath.Join(tmpDir, "empty-config.yaml"),
		invalidYamlFile:   filepath.Join(tmpDir, "invalid-yaml.yaml"),
		invalidDomainFile: filepath.Join(tmpDir, "invalid-domain.yaml"),
		invalidValueFile:  filepath.Join(tmpDir, "invalid-value.yaml"),
		invalidExtFile:    filepath.Join(tmpDir, "invalid-ext.txt"),
		configDir:         filepath.Join(tmpDir, "config-dir"),
		duplicateKeysFile: filepath.Join(tmpDir, "duplicate-keys.yaml"),
		invalidPortFile:   filepath.Join(tmpDir, "invalid-port.yaml"),
		longKeyFile:       filepath.Join(tmpDir, "long-key.yaml"),
		longValueFile:     filepath.Join(tmpDir, "long-value.yaml"),
	}

	// Create test directory
	require.NoError(t, fs.MkdirAll(tmpDir, fileutil.ReadWriteExecuteUserReadExecuteOthers))
	require.NoError(t, fs.MkdirAll(files.configDir, fileutil.ReadWriteExecuteUserReadExecuteOthers))

	// Create valid config file content
	validConfigContent := `
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
    - source: quay.io
      target: harbor.example.com/quay
    - source: gcr.io
      target: harbor.example.com/gcr
`

	// Create invalid domain file content
	invalidDomainContent := `
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
    - source: invalid_domain_with_underscore
      target: harbor.example.com/invalid
`

	// Create invalid value file content (missing slash)
	invalidValueContent := `
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
    - source: quay.io
      target: missingslash
`

	// Invalid YAML content
	invalidYamlContent := `
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
    - invalid: yaml: format
`

	// Duplicate keys content
	duplicateKeysContent := `
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
    - source: docker.io
      target: harbor.example.com/other
`

	// Invalid port content
	invalidPortContent := `
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
    - source: quay.io
      target: myregistry.example.com:99999/path
`

	// Long key content
	longKey := "a" + strings.Repeat("x", MaxKeyLength)
	longKeyContent := `
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
    - source: ` + longKey + `
      target: harbor.example.com/long
`

	// Long value content
	longValue := "a" + strings.Repeat("x", MaxValueLength)
	longValueContent := `
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
    - source: long.example.com
      target: ` + longValue + `
`

	// Write test files
	require.NoError(t, afero.WriteFile(fs, files.validConfigFile, []byte(validConfigContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, files.emptyConfigFile, []byte{}, fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, files.invalidYamlFile, []byte(invalidYamlContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, files.invalidDomainFile, []byte(invalidDomainContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, files.invalidValueFile, []byte(invalidValueContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, files.invalidExtFile, []byte(validConfigContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, files.duplicateKeysFile, []byte(duplicateKeysContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, files.invalidPortFile, []byte(invalidPortContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, files.longKeyFile, []byte(longKeyContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, files.longValueFile, []byte(longValueContent), fileutil.ReadWriteUserReadOthers))

	// Create expected config
	expectedConfig = &Config{
		Registries: RegConfig{
			Mappings: []RegMapping{
				{Source: "docker.io", Target: "harbor.example.com/docker", Enabled: true},
				{Source: "quay.io", Target: "harbor.example.com/quay", Enabled: true},
				{Source: "gcr.io", Target: "harbor.example.com/gcr", Enabled: true},
			},
		},
	}

	return files, expectedConfig
}

// TestIsValidDomain tests the domain validation function
func TestIsValidDomain(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		want   bool
	}{
		{
			name:   "simple domain",
			domain: "example.com",
			want:   true,
		},
		{
			name:   "domain with subdomain",
			domain: "sub.example.com",
			want:   true,
		},
		{
			name:   "wildcard domain",
			domain: "*.example.com",
			want:   true,
		},
		{
			name:   "domain with hyphen",
			domain: "my-example.com",
			want:   true,
		},
		{
			name:   "kubernetes registry",
			domain: "k8s.gcr.io",
			want:   true,
		},
		{
			name:   "regional registry",
			domain: "us-east1.gcr.io",
			want:   true,
		},
		{
			name:   "empty string",
			domain: "",
			want:   false,
		},
		{
			name:   "invalid domain with underscore",
			domain: "invalid_domain.com",
			want:   false,
		},
		{
			name:   "invalid domain with special chars",
			domain: "invalid@domain.com",
			want:   false,
		},
		{
			name:   "invalid domain with leading hyphen",
			domain: "-invalid.com",
			want:   false,
		},
		{
			name:   "invalid domain with trailing hyphen",
			domain: "invalid-.com",
			want:   false,
		},
		{
			name:   "invalid single label",
			domain: "localhost",
			want:   false,
		},
		{
			name:   "invalid empty label",
			domain: "example..com",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidDomain(tt.domain)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestLoadStructuredConfig tests the structured config loading functionality
func TestLoadStructuredConfig(t *testing.T) {
	// Create a memory-backed filesystem for testing
	fs := afero.NewMemMapFs()
	tmpDir := TestTmpDir

	// Create test file paths
	validStructuredFile := filepath.Join(tmpDir, "valid-structured.yaml")
	invalidStructuredFile := filepath.Join(tmpDir, "invalid-structured.yaml")
	emptyMappingsFile := filepath.Join(tmpDir, "empty-mappings.yaml")
	invalidSourceFile := filepath.Join(tmpDir, "invalid-source.yaml")

	// Create test directory
	require.NoError(t, fs.MkdirAll(tmpDir, fileutil.ReadWriteExecuteUserReadExecuteOthers))

	// Create valid structured config file content
	validStructuredContent := `
version: "1"
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
      description: "Docker Hub to Harbor"
    - source: quay.io
      target: harbor.example.com/quay
      description: "Quay to Harbor"
    - source: gcr.io
      target: harbor.example.com/gcr
      description: "GCR to Harbor"
  defaultTarget: harbor.example.com/default
  strictMode: false
compatibility:
  ignoreEmptyFields: true
`

	// Create invalid structured config file content
	invalidStructuredContent := `
version: "1"
registries:
  invalid_key: value
`

	// Create empty mappings file content
	emptyMappingsContent := `
version: "1"
registries:
  mappings: []
  defaultTarget: harbor.example.com/default
`

	// Create invalid source file content
	invalidSourceContent := `
version: "1"
registries:
  mappings:
    - source: invalid_domain_with_underscore
      target: harbor.example.com/invalid
    - source: docker.io
      target: harbor.example.com/docker
  defaultTarget: harbor.example.com/default
`

	// Write test files
	require.NoError(t, afero.WriteFile(fs, validStructuredFile, []byte(validStructuredContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, invalidStructuredFile, []byte(invalidStructuredContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, emptyMappingsFile, []byte(emptyMappingsContent), fileutil.ReadWriteUserReadOthers))
	require.NoError(t, afero.WriteFile(fs, invalidSourceFile, []byte(invalidSourceContent), fileutil.ReadWriteUserReadOthers))

	// Expected valid config result
	expectedMappings := []RegMapping{
		{Source: "docker.io", Target: "harbor.example.com/docker", Description: "Docker Hub to Harbor", Enabled: true},
		{Source: "quay.io", Target: "harbor.example.com/quay", Description: "Quay to Harbor", Enabled: true},
		{Source: "gcr.io", Target: "harbor.example.com/gcr", Description: "GCR to Harbor", Enabled: true},
	}

	// Test LoadStructuredConfig
	tests := []struct {
		name          string
		path          string
		wantConfig    *Config
		wantErr       bool
		errorContains string
	}{
		{
			name: "valid structured config file",
			path: validStructuredFile,
			wantConfig: &Config{
				Version: "1",
				Registries: RegConfig{
					Mappings:      expectedMappings,
					DefaultTarget: "harbor.example.com/default",
					StrictMode:    false,
				},
				Compatibility: CompatibilityConfig{
					IgnoreEmptyFields: true,
				},
			},
			wantErr: false,
		},
		{
			name:          "invalid structured config",
			path:          invalidStructuredFile,
			wantErr:       true,
			errorContains: "mappings section is empty",
		},
		{
			name:          "empty mappings",
			path:          emptyMappingsFile,
			wantErr:       true,
			errorContains: "mappings section is empty",
		},
		{
			name:          "invalid source domain",
			path:          invalidSourceFile,
			wantErr:       true,
			errorContains: "invalid source registry domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip CWD restriction for all these tests
			skipCheck := true

			// Call LoadStructuredConfig with the test filesystem
			got, err := LoadStructuredConfig(fs, tt.path, skipCheck)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantConfig.Version, got.Version)
			assert.Equal(t, tt.wantConfig.Registries.DefaultTarget, got.Registries.DefaultTarget)
			assert.Equal(t, tt.wantConfig.Registries.StrictMode, got.Registries.StrictMode)
			assert.Equal(t, tt.wantConfig.Compatibility.IgnoreEmptyFields, got.Compatibility.IgnoreEmptyFields)

			// Check mappings
			require.Equal(t, len(tt.wantConfig.Registries.Mappings), len(got.Registries.Mappings))
			for i, mapping := range tt.wantConfig.Registries.Mappings {
				assert.Equal(t, mapping.Source, got.Registries.Mappings[i].Source)
				assert.Equal(t, mapping.Target, got.Registries.Mappings[i].Target)
				assert.Equal(t, mapping.Description, got.Registries.Mappings[i].Description)
				assert.Equal(t, mapping.Enabled, got.Registries.Mappings[i].Enabled)
			}

			// Test ToMappings conversion
			mappings := got.ToMappings()
			assert.Equal(t, len(expectedMappings), len(mappings.Entries))
			for i, entry := range mappings.Entries {
				assert.Equal(t, expectedMappings[i].Source, entry.Source)
				assert.Equal(t, expectedMappings[i].Target, entry.Target)
			}
		})
	}

	// Test through LoadConfig
	t.Run("LoadConfig with structured file", func(t *testing.T) {
		// Call LoadConfig with a structured file - should automatically parse it correctly
		got, err := LoadConfig(fs, validStructuredFile, true)
		require.NoError(t, err)

		// Verify it correctly loaded the structured file
		assert.NotNil(t, got)
		assert.Equal(t, 3, len(got.Registries.Mappings))
		assert.Equal(t, "harbor.example.com/default", got.Registries.DefaultTarget)
	})
}

// TestEnabledFlagBehavior tests the behavior of the Enabled flag in registry mappings
func TestEnabledFlagBehavior(t *testing.T) {
	// Create a memory-backed filesystem for testing
	fs := afero.NewMemMapFs()
	tmpDir := TestTmpDir
	configFile := filepath.Join(tmpDir, "enabled-flag.yaml")

	// Create test content with explicitly disabled mapping
	content := `
registries:
  mappings:
    - source: docker.io
      target: registry.example.com/docker
    - source: quay.io
      target: registry.example.com/quay
      enabled: false
      description: "Explicitly disabled"
    - source: k8s.gcr.io
      target: registry.example.com/k8s
      enabled: true
`

	// Set up the filesystem
	require.NoError(t, fs.MkdirAll(tmpDir, fileutil.ReadWriteExecuteUserReadExecuteOthers))
	require.NoError(t, afero.WriteFile(fs, configFile, []byte(content), fileutil.ReadWriteUserReadOthers))

	// Load the config
	config, err := LoadStructuredConfig(fs, configFile, true)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify regular mappings are enabled by default
	assert.True(t, config.Registries.Mappings[0].Enabled, "docker.io mapping should be enabled")

	// Verify explicitly disabled mapping stays disabled
	assert.False(t, config.Registries.Mappings[1].Enabled, "quay.io mapping should remain disabled")
	assert.Equal(t, "Explicitly disabled", config.Registries.Mappings[1].Description)

	// Verify explicitly enabled mapping stays enabled
	assert.True(t, config.Registries.Mappings[2].Enabled, "k8s.gcr.io mapping should be enabled")

	// Convert to Mappings - this should only include enabled entries
	mappings := config.ToMappings()
	require.NotNil(t, mappings)

	// Should only have 2 entries (docker.io and k8s.gcr.io, not quay.io)
	assert.Len(t, mappings.Entries, 2)

	// Verify the right entries were included
	var foundDocker, foundK8s, foundQuay bool
	for _, m := range mappings.Entries {
		switch m.Source {
		case "docker.io":
			foundDocker = true
		case "k8s.gcr.io":
			foundK8s = true
		case "quay.io":
			foundQuay = true
		}
	}

	assert.True(t, foundDocker, "docker.io should be included in mappings")
	assert.True(t, foundK8s, "k8s.gcr.io should be included in mappings")
	assert.False(t, foundQuay, "quay.io should NOT be included in mappings (it's disabled)")
}
