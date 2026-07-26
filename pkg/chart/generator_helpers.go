package chart

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	image "github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/keys"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/override"
)

// findUnsupportedPatterns identifies template expressions and other unsupported structures
// Reverting to original type signature based on linter feedback loop
func (g *Generator) findUnsupportedPatterns(patterns []analysis.ImagePattern) []override.UnsupportedStructure {
	// Revert to using override.UnsupportedStructure
	var unsupported []override.UnsupportedStructure
	for _, p := range patterns {
		// Basic check: Does the value contain template syntax?
		// Using p.Value for the check itself seems correct based on previous logic.
		// Corrected syntax: No escaping needed inside the string literal.
		if strings.Contains(p.Value, "{{") && strings.Contains(p.Value, "}}") {
			unsupported = append(unsupported, override.UnsupportedStructure{
				// Path comes from p.Path (string), split by '.'
				Path: strings.Split(p.Path, "."),
				// Type indicates the reason for being unsupported
				Type: "HelmTemplate",
			})
		}
		// Add more checks for other unsupported structures if needed
	}
	return unsupported
}

// filterEligibleImages identifies which detected image patterns should be processed based on source/exclude lists.
func (g *Generator) filterEligibleImages(detectedImages []analysis.ImagePattern) []analysis.ImagePattern {
	log.Debug("Enter filterEligibleImages")
	defer log.Debug("Exit filterEligibleImages")

	var eligibleImages []analysis.ImagePattern
	log.Debug("Filtering eligible images", "total_detected", len(detectedImages))

	// Pre-normalize source and exclude registries for efficiency
	normalizedSources := make(map[string]bool)
	for _, source := range g.sourceRegistries {
		normalizedSources[image.NormalizeRegistry(source)] = true
	}
	normalizedExcludes := make(map[string]bool)
	for _, exclude := range g.excludeRegistries {
		normalizedExcludes[image.NormalizeRegistry(exclude)] = true
	}
	log.Debug("Pre-normalized registries", "sources", normalizedSources, "excludes", normalizedExcludes)

	for i := range detectedImages {
		pattern := &detectedImages[i]
		// Handle potential errors during parsing more gracefully
		log.Debug("Filtering: Checking pattern", "path", pattern.Path, "value", pattern.Value)
		imgRef, err := g.processImagePattern(pattern)
		if err != nil {
			// If processing fails, skip this pattern for eligibility
			log.Debug("Filtering: Skipping pattern due to processing error", "path", pattern.Path, "error", err)
			continue
		}

		if imgRef == nil {
			// If imgRef is nil even without error (shouldn't happen ideally)
			log.Debug("Filtering: Skipping pattern due to nil imgRef", "path", pattern.Path)
			continue
		}

		// Perform checks using the pre-normalized maps
		normalizedReg := image.NormalizeRegistry(imgRef.Registry)
		isSource := normalizedSources[normalizedReg]
		isExcluded := normalizedExcludes[normalizedReg]
		log.Debug("Filtering: Registry checks", "path", pattern.Path, "registry", imgRef.Registry, "normalized", normalizedReg, "isSource", isSource, "isExcluded", isExcluded)

		if isSource && !isExcluded {
			// *** DEBUG ALIAS ***
			if pattern.Path == theAliasImagePath {
				log.Debug("ALIAS_DEBUG: Pattern MARKED as eligible", "path", pattern.Path)
			}
			eligibleImages = append(eligibleImages, *pattern)
			log.Debug("Filtering: Pattern added as eligible", "path", pattern.Path)
		} else {
			// *** DEBUG ALIAS ***
			if pattern.Path == theAliasImagePath {
				log.Warn("ALIAS_DEBUG: Pattern SKIPPED eligibility", "path", pattern.Path, "isSource", isSource, "isExcluded", isExcluded)
			}
			log.Debug("Filtering: Pattern skipped (not source or excluded)", "path", pattern.Path)
		}
	}

	log.Debug("Finished filtering images", "eligible_count", len(eligibleImages))
	return eligibleImages
}

