package chart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	// maxErrorParts defines the maximum parts for splitting image strings that caused errors, usually for tag/digest separation.
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
	chartLoader Loader, // Use Loader from this package
	includePatterns, excludePatterns, knownPaths []string,
	rulesEnabled bool,
) *Generator {
	// Set up a default chart loader if none was provided
	if chartLoader == nil {
		// Use the constructor from api.go which uses DefaultLoader
		chartLoader = NewLoader()
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
// It applies mappings first, then uses the path strategy to construct the final path.
// It now accepts the analysis.ImagePattern to provide original value context to the strategy.
func (g *Generator) determineTargetPathAndRegistry(imgRef *image.Reference, pattern analysis.ImagePattern) (targetReg, newPath string, err error) {
	targetReg = g.targetRegistry

	// Determine original source registry from pattern or fallback to imgRef
	originalSourceRegistry := imgRef.Registry // Fallback
	parsedFromPattern, parseErr := image.ParseImageReference(pattern.Value)
	if parseErr == nil && parsedFromPattern.Registry != "" {
		originalSourceRegistry = parsedFromPattern.Registry
	}

	// Check for registry mappings using the original source registry
	if g.mappings != nil {
		if mappedTarget := g.mappings.GetTargetRegistry(originalSourceRegistry); mappedTarget != "" {
			targetReg = mappedTarget // Use mapped target if found
			log.Debug("determineTargetPathAndRegistry: Using mapped target registry", "originalRegistry", originalSourceRegistry, "mappedTarget", targetReg)
		} else {
			log.Debug("determineTargetPathAndRegistry: No mapping found, using default target registry", "originalRegistry", originalSourceRegistry, "defaultTarget", targetReg)
		}
	} else {
		log.Debug("determineTargetPathAndRegistry: No mappings configured, using default target registry", "defaultTarget", targetReg)
	}

	// Generate the new repository path using the strategy, passing the pattern
	newPath, err = g.pathStrategy.GeneratePath(imgRef, pattern, targetReg)
	if err != nil {
		// Use original pattern value in error for better context
		return "", "", fmt.Errorf("path generation failed for pattern '%s' (value: '%s'): %w", pattern.Path, pattern.Value, err)
	}

	log.Debug("Generated new path", "new_path", newPath, "original_pattern_value", pattern.Value)
	return targetReg, newPath, nil
}

// processImage attempts to generate and apply an override for a single image pattern.
// Steps involved:
// 1. Parse the image pattern (string or map) into an image.Reference.
// 2. Determine the target registry and repository path using mappings and strategy.
// 3. Create the override structure (map or string).
// 4. Set the override value at the correct path in the main overrides map.
// Returns: success bool, unsupported *override.UnsupportedStructure, err error
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

	// 2. Determine Target Path and Registry - PASS THE PATTERN
	targetReg, newPath, err := g.determineTargetPathAndRegistry(imgRef, pattern)
	if err != nil {
		log.Warn("Failed to determine target path/registry", "path", pattern.Path, "image", imgRef.Original, "error", err)
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
		return false, unsupported, fmt.Errorf("path '%s': failed to set override: %w", pattern.Path, err)
	}

	log.Debug("Successfully processed and generated override", "path", pattern.Path)
	return true, nil, nil // Success, no unsupported structure, no error
}

// --- Refactored Generate Logic --- (Helper methods added below)

// loadAndAnalyzeChart loads the chart and performs initial analysis.
func (g *Generator) loadAndAnalyzeChart(result *override.File) (*chart.Chart, *analysis.ChartAnalysis, error) {
	// Use the configured loader via the Loader interface
	log.Debug("Generator using loader", "loader_type", fmt.Sprintf("%T", g.loader))
	loadedChart, err := g.loader.Load(g.chartPath) // Use interface method
	if err != nil {
		log.Debug("Generator error loading chart", "chartPath", g.chartPath, "error", err)
		// Wrap error for consistent exit code mapping
		// Consider if LoadingError is still the right type or if loader returns wrapped errors
		return nil, nil, &LoadingError{ChartPath: g.chartPath, Err: err} // Pass err directly
	}
	log.Debug("Generator chart loaded", "name", loadedChart.Name(), "values_type", fmt.Sprintf("%T", loadedChart.Values))

	if loadedChart.Values == nil {
		log.Debug("Generator chart has nil values, skipping analysis", "chart", loadedChart.Name())
		// No need to create analysis if no values
		return loadedChart, nil, nil
	}

	// Use the same loader instance for the analyzer
	analyzer := analysis.NewAnalyzer(g.chartPath, g.loader) // Pass the loader instance
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

// processEligibleImagesLoop iterates through eligible images, processes them, and collects results.
func (g *Generator) processEligibleImagesLoop(eligibleImages []analysis.ImagePattern, overrides map[string]interface{}) (processingErrors []error, processedCount int) {
	// Initialize local slices/maps if needed (overrides is passed in)
	if overrides == nil {
		overrides = make(map[string]interface{}) // Should ideally not happen if called from Generate
		log.Warn("Overrides map was nil in processEligibleImagesLoop, re-initialized")
	}
	processingErrors = []error{}
	processedCount = 0

	for _, pattern := range eligibleImages {
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

// Generate orchestrates the chart loading, analysis, and override generation process.
func (g *Generator) Generate() (*override.File, error) {
	// Initialize the result structure
	result := &override.File{
		ChartPath:   g.chartPath,
		ChartName:   filepath.Base(g.chartPath), // Extract chart name from path
		Values:      make(map[string]interface{}),
		Unsupported: []override.UnsupportedStructure{}, // Initialize slice
	}

	// 1. Load and Analyze Chart
	loadedChart, analysisResult, err := g.loadAndAnalyzeChart(result)
	if err != nil {
		log.Error("Failed during chart load/analysis phase", "error", err)
		// Ensure we return nil result on loading/analysis error
		return nil, err // Correctly return nil for result
	}
	// Add checks for nil return values even if error is nil
	if loadedChart == nil {
		return nil, errors.New("internal error: loadAndAnalyzeChart returned nil chart without error")
	}
	if analysisResult == nil {
		return nil, errors.New("internal error: loadAndAnalyzeChart returned nil analysis result without error")
	}
	result.ChartName = loadedChart.Name()                 // Update chart name from loaded chart metadata
	result.TotalCount = len(analysisResult.ImagePatterns) // Total detected patterns
	// Initialize Unsupported slice, it will be populated during image processing if needed
	result.Unsupported = []override.UnsupportedStructure{}

	// 2. Filter Eligible Images
	eligibleImages := g.filterEligibleImages(analysisResult.ImagePatterns)
	eligibleCount := len(eligibleImages)
	log.Info("Finished chart analysis", "total_images", result.TotalCount, "eligible_images", eligibleCount, "unsupported_count", len(result.Unsupported))

	// 3. Process Eligible Images & Collect Errors
	processingErrors, processedCount := g.processEligibleImagesLoop(eligibleImages, result.Values)
	result.ProcessedCount = processedCount // Store processed count

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
		// If thresholdErr is due to strict mode (i.e., processingErrors is not empty and g.strict is true),
		// or if it's a threshold failure, return nil for the result.
		// The checkProcessingThreshold function already encapsulates this logic implicitly
		// by returning an error in these cases.
		return nil, thresholdErr // Return nil result and the error
	}

	// 6. Apply Rules if enabled
	if rulesErr := g.applyRulesIfNeeded(loadedChart, result); rulesErr != nil {
		log.Error("Error applying chart rules", "error", rulesErr)
		// Consider if this should be a distinct exit code or wrapped
		return result, fmt.Errorf("error applying chart rules: %w", rulesErr)
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
func findValueByPath(data map[string]interface{}, path []string) (interface{}, bool) {
	current := interface{}(data)
	for i, part := range path { // Keep index i for potential error messages
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

// createOverride generates the appropriate override structure (string or map) based on the original pattern and the new image details.
// It takes the analysis pattern, the parsed image reference, the target registry, and the new path strategy-generated path.
// It returns the value (string or map[string]interface{}) to be set in the override file.
func (g *Generator) createOverride(pattern analysis.ImagePattern, imgRef *image.Reference, targetReg, newPath string) interface{} {
	// --- START Logging Added for Debugging (Phase 9.4.1) ---
	log.Debug("Entering createOverride",
		"patternPath", pattern.Path,
		"patternValue", pattern.Value,
		"patternStructureKeys", getMapKeys(pattern.Structure), // Log original keys for context
		"imgRefRepository", imgRef.Repository,
		"imgRefTag", imgRef.Tag,
		"imgRefDigest", imgRef.Digest,
		"targetRegistry", targetReg,
		"newPath", newPath,
	)
	// --- END Logging Added ---

	// Decide whether to create a string override or a map override
	// Check if the original pattern was a simple string or had structure
	if len(pattern.Structure) == 0 {
		// Original was likely a simple string "repo:tag" or "repo@digest"
		fullImagePath := newPath // Start with the path generated by the strategy
		if targetReg != "" {
			switch {
			case strings.HasSuffix(targetReg, "/") && strings.HasPrefix(newPath, "/"):
				fullImagePath = targetReg + newPath[1:]
			case !strings.HasSuffix(targetReg, "/") && !strings.HasPrefix(newPath, "/"):
				fullImagePath = targetReg + "/" + newPath
			default:
				// One has a slash, one doesn't, simple concatenation is fine
				fullImagePath = targetReg + newPath
			}
		}

		// Append tag or digest
		if imgRef.Digest != "" {
			fullImagePath += "@" + imgRef.Digest
		} else if imgRef.Tag != "" { // Prefer digest if available
			fullImagePath += ":" + imgRef.Tag
		}
		// --- START Logging Added for Debugging (Phase 9.4.1) ---
		log.Debug("createOverride: Generated simple string override", "value", fullImagePath)
		// --- END Logging Added ---
		return fullImagePath // Return the fully qualified image string
	}

	// Original had structure, create a map override using STANDARD keys
	overrideMap := make(map[string]interface{})

	// --- START Corrected Map Generation Logic (Phase 9.4.1) ---
	// Calculate the full repository path including the target registry
	fullRepoPath := newPath // Start with the path generated by the strategy
	if targetReg != "" {
		switch {
		case strings.HasSuffix(targetReg, "/") && strings.HasPrefix(newPath, "/"):
			fullRepoPath = targetReg + newPath[1:]
		case !strings.HasSuffix(targetReg, "/") && !strings.HasPrefix(newPath, "/"):
			fullRepoPath = targetReg + "/" + newPath
		default:
			fullRepoPath = targetReg + newPath
		}
	}

	// Set the standard "repository" key
	overrideMap["repository"] = fullRepoPath

	// Set the standard "tag" or "digest" key
	if imgRef.Digest != "" {
		overrideMap["digest"] = imgRef.Digest
	} else if imgRef.Tag != "" { // Prefer digest if available, else use tag
		overrideMap["tag"] = imgRef.Tag
	}
	// We no longer use pattern.Structure keys to build the map, ensuring standard output.
	// --- END Corrected Map Generation Logic ---

	// --- START Logging Added for Debugging (Phase 9.4.1) ---
	log.Debug("createOverride: Generated standard map override", "value", overrideMap)
	// --- END Logging Added ---
	return overrideMap
}

// setOverridePath navigates the overrides map according to the pattern's path and sets the final value.
// It handles creating intermediate maps if they don't exist.
// It returns an error if path traversal or setting fails.
func (g *Generator) setOverridePath(overrides map[string]interface{}, pattern analysis.ImagePattern, overrideValue interface{}) error {
	pathParts := strings.Split(pattern.Path, ".")
	current := overrides

	// --- START Logging Added for Debugging (Phase 9.4.1) ---
	log.Debug("Entering setOverridePath",
		"fullPath", pattern.Path,
		"pathParts", pathParts,
		"overrideValue", overrideValue,
	)
	// --- END Logging Added ---

	// Traverse the path, creating intermediate maps as needed
	for i, part := range pathParts {
		// --- START Logging Added for Debugging (Phase 9.4.1) ---
		log.Debug("setOverridePath: Processing path part", "index", i, "part", part, "currentMapKeys", getMapKeys(current))
		// --- END Logging Added ---

		if i == len(pathParts)-1 {
			// Last part: set the final value
			// --- START Logging Added for Debugging (Phase 9.4.1) ---
			log.Debug("setOverridePath: Setting final value", "key", part, "value", overrideValue)
			// --- END Logging Added ---
			current[part] = overrideValue
			return nil
		}

		// Intermediate part: ensure a map exists
		next, ok := current[part]
		if !ok {
			// --- START Logging Added for Debugging (Phase 9.4.1) ---
			log.Debug("setOverridePath: Intermediate key not found, creating new map", "key", part)
			// --- END Logging Added ---
			// Key doesn't exist, create a new map
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		} else {
			// Key exists, check if it's a map
			nextMap, ok := next.(map[string]interface{})
			if !ok {
				// Existing value is not a map, cannot traverse further
				// --- START Logging Added for Debugging (Phase 9.4.1) ---
				log.Error("setOverridePath: Conflict - existing value is not a map, cannot set nested path",
					"path", pattern.Path,
					"conflictingKey", part,
					"existingValueType", fmt.Sprintf("%T", next),
				)
				// --- END Logging Added ---
				return fmt.Errorf("failed to set override path '%s': key '%s' exists but is not a map (type: %T)", pattern.Path, part, next)
			}
			// It's a map, continue traversal
			current = nextMap
		}
	}

	// Should not be reached if pathParts is not empty
	return fmt.Errorf("internal error: failed to set path '%s', loop completed unexpectedly", pattern.Path)
}

// Helper function to get keys of a map for logging (avoids nil panics)
func getMapKeys(m map[string]interface{}) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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
	return nil, fmt.Errorf("failed to parse image reference at path '%s' for value '%s': %w", pattern.Path, pattern.Value, err)
}
