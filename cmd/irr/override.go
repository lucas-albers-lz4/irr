// Package main implements the command-line interface for the irr (Image Relocation and Rewrite) tool.
// This file contains the override command implementation.
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

	internalhelm "github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/chart"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	helmtypes "github.com/lucas-albers-lz4/irr/pkg/helmtypes"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/lucas-albers-lz4/irr/pkg/strategy"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli/values"
	"sigs.k8s.io/yaml"
)

const (
	// ExitHelmInteractionError is returned when there's an error during Helm SDK interaction
	ExitHelmInteractionError = 17
	// ExitInternalError is returned when there's an internal error in command execution
	ExitInternalError       = 30
	chartSourceTypeChart    = "chart"
	chartSourceTypeRelease  = "release"
	autoDetectedChartSource = "auto-detected"
	// trueString represents the string literal "true", commonly used for boolean env vars.
	trueString = "true"
	// unknownSourceDescription is used when the chart source cannot be determined.
	unknownSourceDescription = "unknown"
)

// Variables for testing - isTestMode declaration REMOVED, it's defined in root.go
/*
var (
	isTestMode = false
)
*/

// GeneratorConfig struct with strategy field but no threshold field
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
	// StrictMode enables strict validation (fails on any error)
	StrictMode bool
	// IncludePatterns contains glob patterns for values paths to include
	IncludePatterns []string
	// ExcludePatterns contains glob patterns for values paths to exclude
	ExcludePatterns []string
	// RulesEnabled controls whether the chart parameter rules system is enabled
	RulesEnabled bool
}

// For testing purposes - allows overriding in tests
// var chartLoader = loadChart

