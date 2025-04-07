package chart

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
)

const (
	// defaultFilePerm specifies the default file permission (read/write for owner)
	defaultFilePerm fs.FileMode = 0o600
	// percentageMultiplier is used for calculating percentages
	percentageMultiplier = 100
	// maxSplitTwo is the limit for splitting into at most two parts
	maxSplitTwo = 2
)

// Package chart provides functionality for loading charts, detecting images, and generating override files.
// This package is responsible for processing Helm charts and generating override structures
// that can be used to modify image references in the chart's values.

// Key concepts:
// - ImageReference: Represents a container image reference (registry/repository:tag)
// - OverrideStructure: A nested map that mirrors the chart's values structure
// - PathStrategy: Defines how image paths are transformed in the override structure

// Type hints for map structures:
// map[string]interface{} can contain:
// - Nested maps (for structured data)
// - Arrays (for list values)
// - Strings (for image references and other values)
// - Numbers (for ports, replicas, etc.)
// - Booleans (for feature flags)

// @llm-helper This package uses reflection and type assertions extensively
// @llm-helper The override structure matches Helm's value override format
// @llm-helper Image references can be in multiple formats (string, map with repository/tag)

// DetectedImage represents an image reference found within the chart's values.
type DetectedImage struct {
	Reference *image.Reference
	Path      []string
	Source    string // e.g., "values.yaml", "Chart.yaml"
}

// Generator generates image overrides for a Helm chart
type Generator struct {
	chartPath         string
	targetRegistry    string
	sourceRegistries  []string
	excludeRegistries []string
	pathStrategy      strategy.PathStrategy
	mappings          *registry.Mappings
	strict            bool
	threshold         int
	loader            Loader
}

// NewGenerator creates a new Generator
func NewGenerator(chartPath, targetRegistry string, sourceRegistries, excludeRegistries []string, pathStrategy strategy.PathStrategy, mappings *registry.Mappings, strict bool, threshold int, loader Loader) *Generator {
	if loader == nil {
		loader = NewLoader()
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
		loader:            loader,
	}
}

// locationTypeToString converts a LocationType to its string representation
func locationTypeToString(lt image.LocationType) string {
	switch lt {
	case image.TypeUnknown:
		return "unknown"
	case image.TypeMapRegistryRepositoryTag:
		return "map-registry-repository-tag"
	case image.TypeRepositoryTag:
		return "repository-tag"
	case image.TypeString:
		return "string"
	default:
		return fmt.Sprintf("unknown-%d", lt)
	}
}

