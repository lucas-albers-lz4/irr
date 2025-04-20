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
	"github.com/lalbers/irr/pkg/debug"
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
	debug.Printf("GeneratorLoader: Loading chart from %s", chartPath)

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
		debug.Printf("Helm chart loaded with nil Values, initialized empty map for %s", chartPath)
	}

	debug.Printf("GeneratorLoader successfully loaded chart: %s", loadedChart.Name())
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
	loader            analysis.ChartLoader // Use analysis.ChartLoader interface
	rulesEnabled      bool                 // Whether to apply rules
	rulesRegistry     interface{}          // Rules registry (will be *rules.Registry)
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
		rulesEnabled:      true, // Enable rules by default
		rulesRegistry:     nil,  // Will be initialized on first use
	}
}

// findUnsupportedPatterns identifies template expressions and other unsupported structures
func (g *Generator) findUnsupportedPatterns(detectedImages []analysis.ImagePattern) []override.UnsupportedStructure {
	unsupportedPatterns := []override.UnsupportedStructure{}
	for _, pattern := range detectedImages {
		// Check if the *Value* (the image string/map representation) contains template markers
		// For maps, we might need a more robust check if templates can exist within map values.
		valueToCheck := ""
		foundTemplate := false

		switch pattern.Type {
		case analysis.PatternTypeString:
			valueToCheck = pattern.Value
			if strings.Contains(valueToCheck, "{{") || strings.Contains(valueToCheck, "}}") {
				foundTemplate = true
			}
		case analysis.PatternTypeMap:
			// Check known string fields within the map structure for templates
			if reg, ok := pattern.Structure["registry"].(string); ok && (strings.Contains(reg, "{{") || strings.Contains(reg, "}}")) {
				foundTemplate = true
			} else if repo, ok := pattern.Structure["repository"].(string); ok && (strings.Contains(repo, "{{") || strings.Contains(repo, "}}")) {
				foundTemplate = true
			} else if tag, ok := pattern.Structure["tag"].(string); ok && (strings.Contains(tag, "{{") || strings.Contains(tag, "}}")) {
				foundTemplate = true
			}
		}

		if foundTemplate {
			unsupportedPatterns = append(unsupportedPatterns, override.UnsupportedStructure{
				Path: strings.Split(pattern.Path, "."), // Path should be slice here
				Type: "template",                       // Keep type as template
			})
		}
	}
	return unsupportedPatterns
}

// filterEligibleImages filters detected images based on registry rules
func (g *Generator) filterEligibleImages(detectedImages []analysis.ImagePattern) []analysis.ImagePattern {
	eligibleImages := []analysis.ImagePattern{}
	for _, pattern := range detectedImages {
		var imgRegistry string

		switch pattern.Type {
		case analysis.PatternTypeString:
			imgRef, err := image.ParseImageReference(pattern.Value)
			if err != nil {
				debug.Printf("Skipping pattern at path %s due to parse error on value '%s': %v", pattern.Path, pattern.Value, err)
				continue
			}
			imgRegistry = imgRef.Registry
		case analysis.PatternTypeMap:
			if regVal, ok := pattern.Structure["registry"].(string); ok {
				imgRegistry = regVal
			} else {
				debug.Printf("Skipping map pattern at path %s due to missing registry in structure: %+v", pattern.Path, pattern.Structure)
				continue
			}
		default:
			debug.Printf("Skipping pattern at path %s due to unknown pattern type: %v", pattern.Path, pattern.Type)
			continue
		}

		if imgRegistry == "" {
			debug.Printf("Skipping pattern at path %s due to empty registry", pattern.Path)
			continue
		}

		if g.isExcluded(imgRegistry) {
			debug.Printf("Skipping image pattern from excluded registry '%s' at path %s", imgRegistry, pattern.Path)
			continue
		}
		if len(g.sourceRegistries) > 0 && !g.isSourceRegistry(imgRegistry) {
			debug.Printf("Skipping image pattern from non-source registry '%s' at path %s", imgRegistry, pattern.Path)
			continue
		}
		eligibleImages = append(eligibleImages, pattern)
	}
	return eligibleImages
}

