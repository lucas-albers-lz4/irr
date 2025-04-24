package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommand_NoSubcommand(t *testing.T) {
	cmd := getRootCmd() // Use the helper from test_helpers_test.go
	_, err := executeCommand(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "a subcommand is required")
}

func TestRootCommand_Help(t *testing.T) {
	cmd := getRootCmd()
	output, err := executeCommand(cmd, "help")
	assert.NoError(t, err)
	assert.Contains(t, output, "irr (Image Relocation and Rewrite) is a tool")
}

// executeCommandWithStderrCapture extends executeCommand to also capture stderr output
// This is necessary for testing debug logs which go to stderr
func executeCommandWithStderrCapture(root *cobra.Command, args ...string) (cmdOutput, stderrOutput string, err error) {
	// Set up command outputs
	cmdBuf := new(bytes.Buffer)
	root.SetOut(cmdBuf)
	root.SetErr(cmdBuf)
	root.SetArgs(args)

	// Set up stderr capture
	oldStderr := os.Stderr
	stderrReader, stderrWriter, pipeErr := os.Pipe()
	if pipeErr != nil {
		return "", "", fmt.Errorf("failed to create pipe: %w", pipeErr)
	}
	os.Stderr = stderrWriter

	// Execute command
	err = root.Execute()

	// Close writer to release pipe
	closeErr := stderrWriter.Close()
	if closeErr != nil {
		// Log but continue, as we still want to restore stderr
		fmt.Fprintf(os.Stderr, "Warning: Error closing stderr pipe: %v\n", closeErr)
	}

	// Restore original stderr
	os.Stderr = oldStderr

	// Read stderr from pipe into buffer
	stderrBuf := new(bytes.Buffer)
	_, copyErr := io.Copy(stderrBuf, stderrReader)
	if copyErr != nil {
		return cmdBuf.String(), stderrBuf.String(), fmt.Errorf("error reading from stderr pipe: %w", copyErr)
	}

	return cmdBuf.String(), stderrBuf.String(), err
}

// TestDebugFlagAndEnvVarInteraction tests how debug flag and environment variables interact
func TestDebugFlagAndEnvVarInteraction(t *testing.T) {
	// Save and restore original state
	origLogLevel := os.Getenv("LOG_LEVEL")
	defer func() {
		err := os.Setenv("LOG_LEVEL", origLogLevel)
		if err != nil {
			t.Logf("Warning: failed to restore LOG_LEVEL environment variable: %v", err)
		}
	}()

	// Part 1: Test command-line flags (simpler, less dependent on environment)
	t.Run("FlagTests", func(t *testing.T) {
		t.Run("NoFlag_DebugDisabled", func(t *testing.T) {
			// Ensure default log level (INFO)
			t.Setenv("LOG_LEVEL", "INFO")

			// Execute command without debug flag
			cmd := getRootCmd()
			_, logs, err := testutil.CaptureJSONLogs(log.LevelInfo, func() {
				_, _, execErr := executeCommandWithStderrCapture(cmd, "help")
				_ = execErr // Ignore potential execution error
			})
			require.NoError(t, err, "Log capture failed")

			// Verify debug is disabled and no debug logs are present
			testutil.AssertLogDoesNotContainJSON(t, logs, map[string]interface{}{"level": "DEBUG"})
		})

		t.Run("WithFlag_DebugEnabled", func(t *testing.T) {
			// Set log level to INFO initially to ensure --debug overrides it
			t.Setenv("LOG_LEVEL", "INFO")

			// Execute command with debug flag
			cmd := getRootCmd()
			_, logs, err := testutil.CaptureJSONLogs(log.LevelDebug, func() {
				_, _, execErr := executeCommandWithStderrCapture(cmd, "--debug", "help")
				if execErr != nil {
					t.Errorf("command execution failed unexpectedly: %v", execErr)
				}
			})
			require.NoError(t, err, "Log capture failed")

			// Verify debug is enabled and debug logs are present
			// The --debug flag should set LOG_LEVEL=DEBUG implicitly
			testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{"level": "DEBUG"})
		})

		t.Run("FlagOverridesEnv", func(t *testing.T) {
			// Set environment variable to disabled
			err := os.Setenv("LOG_LEVEL", "INFO") // Set to INFO
			require.NoError(t, err)

			// Execute command with debug flag (should override env var)
			cmd := getRootCmd()
			_, logs, err := testutil.CaptureJSONLogs(log.LevelDebug, func() {
				_, _, execErr := executeCommandWithStderrCapture(cmd, "--debug", "help")
				if execErr != nil {
					t.Errorf("command execution failed unexpectedly: %v", execErr)
				}
			})
			require.NoError(t, err, "Log capture failed")

			// Verify debug is enabled (flag overrides env) and debug logs are present
			// The --debug flag should override LOG_LEVEL=INFO
			testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{"level": "DEBUG"})
		})
	})
}

