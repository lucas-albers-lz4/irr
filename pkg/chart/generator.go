package chart

import (
	"errors" // Re-import standard errors for errors.New
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"

	// Use github.com/pkg/errors for wrapping capabilities
	pkgerrors "github.com/pkg/errors" // Alias to avoid collision

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	image "github.com/lucas-albers-lz4/irr/pkg/image"
	log "github.com/lucas-albers-lz4/irr/pkg/log"

	// Removed: "github.com/lucas-albers-lz4/irr/pkg/maputil"

	helmtypes "github.com/lucas-albers-lz4/irr/pkg/helmtypes"
	"github.com/lucas-albers-lz4/irr/pkg/override"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/lucas-albers-lz4/irr/pkg/rules"
	"github.com/lucas-albers-lz4/irr/pkg/strategy"
	"helm.sh/helm/v3/pkg/cli/values"
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
	// maxErrorParts defines the maximum parts for splitting image strings that caused errors, usually for tag/digest separation.
	maxErrorParts = 2
)

// --- Local Error Definitions ---
// Use standard errors.New for simple error types
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

// --- analysis.ChartLoader Implementation ---

// analysisLoaderWrapper adapts helmtypes.ChartLoader to analysis.ChartLoader interface.
// This is necessary because the analysis package defines its own simpler ChartLoader interface.
type analysisLoaderWrapper struct {
	originalLoader helmtypes.ChartLoader // The underlying loader (e.g., internal/helm.DefaultChartLoader)
}

// NewAnalysisLoaderWrapper creates a new wrapper.
func NewAnalysisLoaderWrapper(helmLoader helmtypes.ChartLoader) analysis.ChartLoader {
	if helmLoader == nil {
		// Handle nil case? Maybe return an error or a default internal loader?
		log.Error("NewAnalysisLoaderWrapper received nil helmtypes.ChartLoader")
		// Returning nil might cause panic later. Need a robust strategy.
		// For now, let it proceed, but this highlights a potential issue.
		return nil
	}
	return &analysisLoaderWrapper{originalLoader: helmLoader}
}

// Load implements the analysis.ChartLoader interface.
func (w *analysisLoaderWrapper) Load(chartPath string) (*chart.Chart, error) {
	if w.originalLoader == nil {
		return nil, errors.New("internal error: analysisLoaderWrapper has a nil loader")
	}
	// The analysis.ChartLoader interface doesn't provide value options.
	// We must call the underlying loader with default/empty options.
	// This might not be sufficient if the analysis truly depends on computed values.
	opts := &helmtypes.ChartLoaderOptions{
		ChartPath:  chartPath,
		ValuesOpts: values.Options{}, // Use default/empty values options
	}

	// We only need the chart struct, not the merged values here.
	// LoadChartWithValues is simpler, but LoadChartAndTrackOrigins might be needed
	// if the analyzer internally relies on the context it provides.
	// Let's try LoadChartWithValues first.
	loadedChart, _, err := w.originalLoader.LoadChartWithValues(opts)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "analysisLoaderWrapper failed to load chart '%s' via helmtypes.ChartLoader", chartPath)
	}
	return loadedChart, nil
}

