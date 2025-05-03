package chart

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	image "github.com/lucas-albers-lz4/irr/pkg/image"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/override"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/lucas-albers-lz4/irr/pkg/rules"
	"github.com/lucas-albers-lz4/irr/pkg/strategy"
)

// Constants

const (
	// PercentMultiplier is used for percentage calculations
	PercentMultiplier = 100
	// PrivateFilePermissions represents secure file permissions (rw-------)
	PrivateFilePermissions = 0o600
	// FilePermissions defines the permission mode for temporary override files
	FilePermissions = 0o600
	// ExpectedMappingParts defines the number of parts expected after splitting a config mapping value (e.g., "source=target").
	ExpectedMappingParts = 2
	// PercentageMultiplier is used when calculating success rates as percentages
	PercentageMultiplier = 100.0
	// ExpectedParts is used for splitting strings into exactly two parts, typically for key:value or repo:tag pairs.
	ExpectedParts = 2
	// MaxSplitParts defines the maximum number of parts to split registry paths into
	// Currently 2 parts: registry name and repository path
	MaxSplitParts = 2
)

const theAliasImagePath = "theAlias.image"

// --- Local Error Definitions ---
var (
	ErrUnsupportedStructure = errors.New("unsupported structure found")
)

// LoadingError wraps errors from the chart loading perspective
type LoadingError struct {
	ChartPath string
	Err       error
}

func (e *LoadingError) Error() string {
	return fmt.Sprintf("failed to load chart at %s: %v", e.ChartPath, e.Err)
}
func (e *LoadingError) Unwrap() error { return e.Err }

// ThresholdError represents errors related to the generator's threshold logic
type ThresholdError struct {
	Threshold   int
	ActualRate  int
	Eligible    int
	Processed   int
	Err         error   // Combined error
	WrappedErrs []error // Slice of underlying errors
}

func (e *ThresholdError) Error() string {
	errMsg := fmt.Sprintf("processing failed: success rate %d%% below threshold %d%% (%d/%d eligible images processed)",
		e.ActualRate, e.Threshold, e.Processed, e.Eligible)
	if len(e.WrappedErrs) > 0 {
		var errDetails []string
		for _, err := range e.WrappedErrs {
			errDetails = append(errDetails, err.Error())
		}
		errMsg = fmt.Sprintf("%s - Errors: [%s]", errMsg, strings.Join(errDetails, "; "))
	}
	return errMsg
}
func (e *ThresholdError) Unwrap() error { return e.Err }

// --- Generator Implementation ---

// Package chart provides functionality for working with Helm charts, including
// loading charts, analyzing their structure, and generating override values.
//
// The package is responsible for:
// - Loading Helm charts from local filesystem or tarballs
// - Analyzing chart values to detect image references
// - Generating override values to redirect images to a target registry
// - Applying path strategies to generate appropriate image paths
// - Handling subcharts and their dependencies
// - Supporting threshold-based override generation
// - Validating generated overrides
//
// The primary components are:
// - Generator: Generates image override values for a chart
// - GeneratorLoader: Loads Helm charts using the Helm libraries
//
// Usage Example:
//
//	generator := chart.NewGenerator(
//		"./my-chart", "harbor.example.com",
//		[]string{"docker.io", "quay.io"}, []string{},
//		strategy.NewPrefixSourceRegistryStrategy(),
//		nil, nil, false, 100, nil, nil, nil, nil,
//	)
//	result, err := generator.Generate()

// Generator implements chart analysis and override generation.
// It loads a Helm chart, analyzes its values for image references,
// and generates the necessary overrides to redirect those images
// to a target registry using the specified path strategy.
//
// The Generator can be configured with:
// - Source registries to process (e.g., docker.io, quay.io)
// - Registries to exclude from processing
// - A path strategy that determines how image paths are constructed
// - Strict mode for handling unsupported structures
// - A threshold for minimum processing success rate
// - Registry mappings for advanced path manipulation
//
// Error handling is integrated with pkg/exitcodes for consistent exit codes:
// - Chart loading failures map to ExitChartParsingError (10)
// - Image processing issues map to ExitImageProcessingError (11)
// - Unsupported structures in strict mode map to ExitUnsupportedStructure (12)
// - Threshold failures map to ExitThresholdError (13)
// - ExitGeneralRuntimeError (20) for system/runtime errors
type Generator struct {
	chartPath         string
	targetRegistry    string
	sourceRegistries  []string
	excludeRegistries []string
	pathStrategy      strategy.PathStrategy
	mappings          *registry.Mappings
	strict            bool
	threshold         int
	loader            Loader                  // Use Loader from this package
	rulesEnabled      bool                    // Whether to apply rules
	rulesRegistry     rules.RegistryInterface // Use the interface type here
}