// OverrideFlags defines the flags used by the override command
type OverrideFlags struct {
	ChartPath         string
	ReleaseName       string
	Namespace         string
	TargetRegistry    string
	SourceRegistries  []string
	ExcludeRegistries []string
	OutputFile        string
	ConfigFile        string
	StrictMode        bool
	IncludePatterns   []string
	ExcludePatterns   []string
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
		Long: `Analyzes a Helm chart to find all container image references (both direct string values " +
			"and map-based structures like 'image.repository', 'image.tag'). It then generates a " +
			"Helm-compatible values file that overrides these references to point to a specified " +
			"target registry, using a defined path strategy.\n\n" +
			"Supports filtering images based on source registries and excluding specific registries. " +
			"Can also utilize a registry mapping file for more complex source-to-target mappings.\n\n" +
			"IMPORTANT NOTES:\n" +
			"- This command can run without a config file, but image redirection correctness depends on your configuration.\n" +
			"- Use 'irr inspect' to identify registries in your chart and 'irr config' to configure mappings.\n" +
			"- When using Harbor as a pull-through cache, ensure your target paths match your Harbor project configuration.`,
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			detectedPluginMode := isRunningAsHelmPlugin() // Call detection function directly

			// Check if we're in plugin mode with a release name
			// Use the directly detected plugin mode status instead of the potentially stale global variable
			hasReleaseName := len(args) > 0 && detectedPluginMode
			// Get chart path flag value early to check if it was explicitly provided
			chartPath, err := cmd.Flags().GetString("chart-path")
			if err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitInputConfigurationError,
					Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
				}
			}
			chartPathProvided := chartPath != ""

			// Get required flags for later checks
			targetRegistry, err := cmd.Flags().GetString("target-registry")
			if err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitInputConfigurationError,
					Err:  fmt.Errorf("failed to get target-registry flag: %w", err),
				}
			}
			sourceRegistries, err := cmd.Flags().GetStringSlice("source-registries")
			if err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitInputConfigurationError,
					Err:  fmt.Errorf("failed to get source-registries flag: %w", err),
				}
			}
			configPath, err := cmd.Flags().GetString("config")
			if err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitInputConfigurationError,
					Err:  fmt.Errorf("failed to get config flag: %w", err),
				}
			}

			var missingFlags []string

			// Chart source check:
			// Require chart-path ONLY if no release name is given positionally in plugin mode,
			// AND chart-path flag was not explicitly provided.
			if !hasReleaseName && !chartPathProvided {
				missingFlags = append(missingFlags, "chart-path")
			}

			// Target registry is always required
			if targetRegistry == "" {
				// Special case: if config file is provided, target-registry *might* be optional later,
				// but for PreRunE simplicity, we require it unless the user explicitly provides a config.
				// The main RunE logic should handle the case where mappings provide the target.
				// Let's keep the original check: require target unless config is specified AND target IS specified.
				// Simplified: Always require target for PreRun check clarity. RunE can be smarter.
				// Re-evaluating: The original logic checks if config path is provided AND target is empty. Keep that.
				if configPath == "" || (configPath != "" && targetRegistry == "") {
					// Correction: Simpler check - target registry is always required in PreRun
					missingFlags = append(missingFlags, "target-registry")
				}
			}

			// Source registries are always required
			if len(sourceRegistries) == 0 {
				// Allow skipping source-registries if a config file is provided, as it might contain mappings.
				// RunE will handle validation if mappings don't cover sources.
				if configPath == "" {
					missingFlags = append(missingFlags, "source-registries")
				}
			}

			if len(missingFlags) > 0 {
				// Remove duplicates just in case logic above adds the same flag twice
				uniqueFlags := make(map[string]bool)
				finalMissing := []string{}
				for _, flag := range missingFlags {
					if !uniqueFlags[flag] {
						uniqueFlags[flag] = true
						finalMissing = append(finalMissing, flag)
					}
				}
				sort.Strings(finalMissing) // Sort for consistent error message
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitMissingRequiredFlag,
					Err:  fmt.Errorf("required flag(s) \"%s\" not set", strings.Join(finalMissing, "\", \"")),
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

	// Optional flags
	cmd.Flags().StringP("output-file", "o", "", "Write output to file instead of stdout")
	cmd.Flags().StringP("config", "f", "", "Path to registry mapping config file")
	cmd.Flags().Bool("strict", false, "Enable strict mode (fails on unsupported structures)")
	cmd.Flags().StringSlice("include-pattern", []string{}, "Glob patterns for values paths to include (comma-separated)")
	cmd.Flags().StringSlice("exclude-pattern", []string{}, "Glob patterns for values paths to exclude (comma-separated)")
	cmd.Flags().Bool("disable-rules", false, "Disable the chart parameter rules system")
	cmd.Flags().Bool("dry-run", false, "Perform a dry run (show changes without writing files)")
	cmd.Flags().StringSliceP("exclude-registries", "e", []string{}, "Registry URLs to exclude from relocation")
	cmd.Flags().Bool("no-validate", false, "Skip the internal Helm template validation check after generating overrides")
	cmd.Flags().String("kube-version", "", "Kubernetes version to use for validation (defaults to current client version)")
	cmd.Flags().StringP("namespace", "n", "default", "Namespace to use (default: default)")
	cmd.Flags().StringP("release-name", "r", "", "Release name to use (only in Helm plugin mode)")

	// Add Helm flags for values processing
	cmd.Flags().StringSlice("values", nil, "Values files to process (can be specified multiple times)")
	cmd.Flags().StringSlice("set", nil, "Set values on the command line (can be specified multiple times)")
	cmd.Flags().StringSlice("set-string", nil, "Set STRING values on the command line (can be specified multiple times)")
	cmd.Flags().StringSlice("set-file", nil, "Set values from files (can be specified multiple times)")

	// Remove the context-aware flag, as it's now the default behavior
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

// getOutputFlags retrieves output file and dry run settings
func getOutputFlags(cmd *cobra.Command, releaseName string) (outputFile string, dryRun bool, err error) {
	outputFile, err = cmd.Flags().GetString("output-file")
	if err != nil {
		return "", false, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-file flag: %w", err),
		}
	}

	// Set default output file in plugin mode with release name
	if outputFile == "" && isRunningAsHelmPlugin() && releaseName != "" {
		outputFile = fmt.Sprintf("%s-overrides.yaml", releaseName)
		log.Info("No output file specified in plugin mode, using default based on release name", "file", outputFile)
	}

	// Get dry run flag
	dryRun, err = cmd.Flags().GetBool("dry-run")
	if err != nil {
		return "", false, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get dry-run flag: %w", err),
		}
	}

	log.Info("Output flags", "outputFile", outputFile, "dryRun", dryRun)
	return outputFile, dryRun, nil
}

// outputOverrides handles writing the generated YAML to the correct destination
// (stdout or file) or logging it for dry-run.
func outputOverrides(cmd *cobra.Command, yamlBytes []byte, outputFile string, dryRun bool) error {
	switch {
	case dryRun:
		// Log that we are doing a dry run and printing to stdout
		log.Info("DRY RUN: Displaying generated override values (stdout)")
		// Print the actual YAML to stdout
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(yamlBytes)); err != nil {
			log.Error("Failed to write dry-run output to stdout", "error", err)
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write dry-run output to stdout: %w", err),
			}
		}
		return nil // Dry run successful
	case outputFile == "":
		// Just output to stdout
		_, err := fmt.Fprintln(cmd.OutOrStdout(), string(yamlBytes))
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write overrides to stdout: %w", err),
			}
		}
		log.Info("Override values printed to stdout")
		return nil
	default:
		// Check if file exists
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

		// Create the directory if it doesn't exist
		dir := filepath.Dir(outputFile)
		if dir != "" && dir != "." {
			if mkDirErr := AppFs.MkdirAll(dir, fileutil.ReadWriteExecuteUserReadExecuteOthers); mkDirErr != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("failed to create output directory: %w", mkDirErr),
				}
			}
		}

		// Write the file
		if writeErr := afero.WriteFile(AppFs, outputFile, yamlBytes, fileutil.ReadWriteUserReadOthers); writeErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write output file '%s': %w", outputFile, writeErr),
			}
		}

		// Log success
		absPath, err := filepath.Abs(outputFile)
		if err == nil {
			log.Info("Override values written", "path", absPath)
		} else {
			log.Info("Override values written", "path", outputFile)
		}

		return nil
	}
}

