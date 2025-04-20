package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"

	"github.com/lalbers/irr/pkg/analyzer"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/lalbers/irr/pkg/image"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/spf13/cobra"
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
	Chart   ChartInfo               `json:"chart" yaml:"chart"`
	Images  []analyzer.ImagePattern `json:"images" yaml:"images"`
	Errors  []string                `json:"errors,omitempty" yaml:"errors,omitempty"`
	Skipped []string                `json:"skipped,omitempty" yaml:"skipped,omitempty"`
}

// InspectFlags holds the command line flags for the inspect command
type InspectFlags struct {
	ChartPath              string
	OutputFile             string
	OutputFormat           string
	GenerateConfigSkeleton bool
	AnalyzerConfig         *analyzer.Config
	SourceRegistries       []string
	ValueFiles             []string
	ReleaseName            string
	Namespace              string
}

const (
	// DefaultConfigSkeletonFilename is the default filename for the generated config skeleton
	DefaultConfigSkeletonFilename = "irr-config.yaml"
	defaultNamespace              = "default" // Define constant for "default"
)

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
	cmd.Flags().String("output-format", "", "Output format (yaml or json)")
	cmd.Flags().Bool("generate-config-skeleton", false, "Generate a config skeleton based on found images")
	cmd.Flags().StringSlice("include-pattern", nil, "Glob patterns for values paths to include during analysis")
	cmd.Flags().StringSlice("exclude-pattern", nil, "Glob patterns for values paths to exclude during analysis")
	cmd.Flags().StringSliceP("source-registries", "r", nil, "Source registries to filter results (optional)")
	cmd.Flags().String("release-name", "", "Release name for Helm plugin mode and template analysis")
	cmd.Flags().String("namespace", "", "Kubernetes namespace for the release and template analysis")
	cmd.Flags().StringSliceP("values", "f", nil, "Specify values in a YAML file or a URL (can specify multiple)")

	return cmd
}

// loadHelmChart loads and validates a Helm chart
func loadHelmChart(chartPath string) (*chart.Chart, error) {
	// Initialize Helm environment
	settings := cli.New()

	// Create action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), "", "", log.Infof); err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to initialize Helm action config: %w", err),
		}
	}

	// Load chart
	loadedChart, err := loader.Load(chartPath)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed,
			Err:  fmt.Errorf("failed to load chart: %w", err),
		}
	}

	return loadedChart, nil
}

// analyzeChart analyzes a chart for image patterns
// DEPRECATED - Logic moved into runInspect and uses new AnalyzeChartValues
/*
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
*/

// writeOutput writes the analysis to a file or stdout
func writeOutput(analysis *ImageAnalysis, flags *InspectFlags) error {
	// Handle generate-config-skeleton flag
	if flags.GenerateConfigSkeleton {
		skeletonFile := flags.OutputFile
		if skeletonFile == "" {
			skeletonFile = DefaultConfigSkeletonFilename
		}
		if err := createConfigSkeleton(analysis.Images, skeletonFile); err != nil {
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
		log.Infof("Analysis written to %s", flags.OutputFile)
	} else {
		fmt.Println(string(output))
	}

	return nil
}

// --- Helper Functions for runInspect ---

// loadChartAndValues handles loading the chart, user values, and coalescing them.
func loadChartAndValues(flags *InspectFlags) (*chart.Chart, map[string]interface{}, error) {
	log.Debugf("Loading chart from path: %s", flags.ChartPath)
	loadedChart, err := loadHelmChart(flags.ChartPath)
	if err != nil {
		return nil, nil, err
	}

	log.Debugf("Loading user values from: %v", flags.ValueFiles)
	settings := cli.New() // Need Helm settings for getter
	valueOpts := values.Options{ValueFiles: flags.ValueFiles}
	userValues, err := valueOpts.MergeValues(getter.All(settings))
	if err != nil {
		return loadedChart, nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to merge user-provided values files: %w", err),
		}
	}
	log.Debugf("Successfully merged %d user value files", len(flags.ValueFiles))

	log.Debugf("Coalescing chart values...")
	finalMergedValues, err := chartutil.CoalesceValues(loadedChart, userValues)
	if err != nil {
		return loadedChart, userValues, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("failed to coalesce chart values: %w", err),
		}
	}
	log.Debugf("Successfully coalesced values.")
	return loadedChart, finalMergedValues, nil
}

