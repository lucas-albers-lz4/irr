package rules

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/chart"
	// Add other necessary imports here
)

// TestNewBitnamiSecurityBypassRule tests the constructor for BitnamiSecurityBypassRule.
func TestNewBitnamiSecurityBypassRule(t *testing.T) {
	rule := NewBitnamiSecurityBypassRule()

	assert.NotNil(t, rule, "Rule should not be nil")
	assert.Equal(t, "bitnami-security-bypass", rule.Name(), "Rule name mismatch")
	assert.Contains(t, rule.Description(), "Bitnami charts", "Rule description mismatch")
	assert.Equal(t, BitnamiSecurityBypassPriority, rule.Priority(), "Rule priority mismatch")

	params := rule.Parameters()
	assert.Len(t, params, 1, "Expected exactly one parameter")
	if len(params) == 1 {
		assert.Equal(t, "global.security.allowInsecureImages", params[0].Path, "Parameter path mismatch")
		assert.Equal(t, true, params[0].Value, "Parameter value mismatch")
		assert.Equal(t, TypeDeploymentCritical, params[0].Type, "Parameter type mismatch")
	}
}

// TestAppliesTo tests the AppliesTo method of the BitnamiSecurityBypassRule.
func TestAppliesTo(t *testing.T) {
	// Create a test chart that should match Bitnami detection (medium/high confidence)
	bitnamiChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name: "test-bitnami-chart",
			Home: "https://bitnami.com/charts", // Indicator 1
			Maintainers: []*chart.Maintainer{ // Indicator 2
				{
					Name: "Bitnami Team",
				},
			},
		},
	}

	// Create a test chart that has low confidence Bitnami detection
	lowConfidenceBitnamiChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name: "low-confidence-bitnami",
			Home: "https://bitnami.com/charts", // Only one indicator
		},
	}

	// Create a test chart that should NOT match Bitnami detection
	nonBitnamiChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name: "test-standard-chart",
			Home: "https://example.com/charts",
		},
	}

	// Create the rule to test
	rule := NewBitnamiSecurityBypassRule()

	// Test the high confidence Bitnami chart
	detectionHigh, appliesHigh := rule.AppliesTo(bitnamiChart)
	assert.True(t, appliesHigh, "Rule should apply to high confidence Bitnami chart")
	assert.Equal(t, ProviderBitnami, detectionHigh.Provider)
	assert.GreaterOrEqual(t, detectionHigh.Confidence, ConfidenceMedium, "Confidence should be Medium or High")

	// Test the low confidence Bitnami chart (should NOT apply based on rule logic)
	_, appliesLow := rule.AppliesTo(lowConfidenceBitnamiChart)
	assert.False(t, appliesLow, "Rule should NOT apply to low confidence Bitnami chart (only one indicator)")

	// Test the non-Bitnami chart
	detectionNon, appliesNon := rule.AppliesTo(nonBitnamiChart)
	assert.False(t, appliesNon, "Rule should not apply to non-Bitnami chart")
	assert.Equal(t, ConfidenceNone, detectionNon.Confidence)
}

func TestBitnamiFallbackHandler_ShouldRetryWithSecurityBypass(t *testing.T) {
	// Test the handler for Bitnami security error detection
	// See https://github.com/bitnami/charts/issues/30850 for details on the security bypass mechanism
	handler := NewBitnamiFallbackHandler()

	// Test cases
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name: "Bitnami Security Error",
			err: errors.New(
				"exit code 16: original containers have been substituted for unrecognized ones. " +
					"if you are sure you want to proceed with non-standard containers, you can skip container image verification by " +
					"setting the global parameter 'global.security.allowInsecureImages' to true"),
			expected: true,
		},
		{
			name: "Missing Exit Code 16",
			err: errors.New(
				"original containers have been substituted for unrecognized ones. " +
					"if you are sure you want to proceed with non-standard containers, you can skip container image verification by " +
					"setting the global parameter 'global.security.allowInsecureImages' to true"),
			expected: false,
		},
		{
			name:     "Missing Error Message About Containers",
			err:      errors.New("exit code 16: some other Bitnami error that doesn't mention containers"),
			expected: false,
		},
		{
			name:     "General Helm Error",
			err:      errors.New("helm template rendering failed: template parsing error"),
			expected: false,
		},
		{
			name:     "Nil Error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.ShouldRetryWithSecurityBypass(tt.err)
			assert.Equal(t, tt.expected, result, "ShouldRetryWithSecurityBypass() returned unexpected result")
		})
	}
}

func TestBitnamiFallbackHandler_ApplySecurityBypass(t *testing.T) {
	handler := NewBitnamiFallbackHandler()

	// Test with empty overrides
	emptyOverrides := make(map[string]interface{})
	err := handler.ApplySecurityBypass(emptyOverrides)
	assert.NoError(t, err)

	// Verify the value was set correctly
	globalMap, ok := emptyOverrides["global"].(map[string]interface{})
	assert.True(t, ok, "global should be a map")
	securityMap, ok := globalMap["security"].(map[string]interface{})
	assert.True(t, ok, "security should be a map")
	allowInsecure, ok := securityMap["allowInsecureImages"].(bool)
	assert.True(t, ok, "allowInsecureImages should be a bool")
	assert.True(t, allowInsecure, "allowInsecureImages should be true")

	// Test with existing data
	existingOverrides := map[string]interface{}{
		"global": map[string]interface{}{
			"security": map[string]interface{}{
				"someOtherSetting": "value",
			},
		},
	}

	err = handler.ApplySecurityBypass(existingOverrides)
	assert.NoError(t, err)

	// Verify the value was added without disturbing existing values
	globalMap, ok = existingOverrides["global"].(map[string]interface{})
	assert.True(t, ok, "global should be a map")
	securityMap, ok = globalMap["security"].(map[string]interface{})
	assert.True(t, ok, "security should be a map")
	allowInsecure, ok = securityMap["allowInsecureImages"].(bool)
	assert.True(t, ok, "allowInsecureImages should be a bool")
	assert.True(t, allowInsecure, "allowInsecureImages should be true")

	someOtherSetting, ok := securityMap["someOtherSetting"].(string)
	assert.True(t, ok, "someOtherSetting should exist and be a string")
	assert.Equal(t, "value", someOtherSetting, "existing values should be preserved")
}
