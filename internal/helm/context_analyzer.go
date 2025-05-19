// Package helm provides internal utilities for interacting with Helm.
package helm

import (
	"fmt"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/log"
)

const (
	// DefaultRegistry is the standard Docker Hub registry
	DefaultRegistry = "docker.io"
	// defaultImageTag is the default tag used when none is specified.
	defaultImageTag = "latest"
	// MaxSplitParts defines the maximum number of parts to split registry/repo paths into.
	MaxSplitParts = 2
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

	// Log top-level keys of the merged values
	topLevelKeys := []string{}
	for k := range a.context.Values {
		topLevelKeys = append(topLevelKeys, k)
	}
	log.Debug("AnalyzeContext: Top-level keys in merged values", "keys", topLevelKeys)

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
		originPath := "values.yaml" // Default origin file
		if origin, exists := a.context.Origins[currentPath]; exists {
			// Use origin.Path if it's a file path, otherwise keep default
			if strings.HasSuffix(origin.Path, ".yaml") || strings.HasSuffix(origin.Path, ".yml") {
				originPath = origin.Path
			}
		}
		return a.analyzeStringValue(val, currentPath, originPath, chartAnalysis)
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
			Path:      currentPath,
			Value:     fmt.Sprintf("%s/%s:%s", registry, repository, tag),
			Structure: imageStructure,
			Count:     1,
		}

		// --- Start: Populate OriginalRegistry AND SourceOrigin ---
		originPath := "values.yaml" // Default origin file
		sourceChartName := ""       // Default chart name
		if origin, exists := a.context.Origins[currentPath]; exists {
			// Use origin.Path if it's a file path, otherwise keep default
			if strings.HasSuffix(origin.Path, ".yaml") || strings.HasSuffix(origin.Path, ".yml") {
				originPath = origin.Path
			}
			sourceChartName = origin.ChartName // Get chart name from origin
		}
		pattern.SourceOrigin = originPath // Set the source origin (file path)

		// Use sourceChartName for OriginalRegistry logic
		if sourceChartName != "" && sourceChartName != a.context.Chart.Metadata.Name {
			log.Debug("Value originates from subchart", "path", currentPath, "sourceChart", sourceChartName)

			// --- Find Subchart AppVersion --- START ---
			var sourceChartAppVersion string
			for _, dep := range a.context.Chart.Dependencies() {
				if dep.Metadata.Name == sourceChartName {
					sourceChartAppVersion = dep.Metadata.AppVersion
					log.Debug("Found AppVersion for source subchart", "chart", sourceChartName, "appVersion", sourceChartAppVersion)
					break
				}
			}
			if sourceChartAppVersion != "" {
				pattern.SourceChartAppVersion = sourceChartAppVersion
			}
			// --- Find Subchart AppVersion --- END ---

			// Determine original registry from the raw map value *before* normalization
			originalRegistry := ""
			if regVal, ok := val["registry"].(string); ok && regVal != "" {
				originalRegistry = regVal
				log.Debug("Found original registry in map structure", "path", currentPath, "originalRegistry", originalRegistry)
			} else {
				// If registry key wasn't present, it effectively used the default OR whatever the string value implied
				// We rely on the parsed registry from normalizeImageValues in this case
				originalRegistry = registry // Use the normalized registry as the effective original
				log.Debug("No explicit registry in map, using normalized as original", "path", currentPath, "originalRegistry", originalRegistry)
			}

			// Populate if different from the final *normalized* registry
			if pattern.Structure != nil {
				if finalRegistry, ok := pattern.Structure["registry"].(string); ok {
					if originalRegistry != finalRegistry {
						pattern.OriginalRegistry = originalRegistry
						log.Debug("Setting OriginalRegistry in pattern", "path", currentPath, "original", originalRegistry, "final", finalRegistry)
					}
				} else {
					log.Warn("Could not access final registry in pattern structure", "path", currentPath)
				}
			} else {
				log.Warn("Pattern structure is nil, cannot set OriginalRegistry", "path", currentPath)
			}
		}
		// --- End: Populate OriginalRegistry AND SourceOrigin ---

		// Add the pattern to the analysis
		chartAnalysis.ImagePatterns = append(chartAnalysis.ImagePatterns, pattern)

		// If we identified this map as a direct image definition, STOP recursion here.
		// We've captured the image; recursing further would create spurious patterns
		// for the '.repository', '.tag', etc. keys within the image map.
		log.Debug("analyzeMapValue: identified as direct image map, stopping recursion", "path", currentPath)
		return nil
	}

	// Always recurse into child map values, analyzeValues handles skipping non-map/string/array leaves.
	// The isDirectImageMapDefinition check above is primarily to *detect* the pattern,
	// not necessarily to stop all recursion for that map.
	// If it wasn't a direct image map, recurse into its children.
	return a.analyzeValues(val, currentPath, chartAnalysis)
}

