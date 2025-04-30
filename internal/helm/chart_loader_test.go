package helm

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/cli/values"
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

		chart, mergedValues, err := loader.LoadChartWithValues(&ChartLoaderOptions{
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

		context, err := loader.LoadChartAndTrackOrigins(&ChartLoaderOptions{
			ChartPath:  TestChartPath,
			ValuesOpts: values.Options{},
		})

		require.NoError(t, err)
		require.NotNil(t, context)

		// Check chart metadata
		assert.Equal(t, "parent-test", context.ChartName)

		// Check merged values
		assert.Contains(t, context.Values, "image")
		assert.Contains(t, context.Values, "parentImage")

		// Check that we have origins for some key values
		origin, exists := context.Origins["image.repository"]
		assert.True(t, exists, "Should have origin for image.repository")
		if exists {
			assert.Equal(t, OriginChartDefault, origin.Type)
			assert.Equal(t, "parent-test", origin.ChartName)
		}

		// Check subchart value origins
		childImageOrigin, exists := context.Origins["child.image.repository"]
		assert.True(t, exists, "Should have origin for child.image.repository")
		if exists {
			assert.Equal(t, OriginChartDefault, childImageOrigin.Type)
			assert.Equal(t, "child", childImageOrigin.ChartName, "Subchart value origin should have subchart name")
		}

		anotherChildImageOrigin, exists := context.Origins["another-child.image.repository"]
		assert.True(t, exists, "Should have origin for another-child.image.repository")
		if exists {
			assert.Equal(t, OriginChartDefault, anotherChildImageOrigin.Type)
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

		context, err := loader.LoadChartAndTrackOrigins(&ChartLoaderOptions{
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
			assert.Equal(t, OriginUserFile, imageOrigin.Type)
			assert.Equal(t, userValuesPath, imageOrigin.Path)
		}

		// Check set value origin
		tagOrigin, exists := context.Origins["another-child.image.tag"]
		assert.True(t, exists)
		if exists {
			assert.Equal(t, OriginUserSet, tagOrigin.Type)
		}

		// Check the merged values
		if imgMap, ok := context.Values["image"].(map[string]interface{}); ok {
			assert.Equal(t, "user/custom-app", imgMap["repository"])
		} else {
			assert.Fail(t, "image should be a map")
		}

		// Check subchart overrides
		if childMap, ok := context.Values["child"].(map[string]interface{}); ok {
			if extraImg, ok := childMap["extraImage"].(map[string]interface{}); ok {
				assert.Equal(t, "user/custom-nginx", extraImg["repository"])
			} else {
				assert.Fail(t, "child.extraImage should be a map")
			}
		} else {
			assert.Fail(t, "child should be a map")
		}

		if anotherChildMap, ok := context.Values["another-child"].(map[string]interface{}); ok {
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
