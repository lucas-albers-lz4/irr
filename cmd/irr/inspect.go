package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lalbers/irr/pkg/analyzer"
	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/lalbers/irr/pkg/image"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball (required)")
	cmd.Flags().StringP("output-format", "f", "yaml", "Output format: yaml")
	cmd.Flags().StringP("output-file", "o", "", "Output file path (default: stdout)")
	cmd.Flags().Bool("generate-config-skeleton", false, "Generate a configuration skeleton based on analysis")

	// Analysis control flags
	cmd.Flags().StringSlice("include-pattern", nil, "Glob patterns for values paths to include during analysis")
	cmd.Flags().StringSlice("exclude-pattern", nil, "Glob patterns for values paths to exclude during analysis")
	cmd.Flags().StringSlice("known-image-paths", nil, "Specific dot-notation paths known to contain images")

	// Mark required flags
	mustMarkFlagRequired(cmd, "chart-path")

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

// createConfigSkeleton creates a config skeleton based on detected images
func createConfigSkeleton(images []ImageInfo, outputFile string) error {
	registries := make(map[string]string)
	for _, img := range images {
		if img.Registry != "" {
			registries[img.Registry] = ""
		}
	}

	configSkeleton := map[string]interface{}{
		"version":           1,
		"target_registry":   "YOUR_TARGET_REGISTRY",
		"registry_mappings": registries,
	}

	skeletonBytes, err := yaml.Marshal(configSkeleton)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal config skeleton: %w", err),
		}
	}

	// Either write to specified output file or stdout
	if outputFile != "" {
		if err := os.WriteFile(outputFile+".skeleton.yaml", skeletonBytes, fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write config skeleton to file: %w", err),
			}
		}
		log.Infof("Config skeleton written to %s.skeleton.yaml", outputFile)
	} else {
		fmt.Println("\n--- CONFIG SKELETON ---")
		fmt.Println(string(skeletonBytes))
	}

	return nil
}

// runInspect implements the inspect command logic
func runInspect(cmd *cobra.Command, _ []string) error {
	// Get flags and configuration
	flags, err := getInspectFlags(cmd)
	if err != nil {
		return err
	}

	// Load chart
	log.Infof("Loading chart from %s", flags.ChartPath)
	chartLoader := &chart.GeneratorLoader{}
	chartData, err := chartLoader.Load(flags.ChartPath)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed,
			Err:  fmt.Errorf("failed to load chart: %w", err),
		}
	}

	// Create analysis result
	chartInfo := ChartInfo{
		Name:         chartData.Name(),
		Version:      chartData.Metadata.Version,
		Path:         flags.ChartPath,
		Dependencies: len(chartData.Dependencies()),
	}

	// Analyze chart values
	patterns, err := analyzer.AnalyzeHelmValues(chartData.Values, flags.AnalyzerConfig)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("failed to analyze chart values: %w", err),
		}
	}

	// Process image information
	images, skipped := processImagePatterns(patterns)
	var errors []string

	// Create analysis result
	analysis := ImageAnalysis{
		Chart:    chartInfo,
		Images:   images,
		Patterns: patterns,
		Errors:   errors,
		Skipped:  skipped,
	}

	// Format output
	var output []byte
	if flags.OutputFormat == "yaml" {
		output, err = yaml.Marshal(analysis)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to YAML: %w", err),
			}
		}
	}

	// Generate config skeleton if requested
	if flags.GenerateConfigSkeleton {
		if err := createConfigSkeleton(images, flags.OutputFile); err != nil {
			return err
		}
	}

	// Write output to file or stdout
	if flags.OutputFile != "" {
		if err := os.WriteFile(flags.OutputFile, output, fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write output to file: %w", err),
			}
		}
		log.Infof("Analysis written to %s", flags.OutputFile)
	} else {
		fmt.Println(string(output))
	}

	return nil
}
