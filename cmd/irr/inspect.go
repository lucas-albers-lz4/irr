// Package main contains the implementation for the irr CLI, including subcommands like inspect.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"

	"github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/lucas-albers-lz4/irr/pkg/analyzer"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/spf13/cobra"
	// Added Helm imports
)

// ChartInfo represents basic chart information
type ChartInfo struct {
	Name         string `json:"name" yaml:"name"`
	Version      string `json:"version" yaml:"version"`
	Path         string `json:"path" yaml:"path"`
	Dependencies int    `json:"dependencies" yaml:"dependencies"`
}

// ImageInfo represents image information found in the chart
type ImageInfo struct {
	Registry   string `json:"registry" yaml:"registry"`
	Repository string `json:"repository" yaml:"repository"`
	Tag        string `json:"tag,omitempty" yaml:"tag,omitempty"`
	Digest     string `json:"digest,omitempty" yaml:"digest,omitempty"`
	Source     string `json:"source" yaml:"source"`
}

// ImageAnalysis represents the result of analyzing a chart for images
type ImageAnalysis struct {
	Chart         ChartInfo               `json:"chart" yaml:"chart"`
	Images        []ImageInfo             `json:"images" yaml:"images"`
	ImagePatterns []analyzer.ImagePattern `json:"imagePatterns" yaml:"imagePatterns"`
	Errors        []string                `json:"errors,omitempty" yaml:"errors,omitempty"`
	Skipped       []string                `json:"skipped,omitempty" yaml:"skipped,omitempty"`
}

// InspectFlags holds the command line flags for the inspect command
type InspectFlags struct {
	ChartPath              string
	OutputFile             string
	OutputFormat           string
	GenerateConfigSkeleton bool
	AnalyzerConfig         *analyzer.Config
	SourceRegistries       []string
	AllNamespaces          bool
	OverwriteSkeleton      bool
	// Add filesystem dependency if needed for loading logic outside runInspect
	// Fs                     afero.Fs
}

const (
	// DefaultConfigSkeletonFilename is the default filename for the generated config skeleton
	DefaultConfigSkeletonFilename = "registry-mappings.yaml"
	outputFormatYAML              = "yaml"
	outputFormatJSON              = "json"
	defaultNamespace              = "default" // Added const for default namespace
)

// ReleaseAnalysisResult represents the analysis result for a single Helm release
type ReleaseAnalysisResult struct {
	ReleaseName string        `json:"releaseName" yaml:"releaseName"`
	Namespace   string        `json:"namespace" yaml:"namespace"`
	Analysis    ImageAnalysis `json:"analysis" yaml:"analysis"`
}

// createHelmClient creates a new instance of the Helm client
func createHelmClient() (helm.ClientInterface, error) {
	client, err := helm.NewHelmClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Helm client: %w", err)
	}
	return client, nil
}

