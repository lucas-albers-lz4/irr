package helm

import (
	"strings"
	"testing"

	analyzer "github.com/lucas-albers-lz4/irr/pkg/analyzer"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"

	helmtypes "github.com/lucas-albers-lz4/irr/pkg/helmtypes"
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
		ctxAnalyzer := NewContextAwareAnalyzer(context)

		// Run analysis
		analysisResult, err := ctxAnalyzer.Analyze()
		require.NoError(t, err, "Analysis should succeed")
		require.NotNil(t, analysisResult)

		// Convert to map for easier checking
		patternsMap := make(map[string]analyzer.ImagePattern)
		for _, p := range analysisResult {
			patternsMap[p.Path] = p
			log.Debug("TestCheck", "path", p.Path, "value", p.Value, "type", p.Type)
		}

		// Check for parent map-based image component: "image.repository"
		if pattern, ok := patternsMap["image.repository"]; ok {
			assert.Equal(t, "string", pattern.Type)
			// Value might be normalized, check for key parts
			assert.Contains(t, pattern.Value, "parent/app", "Parent repo string value mismatch")
		} else {
			assert.Fail(t, "Should find parent image pattern for image.repository")
		}

		// Check for parent image (now expected as a map pattern due to Helm coalescing)
		if pattern, ok := patternsMap["parentImage"]; ok {
			assert.Equal(t, "map", pattern.Type, "parentImage should be detected as a map pattern")
			require.NotNil(t, pattern.Structure, "parentImage map pattern should have structure")
			assert.Equal(t, "docker.io", pattern.Structure.Registry, "parentImage registry mismatch")
			assert.Equal(t, "parent/app", pattern.Structure.Repository, "parentImage repository mismatch")
			assert.Equal(t, "v1.0.0", pattern.Structure.Tag, "parentImage tag mismatch")
		} else {
			assert.Fail(t, "Should find parent image map pattern for parentImage")
		}

		// Check for child map-based image component: "child.image.repository"
		if pattern, ok := patternsMap["child.image.repository"]; ok {
			assert.Equal(t, "string", pattern.Type)
			assert.Contains(t, pattern.Value, "nginx", "Child repo string value mismatch") // Check content
			// TODO: Fix the path logging bug if necessary
		} else {
			// If the previous log bug path `child.child.image.repository` exists, flag that
			if _, bugExists := patternsMap["child.child.image.repository"]; bugExists {
				assert.Fail(t, "Found child image pattern with INCORRECT path 'child.child.image.repository'")
			} else {
				assert.Fail(t, "Should find child image pattern for child.image.repository")
			}
		}

		// Check for child map-based image component: "child.extraImage.repository"
		if pattern, ok := patternsMap["child.extraImage.repository"]; ok {
			assert.Equal(t, "string", pattern.Type)
			assert.Contains(t, pattern.Value, "bitnami/nginx", "Child extra repo string value mismatch")
		} else {
			assert.Fail(t, "Should find child image pattern for child.extraImage.repository")
		}
	})

	t.Run("analyzes values with user overrides", func(t *testing.T) {
		// --- FIX: Create context ONLY with user overrides for this test ---
		// This simulates the highest precedence of user values.
		mergedValues := map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "user/overridden-app",
				"tag":        "v2.0",
			},
			// Include other base values ONLY if strictly necessary for analyzer logic
			// For this test, we only care about verifying the override is found.
		}

		// Create origins map ONLY with the user override origin
		origins := make(map[string]helmtypes.ValueOrigin)
		userOrigin := helmtypes.ValueOrigin{
			Type: helmtypes.OriginUserSet,
			Key:  "--set image...", // Simplified key for test
		}
		origins["image"] = userOrigin            // Origin for the map
		origins["image.repository"] = userOrigin // Origin for the leaf
		origins["image.tag"] = userOrigin        // Origin for the leaf

		context := &helmtypes.ChartAnalysisContext{
			LoadedChart:  chartData, // Keep chart data for context
			MergedValues: mergedValues,
			Origins:      origins,
		}

		// Create analyzer
		ctxAnalyzer := NewContextAwareAnalyzer(context)

		// Run analysis
		analysisResult, err := ctxAnalyzer.Analyze()
		require.NoError(t, err, "Analysis should succeed")

		// Verify we find the overridden image
		log.Debug("Checking for overridden image", "patternCount", len(analysisResult))
		for _, pattern := range analysisResult {
			log.Debug("Checking pattern", "path", pattern.Path, "type", pattern.Type, "value", pattern.Value)
			// Check the map pattern or the string pattern based on analyzer logic
			if pattern.Path == "image" && pattern.Type == "map" {
				log.Debug("Checking map pattern structure", "structure", pattern.Structure)
				if pattern.Structure != nil && pattern.Structure.Repository == "user/overridden-app" {
					log.Debug("Map pattern matched!")
					assert.True(t, true) // Assert immediately on match
					return               // Exit test successfully
				}
			} else if pattern.Path == "image.repository" && pattern.Type == "string" {
				log.Debug("Checking string pattern", "value", pattern.Value, "structure", pattern.Structure)
				// Check contains, as value might be normalized
				if strings.Contains(pattern.Value, "user/overridden-app") {
					log.Debug("String pattern matched via Contains!")
					assert.True(t, true) // Assert immediately on match
					return               // Exit test successfully
				}
				// Also check the structure if it was parsed
				if pattern.Structure != nil && pattern.Structure.Repository == "user/overridden-app" {
					log.Debug("String pattern matched via Structure!")
					assert.True(t, true) // Assert immediately on match
					return               // Exit test successfully
				}
			}
		}
		// If loop completes without finding the pattern
		assert.Fail(t, "Should find overridden image pattern")
	})
}

