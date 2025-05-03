// Package generator_test contains tests for the generator package.
package generator

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/override"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/lucas-albers-lz4/irr/pkg/strategy"
	"github.com/stretchr/testify/require"
)

// TestGenerate tests the override generation logic.
func TestGenerate(t *testing.T) {
	testValues := map[string]interface{}{
		"image": "old-registry.com/my-app:v1",
	}
	// Provide minimal non-nil mappings struct
	mappings := &registry.Mappings{}
	testStrategy := strategy.NewPrefixSourceRegistryStrategy(mappings)

	generator := NewGenerator(
		mappings,
		testStrategy,
		[]string{"old-registry.com"}, // sourceRegistries
		[]string{},                   // excludeRegistries
		false,                        // strictMode
		false,                        // templateMode
	)

	overrideFile, err := generator.Generate("test-chart", testValues)
	require.NoError(t, err, "Generate() should not return an error")

	expectedOverrides := map[string]interface{}{
		"image": map[string]interface{}{
			"registry":   "",
			"repository": "old-registry.com/my-app",
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
		t.Errorf("Generate() override map mismatch (-want +got):\n%s", diff)
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
	mappings := &registry.Mappings{
		Entries: []registry.Mapping{
			{Source: "old-registry.com", Target: "mapped-registry.com/oldreg"},
			// No mapping for other-registry.com
		},
	}
	testStrategy := strategy.NewPrefixSourceRegistryStrategy(mappings)

	generator := NewGenerator(
		mappings,
		testStrategy,
		[]string{"old-registry.com", "other-registry.com"}, // sourceRegistries
		[]string{}, // excludeRegistries
		false,      // strictMode
		false,      // templateMode
	)

	overrideFile, err := generator.Generate("test-chart-mapped", testValues)
	require.NoError(t, err, "Generate() should not return an error")

	expectedOverrides := map[string]interface{}{
		"image": map[string]interface{}{
			"registry":   "mapped-registry.com/oldreg",
			"repository": "mapped-registry.com/oldreg/app1",
			"tag":        "v1",
		},
		"nested": map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "",
				"repository": "other-registry.com/app2",
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
		t.Errorf("Generate() override map mismatch (-want +got):\n%s", diff)
	}
}

func TestRemoveValueAtPath(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		path     []string
		expected map[string]interface{}
	}{
		{
			name:     "Remove top-level key",
			data:     map[string]interface{}{"key1": "value1", "key2": "value2"},
			path:     []string{"key1"},
			expected: map[string]interface{}{"key2": "value2"},
		},
		{
			name:     "Remove nested key",
			data:     map[string]interface{}{"top": map[string]interface{}{"mid": map[string]interface{}{"leaf": "value"}, "other": "data"}},
			path:     []string{"top", "mid", "leaf"},
			expected: map[string]interface{}{"top": map[string]interface{}{"other": "data"}}, // mid becomes empty and is removed
		},
		{
			name:     "Remove key that makes parent empty",
			data:     map[string]interface{}{"parent": map[string]interface{}{"child": "value"}},
			path:     []string{"parent", "child"},
			expected: map[string]interface{}{}, // parent becomes empty and is removed
		},
		{
			name:     "Remove non-existent top-level key",
			data:     map[string]interface{}{"key1": "value1"},
			path:     []string{"nonexistent"},
			expected: map[string]interface{}{"key1": "value1"},
		},
		{
			name:     "Remove non-existent nested key",
			data:     map[string]interface{}{"top": map[string]interface{}{"mid": "value"}},
			path:     []string{"top", "nonexistent", "leaf"},
			expected: map[string]interface{}{"top": map[string]interface{}{"mid": "value"}},
		},
		{
			name:     "Attempt remove through non-map",
			data:     map[string]interface{}{"top": "not a map"},
			path:     []string{"top", "key"},
			expected: map[string]interface{}{"top": "not a map"},
		},
		{
			name:     "Empty path",
			data:     map[string]interface{}{"key": "value"},
			path:     []string{},
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name:     "Nil data map",
			data:     nil,
			path:     []string{"key"},
			expected: nil,
		},
		{
			name:     "Empty data map",
			data:     map[string]interface{}{},
			path:     []string{"key"},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare the data to pass to the function
			var dataToModify map[string]interface{}
			if tt.data != nil {
				// Make a deep copy using override.DeepCopy ONLY for non-nil data
				dataCopy, ok := override.DeepCopy(tt.data).(map[string]interface{})
				if !ok {
					t.Fatalf("DeepCopy did not return a map[string]interface{} for non-nil input")
				}
				dataToModify = dataCopy
			} else {
				// Pass nil directly if the test case data is nil
				dataToModify = nil
			}

			removeValueAtPath(dataToModify, tt.path)

			if !cmp.Equal(tt.expected, dataToModify) {
				diff := cmp.Diff(tt.expected, dataToModify)
				t.Errorf("removeValueAtPath() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNormalizeKubeStateMetricsOverrides(t *testing.T) {
	// Mock mappings
	mockMappings := &registry.Mappings{
		Entries: []registry.Mapping{
			{Source: "registry.k8s.io", Target: "my-target.com/k8s"},
			{Source: "quay.io", Target: "my-target.com/quay"},
		},
	}
	// Mock path strategy with mappings
	mockStrategy := strategy.NewPrefixSourceRegistryStrategy(mockMappings)

	tests := []struct {
		name              string
		detectedImages    []image.DetectedImage
		initialOverrides  map[string]interface{}
		expectedOverrides map[string]interface{}
	}{
		{
			name: "KSM image detected and normalized",
			detectedImages: []image.DetectedImage{
				{
					Path: []string{"custom-path", "image"},
					Reference: &image.Reference{
						Registry:   "registry.k8s.io",
						Repository: "kube-state-metrics/kube-state-metrics",
						Tag:        "v2.10.1",
					},
				},
				{
					Path: []string{"other", "image"},
					Reference: &image.Reference{
						Registry:   "quay.io",
						Repository: "prometheus/node-exporter",
						Tag:        "v1.7.0",
					},
				},
			},
			initialOverrides: map[string]interface{}{ // Simulate overrides placed by generic logic
				"custom-path": map[string]interface{}{ // KSM incorrectly placed here initially
					"image": map[string]interface{}{ // Structure may vary slightly depending on exact logic
						"registry":   "my-target.com/k8s",
						"repository": "registryk8sio/kube-state-metrics/kube-state-metrics",
						"tag":        "v2.10.1",
					},
				},
				"other": map[string]interface{}{ // Other image override remains
					"image": map[string]interface{}{ // Structure may vary slightly
						"registry":   "my-target.com/quay",
						"repository": "quayio/prometheus/node-exporter",
						"tag":        "v1.7.0",
					},
				},
			},
			expectedOverrides: map[string]interface{}{ // KSM moved to top level, custom-path removed
				KubeStateMetricsKey: map[string]interface{}{ // Correct top-level key
					"image": map[string]interface{}{ // Canonical structure
						"registry":   "my-target.com/k8s",
						"repository": "registryk8sio/kube-state-metrics/kube-state-metrics",
						"tag":        "v2.10.1",
					},
				},
				"other": map[string]interface{}{ // Other image override remains
					"image": map[string]interface{}{ // Structure may vary slightly
						"registry":   "my-target.com/quay",
						"repository": "quayio/prometheus/node-exporter",
						"tag":        "v1.7.0",
					},
				},
			},
		},
		{
			name: "KSM image already at correct top-level path",
			detectedImages: []image.DetectedImage{
				{
					Path: []string{KubeStateMetricsKey, "image"}, // Correct path
					Reference: &image.Reference{
						Registry:   "registry.k8s.io",
						Repository: "kube-state-metrics/kube-state-metrics",
						Tag:        "v2.10.1",
					},
				},
			},
			initialOverrides: map[string]interface{}{ // Simulate overrides already correctly placed
				KubeStateMetricsKey: map[string]interface{}{ // Correct top-level key
					"image": map[string]interface{}{ // Canonical structure
						"registry":   "my-target.com/k8s",
						"repository": "registryk8sio/kube-state-metrics/kube-state-metrics",
						"tag":        "v2.10.1",
					},
				},
			},
			expectedOverrides: map[string]interface{}{ // Should remain unchanged
				KubeStateMetricsKey: map[string]interface{}{ // Correct top-level key
					"image": map[string]interface{}{ // Canonical structure
						"registry":   "my-target.com/k8s",
						"repository": "registryk8sio/kube-state-metrics/kube-state-metrics",
						"tag":        "v2.10.1",
					},
				},
			},
		},
		{
			name: "No KSM image detected",
			detectedImages: []image.DetectedImage{
				{
					Path: []string{"app", "image"},
					Reference: &image.Reference{
						Registry:   "docker.io",
						Repository: "library/nginx",
						Tag:        "stable",
					},
				},
			},
			initialOverrides: map[string]interface{}{ // Regular override
				"app": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "my-target.com/dockerhub",
						"repository": "dockerio/library/nginx",
						"tag":        "stable",
					},
				},
			},
			expectedOverrides: map[string]interface{}{ // Should remain unchanged
				"app": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "my-target.com/dockerhub",
						"repository": "dockerio/library/nginx",
						"tag":        "stable",
					},
				},
			},
		},
		{
			name:              "Empty detected images",
			detectedImages:    []image.DetectedImage{},
			initialOverrides:  map[string]interface{}{},
			expectedOverrides: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a deep copy of the initial overrides using override.DeepCopy
			overridesCopy, ok := override.DeepCopy(tt.initialOverrides).(map[string]interface{}) // Use override.DeepCopy and assert type
			if !ok {
				t.Fatalf("DeepCopy did not return a map[string]interface{}")
			}

			normalizeKubeStateMetricsOverrides(tt.detectedImages, overridesCopy, mockStrategy, mockMappings)

			if !cmp.Equal(tt.expectedOverrides, overridesCopy) {
				diff := cmp.Diff(tt.expectedOverrides, overridesCopy)
				t.Errorf("normalizeKubeStateMetricsOverrides() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