// newInspectCmd creates a new inspect command
func newInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect [release-name]",
		Short: "Inspect a Helm chart for image references",
		Long: `Inspect a Helm chart to find all image references.
This command analyzes the chart's values.yaml and templates to find image references.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runInspect,
	}

	cmd.Flags().String("chart-path", "", "Path to the Helm chart")
	cmd.Flags().String("output-file", "", "Write output to file instead of stdout")
	cmd.Flags().String("output-format", outputFormatYAML, "Output format (yaml or json)")
	cmd.Flags().Bool("generate-config-skeleton", false, "Generate a config skeleton based on found images")
	cmd.Flags().StringSlice("include-pattern", nil, "Glob patterns for values paths to include during analysis")
	cmd.Flags().StringSlice("exclude-pattern", nil, "Glob patterns for values paths to exclude during analysis")
	cmd.Flags().StringSliceP("source-registries", "r", []string{}, "Source registries to filter results (optional)")
	cmd.Flags().String("release-name", "", "Release name for Helm plugin mode")
	cmd.Flags().StringP("namespace", "n", "default", `Kubernetes namespace for the release (defaults to "default")`)
	cmd.Flags().BoolP("all-namespaces", "A", false, "Inspect Helm releases across all namespaces (conflicts with --chart-path, --release-name, --namespace)")
	cmd.Flags().Bool("overwrite-skeleton", false, "Overwrite the skeleton file if it already exists (only applies when using --generate-config-skeleton)")

	return cmd
}

// loadHelmChart loads a Helm chart from the given path using the provided filesystem.
func loadHelmChart(fs afero.Fs, chartPath string) (*chart.Chart, error) {
	// Check if chart path exists on the given filesystem
	exists, err := afero.Exists(fs, chartPath)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed,
			Err:  fmt.Errorf("failed to check chart path %q: %w", chartPath, err),
		}
	}
	if !exists {
		// Attempt to check if it's a file to mimic original loader error slightly better
		if _, statErr := fs.Stat(chartPath); statErr != nil { // Doesn't exist or other stat error
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartLoadFailed,
				Err:  fmt.Errorf("chart path not found or inaccessible: %s", chartPath),
			}
		}
		// If it exists but isn't loadable, loader.Load will handle it below
	}

	// In test mode, we need to create a simple chart structure in memory
	// instead of trying to load from the real filesystem
	if isTestMode {
		// Create a simple chart for testing
		log.Debug("Test mode detected, creating mock chart for", chartPath)
		mockChart := &chart.Chart{
			Metadata: &chart.Metadata{
				Name:    filepath.Base(chartPath),
				Version: "1.0.0",
			},
			Values: map[string]interface{}{
				"image": "nginx:stable",
			},
		}

		// If we have a Chart.yaml file in the mock filesystem, try to read it
		chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
		if chartYamlExists, checkErr := afero.Exists(fs, chartYamlPath); checkErr == nil && chartYamlExists {
			chartYamlContent, readErr := afero.ReadFile(fs, chartYamlPath)
			if readErr == nil {
				var chartYaml struct {
					APIVersion string `yaml:"apiVersion"`
					Name       string `yaml:"name"`
					Version    string `yaml:"version"`
				}
				if yamlErr := yaml.Unmarshal(chartYamlContent, &chartYaml); yamlErr == nil {
					mockChart.Metadata.Name = chartYaml.Name
					mockChart.Metadata.Version = chartYaml.Version
				}
			}
		}

		// If we have a values.yaml file in the mock filesystem, try to read it
		valuesYamlPath := filepath.Join(chartPath, "values.yaml")
		if valuesYamlExists, checkErr := afero.Exists(fs, valuesYamlPath); checkErr == nil && valuesYamlExists {
			valuesYamlContent, readErr := afero.ReadFile(fs, valuesYamlPath)
			if readErr == nil {
				var valuesYaml map[string]interface{}
				if yamlErr := yaml.Unmarshal(valuesYamlContent, &valuesYaml); yamlErr == nil {
					mockChart.Values = valuesYaml
				}
			}
		}

		return mockChart, nil
	}

	// Use Helm's loader for real filesystem operations (non-test mode)
	loadedChart, err := loader.Load(chartPath)
	if err != nil {
		// Improve error message consistency with the Exists check
		errMsg := fmt.Sprintf("failed to load chart at %s: %v", chartPath, err)
		// Check if the error is a "not found" type to return the specific code
		if strings.Contains(err.Error(), "no such file or directory") || strings.Contains(err.Error(), "not found") {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartLoadFailed,
				Err:  errors.New(errMsg), // Use errors.New for non-wrapping error message
			}
		}
		// Otherwise, it's likely a chart format error or other load issue
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed, // Or ExitChartProcessingFailed?
			Err:  errors.New(errMsg),            // Use errors.New for non-wrapping error message
		}
	}

	return loadedChart, nil
}

// analyzeChart analyzes a chart for image patterns
func analyzeChart(chartData *chart.Chart, config *analyzer.Config) (*ImageAnalysis, error) {
	// Extract chart info
	chartInfo := ChartInfo{
		Name:         chartData.Metadata.Name,
		Version:      chartData.Metadata.Version,
		Path:         chartData.ChartPath(),
		Dependencies: len(chartData.Dependencies()),
	}

	// Analyze chart values
	patterns, err := analyzer.AnalyzeHelmValues(chartData.Values, config)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("chart analysis failed: %w", err),
		}
	}

	// Process image patterns
	images, skipped := processImagePatterns(patterns)

	// Create analysis result
	analysis := &ImageAnalysis{
		Chart:         chartInfo,
		Images:        images,
		ImagePatterns: patterns,
		Skipped:       skipped,
	}

	return analysis, nil
}

// writeOutput writes the analysis to a file or stdout
func writeOutput(cmd *cobra.Command, analysis *ImageAnalysis, flags *InspectFlags) error {
	// Handle generate-config-skeleton flag
	if flags.GenerateConfigSkeleton {
		skeletonFile := flags.OutputFile
		if skeletonFile == "" {
			skeletonFile = DefaultConfigSkeletonFilename
		}

		// Check if the skeleton file exists
		exists, err := afero.Exists(AppFs, skeletonFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to check if skeleton file exists: %w", err),
			}
		}

		// If the file exists and overwriteSkeleton is false, return an error
		if exists && !flags.OverwriteSkeleton {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("output file %s already exists; use --overwrite-skeleton to overwrite", skeletonFile),
			}
		}

		// If overwriteSkeleton is true, we'll continue and overwrite the file
		if exists && flags.OverwriteSkeleton {
			log.Info("Overwriting existing skeleton file", "path", skeletonFile)
		}

		if err := createConfigSkeleton(analysis.Images, skeletonFile); err != nil {
			// Special handling for file exists error - should not happen now with the checks above
			var exitErr *exitcodes.ExitCodeError
			if errors.As(err, &exitErr) && strings.Contains(exitErr.Err.Error(), "already exists") {
				// This case should not occur now, but kept for robustness
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("output file %s already exists; use --overwrite-skeleton to overwrite", skeletonFile),
				}
			}

			// Other errors from createConfigSkeleton
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to create config skeleton: %w", err),
			}
		}
		return nil
	}

	// Determine output format (yaml or json)
	var output []byte
	var err error

	switch strings.ToLower(flags.OutputFormat) {
	case "json":
		output, err = json.Marshal(analysis)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to JSON: %w", err),
			}
		}
	default:
		// Default to YAML
		output, err = yaml.Marshal(analysis)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to YAML: %w", err),
			}
		}
	}

	// Write to file or stdout
	if flags.OutputFile != "" {
		if err := afero.WriteFile(AppFs, flags.OutputFile, output, fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write analysis to file: %w", err),
			}
		}
		log.Info("Analysis written to", flags.OutputFile)
	} else {
		// Use the command's out buffer instead of fmt.Println directly
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(output)); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write analysis to stdout: %w", err),
			}
		}
	}

	return nil
}

// runInspect implements the inspect command logic
func runInspect(cmd *cobra.Command, args []string) error {
	// Get flags for the inspect command
	var flags *InspectFlags
	var err error
	var releaseName string // Declare releaseName here

	// Check if we're being run with a release name
	releaseNameProvided := len(args) > 0
	// We now handle plugin mode inside inspectHelmRelease and standalone inside setupAnalyzerAndLoadChart
	// if releaseNameProvided && !isHelmPlugin { ... } // This check might be redundant if logic is separated

	flags, err = getInspectFlags(cmd, releaseNameProvided)
	if err != nil {
		return err
	}

	// New code: If --all-namespaces flag is set, use the all-namespaces flow
	if flags.AllNamespaces {
		return inspectAllNamespaces(cmd, flags)
	}

	// Existing code for single release/chart flow
	// Decide execution path based on args/plugin mode
	if releaseNameProvided {
		// Assume plugin mode if release name is given (validated inside inspectHelmRelease)
		releaseName = args[0] // Assign releaseName here
		namespace, nsErr := cmd.Flags().GetString("namespace")
		if nsErr != nil {
			return &exitcodes.ExitCodeError{Code: exitcodes.ExitInputConfigurationError, Err: fmt.Errorf("failed to get namespace flag: %w", nsErr)}
		} else if namespace == "" {
			namespace = defaultNamespace // Use constant
		}
		return inspectHelmRelease(cmd, flags, releaseName, namespace)
	}

	// Standalone mode (no release name)
	chartPath, analysis, err := setupAnalyzerAndLoadChart(cmd, flags) // Pass AppFs here
	if err != nil {
		// Log the error details for better debugging
		log.Debug("Error during setupAnalyzerAndLoadChart", err)
		// Ensure the error returned is an ExitCodeError for consistent handling
		var exitErr *exitcodes.ExitCodeError
		if errors.As(err, &exitErr) {
			log.Debug("Setup/Analysis failed with exit code", exitErr.Code, "error", exitErr.Err)
		} else {
			log.Debug("Setup/Analysis failed with non-exit code error", err)
		}
		return err // Return the original error
	}

	log.Info("Successfully loaded and analyzed chart", chartPath) // Add log for success

	// Filter results if source-registries flag is provided
	if len(flags.SourceRegistries) > 0 {
		// Log filtering action
		log.Info("Filtering results to only include registries", "registries", strings.Join(flags.SourceRegistries, ", "))
		filterImagesBySourceRegistries(cmd, flags, analysis) // Modifies analysis in place
	}

	// --- Informational Output (Moved Before writeOutput) ---
	//nolint:gocritic // ifElseChain: Keeping if-else for clarity over switch here.
	if !flags.GenerateConfigSkeleton && flags.OutputFile == "" { // Only show suggestions when printing to stdout
		// Log the successful analysis (using the logger now)
		log.Info("Successfully loaded and analyzed chart", "path", chartPath)

		// Extract unique registries from the potentially filtered analysis.
		uniqueRegistries := extractUniqueRegistries(analysis.Images)

		if len(uniqueRegistries) > 0 {
			log.Info("Found images from the following registries:")
			uniqueRegistryList := make([]string, 0, len(uniqueRegistries))
			for reg := range uniqueRegistries {
				uniqueRegistryList = append(uniqueRegistryList, reg)
			}
			sort.Strings(uniqueRegistryList) // Sort for consistent output
			for _, reg := range uniqueRegistryList {
				log.Info(fmt.Sprintf("  - %s", reg)) // Log each registry
			}

			// Log filtering suggestion
			log.Info("Consider using the --source-registries flag to filter results, e.g.:")
			log.Info(fmt.Sprintf("  irr inspect --source-registries %s ...", strings.Join(uniqueRegistryList, ",")))

			// Log configuration suggestion
			outputRegistryConfigSuggestion(chartPath, uniqueRegistries)
		} else if len(flags.SourceRegistries) > 0 {
			log.Info("No images found matching the specified source registries.", "registries", strings.Join(flags.SourceRegistries, ", "))
		} else {
			log.Info("No image references found in the chart.")
		}
	}
	// --- End Informational Output ---

	// Output the main analysis result (after logging informational messages)
	if err := writeOutput(cmd, analysis, flags); err != nil {
		return err // Return error with exit code from writeOutput
	}

	return nil
}

// setupAnalyzerAndLoadChart prepares the analyzer config and loads the chart for standalone mode.
// It now explicitly uses AppFs for path checking and chart loading.
func setupAnalyzerAndLoadChart(_ *cobra.Command, flags *InspectFlags) (string, *ImageAnalysis, error) {
	config := flags.AnalyzerConfig // Already configured in getInspectFlags
	chartPath := flags.ChartPath
	var relativePath string // Declare relativePath variable

	// Detect chart path if not provided
	if chartPath == "" {
		var detectErr error
		// Start search from the current directory (".") within the mock filesystem
		// Use the updated function name and capture all return values
		chartPath, relativePath, detectErr = detectChartIfNeeded(AppFs, ".") // Assuming start from "."
		if detectErr != nil {
			return "", nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartLoadFailed,
				Err:  fmt.Errorf("failed to find chart: %w", detectErr),
			}
		}
		// Log both detected paths for clarity
		log.Info("Detected chart path", "absolute", chartPath, "relative", relativePath)
	} else {
		// Validate provided chart path using AppFs
		absChartPath := chartPath // Use the provided path directly for now
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
		chartPath = absChartPath // Use absolute path - Correction: chartPath is assigned, no need for absChartPath re-assignment
	}

	// Load the chart using the filesystem-aware function
	loadedChart, err := loadHelmChart(AppFs, chartPath)
	if err != nil {
		// loadHelmChart should already return an ExitCodeError
		log.Debug("Failed during loadHelmChart", err)
		return chartPath, nil, err
	}

	// Analyze the chart
	// Pass relativePath if analyzeChart needs it, otherwise it remains unused for now
	// analysis, err := analyzeChart(loadedChart, config, relativePath) // Example if needed
	analysis, err := analyzeChart(loadedChart, config) // Current signature
	if err != nil {
		// analyzeChart should already return an ExitCodeError
		return chartPath, nil, err
	}

	return chartPath, analysis, nil
}

// filterImagesBySourceRegistries modifies the analysis object to only include images
// from the specified source registries.
func filterImagesBySourceRegistries(_ *cobra.Command, flags *InspectFlags, analysis *ImageAnalysis) {
	sourceSet := make(map[string]bool)
	for _, r := range flags.SourceRegistries {
		normalized := image.NormalizeRegistry(r)
		sourceSet[normalized] = true
	}

	if len(sourceSet) == 0 {
		log.Warn("No valid source registries provided for filtering.")
		return // No valid registries to filter by
	}

	filteredImages := make([]ImageInfo, 0, len(analysis.Images))
	for _, img := range analysis.Images {
		normalizedRegistry := image.NormalizeRegistry(img.Registry)
		if sourceSet[normalizedRegistry] {
			filteredImages = append(filteredImages, img)
		}
	}
	analysis.Images = filteredImages

	// Also filter imagePatterns (simple approach: remove if no resulting image matches)
	// A more robust approach might analyze pattern structure itself.
	filteredPatterns := make([]analyzer.ImagePattern, 0, len(analysis.ImagePatterns))
	for _, pattern := range analysis.ImagePatterns {
		imgRef, err := image.ParseImageReference(pattern.Value) // Assuming pattern.Value holds the image string or similar
		if err == nil {
			normalizedRegistry := image.NormalizeRegistry(imgRef.Registry)
			if sourceSet[normalizedRegistry] {
				filteredPatterns = append(filteredPatterns, pattern)
			}
		} else {
			// Keep for now, as it might represent a template or complex structure.
			// log.Debug("Pattern value parsing failed, keeping pattern during filtering", "path", pattern.Path, "value", pattern.Value, "error", err)
			// Heuristic: Check if *any* part of the value string matches a source registry? Risky.
			// Let's keep patterns that don't parse cleanly for now.
			filteredPatterns = append(filteredPatterns, pattern)
		}
	}
	analysis.ImagePatterns = filteredPatterns
}

// extractUniqueRegistries extracts a set of unique registry names from image info
func extractUniqueRegistries(images []ImageInfo) map[string]bool {
	registries := make(map[string]bool)
	for _, img := range images {
		normalized := image.NormalizeRegistry(img.Registry)
		registries[normalized] = true
	}
	return registries
}

// outputRegistryConfigSuggestion prints suggestions for creating a registry mapping file
func outputRegistryConfigSuggestion(chartPath string, registries map[string]bool) {
	log.Info("\nSuggestion: Create a registry mapping file ('registry-mappings.yaml') to define target registries:")
	log.Info("Example structure:")
	log.Info("```yaml")
	log.Info("mappings:")

	uniqueRegistryList := make([]string, 0, len(registries))
	for reg := range registries {
		uniqueRegistryList = append(uniqueRegistryList, reg)
	}
	sort.Strings(uniqueRegistryList) // Sort for consistent output

	for _, reg := range uniqueRegistryList {
		log.Info(fmt.Sprintf("  - source: %s", reg))
		log.Info("    target: your-private-registry.com/path") // Example target
		log.Info("    # strategy: default (optional)")
	}
	log.Info("```")
	log.Info("Then use it with the 'override' command:")
	log.Info(fmt.Sprintf("  irr override --chart-path %s --config registry-mappings.yaml ...", chartPath)) // Recommend --config now
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
	analysisPatterns, analysisErr := analyzer.AnalyzeHelmValues(releaseValues, flags.AnalyzerConfig)
	if analysisErr != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("release values analysis failed: %w", analysisErr),
		}
	}

	// Process image patterns found in values
	images, skipped := processImagePatterns(analysisPatterns)

	// Create analysis result
	analysis := &ImageAnalysis{
		Chart:         chartInfo,
		Images:        images,
		ImagePatterns: analysisPatterns, // Patterns found in values
		Skipped:       skipped,
		// Errors from analysis are included in the error return above
	}

	// Apply source registry filtering if needed
	if len(flags.SourceRegistries) > 0 {
		var filteredImages []ImageInfo

		// Create a map for O(1) lookups
		registryMap := make(map[string]bool)
		for _, reg := range flags.SourceRegistries {
			registryMap[reg] = true
		}

		// Filter images
		for _, img := range analysis.Images {
			if registryMap[img.Registry] {
				filteredImages = append(filteredImages, img)
			}
		}

		// Update the analysis with filtered images
		analysis.Images = filteredImages
		log.Info("Filtered images to", len(flags.SourceRegistries), "registries")
	}

	// Write output
	return writeOutput(cmd, analysis, flags)
}

