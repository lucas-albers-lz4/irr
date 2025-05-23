package rules

import (
	"testing"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
)

func TestBaseRule_Methods(t *testing.T) {
	// Create test parameters
	params := []Parameter{
		{
			Path:        "global.security.allowInsecureImages",
			Value:       true,
			Type:        TypeDeploymentCritical,
			Description: "Allows insecure images",
		},
		{
			Path:        "test.parameter",
			Value:       "test-value",
			Type:        TypeTestValidationOnly,
			Description: "Test parameter",
		},
	}

	// Create a base rule
	baseRule := NewBaseRule(
		"test-rule",
		"A test rule for testing purposes",
		params,
		100,
	)

	// Test Name method
	assert.Equal(t, "test-rule", baseRule.Name(), "Name should match what was set")

	// Test Description method
	assert.Equal(t, "A test rule for testing purposes", baseRule.Description(), "Description should match what was set")

	// Test Parameters method
	assert.Equal(t, params, baseRule.Parameters(), "Parameters should match what was set")

	// Test Priority and GetPriority methods (they should return the same value)
	assert.Equal(t, 100, baseRule.Priority(), "Priority should match what was set")
	assert.Equal(t, 100, baseRule.GetPriority(), "GetPriority should match what was set")

	// Test AppliesTo method (base implementation always returns false)
	detection, applies := baseRule.AppliesTo(&chart.Chart{})
	assert.False(t, applies, "Base AppliesTo should always return false")
	assert.Equal(t, ProviderUnknown, detection.Provider, "Base detection should have unknown provider")
	assert.Equal(t, ConfidenceNone, detection.Confidence, "Base detection should have no confidence")

	// Test SetChart method (base implementation does nothing)
	// This is mostly for coverage as it's a no-op function
	baseRule.SetChart(&chart.Chart{})
}

func TestApplyRulesToMap(t *testing.T) {
	// Create test cases in the first part
	tests := createApplyRulesToMapTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runApplyRulesToMapTest(t, tt)
		})
	}
}

// Helper function to create test cases
func createApplyRulesToMapTestCases() []struct {
	name           string
	rules          []Rule
	chart          *chart.Chart
	initialMap     map[string]interface{}
	expectedMap    map[string]interface{}
	expectedChange bool
	expectError    bool
} {
	testCases := createBasicRuleTestCases()
	testCases = append(testCases, createAdvancedRuleTestCases()...)
	return testCases
}

// Basic test cases for ApplyRulesToMap
func createBasicRuleTestCases() []struct {
	name           string
	rules          []Rule
	chart          *chart.Chart
	initialMap     map[string]interface{}
	expectedMap    map[string]interface{}
	expectedChange bool
	expectError    bool
} {
	return []struct {
		name           string
		rules          []Rule
		chart          *chart.Chart
		initialMap     map[string]interface{}
		expectedMap    map[string]interface{}
		expectedChange bool
		expectError    bool
	}{
		{
			name:           "Empty rules",
			rules:          []Rule{},
			chart:          &chart.Chart{Metadata: &chart.Metadata{Name: "test-chart"}},
			initialMap:     map[string]interface{}{},
			expectedMap:    map[string]interface{}{},
			expectedChange: false,
			expectError:    false,
		},
		{
			name:           "Nil chart",
			rules:          []Rule{&mockTestRule{}},
			chart:          nil,
			initialMap:     map[string]interface{}{},
			expectedMap:    map[string]interface{}{},
			expectedChange: false,
			expectError:    false,
		},
		{
			name: "Rule applies, Type 1 parameter",
			rules: []Rule{&mockTestRule{
				baseRule: NewBaseRule("test-rule", "Test rule", []Parameter{
					{
						Path:  "global.security.allowInsecureImages",
						Value: true,
						Type:  TypeDeploymentCritical,
					},
				}, 100),
				applies: true,
			}},
			chart:      &chart.Chart{Metadata: &chart.Metadata{Name: "test-chart"}},
			initialMap: map[string]interface{}{},
			expectedMap: map[string]interface{}{
				"global": map[string]interface{}{
					"security": map[string]interface{}{
						"allowInsecureImages": true,
					},
				},
			},
			expectedChange: true,
			expectError:    false,
		},
		{
			name: "Rule applies, Type 2 parameter (validation only)",
			rules: []Rule{&mockTestRule{
				baseRule: NewBaseRule("test-rule", "Test rule", []Parameter{
					{
						Path:  "kubeVersion",
						Value: "1.22.0",
						Type:  TypeTestValidationOnly,
					},
				}, 100),
				applies: true,
			}},
			chart:          &chart.Chart{Metadata: &chart.Metadata{Name: "test-chart"}},
			initialMap:     map[string]interface{}{},
			expectedMap:    map[string]interface{}{}, // Type 2 parameters should not be added
			expectedChange: false,
			expectError:    false,
		},
		{
			name: "Rule applies, mixed parameter types",
			rules: []Rule{&mockTestRule{
				baseRule: NewBaseRule("test-rule", "Test rule", []Parameter{
					{
						Path:  "global.security.allowInsecureImages",
						Value: true,
						Type:  TypeDeploymentCritical,
					},
					{
						Path:  "kubeVersion",
						Value: "1.22.0",
						Type:  TypeTestValidationOnly,
					},
				}, 100),
				applies: true,
			}},
			chart:      &chart.Chart{Metadata: &chart.Metadata{Name: "test-chart"}},
			initialMap: map[string]interface{}{},
			expectedMap: map[string]interface{}{ // Only Type 1 parameters should be added
				"global": map[string]interface{}{
					"security": map[string]interface{}{
						"allowInsecureImages": true,
					},
				},
			},
			expectedChange: true,
			expectError:    false,
		},
	}
}

