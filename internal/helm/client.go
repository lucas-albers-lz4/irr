package helm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lalbers/irr/pkg/log"
	"github.com/spf13/afero"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// ChartMetadata contains essential metadata about a chart from a release
type ChartMetadata struct {
	Name       string
	Version    string
	Repository string
	Path       string
}

// ClientInterface abstracts Helm SDK interactions
// This allows for mocking in tests and clean separation of concerns
type ClientInterface interface {
	// Release-related operations
	GetReleaseValues(ctx context.Context, releaseName string, namespace string) (map[string]interface{}, error)
	GetReleaseChart(ctx context.Context, releaseName string, namespace string) (*ChartMetadata, error)
	FindChartForRelease(ctx context.Context, releaseName, namespace string) (string, error)
	ValidateRelease(ctx context.Context, releaseName, namespace string, overrideFiles []string, kubeVersion string) error

	// Chart operations
	TemplateChart(ctx context.Context, releaseName string, chartPath string, values map[string]interface{}, namespace string, kubeVersion string) (string, error)

	// Environment information
	GetCurrentNamespace() string
}

// RealHelmClient implements ClientInterface using the actual Helm SDK
type RealHelmClient struct {
	settings     *cli.EnvSettings
	actionConfig *action.Configuration
}

// NewHelmClient creates a new instance of the RealHelmClient
func NewHelmClient() (*RealHelmClient, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)

	// Initialize with default namespace, will be overridden in operations
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER"), func(string, ...interface{}) {}); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm configuration: %w", err)
	}

	return &RealHelmClient{
		settings:     settings,
		actionConfig: actionConfig,
	}, nil
}

// GetReleaseValues fetches values from an installed Helm release
func (c *RealHelmClient) GetReleaseValues(_ context.Context, releaseName, namespace string) (map[string]interface{}, error) {
	log.Debug("Getting release values", "release", releaseName, "namespace", namespace)

	// Configure namespace
	if namespace == "" {
		namespace = c.settings.Namespace()
	}

	// Create a new get values action
	client := action.NewGetValues(c.actionConfig)
	client.AllValues = true // Get both user-supplied and computed values

	// Execute the get values action
	values, err := client.Run(releaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get values for release %q in namespace %q: %w", releaseName, namespace, err)
	}

	return values, nil
}

// GetReleaseChart fetches chart metadata from an installed Helm release
func (c *RealHelmClient) GetReleaseChart(_ context.Context, releaseName, namespace string) (*ChartMetadata, error) {
	log.Debug("Getting release chart info", "release", releaseName, "namespace", namespace)

	// Configure namespace
	if namespace == "" {
		namespace = c.settings.Namespace()
	}

	// Create a new get action
	client := action.NewGet(c.actionConfig)

	// Execute the get action
	release, err := client.Run(releaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release %q in namespace %q: %w", releaseName, namespace, err)
	}

	// Extract chart metadata
	meta := &ChartMetadata{
		Name:    release.Chart.Metadata.Name,
		Version: release.Chart.Metadata.Version,
	}

	// Extract repository if available
	for _, url := range release.Chart.Metadata.Sources {
		meta.Repository = url
		break // Just use the first source URL as repository
	}

	return meta, nil
}