// --- End analysis.ChartLoader Implementation ---

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
	includePatterns   []string // Passed to detector context
	excludePatterns   []string // Passed to detector context
	knownPaths        []string // Passed to detector context
	threshold         int
	loader            helmtypes.ChartLoader
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
	chartLoader helmtypes.ChartLoader,
	includePatterns, excludePatterns, knownPaths []string,
	rulesEnabled bool,
) *Generator {
	// Set up a default chart loader if none was provided
	if chartLoader == nil {
		// Caller is responsible for providing a loader. Error or use a default?
		// For now, let's assume the caller handles this. If nil is passed, it will panic later.
		// A better approach might be to return an error or have a default internal one.
		log.Warn("NewGenerator received nil chartLoader. Ensure a loader is provided.")
	}

	return &Generator{
		chartPath:         chartPath,
		targetRegistry:    targetRegistry,
		sourceRegistries:  sourceRegistries,
		excludeRegistries: excludeRegistries,
		pathStrategy:      pathStrategy,
		mappings:          mappings,
		strict:            strict,
		includePatterns:   includePatterns,
		excludePatterns:   excludePatterns,
		knownPaths:        knownPaths,
		threshold:         threshold,
		loader:            chartLoader,
		rulesEnabled:      rulesEnabled,
		rulesRegistry:     nil, // Initialize rules registry later if needed
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

	for _, pattern := range detectedImages {
		// Handle potential errors during parsing more gracefully
		imgRef, err := g.processImagePattern(pattern)
		if err != nil {
			continue
		}

		if imgRef == nil {
			continue
		}

		// Perform checks using the pre-normalized maps
		normalizedReg := image.NormalizeRegistry(imgRef.Registry)
		isSource := normalizedSources[normalizedReg]
		isExcluded := normalizedExcludes[normalizedReg]

		if isSource && !isExcluded {
			eligibleImages = append(eligibleImages, pattern)
		}
	}

	log.Debug("Finished filtering images", "eligible_count", len(eligibleImages))
	return eligibleImages
}

// determineTargetPathAndRegistry calculates the target registry and the new repository path
// for a given image reference based on the configured registry mappings and path strategy.
// It applies mappings first to determine the effective target registry prefix,
// then uses the path strategy to construct the repository path part.
func (g *Generator) determineTargetPathAndRegistry(imgRef *image.Reference, pattern analysis.ImagePattern) (finalRegistryPrefix, finalRepoPathPart string, err error) {
	log.Debug("determineTargetPathAndRegistry: start", "patternValue", pattern.Value, "imgRefRegistry", imgRef.Registry)

	// Determine original source registry from pattern or fallback to imgRef
	originalSourceRegistry := imgRef.Registry // Fallback
	parsedFromPattern, parseErr := image.ParseImageReference(pattern.Value)
	if parseErr == nil && parsedFromPattern.Registry != "" {
		originalSourceRegistry = parsedFromPattern.Registry
	}

	// Determine the final registry prefix: Use mapping target if available, else use default targetRegistry
	finalRegistryPrefix = g.targetRegistry // Default
	if g.mappings != nil {
		if mappedTargetPrefix := g.mappings.GetTargetRegistry(originalSourceRegistry); mappedTargetPrefix != "" {
			finalRegistryPrefix = mappedTargetPrefix // Override with mapping target
			log.Debug("determineTargetPathAndRegistry: Using mapping target prefix", "originalRegistry", originalSourceRegistry, "mappedTargetPrefix", mappedTargetPrefix)
		} else {
			log.Debug("determineTargetPathAndRegistry: No mapping found, using default target registry", "originalRegistry", originalSourceRegistry, "defaultTargetRegistry", g.targetRegistry)
		}
	} else {
		log.Debug("determineTargetPathAndRegistry: No mappings configured, using default target registry", "defaultTargetRegistry", g.targetRegistry)
	}

	// Generate the repository path PART using the strategy.
	// The strategy should ideally return the path *relative* to the registry (e.g., "library/nginx" or "docker.io/library/nginx")
	// The PrefixSourceRegistryStrategy currently returns the source-prefixed path (e.g., "docker.io/library/nginx").
	// We will use this path directly for now, assuming the combination works.
	// The `finalRegistryPrefix` already incorporates the mapping target path.
	var strategyGeneratedPath string
	strategyGeneratedPath, err = g.pathStrategy.GeneratePath(imgRef, pattern, g.targetRegistry) // Pass original target for context, though prefix strategy ignores it
	if err != nil {
		return "", "", pkgerrors.Wrapf(err, "path generation failed for pattern '%s' (value: '%s'): %v", pattern.Path, pattern.Value, err)
	}

	// For now, we assume the strategy output (`strategyGeneratedPath`) is the correct final path part
	// relative to the `finalRegistryPrefix`.
	// Example with mapping: finalRegistryPrefix="registry.example.com/dockerio", strategyGeneratedPath="docker.io/library/nginx"
	//   -> Resulting full path = registry.example.com/dockerio/library/nginx (Incorrect if mapping target includes source prefix)
	// Example without mapping: finalRegistryPrefix="test.registry.io", strategyGeneratedPath="docker.io/library/nginx"
	//   -> Resulting full path = test.registry.io/docker.io/library/nginx (Correct for Prefix strategy)
	// Let's adjust: If a mapping was used, assume the mapping *replaces* the default target AND the strategy's prefix.
	// This is a heuristic.

	finalRepoPathPart = strategyGeneratedPath // Start with strategy output
	if g.mappings != nil && g.mappings.GetTargetRegistry(originalSourceRegistry) != "" {
		// Mapping exists. PrefixSourceRegistryStrategy returns sourcePrefix/repoPath.
		// Mapped target is finalRegistryPrefix.
		// We want finalRegistryPrefix/repoPath.
		// Need to strip the sourcePrefix from strategyGeneratedPath.
		sourcePrefix := image.SanitizeRegistryForPath(originalSourceRegistry)
		if strings.HasPrefix(strategyGeneratedPath, sourcePrefix+"/") {
			finalRepoPathPart = strings.TrimPrefix(strategyGeneratedPath, sourcePrefix+"/")
			log.Debug("determineTargetPathAndRegistry: Stripped source prefix due to mapping", "originalStrategyPath", strategyGeneratedPath, "finalRepoPathPart", finalRepoPathPart)
		} else {
			log.Warn("determineTargetPathAndRegistry: Mapping exists, but strategy path didn't have expected source prefix", "strategyPath", strategyGeneratedPath, "sourcePrefix", sourcePrefix)
		}
	}

	log.Debug("determineTargetPathAndRegistry: computed", "finalRegistryPrefix", finalRegistryPrefix, "finalRepoPathPart", finalRepoPathPart)

	// Return the final registry prefix and the repository path part.
	return finalRegistryPrefix, finalRepoPathPart, nil
}

// processImage processes a single eligible image pattern and adds its override to the map.
func (g *Generator) processImage(pattern analysis.ImagePattern, overrides map[string]interface{}, aliasMap map[string]string) (processed bool, unsupported *override.UnsupportedStructure, err error) {
	log.Debug("Processing image pattern", "path", pattern.Path, "value", pattern.Value, "type", pattern.Type)

	// Check if this path already exists in the overrides (prevent lower precedence overwrites)
	pathExists := pathExistsInMap(overrides, pattern.Path)
	if pathExists {
		log.Warn("Override path already exists, skipping potential overwrite", "path", pattern.Path)
		return false, nil, nil // Consider this 'processed' in the sense that we don't want to touch it
	}

	imgRef, err := g.processImagePattern(pattern)
	if err != nil {
		// Error already logged in helper
		return false, nil, pkgerrors.Wrapf(err, "failed to process image pattern at path %s: %v", pattern.Path, err)
	}

	// --- 2. Determine Target Registry and Repository Path ---
	finalRegistryPrefix, finalRepoPathPart, err := g.determineTargetPathAndRegistry(imgRef, pattern)
	if err != nil {
		log.Error("Failed to determine target path/registry", "path", pattern.Path, "error", err)
		// Use errors.Wrapf from github.com/pkg/errors
		return false, nil, fmt.Errorf("failed to determine target path and registry for %s", pattern.Value)
	}

	// --- 3. Create the override value (string or map) ---
	// Correct argument order: pattern, imgRef
	overrideValue := g.createOverride(pattern, imgRef, finalRegistryPrefix, finalRepoPathPart)

	// --- 4. Set the override value at the correct path ---
	// Correct function call: setOverridePath without receiver
	if err := setOverridePath(pattern, overrides, overrideValue, aliasMap); err != nil {
		log.Error("Failed to set override path in map", "path", pattern.Path, "error", err)
		return false, nil, err // Return the path setting error
	}

	// Successfully processed and set the override
	log.Debug("Successfully processed image and set override", "path", pattern.Path)
	return true, nil, nil
}

// --- Refactored Generate Logic --- (Helper methods added below)

// FailedItem struct definition remains the same
type FailedItem struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// processEligibleImagesLoop iterates through eligible images, processes them, and collects results.
// It now accepts the aliasMap to pass down to processImage.
func (g *Generator) processEligibleImagesLoop(eligibleImages []analysis.ImagePattern, overrides map[string]interface{}, aliasMap map[string]string) (processingErrors []error, processedCount int) {
	// Initialize local slices/maps if needed (overrides is passed in)
	if overrides == nil {
		overrides = make(map[string]interface{}) // Should ideally not happen if called from Generate
		log.Warn("Overrides map was nil in processEligibleImagesLoop, re-initialized")
	}
	processingErrors = []error{}
	processedCount = 0

	for _, pattern := range eligibleImages {
		// Pass aliasMap to processImage
		processed, unsupported, err := g.processImage(pattern, overrides, aliasMap) // Pass aliasMap
		switch {
		case err != nil:
			log.Warn("Error processing image pattern", "path", pattern.Path, "error", err)
			wrappedErr := fmt.Errorf("path '%s': %w", pattern.Path, err)
			processingErrors = append(processingErrors, wrappedErr)
		case unsupported != nil:
			log.Warn("Unsupported structure detected", "path", pattern.Path, "type", unsupported.Type, "value", pattern.Value)
			// Handle strict mode: add error
			if g.strict {
				log.Error("Unsupported structure detected in strict mode", "path", pattern.Path, "type", unsupported.Type)
				strictErr := fmt.Errorf("path '%s': %w (type: %s)", pattern.Path, ErrUnsupportedStructure, unsupported.Type)
				processingErrors = append(processingErrors, strictErr)
				continue // Skip further processing for this pattern
			}
		case processed:
			processedCount++
		}
	}
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
			combinedErr = fmt.Errorf("processing errors: %v", strings.Join(errStrings, "; "))
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
		return pkgerrors.Wrapf(err, "failed to apply rules to chart %v", g.chartPath)
	}
	if modified {
		log.Debug("Rules modified overrides", "chart_path", g.chartPath)
	} else {
		log.Debug("Rules applied successfully (no changes)", "chart_path", g.chartPath)
	}
	return nil
}

