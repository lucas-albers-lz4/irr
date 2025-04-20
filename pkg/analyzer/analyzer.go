// Package analyzer provides functionality for analyzing Helm charts
// and identifying container image references within them.
// It supports both structured and unstructured image references,
// with a focus on identifying and classifying patterns.
package analyzer

import (
	"fmt"
	"maps"
	"strings"

	"github.com/gobwas/glob"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/strategy"
)

// ImagePattern represents a detected image string and its location.
type ImagePattern struct {
	Path      string          `json:"path"`                // The path within the final merged values, prefixed with subchart alias if applicable (e.g., "grafana.image")
	RawPath   string          `json:"rawPath"`             // The path within the specific value block where the image was found (e.g., "image" or "sub.image")
	Origin    string          `json:"origin"`              // Origin identifier ("." for parent, subchart alias otherwise)
	Type      string          `json:"type"`                // Type of the value ("string" or "map")
	Value     string          `json:"value"`               // The full image string (e.g., "nginx:latest" or constructed from map)
	Structure *ImageStructure `json:"structure,omitempty"` // Detailed structure if Type is "map"
	Count     int             `json:"count"`               // How many times this exact pattern (Path + Value) was found
	Image     string          `json:"image,omitempty"`     // The full image string (e.g., "nginx:latest")
	KeyName   string          `json:"keyName,omitempty"`   // The key name in the map (e.g., "tag" for "nginx:latest")
	MapValue  interface{}     `json:"mapValue,omitempty"`  // The map value for the image
}

// ImageStructure holds the components of an image when defined as a map.
type ImageStructure struct {
	Registry   string `json:"registry,omitempty"`
	Repository string `json:"repository"`
	Tag        string `json:"tag,omitempty"`
}

// Config holds configuration options for the Analyzer.
// It allows customizing the analysis process through configuration settings.
type Config struct {
	// IncludePatterns are glob patterns for paths to include during analysis
	IncludePatterns []string
	// ExcludePatterns are glob patterns for paths to exclude from analysis
	ExcludePatterns []string
	// KnownPaths are specific dot-notation paths known to contain images
	KnownPaths []string
}

// PatternMatcher wraps a compiled regex for matching paths.
// This is a placeholder; a real implementation would use regexp.Regexp.
type PatternMatcher struct {
	// pattern *regexp.Regexp // Placeholder for actual regex implementation
}

// Match checks if the given path matches the pattern.
// Placeholder implementation.
func (pm *PatternMatcher) Match(_ string) bool {
	// Placeholder logic - replace with actual regex matching
	return true // Default to true for now
}

// AnalyzeHelmValues analyzes Helm values content for image patterns.
// Deprecated: Use AnalyzeChartValues for proper subchart handling.
func AnalyzeHelmValues(values map[string]interface{}, config *Config) ([]ImagePattern, error) {
	log.Warnf("DEPRECATED: AnalyzeHelmValues called. This function does not handle subchart values correctly. Use AnalyzeChartValues instead.")
	patterns := []ImagePattern{}
	// Pass nil origin map for basic compatibility
	analyzeMapRecursive("", values, ".", nil, &patterns, config) // Start recursion with root path "" and origin "."
	aggregatedPatterns := aggregatePatterns(patterns)
	return aggregatedPatterns, nil
}