// createOverride generates the override value for a pattern
func (g *Generator) createOverride(pattern analysis.ImagePattern, imgRef *image.Reference, targetReg, newPath string) interface{} {
	if pattern.Type == analysis.PatternTypeString {
		// For string type, generate a full image reference string
		overrideValue := fmt.Sprintf("%s/%s", targetReg, newPath)
		if imgRef.Tag != "" {
			overrideValue = fmt.Sprintf("%s:%s", overrideValue, imgRef.Tag)
		}
		return overrideValue
	}

	// For map type, update the map structure
	overrideMap := map[string]interface{}{
		"registry":   targetReg,
		"repository": newPath,
	}
	if imgRef.Tag != "" {
		overrideMap["tag"] = imgRef.Tag
	}
	return overrideMap
}

// setOverridePath sets the override value at the correct path in the overrides map
func (g *Generator) setOverridePath(overrides map[string]interface{}, pattern analysis.ImagePattern, overrideValue interface{}) error {
	pathParts := strings.Split(pattern.Path, ".")
	current := overrides
	for i := 0; i < len(pathParts)-1; i++ {
		part := pathParts[i]
		if _, exists := current[part]; !exists {
			current[part] = make(map[string]interface{})
		}
		// Check type assertion
		var ok bool
		current, ok = current[part].(map[string]interface{})
		if !ok {
			// Handle error: unexpected type in overrides map
			// This indicates a logic error in how overrides are built.
			// Returning an error is appropriate.
			return fmt.Errorf("internal error: unexpected type at path %s in overrides map", strings.Join(pathParts[:i+1], "."))
		}
	}
	current[pathParts[len(pathParts)-1]] = overrideValue
	return nil // Indicate success for this helper modification
}

// processImagePattern processes a single image pattern and returns its reference
func (g *Generator) processImagePattern(pattern analysis.ImagePattern) (*image.Reference, error) {
	pathForError := pattern.Path // Use the original string path for error messages

	if pattern.Type == analysis.PatternTypeString {
		ref, err := image.ParseImageReference(pattern.Value)
		if err != nil {
			// Wrap the parsing error for context using the string path
			return nil, fmt.Errorf("failed to parse image string '%s' at path %s: %w", pattern.Value, pathForError, err)
		}
		// Store the original path info, split into segments
		ref.Path = strings.Split(pattern.Path, ".") // Split the string path here
		return ref, nil
	}

	// Handle PatternTypeMap
	var imgRegistry, repository, tag string
	var ok bool

	registryVal, exists := pattern.Structure["registry"]
	if !exists {
		return nil, fmt.Errorf("missing 'registry' key in image map at path %s", pathForError)
	}
	imgRegistry, ok = registryVal.(string)
	if !ok {
		return nil, fmt.Errorf("invalid type for 'registry' key (expected string) in image map at path %s", pathForError)
	}

	repositoryVal, exists := pattern.Structure["repository"]
	if !exists {
		return nil, fmt.Errorf("missing 'repository' key in image map at path %s", pathForError)
	}
	repository, ok = repositoryVal.(string)
	if !ok {
		return nil, fmt.Errorf("invalid type for 'repository' key (expected string) in image map at path %s", pathForError)
	}

	tagVal, exists := pattern.Structure["tag"]
	if !exists {
		// Allow missing tag, ParseImageReference handles normalization later if needed
		tag = ""
	} else {
		tag, ok = tagVal.(string)
		if !ok {
			tag = ""
		}
	}

	return &image.Reference{
		Registry:   imgRegistry,
		Repository: repository,
		Tag:        tag,
		Path:       strings.Split(pattern.Path, "."),
	}, nil
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
			debug.Printf("[GENERATE LOOP] Docker Hub library image detected: %s", imgRef.String())
		}

		if mappedValue, ok := g.configMappings[normalizedRegistry]; ok {
			debug.Printf("[GENERATE LOOP] Found config mapping for %s: %s", normalizedRegistry, mappedValue)

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
				debug.Printf("[GENERATE LOOP] Generated new path '%s' for original '%s' using config mapping", newPath, imgRef.Original)
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

	debug.Printf("[GENERATE LOOP] Generated new path '%s' for original '%s'", newPath, imgRef.Original)
	return targetReg, newPath, nil
}