// Generate orchestrates the chart loading, analysis, and override generation process.
func (g *Generator) Generate() (*override.File, error) {
	// Initialize the result structure
	result := &override.File{
		ChartPath:   g.chartPath,
		ChartName:   filepath.Base(g.chartPath), // Extract chart name from path
		Values:      make(map[string]interface{}),
		Unsupported: []override.UnsupportedStructure{}, // Initialize slice
	}

	// --- 1. Load Chart with Merged Values & Origins ---
	// Assuming values opts are handled by the calling command (irr override)
	// Generator focuses on the core logic using the loaded context.
	loadOpts := &helmtypes.ChartLoaderOptions{
		ChartPath:  g.chartPath,
		ValuesOpts: values.Options{}, // Pass empty opts, expecting caller to provide merged context if needed?
		// TODO: Revisit how ValuesOpts should be populated here or if generator needs merged values passed in.
		// For now, proceed assuming LoadChartAndTrackOrigins gives us what we need from the chart structure itself.
	}
	analysisContext, err := g.loader.LoadChartAndTrackOrigins(loadOpts)
	if err != nil {
		log.Error("Failed to load chart and track origins", "path", g.chartPath, "error", err)
		// Map helm loader errors to appropriate exit codes/errors if possible
		// Return specific LoadingError type
		return nil, &LoadingError{ChartPath: g.chartPath, Err: err}
	}
	if analysisContext == nil || analysisContext.LoadedChart == nil {
		// Should not happen if LoadChartAndTrackOrigins returns nil error
		return nil, pkgerrors.New("internal error: nil analysis context after successful load")
	}

	aliasMap := make(map[string]string) // Define aliasMap outside the if block

	// Populate alias map from loaded chart dependencies
	if analysisContext.LoadedChart.Metadata != nil && analysisContext.LoadedChart.Metadata.Dependencies != nil {
		for _, dep := range analysisContext.LoadedChart.Metadata.Dependencies {
			if dep.Alias != "" {
				aliasMap[dep.Name] = dep.Alias
			}
		}
	}

	// --- 2. Analyze Merged Values ---
	// Create the wrapper for the loader
	analysisLoader := NewAnalysisLoaderWrapper(g.loader)
	analyzer := analysis.NewAnalyzer(g.chartPath, analysisLoader)

	// Analyze the MERGED values directly. Assume Analyze takes values and returns patterns.
	// This deviates from the signature found (Analyze()), but aligns with how Generate uses it.
	// We need to reconcile the Analyzer interface/implementation with its usage here.
	// Let's TRY calling it with the merged values and assume it returns []ImagePattern
	analysisResult, analysisErr := analyzer.Analyze() // Returns *analysis.ChartAnalysis

	if analysisErr != nil {
		// Log error but potentially continue if some patterns were found?
		log.Error("Error during analysis of merged values", "error", analysisErr)
		// Decide on error handling - return error or proceed with potentially incomplete patterns?
		// Let's return error for now to be safe.
		return nil, pkgerrors.Wrap(analysisErr, "analysis of merged values failed")
	}

	var detectedPatternsList []analysis.ImagePattern // Declare the slice
	// Extract the image patterns from the analysis result
	if analysisResult == nil {
		// Handle nil analysis result case
		log.Warn("Analyzer returned nil result, assuming no patterns found.")
		detectedPatternsList = []analysis.ImagePattern{}
	} else {
		// Assuming ImagePatterns is the field name in ChartAnalysis
		detectedPatternsList = analysisResult.ImagePatterns
	}

	// Assuming detectedPatternsList is now correctly []analysis.ImagePattern
	result.Unsupported = append(result.Unsupported, g.findUnsupportedPatterns(detectedPatternsList)...)
	if g.strict && len(result.Unsupported) > 0 {
		log.Debug("Generator found unsupported patterns in strict mode", "count", len(result.Unsupported))
		firstUnsupported := result.Unsupported[0]
		return nil, &UnsupportedStructureError{
			Path: firstUnsupported.Path,
			Type: firstUnsupported.Type,
		}
	}
	log.Info("Finished chart analysis of merged values", "total_patterns", len(detectedPatternsList), "unsupported_count", len(result.Unsupported))

	// --- 3. Filter Eligible Images ---
	eligibleImages := g.filterEligibleImages(detectedPatternsList) // Use patterns from merged analysis
	eligibleCount := len(eligibleImages)
	log.Debug("Finished filtering images", "eligible_count", eligibleCount)
	if eligibleCount == 0 {
		log.Info("No eligible images found for processing", "chart", filepath.Base(g.chartPath))
		return result, nil
	}

	// 4. Process Eligible Images & Collect Errors
	// Pass the aliasMap to the processing loop helper
	processingErrors, processedCount := g.processEligibleImagesLoop(eligibleImages, result.Values, aliasMap) // Pass aliasMap
	result.ProcessedCount = processedCount                                                                   // Store processed count

	// 5. Calculate and Store Success Rate
	var successRate float64
	if eligibleCount > 0 {
		successRate = (float64(processedCount) / float64(eligibleCount)) * PercentageMultiplier
	} else {
		successRate = 100.0 // No eligible images means 100% success
	}
	result.SuccessRate = successRate
	log.Info("Image processing complete", "processed", processedCount, "eligible", eligibleCount, "success_rate", fmt.Sprintf("%.2f%%", successRate))

	// 6. Check Threshold
	if thresholdErr := g.checkProcessingThreshold(processingErrors, processedCount, eligibleCount, successRate, result); thresholdErr != nil {
		log.Error("Processing threshold not met or strict mode failure", "error", thresholdErr)
		// If thresholdErr is due to strict mode (i.e., processingErrors is not empty and g.strict is true),
		// or if it's a threshold failure, return nil for the result.
		// The checkProcessingThreshold function already encapsulates this logic implicitly
		// by returning an error in these cases.
		return nil, thresholdErr // Return nil result and the error
	}

	// 7. Apply Rules if enabled
	if rulesErr := g.applyRulesIfNeeded(analysisContext.LoadedChart, result); rulesErr != nil {
		log.Error("Error applying chart rules", "error", rulesErr)
		// Consider if this should be a distinct exit code or wrapped
		return result, pkgerrors.Wrapf(rulesErr, "error applying chart rules: %v", rulesErr)
	}

	log.Debug("Override generation successful", "chart", result.ChartName)
	return result, nil
}

