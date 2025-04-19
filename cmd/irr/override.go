// Package main implements the command-line interface for the irr (Image Relocation and Rewrite) tool.
// This file contains the override command implementation.
//
// IMPORTANT: This file imports Helm SDK packages that require additional dependencies.
// To resolve the missing go.sum entries, run:
//
//	go get helm.sh/helm/v3@v3.14.2
//
//nolint:unused // These functions are used by tests but flagged as unused by the linter
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
	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"sigs.k8s.io/yaml"
	// Helm SDK imports
)

const (
	// ExitHelmInteractionError is returned when there's an error during Helm SDK interaction
	ExitHelmInteractionError = 17
	// ExitInternalError is returned when there's an internal error in command execution
	ExitInternalError       = 30
	chartSourceTypeChart    = "chart"
	chartSourceTypeRelease  = "release"
	autoDetectedChartSource = "auto-detected"
)

// Variables for testing
var (
	isTestMode = false
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
	// ConfigMappings contains registry mappings from the --config flag
	ConfigMappings map[string]string
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
		Long: "Analyzes a Helm chart to find all container image references (both direct string values " +
			"and map-based structures like 'image.repository', 'image.tag'). It then generates a " +
			"Helm-compatible values file that overrides these references to point to a specified " +
			"target registry, using a defined path strategy.\n\n" +
			"Supports filtering images based on source registries and excluding specific registries. " +
			"Can also utilize a registry mapping file for more complex source-to-target mappings.\n\n" +
			"IMPORTANT NOTES:\n" +
			"- This command can run without a config file, but image redirection correctness depends on your configuration.\n" +
			"- Use 'irr inspect' to identify registries in your chart and 'irr config' to configure mappings.\n" +
			"- When using Harbor as a pull-through cache, ensure your target paths match your Harbor project configuration.",
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

	// Optional flags
	cmd.Flags().StringP("output-file", "o", "", "Write output to file instead of stdout")
	cmd.Flags().StringP("config", "f", "", "Path to registry mapping config file")
	cmd.Flags().Bool("strict", false, "Enable strict mode (fails on unsupported structures)")
	cmd.Flags().StringSlice("include-pattern", []string{}, "Glob patterns for values paths to include (comma-separated)")
	cmd.Flags().StringSlice("exclude-pattern", []string{}, "Glob patterns for values paths to exclude (comma-separated)")
	cmd.Flags().Bool("no-rules", false, "Disable chart parameter rules system")
	cmd.Flags().Bool("dry-run", false, "Show what would be generated without writing to file")
	cmd.Flags().StringSliceP("exclude-registries", "e", []string{}, "Registry URLs to exclude from relocation")
	cmd.Flags().Bool("no-validate", false, "Skip validation of generated overrides")
	cmd.Flags().String("kube-version", "", "Kubernetes version to use for validation (defaults to current client version)")
	cmd.Flags().Bool("validate", true, "Validate generated overrides (use --validate=false to skip)")
	cmd.Flags().String("registry-file", "", "Path to legacy registry mapping file (deprecated, use --config instead)")
	cmd.Flags().StringP("namespace", "n", "default", "Namespace to use (default: default)")
	cmd.Flags().StringP("release-name", "r", "", "Release name to use (only in Helm plugin mode)")

	// Hide deprecated or advanced flags
	cmd.Flags().String("path-strategy", "prefix-source-registry", "Path generation strategy (deprecated, only prefix-source-registry is supported)")
	cmd.Flags().StringSlice("known-image-paths", []string{}, "Advanced: Custom glob patterns for known image paths")

	if err := cmd.Flags().MarkHidden("path-strategy"); err != nil {
		log.Debugf("Failed to mark path-strategy flag as hidden: %v", err)
	}
	if err := cmd.Flags().MarkHidden("known-image-paths"); err != nil {
		log.Debugf("Failed to mark known-image-paths flag as hidden: %v", err)
	}

	// Remove deprecated flags that were already not used
	// --output-format: Not used, always YAML
	// --debug-template: Not implemented/used
	// --threshold: No clear use case; binary success preferred
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
	if outputFile == "" && isHelmPlugin && releaseName != "" {
		outputFile = fmt.Sprintf("%s-overrides.yaml", releaseName)
		debug.Printf("No output file specified in plugin mode, using default based on release name: %s", outputFile)
	}

	// Get dry run flag
	dryRun, err = cmd.Flags().GetBool("dry-run")
	if err != nil {
		return "", false, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get dry-run flag: %w", err),
		}
	}

	debug.Printf("Output flags: outputFile=%s, dryRun=%v", outputFile, dryRun)
	return outputFile, dryRun, nil
}

