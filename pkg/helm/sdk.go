package helm

import (
	"fmt"
	"time"

	"github.com/spf13/afero"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// ChartLoader interface for loading Helm charts
type ChartLoader interface {
	Load(path string) (*chart.Chart, error)
}

// TimeProvider interface for time-based operations
type TimeProvider interface {
	Now() time.Time
}

// defaultChartLoader implements ChartLoader using Helm's loader
type defaultChartLoader struct{}

func (d *defaultChartLoader) Load(path string) (*chart.Chart, error) {
	return loader.Load(path)
}

// defaultTimeProvider implements TimeProvider
type defaultTimeProvider struct{}

func (d *defaultTimeProvider) Now() time.Time {
	return time.Now()
}

// FileSystem abstraction
var fs = afero.NewOsFs()

// SetFileSystem allows overriding the default filesystem for testing
func SetFileSystem(newFs afero.Fs) {
	fs = newFs
}

// ResolveChartPath resolves a chart path from a release name using the Helm SDK
func ResolveChartPath(actionConfig *action.Configuration, releaseName, chartPath string) (string, error) {
	// Check if chart path exists
	exists, err := afero.Exists(fs, chartPath)
	if err != nil {
		return "", fmt.Errorf("failed to check chart path: %w", err)
	}
	if !exists {
		return "", fmt.Errorf("chart path %s does not exist", chartPath)
	}
	return chartPath, nil
}

// Plugin represents a Helm plugin
type Plugin struct {
	Name string
	Path string
}

// DiscoverPlugins discovers plugins in the given directory
func DiscoverPlugins(pluginDir string) ([]*Plugin, error) {
	var plugins []*Plugin

	// List all files in the plugin directory
	files, err := afero.ReadDir(fs, pluginDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	// Check each file
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Check if file is executable
		if file.Mode()&0o111 != 0 {
			plugins = append(plugins, &Plugin{
				Name: file.Name(),
				Path: pluginDir + "/" + file.Name(),
			})
		}
	}

	return plugins, nil
}

// LoadChart loads a chart from the given path
func LoadChart(chartPath string) (*chart.Chart, error) {
	loader := &defaultChartLoader{}
	return loader.Load(chartPath)
}

// findImagesInChart finds all image references in a chart's dependencies
func findImagesInChart(chart *chart.Chart) []string {
	var images []string

	// Check values.yaml for image references
	if chart.Values != nil {
		if img, ok := chart.Values["image"].(map[string]interface{}); ok {
			if repo, ok := img["repository"].(string); ok {
				if tag, ok := img["tag"].(string); ok {
					images = append(images, fmt.Sprintf("%s:%s", repo, tag))
				}
			}
		}
	}

	// Check dependencies
	for _, dep := range chart.Dependencies() {
		depImages := findImagesInChart(dep)
		images = append(images, depImages...)
	}

	return images
}