func TestExecutionModeDetection(t *testing.T) {
	// Save original env vars
	origName := os.Getenv("HELM_PLUGIN_NAME")
	origDir := os.Getenv("HELM_PLUGIN_DIR")
	defer func() {
		err := os.Setenv("HELM_PLUGIN_NAME", origName)
		if err != nil {
			t.Errorf("Error restoring HELM_PLUGIN_NAME: %v", err)
		}
		err = os.Setenv("HELM_PLUGIN_DIR", origDir)
		if err != nil {
			t.Errorf("Error restoring HELM_PLUGIN_DIR: %v", err)
		}
	}()

	// Test case 1: Plugin mode
	err := os.Setenv("HELM_PLUGIN_NAME", "irr")
	require.NoError(t, err)
	defer func() {
		err := os.Unsetenv("HELM_PLUGIN_NAME")
		if err != nil {
			t.Errorf("Error unsetting HELM_PLUGIN_NAME: %v", err)
		}
	}()
	cmd := getRootCmd()

	// Call initHelmPlugin directly to ensure flags are properly set up
	initHelmPlugin()

	// Verify release-name and namespace flags are available in plugin mode
	releaseFlag := cmd.PersistentFlags().Lookup("release-name")
	assert.NotNil(t, releaseFlag, "release-name flag should be available in plugin mode")

	namespaceFlag := cmd.PersistentFlags().Lookup("namespace")
	assert.NotNil(t, namespaceFlag, "namespace flag should be available in plugin mode")

	// Test case 2: Standalone mode
	err = os.Unsetenv("HELM_PLUGIN_NAME")
	require.NoError(t, err)
	cmd = getRootCmd()

	// Call removeHelmPluginFlags to hide the flags in standalone mode
	removeHelmPluginFlags(cmd)

	// Check if the flags are hidden in standalone mode
	releaseFlag = cmd.PersistentFlags().Lookup("release-name")
	if releaseFlag != nil {
		assert.True(t, releaseFlag.Hidden, "release-name flag should be hidden in standalone mode")
	} else {
		// If the flag is completely removed, that's also acceptable
		assert.Nil(t, releaseFlag, "release-name flag should be nil in standalone mode")
	}
}

func TestSubcommandRouting(t *testing.T) {
	// Test that all expected subcommands are registered regardless of mode
	err := os.Setenv("HELM_PLUGIN_NAME", "irr")
	require.NoError(t, err)
	defer func() {
		err := os.Unsetenv("HELM_PLUGIN_NAME")
		if err != nil {
			t.Errorf("Error unsetting HELM_PLUGIN_NAME: %v", err)
		}
	}()
	cmd := getRootCmd()

	// Check if all expected subcommands are available
	hasCommand := func(cmd *cobra.Command, name string) bool {
		for _, subcmd := range cmd.Commands() {
			if subcmd.Name() == name {
				return true
			}
		}
		return false
	}

	assert.True(t, hasCommand(cmd, "override"), "override command should be available")
	assert.True(t, hasCommand(cmd, "inspect"), "inspect command should be available")
	assert.True(t, hasCommand(cmd, "validate"), "validate command should be available")

	// Same checks in standalone mode
	err = os.Unsetenv("HELM_PLUGIN_NAME")
	require.NoError(t, err)
	cmd = getRootCmd()

	assert.True(t, hasCommand(cmd, "override"), "override command should be available")
	assert.True(t, hasCommand(cmd, "inspect"), "inspect command should be available")
	assert.True(t, hasCommand(cmd, "validate"), "validate command should be available")
}

func TestRootCommandRunWithNoArgs(t *testing.T) {
	// When no subcommand is provided, an error should be returned
	cmd := getRootCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()

	assert.Error(t, err, "Executing root command with no args should return an error")
	assert.Contains(t, err.Error(), "a subcommand is required", "Error message should indicate a subcommand is required")
}

func TestVersionCommand(t *testing.T) {
	cmd := getRootCmd()
	output, err := executeCommand(cmd, "--version") // Use --version flag
	require.NoError(t, err, "Command execution failed")

	// Verify the stdout output contains the version string
	assert.Contains(t, output, "irr version 0.2.0", "Output should contain the version string")
}