// outputOverrides outputs the generated overrides to a file or stdout
func outputOverrides(_ *cobra.Command, yamlBytes []byte, outputFile string, dryRun bool) error {
	if dryRun {
		log.Infof("DRY RUN: Generated override values:\n%s", string(yamlBytes))
		return nil
	}

	// If outputFile is empty, write to stdout
	if outputFile == "" {
		fmt.Println(string(yamlBytes))
		log.Infof("Override values printed to stdout")
		return nil
	}

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
		if err := AppFs.MkdirAll(dir, fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to create output directory: %w", err),
			}
		}
	}

	// Write the file
	if err := afero.WriteFile(AppFs, outputFile, yamlBytes, fileutil.ReadWriteUserReadOthers); err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to write output file '%s': %w", outputFile, err),
		}
	}

	// Log success
	absPath, err := filepath.Abs(outputFile)
	if err == nil {
		log.Infof("Override values written to %s", absPath)
	} else {
		log.Infof("Override values written to %s", outputFile)
	}

	return nil
}

// setupGeneratorConfig retrieves and configures all options for the generator
func setupGeneratorConfig(cmd *cobra.Command, _ string) (config GeneratorConfig, err error) {
	chartPath, targetRegistry, sourceRegistries, err := getRequiredFlags(cmd)
	if err != nil {
		return config, err
	}
	config.ChartPath = chartPath
	config.TargetRegistry = targetRegistry
	config.SourceRegistries = sourceRegistries

	excludeRegistries, err := getStringSliceFlag(cmd, "exclude-registries")
	if err != nil {
		return config, err
	}
	config.ExcludeRegistries = excludeRegistries

	if err := setupPathStrategy(cmd, &config); err != nil {
		return config, err
	}

	strictMode, err := getBoolFlag(cmd, "strict")
	if err != nil {
		return config, err
	}
	config.StrictMode = strictMode

	includePatterns, excludePatterns, err := getAnalysisControlFlags(cmd)
	if err != nil {
		return config, err
	}
	config.IncludePatterns = includePatterns
	config.ExcludePatterns = excludePatterns

	disableRules, err := getBoolFlag(cmd, "no-rules")
	if err != nil {
		return config, err
	}
	config.RulesEnabled = !disableRules

	if err := loadRegistryMappings(cmd, &config); err != nil {
		return config, err
	}

	logConfigMode(&config)

	if err := validateUnmappableRegistries(&config); err != nil {
		return config, err
	}

	if len(config.ExcludeRegistries) > 0 {
		log.Infof("Excluding registries: %s", strings.Join(config.ExcludeRegistries, ", "))
	}

	return config, nil
}

func setupPathStrategy(cmd *cobra.Command, config *GeneratorConfig) error {
	pathStrategy, err := cmd.Flags().GetString("path-strategy")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get path-strategy flag: %w", err),
		}
	}
	switch pathStrategy {
	case "prefix-source-registry":
		config.Strategy, err = strategy.GetStrategy(pathStrategy, nil)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitCodeInvalidStrategy,
				Err:  fmt.Errorf("failed to create path strategy: %w", err),
			}
		}
	default:
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitCodeInvalidStrategy,
			Err:  fmt.Errorf("unsupported path strategy: %s", pathStrategy),
		}
	}
	return nil
}

func loadRegistryMappings(cmd *cobra.Command, config *GeneratorConfig) error {
	configFile, err := cmd.Flags().GetString("config")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get config flag: %w", err),
		}
	}
	if configFile != "" {
		debug.Printf("Loading registry mappings from config file: %s", configFile)
		mappings, err := registry.LoadMappings(AppFs, configFile, isTestMode)
		if err != nil {
			if os.IsNotExist(err) {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitInputConfigurationError,
					Err:  fmt.Errorf("config file not found: %s", configFile),
				}
			}
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to load config file: %w", err),
			}
		}
		config.Mappings = mappings
	}
	registryFile, err := cmd.Flags().GetString("registry-file")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get registry-file flag: %w", err),
		}
	}
	if registryFile != "" {
		debug.Printf("Loading registry mappings from registry file: %s", registryFile)
		configMap, err := registry.LoadConfig(AppFs, registryFile, isTestMode)
		if err != nil {
			if os.IsNotExist(err) {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitInputConfigurationError,
					Err:  fmt.Errorf("registry file not found: %s", registryFile),
				}
			}
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to load registry file: %w", err),
			}
		}
		config.ConfigMappings = configMap
	}
	return nil
}

