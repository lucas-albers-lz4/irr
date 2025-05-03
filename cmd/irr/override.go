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
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/lucas-albers-lz4/irr/pkg/strategy"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli/values"
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
)

// Variables for testing - isTestMode declaration REMOVED, it's defined in root.go
/*
var (
	isTestMode = false
)
*/

var (
	validate bool // Declare validate variable
	// contextAware bool // REMOVED redeclaration, assuming declared in inspect.go or elsewhere
)

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

	// Add new flags
	cmd.Flags().BoolVar(&validate, "validate", false, "Run helm template to validate generated overrides")
	cmd.Flags().Bool("context-aware", false, "Use context-aware analyzer that handles subchart value merging (experimental)")
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

func setupPathStrategy(config *GeneratorConfig) error {
	// Add nil check for safety, although runOverride should prevent this call with nil config
	if config == nil {
		return errors.New("internal error: setupPathStrategy called with nil config")
	}
	// The --path-strategy flag has been removed. Always use the default.
	config.Strategy = strategy.NewPrefixSourceRegistryStrategy(config.Mappings)
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

		// Add extra debug logging to help diagnose mapping loading issues
		fileInfo, statErr := AppFs.Stat(configFile)
		if statErr != nil {
			log.Warn("Failed to stat config file", "file", configFile, "error", statErr)
		} else {
			log.Debug("Config file stat successful", "file", configFile, "size", fileInfo.Size(), "isDir", fileInfo.IsDir())
		}
		fileContents, readErr := afero.ReadFile(AppFs, configFile)
		if readErr != nil {
			log.Warn("Failed to read config file contents", "file", configFile, "error", readErr)
		} else {
			log.Debug("Config file read successful", "file", configFile, "contentLength", len(fileContents))
			log.Debug("Config file contents", "content", string(fileContents))
		}

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

		// Add debug logging to show the actual entries
		if config.Mappings != nil && len(config.Mappings.Entries) > 0 {
			for i, entry := range config.Mappings.Entries {
				log.Debug("Mapping entry", "index", i, "source", entry.Source, "target", entry.Target)
			}
		} else {
			log.Warn("No mappings loaded from config file or mappings is nil")
		}
	}
	return nil
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

// Helper to populate values.Options from flags with error checking
func getValuesOptionsFromFlags(cmd *cobra.Command) (values.Options, error) {
	var valueOpts values.Options
	var err error

	valueOpts.ValueFiles, err = getStringSliceFlag(cmd, "values")
	if err != nil {
		return valueOpts, err
	}
	valueOpts.Values, err = getStringSliceFlag(cmd, "set")
	if err != nil {
		return valueOpts, err
	}
	valueOpts.StringValues, err = getStringSliceFlag(cmd, "set-string")
	if err != nil {
		return valueOpts, err
	}
	valueOpts.FileValues, err = getStringSliceFlag(cmd, "set-file")
	if err != nil {
		return valueOpts, err
	}
	return valueOpts, nil
}

// Helper to perform context-aware chart analysis (deduplicates logic)
func performContextAwareAnalysis(chartPath string, valueOpts *values.Options) (*helmchart.Chart, *analysis.ChartAnalysis, error) {
	loaderOptions := &internalhelm.ChartLoaderOptions{
		ChartPath:  chartPath,
		ValuesOpts: *valueOpts,
	}
	chartLoader := internalhelm.NewChartLoader()
	chartAnalysisContext, loadErr := chartLoader.LoadChartAndTrackOrigins(loaderOptions)
	switch {
	case loadErr != nil:
		return nil, nil, &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: fmt.Errorf("failed to load chart with values: %w", loadErr)}
	case chartAnalysisContext == nil:
		return nil, nil, errors.New("internal error: LoadChartAndTrackOrigins returned nil context without error")
	case chartAnalysisContext.Chart == nil:
		return nil, nil, &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: errors.New("failed to load chart details from context")}
	}
	contextAnalyzer := internalhelm.NewContextAwareAnalyzer(chartAnalysisContext)
	chartAnalysis, analyzeErr := contextAnalyzer.AnalyzeContext()
	if analyzeErr != nil {
		return nil, nil, &exitcodes.ExitCodeError{Code: exitcodes.ExitChartProcessingFailed, Err: fmt.Errorf("context analysis failed: %w", analyzeErr)}
	}
	return chartAnalysisContext.Chart, chartAnalysis, nil
}

