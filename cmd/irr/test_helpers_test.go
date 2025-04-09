package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

// This file contains helper functions for testing Cobra commands, especially those needing file system interaction.

// setupTestFS creates an in-memory file system and a directory for testing.
func setupTestFS(t *testing.T) (afero.Fs, string) {
	fs := afero.NewMemMapFs()
	// Create a directory for the chart
	chartDir := "/test/chart"
	err := fs.MkdirAll(chartDir, 0o755)
	require.NoError(t, err, "Failed to create test chart directory")
	return fs, chartDir
}

// createDummyChart creates basic Chart.yaml and values.yaml in the specified directory on the given FS.
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

// setupMemoryFSContext sets up a memory filesystem for a test and returns
// a cleanup function that restores the original filesystem state.
// It also ensures DEBUG=1 is set to enable detailed logging.
func setupMemoryFSContext(t *testing.T) (afero.Fs, string, func()) {
	// Save original state
	originalFS := AppFs
	originalDebug := os.Getenv("DEBUG")

	// Set up test environment
	err := os.Setenv("DEBUG", "1")
	require.NoError(t, err, "Failed to set DEBUG environment variable")

	fs := afero.NewMemMapFs()
	chartDir := "/test/chart"
	err = fs.MkdirAll(chartDir, 0o755)
	require.NoError(t, err, "Failed to create test chart directory")

	// Replace global AppFs
	AppFs = fs

	// Create cleanup function
	cleanup := func() {
		// Restore original state
		AppFs = originalFS
		err := os.Setenv("DEBUG", originalDebug)
		if err != nil {
			t.Logf("Warning: Failed to restore DEBUG environment variable: %v", err)
		}
	}

	return fs, chartDir, cleanup
}

// End of file
