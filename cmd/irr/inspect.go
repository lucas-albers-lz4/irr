package main

import (
	"context"
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
	"helm.sh/helm/v3/pkg/cli"

	"github.com/lalbers/irr/pkg/analyzer"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/lalbers/irr/pkg/helm"
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
}

const (
	// DefaultConfigSkeletonFilename is the default filename for the generated config skeleton
	DefaultConfigSkeletonFilename = "irr-config.yaml"
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
		Chart:         chartInfo,
		Images:        images,
		ImagePatterns: patterns,
		Skipped:       skipped,
	}

	return analysis, nil
}

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

// runInspect implements the inspect command logic
func runInspect(cmd *cobra.Command, args []string) error {
	// Get flags for the inspect command
	var flags *InspectFlags
	var err error

	// Check if we're being run with a release name
	releaseNameProvided := len(args) > 0
	if releaseNameProvided && !isHelmPlugin {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("release name can only be used in Helm plugin mode"),
		}
	}

	flags, err = getInspectFlags(cmd, releaseNameProvided)
	if err != nil {
		return err
	}

	if releaseNameProvided {
		releaseName := args[0]
		namespace, err := cmd.Flags().GetString("namespace")
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to get namespace flag: %w", err),
			}
		}
		if namespace == "" {
			namespace = validateTestNamespace
		}
		return inspectHelmRelease(cmd, flags, releaseName, namespace)
	}

	chartPath, analysis, err := setupAnalyzerAndLoadChart(cmd, flags)
	if err != nil {
		return err
	}

	filterImagesBySourceRegistries(cmd, flags, analysis)

	if len(analysis.Images) == 0 {
		log.Warnf("No registries detected in the chart")
		log.Infof("If you expected to find registry references, check your chart's structure or try different analysis patterns")
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitRegistryDetectionError,
			Err:  fmt.Errorf("no registries detected in chart '%s'", chartPath),
		}
	}

	registries := extractUniqueRegistries(analysis.Images)

	if flags.GenerateConfigSkeleton {
		err = createConfigSkeleton(analysis.Images, flags.OutputFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to create config skeleton: %w", err),
			}
		}
		outputRegistrySuggestions(registries)
		return nil
	}

	err = writeOutput(analysis, flags)
	if err != nil {
		return err
	}

	if len(registries) > 0 {
		outputRegistryConfigSuggestion(chartPath, registries)
	}

	return nil
}

func setupAnalyzerAndLoadChart(cmd *cobra.Command, flags *InspectFlags) (string, *ImageAnalysis, error) {
	chartPath := flags.ChartPath
	if chartPath == "" {
		var err error
		chartPath, err = detectChartInCurrentDirectory()
		if err != nil {
			return "", nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("chart path not provided and could not auto-detect chart in current directory: %w", err),
			}
		}
		log.Infof("Auto-detected chart path: %s", chartPath)
	}

	analyzerConfig := &analyzer.Config{}
	includePatterns, excludePatterns, err := getAnalysisPatterns(cmd)
	if err != nil {
		return "", nil, err
	}
	analyzerConfig.IncludePatterns = includePatterns
	analyzerConfig.ExcludePatterns = excludePatterns

	sourceRegistries, err := cmd.Flags().GetStringSlice("source-registries")
	if err != nil {
		return "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get source-registries flag: %w", err),
		}
	}
	flags.SourceRegistries = sourceRegistries

	chartData, err := loadHelmChart(chartPath)
	if err != nil {
		return "", nil, err
	}

	analysis, err := analyzeChart(chartData, analyzerConfig)
	if err != nil {
		return "", nil, err
	}

	return chartPath, analysis, nil
}

func filterImagesBySourceRegistries(_ *cobra.Command, flags *InspectFlags, analysis *ImageAnalysis) {
	if len(flags.SourceRegistries) > 0 {
		log.Infof("Filtering results to only include registries: %s", strings.Join(flags.SourceRegistries, ", "))
		var filteredImages []ImageInfo
		for _, img := range analysis.Images {
			for _, sourceReg := range flags.SourceRegistries {
				if img.Registry == sourceReg {
					filteredImages = append(filteredImages, img)
					break
				}
			}
		}
		if len(filteredImages) == 0 && len(analysis.Images) > 0 {
			log.Warnf("No images found matching the provided source registries")
		}
		analysis.Images = filteredImages
	}
}

