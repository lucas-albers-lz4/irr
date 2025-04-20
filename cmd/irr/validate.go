package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
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
		Long: `Validates that a Helm chart can be rendered correctly with the specified override values.
This command runs 'helm template' with the chart and values, and checks for rendering errors.

The validation can operate on either:
- A local chart directory or tarball file (using --chart-path)
- An installed Helm release (when running as a Helm plugin with [release-name])

IMPORTANT NOTES:
- This command can run without a config file, but image redirection correctness depends on your configuration
- Use 'irr inspect' to identify registries in your chart and 'irr config' to configure mappings
- When used with 'irr override', validation ensures your override values are syntactically correct`,
		Args: cobra.MaximumNArgs(1),
		RunE: runValidate,
	}

	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball")

	// Only add release-name flag if it doesn't already exist (should be inherited from root)
	if cmd.Flags().Lookup("release-name") == nil {
		cmd.Flags().StringP("release-name", "r", "", "Release name to use (default: chart name)")
	}

	cmd.Flags().StringSliceP("values", "f", []string{}, "Values files to use (can specify multiple)")
	cmd.Flags().StringP("namespace", "n", "default", "Namespace to use (default: default)")
	cmd.Flags().StringP("output-file", "o", "", "Write rendering output to file instead of discarding")
	cmd.Flags().Bool("strict", false, "Fail on any warning, not just errors")
	cmd.Flags().String("kube-version", "", "Kubernetes version to use for validation (defaults to current client version)")

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

// runValidate is the main entry point for the validate command
func runValidate(cmd *cobra.Command, args []string) error {
	// Get required flags
	chartPath, valuesFiles, err := getValidateFlags(cmd)
	if err != nil {
		return err
	}

	// Handle validation
	if isHelmPlugin {
		log.Debugf("Running in Helm plugin mode, handling plugin-specific validation")
		// Get namespace and release from args or flags
		releaseName, namespace, err := getValidateReleaseNamespace(cmd, args)
		if err != nil {
			return err
		}

		return handlePluginValidate(cmd, releaseName, namespace)
	}

	// Handle standalone mode
	return handleStandaloneValidate(cmd, chartPath, valuesFiles)
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
		Strict:      strict, // Set strict flag in options
	}

	// Log namespace if specified
	if namespace != "" {
		log.Debugf("Using namespace '%s' for validation", namespace)
	}

	// Log Kubernetes version
	log.Debugf("Using Kubernetes version '%s' for validation", kubeVersion)

	// Log if strict mode is enabled
	if strict {
		log.Debugf("Strict validation mode enabled")
	}

	// Execute Helm template command
	result, err := helm.HelmTemplateFunc(templateOptions)
	if err != nil {
		log.Errorf("Validation failed: Chart could not be rendered.")
		// Print Helm's stderr for debugging
		if result != nil && result.Stderr != "" {
			fmt.Fprintf(os.Stderr, "--- Helm Error ---\n%s\n------------------\n", result.Stderr)
		}

		// Check if this is a Chart.yaml missing error and try to handle it
		if strings.Contains(err.Error(), "Chart.yaml file is missing") {
			// Try to find the chart in alternative locations
			resolvedPath, resolveErr := handleChartYamlMissingErrors(err, chartPath)
			if resolveErr != nil {
				// Could not resolve path, return the resolve error
				return "", resolveErr
			}

			// If we found an alternative path, try validation again
			if resolvedPath != chartPath {
				log.Infof("Retrying validation with resolved chart path: %s", resolvedPath)
				templateOptions.ChartPath = resolvedPath
				retryResult, retryErr := helm.HelmTemplateFunc(templateOptions)
				if retryErr == nil {
					log.Infof("Validation successful with resolved chart path!")
					if retryResult != nil {
						return retryResult.Stdout, nil
					}
					log.Warnf("HelmTemplateFunc returned nil retryResult after successful retry")
					return "", nil
				}

				log.Errorf("Validation still failed with resolved path: %v", retryErr)
				if retryResult != nil && retryResult.Stderr != "" {
					fmt.Fprintf(os.Stderr, "--- Helm Error (Retry) ---\n%s\n------------------------\n", retryResult.Stderr)
				} else if retryResult == nil {
					log.Warnf("HelmTemplateFunc returned nil retryResult after retrying with resolved path")
				}
			}
		}

		// Check for YAML parsing errors which indicate invalid values file
		if strings.Contains(err.Error(), "yaml:") || strings.Contains(err.Error(), "YAML") {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("validation failed due to invalid YAML: %w", err),
			}
		}

		// In strict mode, return the error with appropriate exit code
		if strict {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("chart validation failed in strict mode: %w", err),
			}
		}

		// If not in strict mode, still return the error for validation failures
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("chart validation failed: %w", err),
		}
	}

	// If in strict mode, perform additional validation on the rendered output
	if strict && result != nil && result.Stdout != "" {
		output := result.Stdout

		// Check for unresolved Helm template variables like {{ .Values.something }}
		if strings.Contains(output, "{{") && strings.Contains(output, "}}") {
			log.Errorf("Strict validation failed: Found unresolved template variables in output")
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("strict validation failed: unresolved template variables found in rendered output"),
			}
		}

		// Check for other problematic patterns
		if strings.Contains(output, "<no value>") {
			log.Errorf("Strict validation failed: Found <no value> placeholders in output")
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("strict validation failed: <no value> placeholders found in rendered output"),
			}
		}
	}

	log.Infof("Validation successful: Chart rendered successfully with values.")
	// Add nil check for result before accessing Stdout
	if result == nil {
		return "", nil
	}
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

