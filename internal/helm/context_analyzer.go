package helm

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/log"
)

const (
	// DefaultRegistry is the standard Docker Hub registry
	DefaultRegistry = "docker.io"
)

// ContextAwareAnalyzer is an analyzer that uses the ChartAnalysisContext to analyze charts
// with full awareness of subchart values and their origins.
type ContextAwareAnalyzer struct {
	context *ChartAnalysisContext
}

// NewContextAwareAnalyzer creates a new ContextAwareAnalyzer.
func NewContextAwareAnalyzer(context *ChartAnalysisContext) *ContextAwareAnalyzer {
	return &ContextAwareAnalyzer{
		context: context,
	}
}

// AnalyzeContext analyzes a chart with its merged values, considering value origins.
func (a *ContextAwareAnalyzer) AnalyzeContext() (*analysis.ChartAnalysis, error) {
	if a.context == nil {
		return nil, fmt.Errorf("analysis context is nil")
	}

	chartAnalysis := analysis.NewChartAnalysis()

	// Analyze the merged values from the context
	if err := a.analyzeValues(a.context.Values, "", chartAnalysis); err != nil {
		return nil, fmt.Errorf("failed to analyze values: %w", err)
	}

	return chartAnalysis, nil
}

// analyzeValues recursively analyzes a values map to identify container image references.
func (a *ContextAwareAnalyzer) analyzeValues(values map[string]interface{}, prefix string, chartAnalysis *analysis.ChartAnalysis) error {
	log.Debug("analyzeValues ENTER", "prefix", prefix, "keys", reflect.ValueOf(values).MapKeys())
	defer log.Debug("analyzeValues EXIT", "prefix", prefix)

	for k, v := range values {
		currentPath := k
		if prefix != "" {
			currentPath = prefix + "." + k
		}

		log.Debug("analyzeValues LOOP", "path", currentPath, "type", fmt.Sprintf("%T", v))
		if err := a.analyzeSingleValue(k, v, currentPath, chartAnalysis); err != nil {
			// If analyzing a single value fails, wrap the error with context
			return fmt.Errorf("error analyzing path '%s': %w", currentPath, err)
		}

		// Check for global patterns (registry configurations)
		if k == "global" || strings.HasPrefix(k, "global.") {
			pattern := analysis.GlobalPattern{
				Type: analysis.PatternTypeGlobal,
				Path: a.getSourcePathForValue(currentPath),
			}
			chartAnalysis.GlobalPatterns = append(chartAnalysis.GlobalPatterns, pattern)
		}
	}

	return nil
}

// analyzeSingleValue analyzes a single key-value pair based on the value type.
func (a *ContextAwareAnalyzer) analyzeSingleValue(key string, value interface{}, currentPath string, chartAnalysis *analysis.ChartAnalysis) error {
	log.Debug("analyzeSingleValue ENTER", "path", currentPath, "type", fmt.Sprintf("%T", value))
	defer func() {
		log.Debug("analyzeSingleValue EXIT", "path", currentPath, "imagePatternsCount", len(chartAnalysis.ImagePatterns))
	}()

	switch val := value.(type) {
	case map[string]interface{}:
		return a.analyzeMapValue(val, currentPath, chartAnalysis)
	case string:
		return a.analyzeStringValue(key, val, currentPath, chartAnalysis)
	case []interface{}:
		return a.analyzeArrayValue(val, currentPath, chartAnalysis)
	default:
		// Ignore other types (bool, int, float, nil, etc.)
		return nil
	}
}

// analyzeMapValue handles analysis of map values for image references.
func (a *ContextAwareAnalyzer) analyzeMapValue(val map[string]interface{}, currentPath string, chartAnalysis *analysis.ChartAnalysis) error {
	// Check if the map represents an image definition
	if a.isImageMap(val) {
		// Extract and normalize image values
		registry, repository, tag := a.normalizeImageValues(val)

		// Create an image pattern
		imageStructure := map[string]interface{}{
			"registry":   registry,
			"repository": repository,
			"tag":        tag,
		}

		pattern := analysis.ImagePattern{
			Type:      analysis.PatternTypeMap,
			Path:      a.getSourcePathForValue(currentPath),
			Value:     fmt.Sprintf("%s/%s:%s", registry, repository, tag),
			Structure: imageStructure,
			Count:     1,
		}

		// Add the pattern to the analysis
		chartAnalysis.ImagePatterns = append(chartAnalysis.ImagePatterns, pattern)
		return nil
	}

	// If not an image map, recurse into the map
	return a.analyzeValues(val, currentPath, chartAnalysis)
}

