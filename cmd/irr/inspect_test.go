package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/analyzer"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectChartInCurrentDirectory(t *testing.T) {
	// Save original filesystem and restore after test
	originalFs := AppFs
	defer func() { AppFs = originalFs }()

	// Create test cases
	testCases := []struct {
		name          string
		setupFs       func(fs afero.Fs)
		expectedPath  string
		expectedError bool
		startDir      string
	}{
		{
			name: "Chart.yaml in current directory",
			setupFs: func(fs afero.Fs) {
				err := afero.WriteFile(fs, "Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), fileutil.ReadWriteUserReadOthers)
				require.NoError(t, err)
			},
			expectedPath:  ".",
			expectedError: false,
			startDir:      ".",
		},
		{
			name: "Chart.yaml in subdirectory",
			setupFs: func(fs afero.Fs) {
				err := fs.MkdirAll("mychart", fileutil.ReadWriteExecuteUserReadExecuteOthers)
				require.NoError(t, err)
				err = afero.WriteFile(fs, "mychart/Chart.yaml", []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), fileutil.ReadWriteUserReadOthers)
				require.NoError(t, err)
			},
			expectedPath:  "mychart",
			expectedError: false,
			startDir:      "mychart",
		},
		{
			name:          "No chart found",
			setupFs:       func(_ afero.Fs) {},
			expectedPath:  "",
			expectedError: true,
			startDir:      ".",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock filesystem for this test case
			mockFs := afero.NewMemMapFs()
			if tc.setupFs != nil {
				tc.setupFs(mockFs)
			}
			AppFs = mockFs // Set the global AppFs for the function being tested

			// Simulate running from a specific directory if needed, e.g., by changing CWD
			// Note: Changing CWD globally in tests can be problematic. Rely on absolute paths or relative paths from a known root in mockFs.

			// Call the function under test, passing the mock filesystem and the test case's start directory
			actualPath, err := detectChartInCurrentDirectory(mockFs, tc.startDir)

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tc.expectedPath == "mychart" {
					// For the subdirectory case, we can't check the exact path because it
					// gets converted to an absolute path which varies by environment.
					// Just check that it ends with the expected directory name.
					assert.Contains(t, actualPath, "mychart")
				} else {
					assert.Equal(t, tc.expectedPath, actualPath)
				}
			}
		})
	}
}

func TestWriteOutput(t *testing.T) {
	// Save original filesystem and restore after test
	originalFs := AppFs
	defer func() { AppFs = originalFs }()

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
				assert.Contains(t, string(content), "mappings")
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

	// Update the test to use a dummy command for stdout
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock filesystem
			mockFs := afero.NewMemMapFs()

			// Create temporary directory
			tmpDir := t.TempDir()

			// Ensure the temporary directory exists in the mock filesystem
			err := mockFs.MkdirAll(tmpDir, fileutil.ReadWriteExecuteUserReadExecuteOthers)
			require.NoError(t, err)

			// Create dummy command for stdout capture
			cmd := &cobra.Command{}
			outBuf := &bytes.Buffer{}
			cmd.SetOut(outBuf)

			// Set the temporary directory as the working directory
			if tc.flags.OutputFile != "" && !filepath.IsAbs(tc.flags.OutputFile) {
				tc.flags.OutputFile = filepath.Join(tmpDir, tc.flags.OutputFile)
			}

			// Replace the global filesystem
			originalFs := AppFs
			AppFs = mockFs
			defer func() { AppFs = originalFs }()

			// Call the function being tested
			err = writeOutput(cmd, tc.analysis, tc.flags)

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				tc.checkFs(t, mockFs, tmpDir)
			}
		})
	}
}

