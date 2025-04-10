// Package strategy defines interfaces and implementations for different image path generation strategies.
package strategy

import (
	"fmt"
	"path"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registry"
)

const (
	// maxSplitTwo is used when splitting strings into at most two parts
	maxSplitTwo = 2
)

// PathStrategy defines the interface for generating new image paths.
type PathStrategy interface {
	// GeneratePath takes an original image reference and the target registry (if any),
	// and returns the new repository path (e.g., "new-registry/my-app").
	GeneratePath(originalRef *image.Reference, targetRegistry string) (string, error)
}

// nolint:unused // Kept for potential future uses
var strategyRegistry = map[string]PathStrategy{
	"prefix-source-registry": NewPrefixSourceRegistryStrategy(),
}

// GetStrategy returns a path strategy based on the name
func GetStrategy(name string, _ *registry.Mappings) (PathStrategy, error) {
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

// PrefixSourceRegistryStrategy uses the source registry as a prefix in the new path.
// Example: docker.io/library/nginx -> target-registry.com/docker.io/library/nginx
type PrefixSourceRegistryStrategy struct{}

// NewPrefixSourceRegistryStrategy creates a new PrefixSourceRegistryStrategy.
func NewPrefixSourceRegistryStrategy() *PrefixSourceRegistryStrategy {
	return &PrefixSourceRegistryStrategy{}
}

// GeneratePath implements the PathStrategy interface.
func (s *PrefixSourceRegistryStrategy) GeneratePath(originalRef *image.Reference, targetRegistry string) (string, error) {
	if originalRef == nil {
		return "", fmt.Errorf("cannot generate path for nil reference")
	}

	debug.Printf("PrefixSourceRegistryStrategy: Generating path for original reference: %+v", originalRef)
	debug.Printf("PrefixSourceRegistryStrategy: Target registry: %s", targetRegistry)

	// Split repository into org/name parts
	repoPathParts := strings.SplitN(originalRef.Repository, "/", maxSplitTwo)

	// Always use the sanitized source registry name as the prefix
	pathPrefix := image.SanitizeRegistryForPath(originalRef.Registry)
	debug.Printf("PrefixSourceRegistryStrategy: Using sanitized source registry prefix '%s'", pathPrefix)

	// --- Base Repository Path Calculation (Keep existing logic) ---
	// Ensure we only use the repository path part, excluding any original registry prefix
	baseRepoPath := originalRef.Repository
	if len(repoPathParts) > 1 {
		if len(repoPathParts) > 1 && (strings.Contains(repoPathParts[0], ".") || strings.Contains(repoPathParts[0], ":") || repoPathParts[0] == "localhost") {
			// Heuristic: First part looks like a registry (contains '.' or ':'), so strip it.
			// This handles cases like "quay.io/prometheus/node-exporter"
			debug.Printf("PrefixSourceRegistryStrategy: Stripping potential registry prefix '%s' from repository path '%s'", repoPathParts[0], originalRef.Repository)
			baseRepoPath = strings.Join(repoPathParts[1:], "/")
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

	debug.Printf("FlatStrategy: Generating path for original reference: %+v", originalRef)
	debug.Printf("FlatStrategy: Target registry: %s", targetRegistry)

	// Use the original repository path directly
	baseRepoPath := originalRef.Repository
	debug.Printf("FlatStrategy: Using base repository path: %s", baseRepoPath)

	// NOTE: This function now ONLY returns the repository part (e.g., "library/nginx")
	// The caller (generator) is responsible for prepending the actual target registry.

	finalRepoPathPart := baseRepoPath // Return only the repo part

	// Remove logic that adds tag/digest here, caller handles it.

	debug.Printf("FlatStrategy: Generated final repo path part: %s", finalRepoPathPart)
	return finalRepoPathPart, nil
}
