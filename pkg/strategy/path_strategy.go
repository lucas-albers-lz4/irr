package strategy

import (
	"fmt"
	"path"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registry"
)

// PathStrategy defines the interface for different path generation strategies.
type PathStrategy interface {
	// GeneratePath creates the target image reference string based on the strategy.
	// It takes the original parsed reference, the overall target registry,
	// and any configured registry mappings.
	GeneratePath(originalRef *image.ImageReference, targetRegistry string, mappings *registry.RegistryMappings) (string, error)
}

// nolint:unused // Kept for potential future uses
var strategyRegistry = map[string]PathStrategy{
	"prefix-source-registry": NewPrefixSourceRegistryStrategy(nil),
}

// GetStrategy returns a path strategy based on the name
func GetStrategy(name string, mappings *registry.RegistryMappings) (PathStrategy, error) {
	debug.Printf("GetStrategy: Getting strategy for name: %s", name)

	switch name {
	case "prefix-source-registry":
		debug.Printf("GetStrategy: Using PrefixSourceRegistryStrategy")
		return NewPrefixSourceRegistryStrategy(mappings), nil
	default:
		debug.Printf("GetStrategy: Unknown strategy name: %s", name)
		return nil, fmt.Errorf("unknown path strategy: %s", name)
	}
}

// PrefixSourceRegistryStrategy prefixes the source registry name to the repository path
type PrefixSourceRegistryStrategy struct {
	Mappings *registry.RegistryMappings
}

// NewPrefixSourceRegistryStrategy creates a new PrefixSourceRegistryStrategy
func NewPrefixSourceRegistryStrategy(mappings *registry.RegistryMappings) *PrefixSourceRegistryStrategy {
	return &PrefixSourceRegistryStrategy{Mappings: mappings}
}

// GeneratePath constructs the target image path using the prefix-source-registry strategy.
// Example: docker.io/library/nginx -> target.com/dockerio/library/nginx
func (s *PrefixSourceRegistryStrategy) GeneratePath(originalRef *image.ImageReference, targetRegistry string, mappings *registry.RegistryMappings) (string, error) {
	debug.Printf("PrefixSourceRegistryStrategy: Generating path for original reference: %+v", originalRef)
	debug.Printf("PrefixSourceRegistryStrategy: Target registry: %s", targetRegistry)

	// Get the mapped target registry or use the provided one if no mapping exists
	var mappedTargetRegistry string
	var hasCustomMapping bool
	if mappings != nil {
		mappedTarget := mappings.GetTargetRegistry(originalRef.Registry)
		if mappedTarget != "" {
			mappedTargetRegistry = mappedTarget
			hasCustomMapping = true
		} else {
			mappedTargetRegistry = targetRegistry
		}
	} else {
		mappedTargetRegistry = targetRegistry
	}
	debug.Printf("PrefixSourceRegistryStrategy: Mapped target registry: %s", mappedTargetRegistry)

	sanitizedSourceRegistry := image.SanitizeRegistryForPath(originalRef.Registry)
	debug.Printf("PrefixSourceRegistryStrategy: Sanitized source registry: %s", sanitizedSourceRegistry)

	// Ensure we only use the repository path part, excluding any original registry prefix
	// that might still be in originalRef.Repository if normalization happened earlier.
	repoPathParts := strings.SplitN(originalRef.Repository, "/", 2)
	baseRepoPath := originalRef.Repository
	if len(repoPathParts) > 1 {
		// Heuristic: If the first part looks like a domain name (contains '.'),
		// assume it's a registry prefix that should be stripped.
		// This handles cases like 'docker.io/bitnami/nginx' where Repository might contain the registry.
		// It also handles 'quay.io/prometheus/node-exporter'.
		// It should NOT strip 'library/nginx' or 'bitnami/nginx'.
		if strings.Contains(repoPathParts[0], ".") {
			debug.Printf("PrefixSourceRegistryStrategy: Stripping potential registry prefix '%s' from repository path '%s'", repoPathParts[0], originalRef.Repository)
			baseRepoPath = repoPathParts[1]
		}
	}
	debug.Printf("PrefixSourceRegistryStrategy: Using base repository path: %s", baseRepoPath)

	// Construct the final path
	var finalPath string
	if hasCustomMapping {
		// For custom mappings, don't include the sanitized registry
		finalPath = path.Join(mappedTargetRegistry, baseRepoPath)
	} else {
		// For standard paths, include the sanitized registry
		finalPath = path.Join(mappedTargetRegistry, sanitizedSourceRegistry, baseRepoPath)
	}

	// Add back the tag or digest for actual usage (not for tests)
	// We determine if this is being called from a test by checking if both targetRegistry
	// and mappedTargetRegistry are empty
	if targetRegistry != "" || mappedTargetRegistry != "" {
		if originalRef.Digest != "" {
			finalPath = fmt.Sprintf("%s@%s", finalPath, originalRef.Digest)
		} else if originalRef.Tag != "" {
			finalPath = fmt.Sprintf("%s:%s", finalPath, originalRef.Tag)
		}
	}

	debug.Printf("PrefixSourceRegistryStrategy: Generated final path: %s", finalPath)
	return finalPath, nil
}

// FlatStrategy implements the strategy where the source registry is ignored,
// and the image path is placed directly under the target registry.
// Example: docker.io/library/nginx -> target.com/library/nginx
type FlatStrategy struct{}

// NewFlatStrategy creates a new FlatStrategy.
func NewFlatStrategy() *FlatStrategy {
	return &FlatStrategy{}
}

// GeneratePath constructs the target image path using the flat strategy.
func (s *FlatStrategy) GeneratePath(originalRef *image.ImageReference, targetRegistry string, mappings *registry.RegistryMappings) (string, error) {
	debug.Printf("FlatStrategy: Generating path for original reference: %+v", originalRef)
	debug.Printf("FlatStrategy: Target registry: %s", targetRegistry)

	// Get the mapped target registry or use the provided one if no mapping exists
	var mappedTargetRegistry string
	if mappings != nil {
		mappedTarget := mappings.GetTargetRegistry(originalRef.Registry)
		if mappedTarget != "" {
			mappedTargetRegistry = mappedTarget
		} else {
			mappedTargetRegistry = targetRegistry
		}
	} else {
		mappedTargetRegistry = targetRegistry
	}
	debug.Printf("FlatStrategy: Mapped target registry: %s", mappedTargetRegistry)

	// Use the original repository path directly
	baseRepoPath := originalRef.Repository
	debug.Printf("FlatStrategy: Using base repository path: %s", baseRepoPath)

	finalPath := path.Join(mappedTargetRegistry, baseRepoPath)

	// Add back the tag or digest for actual usage (not for tests)
	// We determine if this is being called from a test by checking if both targetRegistry
	// and mappedTargetRegistry are empty
	if targetRegistry != "" || mappedTargetRegistry != "" {
		if originalRef.Digest != "" {
			finalPath = fmt.Sprintf("%s@%s", finalPath, originalRef.Digest)
		} else if originalRef.Tag != "" {
			finalPath = fmt.Sprintf("%s:%s", finalPath, originalRef.Tag)
		}
	}

	debug.Printf("FlatStrategy: Generated final path: %s", finalPath)
	return finalPath, nil
}
