// Package main provides CLI commands for the irr tool.
//
// IMPORTANT: This file imports Helm SDK packages that require additional dependencies.
// To resolve the missing go.sum entries, run:
//
//	go get helm.sh/helm/v3@v3.14.2
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/exitcodes"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	// Helm SDK imports
	helmAction "helm.sh/helm/v3/pkg/action"
	helmCli "helm.sh/helm/v3/pkg/cli"
)

const (
	// DirPermissions represents directory permissions (rwxr-xr-x)
	DirPermissions = 0o755
	// FilePermissions represents file permissions (rw-r--r--)
	FilePermissions = 0o644
	// ExitHelmInteractionError is returned when there's an error during Helm SDK interaction
	ExitHelmInteractionError = 17
	// ExitInternalError is returned when there's an internal error in command execution
	ExitInternalError = 30
)

// Variables for testing
var (
	isTestMode = false
)

// GeneratorConfig holds all configuration for the generator
type GeneratorConfig struct {
	// ChartPath is the path to the Helm chart directory or archive
	ChartPath string
	// TargetRegistry is the target container registry URL
	TargetRegistry string
	// SourceRegistries is a list of source container registry URLs to relocate
	SourceRegistries []string
	// ExcludeRegistries is a list of container registry URLs to exclude from relocation
	ExcludeRegistries []string
	// Strategy is the path generation strategy to use for image paths
	Strategy strategy.PathStrategy
	// Mappings contains registry mapping configurations
	Mappings *registry.Mappings
	// ConfigMappings contains registry mappings from the --config flag
	ConfigMappings map[string]string
	// StrictMode enables strict validation (fails on any error)
	StrictMode bool
	// Threshold is the minimum percentage of images that must be processed successfully
	Threshold int
	// IncludePatterns contains glob patterns for values paths to include
	IncludePatterns []string
	// ExcludePatterns contains glob patterns for values paths to exclude
	ExcludePatterns []string
	// KnownImagePaths contains specific dot-notation paths known to contain images
	KnownImagePaths []string
	// RulesEnabled controls whether the chart parameter rules system is enabled
	RulesEnabled bool
}

// For testing purposes - allows overriding in tests
var chartLoader = loadChart

// OverrideFlags defines the flags used by the override command
type OverrideFlags struct {
	ChartPath         string
	ReleaseName       string
	Namespace         string
	TargetRegistry    string
	SourceRegistries  []string
	ExcludeRegistries []string
	OutputFile        string
	StrategyName      string
	ConfigFile        string
	StrictMode        bool
	Threshold         int
	IncludePatterns   []string
	ExcludePatterns   []string
	KnownImagePaths   []string
	DisableRules      bool
	DryRun            bool
	Validate          bool
}

// newOverrideCmd creates the cobra command for the 'override' operation.
// This command uses centralized exit codes from pkg/exitcodes for consistent error handling:
// - Input validation failures return codes 1-9 (e.g., ExitMissingRequiredFlag)
// - Chart processing issues return codes 10-19 (e.g., ExitUnsupportedStructure)
// - Runtime/system errors return codes 20-29 (e.g., ExitGeneralRuntimeError)
func newOverrideCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "override [release-name]",
		Short: "Analyzes a Helm chart and generates image override values",
		Long: "Analyzes a Helm chart to find all container image references (both direct string values " +
			"and map-based structures like 'image.repository', 'image.tag'). It then generates a " +
			"Helm-compatible values file that overrides these references to point to a specified " +
			"target registry, using a defined path strategy.\n\n" +
			"Supports filtering images based on source registries and excluding specific registries. " +
			"Can also utilize a registry mapping file for more complex source-to-target mappings." +
			"Includes options for dry-run, strict validation, and success thresholds.",
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Check if we're in plugin mode with a release name
			hasReleaseName := len(args) > 0 && isHelmPlugin
			inTestMode := isTestMode && isHelmPlugin

			// Only check for required flags if not in plugin mode with a release name
			// AND not in test mode
			if !hasReleaseName && !inTestMode {
				// Check required flags
				var missingFlags []string

				chartPath, err := cmd.Flags().GetString("chart-path")
				if err != nil || chartPath == "" {
					missingFlags = append(missingFlags, "chart-path")
				}

				targetRegistry, err := cmd.Flags().GetString("target-registry")
				if err != nil || targetRegistry == "" {
					missingFlags = append(missingFlags, "target-registry")
				}

				sourceRegistries, err := cmd.Flags().GetStringSlice("source-registries")
				if err != nil || len(sourceRegistries) == 0 {
					missingFlags = append(missingFlags, "source-registries")
				}

				// Ensure --target-registry is provided even when --config is used
				configPath, configErr := cmd.Flags().GetString("config")
				if configErr != nil {
					return &exitcodes.ExitCodeError{
						Code: exitcodes.ExitInputConfigurationError,
						Err:  fmt.Errorf("failed to get config flag: %w", configErr),
					}
				}
				if configPath != "" && (err != nil || targetRegistry == "") {
					missingFlags = append(missingFlags, "target-registry")
				}

				if len(missingFlags) > 0 {
					sort.Strings(missingFlags) // Sort for consistent error message
					return &exitcodes.ExitCodeError{
						Code: exitcodes.ExitMissingRequiredFlag,
						Err:  fmt.Errorf("required flag(s) \"%s\" not set", strings.Join(missingFlags, "\", \"")),
					}
				}
			} else {
				// Even in plugin mode, always require target-registry
				targetRegistry, err := cmd.Flags().GetString("target-registry")
				if err != nil || targetRegistry == "" {
					return &exitcodes.ExitCodeError{
						Code: exitcodes.ExitMissingRequiredFlag,
						Err:  fmt.Errorf("required flag(s) \"target-registry\" not set"),
					}
				}

				// Always require source-registries
				sourceRegistries, err := cmd.Flags().GetStringSlice("source-registries")
				if err != nil || len(sourceRegistries) == 0 {
					return &exitcodes.ExitCodeError{
						Code: exitcodes.ExitMissingRequiredFlag,
						Err:  fmt.Errorf("required flag(s) \"source-registries\" not set"),
					}
				}
			}

			return nil
		},
		RunE: runOverride,
	}

	// Set up flags
	setupOverrideFlags(cmd)

	return cmd
}

