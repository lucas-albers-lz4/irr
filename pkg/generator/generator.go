// Package generator orchestrates the process of loading charts, detecting images, and generating override files.
package generator

import (
	"fmt"
	"strings"

	"github.com/lalbers/irr/pkg/analyzer"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	"sigs.k8s.io/yaml"
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

// processImagePattern processes a single image pattern and updates the overrides map.
// Returns true if processing should continue, false if an error occurred and we should stop (in strict mode).
func (g *Generator) processImagePattern(pattern *analyzer.ImagePattern, generatedOverrides map[string]interface{}) bool {
	debug.Printf("Generator DEBUG: Processing pattern: Path='%s', RawPath='%s', Origin='%s', Value='%s', Type='%s', Structure=%+v",
		pattern.Path, pattern.RawPath, pattern.Origin, pattern.Value, pattern.Type, pattern.Structure)

	var originalRegistry, originalRepo, originalTag, originalDigest string
	var isMapBased bool
	var ref *image.Reference // Declare ref here to be accessible later
	var parseErr error       // Declare parseErr here

	if pattern.Type == "map" && pattern.Structure != nil {
		isMapBased = true
		originalRegistry = pattern.Structure.Registry
		originalRepo = pattern.Structure.Repository
		originalTag = pattern.Structure.Tag
		// ImageStructure has no Digest field, assume none for now
		originalDigest = "" // Assume no digest from structure for now
		debug.Printf("Generator: Using structured data for map pattern: Reg='%s', Repo='%s', Tag='%s'", originalRegistry, originalRepo, originalTag)

		// Reconstruct a canonical reference string for path strategy if needed,
		// ensuring Docker Hub normalization is applied if registry is empty.
		var registryForNorm string
		if originalRegistry != "" {
			registryForNorm = originalRegistry
		} else {
			// If original registry is empty, assume Docker Hub for normalization purposes
			// This logic might ideally live within NormalizeRegistry or a helper
			registryForNorm = "docker.io"
		}
		normalizedRegistry := image.NormalizeRegistry(registryForNorm) // Normalize the determined registry

		refString := normalizedRegistry
		if refString != "" {
			refString += "/"
		}
		refString += originalRepo
		if originalTag != "" {
			refString += ":" + originalTag
		} else if originalDigest != "" {
			// Digest handling might need refinement if added to Structure
			refString += "@" + originalDigest
		}

		ref, parseErr = image.ParseImageReference(refString) // Re-parse from structured data
		if parseErr != nil {
			log.Warnf("Could not re-parse reference from structured data for path '%s' ('%s'): %v. Skipping.", pattern.Path, refString, parseErr)
			return true // Continue in non-strict mode
		}
		// If originalDigest was not in Structure, ensure ref reflects that
		// Currently ref.Digest might be empty if tag was present.
		// If Structure had digest, we'd need to set it here.
		// originalDigest = ref.Digest // Update originalDigest if parsing found one not in Structure
		// This part is tricky without Digest in Structure.
	} else {
		// Fallback to parsing the Value string for non-map types or if Structure is nil
		ref, parseErr = image.ParseImageReference(pattern.Value)
		if parseErr != nil {
			logMsg := fmt.Sprintf("Skipping pattern at path '%s' due to unparseable value '%s': %v", pattern.Path, pattern.Value, parseErr)
			if g.StrictMode {
				// In strict mode, wrap the error and signal to stop processing
				err := fmt.Errorf("strict mode: %s: %w", logMsg, parseErr)
				log.Errorf(err.Error()) // Log the error before stopping
				return false            // Signal to stop processing
			}
			log.Warnf(logMsg)
			return true // Continue in non-strict mode
		}
		// Store original components from parsed string ref
		originalRegistry = ref.Registry
		// originalRepo = ref.Repository // Removed (staticcheck SA4006)
		originalTag = ref.Tag
		originalDigest = ref.Digest
	}

	// Ensure ref is not nil before proceeding
	if ref == nil {
		log.Warnf("Internal error: ref is nil after parsing checks for path '%s'. Skipping.", pattern.Path)
		return true // Continue in non-strict mode
	}

	// ---> START Source Registry Filtering <---
	if len(g.SourceRegistries) > 0 {
		isSource := false
		for _, sourceReg := range g.SourceRegistries {
			if image.NormalizeRegistry(ref.Registry) == image.NormalizeRegistry(sourceReg) {
				isSource = true
				break
			}
		}
		if !isSource {
			log.Debugf("Skipping pattern at path '%s': Registry '%s' not in source registries %v.", pattern.Path, ref.Registry, g.SourceRegistries)
			return true // Skip this pattern, continue processing others
		}
	}
	// ---> END Source Registry Filtering <---

	// Use the reliable ref.Registry obtained from either path above
	var mappedRegistry string
	if g.Mappings != nil {
		mappedRegistry = g.Mappings.GetTargetRegistry(ref.Registry)
		debug.Printf("Generator.processImagePattern: Looked up mapping for registry '%s', result: '%s'", ref.Registry, mappedRegistry)
	}

	// Generate the new repository path using the reliable ref
	newRepoPath, pathErr := g.PathStrategy.GeneratePath(ref, mappedRegistry)
	if pathErr != nil {
		logMsg := fmt.Sprintf("Error generating target path for image '%s' (registry: '%s'): %v", ref.Repository, ref.Registry, pathErr)
		if g.StrictMode {
			err := fmt.Errorf("strict mode: %s: %w", logMsg, pathErr)
			log.Errorf(err.Error())
			return false // Signal to stop processing
		}
		log.Warnf(logMsg + ". Skipping override.")
		return true // Continue in non-strict mode
	}

	// Set the override values
	if !g.setImageOverrideValues(generatedOverrides, pattern.Path, newRepoPath, mappedRegistry, originalRegistry, originalTag, originalDigest, isMapBased) {
		// An error occurred while setting values and we are in strict mode
		return false // Signal to stop processing
	}

	return true // Signal processing was successful for this pattern
}

// setImageOverrideValues sets the repository, registry, tag, and digest in the overrides map.
// Returns true on success, false if an error occurred (only relevant in strict mode).
func (g *Generator) setImageOverrideValues(overrides map[string]interface{}, patternPath, newRepoPath, mappedRegistry, originalRegistry, originalTag, originalDigest string, isMapBased bool) bool {
	log.Debugf("setImageOverrideValues START: patternPath='%s', newRepoPath='%s', mappedRegistry='%s'", patternPath, newRepoPath, mappedRegistry)
	basePathSegments := strings.Split(patternPath, ".")
	// Adjust base path if last segment is a common image key
	if len(basePathSegments) > 1 {
		lastKey := basePathSegments[len(basePathSegments)-1]
		if lastKey == "repository" || lastKey == "tag" || lastKey == "registry" || lastKey == "digest" {
			basePathSegments = basePathSegments[:len(basePathSegments)-1]
			log.Debugf("Adjusted base path for image components to: %v (original path: %s)", basePathSegments, patternPath)
		}
	}

	// Set repository
	repoPath := append(append([]string{}, basePathSegments...), "repository")
	log.Debugf("setImageOverrideValues: Setting repository at path: %v", repoPath)
	if err := override.SetValueAtPath(overrides, repoPath, newRepoPath, false); err != nil {
		log.Warnf("Error setting repository override at path '%s': %v. Skipping.", strings.Join(repoPath, "."), err)
		return true // Continue in non-strict mode
	}

	// Set registry
	if mappedRegistry != "" {
		regPath := append(append([]string{}, basePathSegments...), "registry")
		log.Debugf("setImageOverrideValues: Setting registry at path: %v", regPath)
		if err := override.SetValueAtPath(overrides, regPath, mappedRegistry, false); err != nil {
			log.Warnf("Error setting registry override at path '%s': %v. Skipping.", strings.Join(regPath, "."), err)
			return true // Continue in non-strict mode
		}
	} else if originalRegistry != "" {
		// Explicitly set the original registry if no mapping applied
		regPath := append(append([]string{}, basePathSegments...), "registry")
		log.Debugf("setImageOverrideValues: Setting original registry at path: %v", regPath)
		if err := override.SetValueAtPath(overrides, regPath, originalRegistry, false); err != nil {
			log.Warnf("Error setting original registry override at path '%s': %v. Skipping.", strings.Join(regPath, "."), err)
			return true // Continue in non-strict mode
		}
	}

	// Set tag or digest
	switch {
	case originalDigest != "":
		digestPath := append(append([]string{}, basePathSegments...), "digest")
		log.Debugf("setImageOverrideValues: Setting digest at path: %v", digestPath)
		if err := override.SetValueAtPath(overrides, digestPath, originalDigest, false); err != nil {
			log.Warnf("Error setting original digest override at path '%s': %v. Skipping.", strings.Join(digestPath, "."), err)
			return true // Continue in non-strict mode
		}
	case originalTag != "":
		if originalTag == "latest" && isMapBased {
			log.Debugf("Original tag is 'latest' and type was map for path '%s'. Skipping explicit tag override to allow Helm defaults.", patternPath)
		} else {
			tagPath := append(append([]string{}, basePathSegments...), "tag")
			log.Debugf("setImageOverrideValues: Setting tag at path: %v", tagPath)
			if err := override.SetValueAtPath(overrides, tagPath, originalTag, false); err != nil {
				log.Warnf("Error setting original tag override at path '%s': %v. Skipping.", strings.Join(tagPath, "."), err)
				return true // Continue in non-strict mode
			}
		}
	default:
		log.Debugf("Image pattern at path '%s' has neither original tag nor digest. Only repository/registry override will be set.", patternPath)
	}
	log.Debugf("setImageOverrideValues END: Successfully set overrides for patternPath '%s'", patternPath)
	return true // Success
}

// Generate produces the override YAML []byte based on provided image patterns and strategy.
func (g *Generator) Generate(imagePatterns []analyzer.ImagePattern) ([]byte, error) {
	debug.FunctionEnter("Generator.Generate")
	defer debug.FunctionExit("Generator.Generate")

	generatedOverrides := make(map[string]interface{})

	log.Debugf("Generator received %d image patterns to process.", len(imagePatterns))
	for i := range imagePatterns { // Use index to avoid copying large struct
		pattern := &imagePatterns[i]
		if !g.processImagePattern(pattern, generatedOverrides) {
			// processImagePattern returned false, meaning a critical error occurred in strict mode.
			// Return a generic error; the specific error was already logged.
			return nil, fmt.Errorf("strict mode: error processing image pattern at path '%s'", pattern.Path)
		}
	}

	// Apply specific normalization logic after general processing
	// Pass the input patterns to the normalizer
	normalizeKubeStateMetricsOverrides(imagePatterns, generatedOverrides, g.PathStrategy, g.Mappings)

	// Marshal the final override map to YAML bytes
	yamlBytes, err := yaml.Marshal(generatedOverrides)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal final overrides to YAML: %w", err)
	}

	return yamlBytes, nil
}