// performAnalysis runs the core image analysis.
func performAnalysis(analyzerConfig *analyzer.Config, loadedChart *chart.Chart, finalMergedValues map[string]interface{}) ([]analyzer.ImagePattern, error) {
	log.Debugf("Building value origin map...")
	originMap := buildOriginMap(loadedChart)
	log.Debugf("Built origin map with %d entries.", len(originMap))

	log.Debugf("Starting analysis with AnalyzeChartValues...")
	imagePatterns, err := analyzer.AnalyzeChartValues(finalMergedValues, originMap, analyzerConfig)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("chart analysis failed: %w", err),
		}
	}
	log.Debugf("AnalyzeChartValues completed, found %d patterns.", len(imagePatterns))
	return imagePatterns, nil
}

// buildAnalysisResult structures the analysis output.
func buildAnalysisResult(loadedChart *chart.Chart, flags *InspectFlags, imagePatterns []analyzer.ImagePattern) *ImageAnalysis {
	return &ImageAnalysis{
		Chart: ChartInfo{
			Name:         loadedChart.Metadata.Name,
			Version:      loadedChart.Metadata.Version,
			Path:         flags.ChartPath, // Use the validated path
			Dependencies: len(loadedChart.Dependencies()),
		},
		Images:  imagePatterns,
		Skipped: []string{}, // TODO: Populate skipped list if needed
		Errors:  []string{}, // TODO: Populate errors if any occurred during specific steps
	}
}

// suggestConfiguration potentially outputs a registry config suggestion.
func suggestConfiguration(flags *InspectFlags, analysis *ImageAnalysis) {
	if flags.GenerateConfigSkeleton || len(flags.SourceRegistries) > 0 {
		return // Don't suggest if generating skeleton or sources already provided
	}
	registries := extractUniqueRegistriesFromPatterns(analysis.Images) // Adapt helper
	if len(registries) > 0 {
		outputRegistryConfigSuggestion(flags.ChartPath, registries)
	}
}

// --- End Helper Functions ---

