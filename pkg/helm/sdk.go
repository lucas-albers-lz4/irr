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

func (l *defaultChartLoader) Load(path string) (*chart.Chart, error) {
	loadedChart, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}
	return loadedChart, nil
}

// FileSystem abstraction
var fs = afero.NewOsFs()

// SetFileSystem allows overriding the default filesystem for testing
func SetFileSystem(newFs afero.Fs) {
	fs = newFs
}

// ResolveChartPath attempts to resolve a chart path.
// NOTE: This currently only validates if the provided chartPath exists.
// It does not perform actual Helm SDK resolution based on release name or repo.
// Configuration parameter is currently unused.
func ResolveChartPath(_ *action.Configuration, _, chartPath string) (string, error) {
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
	chartLoader := &defaultChartLoader{}
	loadedChart, err := chartLoader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}
	return loadedChart, nil
}