// setupOverrideFlags configures all flags for the override command
func setupOverrideFlags(cmd *cobra.Command) {
	// Required flags
	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball (default: auto-detect)")
	cmd.Flags().StringP("target-registry", "t", "", "Target container registry URL (required)")
	cmd.Flags().StringSliceP(
		"source-registries",
		"s",
		[]string{},
		"Source container registry URLs to relocate (required, comma-separated or multiple flags)",
	)

	// Optional flags with defaults
	cmd.Flags().StringP("output-file", "o", "", "Output file path for the generated overrides YAML (default: stdout)")
	cmd.Flags().StringP("strategy", "p", "prefix-source-registry", "Path generation strategy ('prefix-source-registry')")
	cmd.Flags().Bool("dry-run", false, "Perform analysis and print overrides to stdout without writing to file")
	cmd.Flags().Bool("strict", false, "Enable strict mode (fail on any image parsing/processing error)")
	cmd.Flags().StringSlice(
		"exclude-registries",
		[]string{},
		"Container registry URLs to exclude from relocation (comma-separated or multiple flags)",
	)
	cmd.Flags().Int("threshold", 0, "Minimum percentage of images successfully processed for the command to succeed (0-100, 0 disables)")
	cmd.Flags().String("registry-file", "", "Path to a YAML file containing registry mappings (source: target)")
	cmd.Flags().String("config", "", "Path to a YAML configuration file for registry mappings (map[string]string format)")
	cmd.Flags().Bool("validate", false, "Run 'helm template' with generated overrides to validate chart renderability")
	cmd.Flags().StringP("release-name", "n", "", "Helm release name to get values from before generating overrides (optional)")
	cmd.Flags().String("namespace", "", "Kubernetes namespace for the Helm release (only used with --release-name)")
	cmd.Flags().Bool("disable-rules", false, "Disable the chart parameter rules system (default: enabled)")

	// Analysis control flags
	cmd.Flags().StringSlice("include-pattern", nil, "Glob patterns for values paths to include during analysis")
	cmd.Flags().StringSlice("exclude-pattern", nil, "Glob patterns for values paths to exclude during analysis")
	cmd.Flags().StringSlice("known-image-paths", nil, "Specific dot-notation paths known to contain images")

	// Mark required flags
	for _, flag := range []string{"target-registry", "source-registries"} {
		if err := cmd.MarkFlagRequired(flag); err != nil {
			panic(fmt.Sprintf("failed to mark flag '%s' as required: %v", flag, err))
		}
	}
}

