package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectChartInCurrentDirectory(t *testing.T) {
	// Create test cases
	testCases := []struct {
		name          string
		setupFs       func(fs afero.Fs)
		expectedPath  string
		expectedError bool
	}{
		{
			name: "Chart.yaml in current directory",
			setupFs: func(fs afero.Fs) {
				err := afero.WriteFile(fs, "Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), 0o644)
				require.NoError(t, err)
			},
			expectedPath:  ".",
			expectedError: false,
		},
		{
			name: "Chart.yaml in subdirectory",
			setupFs: func(fs afero.Fs) {
				err := fs.MkdirAll("mychart", 0o755)
				require.NoError(t, err)
				err = afero.WriteFile(fs, "mychart/Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), 0o644)
				require.NoError(t, err)
			},
			expectedPath:  "mychart",
			expectedError: false,
		},
		{
			name:          "No chart found",
			setupFs:       func(_ afero.Fs) {},
			expectedPath:  "",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock filesystem
			mockFs := afero.NewMemMapFs()
			tc.setupFs(mockFs)

			// Replace global filesystem with mock
			reset := SetFs(mockFs)
			defer reset() // Restore original filesystem

			// Call the function
			path, err := detectChartInCurrentDirectory()

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tc.expectedPath == "mychart" {
					// For the subdirectory case, we can't check the exact path because it
					// gets converted to an absolute path which varies by environment.
					// Just check that it ends with the expected directory name.
					assert.Contains(t, path, "mychart")
				} else {
					assert.Equal(t, tc.expectedPath, path)
				}
			}
		})
	}
}

func TestWriteOutput(t *testing.T) {
	// Create test cases
	testCases := []struct {
		name          string
		analysis      *ImageAnalysis
		flags         *InspectFlags
		checkFs       func(t *testing.T, fs afero.Fs, tmpDir string)
		expectedError bool
	}{
		{
			name: "Write to file",
			analysis: &ImageAnalysis{
				Chart: ChartInfo{
					Name:    "test-chart",
					Version: "1.0.0",
				},
				Images: []ImageInfo{
					{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "latest",
					},
				},
			},
			flags: &InspectFlags{
				OutputFile: "output.yaml",
			},
			checkFs: func(t *testing.T, fs afero.Fs, tmpDir string) {
				outputPath := filepath.Join(tmpDir, "output.yaml")
				exists, err := afero.Exists(fs, outputPath)
				assert.NoError(t, err)
				assert.True(t, exists, "Output file should exist")

				content, err := afero.ReadFile(fs, outputPath)
				assert.NoError(t, err)
				assert.Contains(t, string(content), "test-chart")
				assert.Contains(t, string(content), "docker.io")
				assert.Contains(t, string(content), "library/nginx")
			},
			expectedError: false,
		},
		{
			name: "Generate config skeleton",
			analysis: &ImageAnalysis{
				Images: []ImageInfo{
					{
						Registry:   "docker.io",
						Repository: "library/nginx",
					},
					{
						Registry:   "quay.io",
						Repository: "some/image",
					},
				},
			},
			flags: &InspectFlags{
				GenerateConfigSkeleton: true,
				OutputFile:             "config.yaml",
			},
			checkFs: func(t *testing.T, fs afero.Fs, tmpDir string) {
				configPath := filepath.Join(tmpDir, "config.yaml")
				exists, err := afero.Exists(fs, configPath)
				assert.NoError(t, err)
				assert.True(t, exists, "Config file should exist")

				content, err := afero.ReadFile(fs, configPath)
				assert.NoError(t, err)
				assert.Contains(t, string(content), "docker.io")
				assert.Contains(t, string(content), "quay.io")
				assert.Contains(t, string(content), "registry_mappings")
			},
			expectedError: false,
		},
		{
			name: "Output to stdout",
			analysis: &ImageAnalysis{
				Chart: ChartInfo{
					Name:    "test-chart",
					Version: "1.0.0",
				},
			},
			flags: &InspectFlags{
				OutputFile: "",
			},
			checkFs: func(_ *testing.T, _ afero.Fs, _ string) {
				// Nothing to check in filesystem for stdout output
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock filesystem
			mockFs := afero.NewMemMapFs()

			// Create temporary directory
			tmpDir := t.TempDir()

			// Ensure the temporary directory exists in the mock filesystem
			err := mockFs.MkdirAll(tmpDir, 0o755)
			require.NoError(t, err)

			// Update the flags to use the temporary directory path
			flags := *tc.flags
			if flags.OutputFile != "" {
				flags.OutputFile = filepath.Join(tmpDir, flags.OutputFile)
			}

			// Replace global filesystem with mock
			reset := SetFs(mockFs)
			defer reset() // Restore original filesystem

			// Capture stdout
			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			// Call the function
			err = writeOutput(tc.analysis, &flags)

			// Close pipe and restore stdout
			errClose := w.Close()
			os.Stdout = oldStdout
			require.NoError(t, errClose)

			// Read captured stdout
			var buf bytes.Buffer
			_, errCopy := io.Copy(&buf, r)
			require.NoError(t, errCopy)
			output := buf.String()

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// For stdout output, verify content
				if flags.OutputFile == "" {
					assert.Contains(t, output, tc.analysis.Chart.Name)
				}

				// Check filesystem state
				tc.checkFs(t, mockFs, tmpDir)
			}
		})
	}
}
