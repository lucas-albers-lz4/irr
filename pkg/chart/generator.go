package chart

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	// Import the necessary Helm types
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmchartloader "helm.sh/helm/v3/pkg/chart/loader" // Import Helm loader

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
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
type GeneratorLoader struct{}

// Load implements analysis.ChartLoader interface, returning helmchart.Chart
func (l *GeneratorLoader) Load(chartPath string) (*helmchart.Chart, error) { // Return *helmchart.Chart
	debug.Printf("GeneratorLoader: Loading chart from %s", chartPath)

	// Use helm's loader directly
	helmLoadedChart, err := helmchartloader.Load(chartPath)
	if err != nil {
		// Wrap the error from the helm loader
		return nil, fmt.Errorf("helm loader failed for path '%s': %w", chartPath, err)
	}

	// We need to extract values manually if helm loader doesn't merge them automatically?
	// Let's assume `helmchartloader.Load` provides merged values in helmLoadedChart.Values
	if helmLoadedChart.Values == nil {
		helmLoadedChart.Values = make(map[string]interface{}) // Ensure Values is not nil
		debug.Printf("Helm chart loaded with nil Values, initialized empty map for %s", chartPath)
	}

	debug.Printf("GeneratorLoader successfully loaded chart: %s", helmLoadedChart.Name())
	return helmLoadedChart, nil
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
	loader analysis.ChartLoader,
	includePatterns, excludePatterns, knownPaths []string,
) *Generator {
	if loader == nil {
		loader = &GeneratorLoader{}
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
		loader:            loader,
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
		var registry string

		switch pattern.Type {
		case analysis.PatternTypeString:
			imgRef, err := image.ParseImageReference(pattern.Value)
			if err != nil {
				debug.Printf("Skipping pattern at path %s due to parse error on value '%s': %v", pattern.Path, pattern.Value, err)
				continue
			}
			registry = imgRef.Registry
		case analysis.PatternTypeMap:
			if regVal, ok := pattern.Structure["registry"].(string); ok {
				registry = regVal
			} else {
				debug.Printf("Skipping map pattern at path %s due to missing registry in structure: %+v", pattern.Path, pattern.Structure)
				continue
			}
		default:
			debug.Printf("Skipping pattern at path %s due to unknown pattern type: %v", pattern.Path, pattern.Type)
			continue
		}

		if registry == "" {
			debug.Printf("Skipping pattern at path %s due to empty registry", pattern.Path)
			continue
		}

		if g.isExcluded(registry) {
			debug.Printf("Skipping image pattern from excluded registry '%s' at path %s", registry, pattern.Path)
			continue
		}
		if len(g.sourceRegistries) > 0 && !g.isSourceRegistry(registry) {
			debug.Printf("Skipping image pattern from non-source registry '%s' at path %s", registry, pattern.Path)
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
		override := fmt.Sprintf("%s/%s", targetReg, newPath)
		if imgRef.Tag != "" {
			override = fmt.Sprintf("%s:%s", override, imgRef.Tag)
		}
		return override
	}

	// For map type, update the map structure
	override := map[string]interface{}{
		"registry":   targetReg,
		"repository": newPath,
	}
	if imgRef.Tag != "" {
		override["tag"] = imgRef.Tag
	}
	return override
}

// setOverridePath sets the override value at the correct path in the overrides map
func (g *Generator) setOverridePath(overrides map[string]interface{}, pattern analysis.ImagePattern, override interface{}) error {
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
	current[pathParts[len(pathParts)-1]] = override
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
	var registry, repository, tag string
	var ok bool

	registryVal, exists := pattern.Structure["registry"]
	if !exists {
		return nil, fmt.Errorf("missing 'registry' key in image map at path %s", pathForError)
	}
	registry, ok = registryVal.(string)
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
			return nil, fmt.Errorf("invalid type for 'tag' key (expected string) in image map at path %s", pathForError)
		}
	}

	// Construct the image string to leverage ParseImageReference for validation/normalization
	imgStr := registry + "/" + repository
	if tag != "" {
		imgStr += ":" + tag
	}

	// Parse the constructed string to get a validated Reference object
	ref, err := image.ParseImageReference(imgStr)
	if err != nil {
		// Wrap the error with context about the map structure
		return nil, fmt.Errorf("failed to parse constructed image string '%s' from map at path %s: %w", imgStr, pathForError, err)
	}
	// Store the original path info, split into segments
	ref.Path = strings.Split(pattern.Path, ".") // Split the string path here
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

	return &override.File{
		ChartPath:   g.chartPath,
		Overrides:   overrides,
		Unsupported: unsupportedPatterns,
	}, nil
}

// --- Helper methods (isSourceRegistry, isExcluded) ---
func (g *Generator) isSourceRegistry(registry string) bool {
	return isRegistryInList(registry, g.sourceRegistries)
}

func (g *Generator) isExcluded(registry string) bool {
	return isRegistryInList(registry, g.excludeRegistries)
}

func isRegistryInList(registry string, list []string) bool {
	if len(list) == 0 {
		return false
	}
	normalizedRegistry := image.NormalizeRegistry(registry)
	for _, r := range list {
		if normalizedRegistry == image.NormalizeRegistry(r) {
			return true
		}
	}
	return false
}

// ValidateHelmTemplate runs `helm template` with the generated overrides to check validity.
func ValidateHelmTemplate(runner CommandRunner, chartPath string, overrides []byte) error {
	debug.FunctionEnter("ValidateHelmTemplate")
	defer debug.FunctionExit("ValidateHelmTemplate")

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

	args := []string{"template", "release-name", chartPath, "-f", overrideFilePath}
	debug.Printf("Running helm command: helm %v", args)

	output, err := runner.Run("helm", args...)
	if err != nil {
		debug.Printf("Helm template command failed. Error: %v\nOutput:\n%s", err, string(output))
		return fmt.Errorf("helm template command failed: %w. Output: %s", err, string(output))
	}
	debug.Printf("Helm template command successful. Output length: %d", len(output))

	if len(output) == 0 {
		return errors.New("helm template output is empty")
	}
	dec := yaml.NewDecoder(strings.NewReader(string(output)))
	var node interface{}
	for {
		if err := dec.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			debug.Printf("YAML validation failed during decode: %v", err)
			return fmt.Errorf("invalid YAML structure in helm template output: %w", err)
		}
		node = nil
	}

	debug.Println("Helm template output validated successfully.")
	return nil
}

// CommandRunner interface defines an interface for running external commands, useful for testing.
type CommandRunner interface {
	Run(name string, arg ...string) ([]byte, error)
}

// RealCommandRunner implements CommandRunner using os/exec.
type RealCommandRunner struct{}

// Run executes the command using os/exec.
func (r *RealCommandRunner) Run(name string, arg ...string) ([]byte, error) {
	cmd := exec.Command(name, arg...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("command execution failed for '%s %s': %w", name, strings.Join(arg, " "), err)
	}
	return output, nil
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
