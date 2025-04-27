package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/lucas-albers-lz4/irr/pkg/analyzer"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// --- Mock Factories (Global for Test Overrides) ---
var (
	helmClientFactory func() (helm.ClientInterface, error) = createHelmClient
)

// REMOVED TestDetectChartInCurrentDirectory function as it tested outdated logic.
// The functionality is now covered by TestDetectChartIfNeeded.

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

// setupInspectTest handles common setup for inspect tests, including mock filesystem and test mode.
// It returns a cleanup function that should be deferred.
func setupInspectTest(t *testing.T) func() {
	t.Helper()

	// Setup mock filesystem
	originalAppFs := AppFs
	mockFs := afero.NewMemMapFs()
	AppFs = mockFs

	// Save and restore original test mode flag
	originalTestMode := isTestMode
	isTestMode = true // Enable test mode for mock chart loading
	// Return cleanup function
	return func() {
		AppFs = originalAppFs
		isTestMode = originalTestMode
	}
}

// TestInspectStandaloneYAML tests inspecting a chart path with default YAML output to stdout.
func TestInspectStandaloneYAML(t *testing.T) {
	cleanup := setupInspectTest(t)
	defer cleanup()

	// Reuse the existing helper logic for this specific YAML test
	runYamlOutputTest(t, "test/chart", "mychart", "1.2.3", "nginx:stable", true)
}

// TestInspectStandaloneDefaultYAML tests inspecting a chart path uses YAML by default.
func TestInspectStandaloneDefaultYAML(t *testing.T) {
	cleanup := setupInspectTest(t)
	defer cleanup()

	// Reuse the existing helper logic, setting setOutputFormat to false
	runYamlOutputTest(t, "test/chart-default", "defaultchart", "2.0.0", "busybox:latest", false)
}

// TestInspectStandaloneJSONFile tests inspecting a chart path with JSON output to file.
func TestInspectStandaloneJSONFile(t *testing.T) {
	cleanup := setupInspectTest(t)
	defer cleanup()

	mockFs := AppFs // AppFs is already afero.Fs, assertion removed

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

	// Create the real inspect command
	cmd := newInspectCmd()
	cmd.SetArgs([]string{
		"--chart-path", chartPath,
		"--output-file", outputFilePath,
		"--output-format", "json",
	})

	// Create a buffer to capture stdout (should be empty)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer)) // Capture stderr too

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
	// Check for imagePatterns content based on actual analysis
	assert.Contains(t, output, `"images":[{`)          // Check if images array exists and is populated
	assert.Contains(t, output, `"repository":"redis"`) // Check details of the image found
	assert.Contains(t, output, `"imagePatterns":[{`)   // Check if imagePatterns array exists
	assert.Contains(t, output, `"path":"app.image"`)
	assert.Contains(t, output, `"value":"redis:alpine"`)
}

// TestInspectChartNotFound tests error handling when chart path does not exist.
func TestInspectChartNotFound(t *testing.T) {
	cleanup := setupInspectTest(t)
	defer cleanup()

	mockFs := AppFs // AppFs is already afero.Fs, assertion removed
	_ = mockFs      // Prevent unused variable error if not used directly

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
}

// TestInspectPluginMode tests inspecting a release name in plugin mode.
func TestInspectPluginMode(t *testing.T) {
	cleanup := setupInspectTest(t)
	defer cleanup()

	// Save original helm adapter factory and restore after
	originalHelmFactory := helmAdapterFactory
	defer func() { helmAdapterFactory = originalHelmFactory }()

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
		return helm.NewAdapter(mockClient, AppFs, true), nil
	}

	// Create the inspect command
	cmd := newInspectCmd()
	cmd.SetArgs([]string{"my-release", "-n", "my-namespace"}) // Use short flag -n

	// Create a buffer to capture output
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))

	// Execute the command
	err := cmd.Execute()
	require.NoError(t, err)

	// Get the output
	output := out.String()

	// Assertions
	assert.Contains(t, output, "chart:")
	assert.Contains(t, output, "name: release-chart")
	assert.Contains(t, output, "version: \"1.0\"") // YAML output quotes strings
	// Check for images array content
	assert.Contains(t, output, "images:")
	assert.Contains(t, output, "repository: nginx")
	assert.Contains(t, output, "tag: plugin")
	assert.Contains(t, output, "imagePatterns:")
	assert.Contains(t, output, "path: image")
	assert.Contains(t, output, "value: nginx:plugin") // Check raw value from pattern
}