// runInspect implements the inspect command logic using helper functions.
func runInspect(cmd *cobra.Command, args []string) error {
	log.Debugf("--- Starting runInspect ---")
	// 1. Get flags and handle initial setup
	releaseNameProvided := len(args) > 0
	if releaseNameProvided && !isHelmPlugin {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("release name can only be used in Helm plugin mode"),
		}
	}

	flags, err := getInspectFlags(cmd, releaseNameProvided)
	if err != nil {
		log.Errorf("Error getting inspect flags: %v", err)
		return err
	}
	log.Debugf("InspectFlags parsed: %+v", flags) // Log parsed flags

	if releaseNameProvided {
		flags.ReleaseName = args[0]
		if flags.Namespace == "" {
			flags.Namespace = defaultNamespace // Use constant
		}
		log.Debugf("Running in plugin mode context (release: %s, ns: %s)", flags.ReleaseName, flags.Namespace)
	}

	// Determine chart path
	log.Debugf("Determining chart path...")
	chartPath, err := getChartPath(cmd, flags)
	if err != nil {
		log.Errorf("Error determining chart path: %v", err)
		return err
	}
	flags.ChartPath = chartPath
	log.Debugf("Chart path determined: %s", flags.ChartPath)

	// Get analyzer config
	log.Debugf("Getting analyzer config...")
	analyzerConfig, err := getAnalyzerConfig(cmd)
	if err != nil {
		log.Errorf("Error getting analyzer config: %v", err)
		return err
	}
	flags.AnalyzerConfig = analyzerConfig
	log.Debugf("Analyzer config obtained: %+v", flags.AnalyzerConfig)

	// Handle GenerateConfigSkeleton mode separately if needed (or incorporate)
	if flags.GenerateConfigSkeleton {
		// Assuming skeleton generation needs analysis first
		log.Infof("Generating config skeleton...")
		// This duplicates some loading logic, consider refactoring skeleton generation
		// or making analysis mandatory before skeleton generation.
		// For now, keep the flow, but note the potential redundancy.
	}

	// 2. Load chart and values
	log.Debugf("--- Loading chart and values ---")
	loadedChart, finalMergedValues, err := loadChartAndValues(flags)
	if err != nil {
		log.Errorf("Error loading chart and values: %v", err)
		return err
	}
	// Log limited output for merged values to avoid excessive logs
	log.Debugf("Chart loaded: %s, Version: %s", loadedChart.Metadata.Name, loadedChart.Metadata.Version)
	// Directly log potentially large YAML; debug logger should handle level checks.
	// Avoid marshaling potentially huge structures unless debug is definitely enabled.
	mergedValuesYAML, err := yaml.Marshal(finalMergedValues)
	if err != nil {
		log.Warnf("Could not marshal merged values for debugging: %v", err)
	} else {
		if len(mergedValuesYAML) > debugLogTruncateLengthLong { // Limit output size in logs
			trimmedValues := string(mergedValuesYAML[:debugLogTruncateLengthLong]) + "... (truncated)"
			log.Debugf("Final Merged Values (first %d bytes):\n%s", debugLogTruncateLengthLong, trimmedValues)
		} else {
			log.Debugf("Final Merged Values:\n%s", string(mergedValuesYAML))
		}
	}

	// 3. Perform analysis
	log.Debugf("--- Performing analysis ---")
	imagePatterns, err := performAnalysis(flags.AnalyzerConfig, loadedChart, finalMergedValues)
	if err != nil {
		log.Errorf("Error performing analysis: %v", err)
		return err
	}
	log.Debugf("Analysis found %d image patterns.", len(imagePatterns))

	// 4. Build and filter result
	log.Debugf("--- Building and filtering result ---")
	analysis := buildAnalysisResult(loadedChart, flags, imagePatterns)
	log.Debugf("Built initial analysis result.")
	filterImagePatternsBySourceRegistries(cmd, flags, analysis) // Filter modifies analysis.Images in place
	log.Debugf("Filtered analysis result. Image count: %d", len(analysis.Images))

	// 5. Write output
	log.Debugf("--- Writing output ---")
	if err := writeOutput(analysis, flags); err != nil {
		log.Errorf("Error writing output: %v", err)
		return err
	}
	log.Debugf("Output written successfully.")

	// 6. Suggest configuration
	log.Debugf("--- Suggesting configuration (if applicable) ---")
	suggestConfiguration(flags, analysis)
	log.Debugf("Configuration suggestion complete.")

	log.Debugf("--- runInspect finished successfully ---")
	return nil
}

// getChartPath determines the chart path based on flags and environment
func getChartPath(_ *cobra.Command, flags *InspectFlags) (string, error) {
	if flags.ChartPath != "" {
		return flags.ChartPath, nil
	}
	// If no chart path specified, try to detect in current directory
	detectedPath, err := detectChartInCurrentDirectory()
	if err != nil {
		// If detection fails and we're not in plugin mode expecting a release name, it's an error
		if !isHelmPlugin || flags.ReleaseName == "" { // Adjusted condition
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("chart path not specified (--chart-path) even though release name was provided"),
			}
		}
		// If in plugin mode with release name, maybe chart path isn't strictly needed *yet*?
		// This depends on whether inspectHelmRelease actually needs it. Assuming it does for now.
		log.Debugf("Chart path not specified, detection failed, but release name provided. Proceeding cautiously.")
		// Return empty path for now, let subsequent functions fail if it's required.
		// OR: perhaps plugin mode should *always* require --chart-path? TODO: Clarify intent.
		// Let's enforce requiring chart path for now.
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("chart path not specified (--chart-path) even though release name was provided"),
		}
	}
	log.Infof("Chart path not specified, using detected chart: %s", detectedPath)
	return detectedPath, nil
}