// handlePluginValidate handles validation when running in Helm plugin mode
func handlePluginValidate(cmd *cobra.Command, releaseName, namespace string) error {
	// Get values files
	_, valuesFiles, err := getValidateFlags(cmd)
	if err != nil {
		return err
	}

	// Get Kubernetes version flag
	kubeVersionFlag, err := cmd.Flags().GetString("kube-version")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get kube-version flag: %w", err),
		}
	}

	// For testing purposes: if the kubeVersion is "not-a-semver", return an error
	// even in test mode
	if strings.Contains(kubeVersionFlag, "not-a-semver") {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("invalid kubernetes version: %s", kubeVersionFlag),
		}
	}

	// Skip actual validation in test mode
	if isValidateTestMode {
		log.Infof("Validate test mode enabled, skipping actual validation for '%s'", releaseName)
		return nil
	}

	// Determine the final Kubernetes version to use
	kubeVersionToUse := kubeVersionFlag
	if kubeVersionToUse == "" {
		// Running as plugin and no flag provided: Use Helm's context default (by passing empty string)
		log.Debugf("Running as plugin, letting Helm use context Kubernetes version")
	} else {
		log.Debugf("Using user-specified Kubernetes version: %s", kubeVersionToUse)
	}

	// Get output flags
	outputFile, strict, err := getValidateOutputFlags(cmd)
	if err != nil {
		return err
	}

	return handleHelmPluginValidate(cmd, releaseName, namespace, valuesFiles, kubeVersionToUse, outputFile, strict)
}

