package generator

import (
	"fmt"
	"path"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
)

// Generator is responsible for generating Helm override values.
type Generator struct {
	registryMappings  *registry.RegistryMappings
	pathStrategy      strategy.PathStrategy
	sourceRegistries  []string
	excludeRegistries []string
	strictMode        bool
	templateMode      bool
}

// NewGenerator creates a new Generator instance.
func NewGenerator(registryMappings *registry.RegistryMappings, pathStrategy strategy.PathStrategy, sourceRegistries []string, excludeRegistries []string, strictMode bool, templateMode bool) *Generator {
	return &Generator{
		registryMappings:  registryMappings,
		pathStrategy:      pathStrategy,
		sourceRegistries:  sourceRegistries,
		excludeRegistries: excludeRegistries,
		strictMode:        strictMode,
		templateMode:      templateMode,
	}
}

// Generate performs the image detection and override generation.
func (g *Generator) Generate(chartPath string, values map[string]interface{}) (map[string]interface{}, error) {
	debug.Println(">>> ENTERING generator.Generate <<<")
	debug.FunctionEnter("Generator.Generate")
	defer debug.FunctionExit("Generator.Generate")
	debug.Printf("Generating overrides for chart: %s", chartPath)

	detector := image.NewImageDetector(&image.DetectionContext{
		SourceRegistries:  g.sourceRegistries,
		ExcludeRegistries: g.excludeRegistries,
		Strict:            g.strictMode,
		TemplateMode:      g.templateMode,
	})

	detectedImages, unsupportedImages, err := detector.DetectImages(values, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to detect images: %w", err)
	}

	// --- Ensure this logging is present ---
	debug.Println("[DEBUG irr GEN] Raw detected images BEFORE filtering/processing:")
	debug.DumpValue("Detected Images", detectedImages)       // Use DumpValue
	debug.DumpValue("Unsupported Images", unsupportedImages) // Use DumpValue
	// --- End ensure logging ---

	// Process detected images
	processedCount := 0
	overrides := make(map[string]interface{}) // Initialize overrides map

	for _, detected := range detectedImages {
		ref := detected.Reference

		// Check if the image needs rewriting based on source/exclude registries
		if image.IsSourceRegistry(ref, g.sourceRegistries, g.excludeRegistries) {
			debug.Printf("[DEBUG irr GEN] Processing Eligible Image: Path=%v, OriginalRef=%s, Type=%s", detected.Path, ref.String(), detected.Pattern)

			// Determine the target registry using mappings
			targetRegistry := g.registryMappings.GetTargetRegistry(ref.Registry)
			if targetRegistry == "" {
				debug.Printf("[WARN irr GEN] Could not find mapping for source registry: %s. Skipping rewrite.", ref.Registry)
				continue // Skip this image if mapping is missing
			}

			// Generate the *repository path part* using the strategy
			newRepoPath, err := g.pathStrategy.GeneratePath(ref, targetRegistry, g.registryMappings)
			if err != nil {
				debug.Printf("[ERROR irr GEN] Error generating repo path for %v: %v. Skipping.", detected.Path, err)
				continue // Skip on error generating path
			}

			// --- Construct the final newValue (map or string) ---
			var newValue interface{}
			switch detected.Pattern {
			case image.PatternMap:
				// Original was likely map{...}
				// Recreate a map structure, conditionally adding digest or tag.
				newMap := map[string]interface{}{
					"registry":   targetRegistry,
					"repository": newRepoPath,
				}
				// Only add digest if it was valid and non-empty in the original reference
				if ref.Digest != "" && ref.Digest != "sha256:" { // Check against empty and just the prefix
					newMap["digest"] = ref.Digest
				} else if ref.Tag != "" {
					// If digest is invalid/empty, fall back to adding the tag if present
					newMap["tag"] = ref.Tag
				}
				newValue = newMap
				debug.Printf("Original was Map/Unknown. Setting value as map: %v", newMap)

			case image.PatternString:
				// Original was a string "registry/repo:tag" or "repo@digest"
				// Output the combined string reference, conditionally using digest or tag.
				newValueStr := path.Join(targetRegistry, newRepoPath) // Use path.Join for OS-agnostic paths
				// Only add digest if it was valid and non-empty in the original reference
				if ref.Digest != "" && ref.Digest != "sha256:" { // Check against empty and just the prefix
					newValueStr = fmt.Sprintf("%s@%s", newValueStr, ref.Digest)
				} else if ref.Tag != "" {
					// If digest is invalid/empty, fall back to adding the tag if present
					newValueStr = fmt.Sprintf("%s:%s", newValueStr, ref.Tag)
				}
				newValue = newValueStr // Store the string as the value
				debug.Printf("Original was String. Setting value as string: %s", newValueStr)

			default:
				debug.Printf("[ERROR irr GEN] Unsupported Pattern %s for path %v. Skipping.", detected.Pattern, detected.Path)
				processedCount-- // Decrement since we are skipping
				continue         // Skip to the next image
			}
			// --- End Construct newValue ---

			// Use the helper to set the value at the detected path
			debug.Printf("[DEBUG irr GEN] Value to set: %v", newValue) // Changed from DumpValue
			debug.Printf("[DEBUG irr GEN] Calling SetValueAtPath with Path: %v", detected.Path)
			if err := override.SetValueAtPath(overrides, detected.Path, newValue); err != nil {
				debug.Printf("[ERROR irr GEN] Failed to set value at path %v: %v", detected.Path, err)
				// Let strict mode check catch this failure
				continue
			}
			debug.Printf("[DEBUG irr GEN] SetValueAtPath successful for path: %v", detected.Path)
			processedCount++
		} else {
			debug.Printf("[DEBUG irr GEN] Skipping image (not in source registries or excluded): Path=%v, Ref=%s", detected.Path, ref.String())
		}
	}

	// Strict mode check
	if g.strictMode {
		totalEligible := 0
		for _, detected := range detectedImages {
			if image.IsSourceRegistry(detected.Reference, g.sourceRegistries, g.excludeRegistries) {
				totalEligible++
			}
		}

		if totalEligible > 0 && processedCount != totalEligible {
			percentage := 0.0
			if totalEligible > 0 {
				percentage = (float64(processedCount) / float64(totalEligible)) * 100
			}
			// Add processed/total count to error message for clarity
			return nil, fmt.Errorf("processing threshold not met: required 100%%, actual %.0f%% (%d/%d processed)", percentage, processedCount, totalEligible)
		}

		// Also check for unsupported images in strict mode
		if len(unsupportedImages) > 0 {
			debug.Printf("[WARN irr GEN] Strict mode enabled and unsupported image structures found:")
			for _, unsup := range unsupportedImages {
				debug.Printf("  - Path: %v, Type: %d, Error: %v", unsup.Location, unsup.Type, unsup.Error)
			}
			return nil, fmt.Errorf("%d unsupported image structures found in strict mode", len(unsupportedImages))
		}
	}

	return overrides, nil
}
