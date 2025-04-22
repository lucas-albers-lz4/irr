package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
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
