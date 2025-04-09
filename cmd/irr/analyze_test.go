package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocking ---

// mockAnalyzer implements the AnalyzerInterface defined in root.go
type mockAnalyzer struct {
	AnalyzeFunc func() (*analysis.ChartAnalysis, error)
}

// Analyze implements the AnalyzerInterface
func (m *mockAnalyzer) Analyze() (*analysis.ChartAnalysis, error) {
	if m.AnalyzeFunc != nil {
		return m.AnalyzeFunc()
	}
	return analysis.NewChartAnalysis(), nil
}

// --- End Mocking ---

// executeAnalyzeCommand runs the analyze command with args and returns output/error
func executeAnalyzeCommand(cmd *cobra.Command, args ...string) (string, error) {
	return executeCommand(cmd, args...)
}

// setupAnalyzeTestFS creates a temporary filesystem for tests
func setupAnalyzeTestFS(t *testing.T) afero.Fs {
	fs := afero.NewMemMapFs()
	return fs
}

func TestAnalyzeCmd(t *testing.T) {
	// Backup and restore original factory, FS, and command outputs
	originalFactory := currentAnalyzerFactory
	originalFs := AppFs
	defer func() {
		currentAnalyzerFactory = originalFactory
		AppFs = originalFs
	}()

	tests := []struct {
		name              string
		args              []string
		mockAnalyzeFunc   func() (*analysis.ChartAnalysis, error)
		expectErr         bool
		expectErrArgs     bool
		stdOutContains    string
		stdErrContains    string
		expectFile        string
		expectFileContent string
	}{
		{
			name:           "no arguments",
			args:           []string{"analyze"},
			expectErr:      true,
			expectErrArgs:  true,
			stdErrContains: "accepts 1 arg(s), received 0",
		},
		{
			name:           "missing required flag source-registries",
			args:           []string{"analyze", "./fake/chart"},
			expectErr:      true,
			expectErrArgs:  true,
			stdErrContains: "required flag(s) \"source-registries\" not set",
		},
		{
			name:           "too many arguments",
			args:           []string{"analyze", "path1", "path2", "--source-registries", "source.io"},
			expectErr:      true,
			expectErrArgs:  true,
			stdErrContains: "accepts 1 arg(s), received 2",
		},
		{
			name: "success with text output",
			args: []string{"analyze", "./fake/chart", "--source-registries", "source.io"},
			mockAnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
				return &analysis.ChartAnalysis{
					ImagePatterns: []analysis.ImagePattern{
						{Path: "image", Type: analysis.PatternTypeString, Value: "nginx:latest", Count: 1},
					},
					GlobalPatterns: []analysis.GlobalPattern{},
				}, nil
			},
			expectErr:      false,
			stdOutContains: "Chart Analysis",
			stdErrContains: "",
		},
		{
			name: "success with json output",
			args: []string{"analyze", "./fake/chart", "--source-registries", "source.io", "--output", "json"},
			mockAnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
				return &analysis.ChartAnalysis{
					ImagePatterns: []analysis.ImagePattern{
						{Path: "image", Type: analysis.PatternTypeString, Value: "nginx:latest", Count: 1},
					},
				}, nil
			},
			expectErr:      false,
			stdOutContains: `"Path": "image"`,
		},
		{
			name: "success with output file",
			args: []string{"analyze", "../../test-data/charts/basic", "--source-registries", "source.io", "--output-file", "analyze_test_output.txt"},
			mockAnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
				// Provide a simple mock result for the file output test
				result := analysis.NewChartAnalysis()
				result.ImagePatterns = append(result.ImagePatterns, analysis.ImagePattern{Path: "some.image", Value: "source.io/test:1.0"})
				return result, nil
			},
			expectErr:         false,
			expectFile:        "analyze_test_output.txt",
			expectFileContent: "Chart Analysis", // Basic check for text output in file
		},
		{
			name: "analyzer returns error",
			args: []string{"analyze", "./bad/chart", "--source-registries", "source.io"},
			mockAnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
				return nil, fmt.Errorf("mock analyze error: chart not found")
			},
			expectErr:      true,
			expectErrArgs:  false,
			stdErrContains: "mock analyze error: chart not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock for this test case if needed
			if tt.mockAnalyzeFunc != nil {
				// Set up mock analyzer
				mockAnalyzer := &mockAnalyzer{
					AnalyzeFunc: tt.mockAnalyzeFunc,
				}
				currentAnalyzerFactory = func(_ string) AnalyzerInterface {
					return mockAnalyzer
				}
			} else {
				// Ensure tests not needing mock use the default (or a non-panicking stub)
				currentAnalyzerFactory = func(_ string) AnalyzerInterface {
					// Return a mock that produces an expected error for non-mocked execution tests
					return &mockAnalyzer{
						AnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
							return nil, fmt.Errorf("analyzer not mocked for this test")
						},
					}
				}
			}

			// Setup FS
			if tt.expectFile != "" {
				AppFs = afero.NewMemMapFs() // Use memory map for file tests
			} else {
				AppFs = afero.NewOsFs() // Use OS fs otherwise (though output goes to buffer)
			}

			// Create a fresh command tree for THIS test run
			rootCmd := getRootCmd()

			// Execute command using the fresh rootCmd instance
			stdout, err := executeAnalyzeCommand(rootCmd, tt.args...)

			// Assertions (checking err.Error() for errors, stdout for success)
			if tt.expectErr {
				assert.Error(t, err, "Expected an error")
				if tt.stdErrContains != "" {
					// Assert that the error string contains the expected substring
					assert.Contains(t, err.Error(), tt.stdErrContains, "error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Did not expect an error")
				if tt.stdOutContains != "" {
					assert.Contains(t, stdout, tt.stdOutContains, "stdout should contain expected message")
				}
				// Stderr might contain debug/verbose output even on success, so don't assert empty
			}

			// Assert file content if expected
			if tt.expectFile != "" && !tt.expectErr {
				exists, err := afero.Exists(AppFs, tt.expectFile)
				require.NoError(t, err, "Error checking if file exists")
				require.True(t, exists, "Expected output file '%s' to be created", tt.expectFile)
				contentBytes, readErr := afero.ReadFile(AppFs, tt.expectFile)
				require.NoError(t, readErr, "Error reading output file '%s'", tt.expectFile)
				assert.Contains(t, string(contentBytes), tt.expectFileContent, "File content mismatch for '%s'", tt.expectFile)
			}
		})
	}
}

