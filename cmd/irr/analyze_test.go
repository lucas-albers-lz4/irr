package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/lalbers/irr/pkg/analysis" // Need this for ChartAnalysis type
	"github.com/spf13/afero"              // Add afero
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
	return &analysis.ChartAnalysis{}, nil
}

// --- End Mocking ---

// executeCommand runs the command with args and returns stdout, stderr, and error
// It uses buffers to capture output redirected via SetOut/SetErr.
func executeCommand(cmd *cobra.Command, args ...string) (stdout, stderr string, err error) {
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)

	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetArgs(args)

	err = cmd.Execute()

	return outBuf.String(), errBuf.String(), err
}

func TestAnalyzeCmd(t *testing.T) {
	// Backup and restore original factory, FS, and command outputs
	originalFactory := currentAnalyzerFactory
	originalFs := AppFs
	// No need to backup stdout/stderr here, executeCommand handles it per call
	defer func() {
		currentAnalyzerFactory = originalFactory
		AppFs = originalFs
	}()

	tests := []struct {
		name              string
		args              []string
		mockAnalyzeFunc   func() (*analysis.ChartAnalysis, error) // Func to setup mock
		expectErr         bool
		expectErrArgs     bool   // True if error is expected due to args, not execution
		stdOutContains    string // Substring to check in stdout
		stdErrContains    string // Substring to check in stderr
		expectFile        string // Expected filename
		expectFileContent string // Expected file content
	}{
		// --- Arg/Flag Error Cases (No Mocking Needed) ---
		{
			name:           "no arguments",
			args:           []string{"analyze"},
			expectErr:      true,
			expectErrArgs:  true,
			stdErrContains: "accepts 1 arg(s), received 0",
		},
		{
			name:           "too many arguments",
			args:           []string{"analyze", "path1", "path2"},
			expectErr:      true,
			expectErrArgs:  true,
			stdErrContains: "accepts 1 arg(s), received 2",
		},
		// --- Execution Cases (Mocking Needed) ---
		{
			name: "success with text output",
			args: []string{"analyze", "./fake/chart"},
			mockAnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
				return &analysis.ChartAnalysis{
					ImagePatterns: []analysis.ImagePattern{
						{Path: "image", Type: analysis.PatternTypeString, Value: "nginx:latest", Count: 1},
					},
					GlobalPatterns: []analysis.GlobalPattern{},
				}, nil
			},
			expectErr:      false,
			stdOutContains: "Chart Analysis", // Check for text header
			stdErrContains: "",               // Should be no error output
		},
		{
			name: "success with json output",
			args: []string{"analyze", "./fake/chart", "-o", "json"},
			mockAnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
				return &analysis.ChartAnalysis{
					ImagePatterns: []analysis.ImagePattern{
						{Path: "img", Type: analysis.PatternTypeString, Value: "redis:alpine", Count: 1},
					},
				}, nil
			},
			expectErr:      false,
			stdOutContains: `"Path": "img"`, // Check for JSON structure
			stdErrContains: "",
		},
		{
			name: "analyzer returns error",
			args: []string{"analyze", "./bad/chart"},
			mockAnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
				return nil, fmt.Errorf("mock analyze error: chart not found")
			},
			expectErr:      true,
			expectErrArgs:  false,
			stdOutContains: "",
			stdErrContains: "analysis failed: mock analyze error: chart not found", // Check for wrapped error
		},
		{
			name: "success with output file",
			args: []string{"analyze", "./fake/chart", "--file", "analyze_test_output.txt"},
			mockAnalyzeFunc: func() (*analysis.ChartAnalysis, error) {
				return &analysis.ChartAnalysis{ImagePatterns: []analysis.ImagePattern{{Path: "file.image", Value: "test:ok"}}}, nil
			},
			expectErr:         false,
			stdOutContains:    "",
			stdErrContains:    "",
			expectFile:        "analyze_test_output.txt",
			expectFileContent: "Chart Analysis", // Check for start of text format
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
				currentAnalyzerFactory = func(_ string) AnalyzerInterface { // Rename chartPath to _
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
			rootCmd := newRootCmd()

			// Execute command using the fresh rootCmd instance
			stdout, _, err := executeCommand(rootCmd, tt.args...)

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