// setupGeneratorConfig retrieves and configures all options for the generator
// It ONLY gathers flags and populates the struct. Further processing happens in runOverride.
func setupGeneratorConfig(cmd *cobra.Command, _ string) (config GeneratorConfig, err error) {
	// Get required flags first
	chartPath, targetRegistry, sourceRegistries, err := getRequiredFlags(cmd)
	if err != nil {
		return config, err // Return zero config on error
	}
	config.ChartPath = chartPath
	config.TargetRegistry = targetRegistry
	config.SourceRegistries = sourceRegistries

	// Get optional flags
	excludeRegistries, err := getStringSliceFlag(cmd, "exclude-registries")
	if err != nil {
		return config, err // Return zero config on error
	}
	config.ExcludeRegistries = excludeRegistries

	strictMode, err := getBoolFlag(cmd, "strict")
	if err != nil {
		return config, err // Return zero config on error
	}
	config.StrictMode = strictMode

	includePatterns, excludePatterns, err := getAnalysisControlFlags(cmd)
	if err != nil {
		return config, err // Return zero config on error
	}
	config.IncludePatterns = includePatterns
	config.ExcludePatterns = excludePatterns

	disableRules, err := getBoolFlag(cmd, "disable-rules")
	if err != nil {
		return config, err // Return zero config on error
	}
	config.RulesEnabled = !disableRules

	// NOTE: We do NOT call setupPathStrategy, loadRegistryMappings, logConfigMode,
	// or validateUnmappableRegistries here. They are called in runOverride
	// after this function returns successfully.

	// Log excluded registries if any were provided
	if len(config.ExcludeRegistries) > 0 {
		log.Info("Excluding registries", "registries", strings.Join(config.ExcludeRegistries, ", "))
	}

	// Successfully gathered all flags
	return config, nil
}

func setupPathStrategy(_ *cobra.Command, config *GeneratorConfig) error {
	// Add nil check for safety, although runOverride should prevent this call with nil config
	if config == nil {
		return errors.New("internal error: setupPathStrategy called with nil config")
	}
	// The --path-strategy flag has been removed. Always use the default.
	config.Strategy = strategy.NewPrefixSourceRegistryStrategy()
	return nil
}

// skipCWDCheck returns true if we should skip the cwd check for registry files
func skipCWDCheck() bool {
	// Get the flag value (using the global variable populated by Cobra)
	itFlag := integrationTestMode

	// Check the environment variable
	irrTestingEnv := os.Getenv("IRR_TESTING") == trueString

	// Log the check results (optional but helpful for debugging)
	// Note: Cannot use slog here easily as logger isn't passed in. Use fmt for temporary debug if needed.
	// fmt.Fprintf(os.Stderr, "[DEBUG skipCWDCheck] integrationTestFlag=%t, irrTestingEnv=%t\n", itFlag, irrTestingEnv)

	return itFlag || irrTestingEnv
}

// loadRegistryMappings loads registry mappings from config and registry files
func loadRegistryMappings(cmd *cobra.Command, config *GeneratorConfig) error {
	if config == nil {
		return errors.New("loadRegistryMappings: config parameter is nil")
	}
	configFile, err := getStringFlag(cmd, "config")
	if err != nil {
		return err
	}
	if configFile != "" {
		log.Info("Loading registry mappings from config file", "file", configFile)
		// Pass the result of skipCWDCheck() here to control path validation
		shouldSkipCheck := skipCWDCheck()
		log.Debug("Calling registry.LoadStructuredConfig", "configFile", configFile, "skipCWDRestriction", shouldSkipCheck)

		// Use LoadStructuredConfig instead of LoadMappings
		configObj, err := registry.LoadStructuredConfig(AppFs, configFile, shouldSkipCheck)
		if err != nil {
			if os.IsNotExist(err) {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitInputConfigurationError,
					Err:  fmt.Errorf("config file not found: %s", configFile),
				}
			}
			// Wrap the error from LoadStructuredConfig
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to load config file '%s': %w", configFile, err),
			}
		}

		// Convert to Mappings format
		config.Mappings = configObj.ToMappings()
		log.Debug("Successfully loaded mappings from file", "count", len(config.Mappings.Entries))
	}
	return nil
}

func logConfigMode(config *GeneratorConfig) {
	// Add nil check for safety
	if config == nil {
		log.Warn("logConfigMode called with nil config")
		return
	}
	if config.StrictMode {
		log.Info("Running in strict mode - will fail on unrecognized registries or unsupported structures")
	} else {
		log.Info("Running in normal mode - will skip unrecognized registries with warnings")
	}
	if len(config.SourceRegistries) > 0 {
		log.Info("Using source registries", "registries", strings.Join(config.SourceRegistries, ", "))
	}
}

