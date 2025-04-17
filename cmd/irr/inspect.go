package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/analyzer"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/lalbers/irr/pkg/image"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/cobra"
)

const (
	// SecureFilePerms is the permission set used for files created by the inspect command
	SecureFilePerms = 0o600
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
	Chart    ChartInfo               `json:"chart" yaml:"chart"`
	Images   []ImageInfo             `json:"images" yaml:"images"`
	Patterns []analyzer.ImagePattern `json:"patterns" yaml:"patterns"`
	Errors   []string                `json:"errors,omitempty" yaml:"errors,omitempty"`
	Skipped  []string                `json:"skipped,omitempty" yaml:"skipped,omitempty"`
}

// InspectFlags holds the command line flags for the inspect command
type InspectFlags struct {
	ChartPath              string
	OutputFormat           string
	OutputFile             string
	GenerateConfigSkeleton bool
	AnalyzerConfig         *analyzer.Config
	SourceRegistries       []string
}

// newInspectCmd creates a new inspect command
func newInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect a Helm chart for image references",
		Long: `Inspect a Helm chart to find all image references.
This command analyzes the chart's values.yaml and templates to find image references.`,
		RunE: runInspect,
	}

	cmd.Flags().String("chart-path", "", "Path to the Helm chart")
	cmd.Flags().String("output-format", "yaml", "Output format (yaml or json)")
	cmd.Flags().String("output-file", "", "Write output to file instead of stdout")
	cmd.Flags().Bool("generate-config-skeleton", false, "Generate a config skeleton based on found images")
	cmd.Flags().StringSlice("include-pattern", nil, "Glob patterns for values paths to include during analysis")
	cmd.Flags().StringSlice("exclude-pattern", nil, "Glob patterns for values paths to exclude during analysis")
	cmd.Flags().StringSlice("known-image-paths", nil, "Specific dot-notation paths known to contain images")
	cmd.Flags().StringSliceP("source-registries", "r", nil, "Source registries to filter results (optional)")
	cmd.Flags().String("release-name", "", "Release name for Helm plugin mode")
	cmd.Flags().String("namespace", "", "Kubernetes namespace for the release")

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
		Chart:    chartInfo,
		Images:   images,
		Patterns: patterns,
		Skipped:  skipped,
	}

	return analysis, nil
}

// writeOutput writes the analysis output to file or stdout
func writeOutput(analysis *ImageAnalysis, flags *InspectFlags) error {
	// Handle generate-config-skeleton flag
	if flags.GenerateConfigSkeleton {
		skeletonFile := flags.OutputFile
		if skeletonFile == "" {
			skeletonFile = "irr-config.yaml"
		}
		if err := createConfigSkeleton(analysis.Images, skeletonFile); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to create config skeleton: %w", err),
			}
		}
		return nil
	}

	// Marshal analysis to YAML
	output, err := yaml.Marshal(analysis)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal analysis to YAML: %w", err),
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

// runInspect implements the inspect command logic
func runInspect(cmd *cobra.Command, args []string) error {
	// Get command flags
	flags, err := getInspectFlags(cmd)
	if err != nil {
		return err
	}

	// Check if a release name was provided (either via flag or positional argument)
	releaseName, err := cmd.Flags().GetString("release-name")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get release-name flag: %w", err),
		}
	}

	// Check for positional argument as release name if flag is not set and we're running as a plugin
	if releaseName == "" && isHelmPlugin && len(args) > 0 {
		releaseName = args[0]
		log.Infof("Using %s as release name from positional argument", releaseName)
	}

	// Get namespace for Helm operations
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get namespace flag: %w", err),
		}
	}

	// Handle Helm release mode if a release name is provided and we're running as a plugin
	if releaseName != "" {
		if !isHelmPlugin {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("the release name flag is only available when running as a Helm plugin (helm irr ...)"),
			}
		}

		// Create a new Helm client
		helmClient, err := helm.NewHelmClient()
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("failed to initialize Helm client: %w", err),
			}
		}

		// Create adapter with the Helm client
		adapter := helm.NewAdapter(helmClient, AppFs)

		// Perform the inspect operation
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		// Get output file from flags
		outputFile := flags.OutputFile

		// Call the adapter's InspectRelease method with the output file
		err = adapter.InspectRelease(ctx, releaseName, namespace, outputFile)
		if err != nil {
			return err
		}

		// Return success directly as the adapter has already handled the output
		return nil
	}

	// If chart path is not specified, try to detect a chart in the current directory
	if flags.ChartPath == "" {
		chartPath, err := detectChartInCurrentDirectory()
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("chart path not specified and %w", err),
			}
		}
		flags.ChartPath = chartPath
		log.Infof("Detected chart at %s", chartPath)
	}

	// Load chart
	chartData, err := loadHelmChart(flags.ChartPath)
	if err != nil {
		return err
	}

	// Analyze chart
	analysis, err := analyzeChart(chartData, flags.AnalyzerConfig)
	if err != nil {
		return err
	}

	// Filter images if source registries are provided
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
		log.Infof("Filtered images to %d registries", len(flags.SourceRegistries))
	}

	// Write output
	return writeOutput(analysis, flags)
}

