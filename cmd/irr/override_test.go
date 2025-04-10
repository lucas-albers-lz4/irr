package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	// Use testify for assertions
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	// Need analysis types for mocking generator return value
	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/override"
	registry "github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"

	// Need cobra for command execution simulation
	"github.com/spf13/afero"

	"github.com/lalbers/irr/pkg/chart"
)

// IMPORTANT: When writing tests that use an in-memory filesystem (afero.MemMapFs),
// the following considerations must be taken into account:
//
// 1. Set AppFs = fs where fs is your in-memory filesystem
// 2. Always restore AppFs in a defer function: defer func() { AppFs = afero.NewOsFs() }()
// 3. Explicitly set "--registry-file" to "" in command arguments to prevent the root command
//    from accidentally resetting AppFs to a real filesystem.
//
// If AppFs gets reset during command execution, file operations will not use the in-memory
// filesystem, causing tests to fail.

// mockGenerator implements Generator interface for testing
type mockGenerator struct {
	mock.Mock
	OverrideFileToReturn *override.File
	ErrToReturn          error
}

func (m *mockGenerator) Generate() (*override.File, error) {
	args := m.Called()
	// Check error first!
	if err := args.Error(1); err != nil {
		return nil, fmt.Errorf("mock generator error: %w", err)
	}
	// Check for nil result only if no error occurred
	result := args.Get(0)
	if result == nil {
		// If err was nil but result is nil, this is unexpected
		return nil, fmt.Errorf("unexpected nil result from mock generator when no error was returned")
	}
	overrideFile, ok := result.(*override.File)
	if !ok {
		return nil, fmt.Errorf("mock generator returned invalid type: expected *override.File, got %T", result)
	}
	return overrideFile, nil
}

// Local ChartData/Metadata definitions needed for mockLoader
type ChartData struct {
	Metadata *ChartMetadata
	Values   map[string]interface{}
}
type ChartMetadata struct {
	Name    string
	Version string
}

// setupDryRunTestEnvironment configures the environment for dry-run tests
func setupDryRunTestEnvironment(t *testing.T, args []string) (updatedArgs []string, cleanup func()) {
	// Create a temporary directory using afero in-memory FS
	AppFs = afero.NewMemMapFs() // Use MemMapFs for this test
	chartDir, err := afero.TempDir(AppFs, "", "dryrun-chart-")
	require.NoError(t, err, "Failed to create temp chart directory")

	// Create dummy chart files and check for errors
	err = createDummyChart(AppFs, chartDir)
	require.NoError(t, err, "Failed to create dummy chart files")

	// Create a mock Generator that returns a simple override file
	overrideFile := &override.File{
		Overrides: map[string]interface{}{"image": map[string]interface{}{"repository": "target.io/repo", "tag": "newtag"}},
	}
	mockGen := &mockGenerator{}
	mockGen.On("Generate").Return(overrideFile, nil)

	// Save original factory and set up mock
	origFactory := currentGeneratorFactory
	// Define the factory function with only the necessary parameters
	// The parameters are unused because we always return the mock.
	currentGeneratorFactory = func(
		_ string, // chartPath
		_ string, // targetRegistry
		_ []string, // sourceRegistries
		_ []string, // excludeRegistries
		_ strategy.PathStrategy,
		_ *registry.Mappings,
		_ map[string]string, // configMappings
		_ bool, // strict
		_ int, // threshold
		_ analysis.ChartLoader,
		_ []string, // includePatterns
		_ []string, // excludePatterns
		_ []string, // knownPaths
	) GeneratorInterface {
		return mockGen
	}

	// Prepend the chart path argument dynamically
	updatedArgs = append([]string{"--chart-path", chartDir}, args...)

	// Create cleanup function
	cleanup = func() {
		AppFs = afero.NewOsFs()               // Restore global FS
		currentGeneratorFactory = origFactory // Restore original factory
	}

	return updatedArgs, cleanup
}

// assertExitCodeError checks if an error matches the expected ExitCodeError
func assertExitCodeError(t *testing.T, err error, expectedError *exitcodes.ExitCodeError) {
	if expectedError == nil {
		assert.NoError(t, err)
		return
	}

	var exitErr *exitcodes.ExitCodeError
	if assert.ErrorAs(t, err, &exitErr) {
		assert.Equal(t, expectedError.Code, exitErr.Code)
		// Use Contains because the actual error might have more wrapping
		assert.Contains(t, exitErr.Error(), expectedError.Error())
	}
}

func TestOverrideCmdArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedError *exitcodes.ExitCodeError
	}{
		{
			name: "all required flags present but invalid path",
			args: []string{
				"--chart-path", "/nonexistent",
				"--target-registry", "target.io",
				"--source-registries", "source.io",
				"--registry-file", "", // Explicitly set registry-file to empty string
			},
			expectedError: &exitcodes.ExitCodeError{
				Code: 11, // Using explicit exit code value to match what's being returned
				Err:  errors.New("failed to process chart: error analyzing chart /nonexistent: failed to load chart: failed to load chart from /nonexistent: stat /nonexistent: no such file or directory"),
			},
		},
		{
			name: "valid flags with dry run",
			args: []string{
				"--target-registry", "target.io",
				"--source-registries", "source.io",
				"--dry-run",
				"--registry-file", "", // Explicitly set registry-file to empty string
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newOverrideCmd()
			currentArgs := tt.args

			// Special setup for dry run test to avoid path issues
			var cleanupFunc func()
			if tt.name == "valid flags with dry run" {
				currentArgs, cleanupFunc = setupDryRunTestEnvironment(t, tt.args)
				defer cleanupFunc()
			}

			cmd.SetArgs(currentArgs)
			err := cmd.Execute()

			assertExitCodeError(t, err, tt.expectedError)
		})
	}
}

func TestOverrideCommand_MissingFlags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedError *exitcodes.ExitCodeError
	}{
		{
			name: "Missing all",
			args: []string{},
			expectedError: &exitcodes.ExitCodeError{
				Code: exitcodes.ExitMissingRequiredFlag,
				Err:  errors.New(`required flag(s) "chart-path", "source-registries", "target-registry" not set`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newOverrideCmd()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			var exitErr *exitcodes.ExitCodeError
			if assert.ErrorAs(t, err, &exitErr) {
				assert.Equal(t, tt.expectedError.Code, exitErr.Code)
				assert.Equal(t, tt.expectedError.Error(), exitErr.Error())
			}
		})
	}
}

func TestOverrideCommand_GeneratorError(t *testing.T) {
	mockGen := &mockGenerator{}
	mockGen.On("Generate").Return(nil, chart.ErrChartNotFound)

	oldFactory := currentGeneratorFactory
	defer func() { currentGeneratorFactory = oldFactory }()

	// Define the factory function with only the necessary parameters
	// The parameters are unused because we always return the mock.
	currentGeneratorFactory = func(
		_ string, // chartPath
		_ string, // targetRegistry
		_ []string, // sourceRegistries
		_ []string, // excludeRegistries
		_ strategy.PathStrategy,
		_ *registry.Mappings,
		_ map[string]string, // configMappings
		_ bool, // strict
		_ int, // threshold
		_ analysis.ChartLoader,
		_ []string, // includePatterns
		_ []string, // excludePatterns
		_ []string, // knownPaths
	) GeneratorInterface {
		return mockGen
	}

	cmd := newOverrideCmd()
	cmd.SetArgs([]string{
		"--chart-path", "./fake/chart",
		"--target-registry", "target.io",
		"--source-registries", "source.io",
	})

	err := cmd.Execute()
	// Expect the wrapped error message from the mock generator
	expectedErrMsg := "mock generator error: chart not found"

	var exitErr *exitcodes.ExitCodeError
	if assert.ErrorAs(t, err, &exitErr) {
		assert.Equal(t, exitcodes.ExitChartParsingError, exitErr.Code)
		assert.Contains(t, exitErr.Error(), expectedErrMsg)
	}
}

// setupTestEnvironmentVars sets up environment variables for a test and returns a cleanup function
func setupTestEnvironmentVars(t *testing.T, envVars map[string]string) func() {
	if len(envVars) == 0 {
		return func() {}
	}

	originalEnv := make(map[string]string)
	for k, v := range envVars {
		originalEnv[k] = os.Getenv(k) // Store original value
		err := os.Setenv(k, v)
		require.NoErrorf(t, err, "failed to set environment variable %s=%s", k, v)
	}

	// Return cleanup function that restores original environment
	return func() {
		for k, originalValue := range originalEnv {
			if originalValue == "" {
				if err := os.Unsetenv(k); err != nil {
					t.Logf("Warning: Failed to unset environment variable %s: %v", k, err)
				}
			} else {
				if err := os.Setenv(k, originalValue); err != nil {
					t.Logf("Warning: Failed to restore environment variable %s: %v", k, err)
				}
			}
		}
	}
}

