package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
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
//
//nolint:unused // This function is available for future tests requiring an in-memory filesystem with cleanup
func setupMemoryFSContext(t *testing.T) (fs afero.Fs, tempDir string, cleanup func()) {
	// Save original state
	originalFS := AppFs
	originalDebug := os.Getenv("DEBUG")

	// Set up test environment
	err := os.Setenv("DEBUG", "1")
	require.NoError(t, err, "Failed to set DEBUG environment variable")

	fs = afero.NewMemMapFs()
	tempDir = "/test/chart"
	err = fs.MkdirAll(tempDir, fileutil.ReadWriteExecuteUserReadExecuteOthers)
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

// TestHandleTestModeOverride is a more focused test that directly tests the function
// designed for test mode
func TestHandleTestModeOverride(t *testing.T) {
	// Save and restore original values
	originalIsHelmPlugin := isHelmPlugin
	originalIsTestMode := isTestMode
	originalFs := AppFs
	defer func() {
		isHelmPlugin = originalIsHelmPlugin
		isTestMode = originalIsTestMode
		AppFs = originalFs
	}()

	// Set both plugin mode and test mode
	isHelmPlugin = true
	isTestMode = true

	// Setup in-memory filesystem
	AppFs = afero.NewMemMapFs()

	// Create a new command with test args
	cmd := newOverrideCmd()
	cmd.SetArgs([]string{
		"test-release", // Positional arg for release name
		"--target-registry", "target.com",
		"--source-registries", "docker.io",
		"--dry-run",     // Enable dry run
		"--no-validate", // Explicitly disable validation
	})

	// Manually set dry-run flag to true since we're calling handleTestModeOverride directly
	// and not going through the normal flag parsing
	cmd.Flags().Set("dry-run", "true")

	output := &bytes.Buffer{}
	cmd.SetOut(output)

	// Call the test mode handler directly with the release name
	err := handleTestModeOverride(cmd, "test-release")
	require.NoError(t, err)

	// Check output contains expected overrides
	assert.Contains(t, output.String(), "--- Dry Run: Generated Overrides ---")

	// Check that no file was created
	exists, err := afero.Exists(AppFs, "test-release-overrides.yaml")
	assert.NoError(t, err)
	assert.False(t, exists, "Output file should not be created in dry run")
}

// End of file