// AnalyzeChartValues analyzes the fully computed Helm chart values, including subchart defaults,
// using an origin map to correctly prefix paths for overrides.
func AnalyzeChartValues(finalMergedValues map[string]interface{}, originMap map[string]string, config *Config) ([]ImagePattern, error) {
	log.Debugf("Starting Helm chart values analysis with origin tracking")
	patterns := []ImagePattern{}
	// Start recursion with root path "" and origin "."
	analyzeMapRecursive("", finalMergedValues, ".", originMap, &patterns, config)

	// DEBUG: Log raw patterns found before aggregation
	debug.Printf("AnalyzeChartValues: Found %d raw patterns before aggregation:", len(patterns))
	for idx := range patterns { // Use index instead of copying pattern
		p := &patterns[idx]
		debug.Printf("  Raw Pattern %d: Path='%s', RawPath='%s', Origin='%s', Value='%s', Type='%s'",
			idx, p.Path, p.RawPath, p.Origin, p.Value, p.Type)
	}

	// Post-process to aggregate counts for duplicate patterns
	aggregatedPatterns := aggregatePatterns(patterns)

	// DEBUG: Log aggregated patterns before returning
	debug.Printf("AnalyzeChartValues: Returning %d aggregated patterns:", len(aggregatedPatterns))
	for idx := range aggregatedPatterns { // Use index instead of copying pattern
		p := &aggregatedPatterns[idx]
		debug.Printf("  Agg Pattern %d: Path='%s', RawPath='%s', Origin='%s', Value='%s', Type='%s', Count=%d",
			idx, p.Path, p.RawPath, p.Origin, p.Value, p.Type, p.Count)
	}

	log.Infof("Helm chart values analysis complete. Found %d unique image patterns.", len(aggregatedPatterns))
	return aggregatedPatterns, nil
}

// aggregatePatterns merges duplicate ImagePattern entries and sums their counts.
// It now keys based on the fully prefixed Path and the Value.
func aggregatePatterns(patterns []ImagePattern) []ImagePattern {
	patternMap := make(map[string]ImagePattern)
	for i := range patterns { // Use index instead of copying pattern
		p := &patterns[i]
		// Key based on the fully qualified Path and Value for uniqueness
		key := p.Path + ":" + p.Value
		if existing, ok := patternMap[key]; ok {
			existing.Count += p.Count // Assumes Count is always 1 initially
			patternMap[key] = existing
		} else {
			// Ensure initial count is 1 if not already set
			if p.Count == 0 {
				p.Count = 1
			}
			patternMap[key] = *p // Dereference p when assigning
		}
	}

	result := make([]ImagePattern, 0, len(patternMap))
	// Iterate over map keys to avoid copying the pattern struct
	for key := range patternMap {
		result = append(result, patternMap[key])
	}
	return result
}