// TestInspectInvalidOutputFormat tests error handling for invalid output format.
func TestInspectInvalidOutputFormat(t *testing.T) {
	cleanup := setupInspectTest(t)
	defer cleanup()

	mockFs := AppFs // AppFs is already afero.Fs, assertion removed

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
	// Corrected Exit Code Check
	var exitErr *exitcodes.ExitCodeError
	require.ErrorAs(t, err, &exitErr, "Error should be an ExitCodeError")
	assert.Equal(t, exitcodes.ExitInputConfigurationError, exitErr.Code, "Expected input config error code")
}

// TestInspectAllNamespacesSkeleton verifies that `inspect -A --generate-config-skeleton`
// correctly aggregates registries from all releases.
func TestInspectAllNamespacesSkeleton(t *testing.T) {
	cleanup := setupInspectTest(t) // Sets up mock FS and test mode
	defer cleanup()

	// --- Mock Helm Interaction ---

	// Mock Helm Releases
	mockReleases := []*helm.ReleaseElement{
		{Name: "release-1", Namespace: "ns-a"},
		{Name: "release-2", Namespace: "ns-b"},
		{Name: "release-3", Namespace: "ns-a"},
		{Name: "release-4", Namespace: "ns-c"}, // Release with no images
		{Name: "release-5", Namespace: "ns-d"}, // Release that will fail analysis
	}

	// Mock Helm Client (from internal/helm)
	mockHelmClient := &helm.MockHelmClient{} // Correct mock type
	mockHelmClient.On("ListReleases", mock.Anything, true).Return(mockReleases, nil)

	// Mock necessary GetReleaseValues calls (used by adapter -> analyzeRelease)
	mockHelmClient.SetupMockRelease("release-1", "ns-a", map[string]interface{}{"image": "docker.io/library/nginx:latest"}, &helm.ChartMetadata{Name: "chart1", Version: "1.0"})
	mockHelmClient.SetupMockRelease("release-2", "ns-b", map[string]interface{}{"image": "quay.io/prometheus/node-exporter:v1"}, &helm.ChartMetadata{Name: "chart2", Version: "1.0"})
	mockHelmClient.SetupMockRelease(
		"release-3",
		"ns-a",
		map[string]interface{}{
			"image":   "gcr.io/google-containers/pause:3.2",
			"sidecar": "docker.io/library/alpine:edge",
		},
		&helm.ChartMetadata{Name: "chart3", Version: "1.0"},
	)
	mockHelmClient.SetupMockRelease("release-4", "ns-c", map[string]interface{}{"some": "value"}, &helm.ChartMetadata{Name: "chart4", Version: "1.0"}) // No image
	// Simulate GetValues error for release-5 using the mock's error field
	mockHelmClient.GetValuesError = fmt.Errorf("simulated error getting values for release-5")

	// Also mock GetChartFromRelease as it's called by analyzeRelease for successful cases
	mockHelmClient.On("GetChartFromRelease", mock.Anything, "release-1", "ns-a").Return(&helm.ChartMetadata{Name: "chart1", Version: "1.0"}, nil)
	mockHelmClient.On("GetChartFromRelease", mock.Anything, "release-2", "ns-b").Return(&helm.ChartMetadata{Name: "chart2", Version: "1.0"}, nil)
	mockHelmClient.On("GetChartFromRelease", mock.Anything, "release-3", "ns-a").Return(&helm.ChartMetadata{Name: "chart3", Version: "1.0"}, nil)
	mockHelmClient.On("GetChartFromRelease", mock.Anything, "release-4", "ns-c").Return(&helm.ChartMetadata{Name: "chart4", Version: "1.0"}, nil)
	// No mock needed for release-5 GetChartFromRelease as GetReleaseValues should fail first

	// Inject Mocks using the package-level factory variables
	originalHelmClientFactory := helmClientFactory
	helmClientFactory = func() (helm.ClientInterface, error) {
		return mockHelmClient, nil
	}
	defer func() { helmClientFactory = originalHelmClientFactory }()

	originalHelmAdapterFactory := helmAdapterFactory
	// Override adapter factory to return a real adapter constructed with the mock client and mock FS
	// Match the expected signature: func() (*helm.Adapter, error)
	helmAdapterFactory = func() (*helm.Adapter, error) {
		adapter := helm.NewAdapter(mockHelmClient, AppFs, true) // Use mocks, assume isPlugin=true
		return adapter, nil                                     // Return concrete adapter pointer and nil error
	}
	defer func() { helmAdapterFactory = originalHelmAdapterFactory }()

	// --- Execute Command ---
	cmd := newInspectCmd()
	args := []string{
		"-A",
		"--generate-config-skeleton",
		"--output-file", "skeleton.yaml",
		"--overwrite-skeleton", // Prevent error if file exists in mock fs
	}
	cmd.SetArgs(args)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)

	err := cmd.Execute()
	// Expect GetReleaseValues to fail for release-5, but the overall command should succeed
	// because skeleton generation proceeds even with partial failures.
	require.NoError(t, err, "Command execution failed unexpectedly. Stdout: %s, Stderr: %s", out.String(), errOut.String())

	// --- Verify Output ---

	// Verify skeleton file content
	skeletonPath := "skeleton.yaml"
	exists, err := afero.Exists(AppFs, skeletonPath)
	require.NoError(t, err)
	require.True(t, exists, "Expected skeleton file '%s' was not created", skeletonPath)

	contentBytes, err := afero.ReadFile(AppFs, skeletonPath)
	require.NoError(t, err)
	content := string(contentBytes)

	// Assert that all unique registries are present
	assert.Contains(t, content, "source: docker.io", "Skeleton missing docker.io")
	assert.Contains(t, content, "source: quay.io", "Skeleton missing quay.io")
	assert.Contains(t, content, "source: gcr.io", "Skeleton missing gcr.io")

	// Assert that registries are sorted
	dockerIndex := bytes.Index(contentBytes, []byte("source: docker.io"))
	gcrIndex := bytes.Index(contentBytes, []byte("source: gcr.io"))
	quayIndex := bytes.Index(contentBytes, []byte("source: quay.io"))

	assert.True(t, dockerIndex < gcrIndex, "Registries not sorted correctly (docker.io vs gcr.io)")
	assert.True(t, gcrIndex < quayIndex, "Registries not sorted correctly (gcr.io vs quay.io)")

	// Verify mocks were called (adjust counts as needed)
	mockHelmClient.AssertCalled(t, "ListReleases", mock.Anything, true)
	// GetReleaseValues and GetChartFromRelease are called inside analyzeRelease, which is called by processAllReleases
	// Check that they were called for the successful releases
	mockHelmClient.AssertCalled(t, "GetReleaseValues", mock.Anything, "release-1", "ns-a")
	mockHelmClient.AssertCalled(t, "GetChartFromRelease", mock.Anything, "release-1", "ns-a")
	mockHelmClient.AssertCalled(t, "GetReleaseValues", mock.Anything, "release-2", "ns-b")
	mockHelmClient.AssertCalled(t, "GetChartFromRelease", mock.Anything, "release-2", "ns-b")
	mockHelmClient.AssertCalled(t, "GetReleaseValues", mock.Anything, "release-3", "ns-a")
	mockHelmClient.AssertCalled(t, "GetChartFromRelease", mock.Anything, "release-3", "ns-a")
	mockHelmClient.AssertCalled(t, "GetReleaseValues", mock.Anything, "release-4", "ns-c")
	mockHelmClient.AssertCalled(t, "GetChartFromRelease", mock.Anything, "release-4", "ns-c")
	// Assert GetReleaseValues was called for the failing one, but GetChartFromRelease was not
	mockHelmClient.AssertCalled(t, "GetReleaseValues", mock.Anything, "release-5", "ns-d")
	mockHelmClient.AssertNotCalled(t, "GetChartFromRelease", mock.Anything, "release-5", "ns-d")
}

