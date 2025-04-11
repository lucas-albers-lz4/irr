// Package chart_test contains tests for the chart package.
package chart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	// "github.com/spf13/afero" // No longer needed for this test file
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	// Assuming types are needed from analysis or somewhere else - adjust as needed
	// "github.com/lalbers/irr/pkg/analysis"
)

// Helper to create a temporary chart directory for testing DefaultLoader
func createTempChartDir(t *testing.T, name, chartYaml, valuesYaml string) string {
	t.Helper()
	tempDir := t.TempDir()

	chartPath := filepath.Join(tempDir, name)
	err := os.MkdirAll(filepath.Join(chartPath, "templates"), 0o750)
	require.NoError(t, err, "Failed to create chart dir structure")

	err = os.WriteFile(filepath.Join(chartPath, "Chart.yaml"), []byte(chartYaml), 0o600)
	require.NoError(t, err, "Failed to write Chart.yaml")

	err = os.WriteFile(filepath.Join(chartPath, "values.yaml"), []byte(valuesYaml), 0o600)
	require.NoError(t, err, "Failed to write values.yaml")

	// Add a dummy template file
	err = os.WriteFile(filepath.Join(chartPath, "templates", "dummy.yaml"), []byte("kind: Pod"), 0o600)
	require.NoError(t, err, "Failed to write dummy template")

	return chartPath
}

// TestDefaultLoaderLoad tests the Load method of the concrete DefaultLoader implementation.
// It uses the real filesystem via t.TempDir().
func TestDefaultLoaderLoad(t *testing.T) {
	loader := &DefaultLoader{} // Use the DefaultLoader struct
	require.IsType(t, &DefaultLoader{}, loader, "DefaultLoader should be the correct type")

	t.Run("Load From Valid Directory", func(t *testing.T) {
		chartYaml := `
apiVersion: v2
name: testchart-realfs
version: 0.2.0
`
		valuesYaml := `
replicaCount: 2
image:
  repository: nginx-real
  tag: latest
`
		chartDir := createTempChartDir(t, "testchart-realfs", chartYaml, valuesYaml)

		chartInstance, loadErr := loader.Load(chartDir)

		assert.NoError(t, loadErr, "Load should succeed for valid directory")
		require.NotNil(t, chartInstance, "Chart instance should not be nil")
		assert.Equal(t, "testchart-realfs", chartInstance.Name(), "Chart name mismatch")
		assert.Equal(t, "0.2.0", chartInstance.Metadata.Version, "Chart version mismatch")
		assert.Contains(t, chartInstance.Values, "replicaCount", "Values should be loaded")
		assert.Equal(t, float64(2), chartInstance.Values["replicaCount"], "Replica count mismatch") // Helm parses YAML numbers as float64
		assert.NotEmpty(t, chartInstance.Templates, "Templates should be loaded")
	})

	t.Run("Load From Non-Existent Path", func(t *testing.T) {
		nonExistentPath := filepath.Join(t.TempDir(), "does-not-exist")
		chartInstance, loadErr := loader.Load(nonExistentPath)

		assert.Error(t, loadErr, "Load should fail for non-existent path")
		// Error message depends on the Helm version, but should include "no such file or directory"
		assert.Contains(t, loadErr.Error(), "no such file or directory", "Error message should indicate file not found")
		assert.Nil(t, chartInstance, "Chart instance should be nil on error")
	})

	t.Run("Load From File Path (Not Dir or TGZ)", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "not-a-chart.txt")
		err := os.WriteFile(filePath, []byte("hello"), 0o600)
		require.NoError(t, err)

		chartInstance, loadErr := loader.Load(filePath)

		assert.Error(t, loadErr, "Load should fail for a plain file path")
		// Error message depends on the Helm version - check for typical messages
		errMsg := loadErr.Error()
		isExpectedError := strings.Contains(errMsg, "does not appear to be a gzipped archive") ||
			strings.Contains(errMsg, "file does not appear to be a valid chart") ||
			strings.Contains(errMsg, "Chart.yaml file is missing")
		assert.True(t, isExpectedError, "Error should indicate invalid chart format: %s", errMsg)
		assert.Nil(t, chartInstance, "Chart instance should be nil on error")
	})

	// TODO: Test Load From Valid TGZ (Requires creating a tgz file)
	// TODO: Test Load With Subcharts (Requires more complex temp dir setup)
}

// TODO: Add tests for ProcessChart function (Removed, no longer needed)

func setupTestChart(t *testing.T, chartPath string) {
	// Create templates directory
	err := os.MkdirAll(filepath.Join(chartPath, "templates"), 0o750)
	require.NoErrorf(t, err, "failed to create templates directory in %s", chartPath)

	// Create Chart.yaml
	chartYaml := []byte(`
apiVersion: v2
name: test-chart
version: 0.1.0
`)
	err = os.WriteFile(filepath.Join(chartPath, "Chart.yaml"), chartYaml, 0o600)
	require.NoErrorf(t, err, "failed to create Chart.yaml in %s", chartPath)

	// Create values.yaml
	valuesYaml := []byte(`
image:
  repository: nginx
  tag: latest
`)
	err = os.WriteFile(filepath.Join(chartPath, "values.yaml"), valuesYaml, 0o600)
	require.NoErrorf(t, err, "failed to create values.yaml in %s", chartPath)

	// Create a dummy template file
	err = os.WriteFile(filepath.Join(chartPath, "templates", "dummy.yaml"), []byte("kind: Pod"), 0o600)
	require.NoErrorf(t, err, "failed to create dummy.yaml in %s", chartPath)
}

func TestDefaultLoader_LoadChart(t *testing.T) {
	// Create a temporary directory for the test chart
	chartPath, err := os.MkdirTemp("", "irr-test-")
	require.NoErrorf(t, err, "failed to create temp directory")
	defer func() {
		if err := os.RemoveAll(chartPath); err != nil {
			t.Logf("Warning: Failed to cleanup temp directory %s: %v", chartPath, err)
		}
	}()

	// Setup test chart
	setupTestChart(t, chartPath)

	// Test cases
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid chart",
			path:    chartPath,
			wantErr: false,
		},
		{
			name:    "nonexistent chart",
			path:    "/nonexistent/chart",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := &DefaultLoader{} // Use DefaultLoader
			_, err := loader.Load(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultLoader_LoadChartWithInvalidFile(t *testing.T) {
	// Create a temporary directory for the test chart
	chartPath, err := os.MkdirTemp("", "irr-test-")
	require.NoErrorf(t, err, "failed to create temp directory")
	defer func() {
		if err := os.RemoveAll(chartPath); err != nil {
			t.Logf("Warning: Failed to cleanup temp directory %s: %v", chartPath, err)
		}
	}()

	// Create an invalid Chart.yaml
	filePath := filepath.Join(chartPath, "Chart.yaml")
	err = os.WriteFile(filePath, []byte("hello"), 0o600)
	require.NoErrorf(t, err, "failed to create invalid Chart.yaml in %s", chartPath)

	// Test loading the invalid chart
	loader := &DefaultLoader{} // Use DefaultLoader
	_, err = loader.Load(chartPath)
	assert.Error(t, err, "expected error loading invalid chart")
}
