package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
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
	stderrReader, stderrWriter, _ := os.Pipe()
	os.Stderr = stderrWriter

	// Execute command
	err = root.Execute()

	// Close writer to release pipe
	stderrWriter.Close()

	// Restore original stderr
	os.Stderr = oldStderr

	// Read stderr from pipe into buffer
	stderrBuf := new(bytes.Buffer)
	io.Copy(stderrBuf, stderrReader)

	return cmdBuf.String(), stderrBuf.String(), err
}

// TestDebugFlagAndEnvVarInteraction tests how debug flag and environment variables interact
func TestDebugFlagAndEnvVarInteraction(t *testing.T) {
	// Save and restore original state
	origEnv := os.Getenv("IRR_DEBUG")
	defer os.Setenv("IRR_DEBUG", origEnv)

	// Part 1: Test command-line flags (simpler, less dependent on environment)
	t.Run("FlagTests", func(t *testing.T) {
		t.Run("NoFlag_DebugDisabled", func(t *testing.T) {
			// Unset environment variable to avoid interference
			os.Unsetenv("IRR_DEBUG")

			// Reset debug state
			debug.Enabled = false

			// Execute command without debug flag
			cmd := getRootCmd()
			_, stderr, _ := executeCommandWithStderrCapture(cmd, "help")

			// Verify debug is disabled and no debug logs are present
			assert.False(t, debug.Enabled, "debug.Enabled should be false without --debug flag")
			assert.False(t, strings.Contains(stderr, "[DEBUG"), "No debug logs should be present")
		})

		t.Run("WithFlag_DebugEnabled", func(t *testing.T) {
			// Unset environment variable to avoid interference
			os.Unsetenv("IRR_DEBUG")

			// Reset debug state
			debug.Enabled = false

			// Execute command with debug flag
			cmd := getRootCmd()
			_, stderr, _ := executeCommandWithStderrCapture(cmd, "--debug", "help")

			// Verify debug is enabled and debug logs are present
			assert.True(t, debug.Enabled, "debug.Enabled should be true with --debug flag")
			assert.True(t, strings.Contains(stderr, "[DEBUG"), "Debug logs should be present")
		})

		t.Run("FlagOverridesEnv", func(t *testing.T) {
			// Set environment variable to disabled
			os.Setenv("IRR_DEBUG", "false")

			// Reset debug state
			debug.Enabled = false

			// Execute command with debug flag (should override env var)
			cmd := getRootCmd()
			_, stderr, _ := executeCommandWithStderrCapture(cmd, "--debug", "help")

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
			r, w, _ := os.Pipe()
			os.Stderr = w

			// Call debug.Init
			debug.Init(forceEnable)

			// Restore stderr
			w.Close()
			os.Stderr = oldStderr

			// Read captured output
			buf := new(bytes.Buffer)
			io.Copy(buf, r)
			return buf.String()
		}

		t.Run("EnvVar_True", func(t *testing.T) {
			// Setup: Set env var to true
			os.Setenv("IRR_DEBUG", "true")

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
			os.Setenv("IRR_DEBUG", "false")

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
			os.Setenv("IRR_DEBUG", "notabool")

			// Set debug state to true to verify it gets disabled
			debug.Enabled = true

			// Call Init and capture output
			output := captureDebugInit(false) // false = don't force enable

			// Verify debug is disabled and warning is logged
			assert.False(t, debug.Enabled, "debug.Enabled should be false with invalid value")
			assert.True(t, strings.Contains(output, "Invalid boolean value for IRR_DEBUG"), "Should warn about invalid value")
		})

		t.Run("EnvVar_Empty", func(t *testing.T) {
			// Setup: Set env var to empty string
			os.Setenv("IRR_DEBUG", "")

			// Set debug state to true to verify it gets disabled
			debug.Enabled = true

			// Call Init and capture output
			output := captureDebugInit(false) // false = don't force enable

			// Verify debug is disabled and empty env var should behave as if not set
			assert.False(t, debug.Enabled, "debug.Enabled should be false with empty value")

			// NOTE: Empty values may produce a warning in some versions of the debug package
			// What matters is that debug.Enabled is set to false
			t.Logf("Warning output with empty env var: %q", output)
		})

		t.Run("ForceEnable_OverridesEnvVar", func(t *testing.T) {
			// Setup: Set env var to false
			os.Setenv("IRR_DEBUG", "false")

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