// getInspectFlags retrieves flags from the command
func getInspectFlags(cmd *cobra.Command) (*InspectFlags, error) {
	// Get chart path
	chartPath, err := cmd.Flags().GetString("chart-path")
	if err != nil || chartPath == "" {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitMissingRequiredFlag,
			Err:  fmt.Errorf("required flag \"chart-path\" not set"),
		}
	}

	// Normalize chart path
	chartPath, err = filepath.Abs(chartPath)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get absolute path for chart: %w", err),
		}
	}

	// Check if chart exists
	_, err = AppFs.Stat(chartPath)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartNotFound,
			Err:  fmt.Errorf("chart path not found or inaccessible: %s", chartPath),
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

	// Validate output format
	outputFormat = strings.ToLower(outputFormat)
	if outputFormat != "yaml" {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("unsupported output format: %s (only 'yaml' supported)", outputFormat),
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

	// Get generate-config-skeleton flag
	generateConfigSkeleton, err := cmd.Flags().GetBool("generate-config-skeleton")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get generate-config-skeleton flag: %w", err),
		}
	}

	// Get analysis patterns
	includePatterns, excludePatterns, knownPaths, err := getAnalysisPatterns(cmd)
	if err != nil {
		return nil, err
	}

	// Create analyzer config
	analyzerConfig := &analyzer.Config{
		IncludePatterns: includePatterns,
		ExcludePatterns: excludePatterns,
		KnownPaths:      knownPaths,
	}

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
		OutputFormat:           outputFormat,
		OutputFile:             outputFile,
		GenerateConfigSkeleton: generateConfigSkeleton,
		AnalyzerConfig:         analyzerConfig,
		SourceRegistries:       sourceRegistries,
	}, nil
}

// getAnalysisPatterns retrieves the analysis pattern flags
func getAnalysisPatterns(cmd *cobra.Command) (includePatterns, excludePatterns, knownPaths []string, err error) {
	// Get include patterns
	includePatterns, err = cmd.Flags().GetStringSlice("include-pattern")
	if err != nil {
		return nil, nil, nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get include-pattern flag: %w", err),
		}
	}

	// Get exclude patterns
	excludePatterns, err = cmd.Flags().GetStringSlice("exclude-pattern")
	if err != nil {
		return nil, nil, nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get exclude-pattern flag: %w", err),
		}
	}

	// Get known image paths
	knownPaths, err = cmd.Flags().GetStringSlice("known-image-paths")
	if err != nil {
		return nil, nil, nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get known-image-paths flag: %w", err),
		}
	}

	return includePatterns, excludePatterns, knownPaths, nil
}

// processImagePatterns extracts image information from detected patterns
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

// detectChartInCurrentDirectory attempts to find a Helm chart in the current directory
func detectChartInCurrentDirectory() (string, error) {
	// Check if Chart.yaml exists in the current directory
	if _, err := AppFs.Stat("Chart.yaml"); err == nil {
		// Found Chart.yaml in current directory
		return ".", nil
	}

	// Check if there's a chart directory
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
				chartPath, err := filepath.Abs(entry.Name())
				if err != nil {
					return "", fmt.Errorf("failed to get absolute path for chart: %w", err)
				}
				return chartPath, nil
			}
		}
	}

	return "", fmt.Errorf("no Helm chart found in current directory")
}

// createConfigSkeleton generates a configuration skeleton based on the image analysis
func createConfigSkeleton(images []ImageInfo, outputFile string) error {
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

	// Create skeleton config
	config := struct {
		RegistryMappings  map[string]string `yaml:"registry_mappings"`
		ExcludeRegistries []string          `yaml:"exclude_registries,omitempty"`
		PathStrategy      string            `yaml:"path_strategy"`
	}{
		RegistryMappings:  make(map[string]string),
		ExcludeRegistries: []string{},
		PathStrategy:      "prefix-source-registry", // default
	}

	// Fill in registry mappings with placeholders
	for _, registry := range registryList {
		config.RegistryMappings[registry] = "registry.local/" + strings.ReplaceAll(registry, ".", "-")
	}

	// Convert to YAML
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to generate YAML: %w", err)
	}

	// Add comments to the YAML
	yamlWithComments := "# IRR Configuration Skeleton\n" +
		"# Generated based on image analysis\n" +
		"# Replace placeholder values with your actual registry mappings\n\n" +
		string(configYAML)

	// Write the skeleton file
	err = afero.WriteFile(AppFs, outputFile, []byte(yamlWithComments), SecureFilePerms)
	if err != nil {
		return fmt.Errorf("failed to write config skeleton: %w", err)
	}

	log.Infof("Config skeleton written to %s", outputFile)
	return nil
}
