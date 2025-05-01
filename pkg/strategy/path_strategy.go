// Package strategy defines interfaces and implementations for different image path generation strategies.
package strategy

import (
	"fmt"
	"path"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
)

const (
	// maxSplitTwo is used when splitting strings into at most two parts
	maxSplitTwo = 2
)

// PathStrategy defines the interface for generating new image paths.
type PathStrategy interface {
	// GeneratePath takes an original image reference, the analysis pattern containing original value info,
	// and the target registry (if any), and returns the new repository path (e.g., "new-registry/my-app").
	GeneratePath(originalRef *image.Reference, pattern analysis.ImagePattern, targetRegistry string) (string, error)
}

// GetStrategy returns a path strategy based on the name
func GetStrategy(name string, _ *registry.Mappings) (PathStrategy, error) {
	log.Debug("GetStrategy: Getting strategy for name", "name", name)

	switch name {
	case "prefix-source-registry":
		log.Debug("GetStrategy: Using PrefixSourceRegistryStrategy")
		return NewPrefixSourceRegistryStrategy(), nil
	case "flat":
		log.Debug("GetStrategy: Using FlatStrategy")
		return NewFlatStrategy(), nil
	default:
		log.Debug("GetStrategy: Unknown strategy name", "name", name)
		return nil, fmt.Errorf("unknown path strategy: %s", name)
	}
}

// PrefixSourceRegistryStrategy uses the source registry as a prefix in the new path.
// Example: docker.io/library/nginx -> target-registry.com/docker.io/library/nginx
type PrefixSourceRegistryStrategy struct{}

// NewPrefixSourceRegistryStrategy creates a new PrefixSourceRegistryStrategy.
func NewPrefixSourceRegistryStrategy() *PrefixSourceRegistryStrategy {
	return &PrefixSourceRegistryStrategy{}
}

// GeneratePath implements the PathStrategy interface.
func (s *PrefixSourceRegistryStrategy) GeneratePath(originalRef *image.Reference, pattern analysis.ImagePattern, targetRegistry string) (string, error) {
	if originalRef == nil {
		return "", fmt.Errorf("cannot generate path for nil reference")
	}

	log.Debug("PrefixSourceRegistryStrategy: Generating path",
		"originalRef", originalRef.Original,
		"patternPath", pattern.Path,
		"patternValue", pattern.Value,
		"targetRegistry", targetRegistry,
	)

	// --- START: Determine Original Source Registry for Prefix ---
	// Attempt to parse the original value from the pattern to get the source registry.
	// This avoids using the potentially normalized registry from originalRef.
	originalSourceRegistry := originalRef.Registry // Fallback to normalized registry
	parsedFromPattern, err := image.ParseImageReference(pattern.Value)
	if err == nil && parsedFromPattern.Registry != "" {
		originalSourceRegistry = parsedFromPattern.Registry
		log.Debug("PrefixSourceRegistryStrategy: Using original registry from pattern value", "originalSourceRegistry", originalSourceRegistry)
	} else {
		// If parsing pattern.Value failed or registry was empty (e.g., "nginx:latest"),
		// use the registry detected by the main parser (originalRef.Registry),
		// which might be the normalized default (e.g., "docker.io").
		log.Debug("PrefixSourceRegistryStrategy: Falling back to registry from parsed reference", "originalSourceRegistry", originalSourceRegistry, "parseError", err)
	}
	pathPrefix := image.SanitizeRegistryForPath(originalSourceRegistry)
	log.Debug("PrefixSourceRegistryStrategy: Using sanitized original source registry prefix", "pathPrefix", pathPrefix)
	// --- END: Determine Original Source Registry for Prefix ---

	// Split repository into org/name parts (use normalized repository from originalRef)
	repoPathParts := strings.SplitN(originalRef.Repository, "/", maxSplitTwo)

	// --- Base Repository Path Calculation (Keep existing logic using originalRef.Repository) ---
	// Ensure we only use the repository path part, excluding any original registry prefix
	baseRepoPath := originalRef.Repository // Use normalized repo path
	if len(repoPathParts) > 1 {
		if len(repoPathParts) > 1 && (strings.Contains(repoPathParts[0], ".") || strings.Contains(repoPathParts[0], ":") || repoPathParts[0] == "localhost") {
			log.Debug("PrefixSourceRegistryStrategy: Stripping potential registry prefix from repo", "repoPathParts", repoPathParts[0], "originalRef.Repository", originalRef.Repository)
			baseRepoPath = strings.Join(repoPathParts[1:], "/")
		}
	}
	log.Debug("PrefixSourceRegistryStrategy: Using base repository path", "baseRepoPath", baseRepoPath)

	// Handle Docker Hub official images (add library/ prefix if needed)
	if (image.NormalizeRegistry(originalSourceRegistry) == "docker.io") && !strings.Contains(baseRepoPath, "/") {
		log.Debug("PrefixSourceRegistryStrategy: Prepending 'library/' to Docker Hub image path", "baseRepoPath", baseRepoPath)
		baseRepoPath = path.Join("library", baseRepoPath)
	}
	// --- End Base Repository Path Calculation ---

	// Construct the final repository path part by joining the prefix (from original source) and base path (from normalized repo)
	finalRepoPathPart := path.Join(pathPrefix, baseRepoPath)

	log.Debug("PrefixSourceRegistryStrategy: Generated final repo path part", "finalRepoPathPart", finalRepoPathPart)
	return finalRepoPathPart, nil
}

