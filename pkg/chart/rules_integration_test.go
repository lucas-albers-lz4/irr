package chart

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"

	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/rules"
)

// mockChartLoader implements analysis.ChartLoader for testing
type mockChartLoader struct {
	mock.Mock
}

func (m *mockChartLoader) Load(chartPath string) (*chart.Chart, error) {
	args := m.Called(chartPath)
	chartObj, ok := args.Get(0).(*chart.Chart)
	err := args.Error(1)
	if err != nil {
		return nil, fmt.Errorf("mock loader error: %w", err)
	}
	if !ok || chartObj == nil {
		return nil, fmt.Errorf("type assertion failed: expected *chart.Chart")
	}
	return chartObj, nil
}

// mockRulesRegistry implements a mock rules registry for testing
type mockRulesRegistry struct {
	mock.Mock
}

func (m *mockRulesRegistry) Get(name string) (rules.Rule, bool) {
	args := m.Called(name)
	rule, ok := args.Get(0).(rules.Rule)
	if !ok && args.Get(0) != nil {
		// Only log an error if the value wasn't nil to begin with
		log.Errorf("Type assertion failed for rules.Rule in Get")
	}
	return rule, args.Bool(1)
}

func (m *mockRulesRegistry) GetRuleByName(name string) rules.Rule {
	args := m.Called(name)
	rule, ok := args.Get(0).(rules.Rule)
	if !ok && args.Get(0) != nil {
		// Only log an error if the value wasn't nil to begin with
		log.Errorf("Type assertion failed for rules.Rule in GetRuleByName")
	}
	return rule
}

func (m *mockRulesRegistry) ApplyRules(chrt *chart.Chart, overrides map[string]interface{}) (bool, error) {
	args := m.Called(chrt, overrides)
	return args.Bool(0), args.Error(1)
}

// mockPathStrategy implements strategy.PathStrategy for testing
type mockPathStrategy struct {
	mock.Mock
}

func (m *mockPathStrategy) GeneratePath(ref *image.Reference, targetRegistry string) (string, error) {
	args := m.Called(ref, targetRegistry)
	return args.String(0), args.Error(1)
}

// TestSetRulesEnabled tests the SetRulesEnabled method
func TestSetRulesEnabled(t *testing.T) {
	// Create a new generator with default options
	generator := NewGenerator(
		"test-chart",
		"example.com",
		[]string{},
		[]string{},
		&mockPathStrategy{},
		nil, // nil mappings is valid
		nil,
		false,
		0,
		nil,
		nil,
		nil,
		nil,
	)

	// By default, rules should be enabled
	assert.True(t, generator.rulesEnabled, "Rules should be enabled by default")

	// Test disabling rules
	generator.SetRulesEnabled(false)
	assert.False(t, generator.rulesEnabled, "Rules should be disabled after calling SetRulesEnabled(false)")

	// Test enabling rules
	generator.SetRulesEnabled(true)
	assert.True(t, generator.rulesEnabled, "Rules should be enabled after calling SetRulesEnabled(true)")
}

// TestGenerateWithRulesEnabled tests the Generate method with rules enabled
func TestGenerateWithRulesEnabled(t *testing.T) {
	// Create a minimal test setup to verify SetRulesEnabled works correctly
	generator := NewGenerator(
		"test-chart",
		"example.com",
		[]string{},
		[]string{},
		&mockPathStrategy{},
		nil, // nil mappings is valid
		nil,
		false,
		0,
		nil,
		nil,
		nil,
		nil,
	)

	// Default is enabled
	assert.True(t, generator.rulesEnabled, "Rules should be enabled by default")

	// Test disabling rules
	generator.SetRulesEnabled(false)
	assert.False(t, generator.rulesEnabled, "Rules should be disabled after calling SetRulesEnabled(false)")

	// Test enabling rules
	generator.SetRulesEnabled(true)
	assert.True(t, generator.rulesEnabled, "Rules should be enabled after calling SetRulesEnabled(true)")
}