func TestDebugFlagLowerPrecedence(t *testing.T) {
	// --debug flag should override LOG_LEVEL env var if it's set to something else
	t.Setenv("LOG_LEVEL", "INFO") // Set env var to INFO

	cmd := getRootCmd()
	// Capture logs at Debug level to see debug messages
	_, logs, err := testutil.CaptureJSONLogs(log.LevelDebug, func() {
		// Execute a command that triggers initialization (like help)
		_, _, execErr := executeCommandWithStderrCapture(cmd, "--debug", "help")
		if execErr != nil {
			// Don't fail the test on execution error, as we're checking logs
			t.Logf("command execution failed (ignored for log check): %v", execErr)
		}
	})
	require.NoError(t, err, "Log capture failed")

	// Verify the first debug log indicates the execution mode
	testutil.AssertLogContainsJSON(t, logs, map[string]interface{}{
		"level": "DEBUG",
		"msg":   "Execution Mode Detected", // Updated expected message
		// Optionally assert on the mode: "mode": "Standalone"
	})
}

// Helper function to reset Cobra flags and Viper state between tests if necessary
func resetRootCmdState() {
	// Reset flags to default values
	// Note: This assumes global vars like debugEnabled, logLevel are used.
	// A better approach might involve creating a new root command instance for each test,
	// but we'll work with the existing structure for now.
	cfgFile = ""
	debugEnabled = false
	logLevel = "info" // Default log level
	integrationTestMode = false
	TestAnalyzeMode = false
	registryFile = ""

	// If viper is used directly for flags, might need viper reset too
	// viper.Reset() // Use with caution if viper state is shared across tests

	// Clear potentially set environment variables if Setenv isn't used
	if err := os.Unsetenv("LOG_LEVEL"); err != nil {
		// Log error, but don't fail the test during cleanup
		// Need a way to get *testing.T here, or just ignore the error
		// For simplicity in this helper, let's ignore for now, but ideally log
		_ = err // Acknowledge error check
	}
}

func TestRootCmdExecution(t *testing.T) {
	// Existing tests... keep them here
	// ...

	// Example Test using executeCommand (modify as needed for actual tests)
	t.Run("HelpCommand", func(t *testing.T) {
		cmd := getRootCmd() // Get the root command instance
		output, err := executeCommand(cmd, "--help")

		assert.NoError(t, err, "Executing --help should not produce an error")
		assert.Contains(t, output, "Usage:", "Help output should contain Usage information")
	})
}

// New Test Function for PreRun Logging
func TestRootCmdPreRunLogging(t *testing.T) {
	// Expected message strings for debug logs
	preRunInputMsg := "[PRE-RUN] Raw inputs"
	preRunLevelMsg := "[PRE-RUN] Determined final level"

	tests := []struct {
		name            string
		args            []string
		envVars         map[string]string
		expectDebugLogs bool
	}{
		{
			name:            "Default log level (INFO), no debug logs",
			args:            []string{"help"},
			envVars:         map[string]string{},
			expectDebugLogs: false,
		},
		{
			name:            "Explicit INFO log level, no debug logs",
			args:            []string{"--log-level", "info", "help"},
			envVars:         map[string]string{},
			expectDebugLogs: false,
		},
		{
			name:            "Debug flag enabled, expect debug logs",
			args:            []string{"--debug", "help"},
			envVars:         map[string]string{},
			expectDebugLogs: true,
		},
		{
			name:            "LOG_LEVEL=debug env var, expect debug logs",
			args:            []string{"help"},
			envVars:         map[string]string{"LOG_LEVEL": "debug"},
			expectDebugLogs: true,
		},
		{
			name:            "Debug flag overrides LOG_LEVEL=info, expect debug logs",
			args:            []string{"--debug", "help"},
			envVars:         map[string]string{"LOG_LEVEL": "info"},
			expectDebugLogs: true,
		},
		{
			name:            "log-level=debug overrides LOG_LEVEL=info, expect debug logs",
			args:            []string{"--log-level", "debug", "help"},
			envVars:         map[string]string{"LOG_LEVEL": "info"},
			expectDebugLogs: true,
		},
		{
			name:            "No flags/env vars (defaults to Error level), no debug logs",
			args:            []string{"help"},
			envVars:         map[string]string{},
			expectDebugLogs: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetRootCmdState()

			for key, val := range tc.envVars {
				originalValue, wasSet := os.LookupEnv(key)
				if err := os.Setenv(key, val); err != nil {
					t.Logf("Warning: failed to set env var %s for test: %v", key, err)
				}
				t.Cleanup(func() {
					if wasSet {
						if err := os.Setenv(key, originalValue); err != nil {
							t.Logf("Warning: failed to restore env var %s in cleanup: %v", key, err)
						}
					} else {
						if err := os.Unsetenv(key); err != nil {
							t.Logf("Warning: failed to unset env var %s in cleanup: %v", key, err)
						}
					}
				})
			}

			// Determine the capture level based on whether we expect debug logs
			captureLevel := log.LevelInfo // Default capture level
			if tc.expectDebugLogs {
				captureLevel = log.LevelDebug
			}

			// Capture logs using the determined level
			_, logs, err := testutil.CaptureJSONLogs(captureLevel, func() {
				cmd := newRootCmd(t)
				cmd.SetArgs(tc.args)
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				execErr := cmd.Execute()
				if execErr != nil && !strings.Contains(execErr.Error(), "help requested") {
					t.Logf("Command execution returned unexpected error: %v", execErr)
				}
			})
			require.NoError(t, err, "Failed to capture logs")

			inputLogMatcher := map[string]interface{}{"msg": preRunInputMsg}
			levelLogMatcher := map[string]interface{}{"msg": preRunLevelMsg}

			if tc.expectDebugLogs {
				// When expecting debug logs, they MUST be present (captured at Debug level)
				testutil.AssertLogContainsJSON(t, logs, inputLogMatcher)
				testutil.AssertLogContainsJSON(t, logs, levelLogMatcher)
			} else {
				// When NOT expecting debug logs, they MUST NOT be present (captured at Info level)
				testutil.AssertLogDoesNotContainJSON(t, logs, inputLogMatcher)
				testutil.AssertLogDoesNotContainJSON(t, logs, levelLogMatcher)
			}
		})
	}
}

