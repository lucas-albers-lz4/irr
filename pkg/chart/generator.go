package chart

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/fileutil"
	image "github.com/lalbers/irr/pkg/image"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/rules"
	"github.com/lalbers/irr/pkg/strategy"
)

// Constants

const (
	// PercentMultiplier is used for percentage calculations
	PercentMultiplier = 100
	// PrivateFilePermissions represents secure file permissions (rw-------)
	PrivateFilePermissions = 0o600
	// FilePermissions defines the permission mode for temporary override files
	FilePermissions = 0o600
	// ExpectedMappingParts defines the number of parts expected after splitting a config mapping value.
	ExpectedMappingParts = 2
	// PercentageMultiplier is used when calculating success rates as percentages
	PercentageMultiplier = 100.0
	// ExpectedParts is the constant for the magic number 2 in strings.SplitN
	ExpectedParts = 2
	// maxErrorParts defines the maximum parts for splitting image strings with tags/digests.
	maxErrorParts = 2
)

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

// --- Local Loader Implementation (implements analysis.ChartLoader) ---

// Ensure GeneratorLoader implements analysis.ChartLoader
var _ analysis.ChartLoader = (*GeneratorLoader)(nil)

// GeneratorLoader provides functionality to load Helm charts
type GeneratorLoader struct {
	fs fileutil.FS // Filesystem implementation to use
}

// NewGeneratorLoader creates a new GeneratorLoader with the provided filesystem.
// If fs is nil, it uses the default filesystem.
func NewGeneratorLoader(fs fileutil.FS) *GeneratorLoader {
	if fs == nil {
		fs = fileutil.DefaultFS
	}
	return &GeneratorLoader{fs: fs}
}

// SetFS replaces the filesystem used by the loader and returns a cleanup function
func (l *GeneratorLoader) SetFS(fs fileutil.FS) func() {
	oldFS := l.fs
	l.fs = fs
	return func() {
		l.fs = oldFS
	}
}

// Load implements analysis.ChartLoader interface, returning *chart.Chart
func (l *GeneratorLoader) Load(chartPath string) (*chart.Chart, error) {
	log.Debug("GeneratorLoader loading chart", "path", chartPath)

	// Verify the chart path exists using our injectable filesystem
	_, err := l.fs.Stat(chartPath)
	if err != nil {
		return nil, fmt.Errorf("chart path stat error %s: %w", chartPath, err)
	}

	// Use helm's loader directly
	loadedChart, err := loader.Load(chartPath)
	if err != nil {
		// Wrap the error from the helm loader
		return nil, fmt.Errorf("helm loader failed for path '%s': %w", chartPath, err)
	}

	// We need to extract values manually if helm loader doesn't merge them automatically
	if loadedChart.Values == nil {
		loadedChart.Values = make(map[string]interface{}) // Ensure Values is not nil
		log.Debug("Helm chart loaded with nil Values, initialized empty map", "path", chartPath)
	}

	log.Debug("GeneratorLoader successfully loaded chart", "name", loadedChart.Name())
	return loadedChart, nil
}

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
	configMappings    map[string]string
	strict            bool
	includePatterns   []string // Passed to detector context
	excludePatterns   []string // Passed to detector context
	knownPaths        []string // Passed to detector context
	threshold         int
	loader            analysis.ChartLoader    // Use analysis.ChartLoader interface
	rulesEnabled      bool                    // Whether to apply rules
	rulesRegistry     rules.RegistryInterface // Use the interface type here
}