// normalizeKubeStateMetricsOverrides handles the special structure required for kube-state-metrics.
// It finds KSM images from the detected pattern list, creates the canonical override structure under
// the "kube-state-metrics" top-level key, and removes any potentially incorrect placements
// made by the generic override logic.
func normalizeKubeStateMetricsOverrides(
	imagePatterns []analyzer.ImagePattern,
	overrides map[string]interface{},
	p strategy.PathStrategy,
	m *registry.Mappings,
) {
	debug.FunctionEnter("normalizeKubeStateMetricsOverrides")
	defer debug.FunctionExit("normalizeKubeStateMetricsOverrides")

	var ksmImageOverride map[string]interface{}
	var ksmOriginalPrefixedPath string // Store the Path field from the pattern where KSM was originally detected
	var ksmParsedRef *image.Reference

	// Find the KSM image in the pattern list
	for i := range imagePatterns { // Use index to avoid copying large struct
		pattern := &imagePatterns[i]
		// Use Structure if available, otherwise parse Value
		var tempRef *image.Reference
		var err error
		if pattern.Type == "map" && pattern.Structure != nil {
			// Reconstruct ref string from structure for Contains check
			registryForNorm := pattern.Structure.Registry
			if registryForNorm == "" {
				registryForNorm = "docker.io" // Assume docker.io if empty for normalization
			}
			normReg := image.NormalizeRegistry(registryForNorm)
			refStr := normReg
			if refStr != "" {
				refStr += "/"
			}
			refStr += pattern.Structure.Repository
			// Tag/Digest not strictly needed for Contains check on repo
			tempRef, err = image.ParseImageReference(refStr)
		} else {
			tempRef, err = image.ParseImageReference(pattern.Value)
		}

		if err != nil {
			debug.Printf("normalizeKSM: Skipping pattern at path '%s', unparseable value '%s': %v", pattern.Path, pattern.Value, err)
			continue // Skip unparseable
		}

		// Identify KSM image (adjust pattern if needed)
		if strings.Contains(tempRef.Repository, "kube-state-metrics") {
			debug.Printf("Found potential KSM image: %s at path %s (origin: %s)", tempRef.String(), pattern.Path, pattern.Origin)
			ksmOriginalPrefixedPath = pattern.Path
			ksmParsedRef = tempRef // Use the parsed ref for override generation
			break                  // Assume only one KSM image needs this handling
		}
	}

	if ksmParsedRef != nil {
		// Generate the override value structure for KSM
		var mappedRegistry string
		if m != nil {
			mappedRegistry = m.GetTargetRegistry(ksmParsedRef.Registry)
		}
		newRepoPath, pathErr := p.GeneratePath(ksmParsedRef, mappedRegistry)
		if pathErr != nil {
			debug.Printf("Error generating path for KSM image %s: %v", ksmParsedRef.String(), pathErr)
			return // Cannot proceed if path generation fails
		}

		ksmImageOverride = map[string]interface{}{
			"repository": newRepoPath,
		}
		if mappedRegistry != "" {
			ksmImageOverride["registry"] = mappedRegistry
		}
		if ksmParsedRef.Digest != "" {
			ksmImageOverride["digest"] = ksmParsedRef.Digest
		} else if ksmParsedRef.Tag != "" { // Corrected: Use else if
			// Apply same 'latest' heuristic as in main processing loop?
			// Let's assume KSM usually has a specific tag/digest. If not, 'latest' might be ok here.
			ksmImageOverride["tag"] = ksmParsedRef.Tag
		}

		debug.Printf("Prepared KSM override block: %v", ksmImageOverride)

		// Construct the final KSM block
		finalKsmBlock := map[string]interface{}{"image": ksmImageOverride}

		// Check if a KSM block already exists (e.g., from original values)
		if existingKsmBlock, ok := overrides[KubeStateMetricsKey]; ok {
			debug.Printf("Found existing '%s' block: %v. Merging/overwriting.", KubeStateMetricsKey, existingKsmBlock)
			// Simple overwrite
		}

		// Set the canonical KSM block at the top level
		overrides[KubeStateMetricsKey] = finalKsmBlock
		debug.Printf("Set top-level '%s' override: %v", KubeStateMetricsKey, finalKsmBlock)

		// Remove the KSM entry from its original detected path, if it exists and differs from the top-level key
		originalPathSegments := strings.Split(ksmOriginalPrefixedPath, ".")
		if len(originalPathSegments) > 0 && originalPathSegments[0] != KubeStateMetricsKey {
			debug.Printf("Attempting to remove original KSM entry from path: %v", originalPathSegments)
			removeValueAtPath(overrides, originalPathSegments)
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
		debug.Printf("Removed key '%s' at final path segment.", key)
		return
	}

	if val, ok := data[key]; ok {
		if subMap, ok := val.(map[string]interface{}); ok {
			removeValueAtPath(subMap, path[1:])
			// If the subMap becomes empty after removal, remove the key itself
			if len(subMap) == 0 {
				delete(data, key)
				debug.Printf("Removed empty parent key '%s' after recursive removal.", key)
			}
		} else {
			debug.Printf("Cannot traverse path for removal: key '%s' does not contain a map at path %v", key, path)
		}
	} else {
		debug.Printf("Cannot traverse path for removal: key '%s' not found at path %v", key, path)
	}
}

// OverridesToYAML converts the generated override map to YAML.
// Deprecated: Use override.File.ToYAML() instead.
// func OverridesToYAML(overrides map[string]interface{}) ([]byte, error) {
// 	debug.Printf("Marshaling overrides to YAML")
// 	// Wrap error from external YAML library
// 	 yamlBytes, err := yaml.Marshal(overrides)
// 	 if err != nil {
// 		 return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
// 	 }
// 	 return yamlBytes, nil
// }

// Interface defines the methods expected from a generator.
// This interface now needs to match the refactored Generate signature.
type Interface interface {
	Generate(patterns []analyzer.ImagePattern) ([]byte, error)
}