// determineTargetPathAndRegistry uses the path strategy to determine the new path
// and target registry for the given image reference.
func (g *Generator) determineTargetPathAndRegistry(imgRef *image.Reference, _ *analysis.ImagePattern) (targetRegistry, newPath string, err error) {
	log.Debug("Enter determineTargetPathAndRegistry", "inputRegistry", imgRef.Registry, "inputRepository", imgRef.Repository)
	defer log.Debug("Exit determineTargetPathAndRegistry")

	// First check if we have a mapping for this registry
	effectiveTargetRegistry := g.targetRegistry
	mappedTarget := ""

	if g.mappings != nil {
		mappedTarget = g.mappings.GetTargetRegistry(imgRef.Registry)
		if mappedTarget != "" {
			log.Debug("Using mapped target registry", "source", imgRef.Registry, "target", mappedTarget)

			// If the mapped target contains a path, split it into registry and path
			if strings.Contains(mappedTarget, "/") {
				parts := strings.SplitN(mappedTarget, "/", MaxSplitParts)

				// Add length check to prevent panic and satisfy linter
				if len(parts) == 0 { // Check if split result is empty
					log.Error("SplitN resulted in empty slice unexpectedly", "mappedTarget", mappedTarget)
					return "", "", fmt.Errorf("internal error: failed to split mapped target registry path '%s'", mappedTarget)
				}

				// Now safe to access parts[0]
				effectiveTargetRegistry = parts[0]

				// For the path component, we have two options:
				// 1. If the target has structure registry.example.com/prefix, use the prefix as path prefix
				// 2. Otherwise generate a path using the strategy

				// This is case 1 - we have a path component in the mapping
				if len(parts) > 1 && parts[1] != "" {
					// Skip the path strategy for this case and directly construct the path
					// preserving the original repository structure
					finalPath := fmt.Sprintf("%s/%s", parts[1], imgRef.Repository)
					log.Debug("Using mapped target with path prefix directly",
						"registryPart", effectiveTargetRegistry,
						"pathPrefix", parts[1],
						"finalPath", finalPath)
					return effectiveTargetRegistry, finalPath, nil
				}
			}

			// If no path separator or empty path part, just use the mapped target as registry
			effectiveTargetRegistry = mappedTarget
			log.Debug("Using mapped target as registry", "effectiveTargetRegistry", effectiveTargetRegistry)
		} else {
			log.Debug("No mapping found for source registry, using CLI target",
				"sourceRegistry", imgRef.Registry,
				"cliTargetRegistry", g.targetRegistry)

			// Ensure we use the CLI-provided target registry when no mapping is found
			effectiveTargetRegistry = g.targetRegistry

			// Additional check to warn if CLI target is also empty
			if effectiveTargetRegistry == "" {
				log.Warn("No mapping found and no CLI target registry provided",
					"sourceRegistry", imgRef.Registry)
			}
		}
	} else {
		log.Debug("No mappings provided, using CLI target registry",
			"cliTargetRegistry", effectiveTargetRegistry)
	}

	// Call the path strategy to generate the new repository path
	log.Debug("Calling pathStrategy.GeneratePath",
		"strategy", fmt.Sprintf("%T", g.pathStrategy),
		"imgRef", imgRef,
		"effectiveTargetRegistry", effectiveTargetRegistry)

	newRepoPath, err := g.pathStrategy.GeneratePath(imgRef, effectiveTargetRegistry)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate path: %w", err)
	}
	log.Debug("Path strategy generated path", "newRepoPath", newRepoPath)

	log.Debug("Determined target registry and path",
		"finalTargetReg", effectiveTargetRegistry,
		"finalNewRepoPath", newRepoPath)

	return effectiveTargetRegistry, newRepoPath, nil
}

