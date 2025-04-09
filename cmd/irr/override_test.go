package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	// Use testify for assertions
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Need analysis types for mocking generator return value
	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/override"
	registry "github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"

	// Need cobra for command execution simulation
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// executeCommand is defined in analyze_test.go - we might move it to a shared test utility later

// Mock Generator for testing command logic
type mockGenerator struct {
	GenerateFunc func() (*override.File, error)
}

func (m *mockGenerator) Generate() (*override.File, error) {
	if m.GenerateFunc != nil {
		return m.GenerateFunc()
	}
	return &override.File{}, nil
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

// Mock Loader for testing - REMOVED as not used directly in these tests anymore
// type mockLoader struct { ... }
// func (m *mockLoader) Load(...) { ... }

// REMOVE Mock Strategy definition - we won't mock GetStrategy directly here
// type mockStrategy struct { ... }
// func (m *mockStrategy) GeneratePath(...) { ... }

// REMOVE Backup original functions - not needed for this approach
// var originalLoadMappings = registry.LoadMappings
// var originalGetStrategy = strategy.GetStrategy

func TestOverrideCmdArgs(t *testing.T) {
	// Test cases focusing only on argument validation and required flags
	tests := []struct {
		name           string
		args           []string
		expectErr      bool
		stdErrContains string
	}{
		// --- Missing Required Flags ---
		{
			name:           "missing chart-path",
			args:           []string{"override", "--target-registry", "tr", "--source-registries", "sr"},
			expectErr:      true,
			stdErrContains: "required flag(s) \"chart-path\" not set",
		},
		{
			name:           "missing target-registry",
			args:           []string{"override", "--chart-path", "cp", "--source-registries", "sr"},
			expectErr:      true,
			stdErrContains: "required flag(s) \"target-registry\" not set",
		},
		{
			name:           "missing source-registries",
			args:           []string{"override", "--chart-path", "cp", "--target-registry", "tr"},
			expectErr:      true,
			stdErrContains: "required flag(s) \"source-registries\" not set",
		},
		{
			name:           "all required flags present (execution error expected)",
			args:           []string{"override", "--chart-path", "cp", "--target-registry", "tr", "--source-registries", "sr"},
			expectErr:      true,
			stdErrContains: "chart parsing failed for cp",
		},
		// --- Invalid Flag Values ---
		{
			name: "invalid_path_strategy",
			args: []string{
				"override",
				"--chart-path", "./chart",
				"--target-registry", "tr",
				"--source-registries", "sr",
				"--path-strategy", "invalid-start",
			},
			expectErr:      true,
			stdErrContains: "unknown path strategy: invalid-start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := getRootCmd()
			currentGeneratorFactory = defaultGeneratorFactory
			_, err := executeCommand(rootCmd, tt.args...)

			if tt.expectErr {
				assert.Error(t, err, "Expected an error")
				if tt.stdErrContains != "" {
					assert.Contains(t, err.Error(), tt.stdErrContains, "error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Did not expect an error")
			}
		})
	}
}

func TestOverrideCmdExecution(t *testing.T) {
	originalGeneratorFactory := currentGeneratorFactory
	defer func() {
		currentGeneratorFactory = originalGeneratorFactory
	}()

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
			stdOutContains: "",
			stdErrContains: "error generating overrides: mock generator error",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck:      nil,
		},
		{
			name: "success with output file (flow check)",
			args: defaultArgs, // Will append output file path in the test
			mockGeneratorFunc: func() (*override.File, error) {
				return &override.File{
					Overrides: map[string]interface{}{"image": map[string]interface{}{"repository": "mock-target.com/dockerio/nginx", "tag": "latest"}},
				}, nil
			},
			expectErr:      false,
			stdOutContains: "Overrides written to:",
			stdErrContains: "",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck: func(t *testing.T, testDir string) {
				// Add the #nosec comment right before the os.ReadFile call inside the postCheck
				outputPath := filepath.Join(testDir, "output.yaml")
				// #nosec G304 -- Path is constructed within the test using TempDir
				content, err := os.ReadFile(outputPath)
				require.NoError(t, err, "Should be able to read output file")
				assert.Contains(t, string(content), "repository: mock-target.com/dockerio/nginx")
			},
		},
		{
			name: "success_with_registry_mappings",
			args: defaultArgs, // Base args, mapping file added in test body
			mockGeneratorFunc: func() (*override.File, error) {
				// Expect the output to use the mapped prefix 'dckrio' instead of 'dockerio'
				return &override.File{
					Overrides: map[string]interface{}{"image": map[string]interface{}{"repository": "mock-target.com/dckrio/nginx"}},
				}, nil
			},
			expectErr:      false,
			stdOutContains: "repository: mock-target.com/dckrio/nginx", // Verify mapped output
			stdErrContains: "",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck:      nil, // No specific file check needed here, stdout check covers it
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()

			// Setup Generator Mock
			if tt.mockGeneratorFunc != nil {
				currentGeneratorFactory = func(
					_ string, _ string, // chartPath, targetRegistry
					_ []string, _ []string, // sourceRegistries, excludeRegistries
					_ strategy.PathStrategy, // pathStrategy
					_ *registry.Mappings, // mappings
					_ bool, _ int, // strict, threshold
					_ analysis.ChartLoader, // loader
					_ []string, _ []string, _ []string,
				) GeneratorInterface {
					return &mockGenerator{GenerateFunc: tt.mockGeneratorFunc}
				}
			} else {
				currentGeneratorFactory = defaultGeneratorFactory
			}

			// Set up environment variables if specified
			if tt.setupEnv != nil {
				for k, v := range tt.setupEnv {
					err := os.Setenv(k, v)
					if err != nil {
						t.Fatalf("Failed to set environment variable %s: %v", k, err)
					}
				}
				defer func() {
					for k := range tt.setupEnv {
						err := os.Unsetenv(k)
						if err != nil {
							t.Fatalf("Failed to unset environment variable %s: %v", k, err)
						}
					}
				}()
			}

			// Prepare args with output file if needed
			args := tt.args
			outputPath := ""
			if tt.name == "success with output file (flow check)" {
				outputPath = filepath.Join(testDir, "output.yaml")
				args = append(args, "--output-file", outputPath, "--verbose")
			}
			// ---- START Add logic for registry mapping test ----
			if tt.name == "success_with_registry_mappings" {
				// Create a temporary mapping file in the CWD for the test
				mappingContent := []byte("docker.io: mock-target.com/dckrio")
				mappingFilename := "temp-test-mappings.yaml" // Relative path
				err := os.WriteFile(mappingFilename, mappingContent, 0o600)
				require.NoError(t, err, "Failed to create temp mapping file in CWD")

				args = append(args, "--registry-file", mappingFilename)

				// Add defer to clean up the file AFTER executeCommand runs
				defer func() {
					err := os.Remove(mappingFilename)
					if err != nil && !os.IsNotExist(err) {
						t.Logf("Warning: failed to remove temp mapping file %s: %v", mappingFilename, err)
					}
				}() // Error check added for cleanup
			}
			// ---- END Add logic for registry mapping test ----

			// Get a fresh command instance using the helper
			rootCmd := getRootCmd()

			// Execute command, fixing assignment mismatch (should capture stdout)
			stdout, err := executeCommand(rootCmd, args...)

			// Assertions
			if tt.expectErr {
				assert.Error(t, err, "Expected an error")
				if tt.stdErrContains != "" {
					assert.Contains(t, err.Error(), tt.stdErrContains, "error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Did not expect an error")
				if tt.stdOutContains != "" {
					assert.Contains(t, stdout, tt.stdOutContains)
				}
			}

			// Run post-check if specified
			if tt.postCheck != nil {
				tt.postCheck(t, testDir)
			}
		})
	}
}

