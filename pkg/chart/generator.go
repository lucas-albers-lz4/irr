package chart

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"helm.sh/helm/v3/pkg/chart"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/log"
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
	// MaxSplitParts defines the maximum number of parts to split registry paths into
	// Currently 2 parts: registry name and repository path
	MaxSplitParts = 2
)

const theAliasImagePath = "theAlias.image"

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
	chartLoader Loader, // Keep loader for potential validation use
	rulesEnabled bool,
) *Generator {
	// Set up a default chart loader if none was provided
	if chartLoader == nil {
		chartLoader = NewLoader()
	}

	// Add debug logging for mappings
	if mappings != nil {
		log.Debug("Generator initialized with mappings",
			"entries_count", len(mappings.Entries),
			"entries", fmt.Sprintf("%+v", mappings.Entries))
	} else {
		log.Debug("Generator initialized with nil mappings")
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
		loader:            chartLoader,
		rulesEnabled:      rulesEnabled,
		rulesRegistry:     rules.NewRegistry(), // Initialize the rules registry
	}
}

// ProcessedImageDetail struct definition
type ProcessedImageDetail struct {
	Path                string
	OriginalImage       string
	FinalTargetRegistry string // The actual registry part used for this image after mappings/strategy
	FinalRepositoryPath string // The actual repository path used
}