// TemplateChart renders a chart with the given values
func (c *RealHelmClient) TemplateChart(_ context.Context, releaseName, chartPath string, values map[string]interface{}, namespace, kubeVersion string) (string, error) {
	log.Debug("Templating chart", "chartPath", chartPath, "release", releaseName, "namespace", namespace)

	// --- Capture Helm SDK logs ---
	// Create a buffer to capture Helm's log output
	var helmLogBuffer bytes.Buffer
	// Create a logger function that writes to the buffer
	helmLogger := func(format string, v ...interface{}) {
		// Write the formatted log message to the buffer
		// Ensure a newline is added if not present in the format string
		if !strings.HasSuffix(format, "\\n") {
			format += "\\n"
		}
		fmt.Fprintf(&helmLogBuffer, format, v...)
	}
	// Create a *new* action config specifically for this operation to avoid race conditions if the client is reused
	actionConfig := new(action.Configuration)
	// Initialize the new config, setting our custom logger
	// We need to re-initialize settings that might be needed by Init or Install
	if err := actionConfig.Init(c.settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), helmLogger); err != nil {
		return "", fmt.Errorf("failed to initialize Helm action configuration for logging: %w", err)
	}
	// --- End Helm log capture setup ---

	// Use the new actionConfig with the custom logger
	client := action.NewInstall(actionConfig) // Use the config with our logger
	client.ClientOnly = true
	client.DryRun = true
	client.ReleaseName = releaseName
	client.Replace = true
	client.IncludeCRDs = true
	client.Namespace = namespace // Ensure namespace is set on the client too

	if kubeVersion != "" {
		// Need to parse kubeVersion, action.Install uses KubeVersion struct
		parsedKubeVersion, err := chartutil.ParseKubeVersion(kubeVersion)
		if err != nil {
			log.Warn("Could not parse kube-version, using default", "version", kubeVersion, "error", err)
		} else {
			client.KubeVersion = parsedKubeVersion
		}
	}

	// Create chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		// Process any logs captured so far, even on chart load failure
		processHelmLogs(&helmLogBuffer)
		return "", fmt.Errorf("failed to load chart %s: %w", chartPath, err)
	}

	// We need to merge chart values with provided values
	filteredVals, err := chartutil.CoalesceValues(chart, values)
	if err != nil {
		// Process any logs captured so far
		processHelmLogs(&helmLogBuffer)
		return "", fmt.Errorf("failed to merge chart values: %w", err)
	}

	if validationErr := chart.Validate(); validationErr != nil {
		// Process any logs captured so far
		processHelmLogs(&helmLogBuffer)
		return "", fmt.Errorf("chart validation error: %w", validationErr)
	}

	// Template the release
	release, err := client.Run(chart, filteredVals)

	// --- Process captured Helm logs ---
	processHelmLogs(&helmLogBuffer)
	// --- End log processing ---

	if err != nil {
		// Error already includes context, return as is after processing logs
		// Wrapcheck requires wrapping external errors, even if descriptive.
		return "", fmt.Errorf("helm SDK templating failed for chart %s: %w", chartPath, err)
	}

	return release.Manifest, nil
}

// processHelmLogs takes the buffer containing Helm SDK logs, splits them into lines,
// and logs each non-empty line using the application's logger.
func processHelmLogs(buffer *bytes.Buffer) {
	logOutput := buffer.String()
	if logOutput == "" {
		return // Nothing to process
	}

	lines := strings.Split(strings.TrimSpace(logOutput), "\\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" {
			// Log Helm's output as INFO level for visibility.
			// Could potentially parse for "WARN" or "ERROR" prefixes if needed.
			log.Info("[Helm SDK] " + trimmedLine)
		}
	}
}

// GetCurrentNamespace returns the current namespace from Helm environment
func (c *RealHelmClient) GetCurrentNamespace() string {
	return c.settings.Namespace()
}

// IsReleaseNotFoundError checks if the error is a "release not found" error
func IsReleaseNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Use errors.Is to properly handle wrapped errors
	return errors.Is(err, driver.ErrReleaseNotFound)
}

