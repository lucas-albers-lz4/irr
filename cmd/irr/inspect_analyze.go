// Package main contains the implementation for the irr CLI, including subcommands like inspect.
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli/values"

	"github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
)

// setupAnalyzerAndLoadChart prepares the analyzer config and loads the chart for standalone mode.
// Uses the context-aware chart loading to properly handle subcharts.
func setupAnalyzerAndLoadChart(cmd *cobra.Command, flags *InspectFlags) (string, *ImageAnalysis, error) {
	chartPath := flags.ChartPath
	var relativePath string // Declare relativePath variable

	// Detect chart path if not provided
	if chartPath == "" {
		var detectErr error
		chartPath, relativePath, detectErr = detectChartIfNeeded(AppFs, ".") // Assuming start from "."
		if detectErr != nil {
			return "", nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartLoadFailed,
				Err:  fmt.Errorf("failed to find chart: %w", detectErr),
			}
		}
		log.Info("Detected chart path", "absolute", chartPath, "relative", relativePath)
	} else {
		// Validate provided chart path using AppFs
		absChartPath := chartPath
		exists, err := afero.Exists(AppFs, absChartPath)
		if err != nil {
			return "", nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartLoadFailed,
				Err:  fmt.Errorf("failed to check chart path %q: %w", absChartPath, err),
			}
		}
		if !exists {
			return "", nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("chart path not found or inaccessible: %s", absChartPath),
			}
		}
		chartPath = absChartPath
	}

	// Create value options from flags
	valueOpts := &values.Options{}

	// Get values files
	valuesFiles, err := cmd.Flags().GetStringSlice("values")
	if err != nil {
		return "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get values files: %w", err),
		}
	}
	valueOpts.ValueFiles = valuesFiles

	// Get set values
	setValues, err := cmd.Flags().GetStringSlice("set")
	if err == nil && len(setValues) > 0 {
		valueOpts.Values = setValues
	}

	// Get set-string values
	setStringValues, err := cmd.Flags().GetStringSlice("set-string")
	if err == nil && len(setStringValues) > 0 {
		valueOpts.StringValues = setStringValues
	}

	// Get set-file values
	setFileValues, err := cmd.Flags().GetStringSlice("set-file")
	if err == nil && len(setFileValues) > 0 {
		valueOpts.FileValues = setFileValues
	}

	// Create chart loader options
	loaderOptions := &helm.ChartLoaderOptions{
		ChartPath:  chartPath,
		ValuesOpts: *valueOpts,
	}

	// Create chart loader
	chartLoader := helm.NewChartLoader()

	// Load chart and track origins - this properly handles subcharts and dependencies
	chartAnalysisContext, err := chartLoader.LoadChartAndTrackOrigins(loaderOptions)
	if err != nil {
		return "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed,
			Err:  fmt.Errorf("failed to load chart with values: %w", err),
		}
	}
	// Add nil checks
	if chartAnalysisContext == nil {
		return "", nil, errors.New("internal error: LoadChartAndTrackOrigins returned nil context without error")
	}
	if chartAnalysisContext.Chart == nil {
		// Perhaps the path didn't actually contain a chart?
		// Need to determine the correct chartPath variable here, it might not be set yet.
		// Using loaderOptions.ChartPath as the input path.
		return "", nil, fmt.Errorf("failed to load chart details from context for path: %s", loaderOptions.ChartPath)
	}
	if chartAnalysisContext.Chart.Metadata == nil {
		// This indicates a chart was loaded but lacks required metadata
		// Use Name() if available, else fallback to ChartPath()
		chartIdentifier := chartAnalysisContext.Chart.ChartPath()
		if chartAnalysisContext.Chart.Name() != "" {
			chartIdentifier = chartAnalysisContext.Chart.Name()
		}
		return "", nil, fmt.Errorf("loaded chart %s lacks metadata", chartIdentifier)
	}

	// Create context-aware analyzer
	contextAnalyzer := helm.NewContextAwareAnalyzer(chartAnalysisContext)

	// Run analysis
	chartAnalysisResult, err := contextAnalyzer.AnalyzeContext()
	if err != nil {
		return "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("chart analysis failed: %w", err),
		}
	}

	// Process image patterns using the original analysis patterns
	images, skipped := processImagePatterns(chartAnalysisResult.ImagePatterns)

	// Create image analysis for the CLI output, using the original patterns
	analysisResult := &ImageAnalysis{
		Chart: ChartInfo{
			Name:         chartAnalysisContext.Chart.Metadata.Name,
			Version:      chartAnalysisContext.Chart.Metadata.Version,
			Path:         chartAnalysisContext.Chart.ChartPath(),
			Dependencies: len(chartAnalysisContext.Chart.Dependencies()),
		},
		Images:        images,
		ImagePatterns: chartAnalysisResult.ImagePatterns, // Use original patterns
		Skipped:       skipped,
	}

	return chartPath, analysisResult, nil
}