func getAnalyzerConfig(cmd *cobra.Command) (*analyzer.Config, error) {
	includePatterns, err := cmd.Flags().GetStringSlice("include-pattern")
	if err != nil {
		return nil, fmt.Errorf("failed to get include patterns: %w", err)
	}
	excludePatterns, err := cmd.Flags().GetStringSlice("exclude-pattern")
	if err != nil {
		return nil, fmt.Errorf("failed to get exclude patterns: %w", err)
	}
	return &analyzer.Config{
		IncludePatterns: includePatterns,
		ExcludePatterns: excludePatterns,
	}, nil
}

// filterImagePatternsBySourceRegistries filters the analysis results based on specified source registries
func filterImagePatternsBySourceRegistries(_ *cobra.Command, flags *InspectFlags, analysis *ImageAnalysis) {
	if len(flags.SourceRegistries) == 0 {
		log.Debugf("No source registries specified, skipping filtering.")
		return
	}

	log.Infof("Filtering results to only include registries: %s", strings.Join(flags.SourceRegistries, ", "))
	allowedRegistries := make(map[string]struct{})
	for _, reg := range flags.SourceRegistries {
		// Normalize registry names for comparison (e.g., handle docker.io/library)
		normalized := image.NormalizeRegistry(reg)
		allowedRegistries[normalized] = struct{}{}
		// Also add the original in case normalization isn't perfect or needed
		if reg != normalized {
			allowedRegistries[reg] = struct{}{}
		}
	}

	var filteredPatterns []analyzer.ImagePattern
	originalCount := len(analysis.Images)

	for i := range analysis.Images { // Use index instead of copying pattern
		pattern := &analysis.Images[i]
		var reg string
		var parseErr error

		switch pattern.Type {
		case "map":
			if pattern.Structure != nil {
				reg = pattern.Structure.Registry
				// Fallback: If registry is empty in structure, try parsing repository field
				if reg == "" && pattern.Structure.Repository != "" {
					imgRef, err := image.ParseImageReference(pattern.Structure.Repository)
					if err == nil && imgRef.Registry != "" {
						reg = imgRef.Registry
					} else if err != nil {
						parseErr = fmt.Errorf("failed to parse repository field '%s' from map at path '%s' for filtering: %w", pattern.Structure.Repository, pattern.Path, err)
					}
				}
			} else {
				log.Warnf("Pattern at path '%s' has type 'map' but Structure is nil, cannot determine registry for filtering.", pattern.Path)
				// Decide whether to keep or discard - let's discard as we can't verify registry
				continue
			}
		case "string":
			imgRef, err := image.ParseImageReference(pattern.Value)
			if err != nil {
				parseErr = fmt.Errorf("failed to parse image string '%s' from path '%s' for filtering: %w", pattern.Value, pattern.Path, err)
			} else {
				reg = imgRef.Registry
			}
		default:
			log.Warnf("Unknown pattern type '%s' at path '%s', skipping filtering for this pattern.", pattern.Type, pattern.Path)
			// Keep unknown types? Or discard? Let's keep them for now.
			filteredPatterns = append(filteredPatterns, *pattern)
			continue
		}

		// If parsing failed for this pattern, log it and decide whether to keep/discard.
		// Discarding seems safer for filtering if we couldn't determine the registry.
		if parseErr != nil {
			log.Warnf("Skipping pattern filtering due to parsing error: %v", parseErr)
			continue
		}

		// Normalize the extracted registry for comparison
		normalizedReg := image.NormalizeRegistry(reg)
		// Check if the normalized registry is in the allowed list
		if _, ok := allowedRegistries[normalizedReg]; ok {
			log.Debugf("Keeping pattern (Path: '%s', Type: '%s', Registry: '%s') as it matches source filter.", pattern.Path, pattern.Type, reg)
			filteredPatterns = append(filteredPatterns, *pattern)
		} else {
			log.Debugf("Filtering out pattern (Path: '%s', Type: '%s', Registry: '%s') as it does not match source filter.", pattern.Path, pattern.Type, reg)
		}
	}

	filteredCount := len(filteredPatterns)
	analysis.Images = filteredPatterns
	log.Debugf("Filtered image patterns from %d to %d based on %d source registries.", originalCount, filteredCount, len(flags.SourceRegistries))
}

