//go:build integration

// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to run log level test cases
func runLogLevelTest(
	t *testing.T,
	name string,
	baseArgs []string,
	envOverrides map[string]string,
	extraArgs, contains, notContains []string,
) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		harness := NewTestHarness(t)
		defer harness.Cleanup()

		setupMinimalTestChart(t, harness)
		args := baseArgs
		args = append(args, "--chart-path", harness.chartPath, "--output-file", harness.GeneratedOverridesFile())
		args = append(args, extraArgs...)
		_, stderr, err := harness.ExecuteIRRWithStderr(envOverrides, false, args...)
		require.NoError(t, err)
		for _, s := range contains {
			assert.Contains(t, stderr, s)
		}
		for _, s := range notContains {
			assert.NotContains(t, stderr, s)
		}
	})
}

// TestDefaultLogLevels tests the log level behavior under various configurations.
func TestDefaultLogLevels(t *testing.T) {
	baseArgs := []string{
		"override",
		"--target-registry", "test.registry.io",
		"--source-registries", "unmapped.registry.example.com",
	}

	runLogLevelTest(
		t, "Normal_Default_(ERROR)", baseArgs, nil,
		[]string{"--log-level=error"},
		nil,
		[]string{"\"level\":\"INFO\"", "\"level\":\"WARN\"", "\"level\":\"DEBUG\""},
	)
	runLogLevelTest(
		t, "Test_Mode_Default_(INFO)", baseArgs,
		map[string]string{"LOG_LEVEL": ""},
		nil,
		[]string{"\"level\":\"INFO\""},
		[]string{"\"level\":\"DEBUG\""},
	)
	runLogLevelTest(
		t, "Flag_Override_(--log-level_warn)", baseArgs,
		map[string]string{"LOG_LEVEL": ""},
		[]string{"--log-level=warn"},
		[]string{"\"level\":\"WARN\""},
		[]string{"\"level\":\"INFO\"", "\"level\":\"DEBUG\""},
	)
	runLogLevelTest(
		t, "--debug_Flag_Override", baseArgs, nil,
		[]string{"--debug"},
		[]string{"\"level\":\"DEBUG\""},
		nil,
	)
	runLogLevelTest(
		t, "Env_Var_Override_(LOG_LEVEL=debug)", baseArgs,
		map[string]string{"LOG_LEVEL": "debug"},
		nil,
		[]string{"\"level\":\"DEBUG\""},
		nil,
	)
	runLogLevelTest(
		t, "Flag_warn_overrides_Env_debug", baseArgs,
		map[string]string{"LOG_LEVEL": "debug"},
		[]string{"--log-level=warn"},
		[]string{"\"level\":\"WARN\""},
		[]string{"\"level\":\"INFO\"", "\"level\":\"DEBUG\""},
	)
	runLogLevelTest(
		t, "Flag_debug_overrides_Env_info", baseArgs,
		map[string]string{"LOG_LEVEL": "info"},
		[]string{"--debug"},
		[]string{"\"level\":\"DEBUG\""},
		nil,
	)
	runLogLevelTest(
		t, "Flag_debug_overrides_Flag_error", baseArgs, nil,
		[]string{"--debug", "--log-level=error"},
		[]string{"\"level\":\"DEBUG\""},
		nil,
	)
	runLogLevelTest(
		t, "Env_error_overrides_Default_info", baseArgs,
		map[string]string{"LOG_LEVEL": "error"},
		nil,
		nil,
		[]string{"\"level\":\"INFO\"", "\"level\":\"WARN\"", "\"level\":\"DEBUG\""},
	)
}
