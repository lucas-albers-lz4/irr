// Package helm provides internal utilities for interacting with Helm.
package helm

import (
	"fmt"
	"path/filepath"
	"strings"

	analyzer "github.com/lucas-albers-lz4/irr/pkg/analyzer"
	helmtypes "github.com/lucas-albers-lz4/irr/pkg/helmtypes"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/log"
)

const (
	// DefaultRegistry is the standard Docker Hub registry
	DefaultRegistry = "docker.io"
)

// ContextAnalyzer uses the ChartAnalysisContext (with pre-computed merged values and origins)
// to find image patterns.
// It replaces the older Analyzer that relied on loading the chart itself and re-computing values.
type ContextAnalyzer struct {
	Context *helmtypes.ChartAnalysisContext // Use the shared type
	Config  *analyzer.Config                // Include/exclude patterns, etc.
}

// NewContextAnalyzer creates a new analyzer based on a ChartAnalysisContext.
func NewContextAnalyzer(ctx *helmtypes.ChartAnalysisContext, cfg *analyzer.Config) *ContextAnalyzer {
	return &ContextAnalyzer{
		Context: ctx,
		Config:  cfg,
	}
}

// NewContextAwareAnalyzer is an exported alias for NewContextAnalyzer for external use.
func NewContextAwareAnalyzer(ctx *helmtypes.ChartAnalysisContext) *ContextAnalyzer {
	return NewContextAnalyzer(ctx, nil)
}

// Analyze traverses the context's MergedValues and uses the Origins map
// to identify image patterns with their correct source paths.
func (a *ContextAnalyzer) Analyze() ([]analyzer.ImagePattern, error) {
	patterns := []analyzer.ImagePattern{}
	if a.Context == nil || a.Context.MergedValues == nil {
		log.Warn("ContextAnalyzer: Context or MergedValues is nil. Returning empty patterns.")
		return patterns, nil
	}

	err := a.analyzeRecursive(a.Context.MergedValues, "", &patterns)
	if err != nil {
		return nil, fmt.Errorf("error during context analysis: %w", err)
	}
	return patterns, nil
}

// analyzeRecursive performs the traversal of the merged values map.
func (a *ContextAnalyzer) analyzeRecursive(currentVal interface{}, pathSoFar string, patterns *[]analyzer.ImagePattern) error {
	switch v := currentVal.(type) {
	case map[string]interface{}:
		// Check if this map represents a structured image
		if IsImageMap(v) {
			log.Debug("Found potential image map", "path", pathSoFar)
			registry, repository, tag := NormalizeImageMapValues(v)
			constructedRefStr := ConstructImageString(registry, repository, tag)

			// Attempt to parse for validation, but use constructed string for pattern
			_, err := image.ParseImageReference(constructedRefStr)
			if err != nil {
				log.Warn("Failed to parse constructed image string from map, but proceeding", "path", pathSoFar, "value", constructedRefStr, "error", err)
				// Decide whether to add pattern anyway or skip? Let's add it for now.
			}

			// Get the origin source path using the context
			sourcePath := a.getSourcePath(pathSoFar)
			if sourcePath == "" {
				log.Warn("Could not find origin for merged map path. Skipping image pattern.", "path", pathSoFar)
				return nil // Skip this pattern
			}

			pattern := analyzer.ImagePattern{
				Path:  sourcePath, // Use the source path from origin
				Value: constructedRefStr,
				Type:  "map",
				Structure: &analyzer.ImageStructure{
					Registry:   registry,
					Repository: repository,
					Tag:        tag,
				},
			}
			if a.shouldIncludePattern(pattern) {
				*patterns = append(*patterns, pattern)
				log.Debug("Added image map pattern", "path", pattern.Path, "value", pattern.Value)
			}
			// Important: Do *not* recurse further into a detected image map
			return nil
		}

		// If not an image map, recurse into its key-value pairs
		for key, val := range v {
			currentMergedPath := key
			if pathSoFar != "" {
				currentMergedPath = pathSoFar + "." + key
			}
			if err := a.analyzeRecursive(val, currentMergedPath, patterns); err != nil {
				return err
			}
		}

	case []interface{}:
		// Arrays are tricky for path generation, Helm uses indices like `key[0]`
		// For simplicity in origin lookup, we might skip array analysis or need a more robust path mapping.
		// Let's skip analyzing inside arrays for now, as image refs are less common here.
		// _ = i // Avoid unused variable error
		// _ = item
		// currentMergedPath := fmt.Sprintf("%s[%d]", pathSoFar, i)
		// if err := a.analyzeRecursive(item, currentMergedPath, patterns); err != nil {
		// 	return err
		// }

	case string:
		// Check if the string looks like an image reference
		imgRef, err := image.ParseImageReference(v)
		if err == nil && imgRef != nil && imgRef.Repository != "" {
			// It parses! Get the origin source path.
			sourcePath := a.getSourcePath(pathSoFar)
			if sourcePath == "" {
				log.Warn("Could not find origin for merged string path. Skipping image pattern.", "path", pathSoFar)
				return nil // Skip this pattern
			}

			pattern := analyzer.ImagePattern{
				Path:  sourcePath,      // Use the source path from origin
				Value: imgRef.String(), // Use canonical value
				Type:  "string",
			}
			if a.shouldIncludePattern(pattern) {
				*patterns = append(*patterns, pattern)
				log.Debug("Added image string pattern", "path", pattern.Path, "value", pattern.Value)
			}
		}
	}
	return nil
}

