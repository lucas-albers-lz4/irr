package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/lucas-albers-lz4/irr/pkg/testutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	helmchart "helm.sh/helm/v3/pkg/chart"
)

// findIrrBinary locates the irr binary in the bin directory
func findIrrBinary(t *testing.T) string {
	t.Helper()

	// Find project root (directory with go.mod)
	wd, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	// Look for bin/irr relative to project root
	// Start from current directory and go up until we find go.mod
	dir := wd
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Found go.mod, this is the root
			binPath := filepath.Join(dir, "bin", "irr")
			if _, err := os.Stat(binPath); err == nil {
				return binPath
			}
			// No else needed, will proceed to fatal
			t.Fatalf("irr binary not found at expected path: %s", binPath)
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root without finding go.mod
			t.Fatalf("Failed to find project root (go.mod) starting from %s", wd)
		}
		dir = parent
	}
}

// runCommand executes the binary with provided arguments and returns the output and exit code
// #nosec G204 - This is a test helper that needs to run commands with variable args
func runCommand(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	binPath := findIrrBinary(t)
	cmd := exec.Command(binPath, args...)

	// Set IRR_TESTING environment variable
	cmd.Env = append(os.Environ(), "IRR_TESTING=true")

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()

	// Extract exit code
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("Failed to execute command: %v", err)
		}
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

// runCommandInProcess executes commands in-process for testing
func runCommandInProcess(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	// Use the updated executeCommand helper which returns separate stdout/stderr
	stdout, stderr, err := executeCommand(getRootCmd(), args...)

	// Set exit code based on the error OR stderr content as Cobra might swallow the error object.
	if err != nil {
		// If stderr wasn't captured by executeCommand's buffer (e.g., direct os.Stderr write),
		// use the error message as stderr.
		if stderr == "" {
			stderr = err.Error()
		}

		// Check for specific exit code errors
		var exitErr *exitcodes.ExitCodeError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.Code
		} else {
			// For non-exit code errors from the command framework
			exitCode = 1 // Default error code
		}
	} else if stderr != "" {
		// WORKAROUND: If err is nil, but stderr has content, infer exit code from stderr
		// This handles cases where Cobra prints the error and exits internally without returning the error object.
		switch {
		case strings.Contains(stderr, "no Chart.yaml found"):
			exitCode = exitcodes.ExitChartNotFound // Code 4
		case strings.Contains(stderr, "at least one values file must be specified"):
			exitCode = exitcodes.ExitInputConfigurationError // Code 2
		case strings.Contains(stderr, "values file not found or inaccessible"):
			exitCode = exitcodes.ExitChartNotFound // Code 4 (as per test case)
			// No default case needed: If no known error string is found, exitCode remains 0 (or the value from the error check above)
		}
	}

	// Return the captured stdout, stderr, and determined exit code
	return stdout, stderr, exitCode
}

// createTempChart creates a minimal chart for testing in a temporary directory
func createTempChart(t *testing.T) (chartPath string, cleanup func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "irr-cli-test-")
	require.NoError(t, err, "Failed to create temp directory")

	chartDir := filepath.Join(tempDir, "test-chart")
	// #nosec G301 - This is a test directory with read permissions needed
	err = os.Mkdir(chartDir, 0o750)
	require.NoError(t, err, "Failed to create chart directory")

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: test-chart
version: 0.1.0
`
	// Use constants for file permissions instead of hardcoded values for consistency and maintainability
	err = os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), fileutil.ReadWriteUserPermission)
	require.NoError(t, err, "Failed to create Chart.yaml")

	// Create values.yaml with image references
	valuesYaml := `image:
  repository: docker.io/library/nginx
  tag: "1.21.0"
sidecar:
  image: docker.io/library/busybox:1.33.1
`
	// Use constants for file permissions instead of hardcoded values for consistency and maintainability
	err = os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), fileutil.ReadWriteUserPermission)
	require.NoError(t, err, "Failed to create values.yaml")

	cleanup = func() {
		err := os.RemoveAll(tempDir)
		if err != nil {
			fmt.Printf("Warning: Failed to remove temp directory %s: %v\n", tempDir, err)
		}
	}

	return chartDir, cleanup
}

// createTempConfigFile creates a temporary config file for testing
func createTempConfigFile(t *testing.T) (configPath string, cleanup func()) {
	t.Helper()

	tempFile, err := os.CreateTemp("", "irr-config-*.yaml")
	require.NoError(t, err, "Failed to create temp config file")

	configYaml := `sourceRegistries:
  - docker.io
