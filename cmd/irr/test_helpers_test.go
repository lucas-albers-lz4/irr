package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	// Setup mock filesystem
	fs := afero.NewMemMapFs()
	AppFs = fs                               // Set global AppFs for the test
	originalAppFs := AppFs                   // Store original AppFs to restore later
	defer func() { AppFs = originalAppFs }() // Restore original AppFs

	// Create a dummy chart structure
	chartPath := "/fake/chart"
	err := createMockChartFS(fs, chartPath)
	require.NoError(t, err, "Failed to create mock chart structure")

	tests := []struct {
		name           string
		dryRun         bool
		expectFile     bool
		expectedOutput string // Optional: check command output
	}{
		{
			name:       "Test Mode without Dry Run",
			dryRun:     false,
			expectFile: true,
		},
		{
			name:       "Test Mode with Dry Run",
			dryRun:     true,
			expectFile: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset filesystem state for each subtest if necessary (e.g., remove output file)
			// For this test, creating a fresh cmd instance per test is cleaner.
			err := fs.Remove(filepath.Join(chartPath, "override-values.yaml")) // Clean up potential file from previous run
			if err != nil && !os.IsNotExist(err) {
				t.Fatalf("Failed to remove output file: %v", err)
			}

			// Setup Cobra command for each test case
			cmd := newOverrideCmd()                           // No args needed now
			cmd.PersistentFlags().Bool("test-mode", true, "") // Enable test mode
			if err := cmd.Flags().Set("dry-run", fmt.Sprintf("%t", tc.dryRun)); err != nil {
				t.Fatalf("Failed to set dry-run flag: %v", err)
			}
			// Add required flags that were previously missing
			if err := cmd.Flags().Set("source-registries", "docker.io"); err != nil {
				t.Fatalf("Failed to set source-registries flag: %v", err)
			}
			if err := cmd.Flags().Set("target-registry", "registry.example.com"); err != nil {
				t.Fatalf("Failed to set target-registry flag: %v", err)
			}

			// Set the output file path
			expectedOutputPath := filepath.Join(chartPath, "override-values.yaml")
			if err := cmd.Flags().Set("output-file", expectedOutputPath); err != nil {
				t.Fatalf("Failed to set output-file flag: %v", err)
			}

			cmd.SetArgs([]string{"--chart-path=" + chartPath}) // Set required flag

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute the command logic
			execErr := cmd.Execute()
			output := buf.String()

			// Assertions
			assert.NoError(t, execErr, "cmd.Execute() should run without error")

			// Verify file existence based on dry-run flag
			exists, fsErr := afero.Exists(fs, expectedOutputPath)
			assert.NoError(t, fsErr, "Filesystem check should not error")

			if tc.expectFile {
				assert.True(t, exists, "Output file '%s' should exist when dry-run is false", expectedOutputPath)
				// Optionally, verify content if needed
				contentBytes, readErr := afero.ReadFile(fs, expectedOutputPath)
				assert.NoError(t, readErr, "Reading output file should not error")
				content := string(contentBytes)
				assert.Contains(t, content, "mock: true") // Check what's actually in the file
			} else {
				assert.False(t, exists, "Output file '%s' should NOT exist when dry-run is true", expectedOutputPath)
				// Optionally check output for dry-run indication if the command provides it
				// assert.Contains(t, output, "Dry run enabled", "Output should indicate dry run")
			}

			// Log completion for clarity
			log.Debugf("%s completed. Output:\n%s", tc.name, output)
		})
	}
}

// --- Test Helpers ---

// setupTestFs creates a basic mock filesystem structure for testing.
func setupTestFs(fs afero.Fs, chartDir string) {
	// Create base directories
	_ = fs.MkdirAll(filepath.Join(chartDir, "templates"), 0o750)

	// Create Chart.yaml
	chartContent := `apiVersion: v2
name: test-chart
version: 0.1.0
`
	_ = afero.WriteFile(fs, filepath.Join(chartDir, "Chart.yaml"), []byte(chartContent), 0o644)

	// Create values.yaml
	valuesContent := `image:
  repository: docker.io/library/nginx
  tag: latest
`
	_ = afero.WriteFile(fs, filepath.Join(chartDir, "values.yaml"), []byte(valuesContent), 0o644)

	// Create template files
	templateContent := `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - image: {{ .Values.image.repository }}:{{ .Values.image.tag }}
`
	_ = afero.WriteFile(fs, filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(templateContent), 0o644)
}

// executeCommandC captures stdout/stderr for a Cobra command execution and returns the command
// Credits: Adapted from https://github.com/spf13/cobra/blob/main/command_test.go
func executeCommandC(root *cobra.Command, args ...string) (c *cobra.Command, output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)

	// Use a clean argument slice for each test
	root.SetArgs(args)

	c, err = root.ExecuteC()
	return c, buf.String(), err
}

// Helper to create a simple mock chart structure in the given FS
func createMockChartFS(fs afero.Fs, chartPath string) error {
	// Ensure base directory exists
	err := fs.MkdirAll(filepath.Join(chartPath, "templates"), 0o755) // Use 0o755
	if err != nil {
		return fmt.Errorf("failed to create chart directories: %w", err)
	}
	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: mockchart
version: 0.1.0
description: A mock Helm chart for testing`
	err = afero.WriteFile(fs, filepath.Join(chartPath, "Chart.yaml"), []byte(chartYaml), 0o644) // Use 0o644
	if err != nil {
		return fmt.Errorf("failed to write Chart.yaml: %w", err)
	}
	// Create values.yaml
	valuesYaml := `replicaCount: 1
image:
  repository: nginx
  tag: stable
service:
  type: ClusterIP
  port: 80`
	err = afero.WriteFile(fs, filepath.Join(chartPath, "values.yaml"), []byte(valuesYaml), 0o644) // Use 0o644
	if err != nil {
		return fmt.Errorf("failed to write values.yaml: %w", err)
	}
	// Create a template file (e.g., deployment.yaml)
	deploymentYaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-deployment
spec:
  replicas: {{ .Values.replicaCount }}
  template:
    spec:
      containers:
      - name: nginx
        image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"`
	err = afero.WriteFile(fs, filepath.Join(chartPath, "templates", "deployment.yaml"), []byte(deploymentYaml), 0o644) // Use 0o644
	if err != nil {
		return fmt.Errorf("failed to write deployment.yaml: %w", err)
	}
	return nil
}
