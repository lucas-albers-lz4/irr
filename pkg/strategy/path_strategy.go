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
	// MaxSplitTwo is used when splitting strings into at most two parts
	MaxSplitTwo = 2
)

// PathStrategy defines the interface for generating new image paths.
type PathStrategy interface {
	// GeneratePath takes an original image reference and the target registry (if any),
	// and returns the new repository path (e.g., "new-registry/my-app").
	GeneratePath(originalRef *image.Reference, targetRegistry string) (string, error)
}

//nolint:unused // Kept for potential future uses
var strategyRegistry = map[string]PathStrategy{
	"prefix-source-registry": NewPrefixSourceRegistryStrategy(),
	"flat":                   NewFlatStrategy(),
}

// GetStrategy returns a path strategy based on the name
func GetStrategy(name string, _ *registry.Mappings) (PathStrategy, error) {
	debug.Printf("GetStrategy: Getting strategy for name: %s", name)

	switch name {
	case "prefix-source-registry":
		debug.Printf("GetStrategy: Using PrefixSourceRegistryStrategy")
		return NewPrefixSourceRegistryStrategy(), nil
	case "flat":
		debug.Printf("GetStrategy: Using FlatStrategy")
		return NewFlatStrategy(), nil
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

	// Always use the sanitized source registry name as the prefix
	// For an input like docker.io/nginx, originalRef.Registry is "docker.io", originalRef.Repository is "nginx"
	// For an input like quay.io/prometheus/node-exporter, originalRef.Registry is "quay.io", originalRef.Repository is "prometheus/node-exporter"
	// However, sometimes the parser might put the registry in the Repository field, e.g. originalRef.Repository could be "quay.io/prometheus/node-exporter"
	pathPrefix := image.SanitizeRegistryForPath(originalRef.Registry)
	debug.Printf("PrefixSourceRegistryStrategy: Using sanitized source registry prefix '%s'", pathPrefix)

	// Use the repository path directly from the parsed reference
	baseRepoPath := originalRef.Repository
	debug.Printf("PrefixSourceRegistryStrategy: Using base repository path: %s", baseRepoPath)

	// Ensure the baseRepoPath doesn't redundantly start with the original registry hostname.
	// The image parser should ideally separate registry and repository cleanly,
	// but sometimes the registry ends up in the repository part.
	originalRegistryPrefix := originalRef.Registry + "/"
	if strings.HasPrefix(baseRepoPath, originalRegistryPrefix) {
		baseRepoPath = strings.TrimPrefix(baseRepoPath, originalRegistryPrefix)
		debug.Printf("PrefixSourceRegistryStrategy: Stripped original registry prefix from base path, new base path: %s", baseRepoPath)
	}

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

	// Use the original repository path
	baseRepoPath := originalRef.Repository
	debug.Printf("FlatStrategy: Using base repository path: %s", baseRepoPath)

	// Handle Docker Hub official images (add library prefix if needed)
	if (image.NormalizeRegistry(originalRef.Registry) == "docker.io") && !strings.Contains(baseRepoPath, "/") {
		debug.Printf("FlatStrategy: Prepending 'library-' to Docker Hub image path: %s", baseRepoPath)
		baseRepoPath = "library-" + baseRepoPath
	} else {
		// Replace all slashes with dashes to flatten the path
		baseRepoPath = strings.ReplaceAll(baseRepoPath, "/", "-")
		debug.Printf("FlatStrategy: Flattened path: %s", baseRepoPath)
	}

	// Add registry prefix for better organization (optional but recommended)
	registryPrefix := image.SanitizeRegistryForPath(originalRef.Registry)
	finalRepoPathPart := registryPrefix + "-" + baseRepoPath
	debug.Printf("FlatStrategy: Final flattened path: %s", finalRepoPathPart)

	return finalRepoPathPart, nil
}
