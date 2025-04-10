package registry

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create a memory-backed filesystem for testing
	fs := afero.NewMemMapFs()
	tmpDir := "/tmp"

	// Create test file paths
	validConfigFile := filepath.Join(tmpDir, "valid-config.yaml")
	emptyConfigFile := filepath.Join(tmpDir, "empty-config.yaml")
	invalidYamlFile := filepath.Join(tmpDir, "invalid-yaml.yaml")
	invalidDomainFile := filepath.Join(tmpDir, "invalid-domain.yaml")
	invalidValueFile := filepath.Join(tmpDir, "invalid-value.yaml")
	invalidExtFile := filepath.Join(tmpDir, "invalid-ext.txt")
	configDir := filepath.Join(tmpDir, "config-dir")
	duplicateKeysFile := filepath.Join(tmpDir, "duplicate-keys.yaml")
	invalidPortFile := filepath.Join(tmpDir, "invalid-port.yaml")
	longKeyFile := filepath.Join(tmpDir, "long-key.yaml")
	longValueFile := filepath.Join(tmpDir, "long-value.yaml")

	// Create test directory
	require.NoError(t, fs.MkdirAll(tmpDir, 0o755))
	require.NoError(t, fs.MkdirAll(configDir, 0o755))

	// Create valid config file content
	validConfigContent := `
docker.io: harbor.example.com/docker
quay.io: harbor.example.com/quay
gcr.io: harbor.example.com/gcr
`

	// Create invalid domain file content
	invalidDomainContent := `
docker.io: harbor.example.com/docker
invalid_domain_with_underscore: harbor.example.com/invalid
`

	// Create invalid value file content (missing slash)
	invalidValueContent := `
docker.io: harbor.example.com/docker
quay.io: missingslash
`

	// Invalid YAML content
	invalidYamlContent := `
docker.io: harbor.example.com/docker
- invalid: yaml: format
`

	// Duplicate keys content
	duplicateKeysContent := `
docker.io: harbor.example.com/docker
docker.io: harbor.example.com/other
`

	// Invalid port content
	invalidPortContent := `
docker.io: harbor.example.com/docker
quay.io: myregistry.example.com:99999/path
`

	// Long key content
	longKey := "a" + strings.Repeat("x", MaxKeyLength)
	longKeyContent := longKey + ": harbor.example.com/long"

	// Long value content
	longValue := "harbor.example.com/" + strings.Repeat("x", MaxValueLength)
	longValueContent := "docker.io: " + longValue

	// Write test files
	require.NoError(t, afero.WriteFile(fs, validConfigFile, []byte(validConfigContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, emptyConfigFile, []byte(""), 0o644))
	require.NoError(t, afero.WriteFile(fs, invalidYamlFile, []byte(invalidYamlContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, invalidDomainFile, []byte(invalidDomainContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, invalidValueFile, []byte(invalidValueContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, invalidExtFile, []byte(validConfigContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, duplicateKeysFile, []byte(duplicateKeysContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, invalidPortFile, []byte(invalidPortContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, longKeyFile, []byte(longKeyContent), 0o644))
	require.NoError(t, afero.WriteFile(fs, longValueFile, []byte(longValueContent), 0o644))

	// Expected valid config result
	expectedConfig := map[string]string{
		"docker.io": "harbor.example.com/docker",
		"quay.io":   "harbor.example.com/quay",
		"gcr.io":    "harbor.example.com/gcr",
	}

	tests := []struct {
		name          string
		path          string
		wantConfig    map[string]string
		wantErr       bool
		errorContains string
	}{
		{
			name:       "valid config file",
			path:       validConfigFile,
			wantConfig: expectedConfig,
			wantErr:    false,
		},
		{
			name:          "empty file",
			path:          emptyConfigFile,
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
			path:          invalidYamlFile,
			wantErr:       true,
			errorContains: "failed to parse mappings file",
		},
		{
			name:          "invalid domain",
			path:          invalidDomainFile,
			wantErr:       true,
			errorContains: "invalid source registry domain",
		},
		{
			name:          "invalid value (missing slash)",
			path:          invalidValueFile,
			wantErr:       true,
			errorContains: "must contain at least one '/'",
		},
		{
			name:          "invalid file extension",
			path:          invalidExtFile,
			wantErr:       true,
			errorContains: "mappings file path must end with .yaml or .yml",
		},
		{
			name:          "path is a directory",
			path:          configDir,
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
			name:       "empty path",
			path:       "",
			wantConfig: nil,
			wantErr:    false,
		},
		{
			name:          "duplicate keys",
			path:          duplicateKeysFile,
			wantErr:       true,
			errorContains: "duplicate registry key",
		},
		{
			name:          "invalid port number",
			path:          invalidPortFile,
			wantErr:       true,
			errorContains: "invalid port number",
		},
		{
			name:          "key too long",
			path:          longKeyFile,
			wantErr:       true,
			errorContains: "registry key",
		},
		{
			name:          "value too long",
			path:          longValueFile,
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
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tt.wantConfig, got)
			}
		})
	}
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
