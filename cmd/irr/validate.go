package main

import (
	"fmt"
	"os"
	"path/filepath"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/strvals"

	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/cobra"
)

// newValidateCmd creates a new validate command
func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a Helm chart with override values",
		Long: `Validate a Helm chart with override values.
This command validates that the chart can be templated with the provided values.`,
		RunE: runValidate,
	}

	cmd.Flags().String("chart-path", "", "Path to the Helm chart")
	cmd.Flags().String("release-name", "release", "Release name to use for templating")
	cmd.Flags().StringSlice("values", []string{}, "Values files to use (can be specified multiple times)")
	cmd.Flags().StringSlice("set", []string{}, "Set values on the command line (can be specified multiple times)")
	cmd.Flags().String("output-file", "", "Write template output to file instead of validating")
	cmd.Flags().Bool("debug-template", false, "Print template output for debugging")
	cmd.Flags().String("namespace", "", "Namespace to use for templating")

	return cmd
}

// detectChartInCurrentDirectoryIfNeeded attempts to find a Helm chart if chart path is not specified
func detectChartInCurrentDirectoryIfNeeded(chartPath string) (string, error) {
	if chartPath != "" {
		return chartPath, nil
	}

	// Check if Chart.yaml exists in current directory
	if _, err := os.Stat("Chart.yaml"); err == nil {
		// Current directory is a chart
		currentDir, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		return currentDir, nil
	}

	// Check if there's a chart directory
	entries, err := os.ReadDir(".")
	if err != nil {
		return "", fmt.Errorf("failed to read current directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Check if the directory contains Chart.yaml
			chartFile := filepath.Join(entry.Name(), "Chart.yaml")
			if _, err := os.Stat(chartFile); err == nil {
				// Found a chart directory
				chartPath, err := filepath.Abs(entry.Name())
				if err != nil {
					return "", fmt.Errorf("failed to get absolute path for chart: %w", err)
				}
				return chartPath, nil
			}
		}
	}

	return "", fmt.Errorf("no Helm chart found in current directory")
}

// getChartPath gets and validates the chart path from command flags
func getChartPath(cmd *cobra.Command) (string, error) {
	chartPath, err := cmd.Flags().GetString("chart-path")
	if err != nil {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
		}
	}

	// Try to detect chart if not specified
	if chartPath == "" {
		chartPath, err = detectChartInCurrentDirectoryIfNeeded("")
		if err != nil {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("chart path not specified and %w", err),
			}
		}
		log.Infof("Detected chart at %s", chartPath)
		return chartPath, nil
	}

	// Normalize chart path
	chartPath, err = filepath.Abs(chartPath)
	if err != nil {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get absolute path for chart: %w", err),
		}
	}

	// Check if chart exists
	_, err = os.Stat(chartPath)
	if err != nil {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartNotFound,
			Err:  fmt.Errorf("chart path not found or inaccessible: %s", chartPath),
		}
	}

	return chartPath, nil
}

// getValuesFiles gets and validates the values files from command flags
func getValuesFiles(cmd *cobra.Command) ([]string, error) {
	valuesFiles, err := cmd.Flags().GetStringSlice("values")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get values flag: %w", err),
		}
	}

	if len(valuesFiles) == 0 {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitMissingRequiredFlag,
			Err:  fmt.Errorf("at least one values file must be specified with --values"),
		}
	}

	// Check that values files exist
	for _, valueFile := range valuesFiles {
		_, err := os.Stat(valueFile)
		if err != nil {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("values file not found or inaccessible: %s", valueFile),
			}
		}
	}

	return valuesFiles, nil
}

// loadAndMergeValues loads values from files and merges them with set values
func loadAndMergeValues(valuesFiles, setValues []string) (map[string]interface{}, error) {
	finalValues := map[string]interface{}{}

	// Load values from files
	for _, valueFile := range valuesFiles {
		currentValues, err := chartutil.ReadValuesFile(valueFile)
		if err != nil {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartLoadFailed,
				Err:  fmt.Errorf("failed to read values file %s: %w", valueFile, err),
			}
		}
		finalValues = chartutil.CoalesceTables(finalValues, currentValues.AsMap())
	}

	// Handle set values
	for _, value := range setValues {
		if err := strvals.ParseInto(value, finalValues); err != nil {
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to parse set value %q: %w", value, err),
			}
		}
	}

	return finalValues, nil
}

// handleOutput handles the output of the template validation
func handleOutput(outputFile string, debugTemplate bool, manifest string) error {
	switch {
	case outputFile != "":
		if err := os.WriteFile(outputFile, []byte(manifest), fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write template output to file: %w", err),
			}
		}
		log.Infof("Template output written to %s", outputFile)
	case debugTemplate:
		fmt.Println(manifest)
	default:
		log.Infof("Helm template validation completed successfully")
	}
	return nil
}

// runValidate implements the validate command logic
func runValidate(cmd *cobra.Command, _ []string) error {
	// Get chart path
	chartPath, err := getChartPath(cmd)
	if err != nil {
		return err
	}

	// Get values files
	valuesFiles, err := getValuesFiles(cmd)
	if err != nil {
		return err
	}

	// Get other flags
	releaseName, err := cmd.Flags().GetString("release-name")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get release-name flag: %w", err),
		}
	}

	setValues, err := cmd.Flags().GetStringSlice("set")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get set flag: %w", err),
		}
	}

	outputFile, err := cmd.Flags().GetString("output-file")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-file flag: %w", err),
		}
	}

	debugTemplate, err := cmd.Flags().GetBool("debug-template")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get debug-template flag: %w", err),
		}
	}

	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get namespace flag: %w", err),
		}
	}

	// Initialize Helm environment
	settings := cli.New()

	// Create action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, "", log.Infof); err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to initialize Helm action config: %w", err),
		}
	}

	// Load chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed,
			Err:  fmt.Errorf("failed to load chart: %w", err),
		}
	}

	// Load and merge values
	finalValues, err := loadAndMergeValues(valuesFiles, setValues)
	if err != nil {
		return err
	}

	// Create install action for validation
	install := action.NewInstall(actionConfig)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.DryRun = true
	install.ClientOnly = true

	// Run template validation
	rel, err := install.Run(chart, finalValues)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("helm template validation failed: %w", err),
		}
	}

	return handleOutput(outputFile, debugTemplate, rel.Manifest)
}
