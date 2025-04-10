package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
func setupAnalyzeTestFS(_ *testing.T) afero.Fs {
	fs := afero.NewMemMapFs()
	return fs
}

// runAnalyzeTestCase executes a single test case for the analyze command.
func runAnalyzeTestCase(t *testing.T, tt *analyzeTestCase) {
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
}

// analyzeTestCase defines the structure of a test case for analyze command
type analyzeTestCase struct {
	name              string
	args              []string
	mockAnalyzeFunc   func() (*analysis.ChartAnalysis, error)
	expectErr         bool
	expectErrArgs     bool
	stdOutContains    string
	stdErrContains    string
	expectFile        string
	expectFileContent string
}

// createAnalyzeErrorTestCase creates a test case that expects an argument error
func createAnalyzeErrorTestCase(name, errorMsg string, args []string, isArgError bool) analyzeTestCase {
	return analyzeTestCase{
		name:           name,
		args:           args,
		expectErr:      true,
		expectErrArgs:  isArgError,
		stdErrContains: errorMsg,
	}
}

// createSuccessTextOutputTestCase creates a test case for successful text output
func createSuccessTextOutputTestCase() analyzeTestCase {
	return analyzeTestCase{
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
	}
}

// createSuccessJSONOutputTestCase creates a test case for successful JSON output
func createSuccessJSONOutputTestCase() analyzeTestCase {
	return analyzeTestCase{
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
	}
}

// createSuccessFileOutputTestCase creates a test case for successful file output
func createSuccessFileOutputTestCase() analyzeTestCase {
	return analyzeTestCase{
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
		expectFileContent: "{", // Check for start of JSON output
	}
}

// createAnalyzerErrorTestCase creates a test case that expects an analyzer error
func createAnalyzerErrorTestCase() analyzeTestCase {
	return analyzeTestCase{
		name: "analyzer returns error",
		args: []string{"analyze", "./bad/chart", "--source-registries", "source.io"},
		mockAnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
			return nil, fmt.Errorf("mock analyze error: chart not found")
		},
		expectErr:      true,
		expectErrArgs:  false,
		stdErrContains: "mock analyze error: chart not found",
	}
}

// defineAnalyzeTestCases creates and returns test cases for the analyze command tests
func defineAnalyzeTestCases() []analyzeTestCase {
	// Create error test cases
	noArgsCase := createAnalyzeErrorTestCase(
		"no arguments",
		"accepts 1 arg(s), received 0",
		[]string{"analyze"},
		true,
	)

	missingFlagCase := createAnalyzeErrorTestCase(
		"missing required flag source-registries",
		"required flag(s) \"source-registries\" not set",
		[]string{"analyze", "./fake/chart"},
		true,
	)

	tooManyArgsCase := createAnalyzeErrorTestCase(
		"too many arguments",
		"accepts 1 arg(s), received 2",
		[]string{"analyze", "path1", "path2", "--source-registries", "source.io"},
		true,
	)

	// Create success test cases
	textOutputCase := createSuccessTextOutputTestCase()
	jsonOutputCase := createSuccessJSONOutputTestCase()
	fileOutputCase := createSuccessFileOutputTestCase()

	// Create analyzer error test case
	analyzerErrorCase := createAnalyzerErrorTestCase()

	// Return all test cases
	return []analyzeTestCase{
		noArgsCase,
		missingFlagCase,
		tooManyArgsCase,
		textOutputCase,
		jsonOutputCase,
		fileOutputCase,
		analyzerErrorCase,
	}
}

func TestAnalyzeCmd(t *testing.T) {
	// Backup and restore original factory, FS, and command outputs
	originalFactory := currentAnalyzerFactory
	originalFs := AppFs
	defer func() {
		currentAnalyzerFactory = originalFactory
		AppFs = originalFs
	}()

	// Get predefined test cases
	tests := defineAnalyzeTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runAnalyzeTestCase(t, &tt)
		})
	}
}

func TestAnalyzeCommand_Success_TextOutput(t *testing.T) {
	chartPath := "/fake/chart"

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

	// Create a dummy command object to pass to runAnalyze
	cmd := newAnalyzeCmd()
	err := cmd.Flags().Set("source-registries", "source.io")
	require.NoError(t, err)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	// Execute the core run function
	err = runAnalyze(cmd, []string{chartPath})
	require.NoError(t, err)

	// Restore stdout and read captured output
	err = w.Close()
	require.NoError(t, err)
	os.Stdout = oldStdout
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	// Verify output
	assert.Contains(t, output, "Chart Analysis")
	assert.Contains(t, output, "Total image patterns found: 1")
	assert.Contains(t, output, "image.repository")
	assert.Contains(t, output, "string")
	assert.Contains(t, output, "source.io/nginx:1.23")
}

func TestAnalyzeCommand_Success_JsonOutput(t *testing.T) {
	chartPath := "/fake/json/chart"

	// Mock the analyzer factory
	mockResult := analysis.NewChartAnalysis()
	mockResult.ImagePatterns = append(mockResult.ImagePatterns, analysis.ImagePattern{
		Path:  "image",
		Type:  analysis.PatternTypeString,
		Value: "source.io/redis:latest",
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

	// Create a dummy command object and set flags
	cmd := newAnalyzeCmd()
	err := cmd.Flags().Set("source-registries", "source.io")
	require.NoError(t, err)
	err = cmd.Flags().Set("output", "json")
	require.NoError(t, err)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	// Execute the core run function
	err = runAnalyze(cmd, []string{chartPath})
	require.NoError(t, err)

	// Restore stdout and read captured output
	err = w.Close()
	require.NoError(t, err)
	os.Stdout = oldStdout
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	// Verify output is valid JSON
	assert.True(t, strings.HasPrefix(output, "{"), "Expected JSON output")
	var jsonData map[string]interface{}
	err = json.Unmarshal([]byte(output), &jsonData)
	require.NoError(t, err, "Failed to unmarshal JSON output")

	// Basic check for expected data in JSON
	imagePatterns, ok := jsonData["ImagePatterns"].([]interface{})
	assert.True(t, ok, "JSON should have ImagePatterns array")
	assert.Len(t, imagePatterns, 1, "JSON should contain one image pattern")
}

func TestAnalyzeCommand_AnalysisError(t *testing.T) {
	fs := setupAnalyzeTestFS(t)
	AppFs = fs
	chartPath := "/fake/error/chart"
	err := fs.MkdirAll(chartPath, 0o755)
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