// extractUniqueRegistriesFromPatterns extracts unique non-dockerhub registries from patterns
func extractUniqueRegistriesFromPatterns(patterns []analyzer.ImagePattern) map[string]bool {
	registries := make(map[string]bool)
	for i := range patterns { // Use index instead of copy
		pattern := &patterns[i]
		imgRef, err := image.ParseImageReference(pattern.Value)
		if err == nil {
			if imgRef.Registry != "" {
				registries[imgRef.Registry] = true
			}
		} else {
			log.Debugf("Could not parse image value '%s' at path '%s' for registry extraction: %v", pattern.Value, pattern.Path, err)
		}
	}
	return registries
}

func outputRegistryConfigSuggestion(chartPath string, registries map[string]bool) {
	log.Infof("\nRegistry configuration suggestions:")
	log.Infof("To generate a config file with detected registries, run:")
	log.Infof("  irr inspect --chart-path %s --generate-config-skeleton", chartPath)
	log.Infof("Or configure individual mappings with:")
	var regList []string
	for reg := range registries {
		regList = append(regList, reg)
	}
	sort.Strings(regList)
	for _, reg := range regList {
		log.Infof("  irr config --source %s --target YOUR_REGISTRY/%s", reg, strings.ReplaceAll(reg, ".", "-"))
	}
}

// getInspectFlags populates the InspectFlags struct from cobra command flags
func getInspectFlags(cmd *cobra.Command, releaseNameProvided bool) (*InspectFlags, error) {
	flags := &InspectFlags{}
	var err error

	// Get flag values
	flags.ChartPath, err = cmd.Flags().GetString("chart-path")
	if err != nil {
		return nil, fmt.Errorf("failed to get chart-path flag: %w", err)
	}
	flags.OutputFile, err = cmd.Flags().GetString("output-file")
	if err != nil {
		return nil, fmt.Errorf("failed to get output-file flag: %w", err)
	}
	flags.GenerateConfigSkeleton, err = cmd.Flags().GetBool("generate-config-skeleton")
	if err != nil {
		return nil, fmt.Errorf("failed to get generate-config-skeleton flag: %w", err)
	}
	flags.SourceRegistries, err = cmd.Flags().GetStringSlice("source-registries")
	if err != nil {
		return nil, fmt.Errorf("failed to get source-registries flag: %w", err)
	}
	flags.ValueFiles, err = cmd.Flags().GetStringSlice("values")
	if err != nil {
		return nil, fmt.Errorf("failed to get values flag: %w", err)
	}
	flags.Namespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace flag: %w", err)
	}

	// Set namespace default if not provided and not in plugin mode expecting release name context
	if flags.Namespace == "" && !releaseNameProvided { // Check if release name is NOT provided
		flags.Namespace = defaultNamespace // Use constant
	}

	// Analyzer config requires its own retrieval potentially based on other flags
	flags.AnalyzerConfig, err = getAnalyzerConfig(cmd)
	if err != nil {
		return nil, err
	}

	// Mode-specific logic (mostly handled in runInspect now)
	// Determine required flags based on mode
	// if isHelmPlugin {
	// 	if !releaseNameProvided && flags.ReleaseName == "" {
	// 		// Maybe get release name from env? Or error?
	// 		// return nil, fmt.Errorf("release name is required in Helm plugin mode")
	// 	}
	// } else { // Standalone mode
	// 	// Chart path check happens in runInspect using getChartPath
	// }

	// Validate flag combinations
	if flags.GenerateConfigSkeleton && flags.OutputFile != "" {
		log.Warnf("--output-file is ignored when --generate-config-skeleton is used. Skeleton will be written to %s.", DefaultConfigSkeletonFilename)
		flags.OutputFile = "" // Reset OutputFile to avoid confusion in writeOutput
	}
	if flags.GenerateConfigSkeleton && flags.OutputFormat != "" {
		log.Warnf("--output-format is ignored when --generate-config-skeleton is used.")
		flags.OutputFormat = ""
	}

	return flags, nil
}