// processAndGenerateOverride processes a single image pattern and generates an override for it
// It returns a boolean indicating success and an error if one occurred
func (g *Generator) processAndGenerateOverride(
	pattern analysis.ImagePattern,
	imgRef *image.Reference,
	overrides map[string]interface{},
) (bool, error) {
	debug.Printf("[GENERATE LOOP] Processing pattern with Path: %s, Type: %s", pattern.Path, pattern.Type)

	// Determine target registry and path
	targetReg, newPath, err := g.determineTargetPathAndRegistry(imgRef)
	if err != nil {
		log.Warnf("Path generation failed for '%s': %v", imgRef.Original, err)
		debug.Printf("[GENERATE LOOP ERROR] Error in determineTargetPathAndRegistry for %s: %v", pattern.Path, err)
		return false, err
	}

	// Create the override value (string or map)
	overrideValue := g.createOverride(pattern, imgRef, targetReg, newPath)

	// Set the override in the result map
	if err := g.setOverridePath(overrides, pattern, overrideValue); err != nil {
		log.Warnf("Failed to set override for path '%s': %v", pattern.Path, err)
		debug.Printf("[GENERATE LOOP ERROR] Error in setOverridePath for %s: %v", pattern.Path, err)
		return false, err
	}

	return true, nil
}

