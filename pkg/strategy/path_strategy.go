package strategy

import (
	"fmt"

	"github.com/lalbers/helm-image-override/pkg/debug"
	"github.com/lalbers/helm-image-override/pkg/image"
	"github.com/lalbers/helm-image-override/pkg/registry"
)

// PathStrategy defines the interface for transforming image paths
type PathStrategy interface {
	Transform(imgRef *image.ImageReference, targetRegistry string) string
}

// strategyRegistry holds the available strategies
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
	targetPath := image.SanitizeRegistryForPath(imgRef.Registry)
	if s.mappings != nil {
		for _, mapping := range s.mappings.Mappings {
			if mapping.Source == imgRef.Registry {
				targetPath = mapping.Target
				break
			}
		}
	}

	if targetRegistry != "" {
		return fmt.Sprintf("%s/%s/%s", targetRegistry, targetPath, imgRef.Repository)
	}
	return fmt.Sprintf("%s/%s", targetPath, imgRef.Repository)
}