func TestAnalyzeCommand_Success_TextOutput(t *testing.T) {
	fs := setupAnalyzeTestFS(t)
	AppFs = fs // Use mock FS for the command
	chartPath := "/fake/chart"
	err := fs.MkdirAll(chartPath, 0755) // Ensure dir exists
	require.NoError(t, err, "Failed to create test directory")

	// Mock the analyzer factory
	mockResult := analysis.NewChartAnalysis()
	mockResult.ImagePatterns = append(mockResult.ImagePatterns, analysis.ImagePattern{
		Path:  "image.repository",
		Type:  analysis.PatternTypeString,
		Value: "source.io/nginx:1.23",
		Count: 1,
	})
	mockAnalysis := &mockAnalyzer{
		AnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
			return mockResult, nil
		},
	}
	originalFactory := currentAnalyzerFactory
	currentAnalyzerFactory = func(_ string) AnalyzerInterface {
		return mockAnalysis
	}
	defer func() { currentAnalyzerFactory = originalFactory }()

	args := []string{"analyze", chartPath, "--source-registries", "source.io"}
	cmd := getRootCmd() // Get the root command
	output, err := executeAnalyzeCommand(cmd, args...)
	require.NoError(t, err)

	assert.Contains(t, output, "Chart Analysis")
	assert.Contains(t, output, "Total image patterns: 1")
	assert.Contains(t, output, "image.repository")
	assert.Contains(t, output, "string")
	assert.Contains(t, output, "source.io/nginx:1.23")
}

func TestAnalyzeCommand_Success_JsonOutput(t *testing.T) {
	fs := setupAnalyzeTestFS(t)
	AppFs = fs
	chartPath := "/fake/json/chart"
	err := fs.MkdirAll(chartPath, 0755)
	require.NoError(t, err, "Failed to create test directory")

	mockResult := analysis.NewChartAnalysis()
	mockResult.ImagePatterns = append(mockResult.ImagePatterns, analysis.ImagePattern{
		Path:  "image.repository",
		Type:  analysis.PatternTypeString,
		Value: "source.io/nginx:1.23",
		Count: 1,
	})
	mockAnalysis := &mockAnalyzer{
		AnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
			return mockResult, nil
		},
	}
	originalFactory := currentAnalyzerFactory
	currentAnalyzerFactory = func(_ string) AnalyzerInterface {
		return mockAnalysis
	}
	defer func() { currentAnalyzerFactory = originalFactory }()

	args := []string{"analyze", chartPath, "--source-registries", "source.io", "--output", "json"}
	cmd := getRootCmd()
	output, err := executeAnalyzeCommand(cmd, args...)
	require.NoError(t, err)

	// Basic check for JSON structure
	assert.True(t, strings.HasPrefix(strings.TrimSpace(output), "{"), "Expected JSON output")
	var jsonData map[string]interface{}
	err = json.Unmarshal([]byte(output), &jsonData)
	require.NoError(t, err, "Failed to unmarshal JSON output")
	assert.NotNil(t, jsonData["ImagePatterns"])
}

func TestAnalyzeCommand_AnalysisError(t *testing.T) {
	fs := setupAnalyzeTestFS(t)
	AppFs = fs
	chartPath := "/fake/error/chart"
	err := fs.MkdirAll(chartPath, 0755)
	require.NoError(t, err, "Failed to create test directory")

	analysisError := errors.New("failed to analyze")
	mockAnalysis := &mockAnalyzer{
		AnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
			return nil, analysisError
		},
	}
	originalFactory := currentAnalyzerFactory
	currentAnalyzerFactory = func(_ string) AnalyzerInterface {
		return mockAnalysis
	}
	defer func() { currentAnalyzerFactory = originalFactory }()

	args := []string{"analyze", chartPath, "--source-registries", "source.io"}
	cmd := getRootCmd()
	output, err := executeAnalyzeCommand(cmd, args...)
	require.Error(t, err)
	assert.Contains(t, err.Error(), analysisError.Error())
	assert.Contains(t, output, analysisError.Error()) // Cobra prints error by default

	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		assert.Equal(t, exitcodes.ExitChartParsingError, exitErr.ExitCode())
	} else {
		t.Errorf("Expected ExitCodeError, got %T", err)
	}
}

func TestAnalyzeCommand_NoArgs(t *testing.T) {
	args := []string{"analyze"}
	cmd := getRootCmd()
	_, err := executeAnalyzeCommand(cmd, args...)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg(s), received 0")
}
