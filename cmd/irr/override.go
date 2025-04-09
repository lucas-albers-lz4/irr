package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// newOverrideCmd creates the cobra command for the 'override' operation.
// This command uses centralized exit codes from pkg/exitcodes for consistent error handling:
// - Input validation failures return codes 1-9 (e.g., ExitMissingRequiredFlag)
// - Chart processing issues return codes 10-19 (e.g., ExitUnsupportedStructure)
// - Runtime/system errors return codes 20-29 (e.g., ExitGeneralRuntimeError)
func newOverrideCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "override [flags]",
		Short: "Analyzes a Helm chart and generates image override values",
		Long: "Analyzes a Helm chart to find all container image references (both direct string values " +
			"and map-based structures like 'image.repository', 'image.tag'). It then generates a " +
			"Helm-compatible values file that overrides these references to point to a specified " +
			"target registry, using a defined path strategy.\n\n" +
			"Supports filtering images based on source registries and excluding specific registries. " +
			"Can also utilize a registry mapping file for more complex source-to-target mappings." +
			"Includes options for dry-run, strict validation, and success thresholds.",
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
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

			if len(missingFlags) > 0 {
				sort.Strings(missingFlags) // Sort for consistent error message
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitMissingRequiredFlag,
					Err:  fmt.Errorf("required flag(s) \"%s\" not set", strings.Join(missingFlags, "\", \"")),
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
	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball (required)")
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
	cmd.Flags().Bool("validate", false, "Run 'helm template' with generated overrides to validate chart renderability")

	// Analysis control flags
	cmd.Flags().StringSlice("include-pattern", nil, "Glob patterns for values paths to include during analysis")
	cmd.Flags().StringSlice("exclude-pattern", nil, "Glob patterns for values paths to exclude during analysis")
	cmd.Flags().StringSlice("known-image-paths", nil, "Specific dot-notation paths known to contain images")

	// Mark required flags
	for _, flag := range []string{"chart-path", "target-registry", "source-registries"} {
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
	case errors.Is(err, chart.ErrStrictValidationFailed):
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

		// In dry-run mode, don't actually write the file
		return nil
	}

	// Normal mode - write to file if specified, otherwise stdout
	if outputFile != "" {
		err := AppFs.MkdirAll(filepath.Dir(outputFile), 0755)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to create output directory: %w", err),
			}
		}

		err = afero.WriteFile(AppFs, outputFile, yamlBytes, 0644)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write overrides to file: %w", err),
			}
		}
		debug.Printf("Successfully wrote overrides to file: %s", outputFile)
	} else {
		// Write to stdout
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(yamlBytes)); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write overrides to stdout: %w", err),
			}
		}
	}

	return nil
}

// setupGeneratorConfig collects all the necessary configuration for the generator
func setupGeneratorConfig(cmd *cobra.Command) (
	chartPath string,
	targetRegistry string,
	sourceRegistries []string,
	excludeRegistries []string,
	selectedStrategy strategy.PathStrategy,
	mappings *registry.Mappings,
	strictMode bool,
	threshold int,
	includePattern []string,
	excludePattern []string,
	knownPathsVal []string,
	err error,
) {
	// Get required flags
	chartPath, targetRegistry, sourceRegistries, err = getRequiredFlags(cmd)
	if err != nil {
		return
	}

	// Get registry file and mappings
	registryFile, err := getStringFlag(cmd, "registry-file")
	if err != nil {
		return
	}

	// Load registry mappings
	if registryFile != "" {
		mappings, err = registry.LoadMappings(AppFs, registryFile, integrationTestMode)
		if err != nil {
			debug.Printf("Failed to load mappings: %v", err)
			err = fmt.Errorf("failed to load registry mappings from %s: %w", registryFile, err)
			return
		}
		debug.Printf("Successfully loaded %d mappings from %s", len(mappings.Entries), registryFile)
	}

	// Get and validate path strategy
	pathStrategyString, err := getStringFlag(cmd, "strategy")
	if err != nil {
		return
	}

	// Validate strategy
	selectedStrategy, err = strategy.GetStrategy(pathStrategyString, mappings)
	if err != nil {
		err = &exitcodes.ExitCodeError{
			Code: exitcodes.ExitCodeInvalidStrategy,
			Err:  fmt.Errorf("invalid path strategy specified: %s: %w", pathStrategyString, err),
		}
		return
	}

	// Get remaining flags
	excludeRegistries, err = getStringSliceFlag(cmd, "exclude-registries")
	if err != nil {
		return
	}

	threshold, err = getThresholdFlag(cmd)
	if err != nil {
		return
	}

	strictMode, err = getBoolFlag(cmd, "strict")
	if err != nil {
		return
	}

	// Get analysis control flags
	includePattern, err = getStringSliceFlag(cmd, "include-pattern")
	if err != nil {
		return
	}

	excludePattern, err = getStringSliceFlag(cmd, "exclude-pattern")
	if err != nil {
		return
	}

	knownPathsVal, err = getStringSliceFlag(cmd, "known-image-paths")
	if err != nil {
		return
	}

	return
}

// 3. Runtime Errors (20-29):
//   - File I/O errors (ExitGeneralRuntimeError)
//   - System-level failures (ExitGeneralRuntimeError)
func runOverride(cmd *cobra.Command, _ []string) error {
	debug.FunctionEnter("runOverride")
	defer debug.FunctionExit("runOverride")

	// Get output-related flags
	outputFile, err := getStringFlag(cmd, "output-file")
	if err != nil {
		return err
	}

	dryRun, err := getBoolFlag(cmd, "dry-run")
	if err != nil {
		return err
	}

	// Set up all generator configuration
	chartPath, targetRegistry, sourceRegistries, excludeRegistries,
		selectedStrategy, mappings, strictMode, threshold,
		includePattern, excludePattern, knownPathsVal, err := setupGeneratorConfig(cmd)
	if err != nil {
		return err
	}

	// Create generator
	var loader analysis.ChartLoader = &chart.HelmLoader{}
	generator := currentGeneratorFactory(
		chartPath, targetRegistry,
		sourceRegistries, excludeRegistries,
		selectedStrategy, mappings,
		strictMode,
		threshold,
		loader,
		includePattern,
		excludePattern,
		knownPathsVal,
	)

	// Generate overrides
	overrideFile, err := generator.Generate()
	if err != nil {
		return handleGenerateError(err)
	}

	// Marshal the override file to YAML
	yamlBytes, err := overrideFile.ToYAML()
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal overrides to YAML: %w", err),
		}
	}

	// Handle output based on flags
	return outputOverrides(cmd, yamlBytes, outputFile, dryRun)
}
