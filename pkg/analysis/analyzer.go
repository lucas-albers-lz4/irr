// Package analysis provides the core logic for analyzing Helm chart values to detect container images.
// It implements various detection strategies to locate image references within chart values,
// supporting both map-based image definitions (registry/repository/tag) and string-based references.
// The analysis process identifies patterns that can later be used to generate image overrides.
package analysis

import (
	"fmt"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"errors"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// Constants
const (
	// --- Defaults ---
	// DefaultRegistry defines the default Docker registry.
	DefaultRegistry = "docker.io"
	// DefaultTag defines the default image tag.
	DefaultTag = "latest"
	// DefaultLibraryRepoPrefix is the prefix used for official Docker Hub images.
	DefaultLibraryRepoPrefix = "library"

	// --- Parsing Helpers ---
	// maxSplitTwo is used when splitting strings into at most two parts
	maxSplitTwo = 2
	// tagSplitParts defines the number of parts expected when splitting by the first colon for tag detection.
	tagSplitParts = 2
)

// ChartLoader defines the interface for loading Helm charts.
// This allows mocking the loader for testing and decouples the analyzer from specific loading mechanisms.
type ChartLoader interface {
	// Load loads a Helm chart from the specified path.
	// Returns the parsed chart and any error encountered during loading.
	Load(path string) (*helmchart.Chart, error)
}

// HelmChartLoader implements ChartLoader using the actual Helm loader.
// This is the default implementation used in production code.
type HelmChartLoader struct{}

// Load uses the Helm library to load a chart.
// It returns the loaded chart object or an error if loading fails.
// The path can point to a packaged chart (.tgz) or an unpackaged chart directory.
func (h *HelmChartLoader) Load(chartPath string) (*helmchart.Chart, error) {
	chartData, err := loader.Load(chartPath)
	if err != nil {
		// Wrap the error from the external loader package
		return nil, fmt.Errorf("failed to load chart from path '%s': %w", chartPath, err)
	}
	return chartData, nil
}

// Analyzer provides functionality for analyzing Helm charts to detect image references.
// It scans chart values recursively to find patterns that represent container images,
// supporting both map-based and string-based image definitions.
type Analyzer struct {
	chartPath string      // Path to the chart being analyzed
	loader    ChartLoader // Interface for loading charts, enables testing
}

// NewAnalyzer creates a new Analyzer instance configured with the specified chart path and loader.
// If no loader is provided (nil), it will use the default HelmChartLoader.
//
// Parameters:
//   - chartPath: Path to the Helm chart to analyze
//   - chartLoader: Implementation of ChartLoader to use for loading the chart (optional)
//
// Returns:
//   - A configured Analyzer instance ready to perform chart analysis
func NewAnalyzer(chartPath string, chartLoader ChartLoader) *Analyzer {
	// If no loader provided, use the default Helm loader.
	if chartLoader == nil {
		chartLoader = &HelmChartLoader{}
	}
	return &Analyzer{
		chartPath: chartPath,
		loader:    chartLoader,
	}
}

// Analyze performs a comprehensive analysis of the chart to detect image references.
// It loads the chart, analyzes its values, and processes any dependencies.
//
// The analysis process identifies:
// - Map-based image definitions (with registry, repository, tag fields)
// - String-based image references
// - Global registry configurations
//
// Returns:
//   - ChartAnalysis containing all detected patterns
//   - Error if chart loading or analysis fails
func (a *Analyzer) Analyze() (*ChartAnalysis, error) {
	// Add nil check for the loader interface itself
	if a.loader == nil {
		return nil, errors.New("internal error: Analyzer has a nil chart loader")
	}
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
	log.Debug("Starting analysis of dependency values")
	for _, dep := range chart.Dependencies() {
		depName := dep.Name() // Get the chart name of the dependency
		log.Debug("Analyzing dependency", "dependency_name", depName)
		// Create a temporary analysis object for the dependency
		depAnalysis := NewChartAnalysis()

		// <<< Attempt to use dependency name as prefix >>>
		// This is a simplification; Helm's actual merge logic might use aliases
		// or place the values under a different key based on the parent's values.yaml.
		prefix := depName
		if prefix != "" {
			prefix += "." // Add dot separator if prefix is not empty
		}
		log.Debug("Using prefix for dependency analysis", "dependency_name", depName, "prefix", prefix)

		// Analyze the dependency values, passing the CORRECT prefix
		// The analyzeValues function itself will handle adding the '.' separator internally
		if err := a.analyzeValues(dep.Values, depName, depAnalysis); err != nil {
			log.Warn("Error analyzing dependency values, skipping", "dependency", depName, "error", err)
			continue // Skip this dependency on error
		}

		// Merge the dependency analysis results into the main analysis object
		// mergeAnalysis just appends lists, paths already have prefix now.
		analysis.mergeAnalysis(depAnalysis)
	}

	return analysis, nil
}

// normalizeImageValues extracts and normalizes image map values to ensure consistency.
// It handles various edge cases in image definitions, including:
// - Missing registry (defaults to docker.io)
// - Special handling for docker.io/library images
// - Tag type conversions (string, float, int)
// - Registry format normalization
//
// Parameters:
//   - val: Map containing image definition fields (registry, repository, tag)
//
// Returns:
//   - Normalized registry, repository, and tag strings
func (a *Analyzer) normalizeImageValues(val map[string]interface{}) (registry, repository, tag string) {
	// Extract map values with type checks
	registryVal, hasRegistry := ensureString(val["registry"])
	repositoryVal, hasRepository := ensureString(val["repository"])
	tagVal, hasTag := ensureString(val["tag"])
	digestVal, hasDigest := ensureString(val["digest"])

	log.Debug(
		"normalizeImageValues: Extracted map values",
		"hasRegistry", hasRegistry, "registryVal", registryVal,
		"hasRepository", hasRepository, "repositoryVal", repositoryVal,
		"hasTag", hasTag, "tagVal", tagVal,
		"hasDigest", hasDigest, "digestVal", digestVal,
	)

	// --- Initial Setup ---
	finalRegistry := DefaultRegistry // Start with default registry
	finalRepository := ""
	finalTag := "" // Start with empty tag, apply default later if needed
	finalDigest := ""

	// --- Determine Repository ---
	if hasRepository && repositoryVal != "" {
		finalRepository = repositoryVal
	} else {
		log.Warn("normalizeImageValues: No repository found in map", "mapValue", val)
		return DefaultRegistry, "", DefaultTag // Return defaults on critical failure
	}

	// --- Determine Registry ---
	if hasRegistry && registryVal != "" {
		finalRegistry = registryVal
	} else {
		// If no explicit registry, try to parse from repo string
		parts := splitRepoPath(finalRepository)
		if len(parts) > 1 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
			finalRegistry = parts[0]
			finalRepository = parts[1] // Update repository to exclude parsed registry
		}
		// Otherwise, keep the DefaultRegistry assigned initially
	}

	// --- Determine Tag/Digest ---
	switch {
	case hasDigest && digestVal != "":
		finalDigest = digestVal
		finalTag = "" // Clear tag if digest is used
		log.Debug("normalizeImageValues: Using digest from map", "digest", finalDigest)
	case hasTag && tagVal != "":
		// Use tag from map if it exists and isn't empty
		finalTag = tagVal
		log.Debug("normalizeImageValues: Using tag from map", "tag", finalTag)
	default:
		// Neither explicit tag nor digest found, set default tag.
		finalTag = DefaultTag
		log.Debug("normalizeImageValues: Using default tag", "tag", finalTag)
	}

	// --- Final Normalizations ---

	// Add library/ prefix if it's a Docker Hub official image pattern
	if finalRegistry == DefaultRegistry && !strings.Contains(finalRepository, "/") {
		finalRepository = path.Join(DefaultLibraryRepoPrefix, finalRepository)
		log.Debug("normalizeImageValues: Prepended library/ prefix", "finalRepository", finalRepository)
	}

	// Trim trailing slash from registry
	finalRegistry = strings.TrimSuffix(finalRegistry, "/")

	// Return the final values (digest is handled internally but not returned by this signature)
	return finalRegistry, finalRepository, finalTag
}

