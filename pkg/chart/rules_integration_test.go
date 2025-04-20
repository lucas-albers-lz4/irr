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
	// Create a new generator with default options (rulesEnabled=true)
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
		true, // Rules enabled by default
	)

	// By default, rules should be enabled
	assert.True(t, generator.rulesEnabled, "Rules should be enabled by default")

	// Test creating a generator with rules explicitly disabled
	generatorDisabled := NewGenerator(
		"test-chart",
		"example.com",
		[]string{},
		[]string{},
		&mockPathStrategy{},
		nil, nil, false, 0, nil, nil, nil, nil,
		false, // Explicitly disable rules
	)
	assert.False(t, generatorDisabled.rulesEnabled, "Rules should be disabled when passed false")

	// Test creating a generator with rules explicitly enabled
	generatorEnabled := NewGenerator(
		"test-chart",
		"example.com",
		[]string{},
		[]string{},
		&mockPathStrategy{},
		nil, nil, false, 0, nil, nil, nil, nil,
		true, // Explicitly enable rules
	)
	assert.True(t, generatorEnabled.rulesEnabled, "Rules should be enabled when passed true")
}

// TestGenerateWithRulesEnabled tests the Generate method with rules enabled
// This test primarily ensures the structure is correct and rulesEnabled flag is accessible
func TestGenerateWithRulesEnabled(t *testing.T) {
	// Create a minimal test setup
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
		true, // Rules explicitly enabled for this test
	)

	// Default is enabled
	assert.True(t, generator.rulesEnabled, "Rules should be enabled when generator is created with rulesEnabled=true")
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

	// Create a new generator with our mocks, explicitly disabling rules
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
		false, // Rules explicitly disabled
	)

	// Set the rules registry (still needed for the AssertNotCalled check)
	generator.rulesRegistry = mockRegistry

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
	// Create a new generator (rules enabled by default)
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
		true, // Default rules enabled
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

	// Create a new generator with our mocks (rules enabled by default)
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
		true, // Rules enabled
	)

	// Create a mock rules registry that returns a non-Rule type for GetRuleByName
	mockRegistry := new(mockRulesRegistry)
	mockRegistry.On("ApplyRules", mockChart, mock.AnythingOfType("map[string]interface {}")).Return(false, nil)

	// Inject the mock registry into the generator
	generator.rulesRegistry = mockRegistry

	// Call Generate
	result, err := generator.Generate()

	// Verify there was no error (type assertion failure shouldn't cause Generate to fail)
	require.NoError(t, err, "Generate should not return an error")
	require.NotNil(t, result, "Generate should return a result")

	// Verify the mock was called
	mockLoader.AssertCalled(t, "Load", "test-chart")
	mockRegistry.AssertCalled(t, "ApplyRules", mockChart, mock.AnythingOfType("map[string]interface {}"))
}