// Advanced test cases for ApplyRulesToMap
func createAdvancedRuleTestCases() []struct {
	name           string
	rules          []Rule
	chart          *chart.Chart
	initialMap     map[string]interface{}
	expectedMap    map[string]interface{}
	expectedChange bool
	expectError    bool
} {
	return []struct {
		name           string
		rules          []Rule
		chart          *chart.Chart
		initialMap     map[string]interface{}
		expectedMap    map[string]interface{}
		expectedChange bool
		expectError    bool
	}{
		{
			name: "Rule doesn't apply",
			rules: []Rule{&mockTestRule{
				baseRule: NewBaseRule("test-rule", "Test rule", []Parameter{
					{
						Path:  "global.security.allowInsecureImages",
						Value: true,
						Type:  TypeDeploymentCritical,
					},
				}, 100),
				applies: false,
			}},
			chart:          &chart.Chart{Metadata: &chart.Metadata{Name: "test-chart"}},
			initialMap:     map[string]interface{}{},
			expectedMap:    map[string]interface{}{},
			expectedChange: false,
			expectError:    false,
		},
		{
			name: "Multiple rules, all apply",
			rules: []Rule{
				&mockTestRule{
					baseRule: NewBaseRule("rule1", "Rule 1", []Parameter{
						{
							Path:  "param1",
							Value: "value1",
							Type:  TypeDeploymentCritical,
						},
					}, 100),
					applies: true,
				},
				&mockTestRule{
					baseRule: NewBaseRule("rule2", "Rule 2", []Parameter{
						{
							Path:  "param2",
							Value: "value2",
							Type:  TypeDeploymentCritical,
						},
					}, 90),
					applies: true,
				},
			},
			chart:      &chart.Chart{Metadata: &chart.Metadata{Name: "test-chart"}},
			initialMap: map[string]interface{}{},
			expectedMap: map[string]interface{}{
				"param1": "value1",
				"param2": "value2",
			},
			expectedChange: true,
			expectError:    false,
		},
		{
			name: "Rule with path collision (same value)",
			rules: []Rule{
				&mockTestRule{
					baseRule: NewBaseRule("rule1", "Rule 1", []Parameter{
						{
							Path:  "param.nested",
							Value: "same-value",
							Type:  TypeDeploymentCritical,
						},
					}, 100),
					applies: true,
				},
				&mockTestRule{
					baseRule: NewBaseRule("rule2", "Rule 2", []Parameter{
						{
							Path:  "param.nested",
							Value: "same-value",
							Type:  TypeDeploymentCritical,
						},
					}, 90),
					applies: true,
				},
			},
			chart:      &chart.Chart{Metadata: &chart.Metadata{Name: "test-chart"}},
			initialMap: map[string]interface{}{},
			expectedMap: map[string]interface{}{
				"param": map[string]interface{}{
					"nested": "same-value",
				},
			},
			expectedChange: true,
			expectError:    false,
		},
		{
			name: "Existing values in map",
			rules: []Rule{&mockTestRule{
				baseRule: NewBaseRule("test-rule", "Test rule", []Parameter{
					{
						Path:  "global.security.allowInsecureImages",
						Value: true,
						Type:  TypeDeploymentCritical,
					},
				}, 100),
				applies: true,
			}},
			chart: &chart.Chart{Metadata: &chart.Metadata{Name: "test-chart"}},
			initialMap: map[string]interface{}{
				"global": map[string]interface{}{
					"someOtherValue": "keep-me",
				},
			},
			expectedMap: map[string]interface{}{
				"global": map[string]interface{}{
					"someOtherValue": "keep-me",
					"security": map[string]interface{}{
						"allowInsecureImages": true,
					},
				},
			},
			expectedChange: true,
			expectError:    false,
		},
	}
}

// Helper function to run a test case
func runApplyRulesToMapTest(t *testing.T, tt struct {
	name           string
	rules          []Rule
	chart          *chart.Chart
	initialMap     map[string]interface{}
	expectedMap    map[string]interface{}
	expectedChange bool
	expectError    bool
}) {
	// Create a copy of the initial map
	testMap := make(map[string]interface{})
	for k, v := range tt.initialMap {
		testMap[k] = v
	}

	// Apply rules
	changed, err := ApplyRulesToMap(tt.rules, tt.chart, testMap)

	// Check results
	if tt.expectError {
		assert.Error(t, err, "Expected an error")
	} else {
		assert.NoError(t, err, "Did not expect an error")
	}

	assert.Equal(t, tt.expectedChange, changed, "Unexpected change result")
	assert.Equal(t, tt.expectedMap, testMap, "Map does not match expected output")
}