// getRequiredFlags retrieves and validates the required flags for the override command
func getRequiredFlags(cmd *cobra.Command) (chartPath, targetRegistry string, sourceRegistries []string, err error) {
	chartPath, err = cmd.Flags().GetString("chart-path")
	if err != nil || chartPath == "" {
		return "", "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("required flag(s) \"chart-path\" not set"),
		}
	}

	targetRegistry, err = cmd.Flags().GetString("target-registry")
	if err != nil || targetRegistry == "" {
		return "", "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("required flag(s) \"target-registry\" not set"),
		}
	}

	sourceRegistries, err = cmd.Flags().GetStringSlice("source-registries")
	if err != nil || len(sourceRegistries) == 0 {
		return "", "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("required flag(s) \"source-registries\" not set"),
		}
	}

	return chartPath, targetRegistry, sourceRegistries, nil
}

// getStringFlag retrieves a string flag value from the command
func getStringFlag(cmd *cobra.Command, flagName string) (string, error) {
	value, err := cmd.Flags().GetString(flagName)
	if err != nil {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get %s flag: %w", flagName, err),
		}
	}
	return value, nil
}

// getBoolFlag retrieves a boolean flag value from the command
func getBoolFlag(cmd *cobra.Command, flagName string) (bool, error) {
	value, err := cmd.Flags().GetBool(flagName)
	if err != nil {
		return false, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get %s flag: %w", flagName, err),
		}
	}
	return value, nil
}

// getStringSliceFlag retrieves a string slice flag value from the command
func getStringSliceFlag(cmd *cobra.Command, flagName string) ([]string, error) {
	value, err := cmd.Flags().GetStringSlice(flagName)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get %s flag: %w", flagName, err),
		}
	}
	return value, nil
}

// getThresholdFlag retrieves and validates the threshold flag
func getThresholdFlag(cmd *cobra.Command) (int, error) {
	threshold, err := cmd.Flags().GetInt("threshold")
	if err != nil {
		return 0, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get threshold flag: %w", err),
		}
	}

	if threshold < 0 || threshold > 100 {
		return 0, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("threshold must be between 0 and 100: invalid threshold value: %d", threshold),
		}
	}

	return threshold, nil
}

// handleGenerateError converts generator errors to appropriate exit code errors
func handleGenerateError(err error) error {
	switch {
	case errors.Is(err, strategy.ErrThresholdExceeded):
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitThresholdError,
			Err:  fmt.Errorf("failed to process chart: %w", err),
		}
	case errors.Is(err, chart.ErrChartNotFound) || errors.Is(err, chart.ErrChartLoadFailed):
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartParsingError,
			Err:  fmt.Errorf("failed to process chart: %w", err),
		}
	case errors.Is(err, chart.ErrUnsupportedStructure):
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitUnsupportedStructure,
			Err:  fmt.Errorf("failed to process chart: %w", err),
		}
	default:
		// Default to image processing error for any other errors
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitImageProcessingError,
			Err:  fmt.Errorf("failed to process chart: %w", err),
		}
	}
}

// outputOverrides handles the output of override data based on flags
func outputOverrides(cmd *cobra.Command, yamlBytes []byte, outputFile string, dryRun bool) error {
	if dryRun {
		// Dry run mode - output to stdout with headers
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- Dry Run: Generated Overrides ---"); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write dry run header: %w", err),
			}
		}

		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(yamlBytes)); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write overrides in dry run mode: %w", err),
			}
		}

		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- End Dry Run ---"); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write dry run footer: %w", err),
			}
		}

		return nil
	}

	// Output mode - write to a file
	if outputFile != "" {
		// Create the directory if it doesn't exist
		err := AppFs.MkdirAll(filepath.Dir(outputFile), DirPermissions)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to create output directory: %w", err),
			}
		}

		// Write the file
		err = afero.WriteFile(AppFs, outputFile, yamlBytes, FilePermissions)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write override file: %w", err),
			}
		}

		// Check error from Fprintf to satisfy the linter
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Successfully wrote overrides to %s\n", outputFile); err != nil {
			// We've already written the file successfully, so just log this error
			debug.Printf("Warning: Error printing success message: %v", err)
		}
		return nil
	}

	// Just output to stdout
	_, err := fmt.Fprintln(cmd.OutOrStdout(), string(yamlBytes))
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to write overrides to stdout: %w", err),
		}
	}

	return nil
}