// analyzeStringValue handles analysis of string values for image references.
func (a *ContextAwareAnalyzer) analyzeStringValue(key, val, currentPath string, chartAnalysis *analysis.ChartAnalysis) error {
	// Check if the string looks like an image reference
	if a.isImageString(key, val) {
		// Try to parse the image string
		registry, repository, tag := a.parseImageString(val)

		// Create an image pattern
		pattern := analysis.ImagePattern{
			Type:  analysis.PatternTypeString,
			Path:  a.getSourcePathForValue(currentPath),
			Value: val,
			Count: 1,
		}

		// If we successfully parsed the image components, add the structure
		if repository != "" {
			pattern.Structure = map[string]interface{}{
				"registry":   registry,
				"repository": repository,
				"tag":        tag,
			}
		}

		// Add the pattern to the analysis
		chartAnalysis.ImagePatterns = append(chartAnalysis.ImagePatterns, pattern)
	}

	return nil
}

// analyzeArrayValue handles analysis of array values.
func (a *ContextAwareAnalyzer) analyzeArrayValue(val []interface{}, currentPath string, chartAnalysis *analysis.ChartAnalysis) error {
	for i, item := range val {
		itemPath := fmt.Sprintf("%s[%d]", currentPath, i)

		if err := a.analyzeSingleValue("", item, itemPath, chartAnalysis); err != nil {
			return fmt.Errorf("error analyzing array item at path '%s': %w", itemPath, err)
		}
	}

	return nil
}

// isImageMap checks if a map likely represents a Helm image definition.
func (a *ContextAwareAnalyzer) isImageMap(val map[string]interface{}) bool {
	_, hasRepo := val["repository"]
	// Basic check: must have a repository key
	return hasRepo
}

// isImageString uses heuristics to check if a string likely represents a container image reference.
func (a *ContextAwareAnalyzer) isImageString(key, val string) bool {
	// Basic check: at least one slash (/) and optionally a colon (:)
	if strings.Contains(val, "/") && !strings.Contains(val, "{{") {
		return true
	}

	// Keys with 'image' in their name might indicate an image string
	if strings.Contains(strings.ToLower(key), "image") && !strings.Contains(val, "{{") {
		return true
	}

	return false
}

// normalizeImageValues extracts and normalizes image components from a map.
func (a *ContextAwareAnalyzer) normalizeImageValues(val map[string]interface{}) (registry, repository, tag string) {
	// Default values
	registry = DefaultRegistry
	tag = "latest"

	// Extract values from the map
	if reg, ok := val["registry"]; ok && reg != nil {
		registry = fmt.Sprintf("%v", reg)
	}

	if repo, ok := val["repository"]; ok && repo != nil {
		repository = fmt.Sprintf("%v", repo)
	}

	if t, ok := val["tag"]; ok && t != nil {
		tag = fmt.Sprintf("%v", t)
	}

	// Normalize docker.io references
	if registry == DefaultRegistry && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	return registry, repository, tag
}

// parseImageString parses a string image reference into its components.
func (a *ContextAwareAnalyzer) parseImageString(val string) (registry, repository, tag string) {
	// Default values
	registry = DefaultRegistry
	tag = "latest"

	// Basic parsing for format "registry/repository:tag"
	parts := strings.Split(val, "/")
	if len(parts) == 1 {
		// Just repository[:tag]
		repoParts := strings.Split(parts[0], ":")
		repository = repoParts[0]
		if len(repoParts) > 1 {
			tag = repoParts[1]
		}
	} else {
		// registry/repository[:tag] or repository/subpath[:tag]
		registry = parts[0]

		// Check if this is docker.io/library/... or another registry
		if strings.Contains(registry, ".") || registry == "localhost" {
			// Likely a registry
			repository = strings.Join(parts[1:], "/")
		} else {
			// Likely just a repository path, default to docker.io
			registry = DefaultRegistry
			repository = val
		}

		// Handle tag
		if strings.Contains(repository, ":") {
			repoParts := strings.Split(repository, ":")
			repository = repoParts[0]
			if len(repoParts) > 1 {
				tag = repoParts[1]
			}
		}
	}

	// Normalize docker.io references
	if registry == DefaultRegistry && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	return registry, repository, tag
}

// getSourcePathForValue determines the appropriate source path for a value based on its origin.
func (a *ContextAwareAnalyzer) getSourcePathForValue(valuePath string) string {
	if a.context == nil {
		return valuePath
	}

	return a.context.GetSourcePathForValue(valuePath)
}

// GetContext returns the chart analysis context.
func (a *ContextAwareAnalyzer) GetContext() *ChartAnalysisContext {
	return a.context
}
