package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/cobra"
)

// DefaultKubernetesVersion defines the default K8s version used for validation
const DefaultKubernetesVersion = "1.31.0"

// newValidateCmd creates a new validate command
func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a Helm chart with override values",
		Long: `Validate a Helm chart with override values.\n` +
			`This command validates that the chart can be templated with the provided values.\n` +
			fmt.Sprintf("Defaults to Kubernetes version %s if --kube-version is not specified.", DefaultKubernetesVersion),
		RunE: runValidate,
	}

	cmd.Flags().String("chart-path", "", "Path to the Helm chart")
	cmd.Flags().String("release-name", "release", "Release name to use for templating")
	cmd.Flags().StringSlice("values", []string{}, "Values files to use (can be specified multiple times)")
	cmd.Flags().StringSlice("set", []string{}, "Set values on the command line (can be specified multiple times)")
	cmd.Flags().String("output-file", "", "Write template output to file instead of validating")
	cmd.Flags().Bool("debug-template", false, "Print template output for debugging")
	cmd.Flags().String("namespace", "", "Namespace to use for templating")
	cmd.Flags().String("kube-version", "", fmt.Sprintf("Kubernetes version for validation (e.g., '1.31.0'). Defaults to %s", DefaultKubernetesVersion))

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

	kubeVersion, err := cmd.Flags().GetString("kube-version")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get kube-version flag: %w", err),
		}
	}
	if kubeVersion == "" {
		kubeVersion = DefaultKubernetesVersion
		log.Debugf("No --kube-version specified, using default: %s", DefaultKubernetesVersion)
	}

	log.Infof("Validating chart %s with release name %s", chartPath, releaseName)

	// Prepare options for helm template
	templateOptions := &helm.TemplateOptions{
		ReleaseName: releaseName,
		ChartPath:   chartPath,
		ValuesFiles: valuesFiles,
		SetValues:   setValues,
		Namespace:   namespace,
		KubeVersion: kubeVersion,
	}

	// Execute helm template
	result, err := helm.HelmTemplateFunc(templateOptions)
	if err != nil {
		// Attempt to return a more specific exit code if possible
		// Extract the error message for matching
		errStr := err.Error()
		var exitCode int
		var exitErr error

		switch {
		case strings.Contains(errStr, "chart path not found"):
			exitCode = exitcodes.ExitChartNotFound
			exitErr = err
		case strings.Contains(errStr, "failed to load chart") || strings.Contains(errStr, "failed to read values file"):
			exitCode = exitcodes.ExitChartLoadFailed
			exitErr = err
		case strings.Contains(errStr, "failed to parse set value"):
			exitCode = exitcodes.ExitInputConfigurationError
			exitErr = err
		case strings.Contains(errStr, "failed to template chart") || strings.Contains(errStr, "Helm template failed"):
			exitCode = exitcodes.ExitHelmTemplateFailed
			exitErr = err
		default:
			// Generic Helm error
			exitCode = exitcodes.ExitHelmCommandFailed
			exitErr = err
		}

		return &exitcodes.ExitCodeError{
			Code: exitCode,
			Err:  exitErr,
		}
	}

	// Check result (although Template should return error on failure)
	if !result.Success {
		// This path might be less likely now with SDK error handling, but keep for robustness
		log.Errorf("Helm template validation failed: %s", result.Stderr)
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmTemplateFailed,
			Err:  fmt.Errorf("helm template command failed: %s", result.Stderr),
		}
	}

	// Handle output (write to file or log success)
	return handleOutput(outputFile, debugTemplate, result.Stdout)
}