func logConfigMode(config *GeneratorConfig) {
	if config.StrictMode {
		log.Infof("Running in strict mode - will fail on unrecognized registries or unsupported structures")
	} else {
		log.Infof("Running in normal mode - will skip unrecognized registries with warnings")
	}
	if len(config.SourceRegistries) > 0 {
		log.Infof("Using source registries: %s", strings.Join(config.SourceRegistries, ", "))
	}
}

func validateUnmappableRegistries(config *GeneratorConfig) error {
	if len(config.SourceRegistries) == 0 {
		return nil
	}
	if (config.Mappings != nil && len(config.Mappings.Entries) > 0) || len(config.ConfigMappings) > 0 {
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
			if !found && config.ConfigMappings != nil {
				if _, exists := config.ConfigMappings[sourceReg]; exists {
					found = true
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
			log.Warnf("No mapping found for registries: %s", strings.Join(unmappableRegistries, ", "))
			log.Infof("These registries will be redirected using the target registry: %s", config.TargetRegistry)
			log.Infof("To add mappings, use: irr config --source <registry> --target <path>")
			for _, reg := range unmappableRegistries {
				log.Infof("  irr config --source %s --target %s/%s", reg, config.TargetRegistry, strings.ReplaceAll(reg, ".", "-"))
			}
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

	// Check if we have a chart path or need to derive it from release name
	chartSourceDescription := "unknown"
	switch chartSource.SourceType {
	case chartSourceTypeChart:
		chartSourceDescription = chartSource.ChartPath
	case chartSourceTypeRelease:
		chartSourceDescription = fmt.Sprintf("helm-release:%s", chartSource.ReleaseName)
	case autoDetectedChartSource:
		chartSourceDescription = fmt.Sprintf("auto-detected:%s", chartSource.ChartPath)
	}

	log.Infof("Initializing override generator for %s", chartSourceDescription)

	// Create a new generator and run it
	generator, err := createGenerator(chartSource, config)
	if err != nil {
		return nil, err
	}

	// Execute the generator to create the overrides
	log.Infof("Generating override values...")
	overrideFile, err := generator.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate overrides: %w", err)
	}

	// Serialize the overrides to YAML
	yamlBytes, err := yaml.Marshal(overrideFile.Values)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize overrides to YAML: %w", err)
	}

	return yamlBytes, nil
}

// createGenerator creates a generator for the given chart source
func createGenerator(_ *ChartSource, config *GeneratorConfig) (GeneratorInterface, error) {
	// Validate the config
	if config == nil {
		return nil, errors.New("config is nil")
	}

	// Create chart loader instance
	loader := chart.NewLoader()

	// --- Create Override Generator ---
	generator := chart.NewGenerator(
		config.ChartPath,
		config.TargetRegistry,
		config.SourceRegistries,
		config.ExcludeRegistries,
		config.Strategy,
		config.Mappings,
		config.ConfigMappings,
		config.StrictMode,
		0,      // Threshold parameter is not used anymore
		loader, // Use the loader we created
		config.IncludePatterns,
		config.ExcludePatterns,
		nil, // KnownImagePaths parameter is not used anymore
	)

	// Configure rules system
	if config.RulesEnabled {
		generator.SetRulesEnabled(true)
	} else {
		generator.SetRulesEnabled(false)
		log.Infof("Chart parameter rules system is disabled")
	}

	return generator, nil
}

// loadChart loads a Helm chart from the configured source
func loadChart(cs *ChartSource) (*helmchart.Chart, error) {
	if cs == nil {
		return nil, fmt.Errorf("chart source is nil")
	}

	// Check if the file exists when using a physical chart path
	if cs.SourceType == chartSourceTypeChart {
		// Check if the file exists
		if _, err := os.Stat(cs.ChartPath); os.IsNotExist(err) {
			log.Errorf("Chart not found at path: %s", cs.ChartPath)
			return nil, fmt.Errorf("chart not found: %w", err)
		}
	}

	// Create loader using the package function
	loader := chart.NewLoader()

	// Load the chart
	log.Debugf("Loading chart from source: %s", cs.Message)
	c, err := loader.Load(cs.ChartPath)
	if err != nil {
		log.Errorf("Failed to load chart: %v", err)
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	return c, nil
}

// runOverride is the main entry point for the override command
func runOverride(cmd *cobra.Command, args []string) error {
	// Special handling for test mode
	if isTestMode && isHelmPlugin {
		return handleTestModeOverride(cmd, "")
	}

	// Get basic flags first
	chartSource, err := getChartSource(cmd, args)
	if err != nil {
		return err
	}

	// Get configuration for generator
	config, err := setupGeneratorConfig(cmd, chartSource.ReleaseName)
	if err != nil {
		return err
	}

	// Set up the generator based on the chart source
	generator, err := createGenerator(chartSource, &config)
	if err != nil {
		return handleGenerateError(err)
	}

	// Generate the override file
	overrideFile, err := generator.Generate()
	if err != nil {
		return handleGenerateError(err)
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(overrideFile)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal override file to YAML: %w", err),
		}
	}

	// Get output flags
	outputFile, dryRun, err := getOutputFlags(cmd, chartSource.ReleaseName)
	if err != nil {
		return err
	}

	// Output the overrides
	err = outputOverrides(cmd, yamlBytes, outputFile, dryRun)
	if err != nil {
		return err
	}

	// Check if validation should be skipped
	skipValidation, err := cmd.Flags().GetBool("no-validate")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get no-validate flag: %w", err),
		}
	}

	// If we're in dry-run mode or validation is explicitly skipped, stop here
	if dryRun || skipValidation {
		if skipValidation {
			log.Infof("Validation skipped (--no-validate flag provided)")
		}
		return nil
	}

	// If we have an output file, run validation
	if outputFile != "" {
		log.Infof("Validating generated overrides...")
		err = validateChart(cmd, yamlBytes, &config,
			chartSource.SourceType == chartSourceTypeChart,
			chartSource.SourceType == chartSourceTypeRelease,
			chartSource.ReleaseName, chartSource.Namespace)

		if err != nil {
			log.Errorf("Validation failed: %v", err)
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmTemplateFailed,
				Err:  fmt.Errorf("validation of generated overrides failed: %w", err),
			}
		}

		log.Infof("Validation successful")
	} else {
		log.Infof("Skipping validation (no output file)")
	}

	return nil
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
	log.Debugf("handleHelmPluginOverride details: releaseName=%q, namespace=%q, targetRegistry=%q",
		releaseName, namespace, targetRegistry)
	log.Debugf("handleHelmPluginOverride sourceRegistries: %v", config.SourceRegistries)
	log.Debugf("handleHelmPluginOverride pathStrategy: %s", pathStrategy)
	log.Debugf("handleHelmPluginOverride strictMode: %v", config.StrictMode)

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

	// Handle output based on different conditions - pass config parameter
	return handlePluginOverrideOutput(cmd, overrideFile, outputFile, dryRun, releaseName, namespace, config)
}