// analyzeStringValue examines a string value that looks like an image.
// It attempts to parse the string as an image reference and determines which
// registry and repository it contains.
func (a *ContextAwareAnalyzer) analyzeStringValue(val string, currentPath string, originPath string, chartAnalysis *analysis.ChartAnalysis) error {
	// Extract the key from the path for image detection
	parts := strings.Split(currentPath, ".")
	key := currentPath
	if len(parts) > 0 {
		key = parts[len(parts)-1] // Get the last part of the path as the key
	}

	// Skip paths that are unlikely to be image references
	if !a.isProbableImageKeyPath(key, val) {
		log.Debug("analyzeStringValue: Skipping non-probable image path", "path", currentPath)
		return nil
	}

	// 1. Basic Cleanup
	trimmedVal := strings.TrimSpace(val)
	if trimmedVal == "" {
		log.Debug("analyzeStringValue: Empty value, skipping", "path", currentPath)
		return nil
	}

	// 2. Skip Templates
	if strings.Contains(trimmedVal, "{{") && strings.Contains(trimmedVal, "}}") {
		log.Debug("analyzeStringValue: Contains template, skipping", "path", currentPath, "value", trimmedVal)
		return nil
	}

	// 3. Initial Validation with our quick parser
	parsedReg, parsedRepo, parsedTag := a.parseImageStringNoDefaults(trimmedVal)
	if parsedReg == "" && parsedRepo == "" {
		// Very unlikely to be an image value
		log.Debug("analyzeStringValue: Not a valid image-like string", "path", currentPath, "value", trimmedVal)
		return nil
	}
	log.Debug("parseImageStringNoDefaults result", "input", trimmedVal, "registry", parsedReg, "repository", parsedRepo, "tag", parsedTag)

	// 4. Validate Structure (more strict)
	// For string types, we need to make an executive decision on whether this is
	// intended to be an image reference, as opposed to a general string that happens to
	// contain slashes, e.g., "input/output" would parse as reg=input, repo=output
	// This check helps avoid false positives.
	hasStructure := (parsedReg != "" || strings.Contains(parsedRepo, "/"))
	if !hasStructure {
		log.Debug("analyzeStringValue: Failed structural validation, skipping", "path", currentPath, "value", trimmedVal)
		return nil
	}

	// 5. Parse WITH Defaults (for Final Structure)
	// Now that we have higher confidence, parse with defaults for the final structure.
	// Use image.ParseImageReference for potentially more robust parsing than our simple internal one.
	chartMetadata := &image.ChartMetadata{
		Name:       a.context.ChartName,
		Version:    a.context.ChartVersion,
		AppVersion: a.context.AppVersion,
	}
	ref, err := image.ParseImageReference(trimmedVal, chartMetadata)
	if err != nil {
		log.Debug("analyzeStringValue: Parse error after structural validation, skipping", "path", currentPath, "value", trimmedVal, "error", err)
		return nil // Should be rare if step 4 passed, but handle anyway.
	}

	// 6. Create Pattern with Structure
	// Successfully detected an image reference!
	pattern := analysis.ImagePattern{
		Path:  currentPath,
		Type:  analysis.PatternTypeString,
		Value: trimmedVal,
		// Added for context-aware analysis and better output
		SourceOrigin: originPath,
		Structure: map[string]interface{}{
			"registry":   ref.Registry,
			"repository": ref.Repository,
			"tag":        ref.Tag,
		},
		// Add the chart's AppVersion to be used for defaulting
		SourceChartAppVersion: a.context.AppVersion,
		Count:                 1,
	}

	log.Debug("analyzeStringValue: Identified image string via structural validation", "path", currentPath, "value", trimmedVal)
	chartAnalysis.ImagePatterns = append(chartAnalysis.ImagePatterns, pattern)
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

// isProbableImageKeyPath checks if the key and path suggest the value might be an image.
// Acts as an optional optimization filter before more detailed parsing.
func (a *ContextAwareAnalyzer) isProbableImageKeyPath(key string, val string) bool {
	lowerKey := strings.ToLower(key)

	// If the value contains a registry pattern, it's very likely an image
	if strings.Contains(val, "quay.io") || strings.Contains(val, "docker.io") ||
		strings.Contains(val, "gcr.io") || strings.Contains(val, "ghcr.io") ||
		strings.Contains(val, "registry.k8s.io") {
		log.Debug("isProbableImageKeyPath: true (contains registry)", "key", key, "value", val)
		return true
	}

	// Check for exact key names that almost always indicate an image map or string
	if lowerKey == "image" || lowerKey == "repository" {
		log.Debug("isProbableImageKeyPath: true (exact key match)", "key", key)
		return true
	}

	// Check for common image path patterns in Helm charts
	lowerPath := strings.ToLower(key)
	relevantPatterns := []string{
		"image.repository", "image.tag", "repository", "imagerepository",
		"controller.image", "cainjector.image", "webhook.image",
		"image.registry", "registry", "docker", "container",
	}

	for _, pattern := range relevantPatterns {
		if strings.Contains(lowerPath, pattern) {
			log.Debug("isProbableImageKeyPath: true (path pattern match)", "key", key, "pattern", pattern)
			return true
		}
	}

	// Check for key suffix patterns
	suffixPatterns := []string{"Image", "Repository", "Registry", "Container"}
	for _, suffix := range suffixPatterns {
		if strings.HasSuffix(key, suffix) {
			log.Debug("isProbableImageKeyPath: true (key suffix match)", "key", key, "suffix", suffix)
			return true
		}
	}

	// Default to false if no strong indicator found
	log.Debug("isProbableImageKeyPath: false (no strong indicators)", "key", key, "value", val)
	return false
}

// normalizeImageValues extracts normalized image components from a map structure.
func (a *ContextAwareAnalyzer) normalizeImageValues(val map[string]interface{}) (registry, repository, tag string) {
	// Handle registry (optional)
	if regVal, ok := val["registry"].(string); ok && regVal != "" {
		registry = regVal
	} else {
		registry = DefaultRegistry // Default to docker.io if not specified
	}

	// Handle repository (required)
	if repoVal, ok := val["repository"].(string); ok && repoVal != "" {
		repository = repoVal

		// If repository contains registry info (e.g. "quay.io/repo"), extract it
		if strings.Contains(repoVal, "/") {
			parts := strings.SplitN(repoVal, "/", 2)
			if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
				// Override registry from the repository string
				registry = parts[0]
				repository = parts[1]
			}
		}
	}

	// Apply Docker Hub 'library/' normalization for single-name repositories (without slashes)
	if registry == DefaultRegistry && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	// Handle tag (optional)
	if tagVal, ok := val["tag"].(string); ok && tagVal != "" {
		tag = tagVal
	} else if digestVal, ok := val["digest"].(string); ok && digestVal != "" {
		// If digest is present but no tag, leave tag empty (digest will be used)
		tag = ""
	} else {
		// No tag or digest specified, prefer AppVersion if available
		if a.context != nil && a.context.AppVersion != "" {
			tag = a.context.AppVersion
			log.Debug("Using chart's AppVersion as tag", "path", fmt.Sprintf("%v", val), "appVersion", tag)
		} else {
			// Otherwise default to "latest"
			tag = defaultImageTag
			log.Debug("Setting default tag", "path", fmt.Sprintf("%v", val), "tag", tag)
		}
	}

	return registry, repository, tag
}