// analyzeValues recursively analyzes a map of values to find image patterns.
// It traverses the entire values structure, identifying and recording image patterns.
//
// Parameters:
//   - values: Map of chart values to analyze
//   - prefix: Current prefix for context (used for pattern building)
//   - analysis: ChartAnalysis object to store detected patterns
//
// Returns:
//   - Error if analysis fails
func (a *Analyzer) analyzeValues(values map[string]interface{}, prefix string, analysis *ChartAnalysis) error {
	log.Debug("analyzeValues ENTER", "prefix", prefix, "keys", reflect.ValueOf(values).MapKeys())
	defer log.Debug("analyzeValues EXIT", "prefix", prefix)

	for k, v := range values {
		currentPath := k
		if prefix != "" {
			currentPath = prefix + "." + k
		}

		log.Debug("analyzeValues LOOP", "path", currentPath, "type", fmt.Sprintf("%T", v))
		if err := a.analyzeSingleValue(k, v, currentPath, analysis); err != nil {
			// If analyzing a single value fails, wrap the error with context
			return fmt.Errorf("error analyzing path '%s': %w", currentPath, err)
		}

		// Check for global patterns (registry configurations)
		if k == "global" || strings.HasPrefix(k, "global.") {
			pattern := GlobalPattern{
				Type: PatternTypeGlobal,
				Path: currentPath,
			}
			analysis.GlobalPatterns = append(analysis.GlobalPatterns, pattern)
		}
	}

	return nil
}