// processImage handles the processing of a single eligible image pattern.
// NOTE: This function is currently unused and commented out to satisfy the linter.
// It's kept for reference in case functionality needs to be restored in the future.
/*
func (g *Generator) processImage(pattern *analysis.ImagePattern, overrides map[string]interface{}) (bool, *override.UnsupportedStructure, error) {
	log.Debug("Enter processImage", "path", pattern.Path, "value", pattern.Value)
	// *** DEBUG ALIAS ***
	if pattern.Path == theAliasImagePath {
		log.Debug("ALIAS_DEBUG: Enter processImage", "path", pattern.Path)
	}
	defer func() {
		// *** DEBUG ALIAS ***
		if pattern.Path == theAliasImagePath {
			log.Debug("ALIAS_DEBUG: Exit processImage", "path", pattern.Path)
		}
		log.Debug("Exit processImage", "path", pattern.Path) // Keep original exit log
	}()

	// Parse the image reference string
	imgRef, err := g.processImagePattern(pattern)
	if err != nil {
		log.Warn("Failed to parse image pattern", "path", pattern.Path, "value", pattern.Value, "error", err)
		return false, &override.UnsupportedStructure{
			Path: strings.Split(pattern.Path, "."),
			Type: "InvalidImageFormat",
		}, err
	}
	if imgRef == nil {
		// This case should ideally be prevented by error handling in processImagePattern
		log.Error("processImagePattern returned nil imgRef without error", "path", pattern.Path, "value", pattern.Value)
		return false, nil, errors.New("internal error: processImagePattern returned nil without error")
	}

	log.Debug("Parsed image reference", "path", pattern.Path, "registry", imgRef.Registry, "repository", imgRef.Repository, "tag", imgRef.Tag, "digest", imgRef.Digest)

	// Determine the target registry and new path using the strategy and mappings
	targetReg, newPath, err := g.determineTargetPathAndRegistry(imgRef, pattern)
	if err != nil {
		log.Error("Failed to determine target path and registry", "path", pattern.Path, "error", err)
		// Wrap the error for context
		return false, nil, fmt.Errorf("error determining target path for %s: %w", pattern.Path, err)
	}

	log.Debug("Determined target", "path", pattern.Path, "targetRegistry", targetReg, "newPath", newPath)

	// Create the override structure (map)
	overrideValue := g.createOverride(pattern, imgRef, targetReg, newPath)
	log.Debug("Created override value structure", "path", pattern.Path, "overrideValue", overrideValue)

	// *** Add explicit type check ***
	if overrideMap, ok := overrideValue.(map[string]interface{}); ok {
		if repoVal, repoOk := overrideMap[keys.Repository]; repoOk {
			log.Debug("Type check BEFORE setOverridePath", "path", pattern.Path, "repo_type", fmt.Sprintf("%T", repoVal))
		} else {
			log.Warn("Repository key missing in overrideValue BEFORE setOverridePath", "path", pattern.Path)
		}
	} else {
		log.Warn("overrideValue is not a map BEFORE setOverridePath", "path", pattern.Path, "type", fmt.Sprintf("%T", overrideValue))
	}
	// *** End type check ***

	// *** Log path being used for setOverridePath ***
	log.Debug("Calling setOverridePath with path", "patternPath", pattern.Path)

	// Set the override value in the main overrides map
	if err := g.setOverridePath(overrides, pattern, overrideValue); err != nil {
		log.Error("Failed to set override path in map", "path", pattern.Path, "error", err)
		// Wrap the error for context
		return false, nil, fmt.Errorf("error setting override for %s: %w", pattern.Path, err)
	}

	log.Info("Successfully processed image override", "path", pattern.Path, "original", pattern.Value, "new_repo", newPath, "target_registry", targetReg)
	return true, nil, nil // Processed successfully, no unsupported structure error originated here
}
*/

