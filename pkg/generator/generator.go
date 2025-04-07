// Package generator provides functionality for generating Helm override files.
package generator

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registrymapping"
	"github.com/lalbers/irr/pkg/strategy"
)

// Generator handles the generation of override files.
type Generator struct {
	Mappings          *registrymapping.RegistryMappings
	PathStrategy      strategy.PathStrategy
	SourceRegistries  []string
	ExcludeRegistries []string
	StrictMode        bool
	TemplateMode      bool
}

// NewGenerator creates a new Generator instance.
func NewGenerator(mappings *registrymapping.RegistryMappings, pathStrategy strategy.PathStrategy, sourceRegistries, excludeRegistries []string, strictMode, templateMode bool) *Generator {
	return &Generator{
		Mappings:          mappings,
		PathStrategy:      pathStrategy,
		SourceRegistries:  sourceRegistries,
		ExcludeRegistries: excludeRegistries,
		StrictMode:        strictMode,
		TemplateMode:      templateMode,
	}
}

// Generate produces the override values map based on detected images and strategy.
// chartName is currently unused but kept for potential future use (e.g., logging).
func (g *Generator) Generate(_ string, values map[string]interface{}) (map[string]interface{}, error) {
	debug.FunctionEnter("Generator.Generate")
	defer debug.FunctionExit("Generator.Generate")

	detectionContext := &image.DetectionContext{
		SourceRegistries:  g.SourceRegistries,
		ExcludeRegistries: g.ExcludeRegistries,
		Strict:            g.StrictMode,
		TemplateMode:      g.TemplateMode,
		// GlobalRegistry handling might need context here if applicable
	}
	detector := image.NewDetector(*detectionContext)

	detectedImages, unsupportedImages, err := detector.DetectImages(values, []string{})
	if err != nil {
		return nil, fmt.Errorf("error detecting images: %w", err)
	}

	if g.StrictMode && len(unsupportedImages) > 0 {
		return nil, fmt.Errorf("strict mode violation: %d unsupported structures found", len(unsupportedImages))
	}

	generatedOverrides := make(map[string]interface{})

	for _, detected := range detectedImages {
		ref := detected.Reference
		if ref == nil {
			continue // Skip if reference is nil
		}

		// Get target registry from mappings or default (empty)
		var mappedRegistry string
		if g.Mappings != nil {
			mappedRegistry = g.Mappings.GetTargetRegistry(ref.Registry)
			debug.Printf("Generator.Generate: Looked up mapping for registry '%s', result: '%s'", ref.Registry, mappedRegistry)
		}

		// Generate the new repository path
		newRepoPath, pathErr := g.PathStrategy.GeneratePath(ref, mappedRegistry)
		if pathErr != nil {
			// Handle error: log, skip, or return error depending on policy
			debug.Printf("Error generating path for %s: %v", ref.String(), pathErr)
			continue // Skip this image
		}

		// Create the override structure (always map)
		overrideValue := map[string]interface{}{
			"registry":   mappedRegistry,
			"repository": newRepoPath,
		}
		if ref.Digest != "" {
			overrideValue["digest"] = ref.Digest
		} else {
			overrideValue["tag"] = ref.Tag // Ensure tag is present
		}

		// Set the override value in the generated map
		err = override.SetValueAtPath(generatedOverrides, detected.Path, overrideValue)
		if err != nil {
			// Handle error: log, skip, or return error
			debug.Printf("Error setting override at path %v: %v", detected.Path, err)
			continue
		}
	}

	return generatedOverrides, nil
}

// OverridesToYAML converts the generated override map to YAML.
func OverridesToYAML(overrides map[string]interface{}) ([]byte, error) {
	return yaml.Marshal(overrides)
}

// Interface defines the methods expected from a generator.
type Interface interface {
	Generate(chartName string, values map[string]interface{}) (map[string]interface{}, error)
}
