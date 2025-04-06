// Package testutil provides utility functions for testing.
package testutil

import (
	"os"
	"path/filepath"
	"runtime"
)

var (
	// TestDataDir is the absolute path to the test-data directory
	TestDataDir string
	// TestChartsDir is the absolute path to the test-data/charts directory
	TestChartsDir string
)

func init() {
	// Get the directory containing this file
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	// Initialize the test data directories
	TestDataDir = filepath.Join(projectRoot, "test-data")
	TestChartsDir = filepath.Join(TestDataDir, "charts")

	// Verify the directories exist
	if _, err := os.Stat(TestChartsDir); err != nil {
		panic("Test charts directory not found. Make sure you're running from the project root.")
	}
}

// GetChartPath returns the absolute path to a chart in the test-data/charts directory
func GetChartPath(chartName string) string {
	return filepath.Join(TestChartsDir, chartName)
}