// FailedItem struct definition remains the same
type FailedItem struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// processEligibleImagesLoop iterates through eligible images, processes them, and collects results.
// NOTE: This function is currently unused and commented out to satisfy the linter.
// It's kept for reference in case functionality needs to be restored in the future.
/*
func (g *Generator) processEligibleImagesLoop(eligibleImages []analysis.ImagePattern, overrides map[string]interface{}) (processingErrors []error, processedCount int) {
	// Initialize local slices/maps if needed (overrides is passed in)
	if overrides == nil {
		overrides = make(map[string]interface{}) // Should ideally not happen if called from Generate
		log.Warn("Overrides map was nil in processEligibleImagesLoop, re-initialized")
	}
	// Log map address inside loop start
	log.Debug("processEligibleImagesLoop: Map address at START", "map_addr", fmt.Sprintf("%p", overrides))

	processingErrors = []error{}
	processedCount = 0

	for i := range eligibleImages {
		pattern := &eligibleImages[i]
		processed, unsupported, err := g.processImage(pattern, overrides) // PASS local overrides map
		switch {
		case err != nil:
			log.Warn("Error processing image pattern", "path", pattern.Path, "error", err)
			wrappedErr := fmt.Errorf("path '%s': %w", pattern.Path, err)
			processingErrors = append(processingErrors, wrappedErr)
		case unsupported != nil:
			log.Warn("Unsupported structure detected", "path", pattern.Path, "type", unsupported.Type, "value", pattern.Value)
			// Handle strict mode: add error
			if g.strict {
				strictErr := fmt.Errorf("path '%s': %w (type: %s)", pattern.Path, ErrUnsupportedStructure, unsupported.Type)
				processingErrors = append(processingErrors, strictErr)
			}
		case processed:
			processedCount++
		}

		// Log current state of overrides map keys after each pattern
		currentKeys := []string{}
		for k := range overrides { // overrides is the map being modified
			currentKeys = append(currentKeys, k)
		}
		log.Debug("processEligibleImagesLoop: Keys in overrides map after processing path", "processedPath", pattern.Path, "keys", currentKeys, "map_addr", fmt.Sprintf("%p", overrides))
	}
	// Log map address inside loop end
	log.Debug("processEligibleImagesLoop: Map address at END", "map_addr", fmt.Sprintf("%p", overrides))
	return processingErrors, processedCount
}
*/

// checkProcessingThreshold evaluates if the processing met the required threshold.
func (g *Generator) checkProcessingThreshold(processingErrors []error, processedCount, eligibleCount int, successRate float64, _ *override.File) error {
	// Return specific error immediately if in strict mode and errors occurred
	if g.strict && len(processingErrors) > 0 {
		return &ProcessingError{
			Errors: processingErrors,
			Count:  len(processingErrors),
		}
	}

	// Check threshold
	if g.threshold > 0 && int(successRate) < g.threshold {
		log.Warn("Generator success rate below threshold", "rate", fmt.Sprintf("%.2f%%", successRate), "threshold", g.threshold)
		combinedErr := fmt.Errorf("processing errors: %d", len(processingErrors))
		if len(processingErrors) > 0 {
			var errStrings []string
			for _, e := range processingErrors {
				errStrings = append(errStrings, e.Error())
			}
			combinedErr = fmt.Errorf("processing errors: %s", strings.Join(errStrings, "; "))
		}
		// Return threshold error (non-fatal, allows returning partial result)
		return &ThresholdError{
			Threshold:   g.threshold,
			ActualRate:  int(successRate),
			Eligible:    eligibleCount,
			Processed:   processedCount,
			Err:         combinedErr,
			WrappedErrs: processingErrors,
		}
	}
	return nil
}