// setupTestMockGenerator sets up a mock generator for tests
func setupTestMockGenerator(_ *testing.T, mockGeneratorFunc func() (*override.File, error)) func() {
	if mockGeneratorFunc == nil {
		return func() {}
	}

	mockGen := &mockGenerator{}
	result, err := mockGeneratorFunc()
	mockGen.On("Generate").Return(result, err)

	originalFactory := currentGeneratorFactory // Store original to restore later
	// Define the factory function with only the necessary parameters
	// The parameters are unused because we always return the mock.
	currentGeneratorFactory = func(
		_ string, // chartPath
		_ string, // targetRegistry
		_ []string, // sourceRegistries
		_ []string, // excludeRegistries
		_ strategy.PathStrategy,
		_ *registry.Mappings,
		_ map[string]string, // configMappings
		_ bool, // strict
		_ int, // threshold
		_ analysis.ChartLoader,
		_ []string, // includePatterns
		_ []string, // excludePatterns
		_ []string, // knownPaths
	) GeneratorInterface {
		return mockGen
	}

	// Return cleanup function that restores original factory
	return func() {
		currentGeneratorFactory = originalFactory
	}
}

// setupOverrideTestEnvironment encapsulates the common setup logic for override command tests.
// It returns the test directory path, the potentially modified arguments, and a cleanup function.
func setupOverrideTestEnvironment(t *testing.T, tt *struct {
	name              string
	args              []string
	mockGeneratorFunc func() (*override.File, error)
	expectErr         bool
	stdOutContains    string
	stdErrContains    string
	setupEnv          map[string]string
	postCheck         func(t *testing.T, testDir string)
}) (testDir string, currentArgs []string, cleanup func()) {
	testDir = t.TempDir()
	AppFs = afero.NewOsFs() // Use real FS for file operations
	currentArgs = make([]string, len(tt.args))
	copy(currentArgs, tt.args)

	// If test case has output file, modify args to use testDir
	if tt.name == "success with output file (flow check)" { // TODO: Make this condition less brittle
		outputPath := filepath.Join(testDir, "test-output.yaml")
		currentArgs = append(currentArgs, "-o", outputPath)
	}

	// Setup mock generator and get cleanup function
	generatorCleanup := setupTestMockGenerator(t, tt.mockGeneratorFunc)

	// Setup environment variables and get cleanup function
	envCleanup := setupTestEnvironmentVars(t, tt.setupEnv)

	// Create combined cleanup function
	cleanup = func() {
		generatorCleanup()
		envCleanup()
		if err := os.RemoveAll(testDir); err != nil {
			t.Logf("Warning: Failed to cleanup test directory %s: %v", testDir, err)
		}
	}

	return testDir, currentArgs, cleanup
}

// createSuccessTestCase creates a test case for successful command execution
func createSuccessTestCase(args []string, overrideValues map[string]interface{}, expectedOutput string) struct {
	name              string
	args              []string
	mockGeneratorFunc func() (*override.File, error)
	expectErr         bool
	stdOutContains    string
	stdErrContains    string
	setupEnv          map[string]string
	postCheck         func(t *testing.T, testDir string)
} {
	return struct {
		name              string
		args              []string
		mockGeneratorFunc func() (*override.File, error)
		expectErr         bool
		stdOutContains    string
		stdErrContains    string
		setupEnv          map[string]string
		postCheck         func(t *testing.T, testDir string)
	}{
		name: "success execution to stdout",
		args: args,
		mockGeneratorFunc: func() (*override.File, error) {
			return &override.File{
				Overrides: overrideValues,
			}, nil
		},
		expectErr:      false,
		stdOutContains: expectedOutput,
		stdErrContains: "",
		setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
		postCheck:      nil,
	}
}