// Helper function to create a test context
func createTestContext(chartData *chart.Chart) *helmtypes.ChartAnalysisContext {
	// Create a simple context with chart default values
	// Simulate a slightly more realistic merge (though not perfect)
	mergedValues := make(map[string]interface{})
	if chartData.Values != nil {
		mergedValues = chartutil.CoalesceTables(mergedValues, chartData.Values)
	}
	// Simulate subchart merge (using parent-test structure)
	childValues := map[string]interface{}{
		"child": map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "nginx", // Base default from child chart
				"tag":        "latest",
			},
			"extraImage": map[string]interface{}{
				"repository": "bitnami/nginx",
				"tag":        "latest",
			},
		},
	}
	mergedValues = chartutil.CoalesceTables(mergedValues, childValues)

	// Create a more complete origin map for the test case
	origins := make(map[string]helmtypes.ValueOrigin)
	parentOrigin := helmtypes.ValueOrigin{Type: helmtypes.OriginChartDefault, ChartName: chartData.Name(), Path: "values.yaml"}
	childOrigin := helmtypes.ValueOrigin{Type: helmtypes.OriginChartDefault, ChartName: "child", Path: "values.yaml"}
	// Assume merged values reflect parent chart's final state after merging child defaults and parent overrides
	origins["image"] = parentOrigin                // Origin for parent map key
	origins["image.repository"] = parentOrigin     // Origin for parent leaf key
	origins["image.tag"] = parentOrigin            // Origin for parent leaf key
	origins["parentImage"] = parentOrigin          // Origin for parent map key
	origins["parentImage.registry"] = parentOrigin // etc.
	origins["parentImage.repository"] = parentOrigin
	origins["parentImage.tag"] = parentOrigin
	origins["child"] = parentOrigin                 // Origin for the key 'child' itself comes from parent values
	origins["child.image"] = childOrigin            // Origin for the map key 'child.image'
	origins["child.image.repository"] = childOrigin // Origin for the leaf key 'child.image.repository'
	origins["child.image.tag"] = childOrigin
	origins["child.extraImage"] = childOrigin
	origins["child.extraImage.repository"] = childOrigin
	origins["child.extraImage.tag"] = childOrigin

	return &helmtypes.ChartAnalysisContext{
		LoadedChart:  chartData,
		MergedValues: mergedValues,
		Origins:      origins,
	}
}
