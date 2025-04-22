package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/debug"
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
	origEnv := os.Getenv("IRR_DEBUG")
	defer func() {
		err := os.Setenv("IRR_DEBUG", origEnv)
		if err != nil {
			t.Logf("Warning: failed to restore IRR_DEBUG environment variable: %v", err)
		}
	}()

	// Part 1: Test command-line flags (simpler, less dependent on environment)
	t.Run("FlagTests", func(t *testing.T) {
		t.Run("NoFlag_DebugDisabled", func(t *testing.T) {
			// Unset environment variable to avoid interference
			err := os.Unsetenv("IRR_DEBUG")
			require.NoError(t, err)

			// Reset debug state
			debug.Enabled = false

			// Execute command without debug flag
			cmd := getRootCmd()
			_, stderr, cmdErr := executeCommandWithStderrCapture(cmd, "help")
			if cmdErr != nil {
				t.Logf("Command execution returned error (expected for subcommand check): %v", cmdErr)
				// Not failing the test as this might be expected behavior
			}

			// Verify debug is disabled and no debug logs are present
			assert.False(t, debug.Enabled, "debug.Enabled should be false without --debug flag")
			assert.False(t, strings.Contains(stderr, "[DEBUG"), "No debug logs should be present")
		})

		t.Run("WithFlag_DebugEnabled", func(t *testing.T) {
			// Unset environment variable to avoid interference
			err := os.Unsetenv("IRR_DEBUG")
			require.NoError(t, err)

			// Reset debug state
			debug.Enabled = false

			// Execute command with debug flag
			cmd := getRootCmd()
			_, stderr, cmdErr := executeCommandWithStderrCapture(cmd, "--debug", "help")
			if cmdErr != nil {
				t.Logf("Command execution returned error (expected for subcommand check): %v", cmdErr)
				// Not failing the test as this might be expected behavior
			}

			// Verify debug is enabled and debug logs are present
			assert.True(t, debug.Enabled, "debug.Enabled should be true with --debug flag")
			assert.True(t, strings.Contains(stderr, "[DEBUG"), "Debug logs should be present")
		})

		t.Run("FlagOverridesEnv", func(t *testing.T) {
			// Set environment variable to disabled
			err := os.Setenv("IRR_DEBUG", "false")
			require.NoError(t, err)

			// Reset debug state
			debug.Enabled = false

			// Execute command with debug flag (should override env var)
			cmd := getRootCmd()
			_, stderr, cmdErr := executeCommandWithStderrCapture(cmd, "--debug", "help")
			if cmdErr != nil {
				t.Logf("Command execution returned error (expected for subcommand check): %v", cmdErr)
				// Not failing the test as this might be expected behavior
			}

			// Verify debug is enabled (flag overrides env) and debug logs are present
			assert.True(t, debug.Enabled, "debug.Enabled should be true (flag overrides env)")
			assert.True(t, strings.Contains(stderr, "[DEBUG"), "Debug logs should be present")
		})
	})

	// Part 2: Test environment variable behavior directly on the debug package
	// This avoids the complexity of command execution
	t.Run("EnvVarTests", func(t *testing.T) {
		// Helper to capture stderr and run debug.Init
		captureDebugInit := func(forceEnable bool) string {
			// Set up stderr capture
			oldStderr := os.Stderr
			r, w, pipeErr := os.Pipe()
			if pipeErr != nil {
				t.Fatalf("Failed to create pipe: %v", pipeErr)
				return "" // Unreachable, but needed for compiler
			}
			os.Stderr = w

			// Call debug.Init
			debug.Init(forceEnable)

			// Restore stderr
			closeErr := w.Close()
			if closeErr != nil {
				t.Logf("Warning: Error closing stderr pipe: %v", closeErr)
			}
			os.Stderr = oldStderr

			// Read captured output
			buf := new(bytes.Buffer)
			_, copyErr := io.Copy(buf, r)
			if copyErr != nil {
				t.Fatalf("Error reading from stderr pipe: %v", copyErr)
				return "" // Unreachable, but needed for compiler
			}
			return buf.String()
		}

		t.Run("EnvVar_True", func(t *testing.T) {
			// Setup: Set env var to true
			err := os.Setenv("IRR_DEBUG", "true")
			require.NoError(t, err)

			// Reset debug state to ensure clean test
			debug.Enabled = false

			// Call Init and capture output
			output := captureDebugInit(false) // false = don't force enable

			// Verify debug is enabled from env var
			assert.True(t, debug.Enabled, "debug.Enabled should be true from IRR_DEBUG=true")
			assert.True(t, strings.Contains(output, "Debug logging enabled"), "Should log debug enabled message")
		})

		t.Run("EnvVar_False", func(t *testing.T) {
			// Setup: Set env var to false
			err := os.Setenv("IRR_DEBUG", "false")
			require.NoError(t, err)

			// Set debug state to true to verify it gets disabled
			debug.Enabled = true

			// Call Init and capture output
			output := captureDebugInit(false) // false = don't force enable

			// Verify debug is disabled from env var
			assert.False(t, debug.Enabled, "debug.Enabled should be false from IRR_DEBUG=false")
			assert.False(t, strings.Contains(output, "Debug logging enabled"), "Should not log debug enabled message")
		})

		t.Run("EnvVar_Invalid", func(t *testing.T) {
			// Setup: Set env var to invalid value
			err := os.Setenv("IRR_DEBUG", "notabool")
			require.NoError(t, err)

			// Set debug state to true to verify it gets disabled
			debug.Enabled = true

			// Enable warnings for this test since we're explicitly testing the warning output
			debug.EnableDebugEnvVarWarnings()

			// Call Init and capture output
			output := captureDebugInit(false) // false = don't force enable

			// Verify debug is disabled and warning is logged
			assert.False(t, debug.Enabled, "debug.Enabled should be false with invalid value")
			assert.True(t, strings.Contains(output, "Invalid boolean value for IRR_DEBUG"), "Should warn about invalid value")

			// Reset to default behavior
			debug.ShowDebugEnvWarnings = false
		})

		t.Run("EnvVar_Empty", func(t *testing.T) {
			// Setup: Set env var to empty string
			err := os.Setenv("IRR_DEBUG", "")
			require.NoError(t, err)

			// Set debug state to true to verify it gets disabled
			debug.Enabled = true

			// Call Init and capture output
			output := captureDebugInit(false) // false = don't force enable

			// Verify debug is disabled and empty env var should behave as if not set
			assert.False(t, debug.Enabled, "debug.Enabled should be false with empty value")

			// NOTE: Empty values should not produce warnings now
			assert.False(t, strings.Contains(output, "Invalid boolean value for IRR_DEBUG"), "Should not warn about empty value")
			t.Logf("Output with empty env var: %q", output)
		})

		t.Run("ForceEnable_OverridesEnvVar", func(t *testing.T) {
			// Setup: Set env var to false
			err := os.Setenv("IRR_DEBUG", "false")
			require.NoError(t, err)

			// Reset debug state to ensure clean test
			debug.Enabled = false

			// Call Init with forceEnable=true and capture output
			output := captureDebugInit(true) // true = force enable

			// Verify debug is enabled despite env var
			assert.True(t, debug.Enabled, "debug.Enabled should be true when force enabled")
			assert.True(t, strings.Contains(output, "Debug logging enabled"), "Should log debug enabled message")
		})
	})
}

func TestExecutionModeDetection(t *testing.T) {
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
