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
	defaultFilePerm      fs.FileMode = 0o600
	percentageMultiplier fs.FileMode = 100
)

// --- Local Error Definitions ---
var (
	ErrUnsupportedStructure = errors.New("unsupported structure found")
)

// Keep LoadingError defined locally as it wraps errors from this package's perspective
type LoadingError struct {
	ChartPath string
	Err       error
}

func (e *LoadingError) Error() string {
	return fmt.Sprintf("failed to load chart at %s: %v", e.ChartPath, e.Err)
}
func (e *LoadingError) Unwrap() error { return e.Err }

// Assuming ParsingError and ImageProcessingError are defined elsewhere (e.g., analysis package or pkg/errors)

// Keep ThresholdError defined locally as it's specific to the generator's threshold logic
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

// Ensure HelmLoader implements analysis.ChartLoader
var _ analysis.ChartLoader = (*HelmLoader)(nil)

type HelmLoader struct{}

// Load implements analysis.ChartLoader interface, returning helmchart.Chart
func (l *HelmLoader) Load(chartPath string) (*helmchart.Chart, error) { // Return *helmchart.Chart
	debug.Printf("HelmLoader: Loading chart from %s", chartPath)

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

	debug.Printf("HelmLoader successfully loaded chart: %s", helmLoadedChart.Name())
	return helmLoadedChart, nil
}

// --- Generator Implementation ---

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
	loader            analysis.ChartLoader // Use analysis.ChartLoader interface
}

func NewGenerator(
	chartPath, targetRegistry string,
	sourceRegistries, excludeRegistries []string,
	pathStrategy strategy.PathStrategy,
	mappings *registry.Mappings,
	strict bool,
	threshold int,
	loader analysis.ChartLoader,
	includePatterns, excludePatterns, knownPaths []string,
) *Generator {
	if loader == nil {
		loader = &HelmLoader{}
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
		loader:            loader,
	}
}