// Generate performs the actual chart analysis and override generation.
// It loads the chart, analyzes it for image references, and generates the
// appropriate override values to redirect those images to the target registry.
//
// The process includes:
// 1. Loading the chart using the configured loader
// 2. Analyzing the chart for image references
// 3. Filtering images based on source/exclude registries
// 4. Generating overrides for each eligible image
// 5. Applying threshold validation if configured
// 6. Returning the final override file structure
//
// Returns:
//   - *override.File: The generated override file structure
//   - error: An error if generation fails (uses custom error types for specific failures)
func (g *Generator) Generate() (*override.File, error) {
	debug.FunctionEnter("Generator.Generate")
	defer debug.FunctionExit("Generator.Generate")

	// Configure and run the analyzer
	analyzer := analysis.NewAnalyzer(g.chartPath, g.loader)
	debug.Printf("Analyzing chart: %s", g.chartPath)
	analysisResults, err := analyzer.Analyze()
	if err != nil {
		return nil, fmt.Errorf("error analyzing chart %s: %w", g.chartPath, err)
	}

	detectedImages := analysisResults.ImagePatterns
	debug.Printf("Analysis complete. Found %d image patterns.", len(detectedImages))

	// Find unsupported patterns and handle strict mode
	unsupportedPatterns := g.findUnsupportedPatterns(detectedImages)
	if g.strict && len(unsupportedPatterns) > 0 {
		details := []string{}
		log.Warnf("Strict mode enabled: Found %d unsupported image structures:", len(unsupportedPatterns))
		for i, item := range unsupportedPatterns {
			errMsg := fmt.Sprintf("  [%d] Path: %s, Type: %s", i+1, strings.Join(item.Path, "."), item.Type)
			log.Warnf(errMsg)
			_ = append(details, errMsg) // Explicitly indicate we're ignoring the result
		}
		// Return the specific error type directly for correct exit code handling
		return nil, ErrUnsupportedStructure
	}

	// Filter and process detected images
	eligibleImages := g.filterEligibleImages(detectedImages)
	debug.Printf("[GENERATE] Found %d eligible images after filtering.", len(eligibleImages))

	// Generate overrides
	overrides := make(map[string]interface{})
	var processErrors []error
	eligibleCount := len(eligibleImages)
	processedCount := 0

	for _, pattern := range eligibleImages {
		imgRef, err := g.processImagePattern(pattern)
		if err != nil {
			// Log the error from processImagePattern
			log.Warnf("Skipping pattern due to processing error: %v", err)
			processErrors = append(processErrors, err) // Store the error
			debug.Printf("[GENERATE LOOP ERROR] Error in processImagePattern for %s: %v", pattern.Path, err)
			continue
		}

		success, err := g.processAndGenerateOverride(pattern, imgRef, overrides)
		if err != nil {
			processErrors = append(processErrors, err)
			continue
		}

		if success {
			processedCount++
		}
	}

	// Calculate success rate if eligible images were found
	if eligibleCount > 0 {
		successRate := (processedCount * PercentMultiplier) / eligibleCount
		debug.Printf("Success Rate: %d%% of eligible images processed successfully (%d of %d)",
			successRate, processedCount, eligibleCount)

		// Check threshold if set
		if g.threshold > 0 && successRate < g.threshold {
			return nil, &ThresholdError{
				Threshold:   g.threshold,
				ActualRate:  successRate,
				Eligible:    eligibleCount,
				Processed:   processedCount,
				WrappedErrs: processErrors,
			}
		}
	}

	// Load the chart to access metadata for rule application
	loadedChart, err := g.loader.Load(g.chartPath)
	if err != nil {
		log.Errorf("Failed to load chart %s for rule application: %v", g.chartPath, err)
		// Consider returning the error, as rules cannot be applied
		return nil, fmt.Errorf("failed to load chart %s for rule application: %w", g.chartPath, err)
	} else if g.rulesEnabled {
		// Apply rules if enabled and chart loaded successfully
		rulesApplied := false

		// Import rules package only when needed
		if g.rulesRegistry == nil {
			// Use the default registry
			debug.Printf("Initializing rules registry")
			g.initRulesRegistry()
		}

		// Apply rules from the registry if it's initialized
		if g.rulesRegistry != nil {
			regInstance, ok := g.rulesRegistry.(*rules.Registry)
			if !ok {
				log.Warnf("Rules registry type assertion failed")
			} else {
				log.Debugf("Chart [%s]: Applying rules. Override map BEFORE ApplyRules: %v", loadedChart.Name(), overrides)

				rulesApplied, err = regInstance.ApplyRules(loadedChart, overrides)

				log.Debugf("Chart [%s]: Rules applied = %v. Override map AFTER ApplyRules: %v", loadedChart.Name(), rulesApplied, overrides)

				if err != nil {
					log.Errorf("Error applying rules to chart %s: %v", loadedChart.Name(), err)
					// Return the error, as rule application failed
					return nil, fmt.Errorf("failed applying rules to chart %s: %w", loadedChart.Name(), err)
				}
				if rulesApplied {
					debug.Printf("Successfully applied rules to chart: %s", g.chartPath)
				}
			}
		}
	}

	return &override.File{
		ChartPath:      g.chartPath,
		ChartName:      filepath.Base(g.chartPath),
		Values:         overrides,
		Unsupported:    unsupportedPatterns,
		ProcessedCount: processedCount,
		TotalCount:     len(detectedImages),
		SuccessRate:    float64(processedCount) / float64(len(detectedImages)) * PercentageMultiplier,
	}, nil
}

// initRulesRegistry initializes the rules registry
func (g *Generator) initRulesRegistry() {
	// Lazy-load the rules package
	g.rulesRegistry = rules.DefaultRegistry
}

// SetRulesEnabled enables or disables rule application
func (g *Generator) SetRulesEnabled(enabled bool) {
	g.rulesEnabled = enabled
	debug.Printf("Rules system enabled: %v", enabled)
}

// --- Helper methods (isSourceRegistry, isExcluded) ---
func (g *Generator) isSourceRegistry(regStr string) bool {
	for _, src := range g.sourceRegistries {
		if regStr == src {
			return true
		}
	}
	return false
}

func (g *Generator) isExcluded(regStr string) bool {
	for _, ex := range g.excludeRegistries {
		if regStr == ex {
			return true
		}
	}
	return false
}