// applyRulesIfNeeded applies modification rules if they are enabled.
func (g *Generator) applyRulesIfNeeded(loadedChart *chart.Chart, result *override.File) error {
	if !g.rulesEnabled {
		return nil
	}

	log.Debug("Applying rules", "chart_path", g.chartPath)
	if g.rulesRegistry == nil {
		log.Warn("Rules are enabled but rules registry is nil. Skipping rule application.")
		return nil // Or return an error if this state is invalid
	}

	modified, err := g.rulesRegistry.ApplyRules(loadedChart, result.Values)
	if err != nil {
		log.Error("Error applying rules", "chart_path", g.chartPath, "error", err)
		return fmt.Errorf("failed to apply rules to chart %s: %w", g.chartPath, err)
	}
	if modified {
		log.Debug("Rules modified overrides", "chart_path", g.chartPath)
	} else {
		log.Debug("Rules applied successfully (no changes)", "chart_path", g.chartPath)
	}
	return nil
}

// ensureGlobalImageRegistry sets the global.imageRegistry field in the overrides.
// It now uses details from processed images to determine the most appropriate global registry.
func (g *Generator) ensureGlobalImageRegistry(overrides map[string]interface{}, _ []analysis.GlobalPattern, processedDetails []ProcessedImageDetail) {
	log.Debug("Enter ensureGlobalImageRegistry")
	defer log.Debug("Exit ensureGlobalImageRegistry")

	if len(processedDetails) == 0 {
		log.Debug("No processed images, global.imageRegistry will not be set by this function.")
		return
	}

	uniqueTargetRegistries := make(map[string]bool)
	for _, detail := range processedDetails {
		if detail.FinalTargetRegistry != "" {
			uniqueTargetRegistries[detail.FinalTargetRegistry] = true
		}
	}

	var finalGlobalRegistry string
	switch {
	case len(uniqueTargetRegistries) == 1:
		// If all processed images were mapped to a single, consistent registry, use that.
		for reg := range uniqueTargetRegistries { // Get the single key
			finalGlobalRegistry = reg
			break
		}
		log.Debug("Using unique target registry from processed images for global.imageRegistry", "registry", finalGlobalRegistry)
	case len(uniqueTargetRegistries) > 1:
		// Multiple different target registries were used. Fallback to CLI --target-registry.
		log.Debug("Multiple target registries used for processed images. Falling back to CLI --target-registry for global.imageRegistry.", "cliTarget", g.targetRegistry)
		finalGlobalRegistry = g.targetRegistry
	default:
		// No specific target registries were derived from mappings (e.g., all unmapped and processed with CLI target).
		// Or, all FinalTargetRegistry fields were empty (should not happen for processed images).
		// Fallback to CLI --target-registry.
		log.Debug("No unique target registry derivable from processed image mappings. Falling back to CLI --target-registry for global.imageRegistry.", "cliTarget", g.targetRegistry)
		finalGlobalRegistry = g.targetRegistry
	}

	if finalGlobalRegistry == "" {
		log.Debug("Final determined global registry is empty, global.imageRegistry will not be set.")
		return
	}

	if _, exists := overrides["global"]; !exists {
		overrides["global"] = make(map[string]interface{})
		log.Debug("Created 'global' key in overrides map")
	}

	globalOverrides, ok := overrides["global"].(map[string]interface{})
	if !ok {
		log.Warn("'global' key in overrides is not a map, cannot set imageRegistry", "type", reflect.TypeOf(overrides["global"]))
		return
	}

	if _, ok := globalOverrides["imageRegistry"]; !ok {
		globalOverrides["imageRegistry"] = finalGlobalRegistry
		log.Debug("Set global.imageRegistry", "registry", finalGlobalRegistry)
	} else {
		log.Debug("global.imageRegistry already exists", "value", globalOverrides["imageRegistry"])
	}
}

