package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	"github.com/spf13/cobra"
)

const (
	// SecureFilePerms is the permission set used for files created by the inspect command
	SecureFilePerms = 0o600

	// SecureDirPerms is the permission set used for directories created by the inspect command
	SecureDirPerms = 0o700

	// DefaultConfigSkeletonFilename is the default filename for the generated config skeleton
	DefaultConfigSkeletonFilename = "irr-config.yaml"
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
	OutputFormat           string
	OutputFile             string
	GenerateConfigSkeleton bool
	AnalyzerConfig         *analyzer.Config
	SourceRegistries       []string
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
		Chart:         chartInfo,
		Images:        images,
		ImagePatterns: patterns,
		Skipped:       skipped,
	}

	return analysis, nil
}

// writeOutput writes the analysis output to file or stdout
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

	// Marshal analysis to YAML or JSON based on output format
	var output []byte
	var err error

	if flags.OutputFormat == "json" {
		output, err = json.Marshal(analysis)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to JSON: %w", err),
			}
		}
	} else {
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
	// Get flags
	flags, err := getInspectFlags(cmd, len(args) > 0)
	if err != nil {
		return err
	}

	// If running as a Helm plugin with a release name, use that approach
	if isHelmPlugin && len(args) > 0 {
		releaseName := args[0]
		namespace, err := cmd.Flags().GetString("namespace")
		if err != nil {
			namespace = ""
		}
		return inspectHelmRelease(cmd, flags, releaseName, namespace)
	}

	// Determine chart path
	chartPath := flags.ChartPath
	if chartPath == "" {
		// Try to detect chart in current directory
		detectedPath, err := detectChartInCurrentDirectory()
		if err != nil {
			log.Errorf("No chart path provided and failed to detect chart: %v", err)
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("neither chart path nor release name provided, and failed to auto-detect chart"),
			}
		}
		chartPath = detectedPath
		log.Infof("Auto-detected chart path: %s", chartPath)
	}
	log.Infof("Using chart path: %s", chartPath)

	// Load the chart
	chartData, err := loadHelmChart(chartPath)
	if err != nil {
		log.Errorf("Failed to load chart: %s", err)
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartNotFound,
			Err:  fmt.Errorf("chart path not found or invalid: %w", err),
		}
	}

	// Analyze chart
	analysis, err := analyzeChart(chartData, flags.AnalyzerConfig)
	if err != nil {
		log.Errorf("Chart analysis failed: %s", err)
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("chart analysis failed: %w", err),
		}
	}

	// Filter images based on source registries if provided
	if len(flags.SourceRegistries) > 0 {
		var filteredImages []ImageInfo
		for _, img := range analysis.Images {
			// Check if this image's registry is in the source registries list
			for _, srcReg := range flags.SourceRegistries {
				if strings.EqualFold(img.Registry, srcReg) {
					filteredImages = append(filteredImages, img)
					break
				}
			}
		}
		// Replace the images with filtered list
		if len(filteredImages) < len(analysis.Images) {
			log.Infof("Filtered images from %d to %d based on source registries", len(analysis.Images), len(filteredImages))
			analysis.Images = filteredImages
		}
	}

	// Generate config skeleton if requested
	if flags.GenerateConfigSkeleton {
		// If no specific output file was provided for the config skeleton,
		// use the default in the current working directory
		configFile := DefaultConfigSkeletonFilename
		if flags.OutputFile != "" {
			configFile = flags.OutputFile
		}

		// If the file path is not absolute, make it absolute from current directory
		if !filepath.IsAbs(configFile) {
			// Get current working directory
			cwd, err := os.Getwd()
			if err != nil {
				log.Warnf("Failed to get current working directory: %v", err)
			} else {
				configFile = filepath.Join(cwd, configFile)
			}
		}

		err := createConfigSkeleton(analysis.Images, configFile)
		if err != nil {
			log.Errorf("Failed to generate config skeleton: %s", err)
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to generate config skeleton: %w", err),
			}
		}
		// Already wrote config file, don't also write analysis output
		return nil
	}

	// Write output
	err = writeOutput(analysis, flags)
	if err != nil {
		log.Errorf("Failed to write output: %s", err)
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to write output: %w", err),
		}
	}

	return nil
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
	if outputFormat != "yaml" && outputFormat != "json" {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("unsupported output format: %s (supported formats: 'yaml', 'json')", outputFormat),
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
	// Use default filename if none specified
	if outputFile == "" {
		outputFile = DefaultConfigSkeletonFilename
		log.Infof("No output file specified, using default: %s", outputFile)
	}

	// Ensure the directory exists before trying to write the file
	dir := filepath.Dir(outputFile)
	if dir != "" && dir != "." {
		if err := AppFs.MkdirAll(dir, SecureDirPerms); err != nil {
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

	// Create skeleton config
	config := struct {
		RegistryMappings  map[string]string `yaml:"registry_mappings"`
		PrivateRegistries []string          `yaml:"private_registries"`
	}{
		RegistryMappings:  make(map[string]string),
		PrivateRegistries: []string{},
	}

	// Fill in registry mappings with placeholders
	for _, registry := range registryList {
		config.RegistryMappings[registry] = "registry.local/" + strings.ReplaceAll(registry, ".", "-")
	}

	// Marshal to YAML
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config skeleton: %w", err)
	}

	// Add helpful comments
	yamlWithComments := strings.ReplaceAll(
		fmt.Sprintf(`# IRR Configuration File
# 
# This is a skeleton configuration for IRR based on the chart analysis.
# Update the target registry mappings with your actual registries.
# 
# Uncomment and populate the private_registries section if you need to authenticate to any registries.
# Example:
# private_registries:
#   - registry: registry.example.com
#     username: username
#     password: password  # Or use password_file to reference a file containing the password
#     insecure: false     # Set to true to skip TLS verification
#
%s`, string(configYAML)),
		"\n  - []\n",
		"\n#  - registry: registry.example.com\n#    username: username\n#    password: password\n",
	)

	// Write the skeleton file
	err = afero.WriteFile(AppFs, outputFile, []byte(yamlWithComments), SecureFilePerms)
	if err != nil {
		return fmt.Errorf("failed to write config skeleton: %w", err)
	}

	absPath, err := filepath.Abs(outputFile)
	if err == nil {
		log.Infof("Config skeleton written to %s", absPath)
	} else {
		log.Infof("Config skeleton written to %s", outputFile)
	}
	return nil
}