// Generate generates image overrides for the chart
func (g *Generator) Generate() (*override.File, error) {
	// <<< ADD fmt.Printf LOGGING HERE AT THE VERY BEGINNING >>>
	debug.FunctionEnter("Generator.Generate")
	defer debug.FunctionExit("Generator.Generate")

	chartData, err := g.loader.Load(g.chartPath)
	if err != nil {
		// Return specific ParsingError (Corrected type)
		return nil, &ParsingError{FilePath: g.chartPath, Err: err}
	}

	baseValuesCopy := override.DeepCopy(chartData.Values)
	baseValues, ok := baseValuesCopy.(map[string]interface{})
	if !ok {
		// This indicates a fundamental issue with the chart's values structure
		return nil, &ParsingError{FilePath: g.chartPath, Err: errors.New("chart values are not a map[string]interface{}")} // Corrected type
	}
	debug.DumpValue("Base values for type checking", baseValues)

	// --- Extract Global Registry ---
	globalRegistry := ""
	if globalValues, ok := baseValues["global"].(map[string]interface{}); ok {
		if reg, ok := globalValues["imageRegistry"].(string); ok {
			globalRegistry = reg
			debug.Printf("Extracted global.imageRegistry: %s", globalRegistry)
		}
	}

	// --- Use ImageDetector with context ---
	detectionContext := &image.DetectionContext{
		SourceRegistries:  g.sourceRegistries,
		ExcludeRegistries: g.excludeRegistries,
		GlobalRegistry:    globalRegistry,
		Strict:            g.strict,
		// TemplateMode: false, // Assuming not needed for now
	}
	detector := image.NewDetector(*detectionContext)

	// --- ADD DEBUG LOG BEFORE DetectImages ---
	if chartData.Values == nil {
		fmt.Println("[DEBUG irr GEN] >>> WARNING: chartData.Values is nil before calling DetectImages!")
	} else {
		fmt.Printf("[DEBUG irr GEN] >>> Chart values map size: %d\n", len(chartData.Values))
	}
	// --- END DEBUG LOG ---

	images, unsupportedMatches, err := detector.DetectImages(chartData.Values, []string{}) // Pass empty path for root
	if err != nil {
		// Wrap detection error as ImageProcessingError (though could be ChartParsingError if structure is bad)
		// For simplicity, using ImageProcessingError as it relates to finding images.
		return nil, &ImageProcessingError{Err: fmt.Errorf("image detection failed: %w", err)}
	}
	debug.DumpValue("Found images", images)
	debug.DumpValue("Unsupported structures", unsupportedMatches)

	// --- START: Strict Mode Check ---
	if g.strict && len(unsupportedMatches) > 0 {
		// Combine details of unsupported structures into the error message
		var details []string
		for _, match := range unsupportedMatches {
			details = append(details, fmt.Sprintf("path=%v type=%d error='%v'", match.Location, match.Type, match.Error))
		}
		errMsg := fmt.Sprintf("strict mode enabled: unsupported structures found (%d): [%s]",
			len(unsupportedMatches), strings.Join(details, "; "))
		debug.Println(errMsg)
		// Return a generic error indicating the failure due to strict mode.
		// The command layer will interpret this and set the correct exit code (5).
		return nil, errors.New(errMsg)
	}
	// --- END: Strict Mode Check ---

	// Determine eligible images *before* processing
	eligibleImages := []image.DetectedImage{}
	for _, img := range images {
		if img.Reference == nil {
			continue // Should not happen, but guard anyway
		}
		// Skip excluded registries
		if g.isExcluded(img.Reference.Registry) {
			debug.Printf("Skipping image from excluded registry: %s", img.Reference.String())
			continue
		}
		// --- MODIFIED ELIGIBILITY CHECK ---
		// If sourceRegistries list is empty, NO images are eligible.
		if len(g.sourceRegistries) == 0 {
			debug.Printf("Skipping image because source registry list is empty: %s", img.Reference.String())
			continue
		}
		// If sourceRegistries is NOT empty, skip if image registry is not in the list.
		if !g.isSourceRegistry(img.Reference.Registry) {
			debug.Printf("Skipping image from non-source registry: %s", img.Reference.String())
			continue
		}
		// If not excluded and (list is not empty AND registry is in the list)
		eligibleImages = append(eligibleImages, img)
	}
	eligibleImagesCount := len(eligibleImages)
	debug.Printf("Eligible images for processing: %d", eligibleImagesCount)

	var unsupported []override.UnsupportedStructure
	for _, match := range unsupportedMatches {
		unsupported = append(unsupported, override.UnsupportedStructure{
			Path: match.Location,
			Type: locationTypeToString(image.LocationType(match.Type)),
		})
	}

	if eligibleImagesCount == 0 {
		debug.Printf("No eligible images requiring override found in chart.")
		return &override.File{
			ChartPath:   g.chartPath,
			ChartName:   filepath.Base(g.chartPath),
			Overrides:   make(map[string]interface{}),
			Unsupported: unsupported,
		}, nil
	}

	imagesSuccessfullyProcessed := 0
	finalOverrides := make(map[string]interface{}) // Initialize empty map for overrides

	var processingErrors []*ImageProcessingError

	// Loop over ELIGIBLE images only
	for i, img := range eligibleImages {
		processingFailed := false

		// No need to re-check source/exclude here, already filtered
		debug.Printf("Processing eligible image %d/%d: Path: %v, Ref: %s (%s)", i+1, eligibleImagesCount, img.Path, img.Reference.String(), img.Pattern)

		if img.Reference == nil { // Keep guard clause
			debug.Printf("Skipping image %d due to nil reference", i+1)
			processingFailed = true
			processingErrors = append(processingErrors, &ImageProcessingError{
				Path: img.Path,
				Ref:  "<nil>",
				Err:  errors.New("nil image reference detected during processing"),
			})
			continue
		}

		// Get the transformed repository path from the strategy
		transformedRepoPath, pathErr := g.pathStrategy.GeneratePath(img.Reference, g.targetRegistry)
		if pathErr != nil {
			debug.Printf("Error generating path: %v", pathErr)
			// Store error for later threshold check
			processingErrors = append(processingErrors, &ImageProcessingError{
				Path: img.Path,
				Ref:  img.Reference.String(),
				Err:  fmt.Errorf("path strategy failed: %w", pathErr),
			})
			continue // Skip this image
		}
		debug.Printf("Transformed repository path: %s", transformedRepoPath)

		// --- Construct the target value MAP ---
		// Always override with the map structure, regardless of original type (string or map)
		valueToSet := map[string]interface{}{}
		valueToSet["registry"] = g.targetRegistry
		valueToSet["repository"] = transformedRepoPath
		// Preserve original tag or digest
		if img.Reference.Digest != "" {
			valueToSet["digest"] = img.Reference.Digest
		} else {
			// Ensure tag is included even if it was empty in the original
			// (avoids Helm potentially complaining about missing tag if repo/registry change)
			valueToSet["tag"] = img.Reference.Tag
		}
		// --- End Construct the target value MAP ---

		debug.Printf("[DEBUG irr GEN] Processing Eligible Image: Path=%v, OriginalRef=%s, Type=%s", img.Path, img.Reference.String(), img.Pattern)
		debug.DumpValue("[DEBUG irr GEN] Value to set", valueToSet)

		// Set the new value (map) in the NEW override structure
		debug.Printf("[DEBUG irr GEN] Calling SetValueAtPath with Path: %v", img.Path)
		err = override.SetValueAtPath(finalOverrides, img.Path, valueToSet) // Use finalOverrides map
		if err != nil {
			debug.Printf("Error setting value at path %v: %v", img.Path, err)
			processingFailed = true
			processingErrors = append(processingErrors, &ImageProcessingError{
				Path: img.Path,
				Ref:  img.Reference.String(),
				Err:  fmt.Errorf("failed to set override value: %w", err),
			})
			continue // Skip to next image on set value error
		}
		debug.Printf("[DEBUG irr GEN] SetValueAtPath successful for path: %v", img.Path)

		if !processingFailed {
			imagesSuccessfullyProcessed++
		}
	} // End loop over images

	// Check if processing threshold was met
	if eligibleImagesCount > 0 {
		successRate := float64(imagesSuccessfullyProcessed) / float64(eligibleImagesCount) * percentageMultiplier
		debug.Printf("Image processing success rate: %.2f%% (Processed: %d, Eligible: %d, Threshold: %d%%)",
			successRate, imagesSuccessfullyProcessed, eligibleImagesCount, g.threshold)
		if int(successRate) < g.threshold {
			combinedErr := combineProcessingErrors(processingErrors)
			thresholdErr := &ThresholdError{
				Threshold:   g.threshold,
				ActualRate:  int(successRate),
				Eligible:    eligibleImagesCount,
				Processed:   imagesSuccessfullyProcessed,
				WrappedErrs: combinedErr,
			}
			return nil, thresholdErr
		}
	}

	// Return the generated override file
	debug.DumpValue("Final generated overrides", finalOverrides)
	return &override.File{
		ChartPath:   g.chartPath,
		ChartName:   chartData.Name(), // Use chart name from loaded data
		Overrides:   finalOverrides,   // Use the correctly built overrides map
		Unsupported: unsupported,
	}, nil
}