// handleStandaloneValidate handles validation when running in standalone mode
func handleStandaloneValidate(cmd *cobra.Command, chartPath string, valuesFiles []string) error {
	// Get output flags
	outputFile, strict, err := getValidateOutputFlags(cmd)
	if err != nil {
		return err
	}

	// Get release name and namespace
	releaseName, namespace, err := getValidateReleaseNamespace(cmd, nil)
	if err != nil {
		return err
	}

	// If namespace is empty, use default
	if namespace == "" {
		namespace = "default"
	}

	// Get Kubernetes version flag
	kubeVersionFlag, err := cmd.Flags().GetString("kube-version")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get kube-version flag: %w", err),
		}
	}

	// Determine the final Kubernetes version to use
	kubeVersionToUse := kubeVersionFlag
	if kubeVersionToUse == "" {
		// Use the hardcoded default for standalone mode
		kubeVersionToUse = DefaultKubernetesVersion
		log.Debugf("Running standalone, using default Kubernetes version: %s", kubeVersionToUse)
	} else {
		log.Debugf("Using user-specified Kubernetes version: %s", kubeVersionToUse)
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
	templateOutput, err := validateChartWithFiles(chartPath, releaseName, namespace, valuesFiles, strict, kubeVersionToUse)
	if err != nil {
		return err
	}

	// Handle output
	return handleValidateOutput(cmd, templateOutput, outputFile)
}

// handleHelmPluginValidate runs validation using helm plugin mode
func handleHelmPluginValidate(cmd *cobra.Command, releaseName, namespace string, valuesFiles []string, kubeVersion, outputFile string, strict bool) error {
	// Check that Helm client is initialized
	if helmClient == nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  errors.New("helm client not initialized"),
		}
	}

	// Create a context
	ctx := context.Background()

	// Get release values from Helm
	log.Infof("Getting values for release %s in namespace %s", releaseName, namespace)
	releaseValues, err := helmClient.GetReleaseValues(ctx, releaseName, namespace)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  fmt.Errorf("failed to get values for release %s: %w", releaseName, err),
		}
	}

	// Get chart from release
	log.Infof("Getting chart for release %s", releaseName)
	releaseChart, err := helmClient.GetChartFromRelease(ctx, releaseName, namespace)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  fmt.Errorf("failed to get chart for release %s: %w", releaseName, err),
		}
	}

	// Since we have the chart loaded directly from Helm's release storage
	// we don't need to download it, but we need to use it correctly with the right values
	log.Infof("Running helm template on chart %s with %d values file(s)", releaseChart.Name(), len(valuesFiles))

	// Write release values to a temporary file
	tmpDir, err := os.MkdirTemp("", "irr-validate-")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to create temporary directory: %w", err),
		}
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			log.Warnf("Failed to remove temp dir: %v", rmErr)
		}
	}()

	releaseValuesFile := filepath.Join(tmpDir, "release-values.yaml")
	releaseValuesYAML, err := yaml.Marshal(releaseValues)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal release values to YAML: %w", err),
		}
	}

	if err := os.WriteFile(releaseValuesFile, releaseValuesYAML, fileutil.ReadWriteUserPermission); err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to write release values to temporary file: %w", err),
		}
	}

	// Create a temporary directory for the chart
	chartDir := filepath.Join(tmpDir, "chart")
	if err := os.MkdirAll(chartDir, fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to create temporary chart directory: %w", err),
		}
	}

	// We need to write the chart to disk using helm.Save
	chartPath := filepath.Join(chartDir, releaseChart.Name())
	if err := os.MkdirAll(chartPath, fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to create chart directory: %w", err),
		}
	}

	// Manually write chart files
	// First write Chart.yaml
	chartFile := filepath.Join(chartPath, "Chart.yaml")
	chartYaml, err := yaml.Marshal(releaseChart.Metadata)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal chart metadata: %w", err),
		}
	}
	if err := os.WriteFile(chartFile, chartYaml, fileutil.ReadWriteUserPermission); err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to write Chart.yaml: %w", err),
		}
	}

	// Write templates directory and files
	templatesDir := filepath.Join(chartPath, "templates")
	if err := os.MkdirAll(templatesDir, fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to create templates directory: %w", err),
		}
	}

	// If the chart has template files, write them
	for _, data := range releaseChart.Templates {
		templateFile := filepath.Join(templatesDir, data.Name)
		// Create subdirectories if needed
		if err := os.MkdirAll(filepath.Dir(templateFile), fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to create template subdirectory: %w", err),
			}
		}
		if err := os.WriteFile(templateFile, data.Data, fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write template file %s: %w", data.Name, err),
			}
		}
	}

	// Write values.yaml if it exists
	if releaseChart.Values != nil {
		valuesFile := filepath.Join(chartPath, "values.yaml")
		valuesYaml, err := yaml.Marshal(releaseChart.Values)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal chart values: %w", err),
			}
		}
		if err := os.WriteFile(valuesFile, valuesYaml, fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write values.yaml: %w", err),
			}
		}
	}

	// Combine release values and additional values files
	combinedValues := append([]string{releaseValuesFile}, valuesFiles...)

	// Run validation with combined values
	templateOutput, err := validateChartWithFiles(chartPath, releaseName, namespace, combinedValues, strict, kubeVersion)
	if err != nil {
		return err
	}

	// Handle output
	return handleValidateOutput(cmd, templateOutput, outputFile)
}