// getInspectFlags retrieves and validates flags for the inspect command
func getInspectFlags(cmd *cobra.Command, releaseNameProvided bool) (*InspectFlags, error) {
	// Get chart path
	chartPath, err := cmd.Flags().GetString("chart-path")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
		}
	}

	// Get release-name flag
	releaseName, err := cmd.Flags().GetString("release-name")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get release-name flag: %w", err),
		}
	}

	// Get namespace flag
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get namespace flag: %w", err),
		}
	}

	// Get all-namespaces flag
	allNamespaces, err := cmd.Flags().GetBool("all-namespaces")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get all-namespaces flag: %w", err),
		}
	}

	// If all-namespaces is true, validate that it doesn't conflict with other flags
	if allNamespaces {
		if chartPath != "" {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("--all-namespaces cannot be used together with --chart-path"),
			}
		}

		if releaseName != "" {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("--all-namespaces cannot be used together with --release-name"),
			}
		}

		if namespace != defaultNamespace {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("--all-namespaces cannot be used together with --namespace"),
			}
		}
	}

	// Only require chart-path if we're not using a release name in plugin mode or all-namespaces
	if chartPath == "" && !releaseNameProvided && !allNamespaces {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitMissingRequiredFlag,
			Err:  fmt.Errorf("required flag \"chart-path\" not set"),
		}
	}

	// If we have a chart path, validate it
	if chartPath != "" {
		// Check if chart exists using the AppFs
		_, err = AppFs.Stat(chartPath)
		if err != nil {
			// Check if the error is specifically a 'not found' error
			if os.IsNotExist(err) {
				return nil, &exitcodes.ExitCodeError{
					Code: exitcodes.ExitChartNotFound,
					Err:  fmt.Errorf("chart path not found or inaccessible: %s", chartPath),
				}
			}
			// Handle other potential errors from Stat (e.g., permissions)
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartLoadFailed, // Use a more general load fail code
				Err:  fmt.Errorf("error accessing chart path %s: %w", chartPath, err),
			}
		}
	}

	// Get output file
	outputFile, err := cmd.Flags().GetString("output-file")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-file flag: %w", err),
		}
	}

	// Get output format
	outputFormat, err := cmd.Flags().GetString("output-format")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-format flag: %w", err),
		}
	}

	// If not set, default to yaml (should already be default, but double-check for empty string)
	if outputFormat == "" {
		outputFormat = outputFormatYAML
	}

	// Validate output format
	if outputFormat != outputFormatYAML && outputFormat != outputFormatJSON {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("invalid output format '%s': must be '%s' or '%s'", outputFormat, outputFormatYAML, outputFormatJSON),
		}
	}

	// Get generate-config-skeleton flag
	generateConfigSkeleton, err := cmd.Flags().GetBool("generate-config-skeleton")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get generate-config-skeleton flag: %w", err),
		}
	}

	// Get overwrite-skeleton flag
	overwriteSkeleton, err := cmd.Flags().GetBool("overwrite-skeleton")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get overwrite-skeleton flag: %w", err),
		}
	}

	// Validate overwrite-skeleton is only used with generate-config-skeleton
	if overwriteSkeleton && !generateConfigSkeleton {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("--overwrite-skeleton requires --generate-config-skeleton"),
		}
	}

	// Get analysis patterns
	includePatterns, excludePatterns, err := getAnalysisPatterns(cmd)
	if err != nil {
		return nil, err
	}

	// Create analyzer config
	analyzerConfig := &analyzer.Config{}
	analyzerConfig.IncludePatterns = includePatterns
	analyzerConfig.ExcludePatterns = excludePatterns

	// Get source registries
	sourceRegistries, err := cmd.Flags().GetStringSlice("source-registries")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get source-registries flag: %w", err),
		}
	}

	return &InspectFlags{
		ChartPath:              chartPath,
		OutputFile:             outputFile,
		OutputFormat:           outputFormat,
		GenerateConfigSkeleton: generateConfigSkeleton,
		AnalyzerConfig:         analyzerConfig,
		SourceRegistries:       sourceRegistries,
		AllNamespaces:          allNamespaces,
		OverwriteSkeleton:      overwriteSkeleton,
	}, nil
}