// detectChartInCurrentDirectory attempts to find a Helm chart in the current directory
// Returns the path ("." or subdirectory name) or an error if not found.
func detectChartInCurrentDirectory() (string, error) {
	// Check if Chart.yaml exists in the current directory
	if _, err := AppFs.Stat("Chart.yaml"); err == nil {
		// Found Chart.yaml in current directory
		return ".", nil
	}

	// Check if there's a chart directory in the current directory
	entries, err := afero.ReadDir(AppFs, ".")
	if err != nil {
		return "", fmt.Errorf("failed to read current directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Check if the directory contains Chart.yaml
			chartFile := filepath.Join(entry.Name(), "Chart.yaml")
			if _, err := AppFs.Stat(chartFile); err == nil {
				// Found a chart directory
				// Return the relative path to the directory
				return entry.Name(), nil
			}
		}
	}

	return "", fmt.Errorf("no Helm chart found in current directory or direct subdirectories")
}

// createConfigSkeleton generates a registry configuration skeleton based on detected images.
func createConfigSkeleton(patterns []analyzer.ImagePattern, outputFile string) error {
	// If outputFile was initially empty (because --output-file is ignored with --generate-config-skeleton),
	// construct the full path within the current working directory using the default name.
	// If outputFile was provided (e.g., by internal logic, though currently unlikely for skeleton),
	// use it directly.
	effectiveOutputFile := outputFile
	if effectiveOutputFile == "" {
		// Default filename is relative to the current working directory where the command runs
		effectiveOutputFile = DefaultConfigSkeletonFilename
		log.Infof("No output file specified for skeleton, using default relative to CWD: %s", effectiveOutputFile)
	} else {
		// If outputFile was somehow provided, use its absolute path
		// Although currently, the flag parsing logic should make outputFile empty here.
		absPath, err := filepath.Abs(effectiveOutputFile)
		if err != nil {
			log.Warnf("Could not determine absolute path for skeleton output file '%s': %v", effectiveOutputFile, err)
		} else {
			effectiveOutputFile = absPath
		}
		log.Infof("Using specified path for skeleton output: %s", effectiveOutputFile)
	}

	log.Debugf("Effective path for config skeleton: %s", effectiveOutputFile)

	// Ensure the directory exists before trying to write the file
	dir := filepath.Dir(effectiveOutputFile)
	if dir != "" && dir != "." {
		if err := AppFs.MkdirAll(dir, fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
			return fmt.Errorf("failed to create directory '%s' for config skeleton: %w", dir, err)
		}
	}

	// Extract unique registries
	detectedRegistries := make(map[string]bool) // Use map[string]bool just to track unique regs
	for i := range patterns {                   // Use index instead of copying pattern
		pattern := &patterns[i]
		var reg string
		var parseErr error // Variable to hold potential parsing errors

		switch pattern.Type { // Use string comparison for Type
		case "map":
			// Access the Structure field for map types
			if pattern.Structure != nil {
				reg = pattern.Structure.Registry
				// Fallback: If registry is empty in structure, try parsing repository field
				if reg == "" && pattern.Structure.Repository != "" {
					log.Debugf("Registry field empty in map structure at path '%s', attempting to parse repository '%s' as full reference.", pattern.Path, pattern.Structure.Repository)
					imgRef, err := image.ParseImageReference(pattern.Structure.Repository)
					if err == nil && imgRef.Registry != "" {
						reg = imgRef.Registry
					} else if err != nil {
						// Log parsing error but don't stop processing other patterns
						parseErr = fmt.Errorf("failed to parse repository field '%s' from map at path '%s': %w", pattern.Structure.Repository, pattern.Path, err)
					}
				}
			} else {
				log.Warnf("Pattern at path '%s' has type 'map' but Structure is nil, skipping for skeleton.", pattern.Path)
				continue
			}
		case "string":
			// Use parser for string types (Value is guaranteed to be string)
			imgRef, err := image.ParseImageReference(pattern.Value)
			if err != nil {
				// Log parsing error but don't stop processing other patterns
				parseErr = fmt.Errorf("failed to parse image string '%s' from path '%s': %w", pattern.Value, pattern.Path, err)
			} else {
				reg = imgRef.Registry
			}
		default:
			log.Warnf("Unknown pattern type '%s' at path '%s', skipping for skeleton.", pattern.Type, pattern.Path)
			continue
		}

		// Log any parsing error encountered for this pattern
		if parseErr != nil {
			log.Warnf("Skipping pattern for skeleton generation due to parsing error: %v", parseErr)
			continue
		}

		// Logic to skip implicit/default Docker Hub registry for the skeleton
		normalizedReg := image.NormalizeRegistry(reg) // Normalize before comparison
		if reg == "" || normalizedReg == "docker.io" {
			log.Debugf("Skipping pattern (Path: '%s', Type: '%s', Registry: '%s') with implicit/default Docker Hub registry for skeleton.", pattern.Path, pattern.Type, reg)
			continue
		}

		// Use the original non-normalized registry name for the skeleton map key
		if !detectedRegistries[reg] {
			log.Debugf("Adding registry '%s' to skeleton based on pattern at path '%s'", reg, pattern.Path)
			detectedRegistries[reg] = true
		}
	}

	// Sort registries for consistent output
	registryList := make([]string, 0, len(detectedRegistries))
	for reg := range detectedRegistries {
		registryList = append(registryList, reg)
	}
	sort.Strings(registryList)

	// Create structured registry mappings
	mappings := make([]registry.RegMapping, 0, len(registryList))
	for _, reg := range registryList { // reg here is the key from detectedRegistries map
		// Generate a sanitized target registry path as a suggestion
		suggestedTarget := "your-target-registry.com/" + strings.ReplaceAll(strings.ReplaceAll(reg, ".", "-"), "/", "-")

		// Generate description - Special case for known registries if desired, else generic
		description := fmt.Sprintf("Mapping for %s", reg)
		if reg == "quay.io" { // Example: Special description
			description = "Quay.io Container Registry"
		} else if reg == "gcr.io" {
			description = "Google Container Registry"
		} // Add more known registries if needed

		mappings = append(mappings, registry.RegMapping{
			Source:      reg,             // Source should be the registry name from the list
			Target:      suggestedTarget, // Placeholder
			Description: description,
			Enabled:     true, // Default to enabled
		})
	}

	// Create the config structure using the registry package format
	skeletonConfig := registry.Config{
		Version: "1.0", // Use string literal for version
		Registries: registry.RegConfig{
			Mappings:      mappings,
			DefaultTarget: "your-target-registry.com/default", // Placeholder
			StrictMode:    false,
		},
		Compatibility: registry.CompatibilityConfig{
			IgnoreEmptyFields: true, // Example setting
		},
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(skeletonConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config skeleton to YAML: %w", err)
	}

	// Add helpful comments
	yamlWithComments := fmt.Sprintf(`# IRR Configuration File - Skeleton
#
# This skeleton was auto-generated by 'irr inspect --generate-config-skeleton'
# based on detected non-DockerHub registries in the chart.
#
# Instructions:
# 1. Update 'target' fields under 'registries.mappings' with your actual private registry paths.
# 2. Update 'registries.defaultTarget' if you want a fallback for unmapped registries (requires strictMode: false).
# 3. Review 'strictMode'. If true, irr will fail on unmapped source registries.
# 4. Save this file (e.g., as irr-config.yaml) and use it with 'irr override --registry-file irr-config.yaml'
#
# You can also manage mappings using 'irr config set --source <reg> --target <path>'
#
%s`, string(yamlData))

	// Write to file
	if err := afero.WriteFile(AppFs, effectiveOutputFile, []byte(yamlWithComments), fileutil.ReadWriteUserPermission); err != nil {
		return fmt.Errorf("failed to write config skeleton to file '%s': %w", effectiveOutputFile, err)
	}

	log.Infof("Configuration skeleton generated at: %s", effectiveOutputFile)
	return nil
}

// buildOriginMap builds a map where keys are dot-notation paths from the final merged values
// perspective, and values indicate the origin ("." for parent, subchart alias/name otherwise).
func buildOriginMap(parentChart *chart.Chart) map[string]string {
	originMap := make(map[string]string)

	// 1. Process parent chart's default values
	log.Debugf("Building origin map for parent: %s", parentChart.Metadata.Name)
	traverseValuesForOrigin(parentChart.Values, ".", ".", originMap)

	// 2. Build a lookup for loaded dependency charts by name
	loadedDeps := make(map[string]*chart.Chart)
	for _, depChart := range parentChart.Dependencies() {
		if depChart != nil && depChart.Metadata != nil {
			loadedDeps[depChart.Metadata.Name] = depChart
			log.Debugf(" Found loaded dependency chart: %s", depChart.Metadata.Name)
		} else {
			log.Warnf("Parent chart %s has a nil or metadata-less dependency chart object.", parentChart.Metadata.Name)
		}
	}

	// 3. Iterate through the dependency *metadata* from the parent's Chart.yaml
	if parentChart.Metadata != nil && len(parentChart.Metadata.Dependencies) > 0 {
		log.Debugf("Processing %d dependency definitions from parent Chart.yaml...", len(parentChart.Metadata.Dependencies))
		for _, depMeta := range parentChart.Metadata.Dependencies {
			if depMeta == nil {
				log.Warnf("Parent chart %s has a nil entry in Metadata.Dependencies.", parentChart.Metadata.Name)
				continue
			}

			// Determine the alias/prefix for this dependency
			alias := depMeta.Alias
			if alias == "" {
				alias = depMeta.Name // Fallback to name if no alias
			}
			if alias == "" {
				log.Warnf("Dependency definition found with no name or alias in parent %s. Cannot map origin.", parentChart.Metadata.Name)
				continue
			}

			// Find the corresponding *loaded* dependency chart
			loadedDepChart, found := loadedDeps[depMeta.Name]
			if !found {
				log.Warnf("Dependency '%s' (alias: '%s') in %s not found in loaded chart deps. Skipping origin map.",
					depMeta.Name, alias, parentChart.Metadata.Name)
				continue
			}

			// 4. Process the loaded dependency chart's default values
			log.Debugf(" Building origin map for dependency: %s (alias/prefix: %s)", depMeta.Name, alias)
			if loadedDepChart.Values != nil {
				// The path prefix *is* the alias.
				// The origin identifier *is also* the alias.
				traverseValuesForOrigin(loadedDepChart.Values, alias, alias, originMap)
			} else {
				log.Debugf(" Dependency %s has no default values.", depMeta.Name)
			}
		}
	} else {
		log.Debugf("Parent chart %s has no dependency definitions in Metadata.", parentChart.Metadata.Name)
	}

	return originMap
}

// Recursively traverses the values map to identify the origin of each top-level key.
// Combine parameter types
func traverseValuesForOrigin(valueMap map[string]interface{}, pathPrefix, origin string, originMap map[string]string) {
	for key, val := range valueMap {
		// Construct the full path from the perspective of the final merged values map
		fullPath := key
		if pathPrefix != "." { // Prefix with alias if not the parent chart
			fullPath = pathPrefix + "." + key
		}

		// Store the origin for this path
		// This might overwrite if keys overlap, but Helm's coalesce likely handles this; we just care about the final structure's origin.
		originMap[fullPath] = origin
		log.Debugf(" OriginMap: Mapped path '%s' to origin '%s'", fullPath, origin)

		// Recurse only for nested maps
		if nestedMap, ok := val.(map[string]interface{}); ok {
			traverseValuesForOrigin(nestedMap, fullPath, origin, originMap)
		}
		// Slices don't need explicit traversal here; their containing map determines origin.
	}
}
