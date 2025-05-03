package chart_test // Use _test package to avoid import cycles if needed
/*
import (
	"errors"
	"testing"

	helmchart "helm.sh/helm/v3/pkg/chart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/chart"
	"github.com/lucas-albers-lz4/irr/pkg/rules"
	"github.com/lucas-albers-lz4/irr/pkg/testutil"
)

// MockRulesRegistry for testing rules interaction
type MockRulesRegistry struct {
	mock.Mock
}

// ApplyRules mocks the ApplyRules method
func (m *MockRulesRegistry) ApplyRules(chart *helmchart.Chart, overrides map[string]interface{}) (bool, error) {
	args := m.Called(chart, overrides)
	return args.Bool(0), args.Error(1)
}

// SetRulesRegistry allows injecting the mock registry into the generator.
// This assumes Generator has an exported or unexported field for the registry.
// If the field is unexported, this helper might need to be in the chart package itself.
func (g *chart.Generator) SetRulesRegistry(registry rules.RegistryInterface) {
	// Use reflection or an exported setter if field is unexported
	// For simplicity, assuming an exported field `RulesRegistry` or similar access method
	// This needs to be adapted based on actual Generator structure
	// Example using an assumed exported field:
	// g.RulesRegistry = registry
	// If using an unexported field, reflection or placing this helper in `chart` package is needed.
	// Example (if in chart package):
	// g.rulesRegistry = registry
}

// TestInitRulesRegistry verifies that the rules registry is initialized when rules are enabled.
func TestInitRulesRegistry(t *testing.T) {
	// Create a generator with rules enabled but no registry initially provided
	generator := chart.NewGenerator(
		"test-path", "test-target", []string{}, []string{},
		&chart.MockPathStrategy{}, // Use mock strategy
		nil, false, 100,
		&chart.MockChartLoader{}, // Use mock loader
		true, // rulesEnabled = true
	)

	// Assert that rulesRegistry is initially nil or some default uninitialized state
	// This depends on how NewGenerator initializes it. Assuming it starts as nil.
	// assert.Nil(t, generator.rulesRegistry, "rulesRegistry should be nil initially")

	// Call a method that triggers initialization (e.g., applyRulesIfNeeded or a dedicated init method)
	// We need to simulate the scenario where rules are applied.
	// For this test, let's assume there's an internal init or call applyRulesIfNeeded indirectly.

	// Option 1: If there's an explicit init (preferred)
	// generator.initRulesRegistry() // Assuming this exists

	// Option 2: Simulate calling Generate which should trigger initialization if needed
	mockChart := testutil.CreateMockChart("init-test", "1.0", nil)
	mockAnalysis := analysis.NewChartAnalysis() // Empty analysis
	_, err := generator.Generate(mockChart, mockAnalysis)
	// We don't care about the result of Generate here, just that it might trigger init.
	// Handle potential errors from Generate if they are relevant to init failure.
	if err != nil && err.Error() != "cannot generate overrides without analysis results (analysisResult is nil)" {
		// Ignore the specific nil analysis error if it happens, focus on rules init
		// If other errors occur, fail the test as they might indicate issues
		// require.NoError(t, err, "Generate call failed unexpectedly during init test")
	}

	// Assert that rulesRegistry is now non-nil and potentially of the default type
	assert.NotNil(t, generator.GetRulesRegistry(), "rulesRegistry should be initialized after potential use")

	// Optional: Check if it's the default registry type if applicable
	// _, ok := generator.rulesRegistry.(*rules.Registry) // Assuming default is *rules.Registry
	// assert.True(t, ok, "rulesRegistry should be of the default type")
}

// GetRulesRegistry is a hypothetical helper method on Generator to access the internal rulesRegistry.
// Replace this with the actual way to access the registry in your Generator struct.
func (g *chart.Generator) GetRulesRegistry() rules.RegistryInterface {
	// Example assuming unexported field `rulesRegistry`
	// This requires reflection or being in the same package.
	// For demonstration, assume direct access or a proper getter exists.
	// return g.rulesRegistry // Replace with actual access logic
	return nil // Placeholder
}

// TestGeneratorRulesEnabledFlag checks if the rulesEnabled flag is correctly set.
func TestGeneratorRulesEnabledFlag(t *testing.T) {
	// Test case 1: Rules explicitly disabled
	genDisabled := chart.NewGenerator(
		"test", "test", []string{}, []string{}, nil, nil, false, 0, nil, false,
	)
	assert.False(t, genDisabled.IsRulesEnabled(), "Rules should be disabled when generator is created with rulesEnabled=false")

	// Test case 2: Rules explicitly enabled
	genEnabled := chart.NewGenerator(
		"test", "test", []string{}, []string{}, nil, nil, false, 0, nil, true,
	)
	assert.True(t, genEnabled.IsRulesEnabled(), "Rules should be enabled when generator is created with rulesEnabled=true")
}

// IsRulesEnabled is a hypothetical helper method on Generator.
// Replace with the actual way to check if rules are enabled.
func (g *chart.Generator) IsRulesEnabled() bool {
	// Example assuming unexported field `rulesEnabled`
	// return g.rulesEnabled // Replace with actual access logic
	return false // Placeholder
}


// TestGenerateWithRulesDisabled verifies ApplyRules is NOT called when disabled
func TestGenerateWithRulesDisabled(t *testing.T) {
	// Setup: Similar to above, but RulesEnabled: false
	mockChart := testutil.CreateMockChart("test-chart", "1.0.0", map[string]interface{}{
		"image": "docker.io/library/nginx:latest", // Add value for analysis
	})
	// mockLoader := &chart.MockChartLoader{ Chart: mockChart } // Loader not used by Generate
	mockStrategy := &chart.MockPathStrategy{}
	mockRules := &MockRulesRegistry{}
	generator := chart.NewGenerator(
		"test-chart",
		"target.com",
		[]string{"docker.io"},
		[]string{},
		mockStrategy,
		nil, // No mappings
		false, // Strict mode
		100, // Threshold
		nil,   // Pass nil for loader as Generate doesn't use it directly
		false, // RulesEnabled = false
	)
	generator.SetRulesRegistry(mockRules) // Inject mock

	// Provide the analysisResult matching the mock chart
	chartAnalysis := &analysis.ChartAnalysis{
		ImagePatterns: []analysis.ImagePattern{ // Include pattern even if rules disabled
			{Path: "image", Type: analysis.PatternTypeString, Value: "docker.io/library/nginx:latest"},
		},
	}

	// Execute generation
	_, err := generator.Generate(mockChart, chartAnalysis) // Pass analysisResult
	require.NoError(t, err)

	// Assert: ApplyRules should NOT have been called
	mockRules.AssertNotCalled(t, "ApplyRules", mock.Anything, mock.Anything)
}

// TestGenerateWithRulesTypeAssertion tests rule application with specific types
func TestGenerateWithRulesTypeAssertion(t *testing.T) {
	// Setup: Rules enabled, mock chart, provide analysis result
	mockChart := testutil.CreateMockChart("test-chart", "1.0.0", map[string]interface{}{
		"image": "docker.io/library/nginx:latest",
	})
	// mockLoader := &chart.MockChartLoader{Chart: mockChart} // Loader not used by Generate
	mockStrategy := &chart.MockPathStrategy{}
	mockRules := &MockRulesRegistry{}

	// Configure mock rules to expect specific types
	mockRules.On("ApplyRules",
		mock.MatchedBy(func(c *helmchart.Chart) bool { return c.Name() == "test-chart" }), // Match chart
		mock.AnythingOfType("map[string]interface {}"),                                  // Match overrides map
	).Return(true, nil) // Simulate modification

	generator := chart.NewGenerator(
		"test-chart", "example.com", []string{"docker.io"}, []string{}, mockStrategy,
		nil, false, 100, nil, // Pass nil for loader
		true, // RulesEnabled = true
	)
	generator.SetRulesRegistry(mockRules)

	// Provide the analysisResult matching the mock chart
	chartAnalysis := &analysis.ChartAnalysis{
		ImagePatterns: []analysis.ImagePattern{
			{Path: "image", Type: analysis.PatternTypeString, Value: "docker.io/library/nginx:latest"},
		},
	}

	// Execute generation
	_, err := generator.Generate(mockChart, chartAnalysis) // Pass analysisResult
	require.NoError(t, err)

	// Assert ApplyRules was called with expected argument types
	mockRules.AssertExpectations(t)
}


// TestInvalidTemplateHandling verifies error propagation when helm template fails
func TestInvalidTemplateHandling(t *testing.T) {
	// Setup mock validate function to return an error
	originalValidateFunc := chart.SetValidateHelmTemplateInternalFunc(func(chartPath string, overrides []byte) error {
		return errors.New("simulated template error")
	})
	defer chart.SetValidateHelmTemplateInternalFunc(originalValidateFunc) // Restore original

	// Create minimal generator and analysis result
	gen := chart.NewGenerator("test-chart", "target", []string{}, []string{}, nil, nil, false, 0, nil, false)
	mockChart := testutil.CreateMockChart("test-chart", "1.0", nil)
	analysisResult := analysis.NewChartAnalysis() // Empty analysis is fine

	// Generate (should succeed)
	overrideResult, err := gen.Generate(mockChart, analysisResult)
	require.NoError(t, err)
	require.NotNil(t, overrideResult)

	// Convert overrides to YAML
	overrideBytes, err := chart.OverridesToYAML(overrideResult.Values)
	require.NoError(t, err)

	// Call ValidateHelmTemplate - THIS should fail
	err = chart.ValidateHelmTemplate("test-chart", overrideBytes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated template error")
}

// Helper to set the internal validation function for testing
// Needs to be in the chart_test package to access the internal variable
func SetValidateHelmTemplateInternalFunc(f func(string, []byte) error) func(string, []byte) error {
	original := chart.GetValidateHelmTemplateInternalFunc() // Assumes an exported getter
	chart.SetValidateHelmTemplateInternalFunc(f)             // Assumes an exported setter
	return original
}

// Mock implementations for interfaces used by Generator
// (Assuming these might be needed if not already defined)

// MockChartLoader can be defined if not already available
// type MockChartLoader struct { mock.Mock } ...

// MockPathStrategy can be defined if not already available
// type MockPathStrategy struct { mock.Mock } ...
*/