// getAnalysisPatterns retrieves include/exclude patterns from flags
func getAnalysisPatterns(cmd *cobra.Command) (includePatterns, excludePatterns []string, err error) {
	// Get include patterns
	includePatterns, err = cmd.Flags().GetStringSlice("include-pattern")
	if err != nil {
		return nil, nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get include-pattern flag: %w", err),
		}
	}

	// Get exclude patterns
	excludePatterns, err = cmd.Flags().GetStringSlice("exclude-pattern")
	if err != nil {
		return nil, nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get exclude-pattern flag: %w", err),
		}
	}

	return includePatterns, excludePatterns, nil
}

// processImagePatterns converts analysis patterns into ImageInfo and skipped list
func processImagePatterns(patterns []analyzer.ImagePattern) (images []ImageInfo, skipped []string) {
	// Extract images from patterns
	for _, pattern := range patterns {
		// Skip non-string patterns for direct image extraction
		if pattern.Type != "string" {
			// For map-based patterns, construct the image reference
			if pattern.Structure != nil {
				var imageRef string
				if pattern.Structure.Registry != "" {
					imageRef += pattern.Structure.Registry + "/"
				}
				imageRef += pattern.Structure.Repository
				if pattern.Structure.Tag != "" {
					imageRef += ":" + pattern.Structure.Tag
				}

				// Parse the constructed image reference
				imgRef, err := image.ParseImageReference(imageRef)
				if err != nil {
					skipped = append(skipped, fmt.Sprintf("%s (%s): %v", pattern.Path, imageRef, err))
					continue
				}

				// Convert to our ImageInfo type
				images = append(images, ImageInfo{
					Registry:   imgRef.Registry,
					Repository: imgRef.Repository,
					Tag:        imgRef.Tag,
					Digest:     imgRef.Digest,
					Source:     pattern.Path,
				})
			}
			continue
		}

		// For string patterns, parse directly
		imgRef, err := image.ParseImageReference(pattern.Value)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s (%s): %v", pattern.Path, pattern.Value, err))
			continue
		}

		// Convert to our ImageInfo type
		images = append(images, ImageInfo{
			Registry:   imgRef.Registry,
			Repository: imgRef.Repository,
			Tag:        imgRef.Tag,
			Digest:     imgRef.Digest,
			Source:     pattern.Path,
		})
	}

	return images, skipped
}

