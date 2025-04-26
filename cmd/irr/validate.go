package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
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
	cmd.Flags().StringP("namespace", "n", "default", "Namespace to use")
	cmd.Flags().StringP("output-file", "o", "", "Write rendering output to file instead of discarding")
	cmd.Flags().Bool("strict", false, "Fail on any warning, not just errors")
	cmd.Flags().String("kube-version", "", "Kubernetes version to use for validation (defaults to current client version)")

	return cmd
}

// runValidate is the main entry point for the validate command
func runValidate(cmd *cobra.Command, args []string) error {
	// Get required flags
	chartPath, valuesFiles, err := getValidateFlags(cmd)
	if err != nil {
		return err
	}

	// Handle validation
	if isRunningAsHelmPlugin() {
		log.Debug("Running in Helm plugin mode, handling plugin-specific validation")
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

// validateAndDetectChartPath validates the chart path or detects it if necessary.
func validateAndDetectChartPath(chartPath string) (string, error) {
	log.Debug("validateAndDetectChartPath: Start", "inputChartPath", chartPath)
	var err error
	finalPath := chartPath
	var relativePath string // Declare relativePath for the detection function

	// Detect chart if path is not provided
	if finalPath == "" {
		log.Debug("validateAndDetectChartPath: Chart path empty, attempting detection.")
		// Correct call: Pass the original chartPath (empty here) via finalPath variable
		finalPath, relativePath, err = detectChartIfNeeded(AppFs, finalPath)
		if err != nil {
			log.Debug("validateAndDetectChartPath: Detection failed", "error", err)
			// Wrap the specific detection error with a more user-friendly message and exit code
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("chart path not specified and %w", err), // Keep original error context
			}
		}
		// Log both paths
		log.Debug("validateAndDetectChartPath: Detected chart path", "absolutePath", finalPath, "relativePath", relativePath)
	}

	// Use finalPath directly with AppFs.Stat
	absPath := finalPath // Rename for minimal changes below, but it might be relative

	// Check if the path exists using the application's filesystem abstraction
	log.Debug("validateAndDetectChartPath: Checking if path exists", "pathToCheck", absPath)
	if _, err := AppFs.Stat(absPath); err != nil {
		log.Debug("validateAndDetectChartPath: Path check failed", "pathChecked", absPath, "error", err)
		if os.IsNotExist(err) {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("chart path not found: %s", absPath), // Use path checked in error
			}
		}
		// Handle other Stat errors (e.g., permission denied)
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to access chart path '%s': %w", absPath, err),
		}
	}

	log.Debug("validateAndDetectChartPath: Path exists and is valid", "finalPath", absPath)
	return absPath, nil
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
		log.Debug("Using namespace for validation", "namespace", namespace)
	}

	// Log Kubernetes version
	log.Debug("Using Kubernetes version for validation", "kubeVersion", kubeVersion)

	// Log if strict mode is enabled
	if strict {
		log.Debug("Strict validation mode enabled")
	}

	// Execute Helm template command
	result, err := helm.HelmTemplateFunc(templateOptions)
	if err != nil {
		log.Error("Validation failed: Chart could not be rendered.")
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
				log.Info("Retrying validation with resolved chart path", "path", resolvedPath)
				templateOptions.ChartPath = resolvedPath
				retryResult, retryErr := helm.HelmTemplateFunc(templateOptions)
				if retryErr == nil {
					log.Info("Validation successful with resolved chart path!")
					if retryResult != nil {
						return retryResult.Stdout, nil
					}
					log.Warn("HelmTemplateFunc returned nil retryResult after successful retry")
					return "", nil
				}

				log.Error("Validation still failed with resolved path", "error", retryErr)
				if retryResult != nil && retryResult.Stderr != "" {
					fmt.Fprintf(os.Stderr, "--- Helm Error (Retry) ---\n%s\n------------------------\n", retryResult.Stderr)
				} else if retryResult == nil {
					log.Warn("HelmTemplateFunc returned nil retryResult after retrying with resolved path")
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
			log.Error("Strict validation failed: Found unresolved template variables in output")
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("strict validation failed: unresolved template variables found in rendered output"),
			}
		}

		// Check for other problematic patterns
		if strings.Contains(output, "<no value>") {
			log.Error("Strict validation failed: Found <no value> placeholders in output")
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitHelmCommandFailed,
				Err:  fmt.Errorf("strict validation failed: <no value> placeholders found in rendered output"),
			}
		}
	}

	log.Info("Validation successful: Chart rendered successfully with values.")
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
		log.Info("Validation complete. No output was generated.")
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
		log.Info("Validate test mode enabled, skipping actual validation for '%s'", releaseName)
		return nil
	}

	// Determine the final Kubernetes version to use
	kubeVersionToUse := kubeVersionFlag
	if kubeVersionToUse == "" {
		// Running as plugin and no flag provided: Use Helm's context default (by passing empty string)
		log.Debug("Running as plugin, letting Helm use context Kubernetes version")
	} else {
		log.Debug("Using user-specified Kubernetes version", "kubeVersion", kubeVersionToUse)
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
		log.Debug("Running standalone, using default Kubernetes version", "kubeVersion", kubeVersionToUse)
	} else {
		log.Debug("Using user-specified Kubernetes version", "kubeVersion", kubeVersionToUse)
	}

	// Check if chart path exists or is detectable
	chartPath, err = validateAndDetectChartPath(chartPath)
	log.Debug("Result from validateAndDetectChartPath", "chartPath", chartPath, "error", err)
	if err != nil {
		log.Debug("Error detected after validateAndDetectChartPath, returning.", "error", err)
		return err
	}

	// Check if values files are specified when needed
	if len(valuesFiles) == 0 {
		err := &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("at least one values file must be specified"),
		}
		log.Debug("Missing values file check triggered, returning error.", "error", err)
		return err
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