// createAndExecuteGenerator creates and executes a generator for the given chart source
func createAndExecuteGenerator(cmd *cobra.Command, config *GeneratorConfig, contextAware bool) ([]byte, error) {
	log.Info("Initializing override generation", "chartPath", config.ChartPath)

	var loadedChart *helmchart.Chart
	var analysisResult *analysis.ChartAnalysis
	var loadAnalysisErr error

	valueOpts, err := getValuesOptionsFromFlags(cmd)
	if err != nil {
		return nil, err
	}

	if contextAware {
		log.Info("Performing context-aware chart analysis...")
		loadedChart, analysisResult, loadAnalysisErr = performContextAwareAnalysis(config.ChartPath, &valueOpts)
	} else {
		log.Info("Performing legacy chart analysis...")
		legacyLoader := chart.NewLoader()
		var loadErr error
		var legacyLoadedChart *helmchart.Chart
		legacyLoadedChart, loadErr = legacyLoader.Load(config.ChartPath)
		if loadErr != nil {
			loadAnalysisErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: fmt.Errorf("legacy chart load failed: %w", loadErr)}
		} else {
			loadedChart = legacyLoadedChart
			analyzer := analysis.NewAnalyzer(config.ChartPath, legacyLoader)
			var legacyAnalysisResult *analysis.ChartAnalysis
			legacyAnalysisResult, loadErr = analyzer.Analyze()
			if loadErr != nil {
				loadAnalysisErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartProcessingFailed, Err: fmt.Errorf("legacy analysis failed: %w", loadErr)}
			} else {
				analysisResult = legacyAnalysisResult
			}
		}
	}

	if loadAnalysisErr != nil {
		log.Error("Chart loading/analysis failed", "error", loadAnalysisErr)
		return nil, loadAnalysisErr
	}
	if loadedChart == nil {
		log.Error("Internal error: loadedChart is nil after load/analysis phase without error")
		return nil, &exitcodes.ExitCodeError{Code: exitcodes.ExitGeneralRuntimeError, Err: errors.New("internal error: loadedChart missing")}
	}
	if analysisResult == nil {
		log.Warn("Analysis result is nil (e.g., chart has no values/images), proceeding with empty analysis.")
		analysisResult = analysis.NewChartAnalysis()
	}

	if err := setupPathStrategy(config); err != nil {
		return nil, fmt.Errorf("failed to set up path strategy: %w", err)
	}

	generator, err := createGenerator(config, contextAware)
	if err != nil {
		return nil, err
	}

	log.Debug("Creating generator instance just before NewGenerator call",
		"chartPath", config.ChartPath,
		"targetRegistry", config.TargetRegistry,
		"strategy_type", fmt.Sprintf("%T", config.Strategy),
		"strategy_is_nil", config.Strategy == nil,
		"config_ptr", fmt.Sprintf("%p", config))

	overrideResult, err := generator.Generate(loadedChart, analysisResult)
	if err != nil {
		return nil, handleGenerateError(err)
	}

	yamlBytes, err := yaml.Marshal(overrideResult.Values)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
	}

	return yamlBytes, nil
}

// createGenerator creates a generator based on the context-aware flag.
func createGenerator(config *GeneratorConfig, contextAware bool) (*chart.Generator, error) {
	if config == nil {
		return nil, errors.New("nil generator config")
	}

	// Ensure strategy is initialized
	if config.Strategy == nil {
		var err error
		config.Strategy, err = strategy.GetStrategy("prefix-source-registry", config.Mappings)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize default strategy: %w", err)
		}
		log.Debug("Strategy was nil, set default", "strategy", config.Strategy)
	}

	var preloadedLoader *PreloadedChartLoader
	var generatorErr error

	if contextAware {
		log.Info("Creating generator using context-aware analysis...")
		// --- Context-Aware Path ---
		loaderOptions := &internalhelm.ChartLoaderOptions{
			ChartPath: config.ChartPath,
			// No other options needed for initial load in standalone mode
		}
		chartLoader := internalhelm.NewChartLoader()
		chartAnalysisContext, loadErr := chartLoader.LoadChartAndTrackOrigins(loaderOptions)
		switch {
		case loadErr != nil:
			generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: fmt.Errorf("context-aware chart load failed: %w", loadErr)}
		case chartAnalysisContext == nil:
			generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitInternalError, Err: errors.New("internal error: nil chart context without error")}
		case chartAnalysisContext.Chart == nil:
			generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: errors.New("loaded chart context contains nil chart")}
		default:
			// Chart is loaded, create analyzer
			contextAnalyzer := internalhelm.NewContextAwareAnalyzer(chartAnalysisContext)
			chartAnalysis, analyzeErr := contextAnalyzer.AnalyzeContext()
			if analyzeErr != nil {
				generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartProcessingFailed, Err: fmt.Errorf("context analysis failed: %w", analyzeErr)}
			} else {
				// Analysis completed, prepare preloader
				preloadedLoader = &PreloadedChartLoader{
					chart:    chartAnalysisContext.Chart,
					analysis: chartAnalysis,
				}
			}
		}
	} else {
		log.Info("Creating generator using legacy analysis...")
		// --- Legacy Path ---
		// Use the standard chart loader from pkg/chart
		legacyLoader := chart.NewLoader() // Assuming NewLoader exists in pkg/chart
		var loadedChart *helmchart.Chart
		var analysisResult *analysis.ChartAnalysis
		var loadErr error // Declare loadErr for this block scope
		loadedChart, loadErr = legacyLoader.Load(config.ChartPath)
		if loadErr != nil {
			generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: fmt.Errorf("legacy chart load failed: %w", loadErr)}
		} else {
			analyzer := analysis.NewAnalyzer(config.ChartPath, legacyLoader)
			analysisResult, loadErr = analyzer.Analyze()
			if loadErr != nil {
				generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartProcessingFailed, Err: fmt.Errorf("legacy analysis failed: %w", loadErr)}
			} else {
				// Setup preloaded loader on success
				preloadedLoader = &PreloadedChartLoader{
					chart:    loadedChart,
					analysis: analysisResult,
				}
			}
		}
	}

	if generatorErr != nil {
		return nil, generatorErr
	}

	if preloadedLoader == nil {
		return nil, errors.New("internal error: failed to prepare chart analysis data for generator")
	}

	// Add log before calling NewGenerator
	log.Debug("Creating generator instance just before NewGenerator call",
		"chartPath", config.ChartPath,
		"targetRegistry", config.TargetRegistry,
		"strategy_type", fmt.Sprintf("%T", config.Strategy),
		"strategy_is_nil", config.Strategy == nil,
		"config_ptr", fmt.Sprintf("%p", config))

	// --- Create Override Generator (Common logic) ---
	generator := chart.NewGenerator(
		config.ChartPath,
		config.TargetRegistry,
		config.SourceRegistries,
		config.ExcludeRegistries,
		config.Strategy,
		config.Mappings,
		config.StrictMode,
		0,
		preloadedLoader,
		config.RulesEnabled,
	)

	// Log message if rules are disabled
	if !config.RulesEnabled {
		log.Info("Chart parameter rules system is disabled")
	}

	return generator, nil
}

