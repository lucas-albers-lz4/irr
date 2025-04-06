package chart

import (
	"os"
	"path/filepath"
	"testing"

	// "github.com/spf13/afero" // No longer needed for this test file
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	// Assuming types are needed from analysis or somewhere else - adjust as needed
	// "github.com/lalbers/irr/pkg/analysis"
)

// Helper to create a temporary chart directory for testing HelmLoader
func createTempChartDir(t *testing.T, name string, chartYaml string, valuesYaml string) string {
	t.Helper()
	tempDir := t.TempDir()

	chartPath := filepath.Join(tempDir, name)
	err := os.MkdirAll(filepath.Join(chartPath, "templates"), 0750)
	require.NoError(t, err, "Failed to create chart dir structure")

	err = os.WriteFile(filepath.Join(chartPath, "Chart.yaml"), []byte(chartYaml), 0600)
	require.NoError(t, err, "Failed to write Chart.yaml")

	err = os.WriteFile(filepath.Join(chartPath, "values.yaml"), []byte(valuesYaml), 0600)
	require.NoError(t, err, "Failed to write values.yaml")

	// Add a dummy template file
	err = os.WriteFile(filepath.Join(chartPath, "templates", "dummy.yaml"), []byte("kind: Pod"), 0600)
	require.NoError(t, err, "Failed to write dummy template")

	return chartPath
}

// TestHelmLoaderLoad tests the Load method of the concrete helmLoader implementation.
// It uses the real filesystem via t.TempDir().
func TestHelmLoaderLoad(t *testing.T) {
	loader := NewLoader() // Get the concrete implementation
	require.IsType(t, &helmLoader{}, loader, "NewLoader should return helmLoader type")

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
		// Check if the error is file not found related (os specific checks might be needed)
		// Update: Helm loader checks for Chart.yaml first.
		// assert.ErrorContains(t, loadErr, "no such file or directory")
		assert.ErrorContains(t, loadErr, "Chart.yaml file is missing", "Helm loader error mismatch for non-existent path")
		assert.Nil(t, chartInstance, "Chart instance should be nil on error")
	})

	t.Run("Load From File Path (Not Dir or TGZ)", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "not-a-chart.txt")
		err := os.WriteFile(filePath, []byte("hello"), 0600)
		require.NoError(t, err)

		chartInstance, loadErr := loader.Load(filePath)

		assert.Error(t, loadErr, "Load should fail for a plain file path")
		// Helm's loader.LoadDir returns a specific error type here
		// Update: It also checks for Chart.yaml.
		// assert.ErrorContains(t, loadErr, "chart directory not found")
		assert.ErrorContains(t, loadErr, "Chart.yaml file is missing", "Helm loader error mismatch for file path")
		assert.Nil(t, chartInstance, "Chart instance should be nil on error")
	})

	// TODO: Test Load From Valid TGZ (Requires creating a tgz file)
	// TODO: Test Load With Subcharts (Requires more complex temp dir setup)
}

// TODO: Add tests for ProcessChart function (Removed, no longer needed)