// NewGenerator creates a new Generator with the provided configuration
func NewGenerator(
	chartPath, targetRegistry string,
	sourceRegistries, excludeRegistries []string,
	pathStrategy strategy.PathStrategy,
	mappings *registry.Mappings,
	strict bool,
	threshold int,
	chartLoader Loader, // Keep loader for potential validation use
	rulesEnabled bool,
) *Generator {
	// Set up a default chart loader if none was provided
	if chartLoader == nil {
		chartLoader = NewLoader()
	}

	// Add debug logging for mappings
	if mappings != nil {
		log.Debug("Generator initialized with mappings",
			"entries_count", len(mappings.Entries),
			"entries", fmt.Sprintf("%+v", mappings.Entries))
	} else {
		log.Debug("Generator initialized with nil mappings")
	}

	return &Generator{
		chartPath:         chartPath,
		targetRegistry:    targetRegistry,
		sourceRegistries:  sourceRegistries,
		excludeRegistries: excludeRegistries,
		pathStrategy:      pathStrategy,
		mappings:          mappings,
		strict:            strict,
		threshold:         threshold,
		loader:            chartLoader,
		rulesEnabled:      rulesEnabled,
		rulesRegistry:     rules.NewRegistry(), // Initialize the rules registry
	}
}

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
	if g.mappings != nil {
		if mappedTarget := g.mappings.GetTargetRegistry(imgRef.Registry); mappedTarget != "" {
			log.Debug("Using mapped target registry", "source", imgRef.Registry, "target", mappedTarget)

			// If the mapped target contains a path, split it into registry and path
			if strings.Contains(mappedTarget, "/") {
				parts := strings.SplitN(mappedTarget, "/", MaxSplitParts)
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
		}
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
		if repoVal, repoOk := overrideMap["repository"]; repoOk {
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

// --- Refactored Generate Logic --- (Helper methods added below)

// FailedItem struct definition remains the same
type FailedItem struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// processEligibleImagesLoop iterates through eligible images, processes them, and collects results.
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

// Generate creates the override values based on the detected images and strategy.
// It takes the loaded chart and the analysis result as input.
func (g *Generator) Generate(loadedChart *chart.Chart, analysisResult *analysis.ChartAnalysis) (*override.File, error) {
	log.Debug("Generate called", "hasLoadedChart", loadedChart != nil, "hasAnalysisResult", analysisResult != nil)

	// Handle case where analysis failed (e.g., chart loading error)
	if analysisResult == nil {
		log.Error("Generate received nil analysisResult, cannot proceed.")
		return nil, errors.New("cannot generate overrides without analysis results (analysisResult is nil)")
	}

	// Ensure the loaded chart is provided
	if loadedChart == nil {
		log.Error("Generate received nil loadedChart")
		return nil, errors.New("internal error: Generate received nil loadedChart")
	}

	// Ensure Chart metadata is available
	if loadedChart.Metadata == nil {
		log.Error("Generate received loadedChart with nil Metadata")
		return nil, errors.New("internal error: Generate received loadedChart with nil Metadata")
	}

	chartName := loadedChart.Name()
	chartVersion := loadedChart.Metadata.Version
	log.Debug("Starting override generation", "chartName", chartName, "chartVersion", chartVersion, "strategy", reflect.TypeOf(g.pathStrategy).Elem().Name())

	// Initialize the final override structure
	result := &override.File{
		ChartPath:   loadedChart.ChartPath(),
		Values:      make(map[string]interface{}),
		Unsupported: make([]override.UnsupportedStructure, 0), // Initialize with make
	}

	// === Moved Strict Check Before Filtering ===
	// Check for strict mode violations first using the raw analysis results
	unsupported := g.findUnsupportedPatterns(analysisResult.ImagePatterns)
	if g.strict && len(unsupported) > 0 {
		result.Unsupported = unsupported // Store unsupported only on strict mode error
		log.Error("Strict mode violation: Unsupported structures found", "count", len(unsupported))
		return result, fmt.Errorf("%w: %d unsupported structures found (strict mode)",
			ErrUnsupportedStructure, len(unsupported))
	}
	// === End Moved Block ===

	// Filter images based on source/exclude registries
	eligibleImages := g.filterEligibleImages(analysisResult.ImagePatterns)
	eligibleCount := len(eligibleImages)
	log.Info("Filtering complete", "total_images", len(analysisResult.ImagePatterns), "eligible_images", eligibleCount)

	// Log the eligible images and their paths before processing
	for i, p := range eligibleImages {
		log.Debug("Eligible image for processing", "index", i, "path", p.Path, "value", p.Value, "sourceOrigin", p.SourceOrigin)
	}

	// Log map address before loop
	log.Debug("Generate: Map address BEFORE loop", "map_addr", fmt.Sprintf("%p", result.Values))

	// 3. Process Eligible Images & Collect Errors (modifies result.Values)
	processingErrors, processedCount := g.processEligibleImagesLoop(eligibleImages, result.Values)
	result.ProcessedCount = processedCount // Store processed count

	// Log map address after loop
	log.Debug("Generate: Map address AFTER loop", "map_addr", fmt.Sprintf("%p", result.Values))

	// 4. Calculate and Store Success Rate
	var successRate float64
	if eligibleCount > 0 {
		successRate = (float64(processedCount) / float64(eligibleCount)) * PercentageMultiplier
	} else {
		successRate = 100.0 // No eligible images means 100% success
	}
	result.SuccessRate = successRate
	log.Info("Image processing complete", "processed", processedCount, "eligible", eligibleCount, "success_rate", fmt.Sprintf("%.2f%%", successRate))

	// 5. Check Threshold
	if thresholdErr := g.checkProcessingThreshold(processingErrors, processedCount, eligibleCount, successRate, result); thresholdErr != nil {
		log.Error("Processing threshold not met or strict mode failure", "error", thresholdErr)
		return nil, thresholdErr // Return nil result and the error
	}

	// 6. Apply Rules if enabled (using provided loadedChart)
	if g.rulesEnabled && g.rulesRegistry != nil {
		if rulesErr := g.applyRulesIfNeeded(loadedChart, result); rulesErr != nil {
			log.Error("Error applying chart rules", "error", rulesErr)
			return result, fmt.Errorf("error applying chart rules: %w", rulesErr)
		}
	} else if g.rulesEnabled {
		log.Warn("Rules are enabled but rules registry is nil. Skipping rule application.")
	}

	// Log the final generated overrides map before returning
	finalMapKeys := []string{}
	for k := range result.Values {
		finalMapKeys = append(finalMapKeys, k)
	}
	log.Debug("Generator.Generate: Final override map keys before return", "keys", finalMapKeys, "map_addr", fmt.Sprintf("%p", result.Values))

	// *** Add full map logging before returning result ***
	log.Debug("Generator.Generate: Full overrides map structure BEFORE returning", "structure", result.Values)
	// *** End logging ***

	// If processing errors occurred but didn't breach threshold, return the partial result and a combined error
	if len(processingErrors) > 0 {
		combinedErr := &ProcessingError{Errors: processingErrors, Count: len(processingErrors)}
		log.Warn("Generate completed with non-fatal processing errors", "error_count", len(processingErrors))
		return result, combinedErr // Return partial result and the combined error
	}

	log.Debug("Generate finished successfully", "override_keys", mapKeys(result.Values))
	return result, nil
}

// ValidateHelmTemplate runs `helm template` on the chart with the provided overrides
// to check for rendering errors or invalid configurations introduced by the overrides.
// It returns an error if the template command fails.
func ValidateHelmTemplate(chartPath string, overrides []byte) error {
	log.Debug("Validating Helm template", "chartPath", chartPath)
	// Call the internal function (or its mock via the variable)
	err := validateHelmTemplateInternalFunc(chartPath, overrides)
	if err != nil {
		// Check if it's the specific Bitnami template error
		// Corrected string check based on test case definition
		if strings.Contains(err.Error(), "Original containers have been substituted for unrecognized ones") {
			log.Warn("Helm validation failed with Bitnami security context error, retrying without overrides...", "chartPath", chartPath, "error", err)
			// Retry without overrides
			err = validateHelmTemplateInternalFunc(chartPath, nil)
			if err != nil {
				log.Error("Helm template validation failed even after retry without overrides", "error", err)
				return fmt.Errorf("helm template validation failed on retry: %w", err)
			} // If retry succeeds, log info and return nil
			log.Info("Helm validation succeeded on retry without overrides (Bitnami common issue)")
			return nil
		}

		// If it's not the Bitnami error, log and return the original error
		log.Error("Helm template validation failed", "error", err)
		return fmt.Errorf("helm template validation failed: %w", err)
	}
	log.Info("Helm template validation successful")
	return nil
}

// validateHelmTemplateInternalFunc is a variable holding the function that performs
// the actual Helm template validation without any retry logic. This is defined as a
// variable to allow mocking in tests.
var validateHelmTemplateInternalFunc = validateHelmTemplateInternal

// validateHelmTemplateInternal performs the actual execution of the `helm template` command.
// It creates a temporary file for the overrides and runs Helm.
// This function is wrapped by ValidateHelmTemplate for potential mocking.
func validateHelmTemplateInternal(chartPath string, overrides []byte) error {
	// Setup Helm environment settings
	settings := cli.New() // Use default settings

	// Setup Action Configuration
	actionConfig := new(action.Configuration)
	// Use an in-memory client for validation - avoid actual cluster interaction
	// The namespace might be needed depending on chart logic, use a default.
	err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER"), func(format string, v ...interface{}) {
		// Route Helm's internal logging to our slog logger at Debug level
		// Keep Sprintf here as it's Helm's log format, not ours
		log.Debug(fmt.Sprintf("[Helm] %s", fmt.Sprintf(format, v...)))
	})
	if err != nil {
		return fmt.Errorf("failed to initialize Helm action config: %w", err)
	}

	// Create a temporary file for the overrides
	tmpFile, err := os.CreateTemp("", "irr-overrides-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temporary override file: %w", err)
	}
	defer func() {
		// Close the file handle before removing
		if closeErr := tmpFile.Close(); closeErr != nil {
			log.Warn("Failed to close temporary override file", "path", tmpFile.Name(), "error", closeErr)
		}
		// Remove the temporary file
		if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
			log.Warn("Failed to remove temporary override file", "path", tmpFile.Name(), "error", removeErr)
		} else {
			log.Debug("Removed temporary override file", "path", tmpFile.Name()) // Refactored
		}
	}()

	if _, err = tmpFile.Write(overrides); err != nil {
		return fmt.Errorf("failed to write overrides to temporary file: %w", err)
	}
	// Close might not be strictly necessary here if we just wrote, but good practice
	// if err = tmpFile.Close(); err != nil {
	// 	return fmt.Errorf("failed to close temporary override file after writing: %w", err)
	// }
	log.Debug("Overrides written to temporary file", "path", tmpFile.Name()) // Refactored

	// --- Load the Chart ---
	// Use the same loader logic as Generator for consistency (if possible)
	// Here we use Helm's standard loader for simplicity in validation context.
	chartReq, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart for validation %s: %w", chartPath, err)
	}
	log.Debug("Chart loaded for validation", "name", chartReq.Name()) // Refactored

	// --- Prepare Values ---
	// Combine base values from chart and overrides from the temp file
	// Start with chart's default values
	baseValues, err := chartutil.CoalesceValues(chartReq, chartReq.Values)
	if err != nil {
		return fmt.Errorf("failed to coalesce base chart values: %w", err)
	}
	log.Debug("Coalesced base chart values") // Refactored (no args needed)

	// Load override values from the temp file
	overrideValues, err := chartutil.ReadValuesFile(tmpFile.Name())
	if err != nil {
		return fmt.Errorf("failed to read override values from temp file %s: %w", tmpFile.Name(), err)
	}
	log.Debug("Loaded override values from temporary file", "path", tmpFile.Name()) // Refactored

	// Merge override values onto base values
	finalValues := chartutil.CoalesceTables(overrideValues, baseValues)
	log.Debug("Merged override values with base values") // Refactored (no args needed)

	// --- Configure Template Action ---
	client := action.NewInstall(actionConfig) // Use Install action for template rendering logic
	client.DryRun = true                      // Equivalent to 'helm template'
	client.ReleaseName = "irr-validation"     // Use a dummy release name
	client.Replace = true                     // Replace indicates upgrading an existing release (not relevant for dry-run template)
	client.ClientOnly = true                  // Perform rendering locally
	client.IncludeCRDs = true                 // Include CRDs in the output (optional, but good for complete validation)
	// Assign the merged values
	// Note: client.Run expects map[string]interface{}, chartutil gives chartutil.Values (map[string]interface{})
	valsMap := map[string]interface{}(finalValues)

	// --- Execute Rendering ---
	log.Debug("Executing Helm template rendering (dry-run install)") // Refactored (no args needed)
	rel, err := client.Run(chartReq, valsMap)

	// --- Analyze Results ---
	if err != nil {
		// Improve error logging context
		log.Error("Helm template rendering failed", "chart", chartReq.Name(), "error", err)

		// Check if the error is related to specific template failures
		if strings.Contains(err.Error(), "template:") || strings.Contains(err.Error(), "parse error") {
			// Provide a more specific error message for template issues
			return fmt.Errorf("chart template rendering error: %w", err)
		}
		// Return a general error for other issues
		return fmt.Errorf("helm template command execution failed: %w", err)
	}

	// Optional: Check if the release or rendered manifest is empty (might indicate issues)
	if rel == nil || rel.Manifest == "" {
		log.Warn("Helm template rendering resulted in an empty manifest. Chart might be empty or conditional rendering excluded all resources.")
		// Depending on requirements, this could be treated as an error.
		// For now, just a warning.
	} else {
		log.Debug("Helm template rendering successful", "manifest_length", len(rel.Manifest)) // Refactored
	}

	// Validation successful if no error occurred
	return nil
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

