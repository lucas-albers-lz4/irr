package chart

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/release"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/lalbers/irr/pkg/image"
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

// Assuming ParsingError and ImageProcessingError are defined elsewhere (e.g., analysis package or pkg/errors)

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
	log.Debug("GeneratorLoader: Loading chart from %s", chartPath)

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
		log.Debug("Helm chart loaded with nil Values, initialized empty map for %s", chartPath)
	}

	log.Debug("GeneratorLoader successfully loaded chart: %s", loadedChart.Name())
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

// filterEligibleImages filters detected images based on registry rules
func (g *Generator) filterEligibleImages(detectedImages []analysis.ImagePattern) []analysis.ImagePattern {
	eligibleImages := []analysis.ImagePattern{}
	for _, pattern := range detectedImages {
		var imgRegistry string

		switch pattern.Type {
		case analysis.PatternTypeString:
			// Best-effort registry extraction for filtering, without failing on parse error
			if strings.Contains(pattern.Value, "/") {
				parts := strings.SplitN(pattern.Value, "/", ExpectedParts)
				// Basic check: assume first part is registry if it contains '.' or ':' (like a domain or port)
				// This avoids treating paths like 'my/image' as having registry 'my'
				if strings.ContainsAny(parts[0], ".:") {
					imgRegistry = parts[0]
				} else {
					// If no domain/port hint, assume default registry (like Docker Hub)
					// The actual parsing later will handle defaults more accurately
					imgRegistry = image.DefaultRegistry // Assuming image pkg has DefaultRegistry const
				}
			} else {
				// No '/' found, assume default registry
				imgRegistry = image.DefaultRegistry // Assuming image pkg has DefaultRegistry const
			}
			// NOTE: We no longer skip based on parsing error here.
			// The actual parsing error will be handled later in processImagePattern.
		case analysis.PatternTypeMap:
			if regVal, ok := pattern.Structure["registry"].(string); ok {
				imgRegistry = regVal
			} else {
				log.Debug("Skipping map pattern at path %s due to missing registry in structure: %+v", pattern.Path, pattern.Structure)
				continue
			}
		default:
			log.Debug("Skipping pattern at path %s due to unknown pattern type: %v", pattern.Path, pattern.Type)
			continue
		}

		if imgRegistry == "" {
			log.Debug("Skipping pattern at path %s due to empty registry", pattern.Path)
			continue
		}

		if g.isExcluded(imgRegistry) {
			log.Debug("Skipping image pattern from excluded registry '%s' at path %s", imgRegistry, pattern.Path)
			continue
		}
		if len(g.sourceRegistries) > 0 && !g.isSourceRegistry(imgRegistry) {
			log.Debug("Skipping image pattern from non-source registry '%s' at path %s", imgRegistry, pattern.Path)
			continue
		}
		eligibleImages = append(eligibleImages, pattern)
	}
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
	if err := override.SetValueAtPath(overrides, pathParts, overrideValue, false); err != nil {
		return fmt.Errorf("failed to set override path %v: %w", pathParts, err)
	}
	return nil
}

// processImagePattern attempts to parse the image string and potentially apply registry mappings.
func (g *Generator) processImagePattern(pattern analysis.ImagePattern) (*image.Reference, error) {
	// Use ParseImageReference
	ref, err := image.ParseImageReference(pattern.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid image format: %w", err)
	}

	// Apply registry mappings if available
	if g.mappings != nil {
		originalRegistry := ref.Registry
		if mappedTarget := g.mappings.GetTargetRegistry(originalRegistry); mappedTarget != "" {
			log.Debug("Generator: Applying mapping for registry '%s' -> target '%s'", originalRegistry, mappedTarget)
			// Create a copy of the reference before modifying
			newRef := *ref
			// Directly modify the registry field on the copy with the mapped target
			newRef.Registry = mappedTarget

			log.Debug("Generator: Mapped image ref: %s", newRef.String())
			return &newRef, nil // Return the modified copy
		}
		log.Debug("Generator: No mapping found for registry '%s'", originalRegistry)
	}

	// If no mapping applied, return the original parsed reference
	return ref, nil
}

// determineTargetPathAndRegistry determines the target registry and path for an image reference
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
			log.Debug("[GENERATE LOOP] Docker Hub library image detected: %s", imgRef.String())
		}

		if mappedValue, ok := g.configMappings[normalizedRegistry]; ok {
			log.Debug("[GENERATE LOOP] Found config mapping for %s: %s", normalizedRegistry, mappedValue)

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
				log.Debug("[GENERATE LOOP] Generated new path '%s' for original '%s' using config mapping", newPath, imgRef.Original)
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

	log.Debug("[GENERATE LOOP] Generated new path '%s' for original '%s'", newPath, imgRef.Original)
	return targetReg, newPath, nil
}

