package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/exitcodes"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/afero"
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
	if _, err := AppFs.Stat("Chart.yaml"); err == nil {
		// Current directory is a chart
		currentDir, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		return currentDir, nil
	}

	// Check if there's a chart directory
	entries, err := afero.ReadDir(AppFs, ".")
	if err != nil {
		return "", fmt.Errorf("failed to read current directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Check if the directory contains Chart.yaml
			chartFile := filepath.Join(entry.Name(), "Chart.yaml")
			if _, err := AppFs.Stat(chartFile); err == nil {
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
func runValidate(cmd *cobra.Command, args []string) error {
	// Get flags
	chartPath, valuesFiles, err := getValidateFlags(cmd)
	if err != nil {
		return err
	}

	// Get release name and namespace if specified
	releaseName, namespace, err := getValidateReleaseNamespace(cmd, args)
	if err != nil {
		return err
	}

	// Get output flags
	outputFile, strict, err := getValidateOutputFlags(cmd)
	if err != nil {
		return err
	}

	// Check if running as plugin with release name
	if releaseName != "" && isHelmPlugin {
		return handleHelmPluginValidate(cmd, releaseName, namespace, valuesFiles, outputFile)
	}

	// Check if chart path exists or is detectable
	chartPath, err = validateAndDetectChartPath(chartPath)
	if err != nil {
		return err
	}

	// Run validation
	templateOutput, err := validateChartWithFiles(chartPath, releaseName, namespace, valuesFiles, strict)
	if err != nil {
		return err
	}

	// Handle output
	return handleValidateOutput(cmd, templateOutput, outputFile)
}

// getValidateFlags retrieves the basic flags for validate command
func getValidateFlags(cmd *cobra.Command) (chartPath string, valuesFiles []string, err error) {
	chartPath, err = cmd.Flags().GetString("chart-path")
	if err != nil {
		return "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
		}
	}

	valuesFiles, err = cmd.Flags().GetStringSlice("values")
	if err != nil {
		return "", nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get values flag: %w", err),
		}
	}

	return chartPath, valuesFiles, nil
}

// getValidateReleaseNamespace retrieves release name and namespace
func getValidateReleaseNamespace(cmd *cobra.Command, args []string) (string, string, error) {
	releaseName, err := cmd.Flags().GetString("release-name")
	if err != nil {
		return "", "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get release-name flag: %w", err),
		}
	}

	// Check for positional argument as release name
	if releaseName == "" && isHelmPlugin && len(args) > 0 {
		releaseName = args[0]
		log.Infof("Using %s as release name from positional argument", releaseName)
	}

	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return "", "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get namespace flag: %w", err),
		}
	}

	return releaseName, namespace, nil
}

// getValidateOutputFlags retrieves output file and strict mode setting
func getValidateOutputFlags(cmd *cobra.Command) (outputFile string, strict bool, err error) {
	outputFile, err = cmd.Flags().GetString("output-file")
	if err != nil {
		return "", false, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-file flag: %w", err),
		}
	}

	strict, err = cmd.Flags().GetBool("strict")
	if err != nil {
		return "", false, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get strict flag: %w", err),
		}
	}

	return outputFile, strict, nil
}

// validateAndDetectChartPath ensures chart path exists or attempts to detect it
func validateAndDetectChartPath(chartPath string) (string, error) {
	if chartPath == "" {
		// Try to detect chart if path is empty
		detectedPath, err := detectChartInCurrentDirectoryIfNeeded("")
		if err != nil {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("chart path not specified and %w", err),
			}
		}
		chartPath = detectedPath
		log.Infof("Detected chart at %s", chartPath)
	}

	// Make path absolute
	absPath, err := filepath.Abs(chartPath)
	if err != nil {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get absolute path for chart: %w", err),
		}
	}
	chartPath = absPath

	// Check if chart path exists
	if _, err := AppFs.Stat(chartPath); err != nil {
		if os.IsNotExist(err) {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("chart path not found: %s", chartPath),
			}
		}
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to access chart path %s: %w", chartPath, err),
		}
	}

	return chartPath, nil
}

// validateChartWithFiles validates a chart with values files
func validateChartWithFiles(chartPath, releaseName, namespace string, valuesFiles []string, _ bool) (string, error) {
	// Set default release name if not provided
	if releaseName == "" {
		releaseName = "irr-validation"
	}

	// Run the validation by executing helm template
	templateOptions := &helm.TemplateOptions{
		ChartPath:   chartPath,
		ReleaseName: releaseName,
		ValuesFiles: valuesFiles,
		Namespace:   namespace,
	}

	// Log namespace if specified
	if namespace != "" {
		log.Debugf("Using namespace '%s' for validation", namespace)
	}

	// Execute Helm template command
	result, err := helm.Template(templateOptions)
	if err != nil {
		log.Errorf("Validation failed: Chart could not be rendered.")
		// Print Helm's stderr for debugging
		if result != nil && result.Stderr != "" {
			fmt.Fprintf(os.Stderr, "--- Helm Error ---\n%s\n------------------\n", result.Stderr)
		}
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("chart validation failed: %w", err),
		}
	}

	log.Infof("Validation successful: Chart rendered successfully with values.")
	return result.Stdout, nil
}

// handleValidateOutput handles the output of the validation result
func handleValidateOutput(cmd *cobra.Command, templateOutput, outputFile string) error {
	// Use switch statement instead of if-else chain
	switch {
	case outputFile != "":
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
		err = AppFs.MkdirAll(filepath.Dir(outputFile), DirPermissions)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to create output directory: %w", err),
			}
		}

		// Write the file
		err = afero.WriteFile(AppFs, outputFile, []byte(templateOutput), FilePermissions)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write output file: %w", err),
			}
		}

		log.Infof("Successfully wrote rendered templates to %s", outputFile)
	case templateOutput != "":
		// Just output to stdout if we have content
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), templateOutput); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write output to stdout: %w", err),
			}
		}
	default:
		// No output - this shouldn't happen but handle it gracefully
		log.Infof("Validation complete. No output was generated.")
	}

	return nil
}

// handleHelmPluginValidate handles validate command when running as a Helm plugin
func handleHelmPluginValidate(cmd *cobra.Command, releaseName, namespace string, valuesFiles []string, _ string) error {
	// Create a new Helm client
	helmClient, err := helm.NewHelmClient()
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to initialize Helm client: %w", err),
		}
	}

	// Create adapter with the Helm client
	adapter := helm.NewAdapter(helmClient, AppFs)

	// Perform the validation operation
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Call the adapter's ValidateRelease method
	err = adapter.ValidateRelease(ctx, releaseName, namespace, valuesFiles)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("validation failed: %w", err),
		}
	}

	// Since we don't have result output in this case, simply log success
	log.Infof("Validation successful! Chart renders correctly with provided values.")

	return nil
}
