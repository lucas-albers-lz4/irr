package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/cobra"
)

// newValidateCmd creates a new validate command
func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [flags]",
		Short: "Validate a Helm chart with override values",
		Long: "Validates a Helm chart after applying override values files. " +
			"This command executes 'helm template' with the specified values to ensure " +
			"that the chart can be successfully rendered with the provided overrides.",
		Args: cobra.NoArgs,
		RunE: runValidate,
	}

	// Add flags
	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball (default: auto-detect)")
	cmd.Flags().StringP("release-name", "n", "release", "Release name to use for templating")
	cmd.Flags().StringSliceP("values", "f", []string{}, "One or more values files to use for validation (can be specified multiple times)")
	cmd.Flags().StringSlice("set", []string{}, "Set values on the command line (can be specified multiple times)")
	cmd.Flags().StringP("output-file", "o", "", "Output file for helm template result (default: discard)")
	cmd.Flags().Bool("debug-template", false, "Show full helm template output even on success")
	cmd.Flags().String("namespace", "", "Namespace to use for release (only when used with --release-name)")

	// Mark required flags
	mustMarkFlagRequired(cmd, "values")

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

// runValidate implements the validate command logic
func runValidate(cmd *cobra.Command, _ []string) error {
	// Get chart path
	chartPath, err := cmd.Flags().GetString("chart-path")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
		}
	}

	// Try to detect chart if not specified
	if chartPath == "" {
		chartPath, err = detectChartInCurrentDirectoryIfNeeded("")
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("chart path not specified and %w", err),
			}
		}
		log.Infof("Detected chart at %s", chartPath)
	} else {
		// Normalize chart path
		chartPath, err = filepath.Abs(chartPath)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("failed to get absolute path for chart: %w", err),
			}
		}

		// Check if chart exists
		_, err = os.Stat(chartPath)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("chart path not found or inaccessible: %s", chartPath),
			}
		}
	}

	// Get release name
	releaseName, err := cmd.Flags().GetString("release-name")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get release-name flag: %w", err),
		}
	}

	// Get values files
	valuesFiles, err := cmd.Flags().GetStringSlice("values")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get values flag: %w", err),
		}
	}

	if len(valuesFiles) == 0 {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitMissingRequiredFlag,
			Err:  fmt.Errorf("at least one values file must be specified with --values"),
		}
	}

	// Check that values files exist
	for _, valueFile := range valuesFiles {
		_, err := os.Stat(valueFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("values file not found or inaccessible: %s", valueFile),
			}
		}
	}

	// Get set values
	setValues, err := cmd.Flags().GetStringSlice("set")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get set flag: %w", err),
		}
	}

	// Get output file
	outputFile, err := cmd.Flags().GetString("output-file")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-file flag: %w", err),
		}
	}

	// Get debug-template flag
	debugTemplate, err := cmd.Flags().GetBool("debug-template")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get debug-template flag: %w", err),
		}
	}

	// Get namespace
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get namespace flag: %w", err),
		}
	}

	// Check if we need to get values from a release
	// If release-name is specified and doesn't equal the default, attempt to get values
	if releaseName != "release" {
		log.Infof("Getting values for release %s", releaseName)

		// Get values from the release
		valuesResult, err := helm.GetValues(&helm.GetValuesOptions{
			ReleaseName: releaseName,
			Namespace:   namespace,
		})

		if err != nil {
			log.Warnf("Failed to get values for release %s: %v", releaseName, err)
			log.Warnf("Continuing with validation using only provided values files")
		} else {
			// Write release values to temporary file
			tempValuesFile, err := os.CreateTemp("", "release-values-*.yaml")
			if err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("failed to create temporary file for release values: %w", err),
				}
			}
			defer func() {
				if err := os.Remove(tempValuesFile.Name()); err != nil {
					log.Warnf("Failed to remove temporary file %s: %v", tempValuesFile.Name(), err)
				}
			}()

			if _, err := tempValuesFile.WriteString(valuesResult.Stdout); err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("failed to write release values to temporary file: %w", err),
				}
			}

			if err := tempValuesFile.Close(); err != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("failed to close temporary file: %w", err),
				}
			}

			// Add release values file to the beginning of the values files list
			valuesFiles = append([]string{tempValuesFile.Name()}, valuesFiles...)
			log.Infof("Added release values from %s to validation", releaseName)
		}
	}

	// Execute helm template command
	result, err := helm.Template(&helm.TemplateOptions{
		ReleaseName: releaseName,
		ChartPath:   chartPath,
		ValuesFiles: valuesFiles,
		SetValues:   setValues,
	})

	// Check if command succeeded
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("helm template failed: %w", err),
		}
	}

	// Handle output
	switch {
	case outputFile != "":
		if err := os.WriteFile(outputFile, []byte(result.Stdout), fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write template output to file: %w", err),
			}
		}
		log.Infof("Template output written to %s", outputFile)
	case debugTemplate:
		fmt.Println(result.Stdout)
	default:
		log.Infof("Helm template completed successfully")
	}

	return nil
}