// processAndGenerateOverride processes a single image pattern and generates an override for it
// It returns a boolean indicating success and an error if one occurred
func (g *Generator) processAndGenerateOverride(
	pattern analysis.ImagePattern,
	imgRef *image.Reference,
	overrides map[string]interface{},
) (bool, error) {
	log.Debug("[GENERATE LOOP] Processing pattern with Path: %s, Type: %s", pattern.Path, pattern.Type)

	// Determine target registry and path
	targetReg, newPath, err := g.determineTargetPathAndRegistry(imgRef)
	if err != nil {
		log.Warn("Path generation failed for '%s': %v", imgRef.Original, err)
		log.Debug("[GENERATE LOOP ERROR] Error in determineTargetPathAndRegistry for %s: %v", pattern.Path, err)
		return false, err
	}

	// Create the override value (string or map)
	overrideValue := g.createOverride(pattern, imgRef, targetReg, newPath)

	// Set the override in the result map
	if err := g.setOverridePath(overrides, pattern, overrideValue); err != nil {
		log.Warn("Failed to set override for path '%s': %v", pattern.Path, err)
		log.Debug("[GENERATE LOOP ERROR] Error in setOverridePath for %s: %v", pattern.Path, err)
		return false, err
	}

	return true, nil
}

// Generate orchestrates the chart analysis and override generation process
func (g *Generator) Generate() (*override.File, error) {
	log.Debug("Generator: Starting override generation for chart: %s", g.chartPath)

	// Initialize the rules registry if it hasn't been set externally
	if g.rulesRegistry == nil {
		g.initRulesRegistry()
	}

	// Use the loader from the Generator instance to load the chart
	log.Debug("Generator: Using loader: %T", g.loader)
	loadedChart, err := g.loader.Load(g.chartPath)
	if err != nil {
		log.Debug("Generator: Error loading chart %s: %v", g.chartPath, err)
		return nil, &LoadingError{ChartPath: g.chartPath, Err: fmt.Errorf("failed to load chart: %w", err)}
	}

	log.Debug("Generator: Chart loaded: %s, Values type: %T", loadedChart.Name(), loadedChart.Values)

	// Ensure chart values are not nil before proceeding
	if loadedChart.Values == nil {
		log.Debug("Generator: Chart %s has nil values, generating empty override file.", loadedChart.Name())
		// Return an empty override file if there are no values to analyze
		return &override.File{ChartPath: g.chartPath, Values: make(map[string]interface{})}, nil // Correct instantiation
	}

	// Create an analyzer instance with the chart path and loader
	analyzer := analysis.NewAnalyzer(g.chartPath, g.loader) // Pass loader here

	// Analyze the chart (it uses the loader internally if needed)
	detectedImages, analysisErr := analyzer.Analyze() // Capture the error

	// Initialize result structure early
	result := &override.File{
		ChartPath:      g.chartPath,
		Values:         make(map[string]interface{}), // Initialize empty
		Unsupported:    []override.UnsupportedStructure{},
		SuccessRate:    0.0,
		ProcessedCount: 0,
		TotalCount:     0,
	}

	if analysisErr != nil {
		// Handle analysis errors: Log, potentially populate Unsupported, return result
		log.Warn("Analysis of chart failed for %s: %v", g.chartPath, analysisErr)
		// Attempt to add info to Unsupported based on the error
		// NOTE: This is a basic approach; might need refinement based on analyzer error types
		result.Unsupported = append(result.Unsupported, override.UnsupportedStructure{
			Path: []string{"analysis"}, // Generic path as we don't know the specific failing path from err
			Type: "AnalysisError",
		})
		// Return the partially populated result without a top-level error
		// This allows reporting issues without necessarily failing the whole process
		return result, nil
	}

	// Check for unsupported patterns found *during* analysis (if any)
	// Assuming detectedImages might contain unsupported info even if analysisErr is nil
	if detectedImages != nil { // Add nil check for detectedImages
		result.Unsupported = append(result.Unsupported, g.findUnsupportedPatterns(detectedImages.ImagePatterns)...)
		// If strict mode and unsupported patterns found AFTER successful analysis
		if g.strict && len(result.Unsupported) > 0 {
			log.Debug("Generator: Found %d unsupported patterns in strict mode.", len(result.Unsupported))
			firstUnsupported := result.Unsupported[0]
			return nil, &UnsupportedStructureError{
				Path: firstUnsupported.Path,
				Type: firstUnsupported.Type,
			}
		}
	}

	// Filter images based on source and exclude registries using the correct function
	eligibleImages := g.filterEligibleImages(detectedImages.ImagePatterns)                                                                     // Use the correct function signature
	log.Debug("Generator: Found %d total image patterns, %d eligible for processing.", len(detectedImages.ImagePatterns), len(eligibleImages)) // Correct length usage

	// Update total count in result
	result.TotalCount = len(eligibleImages)

	// Use the result.Values map for overrides
	overrides := result.Values
	processedCount := 0
	var processingErrors []error

	// Process eligible images
	for _, pattern := range eligibleImages {
		imgRef, err := g.processImagePattern(pattern)
		if err != nil {
			// Use pattern.Path directly in log
			log.Warn("Skipping image at path '%s': %v", pattern.Path, err)
			// ADDED: Populate Unsupported field for processing errors
			result.Unsupported = append(result.Unsupported, override.UnsupportedStructure{
				Path: strings.Split(pattern.Path, "."), // Split string path into []string
				Type: "InvalidImageFormat",             // More specific type
			})
			// Use pattern.Path directly in error message
			processingErrors = append(processingErrors, fmt.Errorf("path '%s': %w", pattern.Path, err))
			continue // Skip this image if processing fails
		}

		// Use processAndGenerateOverride which handles SetValueAtPath internally
		success, err := g.processAndGenerateOverride(pattern, imgRef, overrides)
		if err != nil {
			// Use pattern.Path directly in log
			log.Warn("Error generating override for path '%s': %v", pattern.Path, err)
			// Use pattern.Path directly in error message
			processingErrors = append(processingErrors, fmt.Errorf("override generation path '%s': %w", pattern.Path, err))
			// Decide if we should continue or stop based on the error
			continue // Continue processing other images for now
		}
		if success {
			processedCount++
		}
	}

	log.Debug("Generator: Processed %d eligible images for chart %s.", processedCount, g.chartPath)

	// Calculate success rate as float64
	var successRate float64
	if len(eligibleImages) > 0 {
		successRate = float64(processedCount*PercentageMultiplier) / float64(len(eligibleImages))
	}

	log.Debug("Generator: Success rate %.2f%% (%d/%d), Threshold %d%%", successRate, processedCount, len(eligibleImages), g.threshold)

	// Check for processing errors first
	if len(processingErrors) > 0 {
		// Aggregate errors for better reporting
		log.Error("Combined error details: %v", func() string {
			var errStrings []string
			for _, e := range processingErrors {
				errStrings = append(errStrings, e.Error())
			}
			return strings.Join(errStrings, "; ")
		}()) // Log combined error messages
		// Only return error if in strict mode
		if g.strict {
			return nil, fmt.Errorf("strict mode: generation failed with %d errors: %w", len(processingErrors), errors.Join(processingErrors...))
		}
		log.Warn("Generation completed with %d errors (non-strict mode). See logs for details.", len(processingErrors))
		// Proceed to return result in non-strict mode
	}

	// Apply rules if enabled
	if g.rulesEnabled {
		if g.rulesRegistry == nil {
			// This should ideally not happen if initRulesRegistry was called
			log.Warn("Rules enabled but registry is nil, skipping rule application.")
		} else {
			log.Debug("Generator: Applying chart rules...")
			modified, err := g.rulesRegistry.ApplyRules(loadedChart, overrides)
			if err != nil {
				// Decide how to handle rule errors (log? return error? depends on policy)
				log.Error("Error applying chart rules: %v", err)
				// Potentially return an error here if rule failures are critical
				// return nil, fmt.Errorf("failed to apply chart rules: %w", err)
			} else if modified {
				log.Debug("Generator: Rules modified the generated overrides.")
			}
		}
	}

	// Update result structure with processed count and success rate
	result.ProcessedCount = processedCount
	result.SuccessRate = successRate

	// Check threshold only if there were eligible images
	if len(eligibleImages) > 0 && successRate < float64(g.threshold) {
		// Use the correct ThresholdError from pkg/chart and populate correctly
		// For simplicity, create the error with the first unsupported pattern found.
		// A more robust implementation might list all or summarize.
		return nil, &ThresholdError{
			Threshold:   g.threshold,
			ActualRate:  int(successRate * PercentageMultiplier),
			Eligible:    len(eligibleImages),
			Processed:   processedCount,
			Err:         fmt.Errorf("success rate %.2f%% below threshold %d%% (%d/%d eligible images processed)", successRate, g.threshold, processedCount, len(eligibleImages)),
			WrappedErrs: processingErrors,
		}
	}

	log.Debug("Generator: Successfully generated overrides for chart %s", g.chartPath)
	return result, nil
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

// ValidateHelmTemplate runs `helm template` with the generated overrides to check for syntax errors.
// If validation fails with a Bitnami-specific security error (exit code 16),
// it will automatically retry with global.security.allowInsecureImages=true.
func ValidateHelmTemplate(chartPath string, overrides []byte) error {
	log.Debug("ValidateHelmTemplate")
	defer log.Debug("ValidateHelmTemplate")

	// Add nil check for the chart path
	if chartPath == "" {
		log.Error("Chart path is empty in ValidateHelmTemplate")
		return fmt.Errorf("chart path cannot be empty")
	}

	// First try - without any modification
	err := validateHelmTemplateInternalFunc(chartPath, overrides)
	if err == nil {
		return nil
	}

	// If we get an error, check if it's the specific Bitnami security error (exit code 16)
	// that can be fixed by adding global.security.allowInsecureImages=true
	handler := rules.NewBitnamiFallbackHandler()

	// Add nil check for handler
	if handler == nil {
		log.Error("Failed to create Bitnami fallback handler in ValidateHelmTemplate")
		return fmt.Errorf("handler creation failed, returning original error: %w", err)
	}

	if handler.ShouldRetryWithSecurityBypass(err) {
		log.Debug("Detected Bitnami security error, retrying with security bypass")

		// Parse the original overrides
		var overrideMap map[string]interface{}
		if unmarshalErr := yaml.Unmarshal(overrides, &overrideMap); unmarshalErr != nil {
			// If we can't parse the overrides, return the original error
			log.Debug("Failed to parse overrides for security bypass: %v", unmarshalErr)
			return err
		}

		// Add the security bypass parameter
		if applyErr := handler.ApplySecurityBypass(overrideMap); applyErr != nil {
			// If we can't apply the bypass, return the original error
			log.Debug("Failed to apply security bypass: %v", applyErr)
			return err
		}

		// Marshal the updated overrides back to YAML
		updatedOverrides, marshalErr := yaml.Marshal(overrideMap)
		if marshalErr != nil {
			// If we can't marshal the updated overrides, return the original error
			log.Debug("Failed to marshal updated overrides: %v", marshalErr)
			return err
		}

		// Try again with the updated overrides
		log.Debug("Retrying validation with security bypass parameter")
		retryErr := validateHelmTemplateInternalFunc(chartPath, updatedOverrides)
		if retryErr == nil {
			log.Info("Successfully validated chart with security bypass parameter")
			return nil
		}

		// If the retry fails, return the new error
		log.Debug("Retry with security bypass failed: %v", retryErr)
		return retryErr
	}

	// For any other error, return it as is
	return err
}

// validateHelmTemplateInternalFunc is a variable holding the function that performs
// the actual Helm template validation without any retry logic. This is defined as a
// variable to allow mocking in tests.
var validateHelmTemplateInternalFunc = validateHelmTemplateInternal

// validateHelmTemplateInternal is the internal implementation function
func validateHelmTemplateInternal(chartPath string, overrides []byte) error {
	// Add nil check for chartPath
	if chartPath == "" {
		return fmt.Errorf("chart path cannot be empty")
	}

	tempDir, err := os.MkdirTemp("", "irr-validate-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Warn("Warning: failed to clean up temp dir %s: %v", tempDir, err)
		}
	}()

	overrideFilePath := filepath.Join(tempDir, "temp-overrides.yaml")
	if err := os.WriteFile(overrideFilePath, overrides, FilePermissions); err != nil {
		return fmt.Errorf("failed to write temp overrides file: %w", err)
	}
	log.Debug("Temporary override file written to: %s", overrideFilePath)

	// Initialize Helm environment
	settings := cli.New()
	// Add nil check for settings
	if settings == nil {
		return fmt.Errorf("failed to initialize Helm CLI settings")
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "", func(string, ...interface{}) {}); err != nil {
		return fmt.Errorf("failed to initialize Helm action config: %w", err)
	}

	// Load the chart
	loadedChart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Add nil check for loadedChart
	if loadedChart == nil {
		return fmt.Errorf("chart loaded from %s is nil", chartPath)
	}

	// Load values from override file
	inputValues, err := chartutil.ReadValuesFile(overrideFilePath)
	if err != nil {
		return fmt.Errorf("failed to read values file: %w", err)
	}

	// Add nil check for values
	if inputValues == nil {
		return fmt.Errorf("values loaded from %s are nil", overrideFilePath)
	}

	// Create a release object
	rel := &release.Release{
		Chart:  loadedChart,
		Config: inputValues.AsMap(), // Use AsMap() for rendering
		Name:   "release-name",
	}

	// Get Kubernetes config for template engine
	restConfig, err := settings.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return fmt.Errorf("failed to get REST config: %w", err)
	}

	// Add nil check for restConfig
	if restConfig == nil {
		return fmt.Errorf("REST config is nil")
	}

	// Render the templates
	rendered, err := engine.New(restConfig).Render(rel.Chart, rel.Config)
	if err != nil {
		log.Debug("Helm template rendering failed. Error: %v", err)
		return fmt.Errorf("helm template rendering failed: %w", err)
	}

	// Add nil check for rendered
	if rendered == nil {
		return fmt.Errorf("rendered templates are nil")
	}

	// Combine all rendered templates
	var outputBuffer bytes.Buffer
	for name, content := range rendered {
		if content != "" && !strings.HasSuffix(name, "NOTES.txt") {
			outputBuffer.WriteString("---\n# Source: " + name + "\n")
			outputBuffer.WriteString(content)
			outputBuffer.WriteString("\n")
		}
	}

	output := outputBuffer.String()
	log.Debug("Helm template rendering successful. Output length: %d", len(output))

	if output == "" {
		return errors.New("helm template output is empty")
	}

	// Decode the input overrides for comparison
	inputOverridesMap := make(map[string]interface{})
	if err := yaml.Unmarshal(overrides, &inputOverridesMap); err != nil {
		log.Debug("Failed to unmarshal input overrides for validation check: %v", err)
		// Don't fail validation just because we can't parse input, but log it.
	} else if len(inputOverridesMap) > 0 {
		// Decode the rendered output YAML
		dec := yaml.NewDecoder(strings.NewReader(output))
		validatedPaths := make(map[string]bool) // Track which input overrides are found and match
		validationErrors := []string{}

		for {
			var renderedDocMap map[string]interface{}
			if err := dec.Decode(&renderedDocMap); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				// If YAML parsing fails, it's a Helm template error, return that.
				return fmt.Errorf("failed to decode rendered YAML output: %w", err)
			}

			// Compare image fields found in inputOverridesMap against renderedDocMap
			for k, v := range inputOverridesMap {
				pathKey := k // The dot-notation path string
				if isLikelyImageMap(v) {
					imageMap, ok := v.(map[string]interface{})
					if !ok {
						return fmt.Errorf("input override at path '%s' is not a valid image map", pathKey)
					}
					// Check if the rendered output has the corresponding image path and values
					renderedVal, found := findValueByPath(renderedDocMap, strings.Split(pathKey, "."))
					if found {
						if reflect.DeepEqual(imageMap, renderedVal) {
							validatedPaths[pathKey] = true // Mark as validated if found and matches
						} else {
							// Found the path, but the value mismatches - this is a definite error
							errMsg := fmt.Sprintf("mismatch at path '%s': expected %v, got %v", pathKey, imageMap, renderedVal)
							validationErrors = append(validationErrors, errMsg)
						}
					}
				}
			}
		}

		// After checking all documents, verify all input overrides were validated somewhere
		for pathKey, inputVal := range inputOverridesMap {
			if isLikelyImageMap(inputVal) && !validatedPaths[pathKey] {
				// If an expected image override was not found and validated in any document where its path exists,
				// check if we recorded a mismatch error for it earlier.
				foundMismatchError := false
				for _, errMsg := range validationErrors {
					if strings.Contains(errMsg, fmt.Sprintf("mismatch at path '%s'", pathKey)) {
						foundMismatchError = true
						break
					}
				}
				// If no mismatch was recorded, it means the path wasn't found at all where expected.
				if !foundMismatchError {
					validationErrors = append(validationErrors, fmt.Sprintf("override for path '%s' not found or applied in rendered output", pathKey))
				}
			}
		}

		if len(validationErrors) > 0 {
			log.Debug("Validation failed: Rendered output does not match overrides. Errors: %s", strings.Join(validationErrors, "; "))
			return fmt.Errorf("validation failed: rendered output does not match overrides: %s", strings.Join(validationErrors, "; "))
		}
		log.Debug("Override values successfully verified in rendered output.")
	}

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

// isLikelyImageMap checks if a map looks like an image structure (e.g., {repository: "...", tag: "..."})
func isLikelyImageMap(v interface{}) bool {
	if mapVal, ok := v.(map[string]interface{}); ok {
		_, repoOk := mapVal["repository"].(string)
		_, regOk := mapVal["registry"].(string)
		// Check for repository and registry as strong indicators
		return repoOk && regOk
	}
	return false
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