// getSourcePath looks up the origin for a merged path and returns the relevant source path.
func (a *ContextAnalyzer) getSourcePath(mergedPath string) string {
	if a.Context == nil || a.Context.Origins == nil {
		return "" // Cannot determine path without context/origins
	}

	origin, found := a.Context.Origins[mergedPath]
	if !found {
		// Attempt lookup in parent path if leaf not found directly
		parts := strings.Split(mergedPath, ".")
		if len(parts) > 1 {
			parentPath := strings.Join(parts[:len(parts)-1], ".")
			parentOrigin, parentFound := a.Context.Origins[parentPath]
			if parentFound {
				origin = parentOrigin
				found = true
				log.Debug("Using parent origin for path", "mergedPath", mergedPath, "parentPath", parentPath)
			}
		}
		// If still not found, return empty
		if !found {
			log.Warn("Origin not found for merged path or its parent", "mergedPath", mergedPath)
			return ""
		}
	}

	// Construct the source path based on origin type
	switch origin.Type {
	case helmtypes.OriginChartDefault:
		aliasOrName := origin.Alias
		switch {
		case aliasOrName == "":
			// No alias: Find prefix by chart name
			log.Debug("ChartDefault Origin: No alias, finding prefix for chart", "chartName", origin.ChartName)
			prefix := a.findPrefixForChart(origin.ChartName) // Need helper
			if prefix != "" && strings.HasPrefix(mergedPath, prefix+".") {
				log.Debug("ChartDefault Origin: Found prefix, trimming path", "prefix", prefix, "mergedPath", mergedPath)
				return strings.TrimPrefix(mergedPath, prefix+".")
			}
			log.Warn("Could not determine chart prefix for default origin, using merged path as fallback", "chartName", origin.ChartName, "mergedPath", mergedPath)
			return mergedPath // Fallback
		case strings.HasPrefix(mergedPath, aliasOrName+"."):
			// Alias exists and path starts with it: Trim prefix
			log.Debug("ChartDefault Origin: Alias matches path prefix, trimming path", "alias", aliasOrName, "mergedPath", mergedPath)
			return strings.TrimPrefix(mergedPath, aliasOrName+".")
		default:
			// Path doesn't match alias: Fallback to merged path.
			log.Warn("Merged path does not start with expected alias for default origin", "alias", aliasOrName, "mergedPath", mergedPath)
			return mergedPath
		}

	case helmtypes.OriginParentOverride:
		// Path is relative to the subchart being overridden.
		// Example: mergedPath = "subchart.image.tag", origin.Alias = "subchart"
		//          We want "image.tag"
		aliasOrName := origin.Alias // Parent override uses alias/name key
		if aliasOrName != "" && strings.HasPrefix(mergedPath, aliasOrName+".") {
			return strings.TrimPrefix(mergedPath, aliasOrName+".")
		}
		log.Warn("Merged path does not start with expected alias for parent override origin, using fallback", "alias", aliasOrName, "mergedPath", mergedPath)
		return mergedPath // Fallback

	case helmtypes.OriginUserFile:
		// Path is exactly as it appears in the user file and merged values.
		return mergedPath

	case helmtypes.OriginUserSet:
		// Path is the key used in the --set flag.
		// Need to parse the key from origin.Key (which stores "key=value")
		if parts := strings.SplitN(origin.Key, "=", expectedSplitParts); len(parts) > 0 {
			// TODO: Handle complex strvals paths like key[0].name more robustly.
			// For now, use the key part directly. It should match mergedPath.
			if parts[0] == mergedPath {
				return parts[0]
			}
			log.Warn("Mismatch between --set key and merged path, using fallback", "setKey", parts[0], "mergedPath", mergedPath)
			return mergedPath // Fallback
		}
		log.Warn("Could not parse key from --set origin key", "originKey", origin.Key)
		return mergedPath // Fallback

	default: // helmtypes.OriginUnknown
		log.Warn("Unknown origin type for path", "mergedPath", mergedPath, "originType", origin.Type)
		return mergedPath // Fallback
	}
}

