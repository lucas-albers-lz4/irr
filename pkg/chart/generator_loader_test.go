package chart

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeneratorLoader_Load tests the Load method of the GeneratorLoader
func TestGeneratorLoader_Load(t *testing.T) {
	// Create a real filesystem loader for real filesystem tests
	loader := NewGeneratorLoader(nil) // Uses default filesystem

	t.Run("Load From Valid Directory", func(t *testing.T) {
		chartYaml := `
apiVersion: v2
name: testchart-generator
version: 0.4.0
`
		valuesYaml := `
replicaCount: 4
image:
  repository: nginx-generator
  tag: latest
`
		chartDir := createTempChartDir(t, "testchart-generator", chartYaml, valuesYaml)

		chartInstance, loadErr := loader.Load(chartDir)

		assert.NoError(t, loadErr, "Load should succeed for valid directory")
		require.NotNil(t, chartInstance, "Chart instance should not be nil")
		assert.Equal(t, "testchart-generator", chartInstance.Name(), "Chart name mismatch")
		assert.Equal(t, "0.4.0", chartInstance.Metadata.Version, "Chart version mismatch")
		assert.Contains(t, chartInstance.Values, "replicaCount", "Values should be loaded")
		assert.Equal(t, float64(4), chartInstance.Values["replicaCount"], "Replica count mismatch")
	})

	t.Run("Load From Non-Existent Path", func(t *testing.T) {
		nonExistentPath := filepath.Join(t.TempDir(), "does-not-exist-generator")
		_, loadErr := loader.Load(nonExistentPath)

		assert.Error(t, loadErr, "Load should fail for non-existent path")
		assert.Contains(t, loadErr.Error(), "chart path stat error", "Error should come from our FS check")
	})

	t.Run("Test FS Injection", func(t *testing.T) {
		// Create a mock filesystem
		mockFs := afero.NewMemMapFs()
		aferofs := fileutil.NewAferoFS(mockFs)

		// Create a loader with the mock filesystem
		mockLoader := NewGeneratorLoader(aferofs)

		// Setup a chart in the mock filesystem
		chartYaml := `
apiVersion: v2
name: mock-chart
version: 1.0.0
`
		valuesYaml := `
replicaCount: 1
`
		chartDir := createMockChartDir(t, mockFs, "mock-chart", chartYaml, valuesYaml)

		// Attempt to load - this will pass our fs.Stat check but fail with Helm's loader
		_, err := mockLoader.Load(chartDir)

		// We expect an error here because Helm's loader can't find the file in the real filesystem
		assert.Error(t, err)
		// But we can verify it passed our fs.Stat check by ensuring the error is from Helm's loader
		// and not from our stat check
		assert.NotContains(t, err.Error(), "chart path stat error")
		assert.Contains(t, err.Error(), "helm loader failed")
	})

	t.Run("Test SetFS", func(t *testing.T) {
		// Create a loader with the default filesystem
		loader := NewGeneratorLoader(nil)

		// Create a mock filesystem
		mockFs := afero.NewMemMapFs()
		aferofs := fileutil.NewAferoFS(mockFs)

		// Replace the filesystem
		cleanup := loader.SetFS(aferofs)

		// Verify using the mock filesystem by checking a non-existent path
		_, err := loader.Load("/non-existent-path")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "chart path stat error")

		// Restore the original filesystem
		cleanup()

		// Verify we're back to the default filesystem
		// This assumes that default filesystem is the OS filesystem
		testFile := filepath.Join(t.TempDir(), "test.txt")
		err = os.WriteFile(testFile, []byte("test"), FilePermissions)
		require.NoError(t, err)

		// After cleanup, the loader should be able to access files in the real filesystem
		// It will pass the Stat check but might fail in the Helm loader part
		_, err = loader.Load(testFile)
		assert.Error(t, err)                                        // Will error because it's not a chart
		assert.NotContains(t, err.Error(), "chart path stat error") // But it should pass our FS stat check
	})
}