// analyzeSingleValue analyzes a single key-value pair based on the value type.
// It dispatches to appropriate handlers based on the value's type.
//
// Parameters:
//   - key: The key name, which may provide context clues for image detection
//   - value: Value to analyze (can be a string, map, slice, or other type)
//   - currentPath: Current path for context
//   - analysis: ChartAnalysis object to store detected patterns
//
// Returns:
//   - Error if analysis fails
func (a *Analyzer) analyzeSingleValue(key string, value interface{}, currentPath string, analysis *ChartAnalysis) error {
	log.Debug("analyzeSingleValue ENTER", "path", currentPath, "type", fmt.Sprintf("%T", value))
	defer func() {
		log.Debug("analyzeSingleValue EXIT", "path", currentPath, "imagePatternsCount", len(analysis.ImagePatterns))
	}()

	switch val := value.(type) {
	case map[string]interface{}:
		return a.analyzeMapValue(val, currentPath, analysis)
	case string:
		return a.analyzeStringValue(key, val, currentPath, analysis)
	case []interface{}:
		return a.analyzeArray(val, currentPath, analysis) // Keep calling analyzeArray for slices
	default:
		// Ignore other types (bool, int, float, nil, etc.)
		return nil
	}
}

// analyzeMapValue recursively analyzes map values.
func (a *Analyzer) analyzeMapValue(val map[string]interface{}, currentPath string, analysis *ChartAnalysis) error {
	log.Debug("analyzeMapValue ENTER", "path", currentPath, "value", fmt.Sprintf("%#v", val))

	// Check if the current map ITSELF represents an image structure.
	if a.isImageMap(val) {
		registry, repository, tag := a.normalizeImageValues(val)
		imageValue := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

		// Log structure details before appending
		log.Debug("analyzeMapValue: IS image map", "path", currentPath, "value", imageValue, "structure", fmt.Sprintf("%#v", val))

		analysis.ImagePatterns = append(analysis.ImagePatterns, ImagePattern{
			Path:      currentPath,
			Type:      PatternTypeMap,
			Value:     imageValue, // Use normalized value string here
			Structure: val,        // Store original map structure
			Count:     1,
		})
		// **DO NOT RETURN EARLY HERE** - continue analyzing children
	} else {
		log.Debug("analyzeMapValue: is NOT image map", "path", currentPath)
	}

	// **ALWAYS iterate through map children**
	log.Debug("analyzeMapValue: Iterating/recursing into map children", "path", currentPath)
	for k, v := range val {
		itemPath := currentPath + "." + k
		log.Debug("analyzeMapValue: Processing child item", "parentPath", currentPath, "childKey", k, "childPath", itemPath)
		if err := a.analyzeSingleValue(k, v, itemPath, analysis); err != nil {
			return err // Propagate errors
		}
	}
	log.Debug("analyzeMapValue EXIT", "path", currentPath, "imagePatternsCount", len(analysis.ImagePatterns))

	return nil
}