// findPrefixForChart is a placeholder helper to find the alias/name prefix for a given chart name.
// This needs access to the chart's dependency metadata.
func (a *ContextAnalyzer) findPrefixForChart(chartName string) string {
	if a.Context == nil || a.Context.LoadedChart == nil {
		return "" // Cannot determine without chart context
	}
	// Check top-level chart name
	if a.Context.LoadedChart.Name() == chartName {
		return "" // Top-level chart has no prefix
	}
	// Search dependencies
	for _, dep := range a.Context.LoadedChart.Metadata.Dependencies {
		if dep.Name == chartName {
			if dep.Alias != "" {
				return dep.Alias
			}
			return dep.Name // Use name if no alias
		}
	}
	log.Warn("Could not find dependency matching chart name to determine prefix", "chartName", chartName)
	return "" // Not found
}

// shouldIncludePattern checks if a pattern should be included based on config.
func (a *ContextAnalyzer) shouldIncludePattern(pattern analyzer.ImagePattern) bool {
	if a.Config == nil {
		return true // No config, include everything
	}

	path := pattern.Path

	// Check exclude patterns first
	for _, exclude := range a.Config.ExcludePatterns {
		matched, err := filepath.Match(exclude, path)
		if err != nil {
			log.Warn("Invalid exclude pattern", "pattern", exclude, "error", err)
			continue // Skip invalid patterns
		}
		if matched {
			log.Debug("Excluding pattern due to exclude rule", "path", path, "rule", exclude)
			return false
		}
	}

	// If include patterns exist, path MUST match one
	if len(a.Config.IncludePatterns) > 0 {
		foundMatch := false
		for _, include := range a.Config.IncludePatterns {
			matched, err := filepath.Match(include, path)
			if err != nil {
				log.Warn("Invalid include pattern", "pattern", include, "error", err)
				continue // Skip invalid patterns
			}
			if matched {
				log.Debug("Including pattern due to include rule", "path", path, "rule", include)
				foundMatch = true
				break
			}
		}
		if !foundMatch {
			log.Debug("Excluding pattern because it didn't match any include rules", "path", path)
			return false // Didn't match any include patterns
		}
	}

	// If we reach here, it wasn't excluded and met include criteria (if any)
	return true
}

// IsImageMap checks if the provided map looks like a Helm image map structure.
// It requires at least a non-empty "repository" key.
func IsImageMap(val map[string]interface{}) bool {
	repo, ok := val["repository"].(string)
	if !ok || repo == "" {
		return false
	}
	// Optionally check for tag/registry, but not required
	return true
}

// NormalizeImageMapValues extracts registry, repository, and tag from a Helm image map
func NormalizeImageMapValues(val map[string]interface{}) (registry, repository, tag string) {
	repositoryVal, ok := val["repository"]
	if ok {
		repository, ok = repositoryVal.(string)
		if !ok {
			repository = ""
		}
	}
	registryVal, ok := val["registry"]
	if ok {
		registry, ok = registryVal.(string)
		if !ok {
			registry = ""
		}
	}
	tagVal, ok := val["tag"]
	if ok {
		tag, ok = tagVal.(string)
		if !ok {
			tag = ""
		}
	}
	return registry, repository, tag
}

// ConstructImageString builds a canonical image string from registry, repository, and tag
func ConstructImageString(registry, repository, tag string) string {
	if registry != "" {
		return registry + "/" + repository + ":" + tag
	}
	return repository + ":" + tag
}