// setupGeneratorConfig collects all the necessary configuration for the generator
func setupGeneratorConfig(cmd *cobra.Command, releaseName string) (config GeneratorConfig, err error) {
	// Check if we have a release name from positional args
	releaseNameProvided := releaseName != "" && isHelmPlugin

	// If running in plugin mode with a release name, don't require chart-path
	if releaseNameProvided {
		// Only get target registry and source registries, chart path is optional
		config.TargetRegistry, err = cmd.Flags().GetString("target-registry")
		if err != nil || config.TargetRegistry == "" {
			err = &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  errors.New("required flag(s) \"target-registry\" not set"),
			}
			return
		}

		config.SourceRegistries, err = cmd.Flags().GetStringSlice("source-registries")
		if err != nil || len(config.SourceRegistries) == 0 {
			err = &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  errors.New("required flag(s) \"source-registries\" not set"),
			}
			return
		}

		// Get chart path if provided (optional in this case)
		config.ChartPath, err = cmd.Flags().GetString("chart-path")
		if err != nil {
			err = &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
			}
			return
		}
	} else {
		// Use existing getRequiredFlags for normal mode (requires chart-path)
		config.ChartPath, config.TargetRegistry, config.SourceRegistries, err = getRequiredFlags(cmd)
		if err != nil {
			return
		}
	}

	// Get registry file and mappings
	registryFile, err := getStringFlag(cmd, "registry-file")
	if err != nil {
		return
	}

	// Load registry mappings
	if registryFile != "" {
		config.Mappings, err = registry.LoadMappings(AppFs, registryFile, integrationTestMode)
		if err != nil {
			debug.Printf("Failed to load mappings: %v", err)
			err = fmt.Errorf("failed to load registry mappings from %s: %w", registryFile, err)
			return
		}
		debug.Printf("Successfully loaded %d mappings from %s", len(config.Mappings.Entries), registryFile)
	}

	// Get config file path
	configFile, err := getStringFlag(cmd, "config")
	if err != nil {
		return
	}

	// Load config mappings
	if configFile != "" {
		config.ConfigMappings, err = registry.LoadConfig(AppFs, configFile, integrationTestMode)
		if err != nil {
			debug.Printf("Failed to load config: %v", err)
			err = &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to load registry config from %s: %w", configFile, err),
			}
			return
		}

		if config.ConfigMappings != nil {
			debug.Printf("Successfully loaded %d config mappings from %s", len(config.ConfigMappings), configFile)
		}
	}

	// Get and validate path strategy
	pathStrategyString, err := getStringFlag(cmd, "strategy")
	if err != nil {
		return
	}

	// Validate strategy
	config.Strategy, err = strategy.GetStrategy(pathStrategyString, config.Mappings)
	if err != nil {
		err = &exitcodes.ExitCodeError{
			Code: exitcodes.ExitCodeInvalidStrategy,
			Err:  fmt.Errorf("invalid path strategy specified: %s: %w", pathStrategyString, err),
		}
		return
	}

	// Get remaining flags
	config.ExcludeRegistries, err = getStringSliceFlag(cmd, "exclude-registries")
	if err != nil {
		return
	}

	config.Threshold, err = getThresholdFlag(cmd)
	if err != nil {
		return
	}

	config.StrictMode, err = getBoolFlag(cmd, "strict")
	if err != nil {
		return
	}

	// Get rules enabled flag
	disableRules, err := getBoolFlag(cmd, "disable-rules")
	if err != nil {
		return
	}
	config.RulesEnabled = !disableRules

	// Get analysis control flags
	config.IncludePatterns, err = getStringSliceFlag(cmd, "include-pattern")
	if err != nil {
		return
	}

	config.ExcludePatterns, err = getStringSliceFlag(cmd, "exclude-pattern")
	if err != nil {
		return
	}

	config.KnownImagePaths, err = getStringSliceFlag(cmd, "known-image-paths")
	if err != nil {
		return
	}

	return
}

// createAndExecuteGenerator creates a generator with the given config and executes it to generate overrides
func createAndExecuteGenerator(chartSource string, config *GeneratorConfig) ([]byte, error) {
	// --- Create Override Generator ---
	log.Infof("Initializing override generator for %s", chartSource)
	generator := chart.NewGenerator(
		config.ChartPath,
		config.TargetRegistry,
		config.SourceRegistries,
		config.ExcludeRegistries,
		config.Strategy,
		config.Mappings,
		config.ConfigMappings,
		config.StrictMode,
		config.Threshold,
		nil, // Use default loader
		config.IncludePatterns,
		config.ExcludePatterns,
		config.KnownImagePaths,
	)

	// Configure rules system
	generator.SetRulesEnabled(config.RulesEnabled)
	if !config.RulesEnabled {
		log.Infof("Chart parameter rules system is disabled")
	}

	// Generate overrides
	overrideFile, err := generator.Generate()
	if err != nil {
		return nil, handleGenerateError(err) // Use existing error handler
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(overrideFile.Values)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal overrides to YAML: %w", err),
		}
	}

	return yamlBytes, nil
}

