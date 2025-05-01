package helm

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/cli/values"

	helmtypes "github.com/lucas-albers-lz4/irr/pkg/helmtypes"
)

const (
	// TestChartPath is the path to the test chart used in tests
	TestChartPath = "../../test-data/charts/parent-test"
)

func TestDefaultChartLoader_LoadChartWithValues(t *testing.T) {
	if _, err := os.Stat(TestChartPath); err != nil {
		t.Skipf("Skipping test: chart path does not exist: %v", err)
	}

	t.Run("basic load with default values", func(t *testing.T) {
		loader := NewChartLoader()

		chart, mergedValues, err := loader.LoadChartWithValues(&helmtypes.ChartLoaderOptions{
			ChartPath:  TestChartPath,
			ValuesOpts: values.Options{},
		})

		require.NoError(t, err)
		require.NotNil(t, chart)
		require.NotNil(t, mergedValues)

		// Check chart metadata
		assert.Equal(t, "parent-test", chart.Name())

		// Check that merged values include expected top-level keys
		assert.Contains(t, mergedValues, "image")
		assert.Contains(t, mergedValues, "parentImage")

		// Check subchart values
		childValues, ok := mergedValues["child"].(map[string]interface{})
		require.True(t, ok, "child values should be present")
		assert.Contains(t, childValues, "image")
		assert.Contains(t, childValues, "extraImage")

		anotherChildValues, ok := mergedValues["another-child"].(map[string]interface{})
		require.True(t, ok, "another-child values should be present")
		assert.Contains(t, anotherChildValues, "image")
		assert.Contains(t, anotherChildValues, "monitoring")
	})
}

func TestDefaultChartLoader_LoadChartAndTrackOrigins(t *testing.T) {
	if _, err := os.Stat(TestChartPath); err != nil {
		t.Skipf("Skipping test: chart path does not exist: %v", err)
	}

	t.Run("load with origin tracking", func(t *testing.T) {
		loader := NewChartLoader()

		context, err := loader.LoadChartAndTrackOrigins(&helmtypes.ChartLoaderOptions{
			ChartPath:  TestChartPath,
			ValuesOpts: values.Options{},
		})

		require.NoError(t, err)
		require.NotNil(t, context)

		// Check chart metadata
		assert.Equal(t, "parent-test", context.LoadedChart.Name())

		// Check merged values
		assert.Contains(t, context.MergedValues, "image")
		assert.Contains(t, context.MergedValues, "parentImage")

		// Check that we have origins for some key values
		origin, exists := context.Origins["image.repository"]
		assert.True(t, exists, "Should have origin for image.repository")
		if exists {
			assert.Equal(t, helmtypes.OriginChartDefault, origin.Type)
			assert.Equal(t, "parent-test", origin.ChartName)
		}

		// Check subchart value origins
		childImageOrigin, exists := context.Origins["child.image.repository"]
		assert.True(t, exists, "Should have origin for child.image.repository")
		if exists {
			assert.Equal(t, helmtypes.OriginChartDefault, childImageOrigin.Type)
			assert.Equal(t, "child", childImageOrigin.ChartName, "Subchart value origin should have subchart name")
		}

		anotherChildImageOrigin, exists := context.Origins["another-child.image.repository"]
		assert.True(t, exists, "Should have origin for another-child.image.repository")
		if exists {
			assert.Equal(t, helmtypes.OriginChartDefault, anotherChildImageOrigin.Type)
			assert.Equal(t, "another-child", anotherChildImageOrigin.ChartName, "Another subchart value origin should have its subchart name")
		}
	})

	t.Run("with user-supplied values", func(t *testing.T) {
		// Create a temporary values file
		tempDir, err := os.MkdirTemp("", "helm-test-*")
		require.NoError(t, err)
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				t.Logf("Warning: Failed to remove temp directory: %v", err)
			}
		}()

		userValuesPath := tempDir + "/user-values.yaml"
		userValues := `
image:
  repository: user/custom-app
  tag: v2.0.0

child:
  extraImage:
    repository: user/custom-nginx
`
		err = os.WriteFile(userValuesPath, []byte(userValues), 0o600)
		require.NoError(t, err)

		loader := NewChartLoader()

		context, err := loader.LoadChartAndTrackOrigins(&helmtypes.ChartLoaderOptions{
			ChartPath: TestChartPath,
			ValuesOpts: values.Options{
				ValueFiles: []string{userValuesPath},
				Values:     []string{"another-child.image.tag=v3.0.0"},
			},
		})

		require.NoError(t, err)
		require.NotNil(t, context)

		// Check user value origins
		imageOrigin, exists := context.Origins["image.repository"]
		assert.True(t, exists)
		if exists {
			assert.Equal(t, helmtypes.OriginUserFile, imageOrigin.Type)
			assert.Equal(t, userValuesPath, imageOrigin.Path)
		}

		// Check set value origin
		tagOrigin, exists := context.Origins["another-child.image.tag"]
		assert.True(t, exists)
		if exists {
			assert.Equal(t, helmtypes.OriginUserSet, tagOrigin.Type)
		}

		// Check the merged values
		if imgMap, ok := context.MergedValues["image"].(map[string]interface{}); ok {
			assert.Equal(t, "user/custom-app", imgMap["repository"])
		} else {
			assert.Fail(t, "image should be a map")
		}

		// Check subchart overrides
		if childMap, ok := context.MergedValues["child"].(map[string]interface{}); ok {
			if extraImg, ok := childMap["extraImage"].(map[string]interface{}); ok {
				assert.Equal(t, "user/custom-nginx", extraImg["repository"])
			} else {
				assert.Fail(t, "child.extraImage should be a map")
			}
		} else {
			assert.Fail(t, "child should be a map")
		}

		if anotherChildMap, ok := context.MergedValues["another-child"].(map[string]interface{}); ok {
			if imgMap, ok := anotherChildMap["image"].(map[string]interface{}); ok {
				assert.Equal(t, "v3.0.0", imgMap["tag"])
			} else {
				assert.Fail(t, "another-child.image should be a map")
			}
		} else {
			assert.Fail(t, "another-child should be a map")
		}
	})
}