// combineProcessingErrors converts a slice of *ImageProcessingError to a slice of error.
func combineProcessingErrors(processingErrors []*ImageProcessingError) []error {
	errs := make([]error, len(processingErrors))
	for i, pe := range processingErrors {
		errs[i] = pe // *ImageProcessingError already satisfies the error interface
	}
	return errs
}

// ThresholdError represents a failure due to not meeting the processing threshold.
type ThresholdError struct {
	Threshold   int
	ActualRate  int
	Eligible    int
	Processed   int
	WrappedErrs []error
}

func (e *ThresholdError) Error() string {
	errMsg := fmt.Sprintf("processing failed: success rate %.2f%% below threshold %d%% (%d/%d eligible images processed)",
		float64(e.ActualRate)/float64(e.Eligible)*percentageMultiplier, e.Threshold, e.Processed, e.Eligible)
	if len(e.WrappedErrs) > 0 {
		var errDetails []string
		for _, err := range e.WrappedErrs {
			errDetails = append(errDetails, err.Error())
		}
		errMsg = fmt.Sprintf("%s - Errors: [%s]", errMsg, strings.Join(errDetails, "; "))
	}
	return errMsg
}

// extractSubtree extracts a submap from a nested map structure based on a path
// nolint:unused // Kept for potential future uses
func extractSubtree(data map[string]interface{}, path []string) map[string]interface{} {
	if len(path) == 0 {
		return nil
	}

	result := make(map[string]interface{})
	current := result
	value := data

	// Build the path structure
	for i, key := range path[:len(path)-1] {
		// Handle array indices
		if strings.HasPrefix(key, "[") && strings.HasSuffix(key, "]") {
			// Extract index
			indexStr := key[1 : len(key)-1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil
			}

			// Get the array from the source data
			parentKey := path[i-1]
			if arr, ok := value[parentKey].([]interface{}); ok && index < len(arr) {
				// Create a new array in the result
				newArr := make([]interface{}, index+1)
				current[parentKey] = newArr

				// Move to the array element
				if mapValue, ok := arr[index].(map[string]interface{}); ok {
					value = mapValue
					newMap := make(map[string]interface{})
					newArr[index] = newMap
					current = newMap
				}
			}
			continue
		}

		// Handle regular map keys
		if nextValue, ok := value[key].(map[string]interface{}); ok {
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
			value = nextValue
		}
	}

	// Set the final value
	lastKey := path[len(path)-1]
	if finalValue, ok := value[lastKey]; ok {
		current[lastKey] = finalValue
	}

	return result
}