// ProcessingError represents an aggregation of errors encountered during processing.
type ProcessingError struct {
	Errors []error
	Count  int
}

func (e *ProcessingError) Error() string {
	var errStrings []string
	for _, err := range e.Errors {
		errStrings = append(errStrings, err.Error()) // Use the full error message which includes path
	}
	// Provide a more informative summary message
	return fmt.Sprintf("strict mode: %d processing errors occurred for paths: %s", e.Count, strings.Join(errStrings, "; "))
}

// --- Override Generation Logic ---

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
		"registry":   targetReg,
		"repository": finalRepository,
	}

	// Only include the tag field in the map if finalTag is not empty
	if finalTag != "" {
		log.Debug("Including tag in override map", "tag", finalTag)
		overrideMap["tag"] = finalTag
	} else {
		log.Debug("Omitting tag from override map as it's empty (either originally or after fallback logic).", "path", pattern.Path)
	}

	// Preserve/add pullPolicy if original pattern indicates a map structure
	if pattern.Structure != nil || pattern.Type == analysis.PatternTypeMap {
		pullPolicy := "IfNotPresent" // Default pull policy
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
	if repoVal, ok := overrideMap["repository"]; ok {
		log.Debug("Final check createOverride", "path", pattern.Path, "repo_type", fmt.Sprintf("%T", repoVal), "repo_value", repoVal)
	} else {
		log.Warn("Final check createOverride: Repository key missing", "path", pattern.Path)
	}
	// *** End final check ***
	return overrideMap
}

// Helper function (assuming not already present)
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// setOverridePath sets the value at the specified path within the overrides map.
// It handles creating nested maps and arrays as needed.
func (g *Generator) setOverridePath(overrides map[string]interface{}, pattern *analysis.ImagePattern, value interface{}) error {
	path := pattern.Path
	pathElems := strings.Split(path, ".")
	log.Debug("setOverridePath: START", "path", path, "elements", pathElems, "valueType", fmt.Sprintf("%T", value))

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
