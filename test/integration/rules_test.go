package integration

import (
	"testing"

	"github.com/lalbers/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// In test/integration/integration_test.go or rules_integration_test.go

func TestRulesSystemIntegration(t *testing.T) {
	bitnamiChartPath := testutil.GetChartPath("clickhouse-operator") // Or minimal-git-image
	nonBitnamiChartPath := testutil.GetChartPath("minimal-test")

	// --- Test Case 1: Bitnami Chart, Rules Enabled (Default) ---
	t.Run("Bitnami_RulesEnabled", func(t *testing.T) {
		h := NewTestHarness(t)
		defer h.Cleanup()

		h.SetupChart(bitnamiChartPath)
		h.SetRegistries("harbor.test.local", []string{"docker.io"}) // Adjust source as needed

		err := h.GenerateOverrides() // Rules are enabled by default
		require.NoError(t, err, "GenerateOverrides failed")

		overrides, err := h.getOverrides()
		require.NoError(t, err, "Failed to get overrides")

		// Assert that the rule was applied
		value, pathExists := h.GetValueFromOverrides(overrides, "global", "security", "allowInsecureImages")
		assert.True(t, pathExists, "Expected path global.security.allowInsecureImages to exist")
		assert.Equal(t, true, value, "allowInsecureImages should be true when rules are enabled")
	})

	// --- Test Case 2: Bitnami Chart, Rules Disabled ---
	t.Run("Bitnami_RulesDisabled", func(t *testing.T) {
		h := NewTestHarness(t)
		defer h.Cleanup()

		h.SetupChart(bitnamiChartPath)
		h.SetRegistries("harbor.test.local", []string{"docker.io"})

		// Generate overrides with rules disabled
		err := h.GenerateOverrides("--disable-rules") // Add the flag
		require.NoError(t, err, "GenerateOverrides with --disable-rules failed")

		overrides, err := h.getOverrides()
		require.NoError(t, err, "Failed to get overrides")

		// Assert that the rule was NOT applied
		_, pathExists := h.GetValueFromOverrides(overrides, "global", "security", "allowInsecureImages")
		assert.False(t, pathExists, "Path global.security.allowInsecureImages should NOT exist when rules are disabled")
		// Optional: If the original chart HAD this value, assert it wasn't added/modified *by the rule*.
		// This might require comparing against original chart values or a more complex check.
		// For now, checking absence is simpler if the original doesn't have it.
	})

	// --- Test Case 3: Non-Bitnami Chart, Rules Enabled ---
	t.Run("NonBitnami_RulesEnabled", func(t *testing.T) {
		h := NewTestHarness(t)
		defer h.Cleanup()

		h.SetupChart(nonBitnamiChartPath)
		h.SetRegistries("harbor.test.local", []string{"docker.io"}) // Adjust source as needed

		err := h.GenerateOverrides() // Rules enabled by default
		require.NoError(t, err, "GenerateOverrides failed for non-Bitnami chart")

		overrides, err := h.getOverrides()
		require.NoError(t, err, "Failed to get overrides")

		// Assert that the rule was NOT applied
		_, pathExists := h.GetValueFromOverrides(overrides, "global", "security", "allowInsecureImages")
		assert.False(t, pathExists, "Path global.security.allowInsecureImages should NOT exist for non-Bitnami chart")
	})
}

// Helper function potentially added to TestHarness or test file
func (h *TestHarness) GetValueFromOverrides(overrides map[string]interface{}, path ...string) (interface{}, bool) {
	var current interface{} = overrides
	for _, key := range path {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false // Path doesn't lead to a map intermediate
		}
		value, exists := currentMap[key]
		if !exists {
			return nil, false // Key doesn't exist at this level
		}
		current = value
	}
	return current, true
}
