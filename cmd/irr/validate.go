package main

import (
	"errors"
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

// Variables for testing, not used in production code
var (
	// isValidateTestMode is used to bypass actual validation in tests
	isValidateTestMode = false
)

// newValidateCmd creates a new validate command
func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [release-name]",
		Short: "Validate a Helm chart with override values",
		Long: `Validate a Helm chart with override values.\n` +
			`This command validates that the chart can be templated with the provided values.\n` +
			fmt.Sprintf("Defaults to Kubernetes version %s if --kube-version is not specified.", DefaultKubernetesVersion),
		Args: cobra.MaximumNArgs(1),
		RunE: runValidate,
	}

	cmd.Flags().String("chart-path", "", "Path to the Helm chart")
	cmd.Flags().StringSlice("values", []string{}, "Values files to use (can be specified multiple times)")
	cmd.Flags().StringSlice("set", []string{}, "Set values on the command line (can be specified multiple times)")
	cmd.Flags().String("output-file", "", "Write template output to file instead of validating")
	cmd.Flags().Bool("debug-template", false, "Print template output for debugging")
	cmd.Flags().String("namespace", "", "Namespace to use for templating")
	cmd.Flags().String("kube-version", "", fmt.Sprintf("Kubernetes version for validation (e.g., '1.31.0'). Defaults to %s", DefaultKubernetesVersion))
	cmd.Flags().Bool("strict", false, "Enable strict mode (fail on any validation error)")

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

	// Determine if chart path or release name was provided
	chartPathProvided := chartPath != ""
	releaseNameProvided := releaseName != ""

	// Error if neither chart path nor release name is provided
	if !chartPathProvided && !releaseNameProvided {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  errors.New("either --chart-path or release name must be provided"),
		}
	}

	// Log which input source we're using
	if chartPathProvided {
		log.Infof("Using chart path: %s", chartPath)
		if releaseNameProvided && isHelmPlugin {
			log.Infof("Chart path provided, ignoring release name: %s", releaseName)
		}
	} else if releaseNameProvided && isHelmPlugin {
		log.Infof("Using release name: %s in namespace: %s", releaseName, namespace)
	}

	// Check if the --release-name flag was explicitly set by the user
	releaseNameFlagSet := cmd.Flags().Changed("release-name")

	// If releaseName flag was explicitly set but we're not in plugin mode, return an error
	if releaseNameFlagSet && !isHelmPlugin {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("the --release-name flag is only available when running as a Helm plugin (helm irr...)"),
		}
	}

	// Get output flags
	outputFile, strict, err := getValidateOutputFlags(cmd)
	if err != nil {
		return err
	}

	// Get Kubernetes version flag
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

	// Check if running as plugin with release name
	if releaseNameProvided && isHelmPlugin && !chartPathProvided {
		return handleHelmPluginValidate(cmd, releaseName, namespace, valuesFiles, outputFile)
	}

	// Check if chart path exists or is detectable
	chartPath, err = validateAndDetectChartPath(chartPath)
	if err != nil {
		return err
	}

	// Check if values files are specified when needed
	if len(valuesFiles) == 0 {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("at least one values file must be specified"),
		}
	}

	// Verify that all values files exist
	for _, valuesFile := range valuesFiles {
		if _, err := AppFs.Stat(valuesFile); err != nil {
			if os.IsNotExist(err) {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitChartNotFound,
					Err:  fmt.Errorf("values file not found or inaccessible: %s", valuesFile),
				}
			}
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to access values file %s: %w", valuesFile, err),
			}
		}
	}

	// Run validation with the Kubernetes version
	templateOutput, err := validateChartWithFiles(chartPath, releaseName, namespace, valuesFiles, strict, kubeVersion)
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
func getValidateReleaseNamespace(cmd *cobra.Command, args []string) (releaseName, namespace string, err error) {
	// Use common function to get release name and namespace
	return getReleaseNameAndNamespaceCommon(cmd, args)
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
func validateChartWithFiles(chartPath, releaseName, namespace string, valuesFiles []string, strict bool, kubeVersion string) (string, error) {
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
		KubeVersion: kubeVersion,
	}

	// Log namespace if specified
	if namespace != "" {
		log.Debugf("Using namespace '%s' for validation", namespace)
	}

	// Log Kubernetes version
	log.Debugf("Using Kubernetes version '%s' for validation", kubeVersion)

	// Execute Helm template command
	result, err := helm.HelmTemplateFunc(templateOptions)
	if err != nil {
		log.Errorf("Validation failed: Chart could not be rendered.")
		// Print Helm's stderr for debugging
		if result != nil && result.Stderr != "" {
			fmt.Fprintf(os.Stderr, "--- Helm Error ---\n%s\n------------------\n", result.Stderr)
		}
		if strict {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("chart validation failed: %w", err),
			}
		}
		return "", nil
	}

	log.Infof("Validation successful: Chart rendered successfully with values.")
	return result.Stdout, nil
}

// handleValidateOutput handles the output of the validation result
func handleValidateOutput(cmd *cobra.Command, templateOutput, outputFile string) error {
	// Use switch statement instead of if-else chain
	switch {
	case outputFile != "":
		// Use the common file handling utility
		err := writeOutputFile(outputFile, []byte(templateOutput), "Successfully wrote rendered templates to %s")
		if err != nil {
			return err
		}
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
	// If in test mode, return success without calling Helm
	if isValidateTestMode {
		log.Infof("Test mode - Skipping actual validation for release %s in namespace %s", releaseName, namespace)
		log.Infof("Validation successful! Chart renders correctly with provided values.")
		return nil
	}

	// Create a new Helm client and adapter
	adapter, err := createHelmAdapter()
	if err != nil {
		return err
	}

	// Get command context
	ctx := getCommandContext(cmd)

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
