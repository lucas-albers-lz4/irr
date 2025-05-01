// Package helm provides internal utilities for interacting with Helm.
package helm

import (
	"fmt"
	"strings"

	// Ensure correct import if needed elsewhere, or remove if unused
	// Assuming chart package is needed

	"github.com/lucas-albers-lz4/irr/pkg/analysis" // Corrected import path assuming it exists
	helmtypes "github.com/lucas-albers-lz4/irr/pkg/helmtypes"
	"github.com/lucas-albers-lz4/irr/pkg/image" // Correct import for ParseImageReference
	"github.com/lucas-albers-lz4/irr/pkg/log"
	// Corrected import path assuming it exists
)

// GetSourcePathForValue attempts to find the original source path for a given path in the merged values.
// It uses the pre-computed Origins map.
func GetSourcePathForValue(c *helmtypes.ChartAnalysisContext, mergedPath string) (string, bool) {
	origin, found := c.Origins[mergedPath]
	if !found {
		log.Debug("Origin not found for merged path", "path", mergedPath)
		parts := strings.Split(mergedPath, ".")
		if len(parts) > 1 {
			parentPath := strings.Join(parts[:len(parts)-1], ".")
			log.Debug("Retrying origin lookup for parent path", "path", parentPath)
		}
		return "", false
	}

	log.Debug("Origin found for merged path", "path", mergedPath, "type", origin.Type, "originPath", origin.Path)
	return origin.Path, true
}

// FindImagePatterns traverses the MergedValues using the Origins map to report correct source paths.
func FindImagePatterns(c *helmtypes.ChartAnalysisContext, includePatterns, excludePatterns []string) ([]analysis.ImagePattern, error) {
	patterns := []analysis.ImagePattern{}
	log.Debug("Starting image pattern search", "mergedValueCount", len(c.MergedValues), "originCount", len(c.Origins))

	if c.MergedValues == nil {
		log.Warn("MergedValues map is nil, cannot find image patterns.")
		return patterns, nil
	}

	err := findPatternsRecursive(c, c.MergedValues, "", &patterns, includePatterns, excludePatterns)
	if err != nil {
		log.Error("Error during recursive pattern search", "error", err)
		return nil, err
	}
	log.Debug("Finished image pattern search", "patternsFound", len(patterns))
	return patterns, nil
}

// findPatternsRecursive is a helper to traverse the merged values map.
func findPatternsRecursive(
	c *helmtypes.ChartAnalysisContext,
	currentVal interface{},
	pathSoFar string,
	patterns *[]analysis.ImagePattern,
	includePatterns, excludePatterns []string,
) error {
	switch v := currentVal.(type) {
	case map[string]interface{}:
		log.Debug("Traversing map", "path", pathSoFar)
		for key, val := range v {
			currentMergedPath := key
			if pathSoFar != "" {
				currentMergedPath = pathSoFar + "." + key
			}

			if err := findPatternsRecursive(c, val, currentMergedPath, patterns, includePatterns, excludePatterns); err != nil {
				return err
			}
		}
	case []interface{}:
		log.Debug("Traversing slice", "path", pathSoFar)
		for i, item := range v {
			currentMergedPath := fmt.Sprintf("%s[%d]", pathSoFar, i)
			if err := findPatternsRecursive(c, item, currentMergedPath, patterns, includePatterns, excludePatterns); err != nil {
				return err
			}
		}
	case string:
		log.Debug("Checking string", "path", pathSoFar)
		imgRef, err := image.ParseImageReference(v)
		if err == nil && imgRef != nil && imgRef.Repository != "" {
			log.Debug("Found potential image ref", "value", v, "path", pathSoFar)
			sourcePath, found := GetSourcePathForValue(c, pathSoFar)
			if !found {
				log.Warn("Could not find origin for merged path. Skipping image pattern.", "path", pathSoFar)
				return nil
			}

			pattern := analysis.ImagePattern{
				Path:  sourcePath,
				Value: v,
				Type:  analysis.PatternTypeString,
			}
			log.Debug("Adding ImagePattern", "path", pattern.Path, "value", pattern.Value)
			*patterns = append(*patterns, pattern)
		} else {
			log.Debug("String is not a valid image reference", "value", v, "path", pathSoFar)
		}
	default:
		log.Debug("Skipping non-string/non-map/non-slice value", "path", pathSoFar, "type", fmt.Sprintf("%T", v))
	}
	return nil
}

// --- Helper functions or other methods for ChartAnalysisContext can go here ---
// Example:
// func matchesPathPattern(path string, patterns []string) bool { ... }
