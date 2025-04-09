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

// Generator implements chart analysis and override generation.
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

// Generate performs the chart analysis and generates overrides.
// The function returns appropriate exit codes through pkg/exitcodes.ExitCodeError:
// - ExitChartParsingError (10) for chart loading/parsing failures
// - ExitImageProcessingError (11) for image reference processing issues
// - ExitUnsupportedStructure (12) when strict mode validation fails
// - ExitThresholdError (13) when success rate is below threshold
// - ExitGeneralRuntimeError (20) for system/runtime errors
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
	debug.DumpValue("[GENERATE] Detected Image Patterns", detectedImages)

	// Find unsupported patterns (e.g., templates)
	unsupportedPatterns := []override.UnsupportedStructure{}
	for _, pattern := range detectedImages {
		if strings.Contains(pattern.Value, "{{") || strings.Contains(pattern.Value, "}}") {
			unsupportedPatterns = append(unsupportedPatterns, override.UnsupportedStructure{
				Path: strings.Split(pattern.Path, "."),
				Type: "template",
			})
		}
	}

	// Strict Mode Check
	if g.strict && len(unsupportedPatterns) > 0 {
		details := []string{}
		log.Warnf("Strict mode enabled: Found %d unsupported image structures:", len(unsupportedPatterns))
		for i, item := range unsupportedPatterns {
			errMsg := fmt.Sprintf("  [%d] Path: %s, Type: %s", i+1, strings.Join(item.Path, "."), item.Type)
			log.Warnf(errMsg)
			details = append(details, errMsg)
		}
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedStructure, strings.Join(details, "; "))
	}

	// Filter and process detected images
	eligibleImages := []analysis.ImagePattern{}
	for _, pattern := range detectedImages {
		var registry string
		var imgRef *image.Reference

		if pattern.Type == analysis.PatternTypeString {
			imgRef, err = image.ParseImageReference(pattern.Value)
			if err != nil {
				debug.Printf("Skipping pattern at path %s due to parse error on value '%s': %v", pattern.Path, pattern.Value, err)
				continue
			}
			registry = imgRef.Registry
		} else if pattern.Type == analysis.PatternTypeMap {
			if regVal, ok := pattern.Structure["registry"].(string); ok {
				registry = regVal
			} else {
				debug.Printf("Skipping map pattern at path %s due to missing registry in structure: %+v", pattern.Path, pattern.Structure)
				continue
			}
		} else {
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

	// Generate overrides
	overrides := make(map[string]interface{})
	var processErrors []error
	eligibleCount := len(eligibleImages)
	processedCount := 0

	for _, pattern := range eligibleImages {
		var imgRef *image.Reference
		var err error

		if pattern.Type == analysis.PatternTypeString {
			imgRef, err = image.ParseImageReference(pattern.Value)
			if err != nil {
				processErrors = append(processErrors, fmt.Errorf("failed to parse image reference '%s': %w", pattern.Value, err))
				continue
			}
		} else {
			// For map type, construct a reference from the structure
			imgRef = &image.Reference{
				Registry:   pattern.Structure["registry"].(string),
				Repository: pattern.Structure["repository"].(string),
				Tag:        pattern.Structure["tag"].(string),
			}
		}

		// Determine target registry
		targetReg := g.targetRegistry
		if g.mappings != nil {
			if mappedTarget := g.mappings.GetTargetRegistry(imgRef.Registry); mappedTarget != "" {
				debug.Printf("Found registry mapping: %s -> %s", imgRef.Registry, mappedTarget)
				targetReg = mappedTarget
			}
		}

		// Generate path using strategy
		newPath, err := g.pathStrategy.GeneratePath(imgRef, targetReg)
		if err != nil {
			processErrors = append(processErrors, fmt.Errorf("path generation failed for '%s': %w", imgRef.String(), err))
			continue
		}

		processedCount++

		// Create override based on pattern type
		var override interface{}
		if pattern.Type == analysis.PatternTypeString {
			// For string type, generate a full image reference string
			override = fmt.Sprintf("%s/%s", targetReg, newPath)
			if imgRef.Tag != "" {
				override = fmt.Sprintf("%s:%s", override, imgRef.Tag)
			}
		} else {
			// For map type, update the map structure
			override = map[string]interface{}{
				"registry":   targetReg,
				"repository": newPath,
			}
			if imgRef.Tag != "" {
				override.(map[string]interface{})["tag"] = imgRef.Tag
			}
		}

		// Set the override at the correct path
		pathParts := strings.Split(pattern.Path, ".")
		current := overrides
		for i := 0; i < len(pathParts)-1; i++ {
			part := pathParts[i]
			if _, exists := current[part]; !exists {
				current[part] = make(map[string]interface{})
			}
			current = current[part].(map[string]interface{})
		}
		current[pathParts[len(pathParts)-1]] = override
	}

	// Check threshold
	if eligibleCount > 0 {
		successRate := (processedCount * 100) / eligibleCount
		if successRate < g.threshold {
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

func (g *Generator) processMapPattern(pattern analysis.ImagePattern) (*image.Reference, error) {
	// Extract and validate required fields
	registry, ok := pattern.Structure["registry"].(string)
	if !ok {
		return nil, fmt.Errorf("registry field is not a string")
	}
	repository, ok := pattern.Structure["repository"].(string)
	if !ok {
		return nil, fmt.Errorf("repository field is not a string")
	}
	tag, ok := pattern.Structure["tag"].(string)
	if !ok {
		return nil, fmt.Errorf("tag field is not a string")
	}

	// Create reference
	ref := &image.Reference{
		Registry:   registry,
		Repository: repository,
		Tag:        tag,
	}

	return ref, nil
}

func (g *Generator) processStringPattern(pattern analysis.ImagePattern) (*image.Reference, error) {
	// Parse string pattern
	ref, err := image.ParseImageReference(pattern.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse string pattern: %w", err)
	}
	return ref, nil
}

func (g *Generator) processPattern(pattern analysis.ImagePattern) (map[string]interface{}, error) {
	var ref *image.Reference
	var err error

	// Process based on pattern type
	if pattern.Type == analysis.PatternTypeString {
		ref, err = g.processStringPattern(pattern)
	} else if pattern.Type == analysis.PatternTypeMap {
		ref, err = g.processMapPattern(pattern)
	} else {
		return nil, fmt.Errorf("unsupported pattern type: %s", pattern.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to process pattern: %w", err)
	}

	// Generate target path
	targetPath, err := g.pathStrategy.GeneratePath(ref, g.targetRegistry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate target path: %w", err)
	}

	// Create override map
	override := map[string]interface{}{
		"path":     pattern.Path,
		"original": pattern.Value,
		"type":     pattern.Type,
		"target": map[string]interface{}{
			"registry":   g.targetRegistry,
			"repository": targetPath,
			"tag":        ref.Tag,
		},
	}

	return override, nil
}

func (g *Generator) updateOverrideMap(current map[string]interface{}, path []string, value interface{}) error {
	if len(path) == 0 {
		return fmt.Errorf("empty path")
	}

	for i := 0; i < len(path)-1; i++ {
		next, ok := current[path[i]]
		if !ok {
			// Create new map if path doesn't exist
			next = make(map[string]interface{})
			current[path[i]] = next
		}

		// Type assert and move to next level
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("path component %s is not a map", path[i])
		}
		current = nextMap
	}

	// Set the final value
	current[path[len(path)-1]] = value
	return nil
}