// ValidateHelmTemplate validates a Helm chart template using the Helm SDK.
// If validation fails with a Bitnami-specific security error (exit code 16),
// it will automatically retry with global.security.allowInsecureImages=true.
func ValidateHelmTemplate(chartPath string, overrides []byte) error {
	debug.FunctionEnter("ValidateHelmTemplate")
	defer debug.FunctionExit("ValidateHelmTemplate")

	// Add nil check for the chart path
	if chartPath == "" {
		log.Errorf("Chart path is empty in ValidateHelmTemplate")
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
		log.Errorf("Failed to create Bitnami fallback handler in ValidateHelmTemplate")
		return fmt.Errorf("handler creation failed, returning original error: %w", err)
	}

	if handler.ShouldRetryWithSecurityBypass(err) {
		debug.Printf("Detected Bitnami security error, retrying with security bypass")

		// Parse the original overrides
		var overrideMap map[string]interface{}
		if unmarshalErr := yaml.Unmarshal(overrides, &overrideMap); unmarshalErr != nil {
			// If we can't parse the overrides, return the original error
			debug.Printf("Failed to parse overrides for security bypass: %v", unmarshalErr)
			return err
		}

		// Add the security bypass parameter
		if applyErr := handler.ApplySecurityBypass(overrideMap); applyErr != nil {
			// If we can't apply the bypass, return the original error
			debug.Printf("Failed to apply security bypass: %v", applyErr)
			return err
		}

		// Marshal the updated overrides back to YAML
		updatedOverrides, marshalErr := yaml.Marshal(overrideMap)
		if marshalErr != nil {
			// If we can't marshal the updated overrides, return the original error
			debug.Printf("Failed to marshal updated overrides: %v", marshalErr)
			return err
		}

		// Try again with the updated overrides
		debug.Printf("Retrying validation with security bypass parameter")
		retryErr := validateHelmTemplateInternalFunc(chartPath, updatedOverrides)
		if retryErr == nil {
			log.Infof("Successfully validated chart with security bypass parameter")
			return nil
		}

		// If the retry fails, return the new error
		debug.Printf("Retry with security bypass failed: %v", retryErr)
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
			log.Warnf("Warning: failed to clean up temp dir %s: %v", tempDir, err)
		}
	}()

	overrideFilePath := filepath.Join(tempDir, "temp-overrides.yaml")
	if err := os.WriteFile(overrideFilePath, overrides, FilePermissions); err != nil {
		return fmt.Errorf("failed to write temp overrides file: %w", err)
	}
	debug.Printf("Temporary override file written to: %s", overrideFilePath)

	// Initialize Helm environment
	settings := cli.New()
	// Add nil check for settings
	if settings == nil {
		return fmt.Errorf("failed to initialize Helm CLI settings")
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "", debug.Printf); err != nil {
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
		debug.Printf("Helm template rendering failed. Error: %v", err)
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
	debug.Printf("Helm template rendering successful. Output length: %d", len(output))

	if output == "" {
		return errors.New("helm template output is empty")
	}

	// Decode the input overrides for comparison
	inputOverridesMap := make(map[string]interface{})
	if err := yaml.Unmarshal(overrides, &inputOverridesMap); err != nil {
		debug.Printf("Failed to unmarshal input overrides for validation check: %v", err)
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
			debug.Printf("Validation failed: Rendered output does not match overrides. Errors: %s", strings.Join(validationErrors, "; "))
			return fmt.Errorf("validation failed: rendered output does not match overrides: %s", strings.Join(validationErrors, "; "))
		}
		debug.Printf("Override values successfully verified in rendered output.")
	}

	return nil
}

// findValueByPath searches for a value in a nested map structure using a dot-separated path.
// TODO: Enhance this to handle array indices if needed.
func findValueByPath(data map[string]interface{}, path []string) (interface{}, bool) {
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
	return nil, false // Should not happen if path is valid
}

// isLikelyImageMap checks if a value is a map likely representing an image override.
func isLikelyImageMap(v interface{}) bool {
	if mapVal, ok := v.(map[string]interface{}); ok {
		_, repoOk := mapVal["repository"].(string)
		_, regOk := mapVal["registry"].(string)
		// Check for repository and registry as strong indicators
		return repoOk && regOk
	}
	return false
}

// OverridesToYAML converts a map of overrides to YAML format
func OverridesToYAML(overrides map[string]interface{}) ([]byte, error) {
	debug.Printf("Marshaling overrides to YAML")
	yamlBytes, err := yaml.Marshal(overrides)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
	}
	return yamlBytes, nil
}