// isExcluded checks if a registry is in the exclude list
func (g *Generator) isExcluded(registry string) bool {
	for _, excluded := range g.excludeRegistries {
		if registry == excluded {
			return true
		}
	}
	return false
}

// isSourceRegistry checks if a registry is in the source list
func (g *Generator) isSourceRegistry(registry string) bool {
	for _, source := range g.sourceRegistries {
		if registry == source {
			return true
		}
	}
	return false
}

// GenerateOverrides generates a map of Helm overrides for image references.
// @param chartData: The loaded Helm chart containing values and dependencies
// @param targetRegistry: The target registry where images should be pushed
// @param sourceRegistries: List of source registries to process
// @param excludeRegistries: List of registries to skip
// @param pathStrategy: Strategy for transforming image paths
// @param verbose: Enable verbose logging
// @returns: map[string]interface{} containing the override structure
// @returns: error if processing fails
// @llm-helper This is the main entry point for generating overrides
func GenerateOverrides(chartData *chart.Chart, targetRegistry string, sourceRegistries []string, excludeRegistries []string, pathStrategy strategy.PathStrategy, verbose bool) (map[string]interface{}, error) {
	debug.FunctionEnter("GenerateOverrides")
	defer debug.FunctionExit("GenerateOverrides")

	debug.DumpValue("Target Registry", targetRegistry)
	debug.DumpValue("Source Registries", sourceRegistries)
	debug.DumpValue("Exclude Registries", excludeRegistries)
	debug.DumpValue("Path Strategy", pathStrategy)

	// Create a new image detector with context
	detectionContext := &image.DetectionContext{
		GlobalRegistry: targetRegistry,
		Strict:         false,
	}
	detector := image.NewDetector(*detectionContext)

	// Process the main chart (Helm loader should have merged values)
	debug.Printf("Processing combined chart values: %s", chartData.Name())
	// Call processChartForOverrides just once with the potentially merged chartData
	overrides, err := processChartForOverrides(chartData, targetRegistry, sourceRegistries, excludeRegistries, pathStrategy, verbose, detector)
	if err != nil {
		// Changed error wrapping to match original intent better
		return nil, fmt.Errorf("error processing chart values: %w", err)
	}
	// The result is directly the overrides map generated
	return overrides, nil
}