// findValueByPath traverses a nested map using a slice of path segments
// and returns the value found at that path.
// It returns the value and a boolean indicating if the path was found.
func findValueByPath(data map[string]interface{}, pathElems []string) (interface{}, bool) {
	current := interface{}(data)
	for i, part := range pathElems { // Keep index i for potential error messages
		mapData, ok := current.(map[string]interface{})
		if !ok {
			log.Debug("findValueByPath: Cannot traverse non-map value", "path_segment_index", i, "path_part", part, "current_type", fmt.Sprintf("%T", current))
			return nil, false // Path segment does not lead to a map
		}
		value, exists := mapData[part]
		if !exists {
			log.Debug("findValueByPath: Key not found", "path_segment_index", i, "path_part", part)
			return nil, false // Key not found at this level
		}
		current = value
	}
	return current, true
}

// OverridesToYAML marshals the provided overrides map into YAML format.
func OverridesToYAML(overrides map[string]interface{}) ([]byte, error) {
	log.Debug("Marshaling overrides to YAML")
	yamlBytes, err := yaml.Marshal(overrides)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
	}
	return yamlBytes, nil
}

// createOverride constructs the override value based on the detected pattern type.
// For map patterns, it creates a map with registry, repository, and tag.
// For string patterns, it creates the full image reference string.
func (g *Generator) createOverride(pattern *analysis.ImagePattern, imgRef *image.Reference, targetReg, newPath string) interface{} {
	log.Debug("Enter createOverride",
		"path", pattern.Path,
		"sourceOrigin", pattern.SourceOrigin,
		"registry", imgRef.Registry,
		"repository", imgRef.Repository,
		"tag", imgRef.Tag, // Log the tag we received
		"digest", imgRef.Digest,
		"targetReg", targetReg,
		"newPath", newPath)
	defer log.Debug("Exit createOverride")

	// Special case: global.imageRegistry should just be a string value containing the target registry,
	// not a full image override map. This ensures it maintains the correct type of a simple string value
	// that templates can use like {{ .Values.global.imageRegistry }}.
	// This special handling applies regardless of the pattern type (global or regular image).
	if pattern.Path == "global.imageRegistry" {
		log.Debug("Handling global.imageRegistry - returning registry string only", "registry", targetReg)
		return targetReg
	}

	// Determine the final image reference string parts
	finalRepository := newPath // The strategy provides the full repository path within the target

	// Determine which tag/digest to use
	finalTag := imgRef.Tag
	finalDigest := imgRef.Digest
	log.Debug("createOverride: Initial tag", "tag", finalTag)

	// Only use AppVersion if tag is empty
	if finalTag == "" && pattern.SourceChartAppVersion != "" {
		log.Debug("Tag is empty, using source chart AppVersion", "appVersion", pattern.SourceChartAppVersion)
		finalTag = pattern.SourceChartAppVersion
	}

	// Construct the override structure
	// This assumes the standard {registry: ..., repository: ..., tag: ...} structure.
	// Adapt if different structures are needed based on chart conventions.
	overrideMap := map[string]interface{}{
		keys.Registry:   targetReg,
		keys.Repository: finalRepository,
	}

	// Only include the tag field in the map if finalTag is not empty
	if finalTag != "" {
		log.Debug("Including tag in override map", "tag", finalTag)
		overrideMap[keys.Tag] = finalTag
	} else {
		log.Debug("Omitting tag from override map as it's empty (either originally or after fallback logic).", "path", pattern.Path)
	}

	// Preserve/add pullPolicy if original pattern indicates a map structure
	if pattern.Structure != nil || pattern.Type == analysis.PatternTypeMap {
		pullPolicy := keys.IfNotPresent // Default pull policy
		if pattern.Structure != nil {
			if pp, ok := pattern.Structure["pullPolicy"].(string); ok && pp != "" {
				pullPolicy = pp // Use original pullPolicy if found
				log.Debug("Preserving original pullPolicy from structure", "pullPolicy", pullPolicy)
			}
		}
		log.Debug("Including pullPolicy in override map", "pullPolicy", pullPolicy)
		overrideMap["pullPolicy"] = pullPolicy
	} else {
		log.Debug("Original pattern was likely a string, not including pullPolicy in override map")
	}

	// TODO: Decide if/how to handle digest overrides. Currently omitted.
	if finalDigest != "" {
		log.Warn("Digest found but override logic currently omits it", "path", pattern.Path, "digest", finalDigest)
	}

	log.Debug("Returning override structure", "overrideMap", overrideMap)
	// *** Add final check inside createOverride ***
	if repoVal, ok := overrideMap[keys.Repository]; ok {
		log.Debug("Final check createOverride", "path", pattern.Path, "repo_type", fmt.Sprintf("%T", repoVal), "repo_value", repoVal)
	} else {
		log.Warn("Final check createOverride: Repository key missing", "path", pattern.Path)
	}
	// *** End final check ***
	return overrideMap
}

