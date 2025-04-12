package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/lalbers/irr/pkg/analyzer"
	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/image"
	log "github.com/lalbers/irr/pkg/log"
)

const (
	// SecureFilePerms is the permission set used for files created by the inspect command
	SecureFilePerms = 0o600
)

// ChartInfo holds the summarized chart information
type ChartInfo struct {
	Name         string `json:"name" yaml:"name"`
	Version      string `json:"version" yaml:"version"`
	Path         string `json:"path" yaml:"path"`
	Dependencies int    `json:"dependencies" yaml:"dependencies"`
}

// ImageInfo holds information about an image reference
type ImageInfo struct {
	Registry   string `json:"registry" yaml:"registry"`
	Repository string `json:"repository" yaml:"repository"`
	Tag        string `json:"tag,omitempty" yaml:"tag,omitempty"`
	Digest     string `json:"digest,omitempty" yaml:"digest,omitempty"`
	Source     string `json:"source" yaml:"source"`
}

// ImageAnalysis holds the comprehensive analysis results
type ImageAnalysis struct {
	Chart    ChartInfo               `json:"chart" yaml:"chart"`
	Images   []ImageInfo             `json:"images" yaml:"images"`
	Patterns []analyzer.ImagePattern `json:"patterns" yaml:"patterns"`
	Errors   []string                `json:"errors,omitempty" yaml:"errors,omitempty"`
	Skipped  []string                `json:"skipped,omitempty" yaml:"skipped,omitempty"`
}

// InspectFlags holds all the flag values for the inspect command
type InspectFlags struct {
	ChartPath              string
	OutputFormat           string
	OutputFile             string
	GenerateConfigSkeleton bool
	AnalyzerConfig         *analyzer.Config
}

// newInspectCmd creates a new inspect command
func newInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect [flags]",
		Short: "Inspect a Helm chart and analyze container image references",
		Long: "Analyze a Helm chart to identify all container image references, " +
			"including direct string values and structured image definitions. " +
			"Provides detailed information about detected images and their locations within the chart values.",
		Args: cobra.NoArgs,
		RunE: runInspect,
	}

	// Add flags
	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball (default: auto-detect)")
	cmd.Flags().StringP("output-format", "f", "yaml", "Output format: yaml")
	cmd.Flags().StringP("output-file", "o", "", "Output file path (default: stdout)")
	cmd.Flags().Bool("generate-config-skeleton", false, "Generate a configuration skeleton based on analysis")

	// Analysis control flags
	cmd.Flags().StringSlice("include-pattern", nil, "Glob patterns for values paths to include during analysis")
	cmd.Flags().StringSlice("exclude-pattern", nil, "Glob patterns for values paths to exclude during analysis")
	cmd.Flags().StringSlice("known-image-paths", nil, "Specific dot-notation paths known to contain images")

	return cmd
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
	_, err = os.Stat(chartPath)
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

	return &InspectFlags{
		ChartPath:              chartPath,
		OutputFormat:           outputFormat,
		OutputFile:             outputFile,
		GenerateConfigSkeleton: generateConfigSkeleton,
		AnalyzerConfig:         analyzerConfig,
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
	// Check if Chart.yaml exists in current directory
	if _, err := os.Stat("Chart.yaml"); err == nil {
		// Current directory is a chart
		currentDir, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		return currentDir, nil
	}

	// Check if there's a chart directory
	entries, err := os.ReadDir(".")
	if err != nil {
		return "", fmt.Errorf("failed to read current directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Check if the directory contains Chart.yaml
			chartFile := filepath.Join(entry.Name(), "Chart.yaml")
			if _, err := os.Stat(chartFile); err == nil {
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

	// Write to file or stdout
	if outputFile != "" {
		err := os.WriteFile(outputFile, []byte(yamlWithComments), SecureFilePerms)
		if err != nil {
			return fmt.Errorf("failed to write config skeleton to file: %w", err)
		}
		log.Infof("Configuration skeleton written to %s", outputFile)
	} else {
		fmt.Println(yamlWithComments)
	}

	return nil
}

// runInspect implements the inspect command logic
func runInspect(cmd *cobra.Command, _ []string) error {
	flags, err := getInspectFlags(cmd)
	if err != nil {
		return err
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

	// Create and initialize the analyzer
	chartLoader := &chart.DefaultLoader{}
	chartData, err := chartLoader.Load(flags.ChartPath)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed,
			Err:  fmt.Errorf("failed to initialize analyzer: %w", err),
		}
	}

	// Extract chart info
	chartInfo := ChartInfo{
		Name:         chartData.Metadata.Name,
		Version:      chartData.Metadata.Version,
		Path:         flags.ChartPath,
		Dependencies: len(chartData.Dependencies()),
	}

	// Analyze chart values
	patterns, err := analyzer.AnalyzeHelmValues(chartData.Values, flags.AnalyzerConfig)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("chart analysis failed: %w", err),
		}
	}

	// Process image patterns
	images, skipped := processImagePatterns(patterns)

	// Create analysis result
	analysis := ImageAnalysis{
		Chart:    chartInfo,
		Images:   images,
		Patterns: patterns,
		Skipped:  skipped,
	}

	// Handle generate-config-skeleton flag
	if flags.GenerateConfigSkeleton {
		skeletonFile := flags.OutputFile
		if skeletonFile == "" {
			skeletonFile = "irr-config.yaml"
		}
		err := createConfigSkeleton(images, skeletonFile)
		if err != nil {
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
		err := os.WriteFile(flags.OutputFile, output, SecureFilePerms)
		if err != nil {
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