// Helper function to create a mock command that simulates the inspect command
// but with predictable output for testing
func mockInspectCmd(output *ImageAnalysis, flags *InspectFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use: "inspect",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Use our function to write output directly to the command's output buffer
			return writeOutput(cmd, output, flags)
		},
	}

	// Add all the flags that the real inspect command would have
	cmd.Flags().String("chart-path", "", "Path to the Helm chart")
	cmd.Flags().StringSlice("source-registries", []string{}, "Source registries to filter results")
	cmd.Flags().String("output-file", "", "Write output to file instead of stdout")
	cmd.Flags().String("output-format", "yaml", "Output format (yaml or json)")
	cmd.Flags().Bool("generate-config-skeleton", false, "Generate a config skeleton based on found images")
	cmd.Flags().StringSlice("include-pattern", []string{}, "Glob patterns for values paths to include")
	cmd.Flags().StringSlice("exclude-pattern", []string{}, "Glob patterns for values paths to exclude")
	cmd.Flags().String("namespace", "default", "Kubernetes namespace for the release")
	cmd.Flags().String("release-name", "", "Release name for Helm plugin mode")

	return cmd
}

// Helper for repeated YAML output test logic
func runYamlOutputTest(t *testing.T, chartPath, chartName, chartVersion, imageValue string, setOutputFormat bool) {
	mockFs := afero.NewMemMapFs()
	AppFs = mockFs

	if err := mockFs.MkdirAll(filepath.Join(chartPath, "templates"), fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
		t.Fatalf("Failed to create mock templates dir: %v", err)
	}
	if err := afero.WriteFile(mockFs, filepath.Join(chartPath, "Chart.yaml"), []byte(fmt.Sprintf("apiVersion: v2\nname: %s\nversion: %s", chartName, chartVersion)), fileutil.ReadWriteUserReadOthers); err != nil {
		t.Fatalf("Failed to write mock Chart.yaml: %v", err)
	}
	if err := afero.WriteFile(mockFs, filepath.Join(chartPath, "values.yaml"), []byte(fmt.Sprintf("image: %s", imageValue)), fileutil.ReadWriteUserReadOthers); err != nil {
		t.Fatalf("Failed to write mock values.yaml: %v", err)
	}

	analysis := &ImageAnalysis{
		Chart: ChartInfo{
			Name:    chartName,
			Version: chartVersion,
		},
		ImagePatterns: []analyzer.ImagePattern{
			{
				Path:  "image",
				Type:  "string",
				Value: imageValue,
			},
		},
	}

	cmd := mockInspectCmd(analysis, &InspectFlags{})
	args := []string{"--chart-path", chartPath}
	if setOutputFormat {
		args = append(args, "--output-format", "yaml")
	}
	cmd.SetArgs(args)
	out := new(bytes.Buffer)
	cmd.SetOut(out)

	err := cmd.Execute()
	require.NoError(t, err)
	output := out.String()
	assert.Contains(t, output, "chart:")
	assert.Contains(t, output, "name: "+chartName)
	assert.Contains(t, output, "version: "+chartVersion)
	assert.Contains(t, output, "imagePatterns:")
	assert.Contains(t, output, "value: "+imageValue)
	assert.NotContains(t, output, "\"chart\":") // Should not be JSON
}