func TestMergeAndTrackEdgeCases(t *testing.T) {
	baseOrigin := helmtypes.ValueOrigin{Type: helmtypes.OriginChartDefault, ChartName: "base"}
	sourceOrigin := helmtypes.ValueOrigin{Type: helmtypes.OriginUserFile, Path: "user.yaml"}

	tests := []struct {
		name            string
		initialTarget   map[string]interface{}
		initialOrigins  map[string]helmtypes.ValueOrigin
		source          map[string]interface{}
		expectedTarget  map[string]interface{}
		expectedOrigins map[string]helmtypes.ValueOrigin
	}{
		{
			name: "Scalar overwrites Map",
			initialTarget: map[string]interface{}{
				"key": map[string]interface{}{"subkey": "a"},
			},
			initialOrigins: map[string]helmtypes.ValueOrigin{
				"key":        baseOrigin,
				"key.subkey": baseOrigin,
			},
			source: map[string]interface{}{
				"key": 123,
			},
			expectedTarget: map[string]interface{}{
				"key": 123,
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{
				"key": sourceOrigin, // Origin updated, subkey origin removed
			},
		},
		{
			name: "Map overwrites Scalar",
			initialTarget: map[string]interface{}{
				"key": 123,
			},
			initialOrigins: map[string]helmtypes.ValueOrigin{
				"key": baseOrigin,
			},
			source: map[string]interface{}{
				"key": map[string]interface{}{"subkey": "a"},
			},
			expectedTarget: map[string]interface{}{
				"key": map[string]interface{}{"subkey": "a"},
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{
				"key":        sourceOrigin, // Origin updated for map
				"key.subkey": sourceOrigin, // Origin added for subkey
			},
		},
		{
			name: "Array overwrites Array",
			initialTarget: map[string]interface{}{
				"key": []string{"a", "b"},
			},
			initialOrigins: map[string]helmtypes.ValueOrigin{"key": baseOrigin},
			source: map[string]interface{}{
				"key": []int{1, 2},
			},
			expectedTarget: map[string]interface{}{
				"key": []int{1, 2}, // Array replaced entirely
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{"key": sourceOrigin},
		},
		{
			name:           "Array overwrites Scalar",
			initialTarget:  map[string]interface{}{"key": 123},
			initialOrigins: map[string]helmtypes.ValueOrigin{"key": baseOrigin},
			source: map[string]interface{}{
				"key": []int{1, 2},
			},
			expectedTarget: map[string]interface{}{
				"key": []int{1, 2},
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{"key": sourceOrigin},
		},
		{
			name: "Scalar overwrites Array",
			initialTarget: map[string]interface{}{
				"key": []int{1, 2},
			},
			initialOrigins: map[string]helmtypes.ValueOrigin{"key": baseOrigin},
			source: map[string]interface{}{
				"key": "hello",
			},
			expectedTarget: map[string]interface{}{
				"key": "hello",
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{"key": sourceOrigin},
		},
		{
			name:           "Source Nil overwrites Scalar",
			initialTarget:  map[string]interface{}{"key": 123},
			initialOrigins: map[string]helmtypes.ValueOrigin{"key": baseOrigin},
			source: map[string]interface{}{
				"key": nil,
			},
			expectedTarget: map[string]interface{}{
				"key": nil,
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{"key": sourceOrigin},
		},
		{
			name: "Source Nil overwrites Map",
			initialTarget: map[string]interface{}{
				"key": map[string]interface{}{"subkey": "a"},
			},
			initialOrigins: map[string]helmtypes.ValueOrigin{
				"key":        baseOrigin,
				"key.subkey": baseOrigin,
			},
			source: map[string]interface{}{
				"key": nil,
			},
			expectedTarget: map[string]interface{}{
				"key": nil,
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{
				"key": sourceOrigin, // Origin updated, subkey origin removed
			},
		},
		{
			name:           "Scalar overwrites Target Nil",
			initialTarget:  map[string]interface{}{"key": nil},
			initialOrigins: map[string]helmtypes.ValueOrigin{"key": baseOrigin},
			source: map[string]interface{}{
				"key": 456,
			},
			expectedTarget: map[string]interface{}{
				"key": 456,
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{"key": sourceOrigin},
		},
		{
			name:           "Map overwrites Target Nil",
			initialTarget:  map[string]interface{}{"key": nil},
			initialOrigins: map[string]helmtypes.ValueOrigin{"key": baseOrigin},
			source: map[string]interface{}{
				"key": map[string]interface{}{"subkey": "b"},
			},
			expectedTarget: map[string]interface{}{
				"key": map[string]interface{}{"subkey": "b"},
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{
				"key":        sourceOrigin,
				"key.subkey": sourceOrigin,
			},
		},
		{
			name: "Deep merge with overwrites",
			initialTarget: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2a": "valA",
					"level2b": map[string]interface{}{"level3": 1},
				},
			},
			initialOrigins: map[string]helmtypes.ValueOrigin{
				"level1":                baseOrigin,
				"level1.level2a":        baseOrigin,
				"level1.level2b":        baseOrigin,
				"level1.level2b.level3": baseOrigin,
			},
			source: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2a": "newValA",                                 // Overwrite leaf
					"level2b": "scalarOverwrite",                         // Overwrite map with scalar
					"level2c": map[string]interface{}{"level3new": true}, // Add new map
				},
			},
			expectedTarget: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2a": "newValA",
					"level2b": "scalarOverwrite",
					"level2c": map[string]interface{}{"level3new": true},
				},
			},
			expectedOrigins: map[string]helmtypes.ValueOrigin{
				"level1":                   sourceOrigin, // Overwritten by map merge
				"level1.level2a":           sourceOrigin, // Overwritten by leaf
				"level1.level2b":           sourceOrigin, // Overwritten by scalar, deeper level3 removed
				"level1.level2c":           sourceOrigin, // Added map
				"level1.level2c.level3new": sourceOrigin, // Added leaf
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Make copies to avoid modifying originals between tests
			target := deepCopyMap(tc.initialTarget)
			origins := deepCopyOrigins(tc.initialOrigins)

			// Run mergeAndTrack and check for errors
			if err := mergeAndTrack(target, tc.source, origins, sourceOrigin, ""); err != nil {
				t.Fatalf("mergeAndTrack failed: %v", err)
			}

			assert.Equal(t, tc.expectedTarget, target, "Merged values mismatch")
			assert.Equal(t, tc.expectedOrigins, origins, "Origins mismatch")
		})
	}
}

// Helper function to deep copy maps (simple version for test data)
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{})
	for k, v := range m {
		if vm, ok := v.(map[string]interface{}); ok {
			cp[k] = deepCopyMap(vm)
		} else {
			// Assume other types (scalars, slices) are copyable by assignment
			cp[k] = v
		}
	}
	return cp
}

// Helper function to deep copy origins map
func deepCopyOrigins(m map[string]helmtypes.ValueOrigin) map[string]helmtypes.ValueOrigin {
	cp := make(map[string]helmtypes.ValueOrigin)
	for k, v := range m {
		cp[k] = v // ValueOrigin is a struct, assignment creates a copy
	}
	return cp
}