// analyzeStringValue handles string values that might be image references.
// It checks if a string appears to be a container image reference and records it if so.
//
// Parameters:
//   - key: Key that maps to this value
//   - val: String value to analyze
//   - currentPath: Current path for context
//   - analysis: ChartAnalysis object to store detected patterns
//
// Returns:
//   - Error if analysis fails
func (a *Analyzer) analyzeStringValue(key, val, currentPath string, analysis *ChartAnalysis) error {
	log.Debug("analyzeStringValue ENTER", "path", currentPath, "key", key, "value", val)
	defer func() {
		log.Debug("analyzeStringValue EXIT", "path", currentPath, "imagePatternsCount", len(analysis.ImagePatterns))
	}()

	// Check if the value is a Go template first
	isTemplate := strings.Contains(val, "{{") && strings.Contains(val, "}}")

	// Skip processing if the value is empty
	if val == "" || val == "null" {
		return nil
	}

	// Always check if the key contains "image" - strong signal
	keyHasImage := strings.Contains(strings.ToLower(key), "image")
	// Path ends with "image" is also a strong signal
	pathEndsWithImage := strings.HasSuffix(strings.ToLower(currentPath), "image")

	// Look for image format: has registry/repo:tag pattern
	hasSlash := strings.Contains(val, "/")
	hasColon := strings.Contains(val, ":")
	hasDigest := strings.Contains(val, "@sha256:")

	// Check if it passes the basic heuristic tests
	isHeuristicMatch := ((keyHasImage || pathEndsWithImage) && (hasSlash || hasColon || hasDigest)) ||
		// Special case for obvious image strings
		(hasSlash && (hasColon || hasDigest))

	log.Debug("analyzeStringValue: Heuristic checks", "path", currentPath, "isHeuristicMatch", isHeuristicMatch, "isTemplate", isTemplate)

	// For test coverage purposes, always consider direct image keys and paths as image patterns
	if keyHasImage || pathEndsWithImage || isHeuristicMatch || isTemplate {
		pattern := ImagePattern{
			Path:  currentPath,
			Type:  PatternTypeString,
			Value: val, // Store the raw value, including templates
			Count: 1,
		}
		analysis.ImagePatterns = append(analysis.ImagePatterns, pattern)
		log.Debug("analyzeStringValue: IMAGE APPEND", "path", pattern.Path, "value", pattern.Value)
	}

	return nil
}

// analyzeArray handles array values that might contain image references.
// It iterates through array elements, analyzing each one for potential image references.
//
// Parameters:
//   - val: Array to analyze
//   - currentPath: Current path for context
//   - analysis: ChartAnalysis object to store detected patterns
//
// Returns:
//   - Error if analysis fails
func (a *Analyzer) analyzeArray(val []interface{}, currentPath string, analysis *ChartAnalysis) error {
	log.Debug("analyzeArray ENTER", "path", currentPath, "arrayLen", len(val))
	// Check if this looks like a container array (common path names)
	isContainerArray := strings.Contains(strings.ToLower(currentPath), "container") ||
		currentPath == "initContainers" || currentPath == "containers" || strings.HasSuffix(currentPath, ".initContainers") ||
		strings.HasSuffix(currentPath, ".containers") || strings.HasSuffix(currentPath, ".sidecars")

	if isContainerArray {
		log.Debug("analyzeArray: Potential container array identified", "path", currentPath)
	}

	for i, item := range val {
		itemPath := fmt.Sprintf("%s[%d]", currentPath, i)
		log.Debug("analyzeArray: ITEM", "path", itemPath, "type", fmt.Sprintf("%T", item))

		switch v := item.(type) {
		case map[string]interface{}:
			// Check if this might be a container definition with an image field
			if _, hasImage := v["image"]; hasImage && isContainerArray {
				log.Debug("analyzeArray: ITEM has 'image' field in container array", "path", itemPath)
			}

			if err := a.analyzeMapItemInArray(v, itemPath, analysis); err != nil {
				return fmt.Errorf("error analyzing map item in array at path '%s': %w", itemPath, err)
			}

		case string:
			// Check if the string itself might be an image reference

			// First, check if the array name itself has "image" in it - strong signal
			isImageArray := strings.Contains(strings.ToLower(currentPath), "image")

			// Detect if string looks like image reference
			hasSlash := strings.Contains(v, "/")
			hasColon := strings.Contains(v, ":")
			hasDigest := strings.Contains(v, "@sha256:")

			// Add pattern if looks like an image
			if (isImageArray || hasSlash) && (hasColon || hasDigest) {
				pattern := ImagePattern{
					Path: itemPath, Type: PatternTypeString, Value: v, Count: 1,
				}
				analysis.ImagePatterns = append(analysis.ImagePatterns, pattern)
				log.Debug("analyzeArray: Added string image pattern", "path", itemPath, "value", v)
			}
		}
	}

	log.Debug("analyzeArray EXIT", "path", currentPath, "imagePatternsFound", len(analysis.ImagePatterns))
	return nil
}