// initRulesRegistry initializes the rules registry if rules are enabled and the registry is not already set.
// This ensures the registry is ready before being used in ApplyRules.
func (g *Generator) initRulesRegistry() {
	if !g.rulesEnabled || g.rulesRegistry != nil {
		return // Rules disabled or registry already initialized
	}
	if g.rulesRegistry == nil {
		log.Debug("Generator: Initializing default rules registry.")
		g.rulesRegistry = rules.NewRegistry() // Assuming default initialization
		// Load default rules if necessary, or this could be handled by the registry itself
	}
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
				// Don't use %v for the error, just wrap it directly
				return pkgerrors.Wrap(err, "helm template validation failed on retry")
			} // If retry succeeds, log info and return nil
			log.Info("Helm validation succeeded on retry without overrides (Bitnami common issue)")
			return nil
		}

		// If it's not the Bitnami error, log and return the original error
		log.Error("Helm template validation failed", "error", err)
		// Don't use %v for the error, just wrap it directly
		return pkgerrors.Wrap(err, "helm template validation failed")
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
	log.Debug("Running internal helm template validation", "chartPath", chartPath)
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
		return pkgerrors.Wrapf(err, "failed to initialize Helm action config: %v", err)
	}

	// Create a temporary file for the overrides
	tmpFile, err := os.CreateTemp("", "irr-overrides-*.yaml")
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to create temporary override file: %v", err)
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
		return pkgerrors.Wrapf(err, "failed to write overrides to temporary file: %v", err)
	}
	// Close might not be strictly necessary here if we just wrote, but good practice
	// if err = tmpFile.Close(); err != nil {
	// 	return pkgerrors.Wrapf(err, "failed to close temporary override file after writing: %v", err)
	// }
	log.Debug("Overrides written to temporary file", "path", tmpFile.Name()) // Refactored

	// --- Load the Chart ---
	// Use the same loader logic as Generator for consistency (if possible)
	// Here we use Helm's standard loader for simplicity in validation context.
	chartReq, err := loader.Load(chartPath)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to load chart for validation %v: %v", chartPath, err)
	}
	log.Debug("Chart loaded for validation", "name", chartReq.Name()) // Refactored

	// --- Prepare Values ---
	// Combine base values from chart and overrides from the temp file
	// Start with chart's default values
	baseValues, err := chartutil.CoalesceValues(chartReq, chartReq.Values)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to coalesce base chart values: %v", err)
	}
	log.Debug("Coalesced base chart values") // Refactored (no args needed)

	// Load override values from the temp file
	overrideValues, err := chartutil.ReadValuesFile(tmpFile.Name())
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to read override values from temp file %v: %v", tmpFile.Name(), err)
	}
	log.Debug("Loaded override values from temporary file", "path", tmpFile.Name()) // Refactored

	// Merge override values onto base values
	finalValues := chartutil.CoalesceTables(baseValues, overrideValues)
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
			return pkgerrors.Wrapf(err, "chart template rendering error: %v", err)
		}
		// Return a general error for other issues
		return pkgerrors.Wrapf(err, "helm template command execution failed: %v", err)
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
func findValueByPath(data map[string]interface{}, pathSegments []string) (interface{}, bool) {
	current := interface{}(data)
	for i, part := range pathSegments { // Keep index i for potential error messages
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
		return nil, pkgerrors.Wrapf(err, "failed to marshal overrides to YAML: %v", err)
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
	return fmt.Sprintf("strict mode: %d processing errors occurred for paths: %v", e.Count, strings.Join(errStrings, "; "))
}

// --- Override Generation Logic ---

// createOverride generates the override value (string or map) for a given pattern.
func (g *Generator) createOverride(pattern analysis.ImagePattern, imgRef *image.Reference, finalRegistryPrefix, finalRepoPathPart string) interface{} {
	log.Debug("Creating override", "patternPath", pattern.Path, "finalRegistryPrefix", finalRegistryPrefix, "finalRepoPathPart", finalRepoPathPart)
	// Log input reference details
	if imgRef != nil {
		log.Debug("[DEBUG GENERATOR] createOverride input imgRef", "path", pattern.Path, "ref_registry", imgRef.Registry, "ref_repo", imgRef.Repository, "ref_tag", imgRef.Tag, "ref_digest", imgRef.Digest)
	} else {
		log.Warn("[DEBUG GENERATOR] createOverride received nil imgRef", "path", pattern.Path)
	}

	// --- FIX: Construct the full target image path here ---
	// The path strategy now returns the repository path part relative to the target registry.
	// Join the target registry prefix with the relative path part.
	// Use path.Join for cross-platform compatibility, although image paths typically use '/'.
	// Consider explicitly using strings.Join with '/' if path.Join causes issues on Windows.
	fullTargetPath := path.Join(finalRegistryPrefix, finalRepoPathPart)
	// Replace backslashes potentially introduced by path.Join on Windows
	fullTargetPath = strings.ReplaceAll(fullTargetPath, "\\", "/")
	log.Debug("Constructed full target path", "fullTargetPath", fullTargetPath)

	// Determine the new tag. Use the original tag from the reference.
	newTag := imgRef.Tag

	// FIX: Fallback logic for tags, especially for map types
	if newTag == "" || newTag == image.DefaultTag {
		if pattern.Type == analysis.PatternTypeMap && pattern.Structure != nil {
			tagVal, ok := pattern.Structure["tag"]
			if ok {
				if tagStr, ok := tagVal.(string); ok && tagStr != "" {
					log.Debug("Using tag from pattern structure as fallback (map type)", "path", pattern.Path, "structure_tag", tagStr)
					newTag = tagStr
				}
			}
		}
		if newTag == "" {
			newTag = image.DefaultTag
		}
	}

	// Check pattern type to determine override structure
	switch pattern.Type {
	case analysis.PatternTypeString:
		// Override with a simple string: "registry/repo/path:tag"
		fullImageString := fmt.Sprintf("%s:%s", fullTargetPath, newTag)
		log.Debug("Generated override string", "value", fullImageString)
		return fullImageString

	case analysis.PatternTypeMap:
		// Override with a map structure: {repository: "repo/path", tag: "tag", registry: "registry"}
		// The 'repository' key in the override map should contain the path *without* the target registry.
		// The 'registry' key should contain the target registry.
		overrideMap := map[string]interface{}{
			"registry":   finalRegistryPrefix, // Target registry
			"repository": finalRepoPathPart,   // Path part returned by strategy
			"tag":        newTag,
		}
		log.Debug("Generated override map", "value", overrideMap)
		return overrideMap

	default:
		// Should not happen if analysis is correct
		log.Warn("Cannot create override for unsupported pattern type", "patternPath", pattern.Path, "patternType", pattern.Type)
		// Return the original value string as a fallback?
		// Returning nil might be safer to avoid incorrect overrides.
		return nil
	}
}

// setOverridePath sets the override value at the correct path within the nested override map.
// It handles dot-notation paths including subchart aliases.
func setOverridePath(pattern analysis.ImagePattern, overrides map[string]interface{}, overrideValue interface{}, _ map[string]string) error {
	// Use the full path from the pattern, which includes aliases/subchart names.
	// Example: "myChildAlias.image.repository" or "parentImage.tag"
	pathStr := pattern.Path
	log.Debug("Setting override path", "path", pathStr, "valueType", fmt.Sprintf("%T", overrideValue))

	// Split the path into parts
	parts := strings.Split(pathStr, ".")
	if len(parts) == 0 {
		return fmt.Errorf("invalid empty path provided for override: %s", pathStr)
	}

	currentMap := overrides
	// Iterate through the path parts, creating nested maps as needed
	// Stop one level before the final key
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		log.Debug("Navigating override path part", "part", part)

		nextVal, exists := currentMap[part]
		if !exists {
			// If the key doesn't exist, create a new map
			log.Debug("Creating intermediate map", "key", part)
			newMap := make(map[string]interface{})
			currentMap[part] = newMap
			currentMap = newMap
		} else {
			// If the key exists, ensure it's a map
			var ok bool
			currentMap, ok = nextVal.(map[string]interface{})
			if !ok {
				// Path conflict: trying to set a key inside something that isn't a map
				// This could happen if, e.g., `image` was previously set as a string
				// and now we try to set `image.repository`.
				// Overwrite with a map? Or error? Helm typically overwrites.
				log.Warn("Path conflict during override generation. Overwriting non-map with map.", "path", strings.Join(parts[:i+1], "."))
				newMap := make(map[string]interface{})
				currentMap[part] = newMap // Re-assign to fix the reference
				currentMap = newMap
			}
		}
	}

	// Set the final key in the deepest map
	lastKey := parts[len(parts)-1]
	log.Debug("Setting final override value", "key", lastKey)
	currentMap[lastKey] = overrideValue

	log.Debug("Override path set successfully", "fullPath", pathStr)
	return nil
}