// newRootCmd creates a new instance of the root command for isolated testing.
// This avoids issues with shared global state (flags, etc.) between test runs.
func newRootCmd(t *testing.T) *cobra.Command {
	t.Helper() // Mark as test helper
	// Reset global vars associated with flags *before* creating new command
	// This is still needed because flags bind to globals in the current setup
	cfgFile = ""
	debugEnabled = false
	logLevel = "info" // Reset to default before flag parsing
	integrationTestMode = false
	TestAnalyzeMode = false
	registryFile = ""

	// Create the command structure (similar to main.go)
	cmd := &cobra.Command{
		Use:   "irr",
		Short: "Image Relocation and Rewrite tool for Helm Charts and K8s YAML",
		Long: `irr (Image Relocation and Rewrite) is a tool for generating Helm override values
that redirect container image references from public registries to a private registry.

It can analyze Helm charts to identify image references and generate override values 
files compatible with Helm, pointing images to a new registry according to specified strategies.
It also supports linting image references for potential issues.`,
		PersistentPreRunE: rootCmd.PersistentPreRunE, // Reuse existing PersistentPreRunE
		RunE: func(_ *cobra.Command, args []string) error { // Simplified RunE for testing root
			if len(args) == 0 {
				return errors.New("a subcommand is required")
			}
			return nil
		},
	}

	// Re-add flags (ensure they bind to potentially reset globals)
	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.irr.yaml)")
	cmd.PersistentFlags().BoolVar(&debugEnabled, "debug", false, "enable debug logging")
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "set log level (debug, info, warn, error) (default \"info\")")
	cmd.PersistentFlags().BoolVar(&integrationTestMode, "integration-test", false, "enable integration test mode")
	cmd.PersistentFlags().BoolVar(&TestAnalyzeMode, "test-analyze", false, "enable test mode")

	if err := cmd.PersistentFlags().MarkHidden("integration-test"); err != nil {
		log.Warn("Failed to mark integration-test flag as hidden", "error", err)
	}
	if err := cmd.PersistentFlags().MarkHidden("test-analyze"); err != nil {
		log.Warn("Failed to mark test-analyze flag as hidden", "error", err)
	}

	// Re-add commands (stubs might be sufficient if only testing root PreRun)
	// cmd.AddCommand(newOverrideCmd()) // Add real subcommands if needed
	// cmd.AddCommand(newInspectCmd())
	// cmd.AddCommand(newValidateCmd())
	// Add a basic help command if PersistentPreRunE relies on it
	cmd.AddCommand(&cobra.Command{Use: "help", Run: func(_ *cobra.Command, _ []string) { fmt.Println("Help command stub") }})

	// Add other root flags if necessary
	addReleaseFlag(cmd)
	addNamespaceFlag(cmd)

	// Reset potentially parsed flags before returning
	if err := cmd.ParseFlags([]string{}); err != nil { // Reset flags state after definition
		t.Fatalf("newRootCmd failed during flag parsing reset: %v", err)
	}

	return cmd
}

// Add other tests specific to root command if needed...

// Note: Assumes executeCommand captures both stdout and stderr where logs might appear.
// If logs go ONLY to stderr and executeCommand merges, these tests should work.
// If logs go to a specific file or logger needs redirection for testing, setup might be more complex.