// validateUnmappableRegistries checks if all provided source registries are covered by mappings.
// It logs warnings or returns an error based on strict mode.
func validateUnmappableRegistries(config *GeneratorConfig) error {
	// Add nil check for safety
	if config == nil {
		return errors.New("internal error: validateUnmappableRegistries called with nil config")
	}

	if len(config.SourceRegistries) == 0 {
		// No source registries provided, nothing to validate
		return nil
	}

	// Check if mappings exist
	hasMappings := (config.Mappings != nil && len(config.Mappings.Entries) > 0)

	// If NO mappings exist at all, check all source registries.
	if !hasMappings {
		if config.StrictMode {
			// Strict mode requires mappings if source registries are specified
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitRegistryDetectionError,
				Err:  fmt.Errorf("strict mode enabled: no mapping found for registries: %s", strings.Join(config.SourceRegistries, ", ")),
			}
		}
		// Non-strict mode: Log warning about all source registries needing mapping
		log.Warn("No mapping found for registries", "registries", strings.Join(config.SourceRegistries, ", "))
		log.Info("These registries will be redirected using the target registry", "target", config.TargetRegistry)
		log.Info("To add mappings, use: irr config --source <registry> --target <path>")
		for _, reg := range config.SourceRegistries {
			log.Info("irr config suggestion", "source", reg, "target", fmt.Sprintf("%s/%s", config.TargetRegistry, strings.ReplaceAll(reg, ".", "-")))
		}
		return nil // Don't error in non-strict mode
	}

	// If mappings *do* exist, check each source registry individually
	unmappableRegistries := make([]string, 0)
	for _, sourceReg := range config.SourceRegistries {
		found := false
		if config.Mappings != nil {
			for _, mapping := range config.Mappings.Entries {
				if mapping.Source == sourceReg {
					found = true
					break
				}
			}
		}
		if !found {
			if strings.HasPrefix(config.TargetRegistry, sourceReg) {
				found = true
			}
		}
		if !found {
			unmappableRegistries = append(unmappableRegistries, sourceReg)
		}
	}
	if len(unmappableRegistries) > 0 {
		if config.StrictMode {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitRegistryDetectionError,
				Err:  fmt.Errorf("strict mode enabled: no mapping found for registries: %s", strings.Join(unmappableRegistries, ", ")),
			}
		}
		log.Warn("No mapping found for registries", "registries", strings.Join(unmappableRegistries, ", "))
		log.Info("These registries will be redirected using the target registry", "target", config.TargetRegistry)
		log.Info("To add mappings, use: irr config --source <registry> --target <path>")
		for _, reg := range unmappableRegistries {
			log.Info("irr config suggestion", "source", reg, "target", fmt.Sprintf("%s/%s", config.TargetRegistry, strings.ReplaceAll(reg, ".", "-")))
		}
	}
	return nil
}

// getAnalysisControlFlags retrieves include/exclude patterns and known image paths
func getAnalysisControlFlags(cmd *cobra.Command) (includePatterns, excludePatterns []string, err error) {
	includePatterns, err = getStringSliceFlag(cmd, "include-pattern")
	if err != nil {
		return
	}

	excludePatterns, err = getStringSliceFlag(cmd, "exclude-pattern")
	if err != nil {
		return
	}

	return
}

// createAndExecuteGenerator creates and executes a generator for the given chart source
func createAndExecuteGenerator(chartSource *ChartSource, config *GeneratorConfig) ([]byte, error) {
	if chartSource == nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  errors.New("chartSource is nil"),
		}
	}

	// Get chart source description for comments
	var chartSourceDescription string
	if chartSource != nil {
		chartSourceDescription = chartSource.Message // Use the message generated by getChartSource
	} else {
		chartSourceDescription = unknownSourceDescription // Fallback if chartSource is somehow nil
	}

	log.Info("Initializing override generator", "source", chartSourceDescription)

	// Create a new generator and run it
	generator, err := createGenerator(chartSource, config)
	if err != nil {
		return nil, err
	}

	// Generate the override file content
	overrideFile, err := generator.Generate()
	if err != nil {
		return nil, handleGenerateError(err)
	}

	// --- Log Final Map Before Marshaling ---
	finalMapLogBytes, logErr := yaml.Marshal(overrideFile.Values)
	if logErr != nil {
		log.Warn("createAndExecuteGenerator: Failed to marshal final map for logging", "error", logErr)
	}
	log.Debug("createAndExecuteGenerator: Final map BEFORE marshaling", "mapYaml", string(finalMapLogBytes))
	// --- End Log ---

	// Convert the final override values map to YAML
	yamlBytes, err := chart.OverridesToYAML(overrideFile.Values)
	if err != nil {
		log.Error("Failed to marshal overrides to YAML", "error", err)
		return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
	}

	return yamlBytes, nil
}