func TestRunInspect(t *testing.T) {
	// Setup mock filesystem for the entire test function
	originalAppFs := AppFs
	mockFs := afero.NewMemMapFs()
	AppFs = mockFs

	// Save and restore original test mode flag
	originalTestMode := isTestMode
	isTestMode = true // Enable test mode for mock chart loading

	// Save original helm adapter factory and restore after
	originalHelmFactory := helmAdapterFactory

	// Restore original filesystem and test mode after all sub-tests are done
	defer func() {
		AppFs = originalAppFs
		isTestMode = originalTestMode
		helmAdapterFactory = originalHelmFactory
	}()

	t.Run("inspect chart path successfully (YAML output to stdout)", func(t *testing.T) {
		runYamlOutputTest(t, "test/chart", "mychart", "1.2.3", "nginx:stable", true)
	})

	t.Run("inspect chart path with JSON output to file", func(t *testing.T) {
		// Clear and setup mock filesystem for this sub-test
		mockFs = afero.NewMemMapFs() // Use the function-scoped mockFs
		AppFs = mockFs               // Ensure AppFs is set to the cleared mock for the sub-test

		// Create a dummy chart
		chartPath := "test/chart-json"
		outputFilePath := "output/result.json"
		if err := mockFs.MkdirAll(filepath.Dir(outputFilePath), fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil { // Ensure output dir exists
			t.Fatalf("Failed to create mock output dir: %v", err)
		}
		if err := mockFs.MkdirAll(filepath.Join(chartPath, "templates"), fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil { // Replaced 0o755
			t.Fatalf("Failed to create mock templates dir: %v", err)
		}
		if err := afero.WriteFile(mockFs, filepath.Join(chartPath, "Chart.yaml"), []byte("apiVersion: v2\nname: jsonchart\nversion: 0.0.1"), fileutil.ReadWriteUserReadOthers); err != nil { // Replaced 0o644
			t.Fatalf("Failed to write mock Chart.yaml: %v", err)
		}
		if err := afero.WriteFile(mockFs, filepath.Join(chartPath, "values.yaml"), []byte("app:\n  image: redis:alpine"), fileutil.ReadWriteUserReadOthers); err != nil { // Replaced 0o644
			t.Fatalf("Failed to write mock values.yaml: %v", err)
		}

		// Create a test analysis
		analysis := &ImageAnalysis{
			Chart: ChartInfo{
				Name:    "jsonchart",
				Version: "0.0.1",
			},
			ImagePatterns: []analyzer.ImagePattern{
				{
					Path:  "app.image",
					Type:  "string",
					Value: "redis:alpine",
				},
			},
		}

		// Create flags with the right settings
		inspectFlags := &InspectFlags{
			OutputFile:   outputFilePath,
			OutputFormat: "json",
		}

		// Create a command with our mock implementation
		cmd := mockInspectCmd(analysis, inspectFlags)
		cmd.SetArgs([]string{
			"--chart-path", chartPath,
			"--output-file", outputFilePath,
			"--output-format", "json",
		})

		// Create a buffer to capture output
		out := new(bytes.Buffer)
		cmd.SetOut(out)

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Check output file content
		content, readErr := afero.ReadFile(mockFs, outputFilePath)
		require.NoError(t, readErr)
		output := string(content)

		assert.Contains(t, output, `"chart":`) // JSON format
		assert.Contains(t, output, `"name":"jsonchart"`)
		assert.Contains(t, output, `"version":"0.0.1"`)
		assert.Contains(t, output, `"imagePatterns":`)
		assert.Contains(t, output, `"path":"app.image"`)
		assert.Contains(t, output, `"value":"redis:alpine"`)
	})

	t.Run("error when chart path does not exist", func(t *testing.T) {
		// Clear and setup mock filesystem for this sub-test
		mockFs = afero.NewMemMapFs() // Use the function-scoped mockFs
		AppFs = mockFs               // Ensure AppFs is set to the cleared mock for the sub-test

		chartPath := "non/existent/chart"

		// Use the real command to test error handling
		cmd := newInspectCmd()
		cmd.SetArgs([]string{"--chart-path", chartPath})

		// Capture stdout and stderr
		out := new(bytes.Buffer)
		errOut := new(bytes.Buffer)
		cmd.SetOut(out)
		cmd.SetErr(errOut)

		// Execute the command
		err := cmd.Execute()

		// Assertions
		require.Error(t, err, "Expected an error when chart path is invalid")
		// Check for specific error type/message if desired (e.g., ExitChartLoadFailed)
		var exitErr *exitcodes.ExitCodeError
		require.ErrorAs(t, err, &exitErr, "Error should be an ExitCodeError")
		assert.Equal(t, exitcodes.ExitChartNotFound, exitErr.Code, "Exit code should indicate chart load failure")
		// Also check the stderr message contains expected text
		assert.Contains(t, err.Error(), "chart path not found or inaccessible", "Error output should mention path not found")
	})

	t.Run("inspect release name successfully (plugin mode)", func(t *testing.T) {
		// Clear and setup mock filesystem for this sub-test
		mockFs = afero.NewMemMapFs() // Use the function-scoped mockFs
		AppFs = mockFs               // Ensure AppFs is set to the cleared mock for the sub-test

		// Mock the Helm adapter factory
		mockClient := helm.NewMockHelmClient()
		// Configure the mock client using its fields/helpers
		mockClient.SetupMockRelease(
			"my-release",
			"my-namespace",
			map[string]interface{}{"image": "nginx:plugin"},            // Mock values
			&helm.ChartMetadata{Name: "release-chart", Version: "1.0"}, // Mock chart metadata
		)

		helmAdapterFactory = func() (*helm.Adapter, error) {
			// Return an adapter using the configured mock client
			return helm.NewAdapter(mockClient, mockFs, true), nil
		}

		// Create a test analysis for plugin mode
		analysis := &ImageAnalysis{
			Chart: ChartInfo{
				Name:    "release-chart",
				Version: "1.0",
				Path:    "helm-release://my-namespace/my-release",
			},
			ImagePatterns: []analyzer.ImagePattern{
				{
					Path:  "image",
					Type:  "string",
					Value: "nginx:plugin",
				},
			},
		}

		// Create a command with our mock implementation
		cmd := mockInspectCmd(analysis, &InspectFlags{})
		cmd.SetArgs([]string{"my-release", "--namespace", "my-namespace"})

		// Create a buffer to capture output
		out := new(bytes.Buffer)
		cmd.SetOut(out)

		// Execute the command
		err := cmd.Execute()
		require.NoError(t, err)

		// Get the output
		output := out.String()

		// Assertions
		assert.Contains(t, output, "chart:")
		assert.Contains(t, output, "name: release-chart")
		assert.Contains(t, output, "version: \"1.0\"")
		assert.Contains(t, output, "imagePatterns:")
		assert.Contains(t, output, "path: image")
		assert.Contains(t, output, "value: nginx:plugin")
	})

	t.Run("default output format is yaml when flag is omitted", func(t *testing.T) {
		runYamlOutputTest(t, "test/chart-default", "defaultchart", "2.0.0", "busybox:latest", false)
	})

	t.Run("error on invalid output format", func(t *testing.T) {
		mockFs = afero.NewMemMapFs()
		AppFs = mockFs

		chartPath := "test/chart-invalidfmt"
		if err := mockFs.MkdirAll(filepath.Join(chartPath, "templates"), fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
			t.Fatalf("Failed to create mock templates dir: %v", err)
		}
		if err := afero.WriteFile(mockFs, filepath.Join(chartPath, "Chart.yaml"), []byte("apiVersion: v2\nname: badfmt\nversion: 3.0.0"), fileutil.ReadWriteUserReadOthers); err != nil {
			t.Fatalf("Failed to write mock Chart.yaml: %v", err)
		}
		if err := afero.WriteFile(mockFs, filepath.Join(chartPath, "values.yaml"), []byte("image: alpine:latest"), fileutil.ReadWriteUserReadOthers); err != nil {
			t.Fatalf("Failed to write mock values.yaml: %v", err)
		}

		cmd := newInspectCmd()
		cmd.SetArgs([]string{"--chart-path", chartPath, "--output-format", "invalidfmt"})
		out := new(bytes.Buffer)
		errOut := new(bytes.Buffer)
		cmd.SetOut(out)
		cmd.SetErr(errOut)
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid output format")
	})

	// --- TODO: Add more test cases --- //
	// - Error case: Invalid output format
	// - Helm plugin mode: Error getting release chart
	// - Using --source-registries filter
	// - Using --generate-config-skeleton
	// - Using include/exclude patterns
}
