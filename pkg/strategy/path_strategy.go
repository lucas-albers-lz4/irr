package strategy

import (
	"fmt"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registry"
)

// PathStrategy defines the interface for transforming image paths
type PathStrategy interface {
	Transform(imgRef *image.ImageReference, targetRegistry string) string
}

// nolint:unused // Kept for potential future uses
var strategyRegistry = map[string]PathStrategy{
	"prefix-source-registry": NewPrefixSourceRegistryStrategy(nil),
}

// GetStrategy returns a path strategy based on the name
func GetStrategy(name string, mappings *registry.RegistryMappings) (PathStrategy, error) {
	debug.FunctionEnter("GetStrategy")
	defer debug.FunctionExit("GetStrategy")

	debug.Printf("Getting strategy for name: %s", name)

	switch name {
	case "prefix-source-registry":
		debug.Println("Using PrefixSourceRegistryStrategy")
		return NewPrefixSourceRegistryStrategy(mappings), nil
	default:
		debug.Printf("Unknown strategy name: %s", name)
		return nil, fmt.Errorf("unknown path strategy: %s", name)
	}
}

// PrefixSourceRegistryStrategy prefixes the source registry name to the repository path
type PrefixSourceRegistryStrategy struct {
	mappings *registry.RegistryMappings
}

// NewPrefixSourceRegistryStrategy creates a new PrefixSourceRegistryStrategy
func NewPrefixSourceRegistryStrategy(mappings *registry.RegistryMappings) *PrefixSourceRegistryStrategy {
	return &PrefixSourceRegistryStrategy{
		mappings: mappings,
	}
}

// Transform transforms the image path using the prefix-source-registry strategy
func (s *PrefixSourceRegistryStrategy) Transform(imgRef *image.ImageReference, targetRegistry string) string {
	// Get the sanitized source registry path component
	sourcePath := image.SanitizeRegistryForPath(imgRef.Registry)

	// Check if we have a mapping override
	if s.mappings != nil {
		for _, mapping := range s.mappings.Mappings {
			if mapping.Source == imgRef.Registry {
				// Use the explicit mapping target as the full path
				return fmt.Sprintf("%s/%s", mapping.Target, imgRef.Repository)
			}
		}
	}

	// No mapping, use default strategy: targetRegistry/sanitizedSource/repository
	// Split repository to remove any existing registry prefix
	repoPath := imgRef.Repository
	if strings.Contains(repoPath, targetRegistry) {
		repoPath = strings.TrimPrefix(repoPath, targetRegistry+"/")
	}

	if targetRegistry != "" {
		return fmt.Sprintf("%s/%s/%s", targetRegistry, sourcePath, repoPath)
	}
	return fmt.Sprintf("%s/%s", sourcePath, repoPath)
}