// parseImageStringNoDefaults parses an image string without applying default registry.
func (a *ContextAwareAnalyzer) parseImageStringNoDefaults(val string) (registry, repository, tag string) {
	// Simplified parsing logic focused on extracting parts without defaulting registry
	// This is a placeholder - a robust implementation should use a proper parser library
	// like docker/distribution/reference, but configured not to default.
	registry = ""
	tag = ""
	remaining := val

	// Check for digest first
	digestIdx := strings.Index(remaining, "@sha256:")
	if digestIdx != -1 {
		// We ignore digest here, just remove it
		remaining = remaining[:digestIdx]
	}

	// Check for tag
	tagIdx := strings.LastIndex(remaining, ":")
	slashIdx := strings.LastIndex(remaining, "/")

	if tagIdx != -1 && tagIdx > slashIdx { // Ensure colon is for tag, not port in registry
		tag = remaining[tagIdx+1:]
		remaining = remaining[:tagIdx]
	}

	// What's left is [registry/]repository
	slashIdx = strings.Index(remaining, "/")
	if slashIdx != -1 {
		firstPart := remaining[:slashIdx]
		// Simple heuristic: if first part contains '.' or ':', assume it's a registry
		if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
			registry = firstPart
			repository = remaining[slashIdx+1:]
		} else {
			// No registry marker found, assume default was intended implicitly OR it's just repo
			repository = remaining
		}
		if tag == "" && strings.Contains(repository, ":") { // Check again if tag was embedded in repo part
			repoParts := strings.SplitN(repository, ":", MaxSplitParts)
			if len(repoParts) > 1 {
				repository = repoParts[0]
				tag = repoParts[1]
			}
		}
	} else {
		// No slash, must be just repository (potentially Docker Hub library image)
		repository = remaining
	}

	log.Debug("parseImageStringNoDefaults result", "input", val, "registry", registry, "repository", repository, "tag", tag)
	return registry, repository, tag
}

// getSourcePathForValue resolves the effective source path for a value based on origin.
// It should return the path as it exists in the *merged* value structure.
func (a *ContextAwareAnalyzer) getSourcePathForValue(valuePath string) string {
	// For context-aware analysis, the ValuePath used in ImageInfo should reflect
	// the path within the fully merged values map (e.g., "subchartAlias.key.subkey").
	// The 'Path' field within analysis.ImagePattern might be repurposed by the analyzer
	// to store the *originating* file/path, but the path passed *into* here
	// during the recursive analysis (`analyzeValues`, `analyzeSingleValue` etc.)
	// represents the structural path in the merged map.
	log.Debug("getSourcePathForValue returning structural path", "path", valuePath)
	return valuePath
}

// GetContext returns the underlying ChartAnalysisContext.
func (a *ContextAwareAnalyzer) GetContext() *ChartAnalysisContext {
	return a.context
}
