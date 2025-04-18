package helm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/lalbers/irr/pkg/debug"
	log "github.com/lalbers/irr/pkg/log"
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
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER"), log.Debugf); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm configuration: %w", err)
	}

	return &RealHelmClient{
		settings:     settings,
		actionConfig: actionConfig,
	}, nil
}

// GetReleaseValues fetches values from an installed Helm release
func (c *RealHelmClient) GetReleaseValues(_ context.Context, releaseName, namespace string) (map[string]interface{}, error) {
	debug.Printf("Getting values for release %q in namespace %q", releaseName, namespace)

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
	debug.Printf("Getting chart info for release %q in namespace %q", releaseName, namespace)

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
	debug.Printf("Templating chart %q with release name %q in namespace %q", chartPath, releaseName, namespace)

	client := action.NewInstall(c.actionConfig)
	client.ClientOnly = true
	client.DryRun = true
	client.ReleaseName = releaseName
	client.Replace = true
	client.IncludeCRDs = true
	client.Namespace = namespace

	if kubeVersion != "" {
		client.KubeVersion = &chartutil.KubeVersion{Version: kubeVersion}
	}

	// Create chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return "", fmt.Errorf("failed to load chart: %w", err)
	}

	// We need to merge chart values with provided values
	filteredVals, err := chartutil.CoalesceValues(chart, values)
	if err != nil {
		return "", fmt.Errorf("failed to merge chart values: %w", err)
	}

	if validationErr := chart.Validate(); validationErr != nil {
		return "", fmt.Errorf("chart validation error: %w", validationErr)
	}

	// Template the release
	release, err := client.Run(chart, filteredVals)
	if err != nil {
		return "", fmt.Errorf("chart templating error: %w", err)
	}

	return release.Manifest, nil
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
		debug.Printf("Checking for chart in cache path: %s", potentialChartPath)
		exists, err := afero.Exists(fs, potentialChartPath)
		if err == nil && exists {
			debug.Printf("Found chart in cache: %s", potentialChartPath)
			return potentialChartPath, nil
		}
		matches, err := afero.Glob(fs, filepath.Join(cachePath, meta.Name+"-*.tgz"))
		if err == nil && len(matches) > 0 {
			sort.Strings(matches)
			chartPath := matches[len(matches)-1]
			debug.Printf("Found chart in Helm cache (glob match): %s", chartPath)
			return chartPath, nil
		}
	}
	return "", nil
}

// FindChartForRelease attempts to find a chart in Helm's cache based on release information
// It provides a robust fallback system to handle Chart.yaml missing errors
func (c *RealHelmClient) FindChartForRelease(ctx context.Context, releaseName, namespace string) (string, error) {
	debug.Printf("Searching for chart for release %q in namespace %q", releaseName, namespace)

	// First, get chart metadata from the release
	meta, err := c.GetReleaseChart(ctx, releaseName, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get chart metadata for release %q: %w", releaseName, err)
	}

	// Try using action.ChartPathOptions.LocateChart first (most reliable)
	debug.Printf("Attempting to locate chart %s:%s via Helm SDK", meta.Name, meta.Version)
	chartPathOptions := action.ChartPathOptions{
		Version: meta.Version,
		RepoURL: meta.Repository,
	}

	// If we have repository info, use it for the chart reference
	chartRef := fmt.Sprintf("%s/%s", meta.Repository, meta.Name)

	// Try locating the chart using Helm's official method
	chartPath, err := chartPathOptions.LocateChart(chartRef, c.settings)
	if err == nil {
		debug.Printf("Successfully located chart at %s", chartPath)
		return chartPath, nil
	}

	// If it failed, log and continue with fallbacks
	debug.Printf("Failed to locate chart via Helm SDK: %v", err)

	// Fallback 1: Try Helm's repository cache directly
	cacheDir := c.settings.RepositoryCache
	if cacheDir != "" {
		debug.Printf("Checking Helm repository cache at %s", cacheDir)

		// Try exact match first
		chartFileName := fmt.Sprintf("%s-%s.tgz", meta.Name, meta.Version)
		cachePath := filepath.Join(cacheDir, chartFileName)

		if _, err := os.Stat(cachePath); err == nil {
			debug.Printf("Found chart in repository cache: %s", cachePath)
			return cachePath, nil
		}

		// If exact match fails, try a glob pattern to find any version
		pattern := filepath.Join(cacheDir, meta.Name+"-*.tgz")
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			// Sort to get the latest version if multiple exist
			sort.Strings(matches)
			chartPath := matches[len(matches)-1]
			debug.Printf("Found chart in repository cache using glob: %s", chartPath)
			return chartPath, nil
		}
	}

	if found, err := findChartInHelmCachePaths(meta, cacheDir); err == nil && found != "" {
		return found, nil
	}

	// Fallback 3: Try to download the chart if we have repo information
	if meta.Repository != "" {
		debug.Printf("Attempting to pull chart %s:%s from repository", meta.Name, meta.Version)

		// Create a temporary directory to store the downloaded chart
		tempDir, err := os.MkdirTemp("", "irr-chart-download-")
		if err != nil {
			debug.Printf("Failed to create temp directory: %v", err)
		}
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				debug.Printf("Failed to remove temp directory: %v", err)
			}
		}()

		// Create a pull action to download the chart
		pull := action.NewPullWithOpts(action.WithConfig(c.actionConfig))
		pull.Settings = c.settings
		pull.DestDir = tempDir
		pull.Version = meta.Version

		// Try to pull the chart
		downloadedPath, err := pull.Run(chartRef)
		if err == nil && downloadedPath != "" {
			debug.Printf("Successfully pulled chart to %s", downloadedPath)
			return downloadedPath, nil
		}

		debug.Printf("Failed to pull chart: %v", err)
	}

	// If all methods fail, return an informative error
	return "", fmt.Errorf("failed to locate chart for release %s in namespace %s. "+
		"Chart name: %s, version: %s. Please provide the chart path explicitly using --chart-path",
		releaseName, namespace, meta.Name, meta.Version)
}