// processPotentialImageMap checks if a map structure might represent an image and appends a pattern if valid.
// It returns true if the map was processed as an image (so recursion should stop), false otherwise.
func processPotentialImageMap(currentPath string, data map[string]interface{}, currentOrigin string, patterns *[]ImagePattern, config *Config) bool {
	debug.Printf("Checking potential image map at raw path: %s", currentPath)
	// Check for repository first, as it's mandatory
	repoVal, repoOk := data["repository"].(string)
	if !repoOk {
		debug.Printf("No 'repository' string found in map at %s, not an image map.", currentPath)
		return false // Not an image map structure
	}

	// Check for tag (optional but common)
	tagVal, tagOk := data["tag"].(string)
	// Check for registry (optional)
	regVal, regOk := data["registry"].(string)
	// Check for digest (mutually exclusive with tag ideally, but we don't enforce that here)
	digestVal, digestOk := data["digest"].(string) // Assuming digest might be present

	// Consider it an image map if repository is present. Tag or Digest might be optional.
	// Refined Check: Require repository. Tag/Digest are optional but good indicators.
	// Let's proceed if repository is found, and capture tag/digest/registry if they exist.
	log.Debugf("Potential image map found at raw path: %s with repository: %s", currentPath, repoVal)

	// Use currentOrigin passed in, don't look up originMap again here.
	origin := currentOrigin
	finalPath := currentPath
	// Prefix path if origin is known (not root '.')
	if origin != "." && origin != "" { // Also handle empty origin string case
		finalPath = origin + "." + currentPath
	}

	// *** ADD matchPath check HERE, before appending pattern ***
	if !matchPath(currentPath, config) { // Match against the raw path
		log.Debugf("Skipping image map at raw path '%s' due to include/exclude patterns.", currentPath)
		return true // Treat as processed (skipped) to prevent recursion
	}

	// Construct the map value representation string and structure
	var mapValueParts []string
	imageStruct := ImageStructure{Repository: repoVal}

	mapValueParts = append(mapValueParts, fmt.Sprintf("repository=%s", repoVal))
	if regOk && regVal != "" {
		mapValueParts = append(mapValueParts, fmt.Sprintf("registry=%s", regVal))
		imageStruct.Registry = regVal
	}
	if tagOk && tagVal != "" {
		// ---> START Digest-in-Tag Handling <---
		processedTag := tagVal
		if strings.Contains(tagVal, "@") {
			log.Warnf("Found '@' symbol within the 'tag' field ('%s') at path '%s'. This is unusual. Attempting to extract tag part.", tagVal, currentPath)
			parts := strings.SplitN(tagVal, "@", strategy.MaxSplitTwo)
			if len(parts) == strategy.MaxSplitTwo { // Ensure the split resulted in exactly two parts
				// Correctly use parts[0] as the tag and parts[1] as the annotation/digest part if needed
				processedTag = strings.TrimSpace(parts[0]) // Assign the part BEFORE "@" to processedTag
				annotation := strings.TrimSpace(parts[1])  // Assign the part AFTER "@" to a potential annotation variable (optional)
				log.Debugf("Extracted tag: '%s' and potential annotation/digest: '%s'", processedTag, annotation)
				// Depending on logic, you might want to use the 'annotation' part (e.g., as a digest)
			}
		}
		// Use the potentially modified tag
		if processedTag != "" { // Add check if processedTag is empty after split
			mapValueParts = append(mapValueParts, fmt.Sprintf("tag=%s", processedTag))
			imageStruct.Tag = processedTag
		}
		// ---> END Digest-in-Tag Handling <---
	}
	if digestOk && digestVal != "" {
		mapValueParts = append(mapValueParts, fmt.Sprintf("digest=%s", digestVal))
		// Potentially add digest to ImageStructure if needed later
	}
	mapValueStr := strings.Join(mapValueParts, ",")

	log.Debugf("Confirmed image map structure at path: %s (Origin: %s, Final Path: %s)", currentPath, origin, finalPath)
	*patterns = append(*patterns, ImagePattern{
		Path:      finalPath,   // Path including origin prefix
		RawPath:   currentPath, // Path without origin prefix
		Origin:    origin,
		Value:     mapValueStr,  // Represent map as string
		Type:      "map",        // Use string literal "map"
		Structure: &imageStruct, // Store captured structure (incl. registry, tag)
		Count:     1,            // Initialize count
	})
	return true // Indicate that this map was processed as an image structure
}

// analyzeMapRecursive analyzes map key-value pairs.
// It now uses the helper function to check for image map structures.
func analyzeMapRecursive(currentPath string, data map[string]interface{}, currentOrigin string, originMap map[string]string, patterns *[]ImagePattern, config *Config) {
	debug.FunctionEnter("analyzeMapRecursive")
	defer debug.FunctionExit("analyzeMapRecursive")
	debug.Printf("Analyzing map at path: %s, Origin: %s, Data keys: %v", currentPath, currentOrigin, maps.Keys(data)) // Use maps.Keys

	// Check if the current map itself represents an image structure
	if processPotentialImageMap(currentPath, data, currentOrigin, patterns, config) {
		debug.Printf("Map at %s processed as image structure, stopping recursion for this branch.", currentPath)
		return // Stop recursion if this map was treated as an image structure
	}

	// Recursively analyze nested maps and slices if not processed as an image map
	for key, value := range data {
		// Construct the path for the nested element
		newRawPath := key
		if currentPath != "" {
			newRawPath = currentPath + "." + key
		}

		// Determine origin for the child element
		// Default: inherit parent origin
		childOrigin := currentOrigin
		// If we are currently in the parent chart context (origin == "."),
		// check if the current key corresponds to a subchart alias.
		if currentOrigin == "." && originMap != nil {
			if mappedOrigin, ok := originMap[key]; ok {
				// Found an origin mapping for this top-level key, use it.
				log.Debugf("Origin map lookup for top-level key '%s' yielded origin '%s'", key, mappedOrigin)
				childOrigin = mappedOrigin
			} else {
				// Key not found in originMap, so it belongs to the parent chart.
				// childOrigin correctly remains "."
				log.Debugf("Top-level key '%s' not found in origin map, assuming origin '.' (parent)", key)
			}
		} else {
			// If currentOrigin is already set (e.g., "grafana"), children simply inherit it.
			// childOrigin already holds currentOrigin, so no action needed.
			log.Debugf("Inheriting non-root origin '%s' for key '%s'", currentOrigin, key)
		}

		log.Debugf("Recursing into key '%s' with path '%s' and determined childOrigin '%s'", key, newRawPath, childOrigin)

		// No matchPath check needed before recursion itself
		analyzeValueRecursive(newRawPath, value, childOrigin, originMap, patterns, config)
	}
}

