// Package analyzer provides functionality for analyzing Helm charts
// and identifying container image references within them.
// It supports both structured and unstructured image references,
// with a focus on identifying and classifying patterns.
package analyzer

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/log"
)

// ImagePattern represents a detected image string and its location.
type ImagePattern struct {
	Path      string          `json:"path"`                // The path within the values structure where the image was found (e.g., "service.image.repository")
	Type      string          `json:"type"`                // Type of the value ("string" or "map")
	Value     string          `json:"value"`               // The full image string (e.g., "nginx:latest" or constructed from map)
	Structure *ImageStructure `json:"structure,omitempty"` // Detailed structure if Type is "map"
	Count     int             `json:"count"`               // How many times this exact pattern was found
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
func AnalyzeHelmValues(values map[string]interface{}, config *Config) ([]ImagePattern, error) {
	log.Debug("Starting Helm values analysis")
	patterns := []ImagePattern{}
	analyzeValuesRecursive("", values, &patterns, config) // Start recursion with root path ""

	// Post-process to aggregate counts for duplicate patterns
	aggregatedPatterns := aggregatePatterns(patterns)

	log.Info("Helm values analysis complete. Found %d unique image patterns.", len(aggregatedPatterns))
	return aggregatedPatterns, nil
}

// aggregatePatterns merges duplicate ImagePattern entries and sums their counts.
func aggregatePatterns(patterns []ImagePattern) []ImagePattern {
	patternMap := make(map[string]ImagePattern)
	for _, p := range patterns {
		// Key based on Path and Value for uniqueness
		key := p.Path + ":" + p.Value
		if existing, ok := patternMap[key]; ok {
			existing.Count += p.Count
			patternMap[key] = existing
		} else {
			patternMap[key] = p
		}
	}

	result := make([]ImagePattern, 0, len(patternMap))
	for _, p := range patternMap {
		result = append(result, p)
	}
	return result
}

// analyzeValuesRecursive performs a deep traversal of the values structure.
func analyzeValuesRecursive(path string, value interface{}, patterns *[]ImagePattern, config *Config) {
	// Handle nil values gracefully
	if value == nil {
		log.Debug("Skipping nil value at path '%s'", path)
		return
	}

	val := reflect.ValueOf(value)

	// Handle pointers by dereferencing
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			log.Debug("Skipping nil pointer at path '%s'", path)
			return
		}
		val = val.Elem() // Dereference the pointer
	}

	// Call the appropriate handler based on the kind
	switch val.Kind() {
	case reflect.Map:
		analyzeMapValue(path, val, patterns, config)
	case reflect.Slice, reflect.Array:
		analyzeSliceValue(path, val, patterns, config)
	case reflect.String:
		analyzeStringValue(path, val, patterns, config)
	case reflect.Interface:
		analyzeInterfaceValue(path, val, patterns, config)
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		log.Debug("Ignoring scalar value of type %s at path '%s'", val.Kind())
	default:
		log.Warn("Ignoring value with unhandled type '%s' at path '%s'. Value: %v", val.Kind(), path, value)
	}
}

// analyzeMapValue handles the analysis logic for map values.
func analyzeMapValue(path string, val reflect.Value, patterns *[]ImagePattern, config *Config) {
	// Check if the map key type is string, required for Helm values traversal.
	if val.Type().Key().Kind() != reflect.String {
		log.Warn("Skipping map with non-string keys at path '%s'. Key type: %s", path, val.Type().Key().Kind())
		return
	}

	// Assert that the value is map[string]interface{} to access keys safely
	mapValue, ok := val.Interface().(map[string]interface{})
	if !ok {
		// Fallback iteration for potentially different map types if direct assertion fails
		log.Warn("Skipping map at path '%s' due to unexpected map type: %T. Attempting iteration.", path, val.Interface())
		iter := val.MapRange()
		for iter.Next() {
			k := iter.Key()
			v := iter.Value()
			keyStr, keyOk := k.Interface().(string) // Ensure key is string
			if !keyOk {
				log.Warn("Skipping non-string key in map at path %s: %v", path, k)
				continue
			}
			newPath := keyStr
			if path != "" {
				newPath = path + "." + keyStr
			}
			analyzeValuesRecursive(newPath, v.Interface(), patterns, config)
		}
		return // Exit after fallback iteration
	}

	// Check if it's a structured image map (registry/repository/tag)
	isImageMap := false
	var registry, repository, tag string

	if repoVal, repoOk := mapValue["repository"]; repoOk {
		if repoStr, ok := repoVal.(string); ok && repoStr != "" {
			isImageMap = true
			repository = repoStr
			log.Debug("Found 'repository' key at '%s': '%s'", path, repository)

			// Handle optional registry and tag. DO NOT apply defaults here.
			registry = ""
			if regVal, regOk := mapValue["registry"]; regOk {
				if regStr, ok := regVal.(string); ok && regStr != "" {
					registry = regStr
				}
			}
			tag = ""
			if tagVal, tagOk := mapValue["tag"]; tagOk {
				if tagStr, ok := tagVal.(string); ok && tagStr != "" {
					tag = tagStr
				}
			}
		}
	}

	if isImageMap {
		// Construct a simple representation of the map content for the Value field.
		mapValueStr := fmt.Sprintf("repository=%s", repository)
		if registry != "" {
			mapValueStr += fmt.Sprintf(",registry=%s", registry)
		}
		if tag != "" {
			mapValueStr += fmt.Sprintf(",tag=%s", tag)
		}
		log.Debug("Found image map at path '%s'. Content: '%s'", path, mapValueStr)

		// Add the detected image pattern
		*patterns = append(*patterns, ImagePattern{
			Path:  path,
			Type:  "map",
			Value: mapValueStr,
			Structure: &ImageStructure{
				Registry:   registry,
				Repository: repository,
				Tag:        tag,
			},
			Count: 1,
		})
		log.Debug("Stopping recursion at image map structure: '%s'", path)
	} else {
		// If not an image map, traverse its children
		log.Debug("Traversing map children at path '%s'", path)
		for key, entryValue := range mapValue {
			newPath := key
			if path != "" {
				newPath = path + "." + key
			}
			analyzeValuesRecursive(newPath, entryValue, patterns, config)
		}
	}
}