// TestInspectAllNamespacesSkeletonWithFilter verifies that `inspect -A --generate-config-skeleton`
// correctly aggregates registries even when `--source-registries` is used for filtering output.
func TestInspectAllNamespacesSkeletonWithFilter(t *testing.T) {
	cleanup := setupInspectTest(t) // Sets up mock FS and test mode
	defer cleanup()

	// --- Mock Helm Interaction ---
	// Same mock setup as TestInspectAllNamespacesSkeleton
	mockReleases := []*helm.ReleaseElement{
		{Name: "release-1", Namespace: "ns-a"}, // docker.io
		{Name: "release-2", Namespace: "ns-b"}, // quay.io
		{Name: "release-3", Namespace: "ns-a"}, // gcr.io, docker.io
	}
	mockHelmClient := &helm.MockHelmClient{}
	mockHelmClient.On("ListReleases", mock.Anything, true).Return(mockReleases, nil)
	mockHelmClient.SetupMockRelease("release-1", "ns-a", map[string]interface{}{"image": "docker.io/library/nginx:latest"}, &helm.ChartMetadata{Name: "chart1", Version: "1.0"})
	mockHelmClient.SetupMockRelease("release-2", "ns-b", map[string]interface{}{"image": "quay.io/prometheus/node-exporter:v1"}, &helm.ChartMetadata{Name: "chart2", Version: "1.0"})
	mockHelmClient.SetupMockRelease(
		"release-3",
		"ns-a",
		map[string]interface{}{
			"image":   "gcr.io/google-containers/pause:3.2",
			"sidecar": "docker.io/library/alpine:edge",
		},
		&helm.ChartMetadata{Name: "chart3", Version: "1.0"},
	)
	mockHelmClient.On("GetChartFromRelease", mock.Anything, "release-1", "ns-a").Return(&helm.ChartMetadata{Name: "chart1", Version: "1.0"}, nil)
	mockHelmClient.On("GetChartFromRelease", mock.Anything, "release-2", "ns-b").Return(&helm.ChartMetadata{Name: "chart2", Version: "1.0"}, nil)
	mockHelmClient.On("GetChartFromRelease", mock.Anything, "release-3", "ns-a").Return(&helm.ChartMetadata{Name: "chart3", Version: "1.0"}, nil)

	// --- Inject Mocks ---
	originalHelmClientFactory := helmClientFactory
	helmClientFactory = func() (helm.ClientInterface, error) {
		return mockHelmClient, nil
	}
	defer func() { helmClientFactory = originalHelmClientFactory }()

	originalHelmAdapterFactory := helmAdapterFactory
	helmAdapterFactory = func() (*helm.Adapter, error) {
		adapter := helm.NewAdapter(mockHelmClient, AppFs, true)
		return adapter, nil
	}
	defer func() { helmAdapterFactory = originalHelmAdapterFactory }()

	// --- Execute Command with --source-registries ---
	cmd := newInspectCmd()
	args := []string{
		"-A",
		"--generate-config-skeleton",
		"--output-file", "skeleton-filtered.yaml",
		"--overwrite-skeleton",
		"--source-registries", "docker.io", // Add the filter flag
	}
	cmd.SetArgs(args)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)

	err := cmd.Execute()
	require.NoError(t, err, "Command execution failed unexpectedly. Stdout: %s, Stderr: %s", out.String(), errOut.String())

	// --- Verify Output ---
	skeletonPath := "skeleton-filtered.yaml"
	exists, err := afero.Exists(AppFs, skeletonPath)
	require.NoError(t, err)
	require.True(t, exists, "Expected skeleton file '%s' was not created", skeletonPath)

	contentBytes, err := afero.ReadFile(AppFs, skeletonPath)
	require.NoError(t, err)
	content := string(contentBytes)

	// Assert that ALL unique registries are present, despite the filter
	assert.Contains(t, content, "source: docker.io", "Skeleton missing docker.io")
	assert.Contains(t, content, "source: quay.io", "Skeleton missing quay.io")
	assert.Contains(t, content, "source: gcr.io", "Skeleton missing gcr.io")

	// Assert that registries are sorted
	dockerIndex := bytes.Index(contentBytes, []byte("source: docker.io"))
	gcrIndex := bytes.Index(contentBytes, []byte("source: gcr.io"))
	quayIndex := bytes.Index(contentBytes, []byte("source: quay.io"))
	assert.True(t, dockerIndex < gcrIndex, "Registries not sorted correctly (docker.io vs gcr.io)")
	assert.True(t, gcrIndex < quayIndex, "Registries not sorted correctly (gcr.io vs quay.io)")

	// --- Verify Mock Calls ---
	mockHelmClient.AssertCalled(t, "ListReleases", mock.Anything, true)
	mockHelmClient.AssertCalled(t, "GetReleaseValues", mock.Anything, "release-1", "ns-a")
	mockHelmClient.AssertCalled(t, "GetChartFromRelease", mock.Anything, "release-1", "ns-a")
	mockHelmClient.AssertCalled(t, "GetReleaseValues", mock.Anything, "release-2", "ns-b")
	mockHelmClient.AssertCalled(t, "GetChartFromRelease", mock.Anything, "release-2", "ns-b")
	mockHelmClient.AssertCalled(t, "GetReleaseValues", mock.Anything, "release-3", "ns-a")
	mockHelmClient.AssertCalled(t, "GetChartFromRelease", mock.Anything, "release-3", "ns-a")
}