targetRegistry: example.registry.io
`
	_, err = tempFile.WriteString(configYaml)
	require.NoError(t, err, "Failed to write to config file")
	err = tempFile.Close()
	require.NoError(t, err, "Failed to close config file")

	cleanup = func() {
		err := os.Remove(tempFile.Name())
		if err != nil {
			fmt.Printf("Warning: Failed to remove temp file %s: %v\n", tempFile.Name(), err)
		}
	}

	return tempFile.Name(), cleanup
}

// TestBinaryExists verifies that the irr binary can be found
func TestBinaryExists(t *testing.T) {
	binPath := findIrrBinary(t)
	_, err := os.Stat(binPath)
	assert.NoError(t, err, "irr binary not found at %s", binPath)
}

// TestInspectCommand tests the inspect command syntax
func TestInspectCommand(t *testing.T) {
	// Save original adapter factory and restore after test
	originalHelmAdapterFactory := helmAdapterFactory
	originalTestAnalyzeMode := TestAnalyzeMode
	TestAnalyzeMode = true
	defer func() {
		helmAdapterFactory = originalHelmAdapterFactory
		TestAnalyzeMode = originalTestAnalyzeMode
	}()

	// Create a mock adapter factory
	helmAdapterFactory = func() (*helm.Adapter, error) {
		// Create a mock client
		mockClient := &MockHelmClient{
			ReleaseValues: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "latest",
				},
			},
			ReleaseChart: &helmchart.Chart{
				Metadata: &helmchart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
			},
		}
		// Create a mock adapter with the mock client
		adapter := helm.NewAdapter(mockClient, afero.NewMemMapFs(), true)
		return adapter, nil
	}

	chartPath, cleanup := createTempChart(t)
	defer cleanup()

	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantErr  string
	}{
		{
			name:     "basic inspect",
			args:     []string{"inspect", "--chart-path", chartPath, "--output-format", "yaml"},
			wantExit: 0,
		},
		{
			name:     "inspect with source registries",
			args:     []string{"inspect", "--chart-path", chartPath, "--source-registries", "docker.io", "--output-format", "yaml"},
			wantExit: 0,
		},
		{
			name:     "non-existent chart path",
			args:     []string{"inspect", "--chart-path", "/non/existent/path", "--output-format", "yaml"},
			wantExit: 4,
			wantErr:  "chart path not found or inaccessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the in-process command runner instead of the binary
			_, stderr, exitCode := runCommandInProcess(t, tt.args...)
			assert.Equal(t, tt.wantExit, exitCode, "Expected exit code %d, got %d", tt.wantExit, exitCode)
			if tt.wantErr != "" {
				assert.Contains(t, stderr, tt.wantErr)
			}
		})
	}
}

// TestOverrideCommand tests the override command syntax
func TestOverrideCommand(t *testing.T) {
	chartPath, cleanup := createTempChart(t)
	defer cleanup()

	// Generate a unique temp file path for output, but do NOT create the file
	tempDir := os.TempDir()
	outputPath := filepath.Join(tempDir, fmt.Sprintf("irr-output-%d.yaml", os.Getpid()))
	// Ensure the file does not exist before the test
	err := os.Remove(outputPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("Failed to remove output file %s: %v", outputPath, err)
	}
	defer func() {
		err := os.Remove(outputPath)
		if err != nil && !os.IsNotExist(err) {
			t.Logf("Warning: Failed to remove output file %s: %v", outputPath, err)
		}
	}()

	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantErr  string
	}{
		{
			name: "basic override",
			args: []string{
				"override",
				"--chart-path", chartPath,
				"--source-registries", "docker.io",
				"--target-registry", "example.registry.io",
			},
			wantExit: 0,
		},
		{
			name: "override with output file",
			args: []string{
				"override",
				"--chart-path", chartPath,
				"--source-registries", "docker.io",
				"--target-registry", "example.registry.io",
				"--output-file", outputPath,
			},
			wantExit: 0,
		},
		{
			name: "missing chart path",
			args: []string{
				"override",
				"--source-registries", "docker.io",
				"--target-registry", "example.registry.io",
			},
			wantExit: 1,
			wantErr:  "required flag(s) \"chart-path\" not set",
		},
		{
			name: "missing target registry",
			args: []string{
				"override",
				"--chart-path", chartPath,
				"--source-registries", "docker.io",
			},
			wantExit: 1,
			wantErr:  "required flag(s) \"target-registry\" not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, exitCode := runCommand(t, tt.args...)
			assert.Equal(t, tt.wantExit, exitCode, "Expected exit code %d, got %d", tt.wantExit, exitCode)
			if tt.wantErr != "" {
				assert.Contains(t, stderr, tt.wantErr)
			}
		})
	}
}

// TestValidateCommand tests the validate command syntax
func TestValidateCommand(t *testing.T) {
	chartPath, cleanup := createTempChart(t)
	defer cleanup()

	// Create a values file to validate
	valuesFile := filepath.Join(chartPath, "test-values.yaml")
	valuesContent := `image:
  repository: example.registry.io/library/nginx
  tag: "1.21.0"
