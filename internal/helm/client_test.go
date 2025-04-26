package helm

import (
	"testing"

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