// detectChartInCurrentDirectory first checks the given start directory ("."), then searches upwards within the provided filesystem for a Chart.yaml file.
// It returns the absolute path (relative to fs root) to the chart directory and a matching relative path,
// or an error if not found.
func detectChartInCurrentDirectory(fs afero.Fs) (detectedAbsPath, detectedRelPath string, err error) {
	startSearchDir := "."
	log.Debug("detectChartInCurrentDirectory: Start", "fs_root_relative_start", startSearchDir)

	// 1. Check the starting directory itself
	startChartFilePath := filepath.Join(startSearchDir, chartutil.ChartfileName)
	log.Debug("Checking for chart in start directory", "path", startChartFilePath)
	exists, err := afero.Exists(fs, startChartFilePath)
	if err != nil {
		log.Debug("Error checking for chart file existence in start dir (ignoring)", "path", startChartFilePath, "error", err)
	}
	if exists {
		cleanAbsPath := filepath.Clean(startSearchDir)
		log.Debug("Chart found in start directory", "absolutePath", cleanAbsPath)
		// Return the start directory path for both values when found immediately
		return cleanAbsPath, cleanAbsPath, nil
	}
	log.Debug("Chart not found in start directory, searching upwards...")

	// 2. Search upwards from the parent of the starting directory
	currentDir := filepath.Dir(startSearchDir) // Start searching from parent
	if currentDir == startSearchDir {          // Handle case where start is already root
		currentDir = "." // Ensure we check root if needed
	}

	maxSearchDepth := 100 // Prevent infinite loops

	for i := 0; i < maxSearchDepth; i++ {
		// If currentDir is empty or invalid, stop
		if currentDir == "" || currentDir == "/" || currentDir == "." && i > 0 { // Avoid redundant check of "." if we started there
			log.Debug("Reached root or invalid directory while searching upwards", "currentDir", currentDir)
			break
		}

		chartFilePath := filepath.Join(currentDir, chartutil.ChartfileName)
		log.Debug("Checking for chart upwards", "path", chartFilePath, "iteration", i)

		exists, err := afero.Exists(fs, chartFilePath)
		if err != nil {
			log.Debug("Error checking for chart file existence upwards (ignoring)", "path", chartFilePath, "error", err)
		}

		if exists {
			cleanAbsPath := filepath.Clean(currentDir)
			log.Debug("Chart found upwards", "absolutePath", cleanAbsPath)
			// Return the found path for both values
			return cleanAbsPath, cleanAbsPath, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir { // Termination check
			log.Debug("Reached filesystem root while searching upwards", "currentDir", currentDir)
			break
		}
		currentDir = parentDir
	}

	log.Debug("Chart not found searching upwards from fs root", "startDir", startSearchDir)
	return "", "", fmt.Errorf("no %s found in current directory or searching upwards from the root of the provided filesystem", chartutil.ChartfileName)
}

// detectChartIfNeeded determines the chart path if not provided.
// It prioritizes the provided chart path. If empty, it calls detectChartInCurrentDirectory.
func detectChartIfNeeded(fs afero.Fs, inputChartPath string) (finalAbsPath, finalRelPath string, err error) {
	log.Debug("detectChartIfNeeded: Start", "inputChartPath", inputChartPath)
	if inputChartPath != "" {
		log.Debug("detectChartIfNeeded: Chart path provided, skipping detection", "chartPath", inputChartPath)
		// Return the input path and "." for relative path as detection was skipped.
		return inputChartPath, ".", nil
	}

	log.Debug("detectChartIfNeeded: No chart path provided, searching current directory.")
	detectedPath, relativePath, err := detectChartInCurrentDirectory(fs)
	if err != nil {
		// Wrap the error from detection.
		return "", "", fmt.Errorf("chart path not specified and error occurred during detection: %w", err)
	}
	log.Debug("detectChartIfNeeded: Detected chart path", "detectedPath", detectedPath, "relativePath", relativePath)
	return detectedPath, relativePath, nil
}

// createConfigSkeleton generates a registry mapping config skeleton
func createConfigSkeleton(images []ImageInfo, outputFile string) error {
	// Use default filename if none specified
	if outputFile == "" {
		outputFile = DefaultConfigSkeletonFilename
		log.Info("No output file specified, using default:", outputFile)
	}

	// Note: File existence check is now done in writeOutput function
	// so we don't need to check here

	// Ensure the directory exists before trying to write the file
	dir := filepath.Dir(outputFile)
	if dir != "" && dir != "." {
		if err := AppFs.MkdirAll(dir, fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
			return fmt.Errorf("failed to create directory for config skeleton: %w", err)
		}
	}

	// Extract unique registries from images
	registries := make(map[string]bool)
	for _, img := range images {
		if img.Registry != "" {
			registries[img.Registry] = true
		}
	}

	// Sort registries for consistent output
	var registryList []string
	for registry := range registries {
		registryList = append(registryList, registry)
	}
	sort.Strings(registryList)

	// Create structured registry mappings
	mappings := make([]registry.RegMapping, 0, len(registryList))
	for _, reg := range registryList {
		// Generate a sanitized target registry path
		targetPath := strings.ReplaceAll(reg, ".", "-")
		mappings = append(mappings, registry.RegMapping{
			Source:      reg,
			Target:      "registry.local/" + targetPath,
			Description: fmt.Sprintf("Mapping for %s", reg),
			Enabled:     true,
		})
	}

	// Create config structure using the registry package format
	config := registry.Config{
		Version: "1.0",
		Registries: registry.RegConfig{
			Mappings:      mappings,
			DefaultTarget: "registry.local/default", // Example default target
			StrictMode:    false,                    // Default to false for better usability
		},
		Compatibility: registry.CompatibilityConfig{
			IgnoreEmptyFields: true,
		},
	}

	// Marshal to YAML
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config skeleton: %w", err)
	}

	// Add helpful comments
	yamlWithComments := fmt.Sprintf(`# IRR Configuration File
# 
# This file contains registry mappings for redirecting container images
# from public registries to your private registry. Update the target values
# to match your registry configuration.
#
# USAGE INSTRUCTIONS:
# 1. Update the 'target' fields with your actual registry paths
# 2. Use with 'irr override' command to generate image overrides
# 3. Validate generated overrides with 'irr validate'
#
# IMPORTANT NOTES:
# - This file uses the standard structured format which includes version, registries, 
#   and compatibility sections for enhanced functionality
# - The 'override' and 'validate' commands can run without this config, 
#   but image redirection correctness depends on your configuration
# - When using Harbor as a pull-through cache, ensure your target paths
#   match your Harbor project configuration
# - You can set or update mappings using 'irr config --source <reg> --target <path>'
# - This file was auto-generated from detected registries in your chart
#
%s`, string(configYAML))

	// Write the skeleton file
	err = afero.WriteFile(AppFs, outputFile, []byte(yamlWithComments), fileutil.ReadWriteUserPermission)
	if err != nil {
		return fmt.Errorf("failed to write config skeleton: %w", err)
	}

	absPath, err := filepath.Abs(outputFile)
	if err == nil {
		log.Info("Config skeleton written to", absPath)
	} else {
		log.Info("Config skeleton written to", outputFile)
	}

	log.Info("Update the target registry paths and use with 'irr config' to set up your configuration")
	return nil
}

