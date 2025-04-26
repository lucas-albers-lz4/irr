// Package generator orchestrates the process of loading charts, detecting images, and generating override files.
package generator

import (
	"fmt"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/override"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/lucas-albers-lz4/irr/pkg/strategy"
)

// KubeStateMetricsKey is the expected top-level key for kube-state-metrics overrides.
const KubeStateMetricsKey = "kube-state-metrics"

// Generator handles the generation of override files.
type Generator struct {
	Mappings          *registry.Mappings
	PathStrategy      strategy.PathStrategy
	SourceRegistries  []string
	ExcludeRegistries []string
	StrictMode        bool
	TemplateMode      bool
}

// NewGenerator creates a new Generator instance.
func NewGenerator(
	mappings *registry.Mappings,
	pathStrategy strategy.PathStrategy,
	sourceRegistries, excludeRegistries []string,
	strictMode, templateMode bool,
) *Generator {
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
// It performs the following steps:
// 1. Creates an image detection context and detector.
// 2. Calls the detector to find image references in the input values.
// 3. Checks for unsupported structures if strict mode is enabled.
// 4. Iterates through detected images:
//   - Skips images without valid references.
//   - Determines the target registry using mappings.
//   - Generates the new repository path using the path strategy.
//   - Creates the override map structure (registry, repository, tag/digest).
//   - Sets the override value at the correct path in the results map.
//
// 5. Applies special normalization for kube-state-metrics overrides.
// 6. Returns the generated map of overrides.
//
// chartName is currently unused but kept for potential future use (e.g., logging).
func (g *Generator) Generate(_ string, values map[string]interface{}) (map[string]interface{}, error) {
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
			// Use key-value pairs for logging
			log.Debug("Looked up mapping for registry", "registry", ref.Registry, "result", mappedRegistry)
		}

		// Generate the new repository path
		newRepoPath, pathErr := g.PathStrategy.GeneratePath(ref, mappedRegistry)
		if pathErr != nil {
			// Handle error: log, skip, or return error depending on policy
			// Use key-value pairs for logging
			log.Debug("Error generating path", "image", ref.String(), "error", pathErr)
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
			// Use key-value pairs for logging
			log.Debug("Error setting override", "path", detected.Path, "error", err)
			continue
		}
	}

	// Apply specific normalization logic after general processing
	normalizeKubeStateMetricsOverrides(detectedImages, generatedOverrides, g.PathStrategy, g.Mappings)

	return generatedOverrides, nil
}

// normalizeKubeStateMetricsOverrides handles the special structure required for kube-state-metrics.
// It finds KSM images from the detected list, creates the canonical override structure under
// the "kube-state-metrics" top-level key, and removes any potentially incorrect placements
// made by the generic override logic.
func normalizeKubeStateMetricsOverrides(
	detectedImages []image.DetectedImage,
	overrides map[string]interface{},
	p strategy.PathStrategy,
	m *registry.Mappings,
) {
	ksmImageOverride := make(map[string]interface{}) // To store the canonical KSM image block
	var ksmDetectedPath []string                     // Store the path where KSM was originally detected

	// Find the KSM image in the detected list
	for _, detected := range detectedImages {
		ref := detected.Reference
		if ref == nil {
			continue
		}

		// Identify KSM image (adjust pattern if needed)
		if strings.Contains(ref.Repository, "kube-state-metrics") {
			log.Debug("Found potential KSM image", ref.String(), "at path", detected.Path)

			// Generate the override value structure for KSM
			var mappedRegistry string
			if m != nil {
				mappedRegistry = m.GetTargetRegistry(ref.Registry)
			}
			newRepoPath, pathErr := p.GeneratePath(ref, mappedRegistry)
			if pathErr != nil {
				log.Debug("Error generating path for KSM image", ref.String(), "error", pathErr)
				continue // Skip if path generation fails
			}

			imageMap := map[string]interface{}{
				"registry":   mappedRegistry,
				"repository": newRepoPath,
			}
			if ref.Digest != "" {
				imageMap["digest"] = ref.Digest
			} else {
				imageMap["tag"] = ref.Tag
			}

			// Store the correctly structured override and the path it was found at
			ksmImageOverride = imageMap
			ksmDetectedPath = detected.Path
			log.Debug("Prepared KSM override block", ksmImageOverride)
			break // Assume only one KSM image needs this handling
		}
	}

	if len(ksmImageOverride) > 0 {
		// Construct the final KSM block
		finalKsmBlock := map[string]interface{}{"image": ksmImageOverride}

		// Check if a KSM block already exists (e.g., from original values)
		if existingKsmBlock, ok := overrides[KubeStateMetricsKey]; ok {
			log.Debug("Found existing", KubeStateMetricsKey, "block", existingKsmBlock, "merging/overwriting")
			// Simple overwrite, assuming our generated block is canonical.
			// More complex merging could be added here if needed.
		}

		// Set the canonical KSM block at the top level
		overrides[KubeStateMetricsKey] = finalKsmBlock
		log.Debug("Set top-level", KubeStateMetricsKey, "override", finalKsmBlock)

		// Remove the KSM entry from its original detected path, if it exists and differs from the top-level key
		if len(ksmDetectedPath) > 0 && ksmDetectedPath[0] != KubeStateMetricsKey {
			log.Debug("Attempting to remove original KSM entry from path", ksmDetectedPath)
			// We need a way to delete a value at a path. The override package might need a DeleteValueAtPath.
			// For now, we assume the SetValueAtPath in the main loop might have placed it.
			// Let's try removing it carefully.
			removeValueAtPath(overrides, ksmDetectedPath)
		}
	}
}

// removeValueAtPath recursively removes a value from a nested map based on a path.
// This is a helper function; ideally, this functionality might exist in the override package.
func removeValueAtPath(data map[string]interface{}, path []string) {
	if len(path) == 0 {
		return
	}

	key := path[0]

	if len(path) == 1 {
		delete(data, key)
		log.Debug("Removed key", key, "at final path segment")
		return
	}

	if val, ok := data[key]; ok {
		if subMap, ok := val.(map[string]interface{}); ok {
			removeValueAtPath(subMap, path[1:])
			// If the subMap becomes empty after removal, remove the key itself
			if len(subMap) == 0 {
				delete(data, key)
				log.Debug("Removed empty parent key", key, "after recursive removal")
			}
		} else {
			log.Debug("Cannot traverse path for removal: key", key, "does not contain a map at path", path)
		}
	} else {
		log.Debug("Cannot traverse path for removal: key", key, "not found at path", path)
	}
}

// OverridesToYAML converts the generated override map to YAML.
// Deprecated: Use override.File.ToYAML() instead.
// func OverridesToYAML(overrides map[string]interface{}) ([]byte, error) {
// 	log.Debug("Marshaling overrides to YAML")
// 	// Wrap error from external YAML library
// 	 yamlBytes, err := yaml.Marshal(overrides)
// 	 if err != nil {
// 		 return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
// 	 }
// 	 return yamlBytes, nil
// }

// Interface defines the methods expected from a generator.
type Interface interface {
	Generate(chartName string, values map[string]interface{}) (map[string]interface{}, error)
}
