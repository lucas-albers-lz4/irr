package main

import (
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

// This file contains helper functions for testing Cobra commands, especially those needing file system interaction.

// setupTestFS creates a temporary in-memory filesystem for testing purposes.
// It returns the afero.Fs instance and the path to the temporary directory.
// Note: This function is currently unused but kept for potential future use.
/*
func setupTestFS(t *testing.T) (fs afero.Fs, tempDir string) {
	fs = afero.NewMemMapFs()
	tempDir, err := afero.TempDir(fs, "", "testfs")
	if err != nil {
		t.Fatalf("Failed to create temp dir in memory filesystem: %v", err)
	}
	return fs, tempDir
}
*/

// createDummyChart creates basic Chart.yaml and values.yaml in the specified directory on the given FS.
// Note: This function is currently unused but kept for potential future use.
/*
func createDummyChart(fs afero.Fs, dir string) error {
	chartYaml := `apiVersion: v2
name: test-chart
version: 0.1.0`
	if err := afero.WriteFile(fs, dir+"/Chart.yaml", []byte(chartYaml), 0o644); err != nil {
		return fmt.Errorf("failed to write dummy Chart.yaml: %w", err)
	}
	valuesYaml := `image:
  registry: source.io
  repository: library/nginx
  tag: 1.20`
	if err := afero.WriteFile(fs, dir+"/values.yaml", []byte(valuesYaml), 0o644); err != nil {
		return fmt.Errorf("failed to write dummy values.yaml: %w", err)
	}
	return nil
}
*/

// setupMemoryFSContext sets up an in-memory filesystem with a temporary directory
func setupMemoryFSContext(t *testing.T) (fs afero.Fs, tempDir string, cleanup func()) {
	// Save original state
	originalFS := AppFs
	originalDebug := os.Getenv("DEBUG")

	// Set up test environment
	err := os.Setenv("DEBUG", "1")
	require.NoError(t, err, "Failed to set DEBUG environment variable")

	fs = afero.NewMemMapFs()
	tempDir = "/test/chart"
	err = fs.MkdirAll(tempDir, 0o755)
	require.NoError(t, err, "Failed to create test chart directory")

	// Replace global AppFs
	AppFs = fs

	// Create cleanup function
	cleanup = func() {
		// Restore original state
		AppFs = originalFS
		err := os.Setenv("DEBUG", originalDebug)
		if err != nil {
			t.Logf("Warning: Failed to restore DEBUG environment variable: %v", err)
		}
	}

	return fs, tempDir, cleanup
}

// End of file
