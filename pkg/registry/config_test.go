package registry

import (
	"path/filepath"
	"strings"
	"testing"

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
		wantConfig    map[string]string
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
			errorContains: "must contain at least one '/'",
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
			errorContains: "failed to read config file",
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
			wantConfig:    nil,
			wantErr:       true,
			errorContains: "no configuration file specified",
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
				assert.Contains(t, err.Error(), tt.errorContains)
				return
			}

			require.NoError(t, err)
			if tt.path == "" {
				assert.Equal(t, EmptyPathResult, got)
			} else {
				assert.Equal(t, tt.wantConfig, got)
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
func setupConfigTestFiles(t *testing.T, fs afero.Fs, tmpDir string) (files TestFilesConfig, expectedConfig map[string]string) {
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
	require.NoError(t, fs.MkdirAll(tmpDir, 0o755))
	require.NoError(t, fs.MkdirAll(files.configDir, 0o755))

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
    - source: ` + longKey + `
      target: harbor.example.com/long
`

	// Long value content
	longValue := "harbor.example.com/" + strings.Repeat("x", MaxValueLength)
	longValueContent := `
registries:
  mappings:
    - source: docker.io
      target: ` + longValue + `
`

	// Write test files
	require.NoError(t, afero.WriteFile(fs, files.validConfigFile, []byte(validConfigContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, files.emptyConfigFile, []byte(""), 0o644))
	require.NoError(t, afero.WriteFile(fs, files.invalidYamlFile, []byte(invalidYamlContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, files.invalidDomainFile, []byte(invalidDomainContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, files.invalidValueFile, []byte(invalidValueContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, files.invalidExtFile, []byte(validConfigContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, files.duplicateKeysFile, []byte(duplicateKeysContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, files.invalidPortFile, []byte(invalidPortContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, files.longKeyFile, []byte(longKeyContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, files.longValueFile, []byte(longValueContent), 0o644))

	// Expected valid config result
	expectedConfig = map[string]string{
		"docker.io": "harbor.example.com/docker",
		"quay.io":   "harbor.example.com/quay",
		"gcr.io":    "harbor.example.com/gcr",
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
	require.NoError(t, fs.MkdirAll(tmpDir, 0o755))

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
	require.NoError(t, afero.WriteFile(fs, validStructuredFile, []byte(validStructuredContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, invalidStructuredFile, []byte(invalidStructuredContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, emptyMappingsFile, []byte(emptyMappingsContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, invalidSourceFile, []byte(invalidSourceContent), 0o644))

	// Expected valid config result
	expectedMappings := []RegMapping{
		{Source: "docker.io", Target: "harbor.example.com/docker", Description: "Docker Hub to Harbor", Enabled: true},
		{Source: "quay.io", Target: "harbor.example.com/quay", Description: "Quay to Harbor", Enabled: true},
		{Source: "gcr.io", Target: "harbor.example.com/gcr", Description: "GCR to Harbor", Enabled: true},
	}

	// Expected converted legacy format
	expectedLegacyFormat := map[string]string{
		"docker.io": "harbor.example.com/docker",
		"quay.io":   "harbor.example.com/quay",
		"gcr.io":    "harbor.example.com/gcr",
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
			errorContains: "mappings file is empty",
		},
		{
			name:          "empty mappings",
			path:          emptyMappingsFile,
			wantErr:       true,
			errorContains: "mappings file is empty",
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

			// Test conversion to legacy format
			legacyFormat := ConvertToLegacyFormat(got)
			assert.Equal(t, expectedLegacyFormat, legacyFormat)

			// Test ToMappings conversion
			mappings := got.ToMappings()
			assert.Equal(t, len(expectedMappings), len(mappings.Entries))
			for i, entry := range mappings.Entries {
				assert.Equal(t, expectedMappings[i].Source, entry.Source)
				assert.Equal(t, expectedMappings[i].Target, entry.Target)
			}
		})
	}

	// Test backward compatibility through LoadConfig
	t.Run("LoadConfig with structured file", func(t *testing.T) {
		// Call LoadConfig with a structured file - should automatically parse it correctly
		got, err := LoadConfig(fs, validStructuredFile, true)
		require.NoError(t, err)
		assert.Equal(t, expectedLegacyFormat, got)
	})
}

// TestEnabledFlagBehavior tests the behavior of the enabled flag in structured config
func TestEnabledFlagBehavior(t *testing.T) {
	// Test case where a mapping is explicitly disabled
	explicitlyDisabled := &Config{
		Registries: RegConfig{
			Mappings: []RegMapping{
				{
					Source:      "docker.io",
					Target:      "harbor.example.com/docker",
					Enabled:     true,
					Description: "Explicitly enabled",
				},
				{
					Source:      "gcr.io",
					Target:      "harbor.example.com/gcr",
					Enabled:     false,                 // Explicitly disabled
					Description: "Explicitly disabled", // Adding description to help with detection
				},
			},
		},
	}

	// Validate should not change the explicitly disabled flag
	err := validateStructuredConfig(explicitlyDisabled, "test-path")
	require.NoError(t, err)

	// Check that explicitly disabled flag remains false
	assert.True(t, explicitlyDisabled.Registries.Mappings[0].Enabled)
	assert.False(t, explicitlyDisabled.Registries.Mappings[1].Enabled)

	// Convert to legacy format - disabled mapping should be excluded
	legacyMap := ConvertToLegacyFormat(explicitlyDisabled)
	assert.Equal(t, 1, len(legacyMap))
	assert.Equal(t, "harbor.example.com/docker", legacyMap["docker.io"])
	assert.NotContains(t, legacyMap, "gcr.io")
}