// analyzeSliceValue handles the analysis logic for slice and array values.
func analyzeSliceValue(path string, val reflect.Value, patterns *[]ImagePattern, config *Config) {
	log.Debug("Traversing slice/array at path '%s' (Length: %d)", path, val.Len())
	for i := 0; i < val.Len(); i++ {
		// Generate path with index, e.g., "ports[0]"
		elemPath := fmt.Sprintf("%s[%d]", path, i)
		analyzeValuesRecursive(elemPath, val.Index(i).Interface(), patterns, config)
	}
}

// analyzeStringValue handles the analysis logic for string values.
func analyzeStringValue(path string, val reflect.Value, patterns *[]ImagePattern, config *Config) {
	strValue := val.String()
	log.Debug("Analyzing string at path '%s'. Value: '%s'", path, strValue)

	// Basic check: Ignore empty strings
	if strValue == "" {
		log.Debug("Ignoring empty string at path '%s'", path)
		return
	}

	// Check if the path matches known image path patterns or suffixes
	keys := strings.Split(path, ".")
	lastKey := ""
	if len(keys) > 0 {
		// Get the last segment, handling array indices like "ports[0]" -> "ports"
		lastKeyPart := keys[len(keys)-1]
		if idx := strings.Index(lastKeyPart, "["); idx != -1 {
			lastKey = lastKeyPart[:idx]
		} else {
			lastKey = lastKeyPart
		}
	}
	isImagePathHeuristic := lastKey == "image" ||
		strings.HasSuffix(lastKey, "Image") ||
		lastKey == "repository"

	// Check if it looks like a Go template
	isTemplate := strings.Contains(strValue, "{{") && strings.Contains(strValue, "}}")

	// Check explicit include/exclude patterns
	isIncluded := config == nil || config.IncludePatterns == nil || len(config.IncludePatterns) == 0 || matchAny(path, config.IncludePatterns)
	isExcluded := config != nil && config.ExcludePatterns != nil && len(config.ExcludePatterns) > 0 && matchAny(path, config.ExcludePatterns)

	log.Debug("String Check - Path: '%s', isImagePathHeuristic: %t, isTemplate: %t, isIncluded: %t, isExcluded: %t", path, isImagePathHeuristic, isTemplate, isIncluded, isExcluded)

	if isImagePathHeuristic && !isTemplate && isIncluded && !isExcluded {
		// We need to check if the string value itself is a valid image reference
		// before considering it for pattern detection.
		// Use non-strict parsing here as we just want to know if it *looks* like an image.
		if _, err := image.ParseImageReference(strValue); err == nil {
			// Valid image string format, but standalone (not in a map)
			// This might be an image string that needs overriding.
			log.Debug("Analyzer: Found potential standalone image string at path %s: %s", path, strValue)
			*patterns = append(*patterns, ImagePattern{Path: path, Type: "string", Value: strValue, Count: 1})
		} else {
			log.Debug("String at path '%s' ('%s') did not pass image reference format validation.", path, strValue)
		}
	} else {
		log.Debug("String at path '%s' ('%s') did not qualify as image pattern (PathMatch=%t, IsTemplate=%t, Included=%t, Excluded=%t)", path, strValue, isImagePathHeuristic, isTemplate, isIncluded, isExcluded)
	}
}

// analyzeInterfaceValue handles the analysis logic for interface values.
func analyzeInterfaceValue(path string, val reflect.Value, patterns *[]ImagePattern, config *Config) {
	if val.IsValid() && !val.IsNil() {
		innerValue := val.Interface()
		innerReflectValue := reflect.ValueOf(innerValue)
		// Only recurse if the underlying type is a map, slice/array, or string
		kind := innerReflectValue.Kind()
		if kind == reflect.Map || kind == reflect.Slice || kind == reflect.Array || kind == reflect.String {
			log.Debug("Recursing into interface{} holding %v at path '%s'", kind, path)
			analyzeValuesRecursive(path, innerValue, patterns, config) // Recurse with the unwrapped value
		} else {
			log.Debug("Ignoring non-map/slice/string value within interface{} at path '%s'. Type: %T", path, innerValue)
		}
	} else {
		log.Debug("Ignoring nil or invalid interface at path '%s'", path)
	}
}

// Helper function (placeholder) to check if a string looks like an image reference
// In a real scenario, this would involve more robust parsing/validation.
// func looksLikeImageReference(s string) bool {
// 	// Basic check: Contains a colon (potentially separating image name and tag)
// 	// or a slash (potentially separating registry/namespace and image name).
// 	// This is a very loose heuristic.
// 	return strings.Contains(s, ":") || strings.Contains(s, "/")
// }

// Additional helper functions or types related to analysis can be defined below.
// For example, pattern matching logic implementation.

// Consider adding functions to load/compile regex patterns for Include/Exclude config.

// matchAny checks if a path matches any of the provided patterns.
// It uses simple glob matching with path.Match.
func matchAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		match, err := filepath.Match(pattern, path)
		// If there's an error with the pattern, consider it non-matching and log the issue
		if err != nil {
			log.Warn("Invalid glob pattern '%s': %v", pattern, err)
			continue
		}
		if match {
			return true
		}
	}
	return false
}
