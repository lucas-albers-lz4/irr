// Package analyzer provides functionality for analyzing Helm charts
// and identifying container image references within them.
// It supports both structured and unstructured image references,
// with a focus on identifying and classifying patterns.
package analyzer

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/log"
)

// ChartDependency defines the relevant fields from a chart dependency for alias mapping.
// Using a local struct avoids direct dependency on helm/chart packages here.
type ChartDependency struct {
	Name  string
	Alias string
}

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

// Analyzer holds configuration and state for the analysis process.
type Analyzer struct {
	config   *Config
	aliasMap map[string]string // Maps chart name to alias
}

// NewAnalyzer creates a new Analyzer instance.
func NewAnalyzer(config *Config, dependencies []*ChartDependency) *Analyzer {
	aliasMap := make(map[string]string)
	for _, dep := range dependencies {
		if dep.Alias != "" {
			aliasMap[dep.Name] = dep.Alias
			log.Debug("Analyzer: Registered alias", "name", dep.Name, "alias", dep.Alias)
		}
	}
	return &Analyzer{
		config:   config,
		aliasMap: aliasMap,
	}
}

// Analyze analyzes Helm values content for image patterns using the Analyzer's configuration.
func (a *Analyzer) Analyze(values map[string]interface{}) ([]ImagePattern, error) {
	log.Debug("Starting Helm values analysis with Analyzer")
	rawPatterns := []ImagePattern{}
	a.analyzeValuesRecursive("", values, &rawPatterns) // Start recursion with root path ""

	// --- START Filtering Logic ---
	filteredPatterns := []ImagePattern{}
	if a.config != nil && (len(a.config.IncludePatterns) > 0 || len(a.config.ExcludePatterns) > 0) {
		for _, p := range rawPatterns {
			path := p.Path
			// Check excludes first
			if len(a.config.ExcludePatterns) > 0 && matchAny(path, a.config.ExcludePatterns) {
				log.Debug("Filtering out excluded path: %s", path)
				continue // Skip this pattern
			}
			// Check includes (only if not excluded)
			if len(a.config.IncludePatterns) > 0 && !matchAny(path, a.config.IncludePatterns) {
				log.Debug("Filtering out path not matching includes: %s", path)
				continue // Skip this pattern
			}
			// If not excluded and matches includes (or no includes specified), keep it
			filteredPatterns = append(filteredPatterns, p)
		}
	} else {
		// No filtering needed
		filteredPatterns = rawPatterns
	}
	// --- END Filtering Logic ---

	// Post-process to aggregate counts for duplicate patterns
	aggregatedPatterns := aggregatePatterns(filteredPatterns)

	log.Info("Helm values analysis complete. Found %d unique image patterns.", len(aggregatedPatterns))
	return aggregatedPatterns, nil
}

// AnalyzeHelmValues remains as a convenience function for backward compatibility
// or simple use cases without alias handling.
func AnalyzeHelmValues(values map[string]interface{}, config *Config) ([]ImagePattern, error) {
	// Create a default analyzer with no dependencies/aliases
	analyzer := NewAnalyzer(config, nil)
	return analyzer.Analyze(values)
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
func (a *Analyzer) analyzeValuesRecursive(path string, value interface{}, patterns *[]ImagePattern) {
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
		a.analyzeMapValue(path, val, patterns)
	case reflect.Slice, reflect.Array:
		a.analyzeSliceValue(path, val, patterns)
	case reflect.String:
		a.analyzeStringValue(path, val, patterns)
	case reflect.Interface:
		a.analyzeInterfaceValue(path, val, patterns)
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		log.Debug("Ignoring scalar value of type %s at path '%s'", val.Kind(), path)
	default:
		log.Warn("Ignoring value with unhandled type '%s' at path '%s'. Value: %v", val.Kind(), path, value)
	}
}

// analyzeMapValue handles the analysis logic for map values.
// It first checks if the map represents a structured image definition
// (containing at least a 'repository' key). If it is, it records the pattern
// and stops recursion for that branch. If not, it recursively calls
// analyzeValuesRecursive for each key-value pair within the map.
func (a *Analyzer) analyzeMapValue(path string, val reflect.Value, patterns *[]ImagePattern) {
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
			a.analyzeValuesRecursive(newPath, v.Interface(), patterns)
		}
		return // Exit after fallback iteration
	}

	// Attempt to handle as a structured image map
	isHandled := a.handleImageMap(path, mapValue, patterns)
	if isHandled {
		return // Stop recursion if handled as an image map
	}

	// If it's not a structured image map, recurse into its values
	log.Debug("Traversing map children at path '%s'", path)
	for key, entryValue := range mapValue {
		// --- START Alias Handling ---
		pathSegment := key
		if alias, aliasExists := a.aliasMap[key]; aliasExists {
			log.Debug("Applying alias for path segment", "originalKey", key, "alias", alias)
			pathSegment = alias // Use the alias for the path construction
		}
		// --- END Alias Handling ---

		newPath := pathSegment // Use the potentially aliased segment
		if path != "" {
			newPath = path + "." + pathSegment
		}
		a.analyzeValuesRecursive(newPath, entryValue, patterns)
	}
}