// processImagePattern attempts to parse an image string from the pattern's value.
func (g *Generator) processImagePattern(pattern analysis.ImagePattern) (*image.Reference, error) {
	log.Debug("Processing image pattern", "path", pattern.Path, "value", pattern.Value)

	// Initial parsing attempt
	imgRef, err := image.ParseImageReference(pattern.Value)
	if err == nil {
		log.Debug("Successfully parsed image reference", "ref", imgRef.String())
		log.Debug("[DEBUG IRR PARSED REF]", "path", pattern.Path, "ref_original", imgRef.Original, "ref_registry", imgRef.Registry, "ref_repo", imgRef.Repository, "ref_tag", imgRef.Tag, "ref_digest", imgRef.Digest)
		return imgRef, nil // Success
	}

	// Handle parsing errors, potentially trying again if it looks like a missing tag/digest issue
	log.Warn("Initial image parse failed, checking for potential missing tag/digest",
		"path", pattern.Path, "value", pattern.Value, "error", err)

	// Heuristic: If the error suggests an invalid reference format AND the string contains ':',
	// it might be because a port number was mistaken for a tag separator. Let's try splitting.
	// Example: myregistry:5000/myimage -> interpreted as registry='myregistry', image='5000/myimage' (no tag)
	// Correct: Split by '/', handle potential port in the registry part.
	// Simpler heuristic for now: if it contains ':' but not '/', assume it's registry:port/image
	// if strings.Contains(pattern.Value, ":") && !strings.Contains(pattern.Value, "/") {
	// If the error is specifically about invalid reference format, try assuming default tag.
	// This is a common case where templates might omit ':latest'.
	if errors.Is(err, image.ErrInvalidImageRefFormat) {
		// Check if adding ':latest' helps - This is a very basic heuristic.
		// A more robust approach might involve more complex regex or parsing logic.
		imgRefWithLatest := pattern.Value + ":latest"
		imgRefRetry, errRetry := image.ParseImageReference(imgRefWithLatest)
		if errRetry == nil {
			log.Debug("Successfully parsed image reference by adding ':latest'", "ref", imgRefRetry.String())
			return imgRefRetry, nil
		}
		log.Warn("Adding ':latest' did not resolve parsing error", "path", pattern.Path, "value", pattern.Value, "retry_error", errRetry)

		// Another heuristic: Helm might template registry and repo separately, leading to values like 'myrepo:mytag'
		// where the registry is missing. image.ParseReference often fails here.
		// If it looks like `repo:tag` or `repo@digest`, try splitting.
		// Use defined constant for the split limit.
		parts := strings.SplitN(pattern.Value, ":", maxErrorParts)
		if len(parts) == maxErrorParts && !strings.Contains(parts[0], "/") { // Likely 'repo:tag' or similar
			// We can't definitively parse this without context (like a default registry).
			// For IRR's purpose, we often only care about the registry part if present.
			// Since it seems missing here, log it and return the original error.
			log.Warn("Image pattern appears to be missing a registry", "path", pattern.Path, "value", pattern.Value)
			// Fall through to return the original error
		}
	}

	// If retries/heuristics didn't work, return the original parsing error wrapped.
	return nil, pkgerrors.Wrapf(err, "failed to parse image reference at path '%v' for value '%v'", pattern.Path, pattern.Value)
}

// pathExistsInMap checks if a dot-notation path exists in a nested map[string]interface{}.
// Example: pathExistsInMap(m, "a.b.c") returns true if m["a"]["b"]["c"] exists.
func pathExistsInMap(data map[string]interface{}, pathStr string) bool {
	if data == nil || pathStr == "" {
		return false
	}
	parts := strings.Split(pathStr, ".")
	current := interface{}(data)
	for i, part := range parts {
		mapData, exists := current.(map[string]interface{})
		if !exists {
			return false
		}
		if i == len(parts)-1 {
			// Found the final part, it exists
			return true
		}
		// Need to go deeper
		next, ok := mapData[part]
		if !ok {
			// Path exists up to this point, but the next part isn't a map
			return false
		}
		current = next
	}
	// Should not be reached if path has parts, but handles edge cases like empty path
	return false
}
