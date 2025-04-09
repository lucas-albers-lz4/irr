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

// mockGenerator implements Generator interface for testing
type mockGenerator struct {
	mock.Mock
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
			},
			expectedError: &exitcodes.ExitCodeError{
				Code: exitcodes.ExitImageProcessingError,
				Err:  errors.New("error analyzing chart /nonexistent: failed to load chart: helm loader failed for path '/nonexistent': stat /nonexistent: no such file or directory"),
			},
		},
		{
			name: "valid flags with dry run",
			args: []string{
				"--target-registry", "target.io",
				"--source-registries", "source.io",
				"--dry-run",
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
				var fs afero.Fs
				var chartDir string
				fs, chartDir = setupTestFS(t) // Use helper
				AppFs = fs                    // Set global FS for the command execution
				require.NoError(t, createDummyChart(fs, chartDir))
				// Prepend the chart path argument dynamically
				currentArgs = append([]string{"--chart-path", chartDir}, currentArgs...)
				cleanupFunc = func() {
					AppFs = afero.NewOsFs() // Restore global FS
				}
				defer cleanupFunc()
			}

			cmd.SetArgs(currentArgs)
			err := cmd.Execute()

			if tt.expectedError == nil {
				assert.NoError(t, err)
			} else {
				var exitErr *exitcodes.ExitCodeError
				if assert.ErrorAs(t, err, &exitErr) {
					assert.Equal(t, tt.expectedError.Code, exitErr.Code)
					// Use Contains because the actual error might have more wrapping
					assert.Contains(t, exitErr.Error(), tt.expectedError.Error())
				}
			}
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

	currentGeneratorFactory = func(chartPath, targetRegistry string, sourceRegistries, excludeRegistries []string, pathStrategy strategy.PathStrategy, mappings *registry.Mappings, strict bool, threshold int, loader analysis.ChartLoader, includePatterns, excludePatterns, knownPaths []string) GeneratorInterface {
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
		assert.Equal(t, exitcodes.ExitImageProcessingError, exitErr.Code)
		assert.Contains(t, exitErr.Error(), expectedErrMsg)
	}
}

// setupOverrideTestEnvironment encapsulates the common setup logic for override command tests.
// It returns the test directory path, the potentially modified arguments, and a cleanup function.
func setupOverrideTestEnvironment(t *testing.T, tt struct {
	name              string
	args              []string
	mockGeneratorFunc func() (*override.File, error)
	expectErr         bool
	stdOutContains    string
	stdErrContains    string
	setupEnv          map[string]string
	postCheck         func(t *testing.T, testDir string)
}) (string, []string, func()) {
	testDir := t.TempDir()
	AppFs = afero.NewOsFs() // Use real FS for file operations
	currentArgs := make([]string, len(tt.args))
	copy(currentArgs, tt.args)

	// If test case has output file, modify args to use testDir
	if tt.name == "success with output file (flow check)" { // TODO: Make this condition less brittle
		outputPath := filepath.Join(testDir, "test-output.yaml")
		currentArgs = append(currentArgs, "-o", outputPath)
	}

	// Setup mock generator
	if tt.mockGeneratorFunc != nil {
		mockGen := &mockGenerator{}
		result, err := tt.mockGeneratorFunc()
		// Handle potential error from the mock setup itself
		mockGen.On("Generate").Return(result, err)
		originalFactory := currentGeneratorFactory // Store original to restore later
		currentGeneratorFactory = func(
			chartPath, targetRegistry string,
			sourceRegistries, excludeRegistries []string,
			pathStrategy strategy.PathStrategy,
			mappings *registry.Mappings,
			strict bool,
			threshold int,
			loader analysis.ChartLoader,
			includePatterns, excludePatterns, knownPaths []string,
		) GeneratorInterface {
			return mockGen
		}
		// Ensure factory is restored even if test panics
		t.Cleanup(func() { currentGeneratorFactory = originalFactory })
	}

	// Setup environment variables
	if tt.setupEnv != nil {
		originalEnv := make(map[string]string)
		for k, v := range tt.setupEnv {
			originalEnv[k] = os.Getenv(k) // Store original value
			err := os.Setenv(k, v)
			require.NoErrorf(t, err, "failed to set environment variable %s=%s", k, v)
		}
		// Ensure env vars are restored even if test panics
		t.Cleanup(func() {
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
		})
	}

	// Cleanup function for the test directory
	cleanup := func() {
		if err := os.RemoveAll(testDir); err != nil {
			t.Logf("Warning: Failed to cleanup test directory %s: %v", testDir, err)
		}
	}

	return testDir, currentArgs, cleanup
}

