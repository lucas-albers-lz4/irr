// Package main contains the implementation for the irr CLI, including subcommands like inspect.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
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
	Registry         string `json:"registry" yaml:"registry"`                                     // The registry detected (might be default)
	Repository       string `json:"repository" yaml:"repository"`                                 // The repository path
	Tag              string `json:"tag,omitempty" yaml:"tag,omitempty"`                           // The tag, if present
	Digest           string `json:"digest,omitempty" yaml:"digest,omitempty"`                     // The digest, if present
	Source           string `json:"source" yaml:"source"`                                         // The dot-notation path in values where found
	OriginalRegistry string `json:"originalRegistry,omitempty" yaml:"originalRegistry,omitempty"` // Added: Original registry from source if different
	ValuePath        string `json:"valuePath,omitempty" yaml:"valuePath,omitempty"`               // Added: Full path from context-aware analysis
}

// ImageAnalysis represents the result of analyzing a chart for images
type ImageAnalysis struct {
	Chart         ChartInfo               `json:"chart" yaml:"chart"`
	Images        []ImageInfo             `json:"images" yaml:"images"`
	ImagePatterns []analysis.ImagePattern `json:"imagePatterns" yaml:"imagePatterns"`
	Errors        []string                `json:"errors,omitempty" yaml:"errors,omitempty"`
	Skipped       []string                `json:"skipped,omitempty" yaml:"skipped,omitempty"`
}

// InspectFlags holds the command line flags for the inspect command
type InspectFlags struct {
	ChartPath              string
	OutputFile             string
	OutputFormat           string
	GenerateConfigSkeleton bool
	AnalyzerConfig         *analysis.Config
	SourceRegistries       []string
	AllNamespaces          bool
	OverwriteSkeleton      bool
	NoSubchartCheck        bool
}

const (
	// DefaultConfigSkeletonFilename is the default filename for the generated config skeleton
	DefaultConfigSkeletonFilename = "registry-mappings.yaml"
	outputFormatYAML              = "yaml"
	outputFormatJSON              = "json"
	defaultNamespace              = "default" // Added const for default namespace
	sliceGrowthBuffer             = 10        // Buffer size for growing slices
)

// ReleaseAnalysisResult represents the analysis result for a single Helm release
type ReleaseAnalysisResult struct {
	ReleaseName string        `json:"releaseName" yaml:"releaseName"`
	Namespace   string        `json:"namespace" yaml:"namespace"`
	Analysis    ImageAnalysis `json:"analysis" yaml:"analysis"`
}

// newInspectCmd creates a new inspect command
func newInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect [release-name]",
		Short: "Inspect a Helm chart for image references",
		Long: `Inspect a Helm chart to find all image references.
This command analyzes the chart's values.yaml and templates to find image references.
It properly handles subcharts and dependency values according to Helm's value merging rules.`,
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
	cmd.Flags().Bool("no-subchart-check", false, "Skip checking for subchart image discrepancies")

	// Add Helm flags
	cmd.Flags().StringSlice("values", nil, "Values files to process (can be specified multiple times)")
	cmd.Flags().StringSlice("set", nil, "Set values on the command line (can be specified multiple times)")
	cmd.Flags().StringSlice("set-string", nil, "Set STRING values on the command line (can be specified multiple times)")
	cmd.Flags().StringSlice("set-file", nil, "Set values from files (can be specified multiple times)")

	// Added new flags
	cmd.Flags().Bool("context-aware", false, "Use context-aware analyzer that handles subchart value merging (experimental)")

	return cmd
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

	// Decide execution path based on args/plugin mode
	if releaseNameProvided {
		// Assume plugin mode if release name is given
		releaseName = args[0] // Assign releaseName here

		// --- Namespace Handling for Plugin Mode ---
		// Get namespace primarily from Helm's environment/settings when running as a plugin.
		// Fall back to the flag if not found or not in plugin mode (though this path assumes plugin mode).
		var namespace string
		var nsErr error

		// Check HELM_NAMESPACE env var first (common for plugins)
		helmNamespaceEnv := os.Getenv("HELM_NAMESPACE")
		if helmNamespaceEnv != "" {
			namespace = helmNamespaceEnv
			log.Debug("Using namespace from HELM_NAMESPACE env var", "namespace", namespace)
		} else {
			// Fallback to the flag defined by irr (might be set if not using helm irr ...)
			namespace, nsErr = cmd.Flags().GetString("namespace")
			if nsErr != nil {
				return &exitcodes.ExitCodeError{Code: exitcodes.ExitInputConfigurationError, Err: fmt.Errorf("failed to get namespace flag: %w", nsErr)}
			}
			log.Debug("Using namespace from --namespace flag", "namespace", namespace)
		}

		// Default if still empty
		if namespace == "" {
			namespace = defaultNamespace // Use constant
			log.Debug("Namespace defaulted", "namespace", namespace)
		}
		// --- End Namespace Handling ---

		return inspectHelmRelease(cmd, flags, releaseName, namespace)
	}

	// Standalone mode (no release name)
	chartPath, analysisResult, err := setupAnalyzerAndLoadChart(cmd, flags) // Pass AppFs here
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
		filterImagesBySourceRegistries(cmd, flags, analysisResult) // Modifies analysis in place
	}

	// Perform subchart check if not explicitly disabled
	if !flags.NoSubchartCheck && chartPath != "" {
		// Check for subchart discrepancies
		if err := checkSubchartDiscrepancy(cmd, chartPath, analysisResult); err != nil {
			// Just log the error, don't fail the command
			log.Warn("Failed to check for subchart discrepancies: %s", err)
		}
	}

	// --- Informational Output (Moved Before writeOutput) ---
	//nolint:gocritic // ifElseChain: Keeping if-else for clarity over switch here.
	if !flags.GenerateConfigSkeleton && flags.OutputFile == "" { // Only show suggestions when printing to stdout
		// Log the successful analysis (using the logger now)
		log.Info("Successfully loaded and analyzed chart", "path", chartPath)

		// Extract unique registries from the potentially filtered analysis.
		uniqueRegistries := extractUniqueRegistries(analysisResult.Images)

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
	if err := writeOutput(cmd, analysisResult, flags); err != nil {
		return err // Return error with exit code from writeOutput
	}

	return nil
}