// analyzeSliceRecursive analyzes slice elements.
func analyzeSliceRecursive(currentPath string, data []interface{}, currentOrigin string, originMap map[string]string, patterns *[]ImagePattern, config *Config) {
	debug.FunctionEnter("analyzeSliceRecursive")
	defer debug.FunctionExit("analyzeSliceRecursive")
	debug.Printf("Analyzing slice at path: %s, Origin: %s, Length: %d", currentPath, currentOrigin, len(data))

	// We generally don't expect a direct slice element to be *the* image map itself.
	// Helm values usually have slices containing maps or strings.
	// Example: containers: [ { name: "nginx", image: "nginx:latest" }, ... ]
	// Example: imagePullSecrets: [ name: "secret1" ]
	// The previous logic checking data[0] seemed potentially flawed or specific to an edge case.
	// If a slice element *is* a map that represents an image, the recursion below handles it.

	// Recursively analyze items within the slice
	for i, item := range data {
		// Construct path for the slice element
		elementPath := fmt.Sprintf("%s[%d]", currentPath, i)
		debug.Printf("Recursing into slice element %d at path: %s", i, elementPath)
		// Children inherit the parent's origin directly. Origin mapping applies to map keys (subchart names).
		// No matchPath check needed before recursion itself
		analyzeValueRecursive(elementPath, item, currentOrigin, originMap, patterns, config)
	}
}

// analyzeStringValue analyzes a string value to see if it looks like an image reference.
// It now includes a check for Go template syntax.
func analyzeStringValue(currentPath, value, currentOrigin string, _ map[string]string, patterns *[]ImagePattern, config *Config) {
	debug.FunctionEnter("analyzeStringValue")
	defer debug.FunctionExit("analyzeStringValue")

	if value == "" {
		debug.Printf("Skipping empty string at path '%s'", currentPath)
		return
	}

	// ---> START REVISED CHANGE: Construct finalPath using passed currentOrigin <-----
	origin := currentOrigin // Trust the origin passed down from the caller
	finalPath := currentPath
	if origin != "." && origin != "" {
		finalPath = origin + "." + currentPath
		log.Debugf("analyzeStringValue: Applying origin prefix '%s' from caller to path '%s', final path: '%s'", origin, currentPath, finalPath)
	}
	// ---> END REVISED CHANGE <-----

	// Perform the path matching check using the *raw* path (currentPath)
	if !matchPath(currentPath, config) {
		log.Debugf("Skipping string value at path '%s' due to include/exclude patterns.", currentPath)
		return
	}

	// ---> START Template Detection <---
	// Check for Go template syntax before attempting to parse as an image
	if strings.Contains(value, "{{") && strings.Contains(value, "}}") {
		log.Debugf("Detected template syntax in string value at path: %s", currentPath)
		*patterns = append(*patterns, ImagePattern{
			Path:    finalPath,
			RawPath: currentPath,
			Origin:  currentOrigin,
			Value:   value,      // Store the raw template string
			Type:    "template", // Mark as template type
			Count:   1,
		})
		return // Don't try to parse templates as regular images
	}
	// ---> END Template Detection <---

	// Attempt to parse the string value as a standard image reference
	// Note: We don't use the parsed ref directly here, just check if it *can* be parsed.
	// The generator will re-parse it later.
	ref, err := image.ParseImageReference(value)
	if err != nil {
		// If parsing fails, it's likely not a valid image reference string.
		log.Debugf("String value at path '%s' ('%s') failed initial parsing: %v", currentPath, value, err)
		return
	}

	// ---> START Stricter Check <---
	// Additionally, check if the original string contained typical separators OR
	// if the parsed reference got a non-default tag or a digest.
	isLikelyImage := strings.Contains(value, ":") || strings.Contains(value, "@") || (ref != nil && (ref.Tag != image.LatestTag || ref.Digest != ""))
	if !isLikelyImage {
		log.Debugf("String value at path '%s' ('%s') parsed but doesn't look like a typical image ref. Skipping.", currentPath, value)
		return
	}
	// ---> END Stricter Check <---

	// If parsing succeeds and it looks like an image, record it.
	log.Debugf("Found potential image string at path: %s, Value: %s", currentPath, value)
	*patterns = append(*patterns, ImagePattern{
		Path:      finalPath,
		RawPath:   currentPath,
		Origin:    currentOrigin,
		Value:     value,
		Type:      "string",
		Structure: nil,
		Count:     1,
	})
	debug.Printf("Found string image pattern: Path='%s', RawPath='%s', Origin='%s', Value='%s'",
		finalPath, currentPath, origin, value)
}