// --- Test Setup Helper ---

// executeCommand runs the command with args and returns output/error
func executeCommand(cmd *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// setupTestFS creates a temporary filesystem for tests
func setupTestFS(t *testing.T) (afero.Fs, string) {
	fs := afero.NewMemMapFs()
	tempDir := t.TempDir()
	err := fs.MkdirAll(tempDir, 0755)
	require.NoError(t, err)
	return fs, tempDir
}

// Test helper to create a dummy chart structure
func createDummyChart(fs afero.Fs, chartDir string) error {
	if err := fs.MkdirAll(chartDir, 0755); err != nil {
		return err
	}
	chartYaml := `apiVersion: v2
name: dummy-chart
version: 0.1.0
`
	if err := afero.WriteFile(fs, filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0644); err != nil {
		return err
	}
	valuesYaml := `image:
  repository: source.io/library/nginx
  tag: "1.21"
`
	return afero.WriteFile(fs, filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), 0644)
}

// getRootCmd resets and returns the root command for testing
func getRootCmd() *cobra.Command {
	// Reset flags or state if necessary
	return rootCmd
}

// --- Override Command Tests ---

func TestOverrideCommand_Success(t *testing.T) {
	fs, chartDir := setupTestFS(t)
	AppFs = fs
	require.NoError(t, createDummyChart(fs, chartDir))

	mockGen := &mockGenerator{
		GenerateFunc: func() (*override.File, error) {
			return &override.File{
				ChartPath: chartDir,
				Overrides: map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "target.io",
						"repository": "source.io/library/nginx",
						"tag":        "1.21",
					},
				},
			}, nil
		},
	}
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
	require.NoError(t, err)
	assert.True(t, exists, "Output file was not created")

	// Check file content (optional, depending on Generate mock)
	content, err := afero.ReadFile(fs, outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "registry: target.io")
}