// TestInspectAllNamespacesStandardOutput verifies that `inspect -A` produces correct
// standard YAML output containing releases from multiple namespaces.
func TestInspectAllNamespacesStandardOutput(t *testing.T) {
	cleanup := setupInspectTest(t)
	defer cleanup()

	// --- Mock Helm Interaction ---
	mockReleases := []*helm.ReleaseElement{
		{Name: "release-1", Namespace: "ns-a"}, // docker.io
		{Name: "release-2", Namespace: "ns-b"}, // quay.io
		{Name: "release-3", Namespace: "ns-a"}, // gcr.io
	}
	mockHelmClient := &helm.MockHelmClient{}
	mockHelmClient.On("ListReleases", mock.Anything, true).Return(mockReleases, nil)
	mockHelmClient.SetupMockRelease("release-1", "ns-a", map[string]interface{}{"image": "docker.io/library/nginx:latest"}, &helm.ChartMetadata{Name: "chart1", Version: "1.0"})
	mockHelmClient.SetupMockRelease("release-2", "ns-b", map[string]interface{}{"app": "quay.io/prometheus/node-exporter:v1"}, &helm.ChartMetadata{Name: "chart2", Version: "1.0"})
	mockHelmClient.SetupMockRelease("release-3", "ns-a", map[string]interface{}{"job": map[string]interface{}{"image": "gcr.io/google-containers/pause:3.2"}}, &helm.ChartMetadata{Name: "chart3", Version: "1.0"})
	mockHelmClient.On("GetChartFromRelease", mock.Anything, mock.Anything, mock.Anything).Return(&helm.ChartMetadata{Name: "mock-chart", Version: "1.0"}, nil) // Simplified GetChart mock

	// --- Inject Mocks ---
	originalHelmClientFactory := helmClientFactory
	helmClientFactory = func() (helm.ClientInterface, error) { return mockHelmClient, nil }
	defer func() { helmClientFactory = originalHelmClientFactory }()
	originalHelmAdapterFactory := helmAdapterFactory
	helmAdapterFactory = func() (*helm.Adapter, error) { return helm.NewAdapter(mockHelmClient, AppFs, true), nil }
	defer func() { helmAdapterFactory = originalHelmAdapterFactory }()

	// --- Execute Command ---
	cmd := newInspectCmd()
	args := []string{
		"-A",
		"-o", "yaml", // Explicitly request YAML output
	}
	cmd.SetArgs(args)
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	err := cmd.Execute()
	require.NoError(t, err, "Command execution failed unexpectedly. Stderr: %s", errBuf.String())

	// --- Verify Output ---
	outputYAML := outBuf.String()

	// Parse the YAML output
	var result struct {
		Releases []ReleaseAnalysisResult `yaml:"releases"`
		Skipped  []string                `yaml:"skipped,omitempty"`
	}
	err = yaml.Unmarshal([]byte(outputYAML), &result)
	require.NoError(t, err, "Failed to unmarshal YAML output")
	require.Len(t, result.Releases, 3, "Expected 3 releases in the output")

	// Check specific releases and their images
	foundR1, foundR2, foundR3 := false, false, false
	for _, rel := range result.Releases {
		switch rel.ReleaseName {
		case "release-1":
			assert.Equal(t, "ns-a", rel.Namespace)
			require.NotEmpty(t, rel.Analysis.Images, "Release 1 should have images")
			assert.Equal(t, "docker.io", rel.Analysis.Images[0].Registry)
			assert.Equal(t, "library/nginx", rel.Analysis.Images[0].Repository)
			foundR1 = true
		case "release-2":
			assert.Equal(t, "ns-b", rel.Namespace)
			require.NotEmpty(t, rel.Analysis.Images, "Release 2 should have images")
			assert.Equal(t, "quay.io", rel.Analysis.Images[0].Registry)
			assert.Equal(t, "prometheus/node-exporter", rel.Analysis.Images[0].Repository)
			foundR2 = true
		case "release-3":
			assert.Equal(t, "ns-a", rel.Namespace)
			require.NotEmpty(t, rel.Analysis.Images, "Release 3 should have images")
			assert.Equal(t, "gcr.io", rel.Analysis.Images[0].Registry)
			assert.Equal(t, "google-containers/pause", rel.Analysis.Images[0].Repository)
			foundR3 = true
		}
	}
	assert.True(t, foundR1, "Release-1 not found in output")
	assert.True(t, foundR2, "Release-2 not found in output")
	assert.True(t, foundR3, "Release-3 not found in output")
}