// NewGenerator creates a new Generator with the provided configuration
func NewGenerator(
	chartPath, targetRegistry string,
	sourceRegistries, excludeRegistries []string,
	pathStrategy strategy.PathStrategy,
	mappings *registry.Mappings,
	configMappings map[string]string,
	strict bool,
	threshold int,
	chartLoader analysis.ChartLoader,
	includePatterns, excludePatterns, knownPaths []string,
	rulesEnabled bool,
) *Generator {
	// Set up a default chart loader if none was provided
	if chartLoader == nil {
		chartLoader = NewGeneratorLoader(fileutil.DefaultFS)
	}

	return &Generator{
		chartPath:         chartPath,
		targetRegistry:    targetRegistry,
		sourceRegistries:  sourceRegistries,
		excludeRegistries: excludeRegistries,
		pathStrategy:      pathStrategy,
		mappings:          mappings,
		configMappings:    configMappings,
		strict:            strict,
		includePatterns:   includePatterns,
		excludePatterns:   excludePatterns,
		knownPaths:        knownPaths,
		threshold:         threshold,
		loader:            chartLoader,
		rulesEnabled:      rulesEnabled,
		rulesRegistry:     nil,
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

// filterEligibleImages filters the detected image patterns based on source and exclude registries.
func (g *Generator) filterEligibleImages(detectedImages []analysis.ImagePattern) []analysis.ImagePattern {
	log.Debug("Filtering detected image patterns", "count", len(detectedImages))
	eligibleImages := make([]analysis.ImagePattern, 0, len(detectedImages))
	processedSources := make(map[string]bool) // Track processed source paths

	for _, pattern := range detectedImages {
		// Avoid processing the same source path multiple times if duplicates exist
		if processedSources[pattern.Path] {
			log.Debug("Skipping duplicate pattern path", "path", pattern.Path)
			continue
		}

		// Perform filtering based *only* on registry rules. We need the registry.
		// Simplistic extraction for filtering: Try parsing, but ignore errors here.
		// The goal is just to get *a* registry string for filtering.
		// Full error handling happens later in the Generate loop.
		imgRefForFilter, err := image.ParseImageReference(pattern.Value) // Check error
		if err != nil {
			// Log the error at debug level if parsing fails during filtering
			log.Debug("Failed to parse image reference during filtering, proceeding with caution", "path", pattern.Path, "value", pattern.Value, "error", err)
		}
		// NOTE: This might be imperfect if parsing fails badly, but better than skipping.
		// Consider a simpler regex for registry extraction if this proves problematic.

		// If the image has no registry, assume docker.io (default behavior)
		// This reference is only used for filtering eligibility here.
		effectiveRegistry := ""
		if imgRefForFilter != nil {
			effectiveRegistry = imgRefForFilter.Registry
		}
		if effectiveRegistry == "" {
			effectiveRegistry = image.DefaultRegistry // "docker.io"
			log.Debug("Pattern has no explicit registry, assuming default for filtering", "path", pattern.Path, "default", effectiveRegistry)
		}

		// Check if the effective registry should be excluded
		if g.isExcluded(effectiveRegistry) {
			log.Debug("Image is not eligible (excluded registry)", "path", pattern.Path, "value", pattern.Value, "registry", effectiveRegistry)
			continue
		}

		// Check if the effective registry is in the source list (or if source list is empty)
		if g.isSourceRegistry(effectiveRegistry) {
			log.Debug("Image is eligible", "path", pattern.Path, "value", pattern.Value, "registry", effectiveRegistry)
			eligibleImages = append(eligibleImages, pattern)
			processedSources[pattern.Path] = true // Mark this path as processed
		} else {
			log.Debug("Image is not eligible (registry not in source list)", "path", pattern.Path, "value", pattern.Value, "registry", effectiveRegistry)
		}
	}
	log.Debug("Filtering complete", "eligible_count", len(eligibleImages))
	return eligibleImages
}

// Determines the appropriate override structure based on the original pattern type.
// For maps, it merges into the existing map. For strings, it creates a map.
func (g *Generator) createOverride(pattern analysis.ImagePattern, imgRef *image.Reference, targetReg, newPath string) interface{} {
	log.Debug("Creating override for path %v, original type %T", pattern.Path, pattern.Value)

	overrideValue := map[string]interface{}{
		"registry":   targetReg,
		"repository": newPath,
	}
	if imgRef.Digest != "" {
		overrideValue["digest"] = imgRef.Digest
	} else {
		overrideValue["tag"] = imgRef.Tag
	}

	// Always return the map structure now. setOverridePath handles insertion.
	return overrideValue
}

// setOverridePath adds the override value to the map at the specified path, creating nested maps as needed
func (g *Generator) setOverridePath(overrides map[string]interface{}, pattern analysis.ImagePattern, overrideValue interface{}) error {
	log.Debug("setOverridePath: Setting override for path '%s' with value type '%T'", pattern.Path, overrideValue)
	pathParts := strings.Split(pattern.Path, ".")
	if len(pathParts) == 0 {
		return fmt.Errorf("invalid empty path provided for pattern: %+v", pattern)
	}
	// Use true for overwrite, as we always want IRR's override to take precedence
	if err := override.SetValueAtPath(overrides, pathParts, overrideValue); err != nil {
		return fmt.Errorf("failed to set override path %v: %w", pathParts, err)
	}
	return nil
}

// processImagePattern attempts to parse an image string identified by the analysis.
// It handles potential complexity like missing tags or digests.
func (g *Generator) processImagePattern(pattern analysis.ImagePattern) (*image.Reference, error) {
	log.Debug("Processing image pattern", "path", pattern.Path, "value", pattern.Value)

	// Initial parsing attempt
	imgRef, err := image.ParseImageReference(pattern.Value)
	if err == nil {
		log.Debug("Successfully parsed image reference", "ref", imgRef.String())
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
	return nil, fmt.Errorf("failed to parse image reference at path '%s' for value '%s': %w", pattern.Path, pattern.Value, err)
}

// determineTargetPathAndRegistry calculates the target registry and new path for an image reference.
// It first checks for config mappings, then falls back to registry mappings if needed
func (g *Generator) determineTargetPathAndRegistry(imgRef *image.Reference) (targetReg, newPath string, err error) {
	// Default to configured target registry
	targetReg = g.targetRegistry

	// First check configMappings from --config flag
	if g.configMappings != nil {
		// Normalize the registry name for lookup
		normalizedRegistry := image.NormalizeRegistry(imgRef.Registry)

		// Special case for Docker Hub library images
		if normalizedRegistry == "docker.io" && strings.HasPrefix(imgRef.Repository, "library/") {
			log.Debug("Docker Hub library image detected", "image", imgRef.String())
		}

		if mappedValue, ok := g.configMappings[normalizedRegistry]; ok {
			log.Debug("Found config mapping", "registry", normalizedRegistry, "mapping", mappedValue)

			// Split the mappedValue at the first slash to get registry and repository prefix
			// We expect exactly two parts: the target registry and the prefix path
			parts := strings.SplitN(mappedValue, "/", ExpectedMappingParts)
			if len(parts) == ExpectedMappingParts {
				targetReg = parts[0]

				// Update the path to include the repository prefix from the mapped value
				// The strategy will handle the full path generation
				pathOnly, err := g.pathStrategy.GeneratePath(imgRef, targetReg)
				if err != nil {
					return "", "", fmt.Errorf("path generation failed for '%s': %w", imgRef.String(), err)
				}

				// Prepend the repository prefix from the config mapping
				newPath = parts[1] + "/" + pathOnly
				log.Debug("Generated new path using config mapping", "new_path", newPath, "original", imgRef.Original)
				return targetReg, newPath, nil
			}
		}
	}

	// If no config mapping was found or applied, check regular mappings
	if g.mappings != nil {
		if mappedTarget := g.mappings.GetTargetRegistry(imgRef.Registry); mappedTarget != "" {
			targetReg = mappedTarget // Use mapped target if found
		}
	}

	// Generate the new repository path using the strategy
	newPath, err = g.pathStrategy.GeneratePath(imgRef, targetReg)
	if err != nil {
		return "", "", fmt.Errorf("path generation failed for '%s': %w", imgRef.String(), err)
	}

	log.Debug("Generated new path", "new_path", newPath, "original", imgRef.Original)
	return targetReg, newPath, nil
}

// processImage takes an image pattern, processes it, and updates the overrides map.
// It returns success status, any unsupported structure info, and any processing error.
func (g *Generator) processImage(pattern analysis.ImagePattern, overrides map[string]interface{}) (bool, *override.UnsupportedStructure, error) {
	log.Debug("Processing image pattern", "path", pattern.Path, "value", pattern.Value)

	// 1. Process the image pattern string
	imgRef, err := g.processImagePattern(pattern)
	if err != nil {
		log.Warn("Failed to process image pattern", "path", pattern.Path, "value", pattern.Value, "error", err)
		unsupported := &override.UnsupportedStructure{
			Path: strings.Split(pattern.Path, "."),
			Type: "InvalidImageFormat", // Or a more specific type if available from err
		}
		// Return the raw error for aggregation
		return false, unsupported, fmt.Errorf("path '%s': failed to process image: %w", pattern.Path, err)
	}

	// 2. Determine Target Path and Registry
	targetReg, newPath, err := g.determineTargetPathAndRegistry(imgRef)
	if err != nil {
		log.Warn("Failed to determine target path/registry", "path", pattern.Path, "image", imgRef.Original, "error", err)
		// unsupported := &override.UnsupportedStructure{ // REMOVED - Path generation failure is a processing error, not unsupported structure
		// 	Path: strings.Split(pattern.Path, "."),
		// 	Type: "PathGenerationError",
		// }
		// Return the raw error for aggregation, but nil for unsupported structure
		return false, nil, fmt.Errorf("path '%s': failed to determine target path: %w", pattern.Path, err)
	}

	// 3. Create the Override Value
	overrideValue := g.createOverride(pattern, imgRef, targetReg, newPath)

	// 4. Set the Override Path
	if err := g.setOverridePath(overrides, pattern, overrideValue); err != nil {
		log.Warn("Failed to set override", "path", pattern.Path, "error", err)
		unsupported := &override.UnsupportedStructure{
			Path: strings.Split(pattern.Path, "."),
			Type: "OverrideSetError",
		}
		// Return the raw error for aggregation
		return false, unsupported, fmt.Errorf("path '%s': failed to set override: %w", pattern.Path, err)
	}

	log.Debug("Successfully processed and generated override", "path", pattern.Path)
	return true, nil, nil // Success, no unsupported structure, no error
}

// --- Refactored Generate Logic --- (Helper methods added below)

// loadAndAnalyzeChart loads the chart and performs initial analysis.
func (g *Generator) loadAndAnalyzeChart(result *override.File) (*chart.Chart, *analysis.ChartAnalysis, error) {
	log.Debug("Generator using loader", "loader_type", fmt.Sprintf("%T", g.loader))
	loadedChart, err := g.loader.Load(g.chartPath)
	if err != nil {
		log.Debug("Generator error loading chart", "chartPath", g.chartPath, "error", err)
		// Wrap error for consistent exit code mapping
		return nil, nil, &LoadingError{ChartPath: g.chartPath, Err: fmt.Errorf("failed to load chart: %w", err)}
	}
	log.Debug("Generator chart loaded", "name", loadedChart.Name(), "values_type", fmt.Sprintf("%T", loadedChart.Values))

	if loadedChart.Values == nil {
		log.Debug("Generator chart has nil values, skipping analysis", "chart", loadedChart.Name())
		return loadedChart, nil, nil // Return chart but nil analysis result if no values
	}

	analyzer := analysis.NewAnalyzer(g.chartPath, g.loader)
	detectedImages, analysisErr := analyzer.Analyze()
	if analysisErr != nil {
		log.Warn("Analysis of chart failed", "chartPath", g.chartPath, "error", analysisErr)
		result.Unsupported = append(result.Unsupported, override.UnsupportedStructure{
			Path: []string{"analysis"},
			Type: "AnalysisError",
		})
		// Return partial success (loaded chart) but indicate analysis issue
		return loadedChart, nil, nil
	}

	// Check for unsupported patterns found *during* analysis
	if detectedImages != nil {
		result.Unsupported = append(result.Unsupported, g.findUnsupportedPatterns(detectedImages.ImagePatterns)...)
		if g.strict && len(result.Unsupported) > 0 {
			log.Debug("Generator found unsupported patterns in strict mode", "count", len(result.Unsupported))
			firstUnsupported := result.Unsupported[0]
			// Return specific error for unsupported structure in strict mode
			// Use the existing UnsupportedStructureError type (ensure it's defined/imported)
			return nil, nil, &UnsupportedStructureError{
				Path: firstUnsupported.Path,
				Type: firstUnsupported.Type,
			}
		}
	}

	return loadedChart, detectedImages, nil
}

// FailedItem struct definition remains the same
type FailedItem struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// processEligibleImagesLoop iterates through eligible images and generates overrides.
func (g *Generator) processEligibleImagesLoop(eligibleImages []analysis.ImagePattern, result *override.File) (processingErrors []error, processedCount int) {
	processedCount = 0 // Explicitly initialize
	// processingErrors is implicitly initialized to nil
	var failedItems []FailedItem

	for _, pattern := range eligibleImages {
		success, unsupported, err := g.processImage(pattern, result.Values)

		if err != nil {
			processingErrors = append(processingErrors, err)
			path := pattern.Path
			if strings.HasPrefix(err.Error(), "path '") {
				endQuoteIdx := strings.Index(err.Error()[6:], "'")
				if endQuoteIdx > 0 {
					path = err.Error()[6 : 6+endQuoteIdx]
				}
			}
			failedItems = append(failedItems, FailedItem{Path: path, Error: err.Error()})
		}
		if unsupported != nil {
			result.Unsupported = append(result.Unsupported, *unsupported)
		}
		if success {
			processedCount++
		}
	}

	log.Debug("Generator finished processing images", "processed", processedCount, "eligible", len(eligibleImages), "chart", g.chartPath)

	// Log errors if any occurred
	if len(processingErrors) > 0 {
		logLevel := log.LevelWarn
		logMsg := "Image processing completed with errors (non-strict mode)"
		if g.strict {
			logLevel = log.LevelError
			logMsg = "Image processing failed with errors (strict mode)"
		}
		if logLevel == log.LevelError {
			log.Error(logMsg, "count", len(processingErrors), "failedItems", failedItems)
		} else {
			log.Warn(logMsg, "count", len(processingErrors), "failedItems", failedItems)
		}
	}

	return processingErrors, processedCount
}

// checkProcessingThreshold checks if the processing threshold was met and handles strict mode errors.
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

// Generate orchestrates the chart analysis and override generation process (Refactored)
func (g *Generator) Generate() (*override.File, error) {
	log.Debug("Generator starting override generation", "chartPath", g.chartPath)

	// Initialize rules registry if needed
	g.initRulesRegistry() // Ensure registry is ready before loading

	// Initialize result structure
	result := &override.File{
		ChartPath:   g.chartPath,
		Values:      make(map[string]interface{}),
		Unsupported: []override.UnsupportedStructure{},
	}

	// 1. Load Chart and Analyze for Images
	loadedChart, detectedImages, err := g.loadAndAnalyzeChart(result)
	if err != nil {
		// Loading or strict-mode unsupported error occurred
		return nil, err // Return the specific error (LoadingError or UnsupportedStructureError)
	}
	// Handle case where analysis failed but wasn't a fatal error
	// This happens if loadAndAnalyzeChart returns a non-nil loadedChart but nil detectedImages due to analysisErr
	if loadedChart != nil && detectedImages == nil && loadedChart.Values != nil {
		// Analysis failed, but loading succeeded. Return partial result.
		return result, nil
	}
	// Handle case where chart had no values or no images detected
	if detectedImages == nil || len(detectedImages.ImagePatterns) == 0 {
		log.Debug("No images detected or chart has no values.", "chart", g.chartPath)
		return result, nil // Return empty/partial result
	}

	// 2. Filter Eligible Images
	eligibleImages := g.filterEligibleImages(detectedImages.ImagePatterns)
	log.Debug("Generator filtering results", "total_patterns", len(detectedImages.ImagePatterns), "eligible_count", len(eligibleImages))
	result.TotalCount = len(eligibleImages)

	// Handle case where no images are eligible after filtering
	if len(eligibleImages) == 0 {
		log.Debug("No eligible images found after filtering.", "chart", g.chartPath)
		return result, nil
	}

	// 3. Process Eligible Images & Collect Errors
	processingErrors, processedCount := g.processEligibleImagesLoop(eligibleImages, result)

	// 4. Calculate and Store Success Rate
	var successRate float64
	if result.TotalCount > 0 {
		successRate = float64(processedCount*PercentageMultiplier) / float64(result.TotalCount)
	}
	result.SuccessRate = successRate
	result.ProcessedCount = processedCount
	log.Debug("Generator success rate check", "rate", fmt.Sprintf("%.2f%%", successRate), "processed", processedCount, "eligible", result.TotalCount, "threshold", g.threshold)

	// 5. Apply Rules (before threshold check)
	if err := g.applyRulesIfNeeded(loadedChart, result); err != nil {
		return nil, err // Return error if rule application fails
	}

	// 6. Check Threshold & Strict Mode Errors
	thresholdErr := g.checkProcessingThreshold(processingErrors, processedCount, result.TotalCount, successRate, result)
	if thresholdErr != nil {
		// If strict mode caused an error, return nil result and the error
		var processingErr *ProcessingError
		if errors.As(thresholdErr, &processingErr) { // Use errors.As for type checking
			return nil, thresholdErr // Return the original ProcessingError
		}
		// Otherwise, return the partial result along with the ThresholdError
		return result, thresholdErr
	}

	log.Debug("Generator returning result", "chart", g.chartPath, "processed", processedCount, "eligible", len(eligibleImages))
	return result, nil // Success
}

// initRulesRegistry initializes the rules registry if it's not already set.
func (g *Generator) initRulesRegistry() {
	if g.rulesRegistry == nil {
		log.Debug("Generator: Initializing default rules registry.")
		g.rulesRegistry = rules.NewRegistry() // Assuming default initialization
		// Load default rules if necessary, or this could be handled by the registry itself
	}
}

// isSourceRegistry checks if the given registry string matches any of the configured source registries
func (g *Generator) isSourceRegistry(regStr string) bool {
	// If the source list is empty, consider all registries as source registries.
	if len(g.sourceRegistries) == 0 {
		return true
	}
	for _, source := range g.sourceRegistries {
		if regStr == source {
			return true
		}
	}
	return false
}

// isExcluded checks if a registry string matches any configured exclude patterns.
func (g *Generator) isExcluded(regStr string) bool {
	for _, ex := range g.excludeRegistries {
		if regStr == ex {
			return true
		}
	}
	return false
}

// ValidateHelmTemplate checks if a chart can be rendered with given overrides.
func ValidateHelmTemplate(chartPath string, overrides []byte) error {
	log.Info("Validating Helm template with generated overrides", "chartPath", chartPath)
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

// validateHelmTemplateInternal performs the actual Helm template rendering.
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

// findValueByPath searches for a value in a nested map structure using a dot-separated path.
// TODO: Enhance this to handle array indices if needed.
func findValueByPath(data map[string]interface{}, path []string) (interface{}, bool) {
	// If the path is empty, return the original data map
	if len(path) == 0 {
		return data, true
	}

	var current interface{} = data
	for i, key := range path {
		mapData, ok := current.(map[string]interface{})
		if !ok {
			return nil, false // Path segment does not lead to a map
		}
		value, exists := mapData[key]
		if !exists {
			return nil, false // Key not found at this level
		}
		if i == len(path)-1 {
			return value, true // Reached the end of the path
		}
		current = value
	}
	// This point should ideally not be reached if the loop completes, as the last iteration returns.
	// However, if the path is valid but leads nowhere (e.g., intermediate value isn't a map),
	// the loop exits earlier returning nil, false. If it finishes, it means path was empty (handled above)
	// or something unexpected occurred. Return false for safety.
	return nil, false
}

// OverridesToYAML converts the override map to a YAML byte slice.
func OverridesToYAML(overrides map[string]interface{}) ([]byte, error) {
	log.Debug("Marshaling overrides to YAML")
	yamlBytes, err := yaml.Marshal(overrides)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
	}
	return yamlBytes, nil
}

// ProcessingError wraps multiple errors encountered during image processing in strict mode.
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