// createErrorTestCase creates a test case for command execution that should result in an error
func createErrorTestCase(args []string, errorMsg string) struct {
	name              string
	args              []string
	mockGeneratorFunc func() (*override.File, error)
	expectErr         bool
	stdOutContains    string
	stdErrContains    string
	setupEnv          map[string]string
	postCheck         func(t *testing.T, testDir string)
} {
	return struct {
		name              string
		args              []string
		mockGeneratorFunc func() (*override.File, error)
		expectErr         bool
		stdOutContains    string
		stdErrContains    string
		setupEnv          map[string]string
		postCheck         func(t *testing.T, testDir string)
	}{
		name: "generator returns error",
		args: args,
		mockGeneratorFunc: func() (*override.File, error) {
			return nil, fmt.Errorf("%s", errorMsg)
		},
		expectErr:      true,
		stdErrContains: "failed to process chart: " + errorMsg,
		setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
		postCheck:      nil,
	}
}

// createOutputFileTestCase creates a test case for checking dry run behavior with output file
func createOutputFileTestCase(args []string) struct {
	name              string
	args              []string
	mockGeneratorFunc func() (*override.File, error)
	expectErr         bool
	stdOutContains    string
	stdErrContains    string
	setupEnv          map[string]string
	postCheck         func(t *testing.T, testDir string)
} {
	return struct {
		name              string
		args              []string
		mockGeneratorFunc func() (*override.File, error)
		expectErr         bool
		stdOutContains    string
		stdErrContains    string
		setupEnv          map[string]string
		postCheck         func(t *testing.T, testDir string)
	}{
		name: "success with output file (flow check)",
		args: args,
		mockGeneratorFunc: func() (*override.File, error) {
			return &override.File{
				Overrides: map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "mock-target.com/dockerio/nginx",
					},
				},
			}, nil
		},
		expectErr:      false,
		stdOutContains: "--- Dry Run: Generated Overrides ---",
		stdErrContains: "",
		setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
		postCheck: func(t *testing.T, testDir string) {
			outputPath := filepath.Join(testDir, "test-output.yaml")
			t.Logf("Checking if override file exists: %s", outputPath)
			_, err := os.Stat(outputPath)
			assert.True(t, os.IsNotExist(err), "Override file should NOT exist in dry run mode")
		},
	}
}

// defineOverrideCmdExecutionTests returns test cases for TestOverrideCmdExecution
func defineOverrideCmdExecutionTests() []struct {
	name              string
	args              []string
	mockGeneratorFunc func() (*override.File, error)
	expectErr         bool
	stdOutContains    string
	stdErrContains    string
	setupEnv          map[string]string
	postCheck         func(t *testing.T, testDir string)
} {
	defaultArgs := []string{
		"override",
		"--chart-path", "./fake/chart",
		"--target-registry", "mock-target.com",
		"--source-registries", "docker.io",
	}

	// Create test cases using helper functions
	successCase := createSuccessTestCase(
		defaultArgs,
		map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "mock-target.com/dockerio/nginx",
			},
		},
		"repository: mock-target.com/dockerio/nginx",
	)

	dryRunCase := createSuccessTestCase(
		append(defaultArgs, "--dry-run"),
		map[string]interface{}{
			"image": "dry-run-image",
		},
		"image: dry-run-image",
	)
	dryRunCase.name = "success with dry run"

	errorCase := createErrorTestCase(defaultArgs, "mock generator error")

	outputFileCase := createOutputFileTestCase(defaultArgs)

	return []struct {
		name              string
		args              []string
		mockGeneratorFunc func() (*override.File, error)
		expectErr         bool
		stdOutContains    string
		stdErrContains    string
		setupEnv          map[string]string
		postCheck         func(t *testing.T, testDir string)
	}{
		successCase,
		dryRunCase,
		errorCase,
		outputFileCase,
	}
}

func TestOverrideCmdExecution(t *testing.T) {
	// Get test cases from the helper function
	tests := defineOverrideCmdExecutionTests()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment using the helper
			testDir, currentArgs, cleanup := setupOverrideTestEnvironment(t, &struct {
				name              string
				args              []string
				mockGeneratorFunc func() (*override.File, error)
				expectErr         bool
				stdOutContains    string
				stdErrContains    string
				setupEnv          map[string]string
				postCheck         func(t *testing.T, testDir string)
			}{
				name:              tt.name,
				args:              tt.args,
				mockGeneratorFunc: tt.mockGeneratorFunc,
				expectErr:         tt.expectErr,
				stdOutContains:    tt.stdOutContains,
				stdErrContains:    tt.stdErrContains,
				setupEnv:          tt.setupEnv,
				postCheck:         tt.postCheck,
			})
			defer cleanup() // Ensure test directory is cleaned up

			// Execute command
			rootCmd := getRootCmd()
			output, err := executeCommand(rootCmd, currentArgs...)

			// Assertions using the helper function
			assertOverrideTestOutcome(t, err, output, tt.expectErr, tt.stdOutContains, tt.stdErrContains)

			// Run post-check if defined
			if tt.postCheck != nil {
				tt.postCheck(t, testDir)
			}
		})
	}
}