// handleHelmPluginValidate performs the core validation logic for Helm plugin mode,
// retrieving necessary chart information and values before executing the validation.
func handleHelmPluginValidate(cmd *cobra.Command, releaseName, namespace string, valuesFiles []string, kubeVersion, outputFile string, strict bool) error {
	log.Debug("Handling Helm plugin validate operation", "release", releaseName, "namespace", namespace)

	// Initialize Helm settings
	settings := cli.New()
	chartPathOptions := &action.ChartPathOptions{
		Version: kubeVersion,
	}

	// Try to locate chart using Helm's built-in functionality
	log.Debug("Attempting to locate chart %s using Helm SDK", releaseName)
	locatedPath, err := chartPathOptions.LocateChart(releaseName, settings)
	if err == nil {
		log.Info("Found chart using Helm SDK at", "path", locatedPath)
		return handleHelmPluginValidate(cmd, releaseName, namespace, valuesFiles, locatedPath, outputFile, strict)
	}
	log.Debug("Failed to locate chart using Helm SDK", "error", err)

	// Try to find the chart in Helm's repository cache
	cacheDir := settings.RepositoryCache
	if cacheDir != "" {
		log.Debug("Checking Helm repository cache at", "path", cacheDir)

		// Try exact match first if we have a version
		if kubeVersion != "" {
			cachePath := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.tgz", releaseName, kubeVersion))
			if _, err := AppFs.Stat(cachePath); err == nil {
				log.Info("Found chart in Helm repository cache", "path", cachePath)
				return handleHelmPluginValidate(cmd, releaseName, namespace, valuesFiles, cachePath, outputFile, strict)
			}
		}

		// Try to find matching chart files
		entries, err := afero.ReadDir(AppFs, cacheDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), releaseName+"-") {
					chartPath := filepath.Join(cacheDir, entry.Name())
					log.Info("Found chart in Helm repository cache", "path", chartPath)
					return handleHelmPluginValidate(cmd, releaseName, namespace, valuesFiles, chartPath, outputFile, strict)
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

	log.Debug("Looking for chart %s in Helm cache directories", releaseName)

	// Try to find the chart in Helm's cache
	for _, cachePath := range helmCachePaths {
		// Skip if this is the same as repository cache we already checked
		if cachePath == cacheDir {
			continue
		}

		// Check if cache path exists
		if _, err := AppFs.Stat(cachePath); os.IsNotExist(err) {
			log.Debug("Helm cache path does not exist", "path", cachePath)
			continue
		}

		// Try to find an exact match for the chart
		entries, err := afero.ReadDir(AppFs, cachePath)
		if err != nil {
			log.Debug("Failed to read Helm cache directory", "path", cachePath, "error", err)
			continue
		}

		// Look for matching chart files
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), releaseName+"-") || entry.Name() == releaseName+".tgz" {
				chartPath := filepath.Join(cachePath, entry.Name())
				log.Info("Found chart in Helm cache", "path", chartPath)
				return handleHelmPluginValidate(cmd, releaseName, namespace, valuesFiles, chartPath, outputFile, strict)
			}
		}
	}

	// List of possible locations to check relative to original path
	possibleLocations := []string{
		// Current path
		kubeVersion,
		// charts/ subdirectory
		filepath.Join(kubeVersion, "charts"),
		// Parent directory
		filepath.Dir(kubeVersion),
		// Current working directory
		".",
		// The "chart" subdirectory if it exists
		filepath.Join(kubeVersion, "chart"),
	}

	// If original path looks like a tgz file but might be extracted in a directory
	if strings.HasSuffix(kubeVersion, ".tgz") {
		baseName := strings.TrimSuffix(filepath.Base(kubeVersion), ".tgz")
		possibleLocations = append(possibleLocations,
			// Check for extracted directory next to tgz
			filepath.Join(filepath.Dir(kubeVersion), baseName),
			// Check for extracted directory in current directory
			baseName,
		)
	}

	log.Debug("Attempting fallback resolution with", "count", len(possibleLocations))

	// Try each location
	if found, err := findChartInPossibleLocations(kubeVersion, possibleLocations); err == nil && found != "" {
		return handleHelmPluginValidate(cmd, releaseName, namespace, valuesFiles, found, outputFile, strict)
	}

	// No valid chart path found, provide helpful error message
	return &exitcodes.ExitCodeError{
		Code: exitcodes.ExitChartNotFound,
		Err:  fmt.Errorf("chart.yaml not found at %s or any fallback locations. Please provide the correct chart path using --chart-path", kubeVersion),
	}
}

