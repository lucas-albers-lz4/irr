package generator

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	"github.com/stretchr/testify/require"
)

// TestGenerate tests the override generation logic.
func TestGenerate(t *testing.T) {
	testValues := map[string]interface{}{
		"image": "old-registry.com/my-app:v1",
	}
	strategy := strategy.NewPrefixSourceRegistryStrategy()
	// Provide minimal non-nil mappings struct
	mappings := &registry.Mappings{}

	generator := NewGenerator(
		mappings,
		strategy,
		[]string{"old-registry.com"}, // sourceRegistries
		[]string{},                   // excludeRegistries
		false,                        // strictMode
		false,                        // templateMode
	)

	overrideFile, err := generator.Generate("test-chart", testValues)
	require.NoError(t, err, "Generate() should not return an error")

	expectedOverrides := map[string]interface{}{
		"image": map[string]interface{}{
			"registry":   "", // No mapping, so target registry is empty
			"repository": "old-registrycom/my-app",
			"tag":        "v1",
		},
	}

	// Marshal both maps to JSON
	actualJSON, errActual := json.Marshal(overrideFile)
	expectedJSON, errExpected := json.Marshal(expectedOverrides)

	if errActual != nil || errExpected != nil {
		t.Fatalf("Failed to marshal maps to JSON: ActualErr=%v, ExpectedErr=%v", errActual, errExpected)
	}

	// Unmarshal back into generic maps for comparison
	var actualMap, expectedMap map[string]interface{}
	errUnmarshalActual := json.Unmarshal(actualJSON, &actualMap)
	errUnmarshalExpected := json.Unmarshal(expectedJSON, &expectedMap)

	if errUnmarshalActual != nil || errUnmarshalExpected != nil {
		t.Fatalf("Failed to unmarshal JSON back to maps: ActualErr=%v, ExpectedErr=%v", errUnmarshalActual, errUnmarshalExpected)
	}

	// Compare the unmarshaled maps using cmp
	if !cmp.Equal(actualMap, expectedMap) {
		diff := cmp.Diff(expectedMap, actualMap)
		// Use the original JSON strings and the diff in the error message
		t.Errorf("Generate() override map mismatch (-want +got):\n%s\nActual JSON: %s\nExpected JSON: %s", diff, string(actualJSON), string(expectedJSON))
	}
}

// TestGenerate_WithMappings tests generation with registry mappings.
func TestGenerate_WithMappings(t *testing.T) {
	testValues := map[string]interface{}{
		"image": "old-registry.com/app1:v1", // Use key 'image'
		"nested": map[string]interface{}{
			"image": "other-registry.com/app2:v2", // Use key 'nested.image'
		},
		"unrelated": map[string]interface{}{
			"config": "excluded.com/app3:v3", // Should be ignored (not an image path)
		},
	}
	strategy := strategy.NewPrefixSourceRegistryStrategy()
	mappings := &registry.Mappings{
		Mappings: []registry.Mapping{
			{Source: "old-registry.com", Target: "mapped-registry.com/oldreg"},
			// No mapping for other-registry.com
		},
	}

	generator := NewGenerator(
		mappings,
		strategy,
		[]string{"old-registry.com", "other-registry.com"}, // sourceRegistries
		[]string{}, // excludeRegistries
		false,      // strictMode
		false,      // templateMode
	)

	overrideFile, err := generator.Generate("test-chart-mapped", testValues)
	require.NoError(t, err, "Generate() should not return an error")

	expectedOverrides := map[string]interface{}{
		"image": map[string]interface{}{
			"registry":   "mapped-registry.com/oldreg", // Mapped target registry
			"repository": "old-registrycom/app1",
			"tag":        "v1",
		},
		"nested": map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "", // No mapping for other-registry.com
				"repository": "other-registrycom/app2",
				"tag":        "v2",
			},
		},
		// 'unrelated' section is correctly excluded
	}

	// Use JSON comparison and unmarshal back
	actualJSON, errActual := json.Marshal(overrideFile)
	expectedJSON, errExpected := json.Marshal(expectedOverrides)

	if errActual != nil || errExpected != nil {
		t.Fatalf("Failed to marshal maps to JSON: ActualErr=%v, ExpectedErr=%v", errActual, errExpected)
	}

	// Unmarshal back into generic maps for comparison
	var actualMap, expectedMap map[string]interface{}
	errUnmarshalActual := json.Unmarshal(actualJSON, &actualMap)
	errUnmarshalExpected := json.Unmarshal(expectedJSON, &expectedMap)

	if errUnmarshalActual != nil || errUnmarshalExpected != nil {
		t.Fatalf("Failed to unmarshal JSON back to maps: ActualErr=%v, ExpectedErr=%v", errUnmarshalActual, errUnmarshalExpected)
	}

	// Compare the unmarshaled maps using cmp
	if !cmp.Equal(actualMap, expectedMap) {
		diff := cmp.Diff(expectedMap, actualMap)
		// Use the original JSON strings and the diff in the error message
		t.Errorf("Generate() override map mismatch (-want +got):\n%s\nActual JSON: %s\nExpected JSON: %s", diff, string(actualJSON), string(expectedJSON))
	}
}
