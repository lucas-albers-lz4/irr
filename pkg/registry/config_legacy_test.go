package registry

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFormatCompatibility(t *testing.T) {
	// Create a memory-backed filesystem for testing
	fs := afero.NewMemMapFs()
	tmpDir := TestTmpDir

	// Create test directory
	require.NoError(t, fs.MkdirAll(tmpDir, 0o755))

	// Test the disabled mappings case directly
	t.Run("disabled mappings in structured", func(t *testing.T) {
		// Create a config with explicitly disabled mapping
		config := &Config{
			Registries: RegConfig{
				Mappings: []RegMapping{
					{
						Source:      "docker.io",
						Target:      "harbor.example.com/docker",
						Description: "Docker Hub",
						Enabled:     true,
					},
					{
						Source:      "quay.io",
						Target:      "harbor.example.com/quay",
						Description: "Quay.io",
						Enabled:     true,
					},
					{
						Source:      "gcr.io",
						Target:      "harbor.example.com/gcr",
						Description: "GCR disabled",
						Enabled:     false,
					},
				},
			},
		}

		// Convert to legacy format
		legacyFormat := ConvertToLegacyFormat(config)

		// Verify only enabled mappings are included
		expected := map[string]string{
			"docker.io": "harbor.example.com/docker",
			"quay.io":   "harbor.example.com/quay",
		}
		assert.Equal(t, expected, legacyFormat)
		assert.NotContains(t, legacyFormat, "gcr.io")

		// Verify ToMappings produces correct entries
		mappings := config.ToMappings()
		assert.Equal(t, len(expected), len(mappings.Entries))
	})

	// Test scenarios to verify compatibility between legacy and structured formats
	tests := []struct {
		name           string
		legacyContent  string
		structContent  string
		expectedResult map[string]string
		wantErr        bool
	}{
		{
			name: "equivalent configs produce identical results",
			legacyContent: `
docker.io: harbor.example.com/docker
quay.io: harbor.example.com/quay
gcr.io: harbor.example.com/gcr
`,
			structContent: `
version: "1"
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
    - source: quay.io
      target: harbor.example.com/quay
    - source: gcr.io
      target: harbor.example.com/gcr
`,
			expectedResult: map[string]string{
				"docker.io": "harbor.example.com/docker",
				"quay.io":   "harbor.example.com/quay",
				"gcr.io":    "harbor.example.com/gcr",
			},
			wantErr: false,
		},
		{
			name: "compatibility flags affect parsing behavior",
			legacyContent: `
# This is not a valid config for structured format
docker.io: harbor.example.com/docker
`,
			structContent: `
version: "1"
registries:
  mappings:
    - source: docker.io
      target: harbor.example.com/docker
compatibility:
  legacyFlatFormat: true
`,
			expectedResult: map[string]string{
				"docker.io": "harbor.example.com/docker",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the legacy and structured format files
			legacyPath := filepath.Join(tmpDir, "legacy-"+tt.name+".yaml")
			structPath := filepath.Join(tmpDir, "struct-"+tt.name+".yaml")

			require.NoError(t, afero.WriteFile(fs, legacyPath, []byte(tt.legacyContent), 0o644))
			require.NoError(t, afero.WriteFile(fs, structPath, []byte(tt.structContent), 0o644))

			// Skip CWD restriction for all tests
			skipCheck := true

			// Test 1: Load legacy format directly
			legacyResult, err := LoadConfig(fs, legacyPath, skipCheck)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedResult, legacyResult)

			// Test 2: Load structured format directly
			structResult, err := LoadConfig(fs, structPath, skipCheck)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedResult, structResult)

			// Test 3: Verify structured parsing directly
			structConfig, err := LoadStructuredConfig(fs, structPath, skipCheck)
			require.NoError(t, err)

			// Convert to legacy format for comparison
			convertedLegacy := ConvertToLegacyFormat(structConfig)
			assert.Equal(t, tt.expectedResult, convertedLegacy)

			// Test 4: Ensure ToMappings produces correct entries
			mappings := structConfig.ToMappings()
			assert.Equal(t, len(tt.expectedResult), len(mappings.Entries))
		})
	}
}

// TestLegacyConfigFallback tests the fallback mechanism when structured parsing fails
func TestLegacyConfigFallback(t *testing.T) {
	// Create a memory-backed filesystem for testing
	fs := afero.NewMemMapFs()
	tmpDir := TestTmpDir

	// Create test directory
	require.NoError(t, fs.MkdirAll(tmpDir, 0o755))

	// Create a config file that's valid in legacy format but not in structured format
	legacyOnlyPath := filepath.Join(tmpDir, "legacy-only.yaml")
	legacyContent := `
docker.io: harbor.example.com/docker
quay.io: harbor.example.com/quay
`
	require.NoError(t, afero.WriteFile(fs, legacyOnlyPath, []byte(legacyContent), 0o644))

	// Expected result for the legacy-only format
	expectedResult := map[string]string{
		"docker.io": "harbor.example.com/docker",
		"quay.io":   "harbor.example.com/quay",
	}

	// Test LoadConfig with fallback to legacy format
	result, err := LoadConfig(fs, legacyOnlyPath, true)
	require.NoError(t, err)
	assert.Equal(t, expectedResult, result)

	// Test structured parsing (should fail)
	_, err = LoadStructuredConfig(fs, legacyOnlyPath, true)
	assert.Error(t, err)
}
