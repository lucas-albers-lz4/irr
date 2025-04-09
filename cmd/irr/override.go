package main

import (
	"errors"
	"fmt"
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
		PreRunE: func(cmd *cobra.Command, args []string) error {
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

	// Required flags
	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball (required)")
	cmd.Flags().StringP("target-registry", "t", "", "Target container registry URL (required)")
	cmd.Flags().StringSliceP("source-registries", "s", []string{}, "Source container registry URLs to relocate (required, comma-separated or multiple flags)")

	// Optional flags with defaults
	cmd.Flags().StringP("output-file", "o", "", "Output file path for the generated overrides YAML (default: stdout)")
	cmd.Flags().StringP("strategy", "p", "prefix-source-registry", "Path generation strategy ('prefix-source-registry')")
	cmd.Flags().Bool("dry-run", false, "Perform analysis and print overrides to stdout without writing to file")
	cmd.Flags().Bool("strict", false, "Enable strict mode (fail on any image parsing/processing error)")
	cmd.Flags().StringSlice("exclude-registries", []string{}, "Container registry URLs to exclude from relocation (comma-separated or multiple flags)")
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

	return cmd
}

// runOverride implements the logic for the override command.
// Error handling follows a consistent pattern using pkg/exitcodes.ExitCodeError:
// 1. Input/Config Errors (1-9):
//   - Missing required flags (ExitMissingRequiredFlag)
//   - Invalid flag values (ExitInputConfigurationError)
//   - Invalid strategy (ExitCodeInvalidStrategy)
//
// 2. Chart Processing Errors (10-19):
//   - Chart parsing failures (ExitChartParsingError)
//   - Image processing issues (ExitImageProcessingError)
//   - Unsupported structures in strict mode (ExitUnsupportedStructure)
//   - Threshold failures (ExitThresholdError)
//
// 3. Runtime Errors (20-29):
//   - File I/O errors (ExitGeneralRuntimeError)
//   - System-level failures (ExitGeneralRuntimeError)
func runOverride(cmd *cobra.Command, args []string) error {
	debug.FunctionEnter("runOverride")
	defer debug.FunctionExit("runOverride")

	// Get validated flag values
	chartPath, err := cmd.Flags().GetString("chart-path")
	if err != nil || chartPath == "" {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("required flag(s) \"chart-path\" not set"),
		}
	}

	targetRegistry, err := cmd.Flags().GetString("target-registry")
	if err != nil || targetRegistry == "" {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("required flag(s) \"target-registry\" not set"),
		}
	}

	sourceRegistries, err := cmd.Flags().GetStringSlice("source-registries")
	if err != nil || len(sourceRegistries) == 0 {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("required flag(s) \"source-registries\" not set"),
		}
	}

	// Get optional flags with error handling
	outputFile, err := cmd.Flags().GetString("output-file")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-file flag: %w", err),
		}
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get dry-run flag: %w", err),
		}
	}

	registryFile, err := cmd.Flags().GetString("registry-file")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get registry-file flag: %w", err),
		}
	}

	pathStrategy, err := cmd.Flags().GetString("strategy")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get strategy flag: %w", err),
		}
	}

	excludeRegistries, err := cmd.Flags().GetStringSlice("exclude-registries")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get exclude-registries flag: %w", err),
		}
	}

	threshold, err := cmd.Flags().GetInt("threshold")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get threshold flag: %w", err),
		}
	}

	if threshold < 0 || threshold > 100 {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("threshold must be between 0 and 100: invalid threshold value: %d", threshold),
		}
	}

	strictMode, err := cmd.Flags().GetBool("strict")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get strict flag: %w", err),
		}
	}

	// Load registry mappings
	var mappings *registry.Mappings
	var loadMappingsErr error
	if registryFile != "" {
		mappings, loadMappingsErr = registry.LoadMappings(AppFs, registryFile, integrationTestMode)
		if loadMappingsErr != nil {
			debug.Printf("Failed to load mappings: %v", loadMappingsErr)
			return fmt.Errorf("failed to load registry mappings from %s: %w", registryFile, loadMappingsErr)
		}
		debug.Printf("Successfully loaded %d mappings from %s", len(mappings.Entries), registryFile)
	}

	// Validate strategy
	selectedStrategy, strategyErr := strategy.GetStrategy(pathStrategy, mappings)
	if strategyErr != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitCodeInvalidStrategy,
			Err:  fmt.Errorf("invalid path strategy specified: %s: %w", pathStrategy, strategyErr),
		}
	}

	// Get analysis control flags
	includePattern, err := cmd.Flags().GetStringSlice("include-pattern")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get include-pattern flag: %w", err),
		}
	}

	excludePattern, err := cmd.Flags().GetStringSlice("exclude-pattern")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get exclude-pattern flag: %w", err),
		}
	}

	knownPathsVal, err := cmd.Flags().GetStringSlice("known-image-paths")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get known-image-paths flag: %w", err),
		}
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

	// Handle output
	if dryRun {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- Dry Run: Generated Overrides ---"); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write to output: %w", err),
			}
		}
		yamlBytes, err := overrideFile.ToYAML()
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal overrides to YAML for dry run: %w", err),
			}
		}
		if _, err := fmt.Fprint(cmd.OutOrStdout(), string(yamlBytes)); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write YAML to output: %w", err),
			}
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "--- End Dry Run ---"); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write to output: %w", err),
			}
		}
		return nil
	}

	// Write output
	yamlBytes, err := overrideFile.ToYAML()
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal overrides to YAML: %w", err),
		}
	}

	if outputFile == "" {
		if _, err := fmt.Fprint(cmd.OutOrStdout(), string(yamlBytes)); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write YAML to output: %w", err),
			}
		}
	} else {
		if err := afero.WriteFile(AppFs, outputFile, yamlBytes, 0o644); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write overrides to file %s: %w", outputFile, err),
			}
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Overrides written to: %s\n", outputFile); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write success message: %w", err),
			}
		}
	}

	return nil
}