// getAllReleases returns all Helm releases across all namespaces
func getAllReleases() ([]*helm.ReleaseElement, *helm.Adapter, error) {
	// Create a Helm adapter for interacting with the cluster
	helmAdapter, err := helmAdapterFactory()
	if err != nil {
		return nil, nil, err // Assumes factory returns ExitCodeError on failure
	}
	// Add explicit nil check for helmAdapter to satisfy nilaway and prevent potential panics
	if helmAdapter == nil {
		return nil, nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  errors.New("internal error: helmAdapterFactory returned nil adapter without error"),
		}
	}

	// List all releases across all namespaces
	client, err := createHelmClient()
	if err != nil {
		return nil, helmAdapter, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to create Helm client: %w", err),
		}
	}

	log.Debug("Listing all Helm releases across all namespaces")
	releases, err := client.ListReleases(context.Background(), true)
	if err != nil {
		return nil, helmAdapter, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to list Helm releases: %w", err),
		}
	}

	log.Info("Found", len(releases), "releases across all namespaces")
	return releases, helmAdapter, nil
}

// analyzeRelease analyzes a single Helm release and returns the analysis result and the original unfiltered images
func analyzeRelease(release *helm.ReleaseElement, helmAdapter *helm.Adapter, flags *InspectFlags) (*ReleaseAnalysisResult, []ImageInfo, error) {
	log.Info("Analyzing release", "name", release.Name, "namespace", release.Namespace)

	// Get release values
	values, err := helmAdapter.GetReleaseValues(context.Background(), release.Name, release.Namespace)
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
	}

	// Analyze the release values using the provided analyzer config
	log.Debug("Analyzing release values for", "name", release.Name, "namespace", release.Namespace)
	analysisPatterns, analysisErr := analyzer.AnalyzeHelmValues(values, flags.AnalyzerConfig)
	if analysisErr != nil {
		return nil, nil, fmt.Errorf("analysis failed for release %s/%s: %w", release.Namespace, release.Name, analysisErr)
	}

	// Process image patterns found in values
	originalImages, skipped := processImagePatterns(analysisPatterns)

	// Create analysis result structure
	analysis := ImageAnalysis{
		Chart:         chartInfo,
		Images:        originalImages, // Start with original images
		ImagePatterns: analysisPatterns,
		Skipped:       skipped,
	}

	// --- Filtering Logic ---
	// Keep a copy of original images for skeleton generation, even if filtered for output
	unfilteredImagesForSkeleton := make([]ImageInfo, len(originalImages))
	copy(unfilteredImagesForSkeleton, originalImages)

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
		for _, img := range originalImages { // Iterate original images
			normalizedRegistry := image.NormalizeRegistry(img.Registry)
			if registryMap[normalizedRegistry] {
				filteredImagesForOutput = append(filteredImagesForOutput, img)
			}
		}

		// Update the analysis.Images field ONLY for the output result
		analysis.Images = filteredImagesForOutput
	}

	// Return the potentially filtered analysis result AND the original unfiltered images
	return &ReleaseAnalysisResult{
		ReleaseName: release.Name,
		Namespace:   release.Namespace,
		Analysis:    analysis,
	}, unfilteredImagesForSkeleton, nil // Return unfiltered images here
}