// analyzeValueRecursive dispatches analysis based on the value type.
// It now correctly passes down the currentOrigin.
func analyzeValueRecursive(currentPath string, value interface{}, currentOrigin string, originMap map[string]string, patterns *[]ImagePattern, config *Config) {
	// No matchPath check needed here, it's handled within the specific type handlers
	switch v := value.(type) {
	case map[string]interface{}:
		analyzeMapRecursive(currentPath, v, currentOrigin, originMap, patterns, config)
	case []interface{}:
		analyzeSliceRecursive(currentPath, v, currentOrigin, originMap, patterns, config)
	case string:
		analyzeStringValue(currentPath, v, currentOrigin, originMap, patterns, config)
		// default:
		// Other types (int, bool, etc.) are ignored
	}
}

// matchPath checks if the given path should be included based on Include/Exclude patterns.
// It checks the raw path (without origin prefix).
func matchPath(rawPath string, config *Config) bool {
	// Add nil check before accessing config fields in debug log
	var includePatterns, excludePatterns []string
	if config != nil {
		includePatterns = config.IncludePatterns
		excludePatterns = config.ExcludePatterns
	}
	debug.Printf("matchPath: Checking path '%s' against config: Includes=%v, Excludes=%v",
		rawPath, includePatterns, excludePatterns)

	if config == nil {
		debug.Printf("matchPath: No config, including path '%s'", rawPath)
		return true // No config means include everything
	}

	// Check excludes first. If excluded, return false immediately.
	if len(excludePatterns) > 0 {
		if matchAny(rawPath, excludePatterns) {
			debug.Printf("matchPath: Path '%s' EXCLUDED by patterns %v", rawPath, excludePatterns)
			return false
		}
	}

	// If include patterns are defined, the path MUST match at least one.
	if len(includePatterns) > 0 {
		if !matchAny(rawPath, includePatterns) {
			debug.Printf("matchPath: Path '%s' NOT INCLUDED by patterns %v", rawPath, includePatterns)
			return false
		}
	}

	// If we reach here, it was not excluded, and if includes were specified, it matched them.
	debug.Printf("matchPath: Path '%s' INCLUDED (passed filters)", rawPath)
	return true
}

// matchAny checks if the path matches any of the glob patterns using gobwas/glob.
func matchAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Compile the glob pattern. Handle potential errors.
		g, err := glob.Compile(pattern, '.') // Use '.' as the separator
		if err != nil {
			log.Warnf("Invalid glob pattern '%s': %v. Skipping this pattern.", pattern, err)
			continue // Skip invalid patterns
		}
		// Check if the path matches the compiled glob
		if g.Match(path) {
			log.Debugf("Path '%s' matched glob pattern '%s'", path, pattern)
			return true
		}
	}
	return false
}

// Additional helper functions or types related to analysis can be defined below.
// For example, pattern matching logic implementation.

// Consider adding functions to load/compile regex patterns for Include/Exclude config.
