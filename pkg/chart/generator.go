package chart

import (
	"bytes"
	"errors"
	"fmt"
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

// Package chart provides functionality for handling Helm charts and generating image override values.
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

// ImageReference represents an image reference found in a chart
type ImageReference struct {
	Path      []string
	Reference *image.ImageReference
}

// Generator generates image overrides for a Helm chart
type Generator struct {
	chartPath         string
	targetRegistry    string
	sourceRegistries  []string
	excludeRegistries []string
	pathStrategy      strategy.PathStrategy
	mappings          *registry.RegistryMappings
	strict            bool
	threshold         int
	loader            Loader
}

// NewGenerator creates a new Generator
func NewGenerator(chartPath, targetRegistry string, sourceRegistries, excludeRegistries []string, pathStrategy strategy.PathStrategy, mappings *registry.RegistryMappings, strict bool, threshold int, loader Loader) *Generator {
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
func (g *Generator) Generate() (*override.OverrideFile, error) {
	debug.FunctionEnter("Generator.Generate")
	defer debug.FunctionExit("Generator.Generate")

	chartData, err := g.loader.Load(g.chartPath)
	if err != nil {
		// Return specific ChartParsingError
		return nil, &ChartParsingError{FilePath: g.chartPath, Err: err}
	}

	baseValuesCopy := override.DeepCopy(chartData.Values)
	baseValues, ok := baseValuesCopy.(map[string]interface{})
	if !ok {
		// This indicates a fundamental issue with the chart's values structure
		return nil, &ChartParsingError{FilePath: g.chartPath, Err: errors.New("chart values are not a map[string]interface{}")}
	}
	debug.DumpValue("Base values for type checking", baseValues)

	images, unsupportedMatches, err := image.DetectImages(chartData.Values, []string{}, g.sourceRegistries, g.excludeRegistries, g.strict)
	if err != nil {
		// Wrap detection error as ImageProcessingError (though could be ChartParsingError if structure is bad)
		// For simplicity, using ImageProcessingError as it relates to finding images.
		return nil, &ImageProcessingError{Err: fmt.Errorf("image detection failed: %w", err)}
	}
	debug.DumpValue("Found images", images)
	debug.DumpValue("Unsupported structures", unsupportedMatches)

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
		// Skip non-source registries (unless source list is empty)
		if len(g.sourceRegistries) > 0 && !g.isSourceRegistry(img.Reference.Registry) {
			debug.Printf("Skipping image from non-source registry: %s", img.Reference.String())
			continue
		}
		eligibleImages = append(eligibleImages, img)
	}
	eligibleImagesCount := len(eligibleImages)
	debug.Printf("Eligible images for processing: %d", eligibleImagesCount)

	var unsupported []override.UnsupportedStructure
	for _, match := range unsupportedMatches {
		unsupported = append(unsupported, override.UnsupportedStructure{
			Path: match.Location,
			Type: locationTypeToString(match.LocationType),
		})
	}

	if eligibleImagesCount == 0 {
		debug.Printf("No eligible images requiring override found in chart.")
		return &override.OverrideFile{
			ChartPath:   g.chartPath,
			ChartName:   filepath.Base(g.chartPath),
			Overrides:   make(map[string]interface{}),
			Unsupported: unsupported,
		}, nil
	}

	imagesSuccessfullyProcessed := 0
	modifiedValuesCopy := override.DeepCopy(baseValues)
	modifiedValues, ok := modifiedValuesCopy.(map[string]interface{})
	if !ok {
		// This should ideally not happen if DeepCopy is correct, but return a generic error if it does.
		return nil, errors.New("internal error: failed to create deep copy of values map")
	}

	var processingErrors []*ImageProcessingError

	// Loop over ELIGIBLE images only
	for i, img := range eligibleImages {
		processingFailed := false

		// No need to re-check source/exclude here, already filtered
		debug.Printf("Processing eligible image %d/%d: Path: %v, Ref: %s (%s)", i+1, eligibleImagesCount, img.Location, img.Reference.String(), locationTypeToString(img.LocationType))

		if img.Reference == nil { // Keep guard clause
			debug.Printf("Skipping image %d due to nil reference", i+1)
			processingFailed = true
			processingErrors = append(processingErrors, &ImageProcessingError{
				Path: img.Location,
				Ref:  "<nil>",
				Err:  errors.New("nil image reference detected during processing"),
			})
			continue
		}

		// Get the transformed repository path from the strategy
		transformedRepoPath, pathErr := g.pathStrategy.GeneratePath(img.Reference, g.targetRegistry, g.mappings)
		if pathErr != nil {
			debug.Printf("Error generating path: %v", pathErr)
			// Store error for later threshold check
			processingErrors = append(processingErrors, &ImageProcessingError{
				Path: img.Location,
				Ref:  img.Reference.String(),
				Err:  fmt.Errorf("path strategy failed: %w", pathErr),
			})
			continue // Skip this image
		}
		debug.Printf("Transformed repository path: %s", transformedRepoPath)

		// Create the image configuration map using the target registry,
		// the transformed repository path, and the original tag/digest.
		imageConfig := map[string]interface{}{
			"registry":   g.targetRegistry,    // Always use the main target registry
			"repository": transformedRepoPath, // Use the path generated by the strategy
		}

		// Add original tag or digest
		if img.Reference.Digest != "" {
			imageConfig["digest"] = img.Reference.Digest
		} else if img.Reference.Tag != "" {
			imageConfig["tag"] = img.Reference.Tag
		} // If neither exists, we don't add them, resulting in 'latest' behavior implicitly

		// Set the value at the correct path
		err = override.SetValueAtPath(modifiedValues, img.Location, imageConfig)
		if err != nil {
			debug.Printf("Error setting value at path %v: %v", img.Location, err)
			// Add error to the list
			processingErrors = append(processingErrors, &ImageProcessingError{
				Path: img.Location,
				Ref:  img.Reference.String(),
				Err:  fmt.Errorf("setting override value failed: %w", err),
			})
			processingFailed = true
		}

		// Count successfully processed images (only if no error occurred for *this* image)
		if !processingFailed {
			imagesSuccessfullyProcessed++
		}
	} // End loop over images

	// Check threshold against ELIGIBLE images count
	if eligibleImagesCount > 0 {
		successPercentage := (imagesSuccessfullyProcessed * 100) / eligibleImagesCount
		debug.Printf("Success rate: %d%% (%d/%d)", successPercentage, imagesSuccessfullyProcessed, eligibleImagesCount)
		if successPercentage < g.threshold {
			debug.Printf("Threshold check failed: %d%% < %d%%", successPercentage, g.threshold)
			// Aggregate errors
			finalErr := &ThresholdNotMetError{Actual: successPercentage, Required: g.threshold}
			if len(processingErrors) > 0 {
				// Combine errors into a single message
				var errorStrings []string
				for _, procErr := range processingErrors {
					errorStrings = append(errorStrings, procErr.Error())
				}
				combinedErrorMsg := fmt.Sprintf("threshold failed (%d%% < %d%%). Processing errors: [%s]",
					successPercentage, g.threshold, strings.Join(errorStrings, "; "))
				// Return a generic error with the combined message
				return nil, errors.New(combinedErrorMsg)
			}
			return nil, finalErr // Return threshold error if no specific processing errors
		}
	} else {
		debug.Printf("No eligible images were processed, threshold check skipped.")
	}

	// Construct the final override file structure
	overrides := make(map[string]interface{})
	for _, img := range images {
		if g.isExcluded(img.Reference.Registry) || !g.isSourceRegistry(img.Reference.Registry) {
			continue
		}

		newValue, found := image.GetValueAtPath(modifiedValues, img.Location)
		if !found {
			debug.Printf("Skipping path %v in final overrides: value not found in modified map", img.Location)
			continue
		}

		err = image.SetValueAtPath(overrides, img.Location, newValue)
		if err != nil {
			debug.Printf("Error setting path %v in final overrides map: %v", img.Location, err)
		}
	}
	debug.DumpValue("Final minimal overrides structure", overrides)

	return &override.OverrideFile{
		ChartPath:   g.chartPath,
		ChartName:   filepath.Base(g.chartPath),
		Overrides:   overrides,
		Unsupported: unsupported,
	}, nil
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
func GenerateOverrides(chartData *chart.Chart, targetRegistry string, sourceRegistries []string, excludeRegistries []string, pathStrategy strategy.PathStrategy, mappings *registry.RegistryMappings, verbose bool) (map[string]interface{}, error) {
	debug.FunctionEnter("GenerateOverrides")
	defer debug.FunctionExit("GenerateOverrides")

	debug.DumpValue("Target Registry", targetRegistry)
	debug.DumpValue("Source Registries", sourceRegistries)
	debug.DumpValue("Exclude Registries", excludeRegistries)
	debug.DumpValue("Path Strategy", pathStrategy)

	result := make(map[string]interface{})

	// Process the main chart (Helm loader should have merged values)
	debug.Printf("Processing combined chart values: %s", chartData.Name())
	// Call processChartForOverrides just once with the potentially merged chartData
	overrides, err := processChartForOverrides(chartData, targetRegistry, sourceRegistries, excludeRegistries, pathStrategy, mappings, verbose)
	if err != nil {
		// Changed error wrapping to match original intent better
		return nil, fmt.Errorf("error processing chart values: %w", err)
	}
	// The result is directly the overrides map generated
	result = overrides

	// Remove the explicit dependency processing loop
	/*
		debug.Printf("Processing %d dependencies", len(chartData.Dependencies()))
		for _, dep := range chartData.Dependencies() {
			debug.Printf("Processing dependency: %s", dep.Name())
			// ... removed code ...
		}
	*/

	debug.DumpValue("Final Combined Overrides", result)
	return result, nil
}

// processChartForOverrides processes a single chart and its values.
func processChartForOverrides(chartData *chart.Chart, targetRegistry string, sourceRegistries []string, excludeRegistries []string, pathStrategy strategy.PathStrategy, mappings *registry.RegistryMappings, verbose bool) (map[string]interface{}, error) {
	debug.FunctionEnter("processChartForOverrides")
	defer debug.FunctionExit("processChartForOverrides")

	debug.Printf("Processing chart: %s", chartData.Name())
	debug.DumpValue("Chart Values for Detection", chartData.Values) // Log values used for detection

	// Initialize an empty map for the overrides generated by this chart scope
	overrides := make(map[string]interface{})

	// No need for baseValues copy anymore
	// baseValuesCopy := override.DeepCopy(chartData.Values)
	// baseValues, ok := baseValuesCopy.(map[string]interface{})
	// if !ok {
	// 	 return nil, fmt.Errorf("chart values are not a valid map[string]interface{}")
	// }

	// Create a new image detector with context
	detector := image.NewImageDetector(&image.DetectionContext{
		GlobalRegistry: targetRegistry,
		Strict:         false,
	})

	detectedImages, unsupported, err := detector.DetectImages(chartData.Values, []string{})
	if err != nil {
		return nil, fmt.Errorf("error detecting images: %w", err)
	}

	debug.Printf("Detected %d images", len(detectedImages))
	debug.Printf("Found %d unsupported structures", len(unsupported))

	// Process each detected image
	for _, img := range detectedImages {
		debug.Printf("Processing image at path: %v", img.Location)
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
		var transformedRepoPath string // Changed variable name for clarity
		transformedRepoPath, err = pathStrategy.GeneratePath(img.Reference, targetRegistry, mappings)
		if err != nil {
			debug.Printf("Error transforming image reference: %v", err)
			continue // Skip this image if transformation fails
		}
		debug.Printf("Transformed repo path: %s", transformedRepoPath)

		// Create the image configuration with extracted parts
		// Always create the structured map override
		imageConfig := map[string]interface{}{
			"registry":   targetRegistry,      // Use the overall target registry
			"repository": transformedRepoPath, // Use the path generated by the strategy
		}

		// Handle tag or digest
		if img.Reference.Digest != "" {
			imageConfig["digest"] = img.Reference.Digest
		} else if img.Reference.Tag != "" {
			imageConfig["tag"] = img.Reference.Tag
		}

		// Set the value at the correct path IN THE NEW OVERRIDES MAP
		err = override.SetValueAtPath(overrides, img.Location, imageConfig)
		if err != nil {
			debug.Printf("Error setting value in override map at path %v: %v", img.Location, err)
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
	return cmd.CombinedOutput()
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
	if err := os.WriteFile(overrideFilePath, overrides, 0600); err != nil {
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
	// and will be developed in a future PR when we have better detection criteria
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
	// Use yaml.v3 decoder which is stricter
	dec := yaml.NewDecoder(bytes.NewReader(yamlData))
	var node yaml.Node
	for dec.Decode(&node) == nil {
		// Successfully decoded a document, continue
	}
	// Check the error after the loop (io.EOF is expected on success)
	if err := dec.Decode(&node); err != nil && err.Error() != "EOF" {
		debug.Printf("YAML validation failed: %v", err)
		return fmt.Errorf("invalid YAML structure: %w", err)
	}
	return nil
}

// validateCommonIssues checks for specific problematic patterns in parsed YAML.
// Note: This function is complex to implement correctly without false positives.
// Keeping it simple or removing might be better initially.
// func validateCommonIssues(obj map[string]interface{}) error { ... }

// cleanupTemplateVariables removes or simplifies Helm template variables
// nolint:unused // Kept for potential future uses
func cleanupTemplateVariables(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		// If string contains template variables, return appropriate default
		if strings.Contains(v, "{{") || strings.Contains(v, "${") {
			// For image-related fields, return empty string
			if strings.Contains(strings.ToLower(v), "image") ||
				strings.Contains(strings.ToLower(v), "repository") ||
				strings.Contains(strings.ToLower(v), "registry") ||
				strings.Contains(strings.ToLower(v), "tag") {
				return ""
			}
			// For address fields, return empty string
			if strings.Contains(strings.ToLower(v), "address") {
				return ""
			}
			// For name fields, return empty string
			if strings.Contains(strings.ToLower(v), "name") {
				return ""
			}
			// For path fields, return empty string
			if strings.Contains(strings.ToLower(v), "path") {
				return ""
			}
			// For boolean fields containing "enabled" or "disabled", return false
			if strings.Contains(strings.ToLower(v), "enabled") || strings.Contains(strings.ToLower(v), "disabled") {
				return false
			}
			// Try to extract default value if present
			if strings.Contains(v, "| default") {
				parts := strings.Split(v, "| default")
				if len(parts) > 1 {
					defaultVal := strings.TrimSpace(parts[1])
					defaultVal = strings.TrimSuffix(defaultVal, "}}")
					defaultVal = strings.TrimSpace(defaultVal)
					// Try to convert to appropriate type
					if defaultVal == "true" {
						return true
					}
					if defaultVal == "false" {
						return false
					}
					if i, err := strconv.Atoi(defaultVal); err == nil {
						return i
					}
					return defaultVal
				}
			}
			return ""
		}
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
	return yaml.Marshal(overrides)
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