// ImageDetector defines the interface for detecting images in chart values
type ImageDetector interface {
	DetectImages(values interface{}, path []string) ([]image.DetectedImage, []image.UnsupportedImage, error)
}

// processChartForOverrides processes a single chart and its values.
func processChartForOverrides(chartData *chart.Chart, targetRegistry string, sourceRegistries []string, excludeRegistries []string, pathStrategy strategy.PathStrategy, _ bool, detector ImageDetector) (map[string]interface{}, error) {
	debug.FunctionEnter("processChartForOverrides")
	defer debug.FunctionExit("processChartForOverrides")

	debug.Printf("Processing chart: %s", chartData.Name())
	debug.DumpValue("Chart Values for Detection", chartData.Values)

	// Initialize an empty map for the overrides generated by this chart scope
	overrides := make(map[string]interface{})

	// Use the provided detector
	detectedImages, unsupportedMatches, err := detector.DetectImages(chartData.Values, []string{})
	if err != nil {
		return nil, fmt.Errorf("error detecting images: %w", err)
	}

	debug.Printf("Detected %d images", len(detectedImages))
	debug.Printf("Found %d unsupported structures", len(unsupportedMatches))

	// Process each detected image
	for _, img := range detectedImages {
		debug.Printf("Processing image at path: %v", img.Path)
		debug.DumpValue("Image Reference", img.Reference)

		// Skip if the image is from an excluded registry
		if img.Reference != nil && img.Reference.Registry != "" {
			if isRegistryExcluded(img.Reference.Registry, excludeRegistries) {
				debug.Printf("Skipping excluded registry: %s", img.Reference.Registry)
				continue
			}
			// Skip if not from a source registry
			if !isRegistryInList(img.Reference.Registry, sourceRegistries) {
				debug.Printf("Skipping non-source registry: %s", img.Reference.Registry)
				continue
			}
		}

		// Transform the image reference using the path strategy
		var transformedRepoPath string
		transformedRepoPath, err = pathStrategy.GeneratePath(img.Reference, targetRegistry)
		if err != nil {
			debug.Printf("Error transforming image reference: %v", err)
			continue // Skip this image if transformation fails
		}
		debug.Printf("Transformed repo path: %s", transformedRepoPath)

		// Create the image configuration with extracted parts
		imageConfig := map[string]interface{}{
			"registry":   targetRegistry,
			"repository": transformedRepoPath,
		}

		// Handle tag or digest
		if img.Reference.Digest != "" {
			imageConfig["digest"] = img.Reference.Digest
		} else if img.Reference.Tag != "" {
			imageConfig["tag"] = img.Reference.Tag
		}

		// Set the value at the correct path IN THE NEW OVERRIDES MAP
		err = override.SetValueAtPath(overrides, img.Path, imageConfig)
		if err != nil {
			debug.Printf("Error setting value in override map at path %v: %v", img.Path, err)
			continue // Skip if we cannot set the override
		}
	}

	// Return only the generated overrides for this chart scope
	debug.DumpValue("Generated Overrides for Scope", overrides)
	return overrides, nil
}