// loadChart loads a chart from either a release or a path
func loadChart(config *GeneratorConfig, loadFromRelease, loadFromPath bool, releaseName, namespace string) (string, error) {
	var chartSource string
	var releaseValuesCacheDir string

	switch {
	case loadFromRelease:
		// Get chart and values from Helm release
		log.Infof("Loading chart and values from release '%s'", releaseName)
		chartSource = fmt.Sprintf("Helm release '%s'", releaseName)

		// Fetch the release using Helm SDK with proper context
		helmSettings := helmCli.New()

		actionConfig := new(helmAction.Configuration)
		// Get values from release
		// Use debugf as the log function for Helm SDK
		debugf := func(format string, v ...interface{}) {
			log.Debugf(format, v...)
		}
		if err := actionConfig.Init(helmSettings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), debugf); err != nil {
			return "", &exitcodes.ExitCodeError{
				Code: ExitHelmInteractionError,
				Err:  fmt.Errorf("failed to initialize Helm configuration: %w", err),
			}
		}

		// Create Get client to get the release
		client := helmAction.NewGet(actionConfig)
		release, err := client.Run(releaseName)
		if err != nil {
			return "", &exitcodes.ExitCodeError{
				Code: ExitHelmInteractionError,
				Err:  fmt.Errorf("failed to get release '%s': %w", releaseName, err),
			}
		}

		// Create a temporary directory to store chart files from release
		releaseValuesCacheDir, err = afero.TempDir(AppFs, "", "irr-release-cache-")
		if err != nil {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to create temporary directory for release cache: %w", err),
			}
		}

		// Save chart path for the generator to use
		config.ChartPath = releaseValuesCacheDir

		// Use these values when initializing the generator
		if release.Chart.Values == nil {
			release.Chart.Values = release.Config
		}

		log.Infof("Successfully retrieved chart and values from release '%s'", releaseName)

	case loadFromPath:
		// Normalize and check chart path (moved from getInspectFlags logic for consistency)
		if config.ChartPath == "" {
			// Try to detect chart if path is empty (might be redundant with PreRunE checks)
			detectedPath, err := detectChartInCurrentDirectoryIfNeeded("")
			if err != nil {
				return "", &exitcodes.ExitCodeError{
					Code: exitcodes.ExitChartNotFound,
					Err:  fmt.Errorf("chart path not specified and %w", err),
				}
			}
			config.ChartPath = detectedPath
			log.Infof("Detected chart at %s", config.ChartPath)
		}

		// Make path absolute
		absPath, err := filepath.Abs(config.ChartPath)
		if err != nil {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to get absolute path for chart: %w", err),
			}
		}
		config.ChartPath = absPath // Use absolute path going forward

		// Check existence
		if _, err := AppFs.Stat(config.ChartPath); err != nil {
			if os.IsNotExist(err) {
				return "", &exitcodes.ExitCodeError{
					Code: exitcodes.ExitChartNotFound,
					Err:  fmt.Errorf("chart path not found: %s", config.ChartPath),
				}
			}
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to access chart path %s: %w", config.ChartPath, err),
			}
		}

		log.Infof("Loading chart from path: %s", config.ChartPath)
		chartSource = fmt.Sprintf("path %s", config.ChartPath)

		// Use our package's chart loading functionality with proper constructor
		chartLoader := chart.NewDefaultLoader(nil)
		// Note: we're not storing loadedChart since it's not used later
		_, err = chartLoader.Load(config.ChartPath)
		if err != nil {
			return "", handleChartLoadError(err, config.ChartPath)
		}
	default:
		// This case should be caught by PreRunE, but handle defensively
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("internal error: either chart path or release name should be set"),
		}
	}

	return chartSource, nil
}