// inspectHelmRelease handles inspection when a release name is provided (plugin mode)
func inspectHelmRelease(cmd *cobra.Command, flags *InspectFlags, releaseName, namespace string) error {
	log.Debug("Running inspect in Helm plugin mode for release", "release", releaseName, "namespace", namespace)

	helmAdapter, err := helmAdapterFactory() // Get adapter (potentially mocked)
	if err != nil {
		return err // Assumes factory returns ExitCodeError on failure
	}
	// Add explicit nil check for helmAdapter to satisfy nilaway and prevent potential panics
	if helmAdapter == nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  errors.New("internal error: helmAdapterFactory returned nil adapter without error"),
		}
	}

	// Get release values
	log.Debug("Getting values for release", "release", releaseName)
	releaseValues, err := helmAdapter.GetReleaseValues(context.Background(), releaseName, namespace)
	if err != nil {
		return &exitcodes.ExitCodeError{ // Wrap error if needed
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to get values for release %s: %w", releaseName, err),
		}
	}

	// Get chart metadata from release (use this instead of loading from potentially non-existent path)
	log.Debug("Getting chart metadata for release", releaseName)
	chartMetadata, err := helmAdapter.GetChartFromRelease(context.Background(), releaseName, namespace)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to get chart info for release %s: %w", releaseName, err),
		}
	}

	// --- Analyze Release Values Directly ---
	// Instead of loading the chart from a path (which might not exist),
	// analyze the values obtained directly from the Helm release.

	// Create a simplified ChartInfo based on available metadata
	chartInfo := ChartInfo{
		Name:    chartMetadata.Name,
		Version: chartMetadata.Version,
		Path:    fmt.Sprintf("helm-release://%s/%s", namespace, releaseName), // Indicate source
		// Dependencies count might not be available without loading the chart files
	}

	// Analyze the release values using the provided analyzer config
	log.Debug("Analyzing release values...")
	analysisPatterns, analysisErr := analysis.AnalyzeHelmValues(releaseValues, flags.AnalyzerConfig)
	if analysisErr != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("release values analysis failed: %w", analysisErr),
		}
	}
	images, skipped := processImagePatterns(analysisPatterns)
	analysisResult := &ImageAnalysis{
		Chart:         chartInfo,
		Images:        images,
		ImagePatterns: analysisPatterns,
		Skipped:       skipped,
	}

	// Apply source registry filtering if needed
	if len(flags.SourceRegistries) > 0 {
		var filteredImages []ImageInfo

		// Create a map for O(1) lookups
		registryMap := make(map[string]bool)
		for _, reg := range flags.SourceRegistries {
			normalized := image.NormalizeRegistry(reg)
			registryMap[normalized] = true
		}

		// Filter images
		for _, img := range analysisResult.Images {
			if registryMap[img.Registry] {
				filteredImages = append(filteredImages, img)
			}
		}

		// Update the analysis with filtered images
		analysisResult.Images = filteredImages
		log.Info("Filtered images to", len(flags.SourceRegistries), "registries")
	}

	// Write output
	return writeOutput(cmd, analysisResult, flags)
}

