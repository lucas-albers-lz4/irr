// Package analysis provides functionality to analyze Helm charts for image references.
package analysis

import (
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// ChartLoader defines the interface for loading Helm charts.
// This allows mocking the loader for testing.
type ChartLoader interface {
	Load(path string) (*chart.Chart, error)
}

// HelmChartLoader implements ChartLoader using the actual Helm loader.
type HelmChartLoader struct{}

// Load uses the Helm library to load a chart.
func (h *HelmChartLoader) Load(path string) (*chart.Chart, error) {
	return loader.Load(path)
}

// Analyzer provides functionality for analyzing Helm charts
type Analyzer struct {
	chartPath string
	loader    ChartLoader // Use the interface instead of direct dependency
}

// NewAnalyzer creates a new Analyzer
// It now takes a ChartLoader as a dependency.
func NewAnalyzer(chartPath string, loader ChartLoader) *Analyzer {
	// If no loader provided, use the default Helm loader.
	if loader == nil {
		loader = &HelmChartLoader{}
	}
	return &Analyzer{
		chartPath: chartPath,
		loader:    loader,
	}
}

// Analyze performs analysis of the chart
func (a *Analyzer) Analyze() (*ChartAnalysis, error) {
	analysis := NewChartAnalysis()

	// Load the chart using the loader interface
	chart, err := a.loader.Load(a.chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	// Analyze values
	if err := a.analyzeValues(chart.Values, "", analysis); err != nil {
		return nil, fmt.Errorf("failed to analyze values: %w", err)
	}

	// Analyze dependencies
	for _, dep := range chart.Dependencies() {
		depAnalysis := NewChartAnalysis()
		if err := a.analyzeValues(dep.Values, "", depAnalysis); err != nil {
			return nil, fmt.Errorf("failed to analyze dependency %s: %w", dep.Name(), err)
		}
		analysis.mergeAnalysis(depAnalysis)
	}

	return analysis, nil
}

// normalizeImageValues extracts and normalizes image map values
func (a *Analyzer) normalizeImageValues(val map[string]interface{}) (string, string, string) {
	// Extract values, handling potential nil cases
	registry, hasRegistry := val["registry"].(string)
	repository, hasRepository := val["repository"].(string)
	if !hasRepository {
		repository = ""
	}

	// Handle tag with type assertion and conversion
	var tag string
	switch t := val["tag"].(type) {
	case string:
		tag = t
	case float64:
		tag = fmt.Sprintf("%.0f", t)
	case int:
		tag = fmt.Sprintf("%d", t)
	case nil:
		tag = "latest"
	default:
		tag = "latest"
	}

	// Default to docker.io if registry is missing or empty
	isDockerRegistry := false
	if !hasRegistry || registry == "" {
		registry = "docker.io"
		isDockerRegistry = true
	} else if registry == "docker.io" {
		isDockerRegistry = true
	}

	// Add library/ prefix ONLY if registry is docker.io and repo is simple (no /)
	if isDockerRegistry && hasRepository && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	// Normalize registry format (trim slash)
	registry = strings.TrimSuffix(registry, "/")

	// Handle registries specified without a TLD (e.g., "myregistry")
	// This should happen AFTER defaulting to docker.io if needed
	if !isDockerRegistry && !strings.Contains(registry, ".") && !strings.Contains(registry, ":") {
		// It's not docker.io, and looks like a hostname without TLD or port.
		// Assume it's meant to be on docker hub under that name? This is ambiguous.
		// Let's revert to the previous logic for this specific case for now.
		// Consider if this case needs clearer definition or should be disallowed.
		registry = "docker.io/" + registry
	}

	return registry, repository, tag
}

// analyzeValues recursively analyzes values to find image patterns
func (a *Analyzer) analyzeValues(values map[string]interface{}, prefix string, analysis *ChartAnalysis) error {
	for k, v := range values {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}

		if err := a.analyzeSingleValue(k, v, path, analysis); err != nil {
			// If analyzing a single value fails, wrap the error with context
			return fmt.Errorf("error analyzing path '%s': %w", path, err)
		}

		// Check for global patterns (registry configurations)
		if a.isGlobalPattern(k, v) {
			pattern := GlobalPattern{
				Type: PatternTypeGlobal,
				Path: path,
			}
			analysis.GlobalPatterns = append(analysis.GlobalPatterns, pattern)
		}
	}

	return nil
}

// analyzeSingleValue analyzes a single key-value pair based on the value type.
func (a *Analyzer) analyzeSingleValue(key string, value interface{}, path string, analysis *ChartAnalysis) error {
	switch val := value.(type) {
	case map[string]interface{}:
		return a.analyzeMapValue(val, path, analysis)
	case string:
		return a.analyzeStringValue(key, val, path, analysis)
	case []interface{}:
		return a.analyzeArray(val, path, analysis) // Keep calling analyzeArray for slices
	default:
		// Ignore other types (bool, int, float, nil, etc.)
		return nil
	}
}

// analyzeMapValue handles analysis when the value is a map.
func (a *Analyzer) analyzeMapValue(val map[string]interface{}, path string, analysis *ChartAnalysis) error {
	// Check if this is an image map pattern
	if a.isImageMap(val) {
		registry, repository, tag := a.normalizeImageValues(val)
		if repository != "" { // Only repository is required for a valid image pattern
			pattern := ImagePattern{
				Path: path,
				Type: PatternTypeMap,
				Structure: map[string]interface{}{
					"registry":   registry,
					"repository": repository,
					"tag":        tag,
				},
				Value: fmt.Sprintf("%s/%s:%s", registry, repository, tag),
				Count: 1,
			}
			analysis.ImagePatterns = append(analysis.ImagePatterns, pattern)
		}
		// Important: If it IS an image map, we *don't* recurse further into it.
	} else {
		// If it's not an image map itself, recurse into its keys/values.
		return a.analyzeValues(val, path, analysis)
	}
	return nil
}

// analyzeStringValue handles analysis when the value is a string.
func (a *Analyzer) analyzeStringValue(key string, val string, path string, analysis *ChartAnalysis) error {
	if a.isImageString(key, val) {
		pattern := ImagePattern{
			Path:  path,
			Type:  PatternTypeString,
			Value: val,
			Count: 1,
		}
		analysis.ImagePatterns = append(analysis.ImagePatterns, pattern)
	}
	return nil
}

// analyzeArray handles array values that might contain image references
func (a *Analyzer) analyzeArray(val []interface{}, path string, analysis *ChartAnalysis) error {
	for i, item := range val {
		itemPath := fmt.Sprintf("%s[%d]", path, i)

		switch v := item.(type) {
		case map[string]interface{}:
			if err := a.analyzeMapItemInArray(v, itemPath, analysis); err != nil {
				return fmt.Errorf("error analyzing map item in array at path '%s': %w", itemPath, err)
			}

		case string:
			// Check if the string itself is an image reference
			if a.isImageString(path, v) { // Pass original path context for string check? Or itemPath?
				// Let's use itemPath for consistency with map logic path construction
				pattern := ImagePattern{
					Path: itemPath, Type: PatternTypeString, Value: v, Count: 1,
				}
				analysis.ImagePatterns = append(analysis.ImagePatterns, pattern)
			}
		}
	}
	return nil
}

// analyzeMapItemInArray handles the logic for processing a map found inside an array element.
func (a *Analyzer) analyzeMapItemInArray(v map[string]interface{}, itemPath string, analysis *ChartAnalysis) error {
	foundPatternInMapItem := false // Flag to prevent duplicate processing

	// 1. Check if this map IS an image map itself
	if a.isImageMap(v) {
		registry, repository, tag := a.normalizeImageValues(v)
		if repository != "" { // Check if it's a valid image map structure
			pattern := ImagePattern{
				Path:      itemPath, // Path is the array index
				Type:      PatternTypeMap,
				Structure: map[string]interface{}{"registry": registry, "repository": repository, "tag": tag},
				Value:     fmt.Sprintf("%s/%s:%s", registry, repository, tag),
				Count:     1,
			}
			analysis.ImagePatterns = append(analysis.ImagePatterns, pattern)
			foundPatternInMapItem = true
		}
	}

	// 2. If it's NOT an image map itself, check if it CONTAINS an 'image:' string key
	if !foundPatternInMapItem {
		if img, ok := v["image"].(string); ok && a.isImageString("image", img) {
			pattern := ImagePattern{
				Path:  itemPath + ".image", // Path includes the field within the array element
				Type:  PatternTypeString,
				Value: img,
				Count: 1,
			}
			analysis.ImagePatterns = append(analysis.ImagePatterns, pattern)
			foundPatternInMapItem = true // Mark as found to avoid redundant recursion
		}
	}

	// 3. Recurse into the map ONLY IF we didn't find a primary pattern above.
	// This prevents adding duplicates when a map IS an image map OR contains \`image:\`
	// but might also contain other nested images deeper within.
	if !foundPatternInMapItem {
		return a.analyzeValues(v, itemPath, analysis)
	}

	return nil
}

// isImageMap checks if a map represents an image configuration
func (a *Analyzer) isImageMap(m map[string]interface{}) bool {
	// Must have repository and either registry or tag
	hasRepository := false
	hasRegistryOrTag := false

	for k := range m {
		switch k {
		case "repository":
			hasRepository = true
		case "registry", "tag":
			hasRegistryOrTag = true
		}
	}

	return hasRepository && hasRegistryOrTag
}

// isImageString checks if a string value represents an image reference
func (a *Analyzer) isImageString(key, value string) bool {
	// Check if key suggests this is an image
	if strings.Contains(strings.ToLower(key), "image") {
		// Basic check for image reference format: repo/name[:tag][@digest]
		parts := strings.Split(value, "/")
		if len(parts) >= 2 {
			lastPart := parts[len(parts)-1]
			return strings.Contains(lastPart, ":") || strings.Contains(lastPart, "@")
		}
	}
	return false
}

// isGlobalPattern checks if this is a global registry configuration
func (a *Analyzer) isGlobalPattern(key string, value interface{}) bool {
	return strings.HasPrefix(key, "global.") && strings.Contains(key, "registry")
}

// mergeAnalysis merges dependency analysis into the main analysis
func (a *ChartAnalysis) mergeAnalysis(dep *ChartAnalysis) {
	a.ImagePatterns = append(a.ImagePatterns, dep.ImagePatterns...)
	a.GlobalPatterns = append(a.GlobalPatterns, dep.GlobalPatterns...)
}