// mergeOverrides merges the source map into the destination map recursively.
// @param dest: Destination map to merge into
// @param src: Source map to merge from
// @llm-helper This function handles nested maps and preserves existing values
// @llm-helper Arrays are replaced, not merged
// @llm-helper Map values are merged recursively
func mergeOverrides(dest, src map[string]interface{}) {
	debug.FunctionEnter("mergeOverrides")
	defer debug.FunctionExit("mergeOverrides")

	debug.DumpValue("Destination Map", dest)
	debug.DumpValue("Source Map", src)

	for k, v := range src {
		debug.Printf("Processing key: %s", k)

		if destV, exists := dest[k]; exists {
			debug.Printf("Key %s exists in destination", k)
			// If both values are maps, merge them recursively
			if destMap, ok := destV.(map[string]interface{}); ok {
				if srcMap, ok := v.(map[string]interface{}); ok {
					debug.Printf("Both values are maps, merging recursively")
					mergeOverrides(destMap, srcMap)
					continue
				}
			}
		}

		// Otherwise, just overwrite the value
		debug.Printf("Setting key %s in destination", k)
		dest[k] = v
	}

	debug.DumpValue("Merged Result", dest)
}

// CommandRunner defines an interface for running external commands.
type CommandRunner interface {
	Run(name string, arg ...string) ([]byte, error)
}

// osCommandRunner implements CommandRunner using the os/exec package.
type osCommandRunner struct{}

// NewOSCommandRunner creates a new CommandRunner that uses the real os/exec package.
func NewOSCommandRunner() CommandRunner {
	return &osCommandRunner{}
}

// Run executes the command and returns its combined stdout/stderr output and error.
func (r *osCommandRunner) Run(name string, arg ...string) ([]byte, error) {
	cmd := exec.Command(name, arg...)
	// Using CombinedOutput captures both stdout and stderr, useful for debugging Helm errors.
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Wrap error from external command execution
		return nil, fmt.Errorf("helm command failed: %w", err)
	}
	return output, nil
}

// ValidateHelmTemplate runs `helm template` and validates the output.
// It now accepts a CommandRunner to allow mocking.
func ValidateHelmTemplate(runner CommandRunner, chartPath string, overrides []byte) error {
	debug.FunctionEnter("ValidateHelmTemplate")
	defer debug.FunctionExit("ValidateHelmTemplate")

	// Create a temporary file for overrides
	tempDir, err := os.MkdirTemp("", "helm-overrides-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir for overrides: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			debug.Printf("Warning: failed to clean up temp dir %s: %v", tempDir, err)
		}
	}() // Clean up temp dir

	overrideFilePath := filepath.Join(tempDir, "overrides.yaml")
	if err := os.WriteFile(overrideFilePath, overrides, defaultFilePerm); err != nil {
		return fmt.Errorf("failed to write temp overrides file: %w", err)
	}
	debug.Printf("Temporary override file written to: %s", overrideFilePath)

	// Prepare helm template command arguments
	args := []string{"template", "release-name", chartPath, "-f", overrideFilePath}
	debug.Printf("Running helm command: helm %v", args)

	// Run the command using the provided runner
	output, err := runner.Run("helm", args...)
	if err != nil {
		// exec.ExitError is common, provide output context
		debug.Printf("Helm template command failed. Error: %v\nOutput:\n%s", err, string(output))
		return fmt.Errorf("helm template command failed: %w. Output: %s", err, string(output))
	}
	debug.Printf("Helm template command successful. Output length: %d", len(output))
	// debug.DumpValue("Helm Template Output", string(output)) // Optional: dump full output

	// Basic validation of the template output
	if err := validateYAML(output); err != nil {
		return fmt.Errorf("failed to parse helm template output: %w", err)
	}

	// Simplified check for common issues on the raw bytes for now
	// A more robust check for lists as map keys would be implemented here
	// This placeholder code is intentionally not implementing the check yet
	/*
		if bytes.Contains(output, []byte("\n  - ")) && bytes.Contains(output, []byte(":\n")) { // Very crude check
			// Placeholder: A more robust check for lists as map keys needed here.
			// Example: Check lines starting with "  -" immediately after a line ending with ":"
			// return fmt.Errorf("common issue detected: map key might be a list (heuristic check)")
		}
	*/

	debug.Println("Helm template output validated successfully.")
	return nil
}