func extractUniqueRegistries(images []ImageInfo) map[string]bool {
	registries := make(map[string]bool)
	for _, img := range images {
		if img.Registry != "" {
			registries[img.Registry] = true
		}
	}
	return registries
}

func outputRegistrySuggestions(registries map[string]bool) {
	if len(registries) > 0 {
		log.Infof("\nDetected registries you may want to configure:")
		var regList []string
		for reg := range registries {
			regList = append(regList, reg)
		}
		sort.Strings(regList)
		for _, reg := range regList {
			log.Infof("  %s -> YOUR_REGISTRY/%s", reg, strings.ReplaceAll(reg, ".", "-"))
		}
		log.Infof("\nYou can configure these mappings with:")
		for _, reg := range regList {
			log.Infof("  irr config --source %s --target YOUR_REGISTRY/%s", reg, strings.ReplaceAll(reg, ".", "-"))
		}
	}
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

// inspectHelmRelease handles inspection logic for a Helm release (extracted from runInspect)
func inspectHelmRelease(cmd *cobra.Command, flags *InspectFlags, releaseName, namespace string) error {
	if !isHelmPlugin {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("the release name flag is only available when running as a Helm plugin (helm irr ...)"),
		}
	}

	// Use the global Helm client initialized in root.go
	if helmClient == nil {
		// Create a new Helm client if not already initialized
		settings := helm.GetHelmSettings()
		helmClient = helm.NewRealHelmClient(settings)
	}

	// Use the Helm client to get chart metadata and values
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// If namespace is not provided, use the default or environment namespace
	if namespace == "" {
		namespace = GetReleaseNamespace(cmd)
	}

	log.Infof("Inspecting release '%s' in namespace '%s'", releaseName, namespace)

	// Get chart metadata
	metadata, err := helmClient.GetReleaseMetadata(ctx, releaseName, namespace)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to get chart metadata for release %s: %w", releaseName, err),
		}
	}

	// Get release values
	values, err := helmClient.GetReleaseValues(ctx, releaseName, namespace)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to get values for release %s: %w", releaseName, err),
		}
	}

	// Create chart info
	chartInfo := ChartInfo{
		Name:    metadata.Name,
		Version: metadata.Version,
		Path:    fmt.Sprintf("release:%s", releaseName),
	}

	// Analyze values
	patterns, err := analyzer.AnalyzeHelmValues(values, flags.AnalyzerConfig)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("release analysis failed: %w", err),
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
		log.Infof("Filtered images to %d registries", len(flags.SourceRegistries))
	}

	// Write output
	return writeOutput(analysis, flags)
}

// getInspectFlags retrieves flags from the command
func getInspectFlags(cmd *cobra.Command, releaseNameProvided bool) (*InspectFlags, error) {
	// Get chart path
	chartPath, err := cmd.Flags().GetString("chart-path")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
		}
	}

	// Only require chart-path if we're not using a release name in plugin mode
	if chartPath == "" && !releaseNameProvided {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitMissingRequiredFlag,
			Err:  fmt.Errorf("required flag \"chart-path\" not set"),
		}
	}

	// If we have a chart path, validate it
	if chartPath != "" {
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

	// Get generate-config-skeleton flag
	generateConfigSkeleton, err := cmd.Flags().GetBool("generate-config-skeleton")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get generate-config-skeleton flag: %w", err),
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
	}, nil
}

// getAnalysisPatterns retrieves the analysis pattern flags
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
// The configuration is generated in the fully structured format, which is the preferred standard.
func createConfigSkeleton(images []ImageInfo, outputFile string) error {
	// Use default filename if none specified
	if outputFile == "" {
		outputFile = DefaultConfigSkeletonFilename
		log.Infof("No output file specified, using default: %s", outputFile)
	}

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
			DefaultTarget: "registry.local/default",
			StrictMode:    false,
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
		log.Infof("Config skeleton written to %s", absPath)
	} else {
		log.Infof("Config skeleton written to %s", outputFile)
	}

	log.Infof("Update the target registry paths and use with 'irr config' to set up your configuration")
	return nil
}