// createGenerator creates a generator for the given chart source using context-aware chart loading
func createGenerator(_ *ChartSource, config *GeneratorConfig) (GeneratorInterface, error) {
	// Validate the config
	if config == nil {
		return nil, errors.New("config is nil")
	}

	// Create value options from the command line
	valueOpts := &values.Options{}

	// Get Helm CLI arguments for value loading - use env vars if running as plugin
	if isRunningAsHelmPlugin() {
		// Retrieve values from environment variables set by Helm
		valuesFiles := os.Getenv("HELM_PLUGIN_VALUES")
		if valuesFiles != "" {
			valueOpts.ValueFiles = strings.Split(valuesFiles, ";")
		}

		setValues := os.Getenv("HELM_PLUGIN_SET")
		if setValues != "" {
			valueOpts.Values = strings.Split(setValues, ";")
		}

		setStringValues := os.Getenv("HELM_PLUGIN_SET_STRING")
		if setStringValues != "" {
			valueOpts.StringValues = strings.Split(setStringValues, ";")
		}

		setFileValues := os.Getenv("HELM_PLUGIN_SET_FILE")
		if setFileValues != "" {
			valueOpts.FileValues = strings.Split(setFileValues, ";")
		}
	}

	// Create chart loader options for context-aware analysis
	loaderOptions := &helmtypes.ChartLoaderOptions{
		ChartPath:  config.ChartPath,
		ValuesOpts: *valueOpts,
	}

	// Use the new context-aware chart loader
	chartLoader := internalhelm.NewChartLoader()
	chartAnalysisContext, err := chartLoader.LoadChartAndTrackOrigins(loaderOptions)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed,
			Err:  fmt.Errorf("failed to load chart with values: %w", err),
		}
	}

	// Create context-aware analyzer and perform analysis
	contextAnalyzer := internalhelm.NewContextAwareAnalyzer(chartAnalysisContext)
	_, err = contextAnalyzer.Analyze()
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  fmt.Errorf("analysis failed: %w", err),
		}
	}

	// Create a custom loader that returns the pre-loaded chart
	preloadedLoader := &PreloadedChartLoader{
		Chart:   chartAnalysisContext.LoadedChart,
		Context: chartAnalysisContext,
	}

	// --- Create Override Generator ---
	generator := chart.NewGenerator(
		config.ChartPath,
		config.TargetRegistry,
		config.SourceRegistries,
		config.ExcludeRegistries,
		config.Strategy,
		config.Mappings,
		config.StrictMode,
		0,               // Threshold parameter is not used anymore
		preloadedLoader, // Use our custom loader with pre-analyzed chart
		config.IncludePatterns,
		config.ExcludePatterns,
		nil,                 // KnownImagePaths parameter is not used anymore
		config.RulesEnabled, // Pass rules enabled status here
	)

	// Log message if rules are disabled
	if !config.RulesEnabled {
		log.Info("Chart parameter rules system is disabled")
	}

	return generator, nil
}

// PreloadedChartLoader is a custom loader that returns a pre-loaded chart and analysis.
type PreloadedChartLoader struct {
	Chart   *helmchart.Chart
	Context *helmtypes.ChartAnalysisContext
}

// errAnalyzeNotApplicable indicates that the Analyze method is not applicable for PreloadedChartLoader.
var errAnalyzeNotApplicable = errors.New("Analyze method is not applicable for PreloadedChartLoader")

// Load returns the pre-loaded chart.
func (l *PreloadedChartLoader) Load(_ string) (*helmchart.Chart, error) {
	return l.Chart, nil
}

// Analyze is not applicable for PreloadedChartLoader and returns an error.
func (l *PreloadedChartLoader) Analyze(_ string) (*analysis.ChartAnalysis, error) {
	return nil, errAnalyzeNotApplicable // Not used in this context, return sentinel error
}

// LoadChartAndTrackOrigins returns the pre-loaded analysis context.
func (l *PreloadedChartLoader) LoadChartAndTrackOrigins(_ *helmtypes.ChartLoaderOptions) (*helmtypes.ChartAnalysisContext, error) {
	return l.Context, nil
}

// LoadChartWithValues returns the pre-loaded chart and nil values (as merged values aren't available).
func (l *PreloadedChartLoader) LoadChartWithValues(_ *helmtypes.ChartLoaderOptions) (*helmchart.Chart, map[string]interface{}, error) {
	return l.Chart, nil, nil // Merged values not available in this context
}

