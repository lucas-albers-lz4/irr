package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

func TestContextAwareAnalyzer_AnalyzeContext(t *testing.T) {
	// Load the test chart
	chartPath := "../../test-data/charts/parent-test"
	chartData, err := loader.Load(chartPath)
	require.NoError(t, err, "Failed to load test chart")

	t.Run("analyzes parent chart values", func(t *testing.T) {
		// Create analysis context
		context := createTestContext(chartData)

		// Create analyzer
		analyzer := NewContextAwareAnalyzer(context)

		// Run analysis
		analysis, err := analyzer.AnalyzeContext()
		require.NoError(t, err, "Analysis should succeed")
		require.NotNil(t, analysis, "Analysis result should not be nil")

		// Verify image patterns were found
		require.Greater(t, len(analysis.ImagePatterns), 0, "Should find at least one image pattern")

		// Check for specific image patterns from parent values
		foundParentImage := false
		foundChildImage := false
		for _, pattern := range analysis.ImagePatterns {
			if pattern.Path == "parentImage.repository" || pattern.Path == "image.repository" {
				foundParentImage = true
			}
			if pattern.Path == "child.image.repository" || pattern.Path == "child.extraImage.repository" {
				foundChildImage = true
			}
		}

		assert.True(t, foundParentImage, "Should find parent image pattern")
		assert.True(t, foundChildImage, "Should find child image pattern")
	})

	t.Run("analyzes values with user overrides", func(t *testing.T) {
		// Create analysis context with user overrides
		userValues := map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "user/overridden-app",
				"tag":        "v2.0",
			},
		}

		// Set origin for user value
		origins := make(map[string]ValueOrigin)
		origins["image.repository"] = ValueOrigin{
			Type: OriginUserSet,
			Path: "--set image.repository=user/overridden-app",
		}

		context := &ChartAnalysisContext{
			Chart:     chartData,
			Values:    userValues,
			Origins:   origins,
			ChartName: chartData.Name(),
		}

		// Create analyzer
		analyzer := NewContextAwareAnalyzer(context)

		// Run analysis
		analysis, err := analyzer.AnalyzeContext()
		require.NoError(t, err, "Analysis should succeed")

		// Verify we find the overridden image
		var foundOverriddenImage bool
		for _, pattern := range analysis.ImagePatterns {
			if pattern.Path == "image.repository" {
				if pattern.Value == "user/overridden-app" || pattern.Value == DefaultRegistry+"/user/overridden-app:v2.0" {
					foundOverriddenImage = true
					break
				}
			}
		}

		assert.True(t, foundOverriddenImage, "Should find overridden image pattern")
	})
}

// Helper function to create a test context
func createTestContext(chartData *chart.Chart) *ChartAnalysisContext {
	// Create a simple context with chart default values
	mergedValues := map[string]interface{}{
		"image": map[string]interface{}{
			"repository": DefaultRegistry + "/parent/app",
			"tag":        "latest",
		},
		"parentImage": map[string]interface{}{
			"registry":   DefaultRegistry,
			"repository": "parent/app",
			"tag":        "v1.0.0",
		},
		"child": map[string]interface{}{
			"image": map[string]interface{}{
				"repository": DefaultRegistry + "/nginx",
				"tag":        "latest",
			},
			"extraImage": map[string]interface{}{
				"repository": DefaultRegistry + "/bitnami/nginx",
				"tag":        "latest",
			},
		},
	}

	// Create some basic origin tracking
	origins := make(map[string]ValueOrigin)
	origins["image.repository"] = ValueOrigin{
		Type:      OriginChartDefault,
		ChartName: chartData.Name(),
		Path:      "values.yaml",
	}
	origins["parentImage.repository"] = ValueOrigin{
		Type:      OriginChartDefault,
		ChartName: chartData.Name(),
		Path:      "values.yaml",
	}
	origins["child.image.repository"] = ValueOrigin{
		Type:      OriginChartDefault,
		ChartName: "child",
		Path:      "values.yaml",
	}

	return &ChartAnalysisContext{
		Chart:     chartData,
		Values:    mergedValues,
		Origins:   origins,
		ChartName: chartData.Name(),
	}
}
