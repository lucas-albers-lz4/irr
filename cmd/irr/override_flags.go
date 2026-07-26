package main

import (
	"errors"
	"fmt"

	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/cli/values"
)

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
	cmd.Flags().String("registry-file", "", "Path to YAML file with registry mappings (defaults to registry-mappings.yaml in the current directory if not provided)")
	cmd.Flags().StringP("config", "f", "", "DEPRECATED: Path to registry mapping config file. Use --registry-file instead.")
	if err := cmd.Flags().MarkDeprecated("config", "use --registry-file instead"); err != nil {
		// Log an error if marking deprecated fails, but don't necessarily halt execution
		// This is a development-time issue, not a runtime user error.
		log.Error("Failed to mark --config flag as deprecated", "error", err)
	}
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
	cmd.Flags().String("output-format", outputFormatYAML, "Output format for overrides (yaml or json)")
}

// getRequiredFlags retrieves and validates the required flags for the override command
// It now considers plugin mode (for chartPath) and if a config file is provided (for target/source registries).
func getRequiredFlags(cmd *cobra.Command, isPluginOperatingOnRelease, isConfigProvided bool) (chartPath, targetRegistry string, sourceRegistries []string, err error) {
	chartPath, err = cmd.Flags().GetString("chart-path")
	if err != nil {
		return "", "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
		}
	}
	// Chart path is required ONLY if not in plugin mode operating on a release.
	if !isPluginOperatingOnRelease && chartPath == "" {
		return "", "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("required flag(s) \"chart-path\" not set (or provide a release name in plugin mode)"),
		}
	}

	targetRegistry, err = cmd.Flags().GetString("target-registry")
	if err != nil {
		return "", "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get target-registry flag: %w", err),
		}
	}
	// Target registry is required ONLY if not provided AND no config file is specified.
	if targetRegistry == "" && !isConfigProvided {
		return "", "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("required flag(s) \"target-registry\" not set (or provide a registry mapping file via --registry-file)"),
		}
	}

	sourceRegistries, err = cmd.Flags().GetStringSlice("source-registries")
	if err != nil {
		return "", "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get source-registries flag: %w", err),
		}
	}
	// Source registries are required ONLY if not provided AND no config file is specified.
	if len(sourceRegistries) == 0 && !isConfigProvided {
		return "", "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("required flag(s) \"source-registries\" not set (or provide a registry mapping file via --registry-file)"),
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