// runOverride is the main entry point for the override command
func runOverride(cmd *cobra.Command, args []string) error {
	// Get release name and namespace from args or flags
	releaseName, namespace, err := getReleaseNameAndNamespace(cmd, args)
	if err != nil {
		return err
	}

	// Check if the --release-name flag was explicitly set by the user
	releaseNameFlagSet := cmd.Flags().Changed("release-name")

	// If releaseName flag was explicitly set but we're not in plugin mode, return an error
	if releaseNameFlagSet && !isHelmPlugin {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("the --release-name flag is only available when running as a Helm plugin (helm irr...)"),
		}
	}

	releaseNameProvided := releaseName != "" && isHelmPlugin

	// If releaseName is provided through args, but we're not in plugin mode, return an error about plugin mode
	if releaseName != "" && !isHelmPlugin {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("release name '%s' can only be used when running as a Helm plugin (helm irr...)", releaseName),
		}
	}

	// Skip most of the normal processing if we're in test mode
	if isTestMode {
		return handleTestModeOverride(cmd, releaseName)
	}

	// If in plugin mode with a release name, handle differently
	if releaseNameProvided {
		debug.Printf("Using release name: %s in namespace: %s", releaseName, namespace)

		// Set up config for Helm plugin mode
		config, err := setupGeneratorConfig(cmd, releaseName)
		if err != nil {
			return err
		}

		// Get output flags
		outputFile, dryRun, err := getOutputFlags(cmd, releaseName)
		if err != nil {
			return err
		}

		// Get path strategy
		pathStrategy, err := cmd.Flags().GetString("strategy")
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to get strategy flag: %w", err),
			}
		}

		// Handle Helm plugin override
		return handleHelmPluginOverride(cmd, releaseName, namespace, &config, pathStrategy, outputFile, dryRun)
	}

	// Set up config for normal chart analysis mode
	config, err := setupGeneratorConfig(cmd, "")
	if err != nil {
		return err
	}

	// Validate chart path in non-plugin mode
	chartPath := config.ChartPath
	if chartPath == "" {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitMissingRequiredFlag,
			Err:  errors.New("required flag(s) \"chart-path\" not set"),
		}
	}

	// Log the chart path being used
	log.Infof("Using chart path: %s", chartPath)

	// Load the Helm chart
	chartSource, err := chartLoader(&config, false, true, "", "")
	if err != nil {
		return handleChartLoadError(err, config.ChartPath)
	}

	// Create and execute generator
	yamlBytes, err := createAndExecuteGenerator(chartSource, &config)
	if err != nil {
		return handleGenerateError(err)
	}

	// Get output flags
	outputFile, dryRun, err := getOutputFlags(cmd, "")
	if err != nil {
		return err
	}

	// Validate chart with generated overrides if requested
	shouldValidate, err := cmd.Flags().GetBool("validate")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get validate flag: %w", err),
		}
	}

	if shouldValidate {
		// For standalone mode, we're loading from path not release
		if err := validateChart(cmd, yamlBytes, &config, true, false, releaseName, namespace); err != nil {
			return err
		}
		log.Infof("Validation successful! Chart renders correctly with overrides.")
	}

	// Output the YAML
	return outputOverrides(cmd, yamlBytes, outputFile, dryRun)
}

// getReleaseNameAndNamespace gets the release name and namespace from the command
func getReleaseNameAndNamespace(cmd *cobra.Command, args []string) (releaseName, namespace string, err error) {
	// Use common function to get release name and namespace
	return getReleaseNameAndNamespaceCommon(cmd, args)
}

// getOutputFlags retrieves output file path and dry-run flag
func getOutputFlags(cmd *cobra.Command, releaseName string) (outputFile string, dryRun bool, err error) {
	outputFile, err = cmd.Flags().GetString("output-file")
	if err != nil {
		return "", false, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-file flag: %w", err),
		}
	}

	// If no output file specified and we have a release name, default to <release-name>-overrides.yaml
	if outputFile == "" && releaseName != "" && !isStdOutRequested(cmd) {
		// Sanitize release name for use as a filename
		sanitizedName := strings.ReplaceAll(releaseName, "/", "-")
		sanitizedName = strings.ReplaceAll(sanitizedName, ":", "-")
		outputFile = sanitizedName + "-overrides.yaml"
		log.Infof("No output file specified, defaulting to %s", outputFile)
	}

	dryRun, err = cmd.Flags().GetBool("dry-run")
	if err != nil {
		return "", false, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get dry-run flag: %w", err),
		}
	}

	return outputFile, dryRun, nil
}

// handleHelmPluginOverride handles the override command when running as a Helm plugin
func handleHelmPluginOverride(cmd *cobra.Command, releaseName, namespace string, config *GeneratorConfig, pathStrategy, outputFile string, dryRun bool) error {
	// Create a new Helm client and adapter
	adapter, err := createHelmAdapter()
	if err != nil {
		return err
	}

	// Get command context
	ctx := getCommandContext(cmd)

	// Get the target registry
	targetRegistry := config.TargetRegistry

	// Call the adapter's OverrideRelease method
	overrideFile, err := adapter.OverrideRelease(ctx, releaseName, namespace, targetRegistry,
		config.SourceRegistries, pathStrategy, helm.OverrideOptions{
			StrictMode: config.StrictMode,
		})
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  fmt.Errorf("failed to override release: %w", err),
		}
	}

	// Handle output based on different conditions
	return handlePluginOverrideOutput(cmd, overrideFile, outputFile, dryRun, releaseName, namespace)
}

