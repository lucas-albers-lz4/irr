// Package analysis provides the core logic for analyzing Helm chart values to detect container images.
package analysis

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
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
	chartData, err := loader.Load(path)
	if err != nil {
		// Wrap the error from the external loader package
		return nil, fmt.Errorf("failed to load chart from path '%s': %w", path, err)
	}
	return chartData, nil
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
func (a *Analyzer) normalizeImageValues(val map[string]interface{}) (registry, repository, tag string) {
	// Extract values, handling potential nil cases
	registryVal, hasRegistry := val["registry"].(string)
	repositoryVal, hasRepository := val["repository"].(string)
	if !hasRepository {
		repository = ""
	} else {
		repository = repositoryVal
	}

	// Handle tag with type assertion and conversion
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
	if !hasRegistry || registryVal == "" {
		registry = "docker.io"
		isDockerRegistry = true
	} else if registryVal == "docker.io" {
		registry = registryVal
		isDockerRegistry = true
	} else {
		registry = registryVal
	}

	// Add library/ prefix ONLY if registry is docker.io and repo is simple (no /)
	if isDockerRegistry && hasRepository && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	// Normalize registry format (trim slash)
	registry = strings.TrimSuffix(registry, "/")

	// Handle registries specified without a TLD (e.g., "myregistry")
	if !isDockerRegistry && !strings.Contains(registry, ".") && !strings.Contains(registry, ":") {
		registry = "docker.io/" + registry
	}

	return // Named return values are assigned implicitly
}

// analyzeValues recursively analyzes values to find image patterns
func (a *Analyzer) analyzeValues(values map[string]interface{}, prefix string, analysis *ChartAnalysis) error {
	debug.Printf("[analyzeValues ENTER] Prefix: '%s', Map Keys: %v", prefix, reflect.ValueOf(values).MapKeys()) // Log map keys
	defer debug.Printf("[analyzeValues EXIT] Prefix: '%s'", prefix)

	for k, v := range values {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}

		debug.Printf("[analyzeValues LOOP] Path: '%s', Type: %T", path, v)
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
	debug.Printf("[analyzeSingleValue ENTER] Path: '%s', Type: %T", path, value)
	defer debug.Printf("[analyzeSingleValue EXIT] Path: '%s', ImagePatterns Count: %d", path, len(analysis.ImagePatterns))

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
	debug.Printf("[analyzeMapValue ENTER] Path: '%s'", path)
	defer debug.Printf("[analyzeMapValue EXIT] Path: '%s', ImagePatterns Count: %d", path, len(analysis.ImagePatterns))

	// Check if this is an image map pattern
	isMap := a.isImageMap(val)
	debug.Printf("[analyzeMapValue] Path: '%s', isImageMap result: %v", path, isMap)
	if isMap {
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
			debug.Printf("[analyzeMapValue IMAGE APPEND] Path: '%s', Value: '%s'", pattern.Path, pattern.Value)
		}
		// Important: If it IS an image map, we *don't* recurse further into it.
	} else {
		// If it's not an image map itself, recurse into its keys/values.
		return a.analyzeValues(val, path, analysis)
	}
	return nil
}

// analyzeStringValue checks a string value for potential image references
func (a *Analyzer) analyzeStringValue(key, val, path string, analysis *ChartAnalysis) error {
	debug.Printf("[analyzeStringValue ENTER] Path: '%s', Value: '%s'", path, val)
	defer debug.Printf("[analyzeStringValue EXIT] Path: '%s', ImagePatterns Count: %d", path, len(analysis.ImagePatterns))

	// Check if the value is a Go template first
	isTemplate := strings.Contains(val, "{{") && strings.Contains(val, "}}")

	// Check using the heuristic (key name, basic format)
	isHeuristicMatch := a.isImageString(key, val)
	debug.Printf("[analyzeStringValue] Path: '%s', isHeuristicMatch: %v, isTemplate: %v", path, isHeuristicMatch, isTemplate)

	// Consider it an image pattern if it passes the heuristic OR if it's a template string.
	// We let the Generator handle parse errors and template checks later.
	if !isHeuristicMatch && !isTemplate {
		debug.Printf("String at path '%s' does not qualify as image pattern based on heuristic or template detection", path)
		return nil // Not considered an error for analysis
	}

	// If heuristic matched OR it's a template, add it to found patterns.
	pattern := ImagePattern{
		Path:  path,
		Type:  PatternTypeString,
		Value: val, // Store the raw value, including templates
		Count: 1,
	}
	analysis.ImagePatterns = append(analysis.ImagePatterns, pattern)
	debug.Printf("[analyzeStringValue IMAGE APPEND] Path: '%s', Value: '%s'", pattern.Path, pattern.Value)

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
		if len(parts) >= minimumSplitParts {
			lastPart := parts[len(parts)-1]
			return strings.Contains(lastPart, ":") || strings.Contains(lastPart, "@")
		}
	}
	return false
}

// isGlobalPattern checks if a key might represent a global pattern
func (a *Analyzer) isGlobalPattern(key string, _ interface{}) bool {
	// Simple heuristic: check if the key is within a 'global' map
	// A more robust check might involve tracking the full path.
	return key == "global" || strings.HasPrefix(key, "global.")
}

// mergeAnalysis merges dependency analysis into the main analysis
func (a *ChartAnalysis) mergeAnalysis(dep *ChartAnalysis) {
	a.ImagePatterns = append(a.ImagePatterns, dep.ImagePatterns...)
	a.GlobalPatterns = append(a.GlobalPatterns, dep.GlobalPatterns...)
}

// AnalyzerInterface defines the interface for chart analysis.
// It allows for different implementations of the analysis logic.
type AnalyzerInterface interface {
	// ... existing code ...
}

// Result holds the findings of the Helm chart analysis.
type Result struct {
	ChartName    string `json:"chartName" yaml:"chartName"`
	ChartVersion string `json:"chartVersion" yaml:"chartVersion"`
	// ... existing code ...
}

const (
	// minimumSplitParts defines the minimum number of parts expected when checking if a string looks like repo/name:tag
	minimumSplitParts = 2
)