// assertOverrideTestOutcome contains the common assertion logic for override command tests.
func assertOverrideTestOutcome(t *testing.T, err error, output string, expectErr bool, stdOutContains, stdErrContains string) {
	t.Helper() // Mark this as a helper function for better test failure reporting

	if expectErr {
		assert.Error(t, err, "Expected an error")
		if stdErrContains != "" {
			assert.Contains(t, err.Error(), stdErrContains, "error message should contain expected text")
		}
	} else {
		assert.NoError(t, err, "Did not expect an error")
		if stdOutContains != "" {
			assert.Contains(t, output, stdOutContains, "output should contain expected text")
		}
	}
}

// TestOverrideCommand_Success tests the successful execution of the override command
func TestOverrideCommand_Success(t *testing.T) {
	// Print test identifier to help debug
	fmt.Println("=== Starting TestOverrideCommand_Success ===")

	// Debug: Print environment variables
	fmt.Println("DEBUG environment:", os.Getenv("DEBUG"))

	// Set up memory filesystem with proper cleanup
	fs, chartDir, cleanup := setupMemoryFSContext(t)
	defer cleanup() // Ensure we clean up even if the test fails

	// Create the test chart
	require.NoError(t, createDummyChart(fs, chartDir))

	// Create a mock generator that we fully control
	mockGen := &mockGenerator{}
	overrideFile := &override.File{
		ChartPath: chartDir,
		Overrides: map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "target.io",
				"repository": "library/nginx",
				"tag":        "1.21",
			},
		},
	}
	mockGen.On("Generate").Return(overrideFile, nil)

	// Save the original factory and restore it after the test
	originalFactory := currentGeneratorFactory
	defer func() { currentGeneratorFactory = originalFactory }()

	// Replace the generator factory with our mock
	currentGeneratorFactory = func(_, _ string, _, _ []string, _ strategy.PathStrategy, _ *registry.Mappings, _ map[string]string, _ bool, _ int, _ analysis.ChartLoader, _, _, _ []string) GeneratorInterface {
		return mockGen
	}

	// Set up command arguments
	outputFile := filepath.Join(chartDir, "test-overrides.yaml")
	args := []string{
		"override",
		"--chart-path", chartDir,
		"--source-registries", "source.io",
		"--target-registry", "target.io",
		"--output-file", outputFile,
		"--registry-file", "",
		"--dry-run=false", // Explicitly disable dry-run mode
	}
	fmt.Printf("DEBUG command args: %v\n", args)
	fmt.Printf("DEBUG output file: %s\n", outputFile)

	// Log the type and state of AppFs and fs
	t.Logf("AppFs type: %T", AppFs)
	t.Logf("fs type: %T", fs)
	t.Logf("Are AppFs and fs the same: %v", AppFs == fs)

	// Direct file write test
	testContent := []byte("test content")
	testFile := filepath.Join(chartDir, "test-direct.txt")
	err := afero.WriteFile(AppFs, testFile, testContent, FilePermissions)
	require.NoError(t, err)
	exists, err := afero.Exists(fs, testFile)
	require.NoError(t, err, "Failed to check if test file exists")
	assert.True(t, exists, "Direct file write test failed")

	// Execute the command
	cmd := getRootCmd()
	output, err := executeCommand(cmd, args...)
	t.Logf("Command output: %s", output)
	t.Logf("Command error: %v", err)
	require.NoError(t, err, "Command execution failed. Output:\n%s", output)

	// Verify the mock was called
	mockGen.AssertExpectations(t)

	// Check if the output file was created
	exists, err = afero.Exists(fs, outputFile)
	require.NoError(t, err, "Failed to check if output file exists")

	// Debug: list all files in the filesystem
	allFiles := []string{}
	err = afero.Walk(fs, "/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			allFiles = append(allFiles, path)
		}
		return nil
	})
	require.NoError(t, err, "Failed to walk filesystem")
	t.Logf("Files in filesystem after command: %v", allFiles)

	assert.True(t, exists, "Output file was not created")

	// Check file content
	content, err := afero.ReadFile(fs, outputFile)
	require.NoError(t, err, "Failed to read output file")
	assert.Contains(t, string(content), "registry: target.io", "Output file missing expected content")
}