func TestOverrideCmdExecution(t *testing.T) {
	// originalGeneratorFactory := currentGeneratorFactory // Managed by setup helper t.Cleanup
	// defer func() { currentGeneratorFactory = originalGeneratorFactory }() // Managed by setup helper t.Cleanup

	defaultArgs := []string{
		"override",
		"--chart-path", "./fake/chart",
		"--target-registry", "mock-target.com",
		"--source-registries", "docker.io",
	}

	tests := []struct {
		name              string
		args              []string
		mockGeneratorFunc func() (*override.File, error)
		expectErr         bool
		stdOutContains    string
		stdErrContains    string
		setupEnv          map[string]string
		postCheck         func(t *testing.T, testDir string)
	}{
		{
			name: "success execution to stdout",
			args: defaultArgs,
			mockGeneratorFunc: func() (*override.File, error) {
				return &override.File{
					Overrides: map[string]interface{}{"image": map[string]interface{}{"repository": "mock-target.com/dockerio/nginx"}},
				}, nil
			},
			expectErr:      false,
			stdOutContains: "repository: mock-target.com/dockerio/nginx",
			stdErrContains: "",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck:      nil,
		},
		{
			name: "success with dry run",
			args: append(defaultArgs, "--dry-run"),
			mockGeneratorFunc: func() (*override.File, error) {
				return &override.File{
					Overrides: map[string]interface{}{"image": "dry-run-image"},
				}, nil
			},
			expectErr:      false,
			stdOutContains: "image: dry-run-image",
			stdErrContains: "",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck:      nil,
		},
		{
			name: "generator returns error",
			args: defaultArgs,
			mockGeneratorFunc: func() (*override.File, error) {
				return nil, fmt.Errorf("mock generator error")
			},
			expectErr:      true,
			stdErrContains: "failed to process chart: mock generator error",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck:      nil,
		},
		{
			name: "success with output file (flow check)",
			args: defaultArgs,
			mockGeneratorFunc: func() (*override.File, error) {
				return &override.File{
					Overrides: map[string]interface{}{"image": map[string]interface{}{"repository": "mock-target.com/dockerio/nginx"}},
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment using the helper
			testDir, currentArgs, cleanup := setupOverrideTestEnvironment(t, tt)
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

func TestOverrideCommand_Success(t *testing.T) {
	fs, chartDir := setupTestFS(t)
	AppFs = fs // Ensure AppFs is set BEFORE executing the command
	require.NoError(t, createDummyChart(fs, chartDir))

	mockGen := &mockGenerator{}
	overrideFile := &override.File{
		ChartPath: chartDir,
		Overrides: map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "target.io",
				"repository": "source.io/library/nginx",
				"tag":        "1.21",
			},
		},
	}
	mockGen.On("Generate").Return(overrideFile, nil)

	originalFactory := currentGeneratorFactory
	currentGeneratorFactory = func(
		_ string, _ string, _ []string, _ []string,
		_ strategy.PathStrategy, _ *registry.Mappings, _ bool, _ int,
		_ analysis.ChartLoader,
		_ []string, _ []string, _ []string,
	) GeneratorInterface {
		return mockGen
	}
	defer func() { currentGeneratorFactory = originalFactory }()

	outputFile := filepath.Join(chartDir, "test-overrides.yaml")
	args := []string{
		"override",
		"--chart-path", chartDir,
		"--target-registry", "target.io",
		"--source-registries", "source.io",
		"--output-file", outputFile,
	}

	cmd := getRootCmd()
	output, err := executeCommand(cmd, args...)
	require.NoError(t, err, "Command execution failed. Output:\n%s", output)

	// Check if the output file was created
	exists, err := afero.Exists(fs, outputFile)
	require.NoError(t, err, "Failed to check if output file exists")
	assert.True(t, exists, "Output file was not created")

	// Check file content
	content, err := afero.ReadFile(fs, outputFile)
	require.NoError(t, err, "Failed to read output file")
	assert.Contains(t, string(content), "registry: target.io", "Output file missing expected content")
}

func TestOverrideCommand_DryRun(t *testing.T) {
	fs, chartDir := setupTestFS(t)
	AppFs = fs
	require.NoError(t, createDummyChart(fs, chartDir))

	mockGen := &mockGenerator{}
	overrideFile := &override.File{
		ChartPath: chartDir,
		Overrides: map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "target.io",
				"repository": "library/nginx",
				"tag":        "latest",
			},
			"sidecar": map[string]interface{}{
				"image": "target.io/library/busybox:latest",
			},
			"initContainer": map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "target.io",
					"repository": "library/alpine",
					"tag":        "3.14",
				},
			},
		},
	}
	mockGen.On("Generate").Return(overrideFile, nil)

	originalFactory := currentGeneratorFactory
	currentGeneratorFactory = func(
		_ string, _ string, _ []string, _ []string,
		_ strategy.PathStrategy, _ *registry.Mappings, _ bool, _ int,
		_ analysis.ChartLoader,
		_ []string, _ []string, _ []string,
	) GeneratorInterface {
		return mockGen
	}
	defer func() { currentGeneratorFactory = originalFactory }()

	outputFile := filepath.Join(chartDir, "test-dryrun.yaml")
	args := []string{
		"override",
		"--chart-path", chartDir,
		"--target-registry", "target.io",
		"--source-registries", "source.io",
		"--output-file", outputFile,
		"--dry-run",
	}

	cmd := getRootCmd()
	output, err := executeCommand(cmd, args...)
	require.NoError(t, err, "Dry run command failed. Output:\n%s", output)

	// Assert dry run output structure
	assert.Contains(t, output, "--- Dry Run: Generated Overrides ---", "missing dry run header")
	assert.Contains(t, output, "registry: target.io", "missing registry override")
	assert.Contains(t, output, "repository: library/nginx", "missing repository override")
	assert.Contains(t, output, "tag: latest", "missing tag override")
	assert.Contains(t, output, "--- End Dry Run ---", "missing dry run footer")

	// Assert file was NOT created
	exists, err := afero.Exists(fs, outputFile)
	require.NoError(t, err, "Failed to check if output file exists")
	assert.False(t, exists, "Output file should not be created in dry run mode")

	// Test without output file
	args = []string{
		"override",
		"--chart-path", chartDir,
		"--target-registry", "target.io",
		"--source-registries", "source.io",
		"--dry-run",
	}

	output, err = executeCommand(cmd, args...)
	require.NoError(t, err, "Dry run command without output file failed. Output:\n%s", output)

	// Assert dry run output is still correct without output file
	assert.Contains(t, output, "--- Dry Run: Generated Overrides ---", "missing dry run header")
	assert.Contains(t, output, "registry: target.io", "missing registry override")
	assert.Contains(t, output, "--- End Dry Run ---", "missing dry run footer")
}