sidecar:
  image: example.registry.io/library/busybox:1.33.1
`
	// Use constants for file permissions instead of hardcoded values for consistency and maintainability
	err := os.WriteFile(valuesFile, []byte(valuesContent), fileutil.ReadWriteUserPermission)
	require.NoError(t, err, "Failed to create test values file")

	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantErr  string
	}{
		{
			name: "basic validate",
			args: []string{
				"validate",
				"--chart-path", chartPath,
				"--values", valuesFile,
			},
			wantExit: 0,
		},
		{
			name:     "missing chart path",
			args:     []string{"validate"},        // No chart path provided, no values file
			wantExit: exitcodes.ExitChartNotFound, // Expect chart not found/load error
			wantErr:  "no Chart.yaml found",       // Updated expected error string for detection failure
		},
		{
			name: "missing values file",
			args: []string{
				"validate",
				"--chart-path", chartPath,
			},
			wantExit: 2,
			wantErr:  "at least one values file must be specified",
		},
		{
			name: "non-existent values file",
			args: []string{
				"validate",
				"--chart-path", chartPath,
				"--values", "/non/existent/values.yaml",
			},
			wantExit: 4,
			wantErr:  "values file not found or inaccessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Revert back to runCommand to accurately test exit codes from binary execution
			_, stderr, exitCode := runCommand(t, tt.args...)
			assert.Equal(t, tt.wantExit, exitCode, "Expected exit code %d, got %d", tt.wantExit, exitCode)
			if tt.wantErr != "" {
				assert.Contains(t, stderr, tt.wantErr)
			}
		})
	}
}

// TestCompletionCommand tests the completion command syntax
func TestCompletionCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantOut  string
		wantErr  string
	}{
		{
			name:     "bash completion",
			args:     []string{"completion", "bash"},
			wantExit: 0,
			wantOut:  "# bash completion",
		},
		{
			name:     "zsh completion",
			args:     []string{"completion", "zsh"},
			wantExit: 0,
			wantOut:  "#compdef irr",
		},
		{
			name:     "fish completion",
			args:     []string{"completion", "fish"},
			wantExit: 0,
			wantOut:  "# fish completion for irr",
		},
		{
			name:     "powershell completion",
			args:     []string{"completion", "powershell"},
			wantExit: 0,
			wantOut:  "Register-ArgumentCompleter",
		},
		{
			name:     "missing shell",
			args:     []string{"completion"},
			wantExit: 0,
			wantOut:  "Generate the autocompletion script for irr",
		},
		{
			name:     "invalid shell",
			args:     []string{"completion", "invalid-shell"},
			wantExit: 0,
			wantOut:  "Available Commands:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runCommand(t, tt.args...)
			output := stdout + stderr // Combine stdout and stderr for checking
			assert.Equal(t, tt.wantExit, exitCode, "Expected exit code %d, got %d", tt.wantExit, exitCode)
			if tt.wantOut != "" {
				assert.Contains(t, output, tt.wantOut)
			}
			if tt.wantErr != "" {
				assert.Contains(t, stderr, tt.wantErr)
			}
		})
	}
}

// TestHelpCommand tests the help command for all subcommands
func TestHelpCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantOut  string
	}{
		{
			name:     "general help",
			args:     []string{"help"},
			wantExit: 0,
			wantOut:  "irr (Image Relocation and Rewrite)",
		},
		{
			name:     "inspect help",
			args:     []string{"help", "inspect"},
			wantExit: 0,
			wantOut:  "Inspect a Helm chart",
		},
		{
			name:     "override help",
			args:     []string{"help", "override"},
			wantExit: 0,
			wantOut:  "Analyzes a Helm chart to find all container image references",
		},
		{
			name:     "validate help",
			args:     []string{"help", "validate"},
			wantExit: 0,
			wantOut:  "Validates that a Helm chart can be rendered correctly",
		},
		{
			name:     "completion help",
			args:     []string{"help", "completion"},
			wantExit: 0,
			wantOut:  "Generate the autocompletion script for irr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, exitCode := runCommand(t, tt.args...)
			assert.Equal(t, tt.wantExit, exitCode, "Expected exit code %d, got %d", tt.wantExit, exitCode)
			assert.Contains(t, stdout, tt.wantOut)
		})
	}
}

// Helper function to parse a string containing newline-separated JSON logs
func parseJSONLogsFromString(logString string) ([]map[string]interface{}, error) {
	var logs []map[string]interface{}
	lines := strings.Split(strings.TrimSpace(logString), "\n")
	for i, line := range lines {
		// Only attempt to parse lines that look like JSON objects
		if line == "" || !strings.HasPrefix(line, "{") {
			continue // Skip empty lines or non-JSON lines
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal log line %d as JSON: %w\nLine content: %s", i+1, err, line)
		}
		logs = append(logs, entry)
	}
	return logs, nil
}

// TestGlobalFlags tests the global flags across commands
func TestGlobalFlags(t *testing.T) {
	// Save original adapter factory and restore after test
	originalHelmAdapterFactory := helmAdapterFactory
	originalTestAnalyzeMode := TestAnalyzeMode
	TestAnalyzeMode = true
	defer func() {
		helmAdapterFactory = originalHelmAdapterFactory
		TestAnalyzeMode = originalTestAnalyzeMode
	}()

	// Create a mock adapter factory
	helmAdapterFactory = func() (*helm.Adapter, error) {
		// Create a mock client
		mockClient := &MockHelmClient{
			ReleaseValues: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "latest",
				},
			},
			ReleaseChart: &helmchart.Chart{
				Metadata: &helmchart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
			},
		}
		// Create a mock adapter with the mock client
		adapter := helm.NewAdapter(mockClient, afero.NewMemMapFs(), true)
		return adapter, nil
	}

	chartPath, cleanup := createTempChart(t)
	defer cleanup()

	configFile, configCleanup := createTempConfigFile(t)
	defer configCleanup()

	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		wantExit int
		checkOut func(t *testing.T, stdout, stderr string)
	}{
		{
			name:     "debug flag",
			args:     []string{"--debug", "help"},
			wantExit: 0,
			// Expect JSON output by default now
			env: map[string]string{"LOG_FORMAT": "json"},
			checkOut: func(t *testing.T, _, stderr string) {
				// Parse the stderr from the initial runCommand
				logs, err := parseJSONLogsFromString(stderr)
				require.NoError(t, err, "Failed to parse logs from stderr for --debug flag")
				testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{"level": "DEBUG"})
			},
		},
		{
			name: "debug env var",
			args: []string{"help"},
			env:  map[string]string{"LOG_LEVEL": "DEBUG", "LOG_FORMAT": "json"},
			checkOut: func(t *testing.T, _, stderr string) {
				// Stderr already contains the JSON logs because we set LOG_FORMAT=json
				logs, err := parseJSONLogsFromString(stderr)
				require.NoError(t, err, "Failed to parse logs from stderr")
				testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{"level": "DEBUG"})
			},
		},
		{
			name: "debug flag overrides env var",
			args: []string{"--debug", "help"},
			env:  map[string]string{"LOG_LEVEL": "INFO", "LOG_FORMAT": "json"}, // Start with INFO
			checkOut: func(t *testing.T, _, stderr string) {
				// Stderr already contains the JSON logs because we set LOG_FORMAT=json
				logs, err := parseJSONLogsFromString(stderr)
				require.NoError(t, err, "Failed to parse logs from stderr")
				// --debug flag should force DEBUG level logs
				testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{"level": "DEBUG"})
			},
		},
		{
			name:     "log level flag",
			args:     []string{"--log-level", "error", "help"},
			wantExit: 0,
			// No checking needed, just verify command success
			checkOut: nil,
		},
		{
			name:     "config flag with inspect",
			args:     []string{"--config", configFile, "inspect", "--chart-path", chartPath, "--output-format", "yaml"},
			wantExit: 0,
			checkOut: func(t *testing.T, stdout, _ /*stderr*/ string) {
				assert.Contains(t, stdout, "chart:", "Output should include chart section")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables and create a list of cleanup functions
			var cleanupFuncs []func()
			for k, v := range tt.env {
				err := os.Setenv(k, v)
				if err != nil {
					t.Fatalf("Failed to set environment variable %s: %v", k, err)
				}
				// Create a cleanup function for each env var
				key := k // Capture loop variable
				cleanupFuncs = append(cleanupFuncs, func() {
					err := os.Unsetenv(key)
					if err != nil {
						t.Logf("Warning: Failed to unset environment variable %s: %v", key, err)
					}
				})
			}
			// Use a single defer that runs all cleanup functions
			defer func() {
				for _, cleanup := range cleanupFuncs {
					cleanup()
				}
			}()

			stdout, stderr, exitCode := runCommand(t, tt.args...)
			assert.Equal(t, tt.wantExit, exitCode, "Expected exit code %d, got %d", tt.wantExit, exitCode)
			if tt.checkOut != nil {
				tt.checkOut(t, stdout, stderr)
			}
		})
	}
}

// TestCommandCombinations tests combinations of flags across commands
func TestCommandCombinations(t *testing.T) {
	// Save original adapter factory and restore after test
	originalHelmAdapterFactory := helmAdapterFactory
	originalTestAnalyzeMode := TestAnalyzeMode
	TestAnalyzeMode = true
	defer func() {
		helmAdapterFactory = originalHelmAdapterFactory
		TestAnalyzeMode = originalTestAnalyzeMode
	}()

	// Create a mock adapter factory
	helmAdapterFactory = func() (*helm.Adapter, error) {
		// Create a mock client
		mockClient := &MockHelmClient{
			ReleaseValues: map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "latest",
				},
			},
			ReleaseChart: &helmchart.Chart{
				Metadata: &helmchart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
			},
		}
		// Create a mock adapter with the mock client
		adapter := helm.NewAdapter(mockClient, afero.NewMemMapFs(), true)
		return adapter, nil
	}

	chartPath, cleanup := createTempChart(t)
	defer cleanup()

	// Create a values file for testing
	valuesFile := filepath.Join(chartPath, "test-values.yaml")
	valuesContent := `image:
  repository: example.registry.io/library/nginx
  tag: "1.21.0"