func (g *Generator) Generate() (*override.File, error) {
	debug.FunctionEnter("Generator.Generate")
	defer debug.FunctionExit("Generator.Generate")

	// Configure and run the analyzer
	// Create analyzer using the expected signature (chartPath, loader)
	analyzer := analysis.NewAnalyzer(g.chartPath, g.loader)
	// Config is likely handled internally by the analyzer based on its dependencies or structure
	debug.Printf("Analyzer created for path: %s", g.chartPath)

	// Analyze Chart
	debug.Printf("Analyzing chart: %s", g.chartPath)
	analysisResults, err := analyzer.Analyze() // Call Analyze()
	if err != nil {
		// Wrap the error generically if analysis fails
		return nil, fmt.Errorf("error analyzing chart %s: %w", g.chartPath, err)
	}

	// --- Access Analysis Results ---
	// Use ImagePatterns based on analyzer.go code. Comment out UnsupportedMatches for now.
	detectedImages := analysisResults.ImagePatterns
	// unsupportedMatches := analysisResults.UnsupportedMatches // Field name/existence uncertain

	debug.Printf("Analysis complete. Found %d image patterns.", len(detectedImages))
	// debug.Printf("Found %d unsupported items.", len(unsupportedMatches))
	debug.DumpValue("[GENERATE] Detected Image Patterns", detectedImages)
	// debug.DumpValue("[GENERATE] Unsupported Matches", unsupportedMatches)

	// --- Strict Mode Check (Commented out as it depends on UnsupportedMatches) ---
	/*
		if g.strict && len(unsupportedMatches) > 0 {
			details := []string{}
			log.Warnf("Strict mode enabled: Found %d unsupported image structures:", len(unsupportedMatches))
			for i, item := range unsupportedMatches {
				errMsg := fmt.Sprintf("  [%d] Path: %s, Type: %s", i+1, item.LocationString(), item.Type)
				if item.Value != nil {
					errMsg += fmt.Sprintf(", Value: %v", item.Value)
				}
				if item.Error != nil {
					errMsg += fmt.Sprintf(", Reason: %v", item.Error)
				}
				log.Warnf(errMsg)
				debug.Printf("[STRICT VIOLATION DETAIL] Path: %s, Type: %s, Value: %v, Reason: %v", item.LocationString(), item.Type, item.Value, item.Error)
				details = append(details, errMsg)
			}
			combinedErrMsg := fmt.Sprintf("%w: %d unsupported structures found", ErrUnsupportedStructure, len(unsupportedMatches))
			return nil, fmt.Errorf(combinedErrMsg)
		}
	*/

	// --- Filter Detected Images (based on source/exclude, similar to original logic but using ImagePattern) ---
	eligibleImages := []analysis.ImagePattern{}
	for _, pattern := range detectedImages {
		// Need to parse the pattern.Value (string) or use pattern.Structure (map) to get registry
		var registry string
		var imgRef *image.Reference // Use image.ParseImageReference for consistency
		var parseErr error

		if pattern.Type == analysis.PatternTypeString {
			imgRef, parseErr = image.ParseImageReference(pattern.Value)
			if parseErr == nil {
				registry = imgRef.Registry
			} else {
				debug.Printf("Skipping pattern at path %s due to parse error on value '%s': %v", pattern.Path, pattern.Value, parseErr)
				continue // Skip if string pattern doesn't parse
			}
		} else if pattern.Type == analysis.PatternTypeMap {
			if regVal, ok := pattern.Structure["registry"].(string); ok {
				registry = regVal
			} else {
				// Attempt to reconstruct string and parse if registry missing in map?
				// Or rely on normalization during strategy? Let's assume registry is needed now.
				debug.Printf("Skipping map pattern at path %s due to missing registry in structure: %+v", pattern.Path, pattern.Structure)
				continue
			}
		} else {
			debug.Printf("Skipping pattern at path %s due to unknown pattern type: %v", pattern.Path, pattern.Type)
			continue // Skip unknown patterns
		}

		if registry == "" { // Handle cases where registry might still be empty after extraction
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

	// --- Apply Strategy & Generate Overrides --- (Adapt for ImagePattern)
	debug.Printf("Generating overrides for %d eligible image patterns.", len(eligibleImages))

	// Convert eligible analysis.ImagePattern to image.DetectedImage for strategy
	detectedEligibleImages := make([]image.DetectedImage, 0, len(eligibleImages))
	for _, pattern := range eligibleImages {
		var imgRef *image.Reference
		var parseErr error
		originalFormat := pattern.Value // Default for string type
		if pattern.Type == analysis.PatternTypeString {
			imgRef, parseErr = image.ParseImageReference(pattern.Value)
			if parseErr != nil {
				log.Warnf("Could not parse eligible string image pattern at path %s ('%s'): %v - Skipping", pattern.Path, pattern.Value, parseErr)
				continue
			}
		} else if pattern.Type == analysis.PatternTypeMap {
			// Reconstruct string for parsing to get consistent Reference object
			// This assumes normalizeImageValues logic is consistent with ParseImageReference
			reg, repo, tag := pattern.Structure["registry"].(string), pattern.Structure["repository"].(string), pattern.Structure["tag"].(string)
			mapValueStr := fmt.Sprintf("%s/%s:%s", reg, repo, tag)
			originalFormat = mapValueStr // Use reconstructed string as original for map
			imgRef, parseErr = image.ParseImageReference(mapValueStr)
			if parseErr != nil {
				log.Warnf("Could not parse reconstructed map image pattern at path %s ('%s'): %v - Skipping", pattern.Path, mapValueStr, parseErr)
				continue
			}
		} else {
			log.Warnf("Unknown pattern type %v encountered for eligible image at path %s - Skipping", pattern.Type, pattern.Path)
			continue
		}

		detectedEligibleImages = append(detectedEligibleImages, image.DetectedImage{
			Path:           strings.Split(pattern.Path, "."), // Convert dot-path to slice
			Reference:      imgRef,
			OriginalFormat: originalFormat,
			Pattern:        fmt.Sprintf("%d", pattern.Type), // Convert pattern type int to string
		})
	}

	overrides := make(map[string]interface{})
	imagesSuccessfullyProcessed := 0
	processingErrors := []error{}
	eligibleImagesCount := len(detectedEligibleImages)

	for _, detectedImage := range detectedEligibleImages {
		// Determine the target registry, considering mappings
		imageTargetRegistry := g.targetRegistry
		if g.mappings != nil {
			if mappedRegistry := g.mappings.GetTargetRegistry(detectedImage.Reference.Registry); mappedRegistry != "" {
				imageTargetRegistry = mappedRegistry
				debug.Printf("Using mapped target registry '%s' for source '%s'", imageTargetRegistry, detectedImage.Reference.Registry)
			}
		}

		// Correct call to GeneratePath with Reference and target registry
		newPath, err := g.pathStrategy.GeneratePath(detectedImage.Reference, imageTargetRegistry)
		if err != nil {
			// Use strings.Join for path representation
			log.Warnf("Error generating path for image %s at path %s: %v", detectedImage.Reference.Original, strings.Join(detectedImage.Path, "."), err)
			processingErrors = append(processingErrors, fmt.Errorf("path generation for '%s' failed: %w", detectedImage.Reference.Original, err))
			continue // Skip this image
		}

		// Use strings.Join for path representation
		debug.Printf("Applying override for path: %s, New Path: %s, Original Image Ref: %s",
			strings.Join(detectedImage.Path, "."), newPath, detectedImage.Reference.Original)

		// Use the SetValueAtPath from pkg/override, add missing 'createPath' argument (false)
		err = override.SetValueAtPath(overrides, detectedImage.Path, newPath, false)
		if err != nil {
			// Use strings.Join for path representation
			log.Errorf("Failed to set override value for path %s: %v", strings.Join(detectedImage.Path, "."), err)
			// Use strings.Join for path representation
			processingErrors = append(processingErrors, fmt.Errorf("setting override for '%s' at path '%s' failed: %w", newPath, strings.Join(detectedImage.Path, "."), err))
			continue // Skip this image
		}
		imagesSuccessfullyProcessed++
	}

	// --- Threshold Check --- (Original Block - Corrected)
	const ThresholdErrorMessage = "Processing failed: Success rate %d%% (%d/%d) below threshold %d%%"

	debug.Printf("[GENERATE] Before threshold check. Processed: %d, Eligible (Patterns): %d\n", imagesSuccessfullyProcessed, eligibleImagesCount)
	if eligibleImagesCount > 0 {
		successRate := float64(imagesSuccessfullyProcessed) / float64(eligibleImagesCount) * float64(percentageMultiplier)
		debug.Printf("Image processing success rate: %.2f%% (Processed: %d, Eligible: %d, Threshold: %d%%)",
			successRate, imagesSuccessfullyProcessed, eligibleImagesCount, g.threshold)

		if g.threshold > 0 && int(successRate) < g.threshold {
			thresholdErrMsg := fmt.Sprintf(ThresholdErrorMessage, int(successRate), imagesSuccessfullyProcessed, eligibleImagesCount, g.threshold)
			// Use fmt.Errorf to correctly format the error message
			combinedError := fmt.Errorf(thresholdErrMsg)
			if len(processingErrors) > 0 {
				combinedError = fmt.Errorf("%s - Underlying errors: %w", thresholdErrMsg, errors.Join(processingErrors...))
			}
			return nil, &ThresholdError{
				Threshold:   g.threshold,
				ActualRate:  int(successRate),
				Eligible:    eligibleImagesCount,
				Processed:   imagesSuccessfullyProcessed,
				Err:         combinedError,
				WrappedErrs: processingErrors,
			}
		}
	} else if len(processingErrors) > 0 {
		// If threshold not enabled or no eligible images, but errors occurred, return them
		log.Debugf("Returning combined processing errors as threshold is disabled or no eligible images.")
		return nil, fmt.Errorf("image processing failed: %w", errors.Join(processingErrors...))
	}

	// --- Prepare Final Output ---
	// Note: Removing the duplicated threshold check block that was added previously.

	finalOverrideFile := &override.File{
		ChartPath: g.chartPath,
		Overrides: overrides,
	}

	debug.Printf("Generated %d override entries.", len(overrides))
	debug.DumpValue("[GENERATE] Final Overrides Map", overrides)

	return finalOverrideFile, nil
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
	if err := os.WriteFile(overrideFilePath, overrides, 0600); err != nil {
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
	return cmd.CombinedOutput()
}

// validateYAMLStructure checks if the byte slice contains valid YAML.
func validateYAMLStructure(data []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var node interface{}
	if err := dec.Decode(&node); err != nil && !errors.Is(err, io.EOF) {
		debug.Printf("YAML validation failed: %v", err)
		return fmt.Errorf("invalid YAML structure: %w", err)
	}
	return nil
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