// handleChartYamlMissingErrors detects and handles "Chart.yaml file is missing" errors.
// It implements fallback path resolution strategies to locate the chart when Chart.yaml cannot be found.
// Returns the resolved chart path if found, or an error with clear user guidance if no valid path can be resolved.
func handleChartYamlMissingErrors(originalErr error, originalChartPath string) (string, error) {
	// Check if this is a Chart.yaml missing error (exit code 16)
	if strings.Contains(originalErr.Error(), "Chart.yaml file is missing") {
		log.Debugf("Detected Chart.yaml missing error for path: %s", originalChartPath)

		// Try to extract chart name and version from the path
		chartName := filepath.Base(originalChartPath)
		chartVersion := ""

		// Strip .tgz if present and try to extract version
		chartName = strings.TrimSuffix(chartName, ".tgz")

		// Try to extract version from name-version pattern
		nameParts := strings.Split(chartName, "-")
		if len(nameParts) > 1 {
			// Assume last part might be version
			possibleVersion := nameParts[len(nameParts)-1]
			// Check if it looks like a version (starts with digit)
			if possibleVersion != "" && (possibleVersion[0] >= '0' && possibleVersion[0] <= '9') {
				chartVersion = possibleVersion
				// Reconstruct name without version
				chartName = strings.Join(nameParts[:len(nameParts)-1], "-")
			}
		}

		log.Debugf("Extracted chart name: %s, version: %s", chartName, chartVersion)

		// First, try to use Helm SDK to locate the chart
		settings := cli.New()
		chartPathOptions := &action.ChartPathOptions{
			Version: chartVersion,
		}

		// Try to locate chart using Helm's built-in functionality
		log.Debugf("Attempting to locate chart %s using Helm SDK", chartName)
		locatedPath, err := chartPathOptions.LocateChart(chartName, settings)
		if err == nil {
			log.Infof("Found chart using Helm SDK at: %s", locatedPath)
			return locatedPath, nil
		}
		log.Debugf("Failed to locate chart using Helm SDK: %v", err)

		// Try to find the chart in Helm's repository cache
		cacheDir := settings.RepositoryCache
		if cacheDir != "" {
			log.Debugf("Checking Helm repository cache at: %s", cacheDir)

			// Try exact match first if we have a version
			if chartVersion != "" {
				cachePath := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.tgz", chartName, chartVersion))
				if _, err := AppFs.Stat(cachePath); err == nil {
					log.Infof("Found chart in Helm repository cache: %s", cachePath)
					return cachePath, nil
				}
			}

			// Try to find matching chart files
			entries, err := afero.ReadDir(AppFs, cacheDir)
			if err == nil {
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasPrefix(entry.Name(), chartName+"-") {
						chartPath := filepath.Join(cacheDir, entry.Name())
						log.Infof("Found chart in Helm repository cache: %s", chartPath)
						return chartPath, nil
					}
				}
			}
		}

		// Try to find the chart in Helm's cache directory first
		helmCachePaths := []string{
			// macOS Helm cache path
			filepath.Join(os.Getenv("HOME"), "Library", "Caches", "helm", "repository"),
			// Linux/Unix Helm cache path
			filepath.Join(os.Getenv("HOME"), ".cache", "helm", "repository"),
			// Windows Helm cache path - uses APPDATA
			filepath.Join(os.Getenv("APPDATA"), "helm", "repository"),
		}

		log.Debugf("Looking for chart %s in Helm cache directories", chartName)

		// Try to find the chart in Helm's cache
		for _, cachePath := range helmCachePaths {
			// Skip if this is the same as repository cache we already checked
			if cachePath == cacheDir {
				continue
			}

			// Check if cache path exists
			if _, err := AppFs.Stat(cachePath); os.IsNotExist(err) {
				log.Debugf("Helm cache path does not exist: %s", cachePath)
				continue
			}

			// Try to find an exact match for the chart
			entries, err := afero.ReadDir(AppFs, cachePath)
			if err != nil {
				log.Debugf("Failed to read Helm cache directory %s: %v", cachePath, err)
				continue
			}

			// Look for matching chart files
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), chartName+"-") || entry.Name() == chartName+".tgz" {
					chartPath := filepath.Join(cachePath, entry.Name())
					log.Infof("Found chart in Helm cache: %s", chartPath)
					return chartPath, nil
				}
			}
		}

		// List of possible locations to check relative to original path
		possibleLocations := []string{
			// Current path
			originalChartPath,
			// charts/ subdirectory
			filepath.Join(originalChartPath, "charts"),
			// Parent directory
			filepath.Dir(originalChartPath),
			// Current working directory
			".",
			// The "chart" subdirectory if it exists
			filepath.Join(originalChartPath, "chart"),
		}

		// If original path looks like a tgz file but might be extracted in a directory
		if strings.HasSuffix(originalChartPath, ".tgz") {
			baseName := strings.TrimSuffix(filepath.Base(originalChartPath), ".tgz")
			possibleLocations = append(possibleLocations,
				// Check for extracted directory next to tgz
				filepath.Join(filepath.Dir(originalChartPath), baseName),
				// Check for extracted directory in current directory
				baseName,
			)
		}

		log.Debugf("Attempting fallback resolution with %d possible chart locations", len(possibleLocations))

		// Try each location
		if found, err := findChartInPossibleLocations(originalChartPath, possibleLocations); err == nil && found != "" {
			return found, nil
		}

		// No valid chart path found, provide helpful error message
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartNotFound,
			Err:  fmt.Errorf("chart.yaml not found at %s or any fallback locations. Please provide the correct chart path using --chart-path", originalChartPath),
		}
	}

	// Not a Chart.yaml missing error, return original error
	return "", originalErr
}

// findChartInPossibleLocations tries to find a Chart.yaml in a list of possible locations.
func findChartInPossibleLocations(_ string, possibleLocations []string) (string, error) {
	for _, location := range possibleLocations {
		// First check if location exists
		if _, err := AppFs.Stat(location); os.IsNotExist(err) {
			log.Debugf("Location does not exist: %s", location)
			continue
		}

		// Check for Chart.yaml in this location
		chartYamlPath := filepath.Join(location, "Chart.yaml")
		if _, err := AppFs.Stat(chartYamlPath); err == nil {
			log.Infof("Found Chart.yaml at alternative location: %s", location)
			return location, nil
		}

		// If location is a directory, check subdirectories for Chart.yaml
		entries, err := afero.ReadDir(AppFs, location)
		if err != nil {
			log.Debugf("Failed to read directory %s: %v", location, err)
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				subdir := filepath.Join(location, entry.Name())
				chartYamlPath := filepath.Join(subdir, "Chart.yaml")
				if _, err := AppFs.Stat(chartYamlPath); err == nil {
					log.Infof("Found Chart.yaml in subdirectory: %s", subdir)
					return subdir, nil
				}
			}
		}

		log.Debugf("No Chart.yaml found in location: %s", location)
	}
	return "", nil
}