// analyzeRelease analyzes a single Helm release and returns the analysis result and the original unfiltered images
func analyzeRelease(release *helm.ReleaseElement, helmAdapter *helm.Adapter, flags *InspectFlags) (*ReleaseAnalysisResult, []ImageInfo, error) {
	log.Info("Analyzing release", "name", release.Name, "namespace", release.Namespace)

	// Get release values
	releaseValues, err := helmAdapter.GetReleaseValues(context.Background(), release.Name, release.Namespace)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get values for release %s/%s: %w", release.Namespace, release.Name, err)
	}

	// Get chart metadata
	chartMetadata, err := helmAdapter.GetChartFromRelease(context.Background(), release.Name, release.Namespace)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get chart info for release %s/%s: %w", release.Namespace, release.Name, err)
	}

	// Create chart info from metadata
	chartInfo := ChartInfo{
		Name:    chartMetadata.Name,
		Version: chartMetadata.Version,
		Path:    fmt.Sprintf("helm-release://%s/%s", release.Namespace, release.Name),
		// Dependencies might be missing when inspecting a release directly
	}

	// --- Use Context-Aware Analyzer for Release Inspection ---
	log.Debug("Analyzing release values using ContextAwareAnalyzer", "name", release.Name, "namespace", release.Namespace)

	// Create a minimal ChartAnalysisContext based on release data
	// NOTE: Chart dependencies and origins are NOT available when inspecting a live release this way.
	// The context-aware analyzer might behave differently without full dependency info.
	// We assume the releaseValues are the fully rendered values.
	// A dummy chart object is created to satisfy the analyzer's needs.
	dummyChart := &chart.Chart{ // Use the imported chart.Chart type
		Metadata: &chart.Metadata{
			Name:    chartMetadata.Name,
			Version: chartMetadata.Version,
		},
		// Dependencies and other fields will be empty/nil
	}
	analysisContext := &helm.ChartAnalysisContext{
		Chart:   dummyChart,
		Values:  releaseValues,
		Origins: map[string]helm.ValueOrigin{}, // Initialize as map of VALUES, not pointers
	}

	contextAnalyzer := helm.NewContextAwareAnalyzer(analysisContext)
	chartAnalysisResult, analysisErr := contextAnalyzer.AnalyzeContext()
	if analysisErr != nil {
		// Use the context-aware analyzer's result
		return nil, nil, fmt.Errorf("context-aware analysis failed for release %s/%s: %w", release.Namespace, release.Name, analysisErr)
	}

	// Process the patterns from the context-aware analyzer
	images, skipped := processImagePatterns(chartAnalysisResult.ImagePatterns) // Use patterns directly

	// Create analysis result structure
	analysisResult := ImageAnalysis{
		Chart:         chartInfo,
		Images:        images,
		ImagePatterns: chartAnalysisResult.ImagePatterns, // Use patterns directly from context-aware analyzer
		Skipped:       skipped,
	}

	// --- Filtering Logic ---
	// Keep a copy of original images for skeleton generation, even if filtered for output
	unfilteredImagesForSkeleton := make([]ImageInfo, len(images))
	copy(unfilteredImagesForSkeleton, images)

	// Apply source registry filtering if needed FOR THE OUTPUT ANALYSIS
	if len(flags.SourceRegistries) > 0 {
		// Create a map for O(1) lookups
		registryMap := make(map[string]bool)
		for _, reg := range flags.SourceRegistries {
			normalized := image.NormalizeRegistry(reg)
			registryMap[normalized] = true
		}

		// Filter images for the output
		filteredImagesForOutput := make([]ImageInfo, 0)
		for _, img := range images { // Iterate original images
			normalizedRegistry := image.NormalizeRegistry(img.Registry)
			if registryMap[normalizedRegistry] {
				filteredImagesForOutput = append(filteredImagesForOutput, img)
			}
		}

		// Update the analysis.Images field ONLY for the output result
		analysisResult.Images = filteredImagesForOutput
	}

	// Return the potentially filtered analysis result AND the original unfiltered images
	return &ReleaseAnalysisResult{
		ReleaseName: release.Name,
		Namespace:   release.Namespace,
		Analysis:    analysisResult,
	}, unfilteredImagesForSkeleton, nil // Return unfiltered images here
}