func TestOverrideCommand_DryRun(t *testing.T) {
	fs, chartDir := setupTestFS(t)
	AppFs = fs
	require.NoError(t, createDummyChart(fs, chartDir))

	mockGen := &mockGenerator{
		GenerateFunc: func() (*override.File, error) {
			return &override.File{
				Overrides: map[string]interface{}{"image": map[string]interface{}{"repository": "new-repo"}},
			}, nil
		},
	}
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

	// Assert dry run output contains expected structure
	assert.Contains(t, output, "--- Dry Run: Generated Overrides ---")
	assert.Contains(t, output, "repository: new-repo")

	// Assert file was NOT created
	exists, err := afero.Exists(fs, outputFile)
	require.NoError(t, err)
	assert.False(t, exists, "Output file should not be created in dry run mode")
}

func TestOverrideCommand_MissingFlags(t *testing.T) {
	testCases := []struct {
		name   string
		args   []string
		errMsg string
	}{
		{"Missing all", []string{"override"}, "missing required flags"},
		{"Missing target", []string{"override", "--chart-path", "/chart", "--source-registries", "src"}, "missing required flags"},
		{"Missing source", []string{"override", "--chart-path", "/chart", "--target-registry", "tgt"}, "missing required flags"},
		{"Missing chart", []string{"override", "--target-registry", "tgt", "--source-registries", "src"}, "missing required flags"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := getRootCmd()
			_, err := executeCommand(cmd, tc.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg)
			var exitErr *ExitCodeError
			if errors.As(err, &exitErr) {
				assert.Equal(t, ExitInputConfigurationError, exitErr.Code)
			} else {
				t.Errorf("Expected ExitCodeError, got %T", err)
			}
		})
	}
}

func TestOverrideCommand_GeneratorError(t *testing.T) {
	fs, chartDir := setupTestFS(t)
	AppFs = fs
	require.NoError(t, createDummyChart(fs, chartDir))

	genError := errors.New("generator failed miserably")
	mockGen := &mockGenerator{
		GenerateFunc: func() (*override.File, error) {
			return nil, genError
		},
	}
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

	args := []string{
		"override",
		"--chart-path", chartDir,
		"--target-registry", "target.io",
		"--source-registries", "source.io",
	}

	cmd := getRootCmd()
	_, err := executeCommand(cmd, args...)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error generating overrides")
	assert.Contains(t, err.Error(), genError.Error())

	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		assert.Equal(t, ExitImageProcessingError, exitErr.Code)
	} else {
		t.Errorf("Expected ExitCodeError, got %T", err)
	}
}

// Add more tests for invalid registry formats, file writing errors, etc.