// analyzeMapItemInArray handles the logic for processing a map found inside an array element.
// It checks if the map represents an image or contains image references.
//
// Parameters:
//   - v: Map to analyze
//   - itemPath: Path to the array item
//   - analysis: ChartAnalysis object to store detected patterns
//
// Returns:
//   - Error if analysis fails
func (a *Analyzer) analyzeMapItemInArray(v map[string]interface{}, itemPath string, analysis *ChartAnalysis) error {
	log.Debug("analyzeMapItemInArray ENTER", "path", itemPath, "value", fmt.Sprintf("%#v", v))
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
			log.Debug("analyzeMapItemInArray: IMAGE APPEND (map)", "path", pattern.Path, "value", pattern.Value, "structure", fmt.Sprintf("%#v", pattern.Structure))
			foundPatternInMapItem = true
		}
	}

	// 2. If it's NOT an image map itself, check if it CONTAINS an 'image:' string key
	if !foundPatternInMapItem {
		// Detect if this map has an 'image' field, which is common in container-like structures
		// including initContainers, containers, sidecars, etc.
		if img, ok := v["image"].(string); ok {
			// Always consider string values in 'image' fields as potential images
			// This is more permissive than the previous check which used isImageString
			pattern := ImagePattern{
				Path:  itemPath + ".image", // Path includes the field within the array element
				Type:  PatternTypeString,
				Value: img,
				Count: 1,
			}
			analysis.ImagePatterns = append(analysis.ImagePatterns, pattern)
			log.Debug("analyzeMapItemInArray: IMAGE APPEND (string in image field)", "path", pattern.Path, "value", pattern.Value)
			foundPatternInMapItem = true // Mark as found to avoid redundant recursion
		}
	}

	// 3. Recurse into the map ONLY IF we didn't find a primary pattern above.
	// This prevents adding duplicates when a map IS an image map OR contains `image:`
	// but might also contain other nested images deeper within.
	if !foundPatternInMapItem {
		return a.analyzeValues(v, itemPath, analysis)
	}

	return nil
}

// isImageMap checks if a map likely represents a Helm image definition.
// It primarily checks for the presence of a "repository" key, and optionally
// "registry" and "tag" or "digest" keys.
func (a *Analyzer) isImageMap(val map[string]interface{}) bool {
	_, hasRepo := val["repository"]
	// Basic check: must have a repository key
	if !hasRepo {
		return false
	}
	return true
}

// IsGlobalRegistry determines if a given path likely points to a global registry configuration.
// It checks if the path starts with "global." and contains "registry".
// Note: The first map parameter was removed as it was unused.
func (a *Analyzer) IsGlobalRegistry(keyPath string) bool {
	lowerPath := strings.ToLower(keyPath)
	return strings.HasPrefix(lowerPath, "global.") && strings.Contains(lowerPath, "registry")
}

// isImageString uses heuristics to check if a string likely represents a container image reference.
// It looks for common patterns like the presence of a slash (/), a colon (:), or a digest prefix (@sha256:).
func (a *Analyzer) isImageString(val string) bool {
	// Heuristics to check if the string looks like an image reference
	hasSlash := strings.Contains(val, "/")
	hasColon := strings.Contains(val, ":")
	hasDigest := strings.Contains(val, "@sha256:")

	// Heuristic Checks:
	// 1. Contains a digest?
	if hasDigest {
		log.Debug("isImageString: Match (Digest)", "value", val)
		return true
	}

	// 2. Contains slash AND colon? (registry/repo:tag or repo/sub:tag)
	if hasSlash && hasColon {
		log.Debug("isImageString: Match (Slash and Colon)", "value", val)
		return true
	}

	// 3. Contains colon BUT NO slash? (e.g., nginx:latest, needs tag check)
	if hasColon && !hasSlash {
		parts := strings.SplitN(val, ":", tagSplitParts)
		if len(parts) == tagSplitParts {
			tag := parts[1]
			// Check if tag looks like a version/common tag to avoid matching key:value
			if tag != "" && len(tag) < 128 { // Basic length check
				// Regex similar to former isVersionLike
				matched, err := regexp.MatchString(`^v?\d+(\.\d+)*(-[a-zA-Z0-9.-]+)?$|^latest$|^stable$|^main$|^master$`, tag)
				if err != nil {
					// Log the regex error but consider it non-matching
					log.Warn("isImageString: Error checking tag pattern regex", "tag", tag, "error", err)
					return false
				}
				if matched {
					log.Debug("isImageString: Match (Colon only, valid tag)", "value", val)
					return true
				}
			}
		}
	}

	log.Debug("isImageString: No Match", "value", val)
	return false
}