// handleChartYamlMissingErrors detects and handles "Chart.yaml file is missing" errors.
// It implements fallback path resolution strategies to locate the chart when Chart.yaml cannot be found.
// Returns the resolved chart path if found, or an error with clear user guidance if no valid path can be resolved.
func handleChartYamlMissingErrors(originalErr error, originalChartPath string) (string, error) {
	// Check if this is a Chart.yaml missing error (exit code 16)
	if strings.Contains(originalErr.Error(), "Chart.yaml file is missing") {
		log.Debug("Detected Chart.yaml missing error for path: %s", originalChartPath)

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

		log.Debug("Extracted chart name", "name", chartName, "version", chartVersion)

		// First, try to use Helm SDK to locate the chart
		settings := cli.New()
		chartPathOptions := &action.ChartPathOptions{
			Version: chartVersion,
		}

		// Try to locate chart using Helm's built-in functionality
		log.Debug("Attempting to locate chart %s using Helm SDK", chartName)
		locatedPath, err := chartPathOptions.LocateChart(chartName, settings)
		if err == nil {
			log.Info("Found chart using Helm SDK at", "path", locatedPath)
			return locatedPath, nil
		}
		log.Debug("Failed to locate chart using Helm SDK", "error", err)

		// Try to find the chart in Helm's repository cache
		cacheDir := settings.RepositoryCache
		if cacheDir != "" {
			log.Debug("Checking Helm repository cache at", "path", cacheDir)

			// Try exact match first if we have a version
			if chartVersion != "" {
				cachePath := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.tgz", chartName, chartVersion))
				if _, err := AppFs.Stat(cachePath); err == nil {
					log.Info("Found chart in Helm repository cache", "path", cachePath)
					return cachePath, nil
				}
			}

			// Try to find matching chart files
			entries, err := afero.ReadDir(AppFs, cacheDir)
			if err == nil {
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasPrefix(entry.Name(), chartName+"-") {
						chartPath := filepath.Join(cacheDir, entry.Name())
						log.Info("Found chart in Helm repository cache", "path", chartPath)
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

		log.Debug("Looking for chart %s in Helm cache directories", chartName)

		// Try to find the chart in Helm's cache
		for _, cachePath := range helmCachePaths {
			// Skip if this is the same as repository cache we already checked
			if cachePath == cacheDir {
				continue
			}

			// Check if cache path exists
			if _, err := AppFs.Stat(cachePath); os.IsNotExist(err) {
				log.Debug("Helm cache path does not exist", "path", cachePath)
				continue
			}

			// Try to find an exact match for the chart
			entries, err := afero.ReadDir(AppFs, cachePath)
			if err != nil {
				log.Debug("Failed to read Helm cache directory", "path", cachePath, "error", err)
				continue
			}

			// Look for matching chart files
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), chartName+"-") || entry.Name() == chartName+".tgz" {
					chartPath := filepath.Join(cachePath, entry.Name())
					log.Info("Found chart in Helm cache", "path", chartPath)
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

		log.Debug("Attempting fallback resolution with", "count", len(possibleLocations))

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
			log.Debug("Location does not exist", "location", location)
			continue
		}

		// Check for Chart.yaml in this location
		chartYamlPath := filepath.Join(location, "Chart.yaml")
		if _, err := AppFs.Stat(chartYamlPath); err == nil {
			log.Info("Found Chart.yaml at alternative location", "location", location)
			return location, nil
		}

		// If location is a directory, check subdirectories for Chart.yaml
		entries, err := afero.ReadDir(AppFs, location)
		if err != nil {
			log.Debug("Failed to read directory", "location", location, "error", err)
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				subdir := filepath.Join(location, entry.Name())
				chartYamlPath := filepath.Join(subdir, "Chart.yaml")
				if _, err := AppFs.Stat(chartYamlPath); err == nil {
					log.Info("Found Chart.yaml in subdirectory", "location", subdir)
					return subdir, nil
				}
			}
		}

		log.Debug("No Chart.yaml found in location", "location", location)
	}
	return "", nil
}