// handlePluginOverrideOutput handles the output of the override operation
func handlePluginOverrideOutput(cmd *cobra.Command, overrideFile, outputFile string, dryRun bool, releaseName, namespace string) error {
	// Use switch statement instead of if-else chain
	switch {
	case dryRun:
		// Dry run mode - output to stdout with headers
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- Dry Run: Generated Overrides ---"); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write dry run header: %w", err),
			}
		}

		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(overrideFile)); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write overrides in dry run mode: %w", err),
			}
		}

		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- End Dry Run ---"); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write dry run footer: %w", err),
			}
		}
	case outputFile != "":
		// Use the common file handling utility
		err := writeOutputFile(outputFile, []byte(overrideFile), "Successfully wrote overrides to %s")
		if err != nil {
			return err
		}
	default:
		// Just output to stdout
		_, err := fmt.Fprintln(cmd.OutOrStdout(), string(overrideFile))
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write overrides to stdout: %w", err),
			}
		}
	}

	// Validate the chart with overrides if requested
	return validatePluginOverrides(cmd, overrideFile, outputFile, dryRun, releaseName, namespace)
}

// validatePluginOverrides validates the generated overrides
func validatePluginOverrides(cmd *cobra.Command, overrideFile, outputFile string, dryRun bool, releaseName, namespace string) error {
	shouldValidate, err := cmd.Flags().GetBool("validate")
	if err == nil && shouldValidate {
		// If we've created an override file, use that directly
		var overrideFiles []string
		if outputFile != "" && !dryRun {
			overrideFiles = append(overrideFiles, outputFile)
		} else {
			// For dry-run or stdout output, write to a temporary file
			tempFile, err := afero.TempFile(AppFs, "", "irr-override-*.yaml")
			if err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("failed to create temp file for validation: %w", err),
				}
			}
			defer func() {
				if err := tempFile.Close(); err != nil {
					log.Warnf("Failed to close temporary file: %v", err)
				}
				if err := AppFs.Remove(tempFile.Name()); err != nil {
					log.Warnf("Failed to remove temporary file: %v", err)
				}
			}()

			if _, err := tempFile.WriteString(overrideFile); err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("failed to write override file: %w", err),
				}
			}

			overrideFiles = append(overrideFiles, tempFile.Name())
		}

		// Create a new Helm client and adapter
		adapter, err := createHelmAdapter()
		if err != nil {
			return err
		}

		// Get command context
		ctx := getCommandContext(cmd)

		err = adapter.ValidateRelease(ctx, releaseName, namespace, overrideFiles)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("failed to validate release: %w", err),
			}
		}

		log.Infof("Validation successful! Chart renders correctly with overrides.")
		log.Infof("To apply these changes, run:\n  helm upgrade %s -n %s -f %s",
			releaseName, namespace, outputFile)
	}

	return nil
}

// handleChartLoadError handles errors from loading a chart
func handleChartLoadError(err error, chartPath string) error {
	// Check if it's already an ExitCodeError
	var exitErr *exitcodes.ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr
	}

	// Handle common chart loading errors
	if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file or directory") {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartNotFound,
			Err:  fmt.Errorf("chart not found at path %s: %w", chartPath, err),
		}
	}

	// Generic chart loading error
	return &exitcodes.ExitCodeError{
		Code: exitcodes.ExitChartNotFound,
		Err:  fmt.Errorf("failed to load chart from %s: %w", chartPath, err),
	}
}

// isStdOutRequested returns true if output should go to stdout (either specifically requested or dry-run mode)
func isStdOutRequested(cmd *cobra.Command) bool {
	// Check for dry-run flag
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		log.Warnf("Failed to get dry-run flag: %v", err)
		return false
	}
	if dryRun {
		return true
	}

	// Check for stdout flag
	stdout, err := cmd.Flags().GetBool("stdout")
	if err != nil {
		log.Warnf("Failed to get stdout flag: %v", err)
		return false
	}
	return stdout
}

