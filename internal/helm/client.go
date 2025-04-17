package helm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lalbers/irr/pkg/debug"
	log "github.com/lalbers/irr/pkg/log"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
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
	TemplateChart(ctx context.Context, releaseName string, chartPath string, values map[string]interface{}, namespace string) (string, error)

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
func (c *RealHelmClient) TemplateChart(_ context.Context, releaseName, chartPath string, values map[string]interface{}, namespace string) (string, error) {
	debug.Printf("Templating chart %q with release name %q in namespace %q", chartPath, releaseName, namespace)

	// Configure namespace
	if namespace == "" {
		namespace = c.settings.Namespace()
	}

	// Create a new install action configured for templating
	client := action.NewInstall(c.actionConfig)
	client.DryRun = true
	client.ReleaseName = releaseName
	client.Replace = true
	client.ClientOnly = true
	client.IncludeCRDs = false
	client.Namespace = namespace

	// Check if chartPath exists
	stat, err := os.Stat(chartPath)
	if err != nil {
		return "", fmt.Errorf("chart path error: %w", err)
	}

	var chrt *chart.Chart
	if stat.IsDir() {
		// Load from directory
		chrt, err = loader.LoadDir(chartPath)
	} else {
		// Load from file (assuming .tgz)
		absPath, err := filepath.Abs(chartPath)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path: %w", err)
		}
		var loadErr error
		chrt, loadErr = loader.Load(absPath)
		if loadErr != nil {
			return "", fmt.Errorf("failed to load chart: %w", loadErr)
		}
	}

	if err != nil {
		return "", fmt.Errorf("failed to load chart: %w", err)
	}

	// For helm template command we need values in a specific format
	// If no values provided, use empty map
	vals := values
	if vals == nil {
		vals = make(map[string]interface{})
	}

	// Merge with chart's default values
	mergedValues, err := chartutil.CoalesceValues(chrt, chartutil.Values(vals))
	if err != nil {
		return "", fmt.Errorf("failed to merge values: %w", err)
	}

	// Render the templates
	rel, err := client.Run(chrt, mergedValues)
	if err != nil {
		return "", fmt.Errorf("template rendering failed: %w", err)
	}

	return rel.Manifest, nil
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