// handleImageMap checks if a map represents a structured image and adds a pattern if so.
// Returns true if the map was handled as an image, false otherwise.
func (a *Analyzer) handleImageMap(path string, mapValue map[string]interface{}, patterns *[]ImagePattern) bool {
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

	if !isImageMap {
		return false // Not a structured image map
	}

	// Construct the image string using extracted components (including the actual tag)
	constructedRefStr := constructImageString(registry, repository, tag)
	if constructedRefStr == "" {
		log.Warn("Could not construct valid image string from map at path '%s'", path)
		return false // Treat as non-image map if construction fails
	}

	// Attempt to parse the constructed string to get a canonical representation
	ref, err := image.ParseImageReference(constructedRefStr)
	if err != nil {
		log.Warn("Failed to parse constructed image string from map, potential non-image?", "path", path, "value", constructedRefStr, "error", err)
		return false // Treat as non-image map if parsing fails
	}

	// Canonicalize the reference
	image.NormalizeImageReference(ref)
	canonicalValue := ref.String()

	// Apply alias handling to the pattern path
	finalPatternPath := a.applyAliasToPath(path)

	log.Debug("Found image map at path '%s'. Value: '%s'", finalPatternPath, constructedRefStr)

	// Add the detected image pattern with the potentially aliased path
	*patterns = append(*patterns, ImagePattern{
		Path:  finalPatternPath,
		Type:  "map",
		Value: canonicalValue,
		Structure: &ImageStructure{
			Registry:   registry,
			Repository: repository,
			Tag:        tag,
		},
		Count: 1,
	})
	return true // Successfully handled as an image map
}

// applyAliasToPath applies alias mapping to a given dot-notation path.
func (a *Analyzer) applyAliasToPath(path string) string {
	finalPath := path
	if lastSegmentIndex := strings.LastIndex(path, "."); lastSegmentIndex != -1 {
		parentPath := path[:lastSegmentIndex]
		lastKey := path[lastSegmentIndex+1:]
		if alias, aliasExists := a.aliasMap[lastKey]; aliasExists {
			finalPath = parentPath + "." + alias
			log.Debug("Applying alias to pattern path", "originalPath", path, "finalPath", finalPath)
		}
	} else {
		// Handle top-level keys that might be aliases
		if alias, aliasExists := a.aliasMap[path]; aliasExists {
			finalPath = alias
			log.Debug("Applying alias to top-level pattern path", "originalPath", path, "finalPath", finalPath)
		}
	}
	return finalPath
}

// analyzeSliceValue handles the analysis logic for slice and array values.
func (a *Analyzer) analyzeSliceValue(path string, val reflect.Value, patterns *[]ImagePattern) {
	log.Debug("Traversing slice/array at path '%s' (Length: %d)", path, val.Len())
	for i := 0; i < val.Len(); i++ {
		// Generate path with index, e.g., "ports[0]"
		elemPath := fmt.Sprintf("%s[%d]", path, i)
		a.analyzeValuesRecursive(elemPath, val.Index(i).Interface(), patterns)
	}
}

// analyzeStringValue handles the analysis logic for string values.
// It attempts to parse the string as an image reference and records it if successful.
func (a *Analyzer) analyzeStringValue(path string, val reflect.Value, patterns *[]ImagePattern) {
	strValue := val.String()
	log.Debug("Analyzing string value at path '%s': '%s'", path, strValue)

	// Heuristic: Skip strings that are likely part of an already processed map structure
	// This relies on analyzeMapValue returning early. Check common sub-field names.
	pathLower := strings.ToLower(path)
	if strings.HasSuffix(pathLower, ".repository") ||
		strings.HasSuffix(pathLower, ".tag") ||
		strings.HasSuffix(pathLower, ".registry") ||
		strings.HasSuffix(pathLower, ".digest") {
		log.Debug("Skipping likely sub-field string at path '%s'", path)
		return
	}

	// Heuristic: Skip common non-image keys, case-insensitive
	// Extend this list as needed based on common Helm chart patterns
	baseKey := path
	if idx := strings.LastIndex(path, "."); idx != -1 {
		baseKey = path[idx+1:]
	}
	baseKeyLower := strings.ToLower(baseKey)
	commonNonImageKeys := []string{
		"name", "fullname", "namespace", "version", "appversion", "description",
		"type", "kind", "apiversion", "enabled", "disabled", "serviceaccount",
		"secretname", "configmapname", "hostname", "port", "protocol", "url",
		"username", "password", "key", "value", "label", "annotation", "pullpolicy", // Also check pullPolicy here
		"storageclass", "accessmode", "size", "path", "command", "args",
		// Add more as identified
	}
	for _, key := range commonNonImageKeys {
		if baseKeyLower == key {
			log.Debug("Skipping common non-image key string at path '%s'", path)
			return
		}
	}

	// Attempt to parse the string value as an image reference
	ref, err := image.ParseImageReference(strValue)
	if err != nil {
		// Check if it's a potentially templated value
		if strings.Contains(strValue, "{{") || strings.Contains(strValue, "}}") {
			log.Debug("Ignoring likely templated string value at path '%s': '%s'", path, strValue)
		} else {
			log.Debug("String value at path '%s' is not a valid image reference: '%s'. Error: %v", path, strValue, err)
		}
		return // Not a valid image reference
	}

	// Canonicalize the reference
	image.NormalizeImageReference(ref)
	canonicalValue := ref.String() // Use the canonical string representation
	log.Debug("Found potential image string at path '%s'. Original: '%s', Canonical: '%s'", path, strValue, canonicalValue)

	// Add the detected image pattern
	*patterns = append(*patterns, ImagePattern{
		Path:  path,
		Type:  "string",
		Value: canonicalValue, // Use canonical value
		Count: 1,
	})
}