// runOverridePluginMode handles the logic when override is run in plugin mode.
func runOverridePluginMode(cmd *cobra.Command, releaseName, namespace, outputFile string, dryRun bool) error {
	log.Debug("Plugin mode detected", "releaseName", releaseName)
	var config GeneratorConfig
	// Gather required flags directly, skipping chart-path validation
	targetRegistry, err := getStringFlag(cmd, "target-registry")
	if err != nil {
		return err
	}
	if targetRegistry == "" {
		return &exitcodes.ExitCodeError{Code: exitcodes.ExitMissingRequiredFlag, Err: errors.New("required flag \"target-registry\" not set")}
	}

	sourceRegistries, err := getStringSliceFlag(cmd, "source-registries")
	if err != nil {
		return err
	}
	// Check source-registries requirement, allowing skip if config file is present
	configFile, err := getStringFlag(cmd, "config")
	if err != nil {
		return err
	}
	if len(sourceRegistries) == 0 && configFile == "" {
		return &exitcodes.ExitCodeError{Code: exitcodes.ExitMissingRequiredFlag, Err: errors.New("required flag \"source-registries\" not set (or provide --config)")}
	}

	config.TargetRegistry = targetRegistry
	config.SourceRegistries = sourceRegistries
	config.ChartPath = "" // Explicitly set ChartPath to empty for plugin mode config

	// Gather optional flags for plugin mode
	excludeRegistries, err := getStringSliceFlag(cmd, "exclude-registries")
	if err != nil {
		return err
	}
	config.ExcludeRegistries = excludeRegistries

	strictMode, err := getBoolFlag(cmd, "strict")
	if err != nil {
		return err
	}
	config.StrictMode = strictMode

	includePatterns, excludePatterns, err := getAnalysisControlFlags(cmd)
	if err != nil {
		return err
	}
	config.IncludePatterns = includePatterns
	config.ExcludePatterns = excludePatterns

	disableRules, err := getBoolFlag(cmd, "disable-rules")
	if err != nil {
		return err
	}
	config.RulesEnabled = !disableRules

	// --- Common Config Setup (after mode-specific gathering) ---
	if err := setupPathStrategy(cmd, &config); err != nil {
		return err
	}
	if err := loadRegistryMappings(cmd, &config); err != nil {
		return err
	}
	logConfigMode(&config)
	if err := validateUnmappableRegistries(&config); err != nil {
		return err
	}
	// --- End Common Config Setup ---

	// Call plugin-specific handler
	return handleHelmPluginOverride(cmd, releaseName, namespace, &config, "", outputFile, dryRun)
}

// runOverrideStandaloneMode handles the logic when override is run in standalone mode.
func runOverrideStandaloneMode(cmd *cobra.Command, outputFile string, dryRun bool) error {
	log.Debug("Standalone mode detected (no release name provided)")
	// Call original setup which requires chart-path
	config, err := setupGeneratorConfig(cmd, "") // releaseName is "" here
	if err != nil {
		return err // Config setup failed
	}

	// Auto-detect chart path if not provided
	if config.ChartPath == "" { // Check config.ChartPath which setupGeneratorConfig sets
		log.Info("No chart path provided, attempting to detect chart...")
		var relativePath string // Declare relativePath
		var detectErr error     // Declare detectErr
		// Call the updated function and capture all return values
		detectedPath, relativePath, detectErr := detectChartIfNeeded(AppFs, ".")
		if detectErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartLoadFailed,
				Err:  fmt.Errorf("chart path not provided and could not auto-detect chart: %w", detectErr),
			}
		}
		config.ChartPath = detectedPath // Update config
		// Log both paths for consistency, even if relativePath is unused here
		log.Info("Using detected chart path", "absolute", detectedPath, "relative", relativePath)
	}

	// --- Common Config Setup (after mode-specific gathering) ---
	if err := setupPathStrategy(cmd, &config); err != nil {
		return err
	}
	if err := loadRegistryMappings(cmd, &config); err != nil {
		return err
	}
	logConfigMode(&config)
	if err := validateUnmappableRegistries(&config); err != nil {
		return err
	}
	// --- End Common Config Setup ---

	chartSource := &ChartSource{
		SourceType: ChartSourceTypeChart,
		ChartPath:  config.ChartPath,
	}

	// Execute the generator
	yamlBytes, err := createAndExecuteGenerator(chartSource, &config)
	if err != nil {
		return handleGenerateError(err) // Handles exit codes
	}

	// Validate the generated overrides
	noValidate, noValErr := getBoolFlag(cmd, "no-validate")
	if noValErr != nil {
		log.Warn("Failed to get no-validate flag, defaulting to false (validation will run)", "error", noValErr)
		noValidate = false // Default to running validation if flag access fails
	}

	log.Debug("Standalone Mode Validation Check", "noValidateFlag", noValidate)

	// If no-validate is false, run validation
	if !noValidate {
		log.Debug("Standalone Mode: Running internal validation.")

		if err := validateChart(cmd, yamlBytes, &config, true, false, "", ""); err != nil {
			return err
		}
	}

	// Output the results
	return outputOverrides(cmd, yamlBytes, outputFile, dryRun)
}

// runOverride is the main execution function for the override command
func runOverride(cmd *cobra.Command, args []string) error {
	// Determine if running in test mode
	isTestMode, err := getBoolFlag(cmd, "test-mode")
	if err != nil {
		log.Warn("Failed to get test-mode flag, defaulting to false", "error", err)
		isTestMode = false
	}

	// Get release name and namespace
	releaseName, namespace, err := getReleaseNameAndNamespace(cmd, args)
	if err != nil {
		return err
	}

	// Handle test mode early if enabled
	if isTestMode {
		return handleTestModeOverride(cmd, releaseName)
	}

	// Get output flags (needed regardless of mode)
	outputFile, dryRun, err := getOutputFlags(cmd, releaseName)
	if err != nil {
		return err
	}

	// Handle Helm plugin mode vs. Standalone mode
	if releaseName != "" { // Plugin Mode
		return runOverridePluginMode(cmd, releaseName, namespace, outputFile, dryRun)
	}

	// Standalone Mode
	return runOverrideStandaloneMode(cmd, outputFile, dryRun)
}