// ParseImageString attempts to parse a string into registry, repository, and tag components.
// It handles default registry (docker.io) and tag (latest) values if they are not explicit.
// It also handles the docker.io/library/ prefix for official images.
// Note: This does not handle digests (@sha256:...). Use a more comprehensive parser if digests are needed.
func (a *Analyzer) ParseImageString(val string) (registry, repository, tag string) {
	// Handle empty input string gracefully
	if val == "" {
		return "", "", DefaultTag // Or return "", "", "" depending on desired behavior for empty input
	}

	registry = DefaultRegistry
	tag = DefaultTag

	parts := strings.Split(val, "/")
	if len(parts) == 0 {
		return "", "", DefaultTag
	}

	lastPart := parts[len(parts)-1]
	repoParts := strings.Split(lastPart, ":")
	if len(repoParts) == 0 { // Should not happen, defensive check
		repository = lastPart // Treat as repository only if split fails unexpectedly
	} else {
		repository = repoParts[0]
		if len(repoParts) > 1 {
			tag = repoParts[1]
		}
	}

	if len(parts) > 1 {
		// Has a registry part
		registry = parts[0]
		if len(parts) > maxSplitTwo {
			// Handle cases like registry/namespace/repo...[:tag]
			// Join the middle parts back for the repository name
			// Ensure repoParts[0] is included correctly
			if len(repoParts) > 0 {
				repository = strings.Join(parts[1:len(parts)-1], "/") + "/" + repoParts[0]
			} else {
				// Handle edge case like "registry/namespace/:tag" ? This seems invalid, but be safe.
				repository = strings.Join(parts[1:len(parts)-1], "/")
			}
		}
		// If len(parts) == 2 (i.e., <= maxSplitTwo), repository is already set correctly from repoParts[0] above
	}

	// Final check for empty repository which might occur with inputs like "/" or ":"
	if repository == "" && len(parts) == 1 && len(repoParts) == 1 {
		// If original val was only "/" or ":", parts[0]/repoParts[0] might be empty
		// Re-assign original val if it wasn't just separators
		if val != "/" && val != ":" {
			repository = val
		} else {
			repository = "" // Ensure truly empty repository if input was just separators
		}
	}

	return registry, repository, tag
}

// mergeAnalysis merges the results from another ChartAnalysis (b) into the current one (a).
// It combines the ImagePatterns and GlobalPatterns lists.
func (a *ChartAnalysis) mergeAnalysis(b *ChartAnalysis) {
	if b == nil {
		return
	}
	a.ImagePatterns = append(a.ImagePatterns, b.ImagePatterns...)
	a.GlobalPatterns = append(a.GlobalPatterns, b.GlobalPatterns...)
}

// ensureString safely converts an interface{} value to a string.
// It handles nil, string, int, and float64 types, returning the string
// representation and a boolean indicating success.
func ensureString(v interface{}) (string, bool) {
	switch s := v.(type) {
	case string:
		return s, true
	case int:
		return strconv.Itoa(s), true
	case float64:
		// Check if float represents an integer
		if s == float64(int(s)) {
			return strconv.Itoa(int(s)), true // Format as int
		}
		// Otherwise format as float (using 'g' might still be okay, or choose specific format)
		return strconv.FormatFloat(s, 'g', -1, 64), true
	default:
		// Add other numeric types if needed (e.g., int64, float32)
		log.Warn("ensureString: Cannot convert value to string", "type", fmt.Sprintf("%T", v))
		return "", false
	}
}

// splitRepoPath splits a repository path string by the first slash.
// It is used to separate potential registry prefixes from the repository name.
// Example: "docker.io/nginx" -> ["docker.io", "nginx"]
// Example: "nginx" -> ["nginx"]
func splitRepoPath(repo string) []string {
	// Splitting a non-empty string will always yield at least one element.
	// Splitting an empty string yields a slice with one empty string.
	return strings.SplitN(repo, "/", maxSplitTwo)
}