// analyzeInterfaceValue handles values of type interface{}.
// It retrieves the underlying concrete value and calls analyzeValuesRecursive.
func (a *Analyzer) analyzeInterfaceValue(path string, val reflect.Value, patterns *[]ImagePattern) {
	// Check if the interface value itself is valid and non-nil before proceeding
	if !val.IsValid() {
		log.Debug("Ignoring invalid interface{} at path '%s'", path)
		return
	}

	// Use IsNil() only for kinds where it's valid
	canCheckNil := false
	switch val.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		canCheckNil = true
	}

	if canCheckNil && val.IsNil() {
		log.Debug("Ignoring nil interface{} value at path '%s'", path)
		return
	}

	// Get the concrete value contained within the interface
	innerValue := val.Interface()
	if innerValue == nil { // Check if the concrete value itself is nil
		log.Debug("Ignoring interface{} containing a nil concrete value at path '%s'", path)
		return
	}

	innerReflectValue := reflect.ValueOf(innerValue)
	kind := innerReflectValue.Kind()

	// Only recurse if the underlying type is a map, slice/array, or string
	if kind == reflect.Map || kind == reflect.Slice || kind == reflect.Array || kind == reflect.String {
		log.Debug("Recursing into interface{} holding %v at path '%s'", kind, path)
		a.analyzeValuesRecursive(path, innerValue, patterns) // Recurse with the unwrapped value
	} else {
		log.Debug("Ignoring non-map/slice/string value within interface{} at path '%s'. Type: %T", path, innerValue)
	}
}

// constructImageString attempts to build a valid image string from map components.
// Returns an empty string if repository is missing. Applies default tag if needed.
func constructImageString(registry, repository, tag string) string {
	if repository == "" {
		return "" // Repository is mandatory
	}
	if tag == "" {
		tag = image.DefaultTag // Apply default tag if missing
	}
	if registry != "" {
		return fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	}
	return fmt.Sprintf("%s:%s", repository, tag)
	// Note: This doesn't handle digests explicitly, relying on ParseImageReference later if needed.
}

// matchAny checks if the path matches any of the glob patterns.
// Supports '*' for a single segment and '**' for any number of segments in dot-notation paths.
func matchAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchDotPattern(path, pattern) {
			return true
		}
	}
	return false
}

// matchDotPattern matches dot-separated paths with '*' and '**' support.
func matchDotPattern(path, pattern string) bool {
	pathSegs := splitDotPath(path)
	patSegs := splitDotPath(pattern)
	return matchSegments(pathSegs, patSegs)
}

func splitDotPath(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ".")
}

func matchSegments(pathSegs, patSegs []string) bool {
	for len(patSegs) > 0 {
		if len(pathSegs) == 0 && patSegs[0] != "**" {
			return false
		}
		switch patSegs[0] {
		case "**":
			// '**' matches any number of segments (including zero)
			if len(patSegs) == 1 {
				return true // '**' at end matches all
			}
			// Try to match the rest of the pattern at every possible position
			for i := 0; i <= len(pathSegs); i++ {
				if matchSegments(pathSegs[i:], patSegs[1:]) {
					return true
				}
			}
			return false
		case "*":
			// '*' matches exactly one segment
			pathSegs = pathSegs[1:]
			patSegs = patSegs[1:]
		default:
			// Add this check: if path is exhausted but pattern is not, no match.
			if len(pathSegs) == 0 {
				return false
			}
			// Now safe to access pathSegs[0]
			if pathSegs[0] != patSegs[0] {
				return false
			}
			pathSegs = pathSegs[1:]
			patSegs = patSegs[1:]
		}
	}
	return len(pathSegs) == 0 && len(patSegs) == 0
}
