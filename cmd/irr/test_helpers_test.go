package main

import (
	"fmt"
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

// End of file