// validateChart validates a chart with the generated overrides
func validateChart(cmd *cobra.Command, yamlBytes []byte, config *GeneratorConfig, loadFromPath, loadFromRelease bool, releaseName, namespace string) error {
	// Create a temporary file to store the overrides
	tempFile, err := afero.TempFile(AppFs, "", "irr-override-*.yaml")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to create temporary file for validation: %w", err),
		}
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			log.Warnf("Failed to close temporary file: %v", err)
		}
		if err := AppFs.Remove(tempFile.Name()); err != nil {
			log.Warnf("Failed to remove temporary file: %v", err)
		}
	}()

	// Write the overrides to the temporary file
	if _, err := tempFile.Write(yamlBytes); err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to write overrides to temporary file: %w", err),
		}
	}

	// Get Helm version flag
	kubeVersion, err := cmd.Flags().GetString("kube-version")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get kube-version flag: %w", err),
		}
	}

	// Get strict mode flag
	strictMode := config.StrictMode

	var validationResult string
	switch {
	case loadFromPath:
		// Validate chart with overrides
		validationResult, err = validateChartWithFiles(config.ChartPath, "", "", []string{tempFile.Name()}, strictMode, kubeVersion)
	case loadFromRelease:
		// For release, use adapter to validate
		adapter, adapterErr := createHelmAdapter()
		if adapterErr != nil {
			return adapterErr
		}

		// Get command context
		ctx := getCommandContext(cmd)

		// Validate the release with the overrides
		valErr := adapter.ValidateRelease(ctx, releaseName, namespace, []string{tempFile.Name()})
		if valErr != nil {
			err = &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("failed to validate release: %w", valErr),
			}
		} else {
			validationResult = "Validation successful! Chart renders correctly with overrides."
		}
	default:
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("internal error: neither loadFromPath nor loadFromRelease is true"),
		}
	}

	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to validate chart with overrides: %w", err),
		}
	}

	log.Infof(validationResult)
	return nil
}

// handleTestModeOverride handles the override logic when IRR_TESTING is set.
func handleTestModeOverride(cmd *cobra.Command, releaseName string) error {
	// Get output flags
	outputFile, dryRun, err := getOutputFlags(cmd, releaseName)
	if err != nil {
		return err
	}

	releaseNameProvided := releaseName != "" && isHelmPlugin

	// Log what we're doing
	if releaseNameProvided {
		log.Infof("Using %s as release name from positional argument", releaseName)
	} else {
		chartPath, err := cmd.Flags().GetString("chart-path")
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
			}
		}
		log.Infof("Using chart path: %s", chartPath)
	}

	// Get validation flag
	shouldValidate, err := cmd.Flags().GetBool("validate")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get validate flag: %w", err),
		}
	}

	// Create mock output
	yamlContent := "mock: true\ngenerated: true\n"
	if releaseNameProvided {
		yamlContent += fmt.Sprintf("release: %s\n", releaseName)

		// Add namespace information for tests
		namespace, err := cmd.Flags().GetString("namespace")
		if err == nil {
			// If no namespace specified, use "default"
			if namespace == "" {
				namespace = "default"
			}
			yamlContent += fmt.Sprintf("namespace: %s\n", namespace)
		}
	}

	targetRegistry, err := cmd.Flags().GetString("target-registry")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get target-registry flag: %w", err),
		}
	}
	if targetRegistry != "" {
		yamlContent += fmt.Sprintf("targetRegistry: %s\n", targetRegistry)
	}

	// Create the output file if specified
	switch {
	case outputFile != "" && !dryRun:
		if err := AppFs.MkdirAll(filepath.Dir(outputFile), DirPermissions); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to create output directory: %w", err),
			}
		}
		exists, err := afero.Exists(AppFs, outputFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to check if output file exists: %w", err),
			}
		}
		if exists {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("output file '%s' already exists", outputFile),
			}
		}
		if err := afero.WriteFile(AppFs, outputFile, []byte(yamlContent), FilePermissions); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write override file: %w", err),
			}
		}
		log.Infof("Successfully wrote overrides to %s", outputFile)
	case dryRun:
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- Dry Run: Generated Overrides ---"); err != nil {
			return fmt.Errorf("failed to write dry run header: %w", err) // Wrap error
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), yamlContent); err != nil {
			return fmt.Errorf("failed to write overrides in dry run mode: %w", err) // Wrap error
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- End Dry Run ---"); err != nil {
			return fmt.Errorf("failed to write dry run footer: %w", err) // Wrap error
		}

		// Add validation output to dry run if requested
		if shouldValidate {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Validation successful! Chart renders correctly with overrides."); err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitGeneralRuntimeError,
					Err:  fmt.Errorf("failed to write validation success message: %w", err),
				}
			}
		}
	default:
		if shouldValidate {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Validation successful! Chart renders correctly with overrides."); err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitGeneralRuntimeError,
					Err:  fmt.Errorf("failed to write validation success message: %w", err),
				}
			}
		}
	}

	return nil
}
