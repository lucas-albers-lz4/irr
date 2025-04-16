package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/chart"
)

func TestRegistry_GetRules(t *testing.T) {
	// Create a new registry
	registry := NewRegistry()

	// Get the rules
	rules := registry.GetRules()

	// Verify we have at least the default rules
	assert.NotEmpty(t, rules, "Registry should have at least one default rule")

	// Verify the default Bitnami security bypass rule is present
	foundBitnamiRule := false
	for _, rule := range rules {
		if rule.Name() == "bitnami-security-bypass" {
			foundBitnamiRule = true
			break
		}
	}
	assert.True(t, foundBitnamiRule, "Registry should contain the Bitnami security bypass rule")

	// Add a custom rule
	customRule := NewBaseRule("custom-rule", "A custom test rule", []Parameter{}, 100)
	registry.AddRule(&customRule)

	// Get the updated rules
	updatedRules := registry.GetRules()

	// Verify the custom rule was added
	assert.Equal(t, len(rules)+1, len(updatedRules), "Registry should have one more rule after adding a custom rule")

	// Verify the custom rule is present
	foundCustomRule := false
	for _, rule := range updatedRules {
		if rule.Name() == "custom-rule" {
			foundCustomRule = true
			break
		}
	}
	assert.True(t, foundCustomRule, "Registry should contain the custom rule")
}

func TestRegistry_IsEnabled(t *testing.T) {
	// Create a new registry
	registry := NewRegistry()

	// By default, registry should be enabled
	assert.True(t, registry.IsEnabled(), "Registry should be enabled by default")

	// Disable the registry
	registry.SetEnabled(false)

	// Verify the registry is now disabled
	assert.False(t, registry.IsEnabled(), "Registry should be disabled after SetEnabled(false)")

	// Enable the registry again
	registry.SetEnabled(true)

	// Verify the registry is enabled again
	assert.True(t, registry.IsEnabled(), "Registry should be enabled after SetEnabled(true)")
}

func TestRegistry_SetEnabled(t *testing.T) {
	// Create a new registry
	registry := NewRegistry()

	// Test disabling the registry
	registry.SetEnabled(false)
	assert.False(t, registry.IsEnabled(), "Registry should be disabled after SetEnabled(false)")

	// Test enabling the registry
	registry.SetEnabled(true)
	assert.True(t, registry.IsEnabled(), "Registry should be enabled after SetEnabled(true)")

	// Test setting to the same value (enabled)
	registry.SetEnabled(true)
	assert.True(t, registry.IsEnabled(), "Registry should remain enabled after SetEnabled(true) when already enabled")

	// Test setting to the same value (disabled)
	registry.SetEnabled(false)
	assert.False(t, registry.IsEnabled(), "Registry should remain disabled after SetEnabled(false) when already disabled")
}

// mockRule implements the Rule interface for testing
type mockRule struct {
	BaseRule
	appliesToFunc func(*chart.Chart) (Detection, bool)
}

func (r *mockRule) AppliesTo(_ *chart.Chart) (Detection, bool) {
	// Don't pass the parameter since it's unused
	return r.appliesToFunc(nil)
}

func TestRegistry_ApplyRules(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		rulesApply     bool
		expectedChange bool
		expectError    bool
	}{
		{
			name:           "Registry enabled, rule applies",
			enabled:        true,
			rulesApply:     true,
			expectedChange: true,
			expectError:    false,
		},
		{
			name:           "Registry disabled, rule applies",
			enabled:        false,
			rulesApply:     true,
			expectedChange: false,
			expectError:    false,
		},
		{
			name:           "Registry enabled, rule doesn't apply",
			enabled:        true,
			rulesApply:     false,
			expectedChange: false,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test chart
			testChart := &chart.Chart{
				Metadata: &chart.Metadata{
					Name: "test-chart",
				},
			}

			// Create a registry
			registry := NewRegistry()

			// Set registry enabled state
			registry.SetEnabled(tt.enabled)

			// Clear default rules
			registry.rules = []Rule{}

			// Add a mock rule that applies or doesn't based on the test case
			mockRule := &mockRule{
				BaseRule: NewBaseRule("test-rule", "Test rule", []Parameter{
					{
						Path:  "test.parameter",
						Value: "test-value",
						Type:  TypeDeploymentCritical,
					},
				}, 100),
				appliesToFunc: func(_ *chart.Chart) (Detection, bool) {
					return Detection{
						Provider:   ProviderBitnami,
						Confidence: ConfidenceHigh,
					}, tt.rulesApply
				},
			}
			registry.AddRule(mockRule)

			// Create an override map
			overrideMap := make(map[string]interface{})

			// Apply rules
			changed, err := registry.ApplyRules(testChart, overrideMap)

			// Check results
			if tt.expectError {
				assert.Error(t, err, "Expected an error")
			} else {
				assert.NoError(t, err, "Did not expect an error")
			}

			assert.Equal(t, tt.expectedChange, changed, "Unexpected change result")

			// If rules were applied and registry is enabled, verify the parameter was added
			if tt.enabled && tt.rulesApply {
				test, ok := overrideMap["test"].(map[string]interface{})
				assert.True(t, ok, "test parameter should be a map")
				if ok {
					parameter, ok := test["parameter"].(string)
					assert.True(t, ok, "parameter should be a string")
					assert.Equal(t, "test-value", parameter, "parameter value should match")
				}
			} else {
				// Otherwise, verify the override map remains empty
				assert.Empty(t, overrideMap, "override map should be empty")
			}
		})
	}
}
