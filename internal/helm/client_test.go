package helm

import (
	"bytes"
	"context"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHelmClient verifies that NewHelmClient creates a non-nil client without errors.
func TestNewHelmClient(t *testing.T) {
	client, err := NewHelmClient()

	require.NoError(t, err, "NewHelmClient should not return an error in a standard environment")
	assert.NotNil(t, client, "NewHelmClient should return a non-nil client")
	assert.NotNil(t, client.settings, "Client settings should be initialized")
	assert.NotNil(t, client.actionConfig, "Client actionConfig should be initialized")

	// Note: This test does not cover the error path within actionConfig.Init,
	// as that would require deeper mocking of Helm SDK internals.
}

// TestGetActionConfig verifies that getActionConfig returns a valid config.
func TestGetActionConfig(t *testing.T) {
	// First, create a RealHelmClient instance
	client, err := NewHelmClient()
	require.NoError(t, err, "Failed to create Helm client for test setup")
	require.NotNil(t, client, "Helm client is nil during test setup")

	t.Run("valid namespace", func(t *testing.T) {
		cfg, err := client.getActionConfig("test-namespace")
		require.NoError(t, err, "getActionConfig failed for valid namespace")
		assert.NotNil(t, cfg, "getActionConfig should return non-nil config")
		// We could potentially check if the namespace was set correctly if the struct exposed it,
		// but Helm's action.Configuration might not make that easy.
	})

	t.Run("empty namespace uses default", func(t *testing.T) {
		// Assumes the default namespace from client.settings is used
		cfg, err := client.getActionConfig("")
		require.NoError(t, err, "getActionConfig failed for empty namespace")
		assert.NotNil(t, cfg, "getActionConfig should return non-nil config for empty namespace")
	})

	// Note: Testing the error path within cfg.Init is difficult without
	// mocking Helm internals (like RESTClientGetter).
}

// TestProcessHelmLogs verifies that processHelmLogs properly processes Helm SDK log output.
func TestProcessHelmLogs(t *testing.T) {
	// Create a buffer with some test log content
	var buffer bytes.Buffer
	buffer.WriteString("First log line\nSecond log line\nThird log line")

	// Create a log capture to verify our log output
	logOutput, err := testutil.CaptureLogOutput(log.LevelInfo, func() {
		// Call the function being tested
		processHelmLogs(&buffer)
	})
	require.NoError(t, err, "Log capture should not fail")

	// Verify logs were properly processed - they'll be combined because of how
	// the log output is captured in JSON format
	assert.Contains(t, logOutput, "[Helm SDK] First log line")
	// The test log output contains all lines in a single JSON log message

	// Test with empty buffer
	var emptyBuffer bytes.Buffer
	emptyLogOutput, err := testutil.CaptureLogOutput(log.LevelInfo, func() {
		processHelmLogs(&emptyBuffer)
	})
	require.NoError(t, err, "Log capture should not fail")
	assert.Empty(t, emptyLogOutput, "Empty buffer should produce no logs")

	// Test with whitespace-only lines
	var whitespaceBuffer bytes.Buffer
	whitespaceBuffer.WriteString("  \n\t\n   ")
	whitespaceLogOutput, err := testutil.CaptureLogOutput(log.LevelInfo, func() {
		processHelmLogs(&whitespaceBuffer)
	})
	require.NoError(t, err, "Log capture should not fail")
	assert.Empty(t, whitespaceLogOutput, "Whitespace-only buffer should produce no logs")
}

// TestGetCurrentNamespace verifies that GetCurrentNamespace returns the namespace from settings.
func TestGetCurrentNamespace(t *testing.T) {
	// Create a client instance
	client, err := NewHelmClient()
	require.NoError(t, err, "Failed to create Helm client for test setup")

	// Get the namespace
	namespace := client.GetCurrentNamespace()

	// Verify it matches the client's settings namespace
	assert.Equal(t, client.settings.Namespace(), namespace,
		"GetCurrentNamespace should return the same value as settings.Namespace()")

	// We can't easily test with different namespaces without modifying unexported fields
	// or creating a deeper mock, but this covers the basic functionality
}

// TestFindChartForRelease tests the FindChartForRelease function's error handling
func TestFindChartForRelease(t *testing.T) {
	// Note: This test primarily verifies error handling paths rather than full functionality,
	// as proper testing requires mocking Helm SDK calls that are challenging to mock

	// Create a client instance for basic test configuration
	client, err := NewHelmClient()
	require.NoError(t, err, "Failed to create client for test")

	t.Run("invalid namespace should error", func(t *testing.T) {
		// We'll simulate an error in getActionConfig by replacing it temporarily
		// However, we can't easily do this without refactoring the code, so we'll
		// just skip direct simulation of this error path
		//
		// Instead, let's verify that the function itself exists and returns
		// an expected error when the release doesn't exist
		path, err := client.FindChartForRelease(context.Background(), "non-existent-release", "test-namespace")
		assert.Error(t, err, "Should error on non-existent release")
		assert.Equal(t, "", path, "Should return empty path on error")
		assert.Contains(t, err.Error(), "release", "Error should mention release")
	})

	// We can't easily test successful paths without mocking the Helm SDK's
	// action.Get and other low-level components
}

// TestRealClientValidateRelease tests the ValidateRelease method of the real client
func TestRealClientValidateRelease(t *testing.T) {
	// Create a client instance
	client, err := NewHelmClient()
	require.NoError(t, err, "Failed to create client for test")

	// The current implementation is just a placeholder that logs a warning and returns nil
	// So we'll test that it doesn't return an error
	logOutput, err := testutil.CaptureLogOutput(log.LevelWarn, func() {
		err := client.ValidateRelease(context.Background(), "test-release", "test-namespace", []string{"values.yaml"}, "1.23.0")
		assert.NoError(t, err, "Placeholder implementation should not return an error")
	})
	require.NoError(t, err, "Log capture should not fail")
	assert.Contains(t, logOutput, "not fully implemented", "Should log a warning about not being fully implemented")
}