// FlatStrategy creates a flat path by replacing slashes with dashes.
// Example: library/nginx -> library-nginx
type FlatStrategy struct{}

// NewFlatStrategy creates a new FlatStrategy.
func NewFlatStrategy() *FlatStrategy {
	return &FlatStrategy{}
}

// GeneratePath implements the PathStrategy interface.
func (s *FlatStrategy) GeneratePath(originalRef *image.Reference, pattern analysis.ImagePattern, targetRegistry string) (string, error) {
	// TODO: Update FlatStrategy logic if needed based on new signature/pattern info
	if originalRef == nil {
		return "", fmt.Errorf("original image reference is nil")
	}

	log.Debug("FlatStrategy: Generating path for original reference", "originalRef", originalRef)
	log.Debug("FlatStrategy: Target registry", "targetRegistry", targetRegistry)
	log.Debug("FlatStrategy: Pattern info", "patternPath", pattern.Path, "patternValue", pattern.Value)

	// Determine original source registry (similar to Prefix strategy)
	originalSourceRegistry := originalRef.Registry // Fallback
	parsedFromPattern, err := image.ParseImageReference(pattern.Value)
	if err == nil && parsedFromPattern.Registry != "" {
		originalSourceRegistry = parsedFromPattern.Registry
	}

	// Use the original repository path from the reference
	baseRepoPath := originalRef.Repository
	log.Debug("FlatStrategy: Using base repository path", "baseRepoPath", baseRepoPath)

	// Handle Docker Hub official images (add library prefix if needed)
	if (image.NormalizeRegistry(originalSourceRegistry) == "docker.io") && !strings.Contains(baseRepoPath, "/") {
		log.Debug("FlatStrategy: Prepending 'library-' to Docker Hub image path", "baseRepoPath", baseRepoPath)
		baseRepoPath = "library-" + baseRepoPath
	} else {
		// Replace all slashes with dashes to flatten the path
		baseRepoPath = strings.ReplaceAll(baseRepoPath, "/", "-")
		log.Debug("FlatStrategy: Flattened path", "baseRepoPath", baseRepoPath)
	}

	// Add registry prefix for better organization (optional but recommended)
	registryPrefix := image.SanitizeRegistryForPath(originalSourceRegistry)
	finalRepoPathPart := registryPrefix + "-" + baseRepoPath

	log.Debug("FlatStrategy: Final flattened path", "finalRepoPathPart", finalRepoPathPart)

	return finalRepoPathPart, nil
}

// ---
// Logging migration progress note:
// - pkg/strategy/path_strategy.go: All debug logging migrated to slog-based logger (log.Debug, log.Error, log.Warn).
// - All debug.* calls replaced with slog style logging.
// - Next: Continue migration in other files using the debug package.
// ---
