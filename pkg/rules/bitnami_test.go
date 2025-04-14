package rules

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