// Helper function (assuming not already present)
func mapKeys(m map[string]interface{}) []string {
	keyList := make([]string, 0, len(m))
	for k := range m {
		keyList = append(keyList, k)
	}
	return keyList
}

// setOverridePath sets the value at the specified path within the overrides map.
// It handles creating nested maps and arrays as needed.
func (g *Generator) setOverridePath(overrides map[string]interface{}, pattern *analysis.ImagePattern, value interface{}) error {
	path := pattern.Path
	pathElems := strings.Split(path, ".")
	log.Debug("setOverridePath: START", "path", path, "elements", pathElems, "valueType", fmt.Sprintf("%T", value))

	// Defensive check: Ensure pathElems is not empty, although Split usually returns [""] for empty path.
	if len(pathElems) == 0 {
		log.Error("Internal error: Path split resulted in empty slice", "path", path)
		return fmt.Errorf("internal error: cannot process empty path elements for path '%s'", path)
	}

	currentMap := overrides
	// Traverse path until the second-to-last element, creating maps if necessary
	for i := 0; i < len(pathElems)-1; i++ {
		key := pathElems[i]
		log.Debug("setOverridePath: Traversing", "key", key)

		// Check if this is an array index path (e.g., "containers[0]")
		if strings.Contains(key, "[") && strings.Contains(key, "]") {
			openBracketIndex := strings.Index(key, "[")
			closeBracketIndex := strings.Index(key, "]")

			// Verify indices are valid (openBracketIndex != -1 && closeBracketIndex > openBracketIndex)
			if openBracketIndex == -1 || closeBracketIndex <= openBracketIndex {
				return fmt.Errorf("malformed array index notation in path %s", path)
			}

			arrayKey := key[:openBracketIndex]
			indexStr := key[openBracketIndex+1 : closeBracketIndex]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return fmt.Errorf("invalid array index in path %s: %w", path, err)
			}

			// Get or create the array
			var arr []interface{}
			if existingArr, ok := currentMap[arrayKey]; ok {
				if arr, ok = existingArr.([]interface{}); !ok {
					// Key exists but is not an array, create new array
					arr = make([]interface{}, index+1)
					currentMap[arrayKey] = arr
				}
			} else {
				// Create new array with enough capacity
				arr = make([]interface{}, index+1)
				currentMap[arrayKey] = arr
			}

			// Ensure array has enough capacity
			if index >= len(arr) {
				newArr := make([]interface{}, index+1)
				copy(newArr, arr)
				arr = newArr
				currentMap[arrayKey] = arr
			}

			// Create or get the map at the array index
			if arr[index] == nil {
				arr[index] = make(map[string]interface{})
			}
			if nextMap, ok := arr[index].(map[string]interface{}); ok {
				currentMap = nextMap
			} else {
				// Replace with a new map if not a map
				newMap := make(map[string]interface{})
				arr[index] = newMap
				currentMap = newMap
			}
		} else {
			// Regular map path handling
			if nextLevel, ok := currentMap[key]; ok {
				// Key exists, check if it's a map
				if nextMap, ok := nextLevel.(map[string]interface{}); ok {
					currentMap = nextMap // Move deeper
					log.Debug("setOverridePath: Moved into existing map", "key", key)
				} else {
					// Key exists but is not a map. We need to replace it to continue traversal.
					log.Warn("setOverridePath: Overwriting existing non-map value with map to continue traversal", "key", key, "existingType", fmt.Sprintf("%T", nextLevel))
					newMap := make(map[string]interface{})
					currentMap[key] = newMap
					currentMap = newMap
				}
			} else {
				// Key doesn't exist, create a new map
				log.Debug("setOverridePath: Creating new map for key", "key", key)
				newMap := make(map[string]interface{})
				currentMap[key] = newMap
				currentMap = newMap // Move deeper
			}
		}
	}

	// Set the final value at the last key
	// Check again if pathElems is empty before accessing the last element (already covered by the check above, but kept for clarity)
	if len(pathElems) == 0 {
		// This should be unreachable due to the check after Split
		return fmt.Errorf("internal error: path elements became empty unexpectedly for path '%s'", path)
	}
	finalKey := pathElems[len(pathElems)-1]

	// Handle array index in the final key
	if strings.Contains(finalKey, "[") && strings.Contains(finalKey, "]") {
		openBracketIndex := strings.Index(finalKey, "[")
		closeBracketIndex := strings.Index(finalKey, "]")

		// Verify indices are valid (openBracketIndex != -1 && closeBracketIndex > openBracketIndex)
		if openBracketIndex == -1 || closeBracketIndex <= openBracketIndex {
			return fmt.Errorf("malformed array index notation in final key %s", finalKey)
		}

		arrayKey := finalKey[:openBracketIndex]
		indexStr := finalKey[openBracketIndex+1 : closeBracketIndex]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			return fmt.Errorf("invalid array index in final key %s: %w", finalKey, err)
		}

		// Get or create the array
		var arr []interface{}
		if existingArr, ok := currentMap[arrayKey]; ok {
			if arr, ok = existingArr.([]interface{}); !ok {
				arr = make([]interface{}, index+1)
				currentMap[arrayKey] = arr
			}
		} else {
			arr = make([]interface{}, index+1)
			currentMap[arrayKey] = arr
		}

		// Ensure array has enough capacity
		if index >= len(arr) {
			newArr := make([]interface{}, index+1)
			copy(newArr, arr)
			arr = newArr
			currentMap[arrayKey] = arr
		}

		// Set the value at the array index
		arr[index] = value
	} else {
		// Regular key handling
		log.Debug("setOverridePath: Setting final value", "finalKey", finalKey, "value", value, "parentMapKeys", mapKeys(currentMap))
		currentMap[finalKey] = value
	}

	log.Debug("setOverridePath: END", "path", path)
	return nil
}

// processImagePattern extracts image details using the image package.
// Logs errors internally but returns them for the caller to decide action.
func (g *Generator) processImagePattern(pattern *analysis.ImagePattern) (*image.Reference, error) {
	log.Debug("Enter processImagePattern", "path", pattern.Path, "value", pattern.Value)
	defer log.Debug("Exit processImagePattern")

	// ParseImageReference handles normalization internally
	imgRef, err := image.ParseImageReference(pattern.Value)
	if err != nil {
		log.Error("Failed to parse image reference", "path", pattern.Path, "value", pattern.Value, "error", err)
		// Return the error to be handled by the caller (processImage)
		return nil, fmt.Errorf("parsing image '%s' at path '%s': %w", pattern.Value, pattern.Path, err)
	}

	log.Debug("Successfully parsed image reference", "ref", imgRef)
	return imgRef, nil
}

// SetOverridePath sets a value at a given path in the override map, creating intermediate maps as needed.
// This is an exported version of setOverridePath to enable testing.
func (g *Generator) SetOverridePath(overrides map[string]interface{}, pattern *analysis.ImagePattern, value interface{}) error {
	return g.setOverridePath(overrides, pattern, value)
}