// findChartInHelmCachePaths tries to find a chart in a list of Helm cache directories.
func findChartInHelmCachePaths(meta *ChartMetadata, cacheDir string) (string, error) {
	helmCachePaths := []string{
		filepath.Join(os.Getenv("HOME"), "Library", "Caches", "helm", "repository"),
		filepath.Join(os.Getenv("HOME"), ".cache", "helm", "repository"),
		filepath.Join(os.Getenv("APPDATA"), "helm", "repository"),
	}
	fs := afero.NewOsFs()
	for _, cachePath := range helmCachePaths {
		if cachePath == cacheDir {
			continue
		}
		potentialChartPath := filepath.Join(cachePath, meta.Name+"-"+meta.Version+".tgz")
		log.Debug("Checking generic cache path", "path", potentialChartPath)
		exists, err := afero.Exists(fs, potentialChartPath)
		if err == nil && exists {
			log.Debug("Found chart in generic cache (exact version)", "path", potentialChartPath)
			return potentialChartPath, nil
		}
		matches, err := afero.Glob(fs, filepath.Join(cachePath, meta.Name+"-*.tgz"))
		if err == nil && len(matches) > 0 {
			sort.Strings(matches)
			chartPath := matches[len(matches)-1]
			log.Debug("Found chart in generic cache (glob match)", "path", chartPath)
			return chartPath, nil
		}
	}
	return "", nil
}

// FindChartForRelease locates the chart path for a given Helm release.
func (c *RealHelmClient) FindChartForRelease(_ context.Context, releaseName, namespace string) (string, error) {
	// First, get the release info to find the chart metadata
	// We need a config for the 'get' action
	cfg, err := c.getActionConfig(namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get Helm action config for namespace %s: %w", namespace, err)
	}

	// Use Helm's 'get' action to retrieve release information
	getAction := action.NewGet(cfg)
	release, err := getAction.Run(releaseName)
	if err != nil {
		// Check if the error is specifically 'release not found'
		if errors.Is(err, driver.ErrReleaseNotFound) || strings.Contains(err.Error(), "release: not found") {
			return "", fmt.Errorf("release %q not found in namespace %q: %w", releaseName, namespace, err)
		}
		return "", fmt.Errorf("failed to get release %q: %w", releaseName, err)
	}

	if release == nil || release.Chart == nil || release.Chart.Metadata == nil {
		return "", fmt.Errorf("could not retrieve valid chart metadata for release %q", releaseName)
	}

	// Now, attempt to find the chart in the local cache using action.ChartPathOptions
	chartPathOptions := action.ChartPathOptions{
		Version: release.Chart.Metadata.Version,
		// Optionally add RepoURL if available and needed for location logic
		// RepoURL: release.Chart.Metadata.RepoURL, // Assuming RepoURL field exists
	}

	chartPath, err := chartPathOptions.LocateChart(release.Chart.Metadata.Name, c.settings)
	if err != nil {
		// Log a warning if not found in cache
		log.Warn("Could not locate chart in local Helm cache", "chart", release.Chart.Metadata.Name, "version", release.Chart.Metadata.Version, "error", err)
		// Return an error because loader.Load needs a valid path.
		return "", fmt.Errorf("failed to locate chart %q version %q in Helm cache: %w", release.Chart.Metadata.Name, release.Chart.Metadata.Version, err)
	}

	log.Debug("Located chart path for release", "release", releaseName, "chartPath", chartPath)
	return chartPath, nil
}

// ValidateRelease validates a Helm release against provided override files
func (c *RealHelmClient) ValidateRelease(_ context.Context, _, _ string, _ []string, _ string) error {
	// Placeholder implementation until fully defined
	log.Warn("ValidateRelease is not fully implemented yet.")
	return nil // Added missing return
}

// getActionConfig returns a new action configuration for the given namespace
func (c *RealHelmClient) getActionConfig(namespace string) (*action.Configuration, error) {
	cfg := new(action.Configuration)

	// Initialize the configuration using the correct logger function signature
	// Define the logger function inline or use a package-level function if preferred
	helmLogger := func(format string, v ...interface{}) {
		// Use the application's logger (e.g., log.Debug)
		// Add prefix to distinguish Helm SDK logs
		log.Debug(fmt.Sprintf("[Helm SDK] "+format, v...))
	}

	if err := cfg.Init(c.settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), helmLogger); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action config: %w", err)
	}
	return cfg, nil
}
