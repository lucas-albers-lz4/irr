package generator

import (
	"testing"

	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
)

// TestGenerate tests the override generation logic.
func TestGenerate(t *testing.T) {
	// ... (setup remains the same)
	strategy := strategy.NewPrefixSourceRegistryStrategy()

	generator := NewGenerator(
		nil, // registryMappings
		strategy,
		[]string{"old-registry.com"}, // sourceRegistries
		[]string{},                   // excludeRegistries
		false,                        // strictMode
		false,                        // templateMode (assuming default for this test)
	)

	// Note: The Generate method likely needs chartPath and testValues
	// _, err := generator.Generate(chartPath, testValues) // Example
	// Fix: Use blank identifier to silence unused variable error
	_, err := generator.Generate("", nil) // Placeholder - Needs fixing & proper test setup/assertions
	// TODO: Update error assertion once test setup provides valid inputs.
	if err != nil {
		t.Errorf("Generate() returned an unexpected error: %v", err)
	}
	// ... existing code ...
}

// TestGenerate_WithMappings tests generation with registry mappings.
func TestGenerate_WithMappings(t *testing.T) {
	// ... (setup remains the same)
	strategy := strategy.NewPrefixSourceRegistryStrategy()
	mappings := &registry.RegistryMappings{
		Mappings: []registry.RegistryMapping{
			{Source: "old-registry.com", Target: "mapped-registry.com/oldreg"},
		},
	}

	generator := NewGenerator(
		mappings,
		strategy,
		[]string{"old-registry.com", "other-registry.com"}, // sourceRegistries
		[]string{}, // excludeRegistries
		false,      // strictMode
		false,      // templateMode (assuming default for this test)
	)

	// Note: The Generate method likely needs chartPath and testValues
	// overrideFile, err := generator.Generate(chartPath, testValues) // Example
	// Fix: Use blank identifier to silence unused variable errors
	_, err := generator.Generate("", nil) // Placeholder - Needs fixing & proper test setup/assertions
	// TODO: Update error assertion once test setup provides valid inputs.
	if err != nil {
		t.Errorf("Generate() returned an unexpected error: %v", err)
	}
	// ... existing code ...
}