// getReleaseNameAndNamespace gets the release name and namespace from the command
func getReleaseNameAndNamespace(cmd *cobra.Command, args []string) (releaseName, namespace string, err error) {
	// Use common function to get release name and namespace
	return getReleaseNameAndNamespaceCommon(cmd, args)
}

// handleHelmPluginOverride handles the override command when running as a Helm plugin
func handleHelmPluginOverride(cmd *cobra.Command, releaseName, namespace string, config *GeneratorConfig, pathStrategy, outputFile string, dryRun bool) error {
	// Add nil check for config
	if config == nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  errors.New("internal error: generator config is nil in handleHelmPluginOverride"),
		}
	}

	// Create a new Helm client and adapter
	adapter, err := createHelmAdapter()
	if err != nil {
		return err
	}

	// Get command context
	ctx := getCommandContext(cmd)

	// Get the target registry
	targetRegistry := config.TargetRegistry

	// Add debug logging to troubleshoot nil pointer issue
	log.Debug("handleHelmPluginOverride details", "releaseName", releaseName, "namespace", namespace, "targetRegistry", targetRegistry)
	log.Debug("handleHelmPluginOverride sourceRegistries", "sourceRegistries", config.SourceRegistries)
	log.Debug("handleHelmPluginOverride pathStrategy", "pathStrategy", pathStrategy)
	log.Debug("handleHelmPluginOverride strictMode", "strictMode", config.StrictMode)

	// Call the adapter's OverrideRelease method
	overrideFile, err := adapter.OverrideRelease(ctx, releaseName, namespace, targetRegistry,
		config.SourceRegistries, pathStrategy, internalhelm.OverrideOptions{
			StrictMode: config.StrictMode,
		})
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  fmt.Errorf("failed to override release: %w", err),
		}
	}

	// Handle output based on different conditions - pass config parameter
	return handlePluginOverrideOutput(cmd, string(overrideFile), outputFile, dryRun, releaseName, namespace, config)
}

// handlePluginOverrideOutput handles the output of the override operation
// Remove the contextAware parameter
func handlePluginOverrideOutput(cmd *cobra.Command, overrideFile, outputFile string, dryRun bool, releaseName, namespace string, config *GeneratorConfig) error {
	switch {
	case dryRun:
		// Dry run mode - output to stdout with headers
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- Dry Run: Generated Overrides ---"); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write dry run header: %w", err),
			}
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), overrideFile); err != nil {
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
		_, err := fmt.Fprintln(cmd.OutOrStdout(), overrideFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write overrides to stdout: %w", err),
			}
		}
	}
	// Validate the chart with overrides if requested - pass config parameter
	return validatePluginOverrides(cmd, overrideFile, outputFile, dryRun, releaseName, namespace, config)
}

// validatePluginOverrides validates the generated overrides using Helm template.
// Remove the contextAware parameter
func validatePluginOverrides(cmd *cobra.Command, overrideFile, outputFile string, dryRun bool, releaseName, namespace string, config *GeneratorConfig) error {
	log.Debug("Entering validatePluginOverrides", "overrideFile", overrideFile, "outputFile", outputFile, "dryRun", dryRun)

	// Get the --no-validate flag value
	noValidate, err := cmd.Flags().GetBool("no-validate")
	if err != nil {
		log.Error("Failed to read --no-validate flag", "error", err)
		noValidate = false // Default to validating if flag access fails
	}

	log.Debug("Plugin Mode Validation Check", "noValidateFlag", noValidate)

	// Skip validation if --no-validate is set
	if noValidate {
		log.Info("Skipping validation due to --no-validate flag.")
		return nil
	}

	// If we reach here, validation should run.
	log.Debug("Plugin Mode: Proceeding with internal validation.")

	// If dry-run, validation should still conceptually happen, but we don't read a file.
	// The overrideFile variable in dry-run mode should contain the YAML content directly.

	var yamlBytes []byte
	if dryRun {
		// In dry-run, overrideFile argument holds the content, not a path
		yamlBytes = []byte(overrideFile)
		log.Debug("Dry-run mode: Using override content directly for validation")
	} else {
		// Read the generated override file content if not dry-run
		yamlBytes, err = os.ReadFile(overrideFile) // #nosec G304 - overrideFile is generated by this process, not user-supplied
		if err != nil {
			log.Error("Failed to read generated override file for validation", "file", overrideFile, "error", err)
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed, // Use existing code for validation failure
				Err:  fmt.Errorf("failed to read generated override file '%s': %w", overrideFile, err),
			}
		}
	}

	// Perform the validation using the common validateChart function
	log.Info("Validating generated overrides with Helm template...")
	if err := validateChart(cmd, yamlBytes, config, false, true, releaseName, namespace); err != nil {
		log.Error("Validation failed: Chart could not be rendered with generated overrides.", "error", err)
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed, // Use existing code for validation failure
			Err:  fmt.Errorf("validation failed: %w", err),
		}
	}

	log.Info("Validation successful.")
	return nil
}

