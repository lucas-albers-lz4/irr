// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultLogLevels tests the log level behavior under various configurations.
func TestDefaultLogLevels(t *testing.T) {
	baseArgs := []string{
		"override", // Use 'override' command as it exercises logging
		"--target-registry", "test.registry.io",
		"--source-registries", "docker.io",
	}

	// Test Case 1: Normal default (should be ERROR level)
	t.Run("Normal_Default_(ERROR)", func(t *testing.T) {
		t.Parallel() // Allow parallel execution
		harness := NewTestHarness(t)
		defer harness.Cleanup()
		setupMinimalTestChart(t, harness) // Create the minimal chart

		commonArgs := append(baseArgs, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		// Explicitly set log level to error for this test case
		args := append(commonArgs, "--log-level=error")

		_, stderr, err := harness.ExecuteIRRWithStderr(nil, args...)
		require.NoError(t, err)

		// With level=error, no INFO, WARN, or DEBUG messages should appear
		assert.NotContains(t, stderr, `"level":"INFO"`, "stderr should NOT contain INFO messages with default ERROR level")
		assert.NotContains(t, stderr, `"level":"WARN"`, "stderr should NOT contain WARN messages with default ERROR level")
		assert.NotContains(t, stderr, `"level":"DEBUG"`, "stderr should NOT contain DEBUG messages with default ERROR level")
		// Check if any error messages appeared (optional, might be none on success)
		// assert.Contains(t, stderr, `"level":"ERROR"`, "stderr might contain ERROR messages")
	})

	// Test Case 2: Test mode default (should be INFO level due to --integration-test)
	t.Run("Test_Mode_Default_(INFO)", func(t *testing.T) {
		t.Parallel() // Allow parallel execution
		harness := NewTestHarness(t)
		defer harness.Cleanup()
		setupMinimalTestChart(t, harness)

		// Harness automatically adds --integration-test
		commonArgs := append(baseArgs, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		// No explicit log level flag -> should default to INFO because harness adds --integration-test
		args := commonArgs

		_, stderr, err := harness.ExecuteIRRWithStderr(nil, args...)
		require.NoError(t, err)

		// INFO is the default, WARN and ERROR might appear
		assert.Contains(t, stderr, `"level":"INFO"`, "stderr should contain INFO messages with test mode INFO level")
		// assert.Contains(t, stderr, `"level":"WARN"`, "stderr might contain WARN messages")
		// assert.Contains(t, stderr, `"level":"ERROR"`, "stderr might contain ERROR messages")
		assert.NotContains(t, stderr, `"level":"DEBUG"`, "stderr should NOT contain DEBUG messages with test mode INFO level")
	})

	// Test Case 3: Flag override (--log-level warn)
	t.Run("Flag_Override_(--log-level_warn)", func(t *testing.T) {
		t.Parallel() // Allow parallel execution
		harness := NewTestHarness(t)
		defer harness.Cleanup()
		setupMinimalTestChart(t, harness)

		commonArgs := append(baseArgs, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		args := append(commonArgs, "--log-level=warn") // Override level

		// Explicitly ensure LOG_LEVEL is not set for the subprocess in this test
		envOverrides := map[string]string{"LOG_LEVEL": ""}

		_, stderr, err := harness.ExecuteIRRWithStderr(envOverrides, args...)
		require.NoError(t, err)

		// WARN and ERROR might appear
		assert.Contains(t, stderr, `"level":"WARN"`, "stderr should contain WARN messages with WARN level")
		// assert.Contains(t, stderr, `"level":"ERROR"`, "stderr might contain ERROR messages")
		assert.NotContains(t, stderr, `"level":"INFO"`, "stderr should NOT contain INFO messages with WARN level")
		assert.NotContains(t, stderr, `"level":"DEBUG"`, "stderr should NOT contain DEBUG messages with WARN level")
	})

	// Test Case 4: Flag override (--debug)
	t.Run("--debug_Flag_Override", func(t *testing.T) {
		t.Parallel() // Allow parallel execution
		harness := NewTestHarness(t)
		defer harness.Cleanup()
		setupMinimalTestChart(t, harness)

		commonArgs := append(baseArgs, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		args := append(commonArgs, "--debug") // Override level via --debug

		_, stderr, err := harness.ExecuteIRRWithStderr(nil, args...)
		require.NoError(t, err)

		// All levels might appear
		assert.Contains(t, stderr, `"level":"DEBUG"`, "stderr should contain DEBUG messages with DEBUG level")
		// assert.Contains(t, stderr, `"level":"INFO"`, "stderr might contain INFO messages")
		// assert.Contains(t, stderr, `"level":"WARN"`, "stderr might contain WARN messages")
		// assert.Contains(t, stderr, `"level":"ERROR"`, "stderr might contain ERROR messages")
	})

	// Test Case 5: Environment variable override (LOG_LEVEL=debug)
	t.Run("Env_Var_Override_(LOG_LEVEL=debug)", func(t *testing.T) {
		t.Parallel() // Allow parallel execution
		harness := NewTestHarness(t)
		defer harness.Cleanup()
		setupMinimalTestChart(t, harness)

		commonArgs := append(baseArgs, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		args := commonArgs // No flag overrides

		// Override environment variable for the subprocess
		envOverrides := map[string]string{"LOG_LEVEL": "debug"}

		_, stderr, err := harness.ExecuteIRRWithStderr(envOverrides, args...)
		require.NoError(t, err)

		// All levels might appear
		assert.Contains(t, stderr, `"level":"DEBUG"`, "stderr should contain DEBUG messages with DEBUG level")
	})

	// --- NEW PRECEDENCE TESTS --- //

	// Test Case 6: Flag (--log-level=warn) vs Env Var (LOG_LEVEL=debug)
	t.Run("Flag_warn_overrides_Env_debug", func(t *testing.T) {
		t.Parallel() // Allow parallel execution
		harness := NewTestHarness(t)
		defer harness.Cleanup()
		setupMinimalTestChart(t, harness)

		commonArgs := append(baseArgs, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		args := append(commonArgs, "--log-level=warn") // Flag should win
		envOverrides := map[string]string{"LOG_LEVEL": "debug"}

		_, stderr, err := harness.ExecuteIRRWithStderr(envOverrides, args...)
		require.NoError(t, err)

		// Expect WARN level: WARN/ERROR ok, INFO/DEBUG not ok.
		assert.Contains(t, stderr, `"level":"WARN"`, "stderr should contain WARN messages") // Or ERROR
		assert.NotContains(t, stderr, `"level":"INFO"`, "stderr should NOT contain INFO messages when flag=warn overrides env=debug")
		assert.NotContains(t, stderr, `"level":"DEBUG"`, "stderr should NOT contain DEBUG messages when flag=warn overrides env=debug")
	})

	// Test Case 7: Flag (--debug) vs Env Var (LOG_LEVEL=info)
	t.Run("Flag_debug_overrides_Env_info", func(t *testing.T) {
		t.Parallel() // Allow parallel execution
		harness := NewTestHarness(t)
		defer harness.Cleanup()
		setupMinimalTestChart(t, harness)

		commonArgs := append(baseArgs, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		args := append(commonArgs, "--debug") // Flag should win
		envOverrides := map[string]string{"LOG_LEVEL": "info"}

		_, stderr, err := harness.ExecuteIRRWithStderr(envOverrides, args...)
		require.NoError(t, err)

		// Expect DEBUG level: All levels ok, but DEBUG must be present.
		assert.Contains(t, stderr, `"level":"DEBUG"`, "stderr should contain DEBUG messages when flag=debug overrides env=info")
	})

	// Test Case 8: Flag (--debug) vs Flag (--log-level=error)
	t.Run("Flag_debug_overrides_Flag_error", func(t *testing.T) {
		t.Parallel() // Allow parallel execution
		harness := NewTestHarness(t)
		defer harness.Cleanup()
		setupMinimalTestChart(t, harness)

		commonArgs := append(baseArgs, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		// Both flags are present, --debug should win
		args := append(commonArgs, "--debug", "--log-level=error")

		_, stderr, err := harness.ExecuteIRRWithStderr(nil, args...)
		require.NoError(t, err)

		// Expect DEBUG level: All levels ok, but DEBUG must be present.
		assert.Contains(t, stderr, `"level":"DEBUG"`, "stderr should contain DEBUG messages when flag=debug overrides flag=error")
	})

	// Test Case 9: Env Var (LOG_LEVEL=error) vs Default (Test Mode INFO)
	t.Run("Env_error_overrides_Default_info", func(t *testing.T) {
		t.Parallel() // Allow parallel execution
		harness := NewTestHarness(t)
		defer harness.Cleanup()
		setupMinimalTestChart(t, harness)

		commonArgs := append(baseArgs, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		args := commonArgs // No flags passed, rely on env var and default
		envOverrides := map[string]string{"LOG_LEVEL": "error"}

		_, stderr, err := harness.ExecuteIRRWithStderr(envOverrides, args...)
		require.NoError(t, err)

		// Expect ERROR level: Only ERROR ok.
		assert.NotContains(t, stderr, `"level":"INFO"`, "stderr should NOT contain INFO messages when env=error overrides default=info")
		assert.NotContains(t, stderr, `"level":"WARN"`, "stderr should NOT contain WARN messages when env=error overrides default=info")
		assert.NotContains(t, stderr, `"level":"DEBUG"`, "stderr should NOT contain DEBUG messages when env=error overrides default=info")
	})
}