// getInspectFlags retrieves and validates flags for the inspect command
func getInspectFlags(cmd *cobra.Command, releaseNameProvided bool) (*InspectFlags, error) {
	flags := &InspectFlags{}

	// Get chart path from --chart-path flag
	var err error
	flags.ChartPath, err = cmd.Flags().GetString("chart-path")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
		}
	}

	// Get output file path from --output-file flag
	flags.OutputFile, err = cmd.Flags().GetString("output-file")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-file flag: %w", err),
		}
	}

	// Get output format from --output-format flag
	flags.OutputFormat, err = cmd.Flags().GetString("output-format")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-format flag: %w", err),
		}
	}

	// Validate output format is supported
	if flags.OutputFormat != outputFormatYAML && flags.OutputFormat != outputFormatJSON {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("unsupported output format %q; supported formats: %s, %s", flags.OutputFormat, outputFormatYAML, outputFormatJSON),
		}
	}

	// Get generate-config-skeleton flag
	flags.GenerateConfigSkeleton, err = cmd.Flags().GetBool("generate-config-skeleton")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get generate-config-skeleton flag: %w", err),
		}
	}

	// Get overwrite-skeleton flag
	flags.OverwriteSkeleton, err = cmd.Flags().GetBool("overwrite-skeleton")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get overwrite-skeleton flag: %w", err),
		}
	}

	// Get no-subchart-check flag
	flags.NoSubchartCheck, err = cmd.Flags().GetBool("no-subchart-check")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get no-subchart-check flag: %w", err),
		}
	}

	// Get all-namespaces flag
	flags.AllNamespaces, err = cmd.Flags().GetBool("all-namespaces")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get all-namespaces flag: %w", err),
		}
	}

	// Validate conflicts with all-namespaces
	if flags.AllNamespaces {
		if flags.ChartPath != "" {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  errors.New("--all-namespaces cannot be used with --chart-path"),
			}
		}
		// If release name was provided, flag conflict
		if releaseNameProvided {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  errors.New("--all-namespaces cannot be used with a release name"),
			}
		}
		// Check if --namespace was explicitly set (if it's not default)
		namespace, nsErr := cmd.Flags().GetString("namespace")
		if nsErr == nil && namespace != defaultNamespace && namespace != "" {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  errors.New("--all-namespaces cannot be used with --namespace"),
			}
		}
	}

	// Validate output file path now to avoid later issues
	if flags.OutputFile != "" {
		// Check if directory exists
		outDir := filepath.Dir(flags.OutputFile)
		if stat, err := os.Stat(outDir); err != nil || !stat.IsDir() {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("output directory %q does not exist or is not a directory", outDir),
			}
		}

		// Check if output file is writable (or can be created)
		// Case 1: File exists - check if we can write to it
		if stat, err := os.Stat(flags.OutputFile); err == nil {
			if flags.GenerateConfigSkeleton && !flags.OverwriteSkeleton {
				return nil, &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("skeleton file %q already exists; use --overwrite-skeleton to overwrite", flags.OutputFile),
				}
			}
			// Check if it's a regular file
			if !stat.Mode().IsRegular() {
				return nil, &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("output path %q exists but is not a regular file", flags.OutputFile),
				}
			}
			// Check write permission (attempt to open for writing)
			f, err := os.OpenFile(flags.OutputFile, os.O_WRONLY, 0)
			if err != nil {
				return nil, &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("cannot write to output file %q: %w", flags.OutputFile, err),
				}
			}
			if err := f.Close(); err != nil {
				log.Warn("Error closing file after permission check", "error", err)
			}
		}
		// Case 2: File doesn't exist - check if we can create it
		// Attempt to create and then remove the file
		f, err := os.OpenFile(flags.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileutil.ReadWriteUserReadOthers)
		if err != nil {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("cannot create output file %q: %w", flags.OutputFile, err),
			}
		}
		if err := f.Close(); err != nil {
			log.Warn("Error closing temporary file", "error", err)
		}
		// Only remove the file if it didn't exist before
		if _, err := os.Stat(flags.OutputFile); err == nil {
			if err := os.Remove(flags.OutputFile); err != nil {
				log.Warn("Failed to remove temporary file", "path", flags.OutputFile, "error", err)
			}
		}
	}

	// Get the analyzer config with include/exclude patterns
	includePatterns, excludePatterns, err := getAnalysisPatterns(cmd)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get analysis patterns: %w", err),
		}
	}

	config := &analysis.Config{
		IncludePatterns: includePatterns,
		ExcludePatterns: excludePatterns,
	}
	flags.AnalyzerConfig = config

	// Get source registries
	sourceRegistries, err := cmd.Flags().GetStringSlice("source-registries")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get source-registries flag: %w", err),
		}
	}
	flags.SourceRegistries = sourceRegistries

	// Return the extracted flags
	return flags, nil
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
