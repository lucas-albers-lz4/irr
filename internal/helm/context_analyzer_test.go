package helm

import (
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

func TestContextAwareAnalyzer_AnalyzeContext(t *testing.T) {
	// Set debug logging for this test
	originalLevel := log.CurrentLevel()
	log.SetLevel(log.LevelDebug)
	defer log.SetLevel(originalLevel) // Reset after test

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
		analysisResult, err := analyzer.AnalyzeContext()
		require.NoError(t, err, "Analysis should succeed")
		require.NotNil(t, analysisResult)

		// Convert to map for easier checking
		patternsMap := make(map[string]analysis.ImagePattern)
		for _, p := range analysisResult.ImagePatterns {
			patternsMap[p.Path] = p
			log.Debug("TestCheck", "path", p.Path, "value", p.Value, "type", p.Type)
		}

		// Check for parent map-based image component: "image.repository"
		// if pattern, ok := patternsMap["image.repository"]; ok {
		// 	assert.Equal(t, analysis.PatternTypeString, pattern.Type)
		// 	// Value might be normalized, check for key parts
		// 	assert.Contains(t, pattern.Value, "parent/app", "Parent repo string value mismatch")
		// } else {
		// 	assert.Fail(t, "Should find parent image pattern for image.repository")
		// }

		// Check for parent image (now expected as a map pattern due to Helm coalescing)
		if pattern, ok := patternsMap["parentImage"]; ok {
			assert.Equal(t, analysis.PatternTypeMap, pattern.Type, "parentImage should be detected as a map pattern")
			require.NotNil(t, pattern.Structure, "parentImage map pattern should have structure")
			assert.Equal(t, "docker.io", pattern.Structure["registry"], "parentImage registry mismatch")
			assert.Equal(t, "parent/app", pattern.Structure["repository"], "parentImage repository mismatch")
			assert.Equal(t, "v1.0.0", pattern.Structure["tag"], "parentImage tag mismatch")
		} else {
			assert.Fail(t, "Should find parent image map pattern for parentImage")
		}

		// Check for child map-based image component: "child.image.repository"
		// if pattern, ok := patternsMap["child.image.repository"]; ok {
		// 	assert.Equal(t, analysis.PatternTypeString, pattern.Type)
		// 	assert.Contains(t, pattern.Value, "nginx", "Child repo string value mismatch") // Check content
		// 	// TODO: Fix the path logging bug if necessary
		// } else {
		// 	// If the previous log bug path `child.child.image.repository` exists, flag that
		// 	if _, bugExists := patternsMap["child.child.image.repository"]; bugExists {
		// 		assert.Fail(t, "Found child image pattern with INCORRECT path 'child.child.image.repository'")
		// 	} else {
		// 		assert.Fail(t, "Should find child image pattern for child.image.repository")
		// 	}
		// }

		// Check for child map-based image component: "child.extraImage.repository"
		// if pattern, ok := patternsMap["child.extraImage.repository"]; ok {
		// 	assert.Equal(t, analysis.PatternTypeString, pattern.Type)
		// 	assert.Contains(t, pattern.Value, "bitnami/nginx", "Child extra repo string value mismatch")
		// } else {
		// 	assert.Fail(t, "Should find child image pattern for child.extraImage.repository")
		// }
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
		analysisResult, err := analyzer.AnalyzeContext()
		require.NoError(t, err, "Analysis should succeed")

		// Verify we find the overridden image
		var foundOverriddenImage bool
		for _, pattern := range analysisResult.ImagePatterns {
			// Check for the correct path "image" (map pattern)
			if pattern.Path == "image" && pattern.Type == analysis.PatternTypeMap {
				// Check if the structure contains the overridden repository
				if repo, ok := pattern.Structure["repository"].(string); ok && repo == "user/overridden-app" {
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
