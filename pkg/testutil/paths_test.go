// Package testutil_test contains tests for the testutil package.
package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

// fsTest is a helper struct for filesystem mocking
type fsTest struct {
	fs afero.Fs
	t  *testing.T
}

// setupMockFS creates a mock filesystem for testing
func setupMockFS(t *testing.T) *fsTest {
	fs := afero.NewMemMapFs()

	// Set up the test directory structure
	projectRoot := "/mock-project-root"
	testDataDir := filepath.Join(projectRoot, "test-data")
	testChartsDir := filepath.Join(testDataDir, "charts")

	// Create the test directories
	if err := fs.MkdirAll(testChartsDir, fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
		t.Fatalf("Failed to create mock test charts directory: %v", err)
	}

	// Create some test chart directories
	chartDirs := []string{
		"basic-chart",
		filepath.FromSlash("nested/sub-chart"), // Use FromSlash to handle path separators properly
		"complex-chart-with-dependencies",
	}

	for _, dir := range chartDirs {
		chartPath := filepath.Join(testChartsDir, dir)
		if err := fs.MkdirAll(chartPath, fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
			t.Fatalf("Failed to create mock chart directory %s: %v", dir, err)
		}

		// Add a simple Chart.yaml to each chart
		chartYaml := filepath.Join(chartPath, "Chart.yaml")
		err := afero.WriteFile(fs, chartYaml, []byte("name: "+filepath.Base(dir)), fileutil.ReadWriteUserReadOthers)
		require.NoError(t, err)
	}

	return &fsTest{fs: fs, t: t}
}

// TestGetChartPath tests the GetChartPath function
func TestGetChartPath(t *testing.T) {
	// Save original TestChartsDir
	origTestChartsDir := TestChartsDir
	defer func() {
		TestChartsDir = origTestChartsDir
	}()

	// Define test cases
	testCases := []struct {
		name      string
		chartName string
		want      string
	}{
		{
			name:      "simple chart name",
			chartName: "basic-chart",
			want:      filepath.Join(TestChartsDir, "basic-chart"),
		},
		{
			name:      "nested chart path",
			chartName: filepath.FromSlash("nested/sub-chart"), // Use FromSlash to handle path separators properly
			want:      filepath.Join(TestChartsDir, filepath.FromSlash("nested/sub-chart")),
		},
		{
			name:      "complex chart name",
			chartName: "complex-chart-with-dependencies",
			want:      filepath.Join(TestChartsDir, "complex-chart-with-dependencies"),
		},
		{
			name:      "chart that doesn't exist",
			chartName: "nonexistent-chart",
			want:      filepath.Join(TestChartsDir, "nonexistent-chart"),
		},
		{
			name:      "empty chart name",
			chartName: "",
			want:      TestChartsDir,
		},
	}

	// In a real environment
	for _, tc := range testCases {
		t.Run("real_"+tc.name, func(t *testing.T) {
			got := GetChartPath(tc.chartName)
			if got != tc.want {
				t.Errorf("GetChartPath(%q) = %q, want %q", tc.chartName, got, tc.want)
			}
		})
	}

	// With mock filesystem
	t.Run("mock_filesystem", func(t *testing.T) {
		// Set up mock filesystem
		mockTest := setupMockFS(t)

		// Temporarily override TestChartsDir with mock value
		mockChartsDir := "/mock-project-root/test-data/charts"
		TestChartsDir = mockChartsDir

		for _, tc := range testCases {
			t.Run("mock_"+tc.name, func(t *testing.T) {
				// Adjust expected path for mock filesystem
				want := filepath.Join(mockChartsDir, tc.chartName)

				got := GetChartPath(tc.chartName)
				if got != want {
					t.Errorf("GetChartPath(%q) = %q, want %q", tc.chartName, got, want)
				}

				// Check if the file exists in our mock filesystem
				if tc.chartName != "" && tc.chartName != "nonexistent-chart" {
					chartYamlPath := filepath.Join(got, "Chart.yaml")
					exists, err := afero.Exists(mockTest.fs, chartYamlPath)
					if err != nil {
						t.Fatalf("Error checking file existence: %v", err)
					}
					if !exists {
						t.Errorf("Expected Chart.yaml at %q to exist in mock filesystem", chartYamlPath)
					}
				}
			})
		}
	})
}

// TestTestDataDirInitialization tests the initialization of TestDataDir and TestChartsDir
func TestTestDataDirInitialization(t *testing.T) {
	if TestDataDir == "" {
		t.Error("TestDataDir is empty, expected non-empty value")
	}

	if TestChartsDir == "" {
		t.Error("TestChartsDir is empty, expected non-empty value")
	}

	// Verify that TestChartsDir is a subdirectory of TestDataDir
	if filepath.Dir(TestChartsDir) != TestDataDir {
		t.Errorf("TestChartsDir %q is not a subdirectory of TestDataDir %q", TestChartsDir, TestDataDir)
	}

	// Check that the directories exist
	if _, err := os.Stat(TestDataDir); err != nil {
		t.Errorf("TestDataDir %q does not exist: %v", TestDataDir, err)
	}

	if _, err := os.Stat(TestChartsDir); err != nil {
		t.Errorf("TestChartsDir %q does not exist: %v", TestChartsDir, err)
	}
}