// setupDryRunTest prepares the test environment for dry run testing
func setupDryRunTest(t *testing.T) (fs afero.Fs, chartDir string, cleanup func(), overrideFile *override.File) {
	fs = afero.NewMemMapFs()
	AppFs = fs
	chartDir = t.TempDir()
	require.NoError(t, fs.MkdirAll(chartDir, 0o755), "Failed to create chart directory")

	// Create test override file
	overrideFile = &override.File{
		Overrides: map[string]interface{}{
			"global": map[string]interface{}{
				"imageRegistry": "target.io",
			},
			"image": map[string]interface{}{
				"registry":   "target.io",
				"repository": "library/nginx",
				"tag":        "latest",
			},
		},
	}

	// Save original factory
	originalFactory := currentGeneratorFactory

	// Create cleanup function
	cleanup = func() {
		currentGeneratorFactory = originalFactory
	}

	return fs, chartDir, cleanup, overrideFile
}

// setupMockGenerator sets up the mock generator and returns a cleanup function
func setupMockGenerator(_ *testing.T, overrideFile *override.File) func() {
	originalFactory := currentGeneratorFactory
	currentGeneratorFactory = func(
		_ string, _ string,
		_ []string, _ []string,
		_ strategy.PathStrategy,
		_ *registry.Mappings,
		_ map[string]string,
		_ bool,
		_ int,
		_ analysis.ChartLoader,
		_ []string, _ []string, _ []string,
	) GeneratorInterface {
		mock := &mockGenerator{}
		mock.On("Generate").Return(overrideFile, nil)
		return mock
	}
	return func() { currentGeneratorFactory = originalFactory }
}

// assertDryRunOutput verifies the output of a dry run command contains expected content
func assertDryRunOutput(t *testing.T, output string) {
	assert.Contains(t, output, "--- Dry Run: Generated Overrides ---", "missing dry run header")
	assert.Contains(t, output, "registry: target.io", "missing registry override")
	assert.Contains(t, output, "repository: library/nginx", "missing repository override")
	assert.Contains(t, output, "tag: latest", "missing tag override")
	assert.Contains(t, output, "--- End Dry Run ---", "missing dry run footer")
}

// assertNoOutputFile verifies that no output file was created in dry run mode
func assertNoOutputFile(t *testing.T, fs afero.Fs, outputFile string) {
	exists, err := afero.Exists(fs, outputFile)
	require.NoError(t, err, "Failed to check if output file exists")
	assert.False(t, exists, "Output file should not be created in dry run mode")
}

func TestOverrideCommand_DryRun(t *testing.T) {
	// Set up test environment
	fs, chartDir, cleanup, overrideFile := setupDryRunTest(t)
	defer cleanup() // Ensure we clean up even if the test fails

	// Set up mock generator
	generatorCleanup := setupMockGenerator(t, overrideFile)
	defer generatorCleanup()

	// Test with output file
	outputFile := filepath.Join(chartDir, "test-dryrun.yaml")
	args := []string{
		"override",
		"--chart-path", chartDir,
		"--target-registry", "target.io",
		"--source-registries", "source.io",
		"--output-file", outputFile,
		"--dry-run",
		"--registry-file", "", // Explicitly set registry-file to empty string
	}

	cmd := getRootCmd()
	output, err := executeCommand(cmd, args...)
	require.NoError(t, err, "Dry run command failed")

	// Assert dry run output and no file creation
	assertDryRunOutput(t, output)
	assertNoOutputFile(t, fs, outputFile)

	// Test without output file
	args = []string{
		"override",
		"--chart-path", chartDir,
		"--target-registry", "target.io",
		"--source-registries", "source.io",
		"--dry-run",
		"--registry-file", "", // Explicitly set registry-file to empty string
	}

	output, err = executeCommand(cmd, args...)
	require.NoError(t, err, "Dry run command without output file failed")

	// Assert basic dry run output is still correct
	assert.Contains(t, output, "--- Dry Run: Generated Overrides ---", "missing dry run header")
	assert.Contains(t, output, "registry: target.io", "missing registry override")
	assert.Contains(t, output, "--- End Dry Run ---", "missing dry run footer")
}
