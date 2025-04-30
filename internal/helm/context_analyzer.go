// Package helm provides internal utilities for interacting with Helm.
package helm

import (
	"fmt"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/log"
)

const (
	// DefaultRegistry is the standard Docker Hub registry
	DefaultRegistry = "docker.io"
	// defaultImageTag is the default tag used when none is specified.
	defaultImageTag = "latest"
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
	// Use the refined check to see if this map *directly* defines an image
	if a.isDirectImageMapDefinition(val) {
		// Extract and normalize image values
		registry, repository, tag := a.normalizeImageValues(val)

		// Create an image pattern for the map itself
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

		// If it IS an image map definition, we *might* still need to recurse
		// if other keys exist alongside repository/tag/registry.
		// Let's recurse anyway for now, analyzeValues handles individual fields.
		// The isDirectImageMapDefinition check prevents infinite loops for simple image maps.
		log.Debug("analyzeMapValue: identified as direct image map, but recursing anyway", "path", currentPath)
	}

	// Always recurse into child map values, analyzeValues handles skipping non-map/string/array leaves.
	// The isDirectImageMapDefinition check above is primarily to *detect* the pattern,
	// not necessarily to stop all recursion for that map.
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

// isDirectImageMapDefinition provides a stricter check to identify maps that
// directly define an image using standard keys.
func (a *ContextAwareAnalyzer) isDirectImageMapDefinition(val map[string]interface{}) bool {
	repoVal, hasRepo := val["repository"]
	tagVal, hasTag := val["tag"]

	// Must have repository key
	if !hasRepo {
		return false
	}
	// Repository value must be a non-empty string
	repoStr, repoIsString := repoVal.(string)
	if !repoIsString || repoStr == "" {
		return false
	}

	// Must have tag key (for now, ignoring digest)
	if !hasTag {
		return false
	}
	// Tag value must be a non-empty string
	tagStr, tagIsString := tagVal.(string)
	if !tagIsString || tagStr == "" {
		return false
	}

	// Optional: Check registry if present
	if regVal, hasReg := val["registry"]; hasReg {
		regStr, regIsString := regVal.(string)
		if !regIsString || regStr == "" {
			return false // Registry present but empty or wrong type
		}
	}

	// Passed all checks
	return true
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
	// Defaults
	registry = DefaultRegistry
	repository = ""
	tag = defaultImageTag

	// Prioritize values directly from the map
	if r, ok := val["registry"].(string); ok && r != "" {
		registry = r
	}
	if repoVal, ok := val["repository"].(string); ok && repoVal != "" {
		repository = repoVal // Use repository directly from map
	}
	if tagVal, ok := val["tag"].(string); ok && tagVal != "" {
		tag = tagVal // Use tag directly from map
	}

	// Basic check: If repository is missing, it's not a valid image map for our purposes
	if repository == "" {
		log.Warn("normalizeImageValues: 'repository' key missing or empty", "map", val)
		return "", "", "" // Cannot proceed without repository
	}

	// REFINED TAG LOGIC:
	// If tag wasn't explicitly in map, AND repo string looks like it might contain a tag
	if tag == defaultImageTag && strings.Contains(repository, ":") {
		_, parsedRepo, parsedTag := a.parseImageString(repository)
		if parsedRepo != "" && parsedTag != "" {
			log.Debug("normalizeImageValues: Using tag parsed from repository string", "parsedTag", parsedTag)
			repository = parsedRepo // Update repo if tag was embedded
			tag = parsedTag         // Use tag found in repo string
		}
	}

	// Final normalization (e.g., docker.io/library/) - Apply AFTER deciding repo/tag
	if registry == DefaultRegistry && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	return registry, repository, tag
}

// parseImageString attempts to parse a string into image components.
func (a *ContextAwareAnalyzer) parseImageString(val string) (registry, repository, tag string) {
	// Default values
	registry = DefaultRegistry
	tag = defaultImageTag // Use constant

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
			repository = val // Use the full original value as repo path if first part is not a registry
		}

		// Handle tag (extract from the repository part if present)
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

// getSourcePathForValue resolves the effective source path for a value based on origin.
// For now, it returns the structural path as determined by traversal.
// TODO: Enhance this to properly use origin information if needed for more complex scenarios (e.g., aliases).
func (a *ContextAwareAnalyzer) getSourcePathForValue(valuePath string) string {
	// Simply return the structural path derived from map traversal.
	// The path already includes prefixes like "child." based on the merged value structure.
	log.Debug("getSourcePathForValue returning structural path", "path", valuePath)
	return valuePath
}

// GetContext returns the analysis context.
func (a *ContextAwareAnalyzer) GetContext() *ChartAnalysisContext {
	return a.context
}