// mockTestRule implements the Rule interface for testing
type mockTestRule struct {
	baseRule BaseRule
	applies  bool
}

func (r *mockTestRule) Name() string {
	return r.baseRule.Name()
}

func (r *mockTestRule) Description() string {
	return r.baseRule.Description()
}

func (r *mockTestRule) Parameters() []Parameter {
	return r.baseRule.Parameters()
}

func (r *mockTestRule) Priority() int {
	return r.baseRule.Priority()
}

func (r *mockTestRule) AppliesTo(_ *chart.Chart) (Detection, bool) {
	return Detection{
		Provider:   ProviderBitnami,
		Confidence: ConfidenceHigh,
	}, r.applies
}

func TestApplyRulesToMap_LogOutput(t *testing.T) {
	// Define a test rule with parameters
	rule := &mockTestRule{
		baseRule: BaseRule{
			name:        "test-log-rule",
			description: "Rule for testing logging",
			priority:    10,
			parameters: []Parameter{
				{Path: "global.security.allowInsecureImages", Value: true},
			},
		},
		applies: true, // Ensure this rule applies
	}

	// Create a dummy chart
	testChart := &chart.Chart{
		Metadata: &chart.Metadata{Name: "test-chart"},
	}

	// Create an override map
	overrideMap := make(map[string]interface{})

	// Set log level to debug (use slog level)
	originalLevel := log.CurrentLevel()
	log.SetLevel(log.LevelDebug)
	defer log.SetLevel(originalLevel)

	// --- Test with Rules Enabled ---
	_, enabledLogs, err := testutil.CaptureJSONLogs(log.LevelDebug, func() {
		// Apply the rule and check error
		_, applyErr := ApplyRulesToMap([]Rule{rule}, testChart, overrideMap)
		assert.NoError(t, applyErr, "ApplyRulesToMap should not produce an error")
	})
	require.NoError(t, err, "Log capture should not produce an error")

	// Check log output contains expected information using JSON helpers

	// Assertion 0: Check for initial rule checking message
	testutil.AssertLogContainsJSON(t, enabledLogs, map[string]interface{}{
		"level":      "DEBUG",
		"msg":        "Checking rules for chart",
		"rule_count": 1.0, // JSON unmarshals numbers as float64
		"chart_name": "test-chart",
	})

	// Assertion 1: Check for the rule application message
	testutil.AssertLogContainsJSON(t, enabledLogs, map[string]interface{}{
		"level":      "DEBUG",
		"msg":        "Rule applies to chart",
		"rule_name":  "test-log-rule",
		"chart_name": "test-chart",
		"confidence": 3.0, // JSON unmarshals numbers as float64
	})

	// Assertion 2: Check for the parameter check message
	testutil.AssertLogContainsJSON(t, enabledLogs, map[string]interface{}{
		"level":      "DEBUG",
		"msg":        "Checking parameter",
		"chart_name": "test-chart",
		"rule_name":  "test-log-rule",
		"param_path": "global.security.allowInsecureImages",
		"param_type": 0.0, // Type 0 (Informational), JSON unmarshals as float64
	})

	// --- Test with Rules Disabled ---
	_, disabledLogs, err := testutil.CaptureJSONLogs(log.LevelDebug, func() {
		// Create registry with rules disabled
		registry := NewRegistry()
		registry.SetEnabled(false)

		// Apply rules through registry and check error
		_, applyErr := registry.ApplyRules(testChart, overrideMap)
		assert.NoError(t, applyErr, "registry.ApplyRules should not produce an error")
	})
	require.NoError(t, err, "Log capture should not produce an error")

	// Check log output contains message about rules being disabled
	testutil.AssertLogContainsJSON(t, disabledLogs, map[string]interface{}{
		"level": "DEBUG",
		"msg":   "Rules system is disabled, skipping rule application", // Match actual log msg
	})
}

/*
// Removed TestSetChart as the SetChart method is defined on the Rule interface
// but not implemented by BaseRule or the mockTestRule.
// Coverage report likely indicates it's not implemented/used by any concrete type.
func TestSetChart(t *testing.T) {
	assertions := assert.New(t)
	// Use a mock rule that embeds BaseRule
	mockRule := &mockTestRule{
		baseRule: NewBaseRule("test-rule", "test description", nil, 0),
		applies:  true, // or false, doesn't matter for SetChart
	}
	chartMeta := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "my-chart",
			Version: "1.0.0",
		},
	}

	mockRule.SetChart(chartMeta)

	// Assertions remain the same, ensuring the call doesn't panic
	// and the rule object is still valid.
	assertions.NotNil(mockRule, "Rule should not be nil after setting chart")
	// If mockTestRule or BaseRule had internal state affected by SetChart,
	// we would add assertions here.
}
*/
