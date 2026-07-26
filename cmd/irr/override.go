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
	"sort"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/chart"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/lucas-albers-lz4/irr/pkg/strategy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	helmchart "helm.sh/helm/v3/pkg/chart"
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
	nilConfigPlaceholder = "<nil config>"
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

			// Determine if a release name is provided (either as a positional argument or via the --release-name flag)
			// This is crucial for deciding if --chart-path is mandatory.
			var releaseNameArg string
			if len(args) > 0 {
				releaseNameArg = args[0]
			}
			releaseNameFlag, flagErr := cmd.Flags().GetString("release-name")
			if flagErr != nil {
				log.Debug("Error getting release-name flag", "error", flagErr)
				// Continue with empty releaseNameFlag rather than returning an error
				// since this check is just for determining if --chart-path is required
				releaseNameFlag = ""
			}
			hasReleaseName := (releaseNameArg != "" || releaseNameFlag != "") && detectedPluginMode

			chartPath, err := cmd.Flags().GetString("chart-path")
			if err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitInputConfigurationError,
					Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
				}
			}
			chartPathProvided := chartPath != ""

			// Get other potentially required flags for validation
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
			configFilePath, configErr := cmd.Flags().GetString("config")
			if configErr != nil {
				log.Debug("Error getting config flag", "error", configErr)
				// Continue with empty configFilePath rather than returning an error
				configFilePath = ""
			}
			isConfigProvided := configFilePath != ""

			var missingFlags []string

			// Chart source check:
			// --chart-path is required if not in plugin mode with a release name.
			if !hasReleaseName && !chartPathProvided {
				missingFlags = append(missingFlags, "chart-path")
			}

			// Target registry check:
			// Required unless a config file is provided (which might define targets through mappings).
			if targetRegistry == "" && !isConfigProvided {
				missingFlags = append(missingFlags, "target-registry")
			}

			// Source registries check:
			// Required unless a config file is provided (which might imply sources through mappings).
			if len(sourceRegistries) == 0 && !isConfigProvided {
				missingFlags = append(missingFlags, "source-registries")
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

// runOverride is the main execution function for the override command
func runOverride(cmd *cobra.Command, args []string) error {
	log.Debug("Executing runOverride")

	outputFile, dryRun, err := getOutputFlags(cmd, "")
	if err != nil {
		return err
	}

	isPlugin := isRunningAsHelmPlugin()
	releaseName := ""
	isPluginOperatingOnRelease := false

	if isPlugin {
		log.Debug("Running in Helm Plugin mode")
		// Parse release name from args or --release-name
		if len(args) > 0 {
			releaseName = args[0]
		} else {
			var getErr error
			releaseName, getErr = getStringFlag(cmd, "release-name")
			if getErr != nil {
				return getErr
			}
			// No explicit error if releaseName is still empty, setupGeneratorConfig will handle it if chart-path also missing
		}

		if releaseName != "" {
			isPluginOperatingOnRelease = true
			// Refine outputFile if it was defaulted based on an empty releaseName initially by getOutputFlags
			if outputFile == "-overrides.yaml" { // This condition checks if getOutputFlags used empty releaseName
				outputFile = fmt.Sprintf("%s-overrides.yaml", releaseName)
				log.Info("Default output file refined in plugin mode with release name", "file", outputFile)
			}
		} else if len(args) == 0 && releaseName == "" {
			// If in plugin mode but no release name (positional or flag), it implies an error or standalone-like usage within plugin context.
			// The PreRunE should ideally catch if chart-path is also missing.
			// For RunE, isPluginOperatingOnRelease remains false, setupGeneratorConfig will require chart-path.
			log.Debug("Plugin mode detected, but no release name provided. Chart path will be required.")
		}

		// Determine namespace with correct precedence:
		// 1. Explicitly set --namespace flag
		// 2. HELM_NAMESPACE environment variable
		// 3. Default to "default"
		var namespace string
		namespaceFlag := cmd.Flag("namespace") // Get the pflag.Flag object

		if namespaceFlag != nil && namespaceFlag.Changed {
			// User explicitly set the -n or --namespace flag
			namespace = namespaceFlag.Value.String()
			log.Debug("Using namespace from explicitly set flag", "namespace", namespace)
		} else {
			// Flag was not set by user, try HELM_NAMESPACE
			envNamespace := os.Getenv("HELM_NAMESPACE")
			if envNamespace != "" {
				namespace = envNamespace
				log.Debug("Using namespace from HELM_NAMESPACE environment variable", "namespace", namespace)
			} else {
				// Fallback to "default" if neither flag nor env var is set
				namespace = defaultNamespace
				log.Debug("Falling back to default namespace", "namespace", namespace)
			}
		}

		// Get Helm adapter
		helmAdapter, errAdapter := helmAdapterFactory()
		if errAdapter != nil {
			return errAdapter
		}
		if helmAdapter == nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  errors.New("internal error: helmAdapterFactory returned nil adapter without error"),
			}
		}

		// Fetch release values and chart metadata
		releaseValues, errValues := helmAdapter.GetReleaseValues(cmd.Context(), releaseName, namespace)
		if errValues != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("failed to get values for release %s in namespace %s: %w", releaseName, namespace, errValues),
			}
		}
		chartMetadata, errChartMeta := helmAdapter.GetChartFromRelease(cmd.Context(), releaseName, namespace)
		if errChartMeta != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("failed to get chart info for release %s in namespace %s: %w", releaseName, namespace, errChartMeta),
			}
		}

		// Prepare minimal chart object for generator
		dummyChart := &helmchart.Chart{
			Metadata: &helmchart.Metadata{
				Name:    chartMetadata.Name,
				Version: chartMetadata.Version,
			},
		}

		// Prepare analysis result using context-aware analyzer
		analyzer := analysis.NewAnalyzer("", nil) // No chart path, no loader needed for direct values
		analysisResult, analyzeErr := analyzer.AnalyzeValues(releaseValues)
		if analyzeErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartProcessingFailed,
				Err:  fmt.Errorf("release values analysis failed: %w", analyzeErr),
			}
		}

		// Prepare generator config (reuse flag parsing logic)
		generatorConfig, err := setupGeneratorConfig(cmd, isPluginOperatingOnRelease)
		if err != nil {
			return err
		}
		// Set/override chart path for plugin mode if operating on a release
		if isPluginOperatingOnRelease {
			generatorConfig.ChartPath = fmt.Sprintf("helm-release://%s/%s", namespace, releaseName)
		}

		if err := loadRegistryMappings(cmd, &generatorConfig); err != nil {
			return err
		}

		// Derive source registries from mappings if not explicitly provided.
		deriveSourceRegistriesFromMappings(&generatorConfig)

		pathStrategy, err := setupPathStrategy(&generatorConfig)
		if err != nil {
			return err
		}
		generatorConfig.Strategy = pathStrategy

		generator := chart.NewGenerator(
			generatorConfig.ChartPath,
			generatorConfig.TargetRegistry,
			generatorConfig.SourceRegistries,
			generatorConfig.ExcludeRegistries,
			generatorConfig.Strategy,
			generatorConfig.Mappings,
			generatorConfig.StrictMode,
			0,
			&PreloadedChartLoader{chart: dummyChart, analysis: analysisResult},
			generatorConfig.RulesEnabled,
		)

		overrideResult, err := generator.Generate(dummyChart, analysisResult)
		if err != nil {
			return handleGenerateError(err)
		}
		yamlBytes, err := yaml.Marshal(overrideResult.Values)
		if err != nil {
			return fmt.Errorf("failed to marshal overrides to YAML: %w", err)
		}
		return outputOverrides(cmd, yamlBytes, outputFile, dryRun)
	}
	log.Debug("Running in Standalone mode")
	return runOverrideStandaloneMode(cmd, outputFile, dryRun, false)
}