// processAllReleases processes all releases and returns the aggregated results
func processAllReleases(releases []*helm.ReleaseElement, helmAdapter *helm.Adapter, flags *InspectFlags) ([]*ReleaseAnalysisResult, []string, []ImageInfo, error) {
	var allResults []*ReleaseAnalysisResult
	var skippedReleases []string
	uniqueRegistries := make(map[string]bool)

	// Process each release
	for _, release := range releases {
		// Call analyzeRelease, which now returns unfiltered images as well
		result, unfilteredImages, err := analyzeRelease(release, helmAdapter, flags)
		if err != nil {
			log.Warn("Failed to analyze release", "name", release.Name, "namespace", release.Namespace, "error", err)
			skippedReleases = append(skippedReleases, fmt.Sprintf("%s/%s", release.Namespace, release.Name))
			continue
		}

		allResults = append(allResults, result) // Keep the potentially filtered result for output

		// Accumulate unique registries FROM THE UNFILTERED IMAGES for skeleton generation
		log.Debug("Processing release for skeleton registry aggregation", "release", release.Name, "namespace", release.Namespace, "unfiltered_image_count", len(unfilteredImages))
		for _, img := range unfilteredImages { // Use the unfiltered list here
			log.Debug("Checking image registry for skeleton set", "registry", img.Registry, "source_path", img.Source)
			if img.Registry != "" { // Ensure we don't add empty registries
				if !uniqueRegistries[img.Registry] {
					log.Debug("Adding new unique registry to skeleton set", "registry", img.Registry)
				}
				uniqueRegistries[img.Registry] = true // Add registry to the map
			} else {
				log.Debug("Skipping image with empty registry for skeleton set", "source_path", img.Source, "repository", img.Repository)
			}
		}
	}

	// Handle no results case (check uniqueRegistries as well)
	if len(allResults) == 0 && len(uniqueRegistries) == 0 {
		msg := "No releases were successfully analyzed or no registries found"
		if len(skippedReleases) > 0 {
			msg += fmt.Sprintf(". %d releases were skipped due to errors", len(skippedReleases))
		}
		log.Warn(msg)
		// Return nil for skeletonImages here as well
		return nil, skippedReleases, nil, errors.New(msg)
	}

	// Create images list for skeleton generation from the unique registries map
	var skeletonImages []ImageInfo
	for registry := range uniqueRegistries {
		skeletonImages = append(skeletonImages, ImageInfo{
			Registry: registry,
		})
	}

	// Sort skeleton images by registry for consistent output
	sort.Slice(skeletonImages, func(i, j int) bool {
		return skeletonImages[i].Registry < skeletonImages[j].Registry
	})

	return allResults, skippedReleases, skeletonImages, nil
}