// handleTestModeOverride handles the override logic when running in test mode.
func handleTestModeOverride(cmd *cobra.Command, releaseName string) error {
	// Get output flags
	outputFile, dryRun, err := getOutputFlags(cmd, releaseName)
	if err != nil {
		return err
	}

	releaseNameProvided := releaseName != "" && isRunningAsHelmPlugin()

	// Log what we're doing
	if releaseNameProvided {
		log.Info("Using release name from positional argument", "releaseName", releaseName)
	} else {
		chartPath, err := cmd.Flags().GetString("chart-path")
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
			}
		}
		log.Info("Using chart path", "path", chartPath)
	}

	// Get validation flag status based on --no-validate
	noValidate, err := cmd.Flags().GetBool("no-validate")
	if err != nil {
		// Log error but assume validation should run if flag access fails in test mode?
		// Or maybe assume validation is skipped? Let's assume skipped for safety in test helper.
		log.Warn("Failed to get no-validate flag in test mode, assuming validation is skipped", "error", err)
		noValidate = true
	}
	shouldValidate := !noValidate // Determine if validation should happen

	// Create mock output
	yamlContent := "mock: true\ngenerated: true\n"
	if releaseNameProvided {
		yamlContent += fmt.Sprintf("release: %s\n", releaseName)

		// Add namespace information for tests
		namespace, err := getStringFlag(cmd, "namespace")
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to get namespace flag: %w", err),
			}
		}
		if namespace == "" {
			namespace = validateTestNamespace
		}
		yamlContent += fmt.Sprintf("namespace: %s\n", namespace)
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
		if mkDirErr := AppFs.MkdirAll(filepath.Dir(outputFile), fileutil.ReadWriteExecuteUserReadExecuteOthers); mkDirErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to create output directory: %w", mkDirErr),
			}
		}
		exists, checkErr := afero.Exists(AppFs, outputFile)
		if checkErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to check if output file exists: %w", checkErr),
			}
		}
		if exists {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("output file '%s' already exists", outputFile),
			}
		}
		if writeErr := afero.WriteFile(AppFs, outputFile, []byte(yamlContent), fileutil.ReadWriteUserReadOthers); writeErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write override file: %w", writeErr),
			}
		}
		log.Info("Successfully wrote overrides to %s", outputFile)
	case dryRun:
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- Dry Run: Generated Overrides ---"); err != nil {
			return fmt.Errorf("failed to write dry run header: %w", err) // Wrap error
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), yamlContent); err != nil {
			return fmt.Errorf("failed to write overrides in dry run mode: %w", err) // Wrap error
		}

		// Add validation output to dry run if requested
		if shouldValidate { // Check the derived validation status
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Validation successful! Chart renders correctly with overrides."); err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitGeneralRuntimeError,
					Err:  fmt.Errorf("failed to write validation success message: %w", err),
				}
			}
		}
	default:
		// Default case: Output to stdout when no file is specified and not dry run
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), yamlContent); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write override content to stdout: %w", err),
			}
		}
	}

	return nil
}

// validateChart validates a chart with the generated overrides
func validateChart(cmd *cobra.Command, yamlBytes []byte, config *GeneratorConfig, loadFromPath, loadFromRelease bool, releaseName, namespace string) error {
	// Add nil check for config
	if config == nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  errors.New("internal error: generator config is nil in validateChart"),
		}
	}

	// Create a temporary file to store the overrides
	tempFile, err := afero.TempFile(AppFs, "", "irr-override-*.yaml")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to create temporary file for validation: %w", err),
		}
	}
	// Add explicit nil check, as afero.TempFile might theoretically return (nil, nil)
	if tempFile == nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  errors.New("failed to create temporary file: file handle is nil"),
		}
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			log.Warn("Failed to close temporary file", "error", err)
		}
		if err := AppFs.Remove(tempFile.Name()); err != nil {
			log.Warn("Failed to remove temporary file", "error", err)
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
	// If not specified, use default
	if kubeVersion == "" {
		kubeVersion = DefaultKubernetesVersion
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
		valErr := adapter.ValidateRelease(ctx, releaseName, namespace, []string{tempFile.Name()}, kubeVersion)
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

	log.Info(validationResult)
	return nil
}

// isStdOutRequested returns true if output should go to stdout (either specifically requested or dry-run mode)
func isStdOutRequested(cmd *cobra.Command) bool {
	// Check for dry-run flag
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		log.Warn("Failed to get dry-run flag", "error", err)
		// Continue checking other conditions
	}
	if dryRun {
		return true // Dry run always implies stdout-like behavior (no file write)
	}

	// Check if output-file is explicitly set to "-"
	outputFile, err := cmd.Flags().GetString("output-file")
	if err != nil {
		log.Warn("Failed to get output-file flag", "error", err)
		return false // Cannot determine if stdout requested if flag access fails
	}
	return outputFile == "-"
}