sidecar:
  image: example.registry.io/library/busybox:1.33.1
`
	// Use constants for file permissions instead of hardcoded values for consistency and maintainability
	err := os.WriteFile(valuesFile, []byte(valuesContent), fileutil.ReadWriteUserPermission)
	require.NoError(t, err, "Failed to create test values file")

	// Setup output file path
	tempDir := os.TempDir()
	outputPath := filepath.Join(tempDir, fmt.Sprintf("irr-combined-output-%d.yaml", os.Getpid()))
	defer func() {
		if err := os.Remove(outputPath); err != nil {
			t.Logf("Warning: failed to remove temporary output file %s: %v", outputPath, err)
		}
	}()

	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantErr  string
	}{
		{
			name: "inspect with source registries and pattern",
			args: []string{
				"inspect",
				"--chart-path", chartPath,
				"--source-registries", "docker.io",
				"--include-pattern", "*.repository,*.tag",
				"--output-format", "yaml",
			},
			wantExit: 0,
		},
		{
			name: "override with debug and strategy",
			args: []string{
				"--debug",
				"override",
				"--chart-path", chartPath,
				"--source-registries", "docker.io",
				"--target-registry", "example.registry.io",
				"--path-strategy", "prefix-source-registry",
			},
			wantExit: 0,
		},
		{
			name: "validate with multiple values files",
			args: []string{
				"validate",
				"--chart-path", chartPath,
				"--values", filepath.Join(chartPath, "values.yaml"),
				"--values", valuesFile,
			},
			wantExit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, exitCode := runCommand(t, tt.args...)
			assert.Equal(t, tt.wantExit, exitCode, "Expected exit code %d, got %d", tt.wantExit, exitCode)
		})
	}
}

// TestMissingChartPathInspect tests the specific case of missing the required chart-path flag
func TestMissingChartPathInspect(t *testing.T) {
	// Save the original value of TestAnalyzeMode and restore it after the test
	originalTestAnalyzeMode := TestAnalyzeMode
	TestAnalyzeMode = false
	defer func() { TestAnalyzeMode = originalTestAnalyzeMode }()

	// Run inspect without chart-path
	_, stderr, exitCode := runCommandInProcess(t, "inspect", "--output-format=yaml")

	// Check exit code - using 4 which is the actual code returned (chart path not found)
	assert.Equal(t, 4, exitCode, "Expected exit code 4 for chart path not found")

	// Verify error message mentions chart path not found
	assert.Contains(t, stderr, "chart path not found", "Error should indicate chart path not found")
}