// handlePluginOverrideOutput handles the output of the override operation
// Add config parameter to function signature
func handlePluginOverrideOutput(cmd *cobra.Command, overrideFile, outputFile string, dryRun bool, releaseName, namespace string, config *GeneratorConfig) error {
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

	// Validate the chart with overrides if requested - pass config parameter
	return validatePluginOverrides(cmd, overrideFile, outputFile, dryRun, releaseName, namespace, config)
}

// validatePluginOverrides validates the generated overrides
func validatePluginOverrides(cmd *cobra.Command, overrideFile, outputFile string, dryRun bool, releaseName, namespace string, config *GeneratorConfig) error {
	shouldValidate, err := cmd.Flags().GetBool("validate")
	if err == nil && shouldValidate {
		// Add nil check for config here as well, though it might be redundant
		if config == nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  errors.New("internal error: generator config is nil during plugin validation"),
			}
		}
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

		// Create a new Helm client and adapter
		adapter, err := createHelmAdapter()
		if err != nil {
			return err
		}

		// Get command context
		ctx := getCommandContext(cmd)

		err = adapter.ValidateRelease(ctx, releaseName, namespace, overrideFiles, kubeVersion)
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
				namespace = validateTestNamespace
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
		if err := AppFs.MkdirAll(filepath.Dir(outputFile), fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
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
		if err := afero.WriteFile(AppFs, outputFile, []byte(yamlContent), fileutil.ReadWriteUserReadOthers); err != nil {
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

	log.Infof(validationResult)
	return nil
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