// TestInspectAllNamespacesWithFilterStandardOutput verifies that `inspect -A --source-registries`
// correctly filters the standard YAML output across multiple namespaces.
func TestInspectAllNamespacesWithFilterStandardOutput(t *testing.T) {
	cleanup := setupInspectTest(t)
	defer cleanup()

	// --- Mock Helm Interaction ---
	mockReleases := []*helm.ReleaseElement{
		{Name: "release-a1", Namespace: "ns-a"}, // docker.io, quay.io
		{Name: "release-b1", Namespace: "ns-b"}, // docker.io, gcr.io
		{Name: "release-c1", Namespace: "ns-c"}, // quay.io only
	}
	mockHelmClient := &helm.MockHelmClient{}
	mockHelmClient.On("ListReleases", mock.Anything, true).Return(mockReleases, nil)
	mockHelmClient.SetupMockRelease("release-a1", "ns-a",
		map[string]interface{}{
			"image1": "docker.io/image-a:1",
			"image2": "quay.io/image-q:1",
		},
		&helm.ChartMetadata{Name: "charta", Version: "1.0"},
	)
	mockHelmClient.SetupMockRelease("release-b1", "ns-b",
		map[string]interface{}{
			"main": "docker.io/image-b:2",
			"side": "gcr.io/image-g:2",
		},
		&helm.ChartMetadata{Name: "chartb", Version: "1.0"},
	)
	mockHelmClient.SetupMockRelease("release-c1", "ns-c",
		map[string]interface{}{"app": "quay.io/image-c:3"},
		&helm.ChartMetadata{Name: "chartc", Version: "1.0"},
	)
	mockHelmClient.On("GetChartFromRelease", mock.Anything, mock.Anything, mock.Anything).Return(&helm.ChartMetadata{Name: "mock-chart", Version: "1.0"}, nil)

	// --- Inject Mocks ---
	originalHelmClientFactory := helmClientFactory
	helmClientFactory = func() (helm.ClientInterface, error) { return mockHelmClient, nil }
	defer func() { helmClientFactory = originalHelmClientFactory }()
	originalHelmAdapterFactory := helmAdapterFactory
	helmAdapterFactory = func() (*helm.Adapter, error) { return helm.NewAdapter(mockHelmClient, AppFs, true), nil }
	defer func() { helmAdapterFactory = originalHelmAdapterFactory }()

	// --- Execute Command ---
	cmd := newInspectCmd()
	args := []string{
		"-A",
		"-o", "yaml",
		"--source-registries", "docker.io", // Filter for docker.io
	}
	cmd.SetArgs(args)
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	err := cmd.Execute()
	require.NoError(t, err, "Command execution failed unexpectedly. Stderr: %s", errBuf.String())

	// --- Verify Output ---
	outputYAML := outBuf.String()

	// Parse the YAML output
	var result struct {
		Releases []ReleaseAnalysisResult `yaml:"releases"`
		Skipped  []string                `yaml:"skipped,omitempty"`
	}
	err = yaml.Unmarshal([]byte(outputYAML), &result)
	require.NoError(t, err, "Failed to unmarshal YAML output")

	// Expecting only releases A1 and B1 (filtered)
	require.Len(t, result.Releases, 2, "Expected only 2 releases after filtering")

	foundA1, foundB1 := false, false
	for _, rel := range result.Releases {
		switch rel.ReleaseName {
		case "release-a1":
			assert.Equal(t, "ns-a", rel.Namespace)
			require.Len(t, rel.Analysis.Images, 1, "Release A1 should have exactly 1 image after filtering")
			assert.Equal(t, "docker.io", rel.Analysis.Images[0].Registry)
			assert.Equal(t, "image-a", rel.Analysis.Images[0].Repository)
			foundA1 = true
		case "release-b1":
			assert.Equal(t, "ns-b", rel.Namespace)
			require.Len(t, rel.Analysis.Images, 1, "Release B1 should have exactly 1 image after filtering")
			assert.Equal(t, "docker.io", rel.Analysis.Images[0].Registry)
			assert.Equal(t, "image-b", rel.Analysis.Images[0].Repository)
			foundB1 = true
		default:
			t.Errorf("Unexpected release found in filtered output: %s/%s", rel.Namespace, rel.ReleaseName)
		}
	}
	assert.True(t, foundA1, "Release-a1 not found in filtered output")
	assert.True(t, foundB1, "Release-b1 not found in filtered output")
}

// TestRunInspect tests the RunInspect function.