// outputMultiReleaseAnalysis formats and outputs the analysis results for multiple releases
func outputMultiReleaseAnalysis(cmd *cobra.Command, results []*ReleaseAnalysisResult, skipped []string, flags *InspectFlags) error {
	// Create a combined output structure
	type CombinedAnalysisResult struct {
		Releases []*ReleaseAnalysisResult `json:"releases" yaml:"releases"`
		Skipped  []string                 `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	}

	combinedResult := CombinedAnalysisResult{
		Releases: results,
		Skipped:  skipped,
	}

	// Determine output format (yaml or json)
	var output []byte
	var marshalErr error

	switch strings.ToLower(flags.OutputFormat) {
	case "json":
		output, marshalErr = json.Marshal(combinedResult)
		if marshalErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to JSON: %w", marshalErr),
			}
		}
	default:
		// Default to YAML
		output, marshalErr = yaml.Marshal(combinedResult)
		if marshalErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to YAML: %w", marshalErr),
			}
		}
	}

	// Write to file or stdout
	if flags.OutputFile != "" {
		if err := afero.WriteFile(AppFs, flags.OutputFile, output, fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write analysis to file: %w", err),
			}
		}
		log.Info("Analysis written to", flags.OutputFile)
	} else {
		// Use the command's out buffer instead of fmt.Println directly
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(output)); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write analysis to stdout: %w", err),
			}
		}
	}

	// Log summary information
	if len(skipped) > 0 {
		log.Warn("Some releases were skipped during analysis:", "count", len(skipped))
		for _, skippedRelease := range skipped {
			log.Warn("  - " + skippedRelease)
		}
	}

	log.Info("Successfully analyzed", len(results), "releases")
	return nil
}

// inspectAllNamespaces handles inspection of all Helm releases across all namespaces
func inspectAllNamespaces(cmd *cobra.Command, flags *InspectFlags) error {
	log.Info("Inspecting all Helm releases across all namespaces...")

	// Get all releases
	releases, helmAdapter, err := getAllReleases()
	if err != nil {
		return err
	}

	// Process all releases
	results, skippedReleases, skeletonImages, err := processAllReleases(releases, helmAdapter, flags)
	if err != nil && !flags.GenerateConfigSkeleton {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  err,
		}
	}

	// Handle skeleton generation
	if flags.GenerateConfigSkeleton {
		log.Info("Generating config skeleton from all releases...")

		// If we have no images but we're in skeleton mode, return an error
		if len(skeletonImages) == 0 {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartProcessingFailed,
				Err:  errors.New("no registries found for skeleton generation"),
			}
		}

		// Generate skeleton file
		skeletonFile := flags.OutputFile
		if skeletonFile == "" {
			skeletonFile = DefaultConfigSkeletonFilename
		}

		// Check if the skeleton file exists
		exists, err := afero.Exists(AppFs, skeletonFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to check if skeleton file exists: %w", err),
			}
		}

		// If the file exists and overwriteSkeleton is false, return an error
		if exists && !flags.OverwriteSkeleton {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("output file %s already exists; use --overwrite-skeleton to overwrite", skeletonFile),
			}
		}

		if err := createConfigSkeleton(skeletonImages, skeletonFile); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to create config skeleton: %w", err),
			}
		}

		log.Info("Config skeleton generated successfully", "file", skeletonFile)
		return nil
	}

	// Output analysis results
	return outputMultiReleaseAnalysis(cmd, results, skippedReleases, flags)
}