// validateYAML checks if the byte slice contains valid YAML.
// Keep this unexported as it's an internal helper for ValidateHelmTemplate.
func validateYAML(yamlData []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(yamlData))
	var node yaml.Node
	for dec.Decode(&node) == nil { //revive:disable-line:empty-block
		// This loop consumes all YAML documents in the stream.
	}

	// After the loop, check for decoding errors that are not EOF.
	// The last call to Decode that returns io.EOF signifies successful parsing of all documents.
	if err := dec.Decode(&node); err != nil && !errors.Is(err, io.EOF) {
		debug.Printf("YAML validation failed: %v", err)
		return fmt.Errorf("invalid YAML structure: %w", err) // Use original error message format
	}
	return nil // No error means valid YAML
}

// validateCommonIssues checks for specific problematic patterns in parsed YAML.
// Note: This function is complex to implement correctly without false positives.
// Keeping it simple or removing might be better initially.
// func validateCommonIssues(obj map[string]interface{}) error { ... }

// cleanupTemplateVariables removes or simplifies Helm template variables
// It prioritizes extracting default values if specified.
// nolint:unused // Kept for potential future uses
func cleanupTemplateVariables(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		// Check if it looks like a template variable
		if strings.Contains(v, "{{") || strings.Contains(v, "${") {
			// First, try to extract default value if present
			if strings.Contains(v, "| default") {
				parts := strings.SplitN(v, "| default", maxSplitTwo)
				if len(parts) > 1 {
					defaultValStr := strings.TrimSpace(parts[1])
					// More robustly remove trailing template characters like '}}', ' }', etc.
					closingMarkers := []string{"}}", "}"}
					for _, marker := range closingMarkers {
						if strings.HasSuffix(defaultValStr, marker) {
							defaultValStr = strings.TrimSpace(strings.TrimSuffix(defaultValStr, marker))
							break // Stop after removing the first found marker from the end
						}
					}

					// Try to convert to appropriate type
					if defaultValStr == "true" {
						return true
					}
					if defaultValStr == "false" {
						return false
					}
					if i, err := strconv.Atoi(defaultValStr); err == nil {
						return i
					}
					// Unquote string values
					if unquoted, err := strconv.Unquote(defaultValStr); err == nil {
						return unquoted
					}
					// Return the trimmed default string if it wasn't quoted or convertible
					return defaultValStr
				}
			}

			// If no default value, apply heuristics
			vLower := strings.ToLower(v)
			// Check for boolean flags FIRST
			if strings.Contains(vLower, "enabled") || strings.Contains(vLower, "disabled") {
				return false // Default to false for boolean flags without explicit defaults
			}
			// Then check for image-related fields
			if strings.Contains(vLower, "image") ||
				strings.Contains(vLower, "repository") ||
				strings.Contains(vLower, "registry") ||
				strings.Contains(vLower, "tag") {
				return "" // Empty string for image fields without defaults
			}
			// Generic fallback for other templates without defaults
			return "" // Or potentially nil, depending on desired behavior
		}
		// Not a template string, return as is
		return v
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			if val == nil {
				continue // Skip nil values
			}
			result[key] = cleanupTemplateVariables(val)
		}
		return result
	case []interface{}:
		result := make([]interface{}, 0, len(v))
		for _, item := range v {
			if item == nil {
				continue // Skip nil values
			}
			result = append(result, cleanupTemplateVariables(item))
		}
		return result
	case float64:
		// Convert float64 to int if it's a whole number
		if float64(int(v)) == v {
			return int(v)
		}
		return v
	case nil:
		return nil
	default:
		return v
	}
}

// OverridesToYAML converts a map of overrides to YAML format
func OverridesToYAML(overrides map[string]interface{}) ([]byte, error) {
	debug.Printf("Marshaling overrides to YAML")
	// Wrap error from external YAML library
	yamlBytes, err := yaml.Marshal(overrides)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
	}
	return yamlBytes, nil
}

// Helper function to check if a registry is in a list
func isRegistryInList(registry string, list []string) bool {
	for _, r := range list {
		if r == registry {
			return true
		}
	}
	return false
}

// Helper function to check if a registry is excluded
func isRegistryExcluded(registry string, excludeList []string) bool {
	return isRegistryInList(registry, excludeList)
}
