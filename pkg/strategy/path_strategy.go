// Package strategy defines interfaces and implementations for generating image paths.
package strategy

import (
	"fmt"
	"path"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registrymapping"
)

// PathStrategy defines the interface for different path generation strategies.
type PathStrategy interface {
	// GeneratePath creates the target image reference string based on the strategy.
	// It takes the original parsed reference and the overall target registry.
	GeneratePath(originalRef *image.ImageReference, targetRegistry string) (string, error)
}

// nolint:unused // Kept for potential future uses
var strategyRegistry = map[string]PathStrategy{
	"prefix-source-registry": NewPrefixSourceRegistryStrategy(),
}

// GetStrategy returns a path strategy based on the name
func GetStrategy(name string, mappings *registrymapping.RegistryMappings) (PathStrategy, error) {
	debug.Printf("GetStrategy: Getting strategy for name: %s", name)

	switch name {
	case "prefix-source-registry":
		debug.Printf("GetStrategy: Using PrefixSourceRegistryStrategy")
		return NewPrefixSourceRegistryStrategy(), nil
	default:
		debug.Printf("GetStrategy: Unknown strategy name: %s", name)
		return nil, fmt.Errorf("unknown path strategy: %s", name)
	}
}

// PrefixSourceRegistryStrategy prefixes the source registry name to the repository path
type PrefixSourceRegistryStrategy struct {
	// Mappings *registrymapping.RegistryMappings // Remove field
}

// NewPrefixSourceRegistryStrategy creates a new PrefixSourceRegistryStrategy
func NewPrefixSourceRegistryStrategy() *PrefixSourceRegistryStrategy {
	return &PrefixSourceRegistryStrategy{}
}

// GeneratePath constructs the target image path using the prefix-source-registry strategy.
// Example: docker.io/library/nginx -> target.com/dockerio/library/nginx
// Example with mapping docker.io -> dckr: docker.io/library/nginx -> target.com/dckr/library/nginx
// This function ONLY returns the repository path part (e.g., "dockerio/library/nginx" or "dckr/library/nginx").
// The caller (generator) prepends the target registry and appends the tag/digest.
func (s *PrefixSourceRegistryStrategy) GeneratePath(originalRef *image.ImageReference, targetRegistry string) (string, error) {
	debug.Printf("PrefixSourceRegistryStrategy: Generating path for original reference: %+v", originalRef)
	debug.Printf("PrefixSourceRegistryStrategy: Target registry: %s", targetRegistry)

	// Always use the sanitized source registry name as the prefix
	pathPrefix := image.SanitizeRegistryForPath(originalRef.Registry)
	debug.Printf("PrefixSourceRegistryStrategy: Using sanitized source registry prefix '%s'", pathPrefix)

	// --- Base Repository Path Calculation (Keep existing logic) ---
	// Ensure we only use the repository path part, excluding any original registry prefix
	repoPathParts := strings.SplitN(originalRef.Repository, "/", 2)
	baseRepoPath := originalRef.Repository
	if len(repoPathParts) > 1 {
		// Heuristic: Check if the first part looks like a domain (contains '.')
		// Handle cases like 'docker.io/bitnami/nginx' or 'quay.io/prometheus/node-exporter'
		if strings.Contains(repoPathParts[0], ".") {
			debug.Printf("PrefixSourceRegistryStrategy: Stripping potential registry prefix '%s' from repository path '%s'", repoPathParts[0], originalRef.Repository)
			baseRepoPath = repoPathParts[1]
		}
	}
	debug.Printf("PrefixSourceRegistryStrategy: Using base repository path: %s", baseRepoPath)

	// Handle Docker Hub official images (add library/ prefix if needed)
	if (image.NormalizeRegistry(originalRef.Registry) == "docker.io") && !strings.Contains(baseRepoPath, "/") {
		debug.Printf("PrefixSourceRegistryStrategy: Prepending 'library/' to Docker Hub image path: %s", baseRepoPath)
		baseRepoPath = path.Join("library", baseRepoPath)
	}
	// --- End Base Repository Path Calculation ---

	// Construct the final repository path part by joining the prefix and base path
	finalRepoPathPart := path.Join(pathPrefix, baseRepoPath)

	debug.Printf("PrefixSourceRegistryStrategy: Generated final repo path part: %s", finalRepoPathPart)
	return finalRepoPathPart, nil
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
func (s *FlatStrategy) GeneratePath(originalRef *image.ImageReference, targetRegistry string) (string, error) {
	debug.Printf("FlatStrategy: Generating path for original reference: %+v", originalRef)
	debug.Printf("FlatStrategy: Target registry: %s", targetRegistry)

	// Get the mapped target registry (only needed if we construct full path here)
	// For Flat strategy, we just need the base repository path.
	// var mappedTargetRegistry string // No longer needed here
	// ... mapping logic removed ...

	// Use the original repository path directly
	baseRepoPath := originalRef.Repository
	debug.Printf("FlatStrategy: Using base repository path: %s", baseRepoPath)

	// NOTE: This function now ONLY returns the repository part (e.g., "library/nginx")
	// The caller (generator) is responsible for prepending the actual target registry.

	// finalPath := path.Join(mappedTargetRegistry, baseRepoPath) // Remove this
	finalRepoPathPart := baseRepoPath // Return only the repo part

	// Remove logic that adds tag/digest here, caller handles it.
	// ... tag/digest logic removed ...

	debug.Printf("FlatStrategy: Generated final repo path part: %s", finalRepoPathPart)
	return finalRepoPathPart, nil
}