// PreloadedChartLoader is a custom loader that returns a pre-loaded chart and analysis.
// It implements the chart.Loader interface.
type PreloadedChartLoader struct {
	chart    *helmchart.Chart
	analysis *analysis.ChartAnalysis
}

// Load implements the chart.Loader interface.
func (l *PreloadedChartLoader) Load(_ string) (*helmchart.Chart, error) {
	return l.chart, nil
}

// Analyze implements the analysis.ChartLoader interface.
func (l *PreloadedChartLoader) Analyze(_ string) (*analysis.ChartAnalysis, error) {
	return l.analysis, nil
}

// runOverrideStandaloneMode handles override generation when running in standalone mode.
func runOverrideStandaloneMode(cmd *cobra.Command, outputFile string, dryRun bool) error {
	generatorConfig, err := setupGeneratorConfig(cmd, "")
	if err != nil {
		return err
	}

	// Load registry mappings after setting up the basic config
	if err := loadRegistryMappings(cmd, &generatorConfig); err != nil {
		return err
	}

	// Add debug logging to check if mappings were loaded
	if generatorConfig.Mappings != nil && len(generatorConfig.Mappings.Entries) > 0 {
		log.Debug("Registry mappings loaded correctly in runOverrideStandaloneMode",
			"entries_count", len(generatorConfig.Mappings.Entries))
	} else {
		log.Warn("No registry mappings loaded in runOverrideStandaloneMode")
	}

	contextAware, err := getBoolFlag(cmd, "context-aware")
	if err != nil {
		return err
	}
	yamlBytes, err := createAndExecuteGenerator(cmd, &generatorConfig, contextAware)
	if err != nil {
		return err
	}
	return outputOverrides(cmd, yamlBytes, outputFile, dryRun)
}

// runOverride is the main execution function for the override command
func runOverride(cmd *cobra.Command, args []string) error {
	log.Debug("Executing runOverride")

	outputFile, dryRun, err := getOutputFlags(cmd, "")
	if err != nil {
		return err
	}

	isPlugin := isRunningAsHelmPlugin()
	releaseName := ""

	if isPlugin {
		log.Debug("Running in Helm Plugin mode")
		if len(args) == 0 {
			var getErr error
			releaseName, getErr = getStringFlag(cmd, "release-name")
			if getErr != nil {
				return getErr
			}
			if releaseName == "" {
				return &exitcodes.ExitCodeError{Code: exitcodes.ExitMissingRequiredFlag, Err: errors.New("release name required in plugin mode (provide as argument or --release-name flag)")}
			}
		}
		log.Error("Helm plugin mode execution for override is not yet fully refactored. Please use standalone mode.")
		return &exitcodes.ExitCodeError{Code: exitcodes.ExitGeneralRuntimeError, Err: errors.New("plugin mode not implemented after refactor")}
	}
	log.Debug("Running in Standalone mode")
	return runOverrideStandaloneMode(cmd, outputFile, dryRun)
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