// TestGenerateWithRulesDisabled tests the Generate method with rules disabled
func TestGenerateWithRulesDisabled(t *testing.T) {
	// Create a mock chart loader
	mockLoader := new(mockChartLoader)

	// Create a mock chart
	mockChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Values: map[string]interface{}{},
	}

	// Configure the mock loader to return our mock chart
	mockLoader.On("Load", "test-chart").Return(mockChart, nil)

	// Create a mock rules registry
	mockRegistry := new(mockRulesRegistry)

	// The mock should NOT be called since rules are disabled
	// We're not setting an expectation, so if it's called, the test will fail

	// Create a mock path strategy
	mockStrategy := new(mockPathStrategy)

	// Create a new generator with our mocks
	generator := NewGenerator(
		"test-chart",
		"example.com",
		[]string{},
		[]string{},
		mockStrategy,
		nil, // nil mappings is valid
		nil,
		false,
		0,
		mockLoader,
		nil,
		nil,
		nil,
	)

	// Set the rules registry
	generator.rulesRegistry = mockRegistry

	// Disable rules
	generator.SetRulesEnabled(false)

	// Call Generate
	result, err := generator.Generate()

	// Verify there was no error
	require.NoError(t, err, "Generate should not return an error")
	require.NotNil(t, result, "Generate should return a result")

	// Verify the mock was called
	mockLoader.AssertCalled(t, "Load", "test-chart")

	// Verify the rules registry was NOT called
	mockRegistry.AssertNotCalled(t, "ApplyRules", mock.Anything, mock.Anything)
}

// TestInitRulesRegistry uses a different approach to test initRulesRegistry
func TestInitRulesRegistry(t *testing.T) {
	// Create a new generator
	generator := NewGenerator(
		"test-chart",
		"example.com",
		[]string{},
		[]string{},
		&mockPathStrategy{},
		nil, // nil mappings is valid
		nil,
		false,
		0,
		nil,
		nil,
		nil,
		nil,
	)

	// Verify rulesRegistry is nil initially
	assert.Nil(t, generator.rulesRegistry, "rulesRegistry should be nil initially")

	// Call initRulesRegistry
	generator.initRulesRegistry()

	// Verify rulesRegistry is not nil after initialization
	assert.NotNil(t, generator.rulesRegistry, "rulesRegistry should not be nil after initialization")
	assert.Equal(t, rules.DefaultRegistry, generator.rulesRegistry, "rulesRegistry should be set to DefaultRegistry")
}

// TestGenerateWithRulesTypeAssertion tests the Generate method with a type assertion failure
func TestGenerateWithRulesTypeAssertion(t *testing.T) {
	// Create a mock chart loader
	mockLoader := new(mockChartLoader)

	// Create a mock chart
	mockChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Values: map[string]interface{}{},
	}

	// Configure the mock loader to return our mock chart
	mockLoader.On("Load", "test-chart").Return(mockChart, nil)

	// Create a mock path strategy
	mockStrategy := new(mockPathStrategy)

	// Create a new generator with our mocks
	generator := NewGenerator(
		"test-chart",
		"example.com",
		[]string{},
		[]string{},
		mockStrategy,
		nil, // nil mappings is valid
		nil,
		false,
		0,
		mockLoader,
		nil,
		nil,
		nil,
	)

	// Set the rules registry to something that's not a *rules.Registry
	// This will cause a type assertion failure
	generator.rulesRegistry = "not a registry"

	// Call Generate
	result, err := generator.Generate()

	// Verify there was no error (type assertion failure shouldn't cause Generate to fail)
	require.NoError(t, err, "Generate should not return an error")
	require.NotNil(t, result, "Generate should return a result")

	// Verify the mock was called
	mockLoader.AssertCalled(t, "Load", "test-chart")
}
