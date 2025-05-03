// Package strategy defines interfaces and implementations for different image path generation strategies.
package strategy

import (
	"fmt"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/image"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
)

// DefaultLibraryRepoPrefix is the prefix used for official Docker Hub images.
const DefaultLibraryRepoPrefix = "library" // Duplicated from pkg/analysis to avoid import cycle.

// PathStrategy defines the interface for generating new image paths.
type PathStrategy interface {
	// GeneratePath takes an original image reference and the target registry (if any),
	// and returns the new repository path (e.g., "new-registry/my-app").
	GeneratePath(originalRef *image.Reference, targetRegistry string) (string, error)
}

// GetStrategy returns a path strategy based on the name
func GetStrategy(name string, mappings *registry.Mappings) (PathStrategy, error) {
	log.Debug("GetStrategy: Getting strategy for name", "name", name)

	switch name {
	case "prefix-source-registry":
		log.Debug("GetStrategy: Using PrefixSourceRegistryStrategy")
		return NewPrefixSourceRegistryStrategy(mappings), nil
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
type PrefixSourceRegistryStrategy struct {
	mappings *registry.Mappings
}

// NewPrefixSourceRegistryStrategy creates a new PrefixSourceRegistryStrategy.
func NewPrefixSourceRegistryStrategy(mappings *registry.Mappings) *PrefixSourceRegistryStrategy {
	return &PrefixSourceRegistryStrategy{
		mappings: mappings,
	}
}

// GeneratePath constructs a target path by prefixing the
// original repository path with the sanitized source registry name.
// Example: docker.io/library/nginx -> <target_registry>/docker.io/library/nginx
func (s *PrefixSourceRegistryStrategy) GeneratePath(imgRef *image.Reference, effectiveTargetRegistry string) (string, error) {
	log.Debug("PrefixSourceRegistryStrategy: Generating path for original reference", "originalRef", imgRef)
	log.Debug("PrefixSourceRegistryStrategy: Target registry", "targetRegistry", effectiveTargetRegistry)

	// IMPORTANT: The chart.Generator already handles registry mappings and passes us the
	// effectiveTargetRegistry. We should NOT do mapping lookups again, as that creates
	// a double-handling situation.
	//
	// However, we keep this logic for backward compatibility with direct usage of the strategy.
	// In normal usage through the generator, this code shouldn't be reached since
	// effectiveTargetRegistry would already be set correctly.
	if effectiveTargetRegistry == "" && s.mappings != nil {
		if mappedTarget := s.mappings.GetTargetRegistry(imgRef.Registry); mappedTarget != "" {
			log.Debug("PrefixSourceRegistryStrategy: Found registry mapping", "source", imgRef.Registry, "target", mappedTarget)
			// Extract the repository path from the mapped target
			if strings.Contains(mappedTarget, "/") {
				// If the mapped target contains a path, use it as a prefix
				log.Debug("PrefixSourceRegistryStrategy: Using mapped target path as prefix", "mappedTarget", mappedTarget)
				return fmt.Sprintf("%s/%s", strings.TrimSuffix(mappedTarget, "/"), imgRef.Repository), nil
			}
			// Store the mapped target for use in path construction
			effectiveTargetRegistry = mappedTarget
			log.Debug("PrefixSourceRegistryStrategy: Updated effective target registry", "effectiveTargetRegistry", effectiveTargetRegistry)
		}
	}

	// Normalize the registry name for path-friendly formatting
	normalizedReg := image.NormalizeRegistry(imgRef.Registry)
	log.Debug("NormalizeRegistry: Input '%s' -> Normalized '%s'", imgRef.Registry, normalizedReg)

	// Generate path prefix for the repository
	// Important: For compatibility with tests and expected behavior, we need to
	// preserve the original registry name (with dots) in the repository path.

	// Get the original repository path
	finalRepo := imgRef.Repository

	// Handle Docker Hub official images
	if normalizedReg == image.DefaultRegistry && !strings.Contains(finalRepo, "/") {
		// This is a Docker Hub official image (e.g., "nginx" without a namespace)
		// We prepend the "library/" prefix as per Docker Hub convention
		finalRepo = DefaultLibraryRepoPrefix + "/" + finalRepo
		log.Debug("PrefixSourceRegistryStrategy: Prepended 'library/' to Docker Hub image path", "baseRepoPath", finalRepo)
	}

	// Preserve the original registry with dots in the path
	finalPath := fmt.Sprintf("%s/%s", normalizedReg, finalRepo)
	log.Debug("PrefixSourceRegistryStrategy: Returning final path", "finalPath", finalPath)

	return finalPath, nil
}

// FlatStrategy creates a flat path by replacing slashes with dashes.
// Example: library/nginx -> library-nginx
type FlatStrategy struct{}

// NewFlatStrategy creates a new FlatStrategy.
func NewFlatStrategy() *FlatStrategy {
	return &FlatStrategy{}
}

// GeneratePath implements the PathStrategy interface.
func (s *FlatStrategy) GeneratePath(originalRef *image.Reference, targetRegistry string) (string, error) {
	if originalRef == nil {
		return "", fmt.Errorf("original image reference is nil")
	}

	log.Debug("FlatStrategy: Generating path for original reference", "originalRef", originalRef)
	log.Debug("FlatStrategy: Target registry", "targetRegistry", targetRegistry)

	// Use the original repository path
	baseRepoPath := originalRef.Repository
	log.Debug("FlatStrategy: Using base repository path", "baseRepoPath", baseRepoPath)

	// Handle Docker Hub official images (add library prefix if needed)
	if (image.NormalizeRegistry(originalRef.Registry) == "docker.io") && !strings.Contains(baseRepoPath, "/") {
		log.Debug("FlatStrategy: Prepending 'library-' to Docker Hub image path", "baseRepoPath", baseRepoPath)
		baseRepoPath = "library-" + baseRepoPath
	} else {
		// Replace all slashes with dashes to flatten the path
		baseRepoPath = strings.ReplaceAll(baseRepoPath, "/", "-")
		log.Debug("FlatStrategy: Flattened path", "baseRepoPath", baseRepoPath)
	}

	// Add registry prefix for better organization (optional but recommended)
	registryPrefix := image.SanitizeRegistryForPath(originalRef.Registry)
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