// Generate produces the override values map based on detected images and strategy.
func (g *Generator) Generate(loadedChart *chart.Chart, analysisResult *analysis.ChartAnalysis) (*override.File, error) {
	log.Debug("Generate called", "hasLoadedChart", loadedChart != nil, "hasAnalysisResult", analysisResult != nil)
	if analysisResult == nil || loadedChart == nil {
		return nil, fmt.Errorf("cannot generate overrides without analysis results (analysisResult is nil)")
	}

	actualOverrides := make(map[string]interface{}) // This will populate resultFile.Values
	var processingErrors []error
	var unsupportedStructures []override.UnsupportedStructure // Collect these if strict mode is off but found
	processedCount := 0

	eligibleImages := g.filterEligibleImages(analysisResult.ImagePatterns)
	log.Info("Filtering complete", "total_images", len(analysisResult.ImagePatterns), "eligible_images", len(eligibleImages))

	if g.strict {
		strictUnsupported := g.findUnsupportedPatterns(analysisResult.ImagePatterns)
		if len(strictUnsupported) > 0 {
			errMsg := "strict mode violation: unsupported structures found:\n"
			for _, us := range strictUnsupported {
				errMsg += fmt.Sprintf("  - Path: %s, Type: %s\n", strings.Join(us.Path, "."), us.Type)
			}
			log.Error(errMsg)
			// Always return an empty slice, not nil
			return &override.File{Unsupported: append([]override.UnsupportedStructure{}, strictUnsupported...), ChartPath: g.chartPath, ChartName: loadedChart.Name()}, fmt.Errorf("%s", errMsg)
		}
	} else {
		unsupportedStructures = g.findUnsupportedPatterns(analysisResult.ImagePatterns)
		if len(unsupportedStructures) > 0 {
			log.Warn("Unsupported structures found (strict mode is off)", "count", len(unsupportedStructures))
		}
	}

	var processedDetails []ProcessedImageDetail

	for i := range eligibleImages {
		pattern := &eligibleImages[i]
		log.Debug("Eligible image for processing", "index", i, "path", pattern.Path, "value", pattern.Value, "sourceOrigin", pattern.SourceOrigin)

		imgRef, err := g.processImagePattern(pattern)
		if err != nil {
			log.Warn("Failed to parse image reference during override generation", "path", pattern.Path, "value", pattern.Value, "error", err)
			processingErrors = append(processingErrors, fmt.Errorf("path %s: %w", pattern.Path, err))
			continue
		}
		if imgRef == nil {
			log.Warn("Nil image reference after parsing, skipping", "path", pattern.Path)
			processingErrors = append(processingErrors, fmt.Errorf("path %s: nil image reference", pattern.Path))
			continue
		}

		targetActualRegistry, newPath, err := g.determineTargetPathAndRegistry(imgRef, pattern)
		if err != nil {
			log.Warn("Failed to determine target path and registry", "path", pattern.Path, "image", imgRef.Original, "error", err)
			// Update error message to match test expectation
			processingErrors = append(processingErrors, fmt.Errorf("error determining target path for %s: %w", pattern.Path, err))
			continue
		}
		log.Debug("Determined target for override", "path", pattern.Path, "originalImage", imgRef.Original, "targetRegistry", targetActualRegistry, "newRepositoryPath", newPath)

		overrideValue := g.createOverride(pattern, imgRef, targetActualRegistry, newPath)

		if err := g.setOverridePath(actualOverrides, pattern, overrideValue); err != nil {
			log.Error("Failed to set override path", "path", pattern.Path, "error", err)
			processingErrors = append(processingErrors, fmt.Errorf("setting override for path %s: %w", pattern.Path, err))
			continue
		}
		log.Info("Successfully processed image override",
			"path", pattern.Path,
			"original", imgRef.Original,
			"new_repo", newPath,
			"target_registry", targetActualRegistry)

		processedCount++
		processedDetails = append(processedDetails, ProcessedImageDetail{
			Path:                pattern.Path,
			OriginalImage:       imgRef.Original,
			FinalTargetRegistry: targetActualRegistry,
			FinalRepositoryPath: newPath,
		})
	}

	successRate := 0.0
	if len(eligibleImages) > 0 {
		successRate = (float64(processedCount) / float64(len(eligibleImages))) * PercentageMultiplier
	} else if len(analysisResult.ImagePatterns) == 0 {
		successRate = PercentageMultiplier
	}

	log.Info("Image processing complete", "processed", processedCount, "eligible", len(eligibleImages), "success_rate", fmt.Sprintf("%.2f%%", successRate))

	// Always return an empty slice, not nil, for Unsupported
	resultFile := &override.File{
		Values:         actualOverrides,
		Unsupported:    append([]override.UnsupportedStructure{}, unsupportedStructures...),
		SuccessRate:    successRate, // This is float64
		TotalCount:     len(analysisResult.ImagePatterns),
		ProcessedCount: processedCount,
		ChartPath:      g.chartPath,
		ChartName:      loadedChart.Name(),
	}

	if processedCount > 0 {
		g.ensureGlobalImageRegistry(resultFile.Values, analysisResult.GlobalPatterns, processedDetails)
	} else {
		log.Debug("No images processed, skipping ensureGlobalImageRegistry")
		// If no images processed, but global patterns exist, they might still be added by ensureGlobalImageRegistry if logic changes
		// For now, it relies on processedDetails. If global.imageRegistry should be set even with 0 processed images based on CLI, this needs adjustment.
	}

	if len(eligibleImages) == 0 && len(g.sourceRegistries) > 0 && processedCount == 0 {
		log.Warn("No images found from the specified source registries that require an override.")
	}

	if err := g.checkProcessingThreshold(processingErrors, processedCount, len(eligibleImages), successRate, resultFile); err != nil {
		return resultFile, err
	}

	if g.rulesEnabled {
		if err := g.applyRulesIfNeeded(loadedChart, resultFile); err != nil {
			log.Error("Error applying rules", "error", err)
		}
	}

	log.Debug("Generator.Generate: Final override map keys before return", "keys", mapKeys(resultFile.Values), "map_addr", fmt.Sprintf("%p", resultFile.Values))
	// Compare log.CurrentLevel() (which returns slog.Level from the custom package, which is an alias for std slog.Level)
	// with the standard slog.LevelDebug constant.
	if log.CurrentLevel() <= slog.LevelDebug {
		log.Debug("Generator.Generate: Full overrides map structure BEFORE returning", "structure", resultFile.Values)
	}

	// Combine processing errors if any, to return with potentially partial result
	if len(processingErrors) > 0 {
		return resultFile, &ProcessingError{Errors: processingErrors, Count: len(processingErrors)}
	}

	return resultFile, nil
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
