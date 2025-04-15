// Package chart provides functionality for loading and processing Helm charts.
package chart

import (
	"fmt"
	"path/filepath"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/fileutil"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	// "helm.sh/helm/v3/pkg/chartutil" // Not needed after removing unused funcs
	// "sigs.k8s.io/yaml" // Not needed after removing unused funcs
)

// Loader defines the interface for loading Helm charts.
// This interface allows for dependency injection and testability.
// Implementations should handle loading charts from filesystem paths
// and return the loaded chart along with any error encountered.
type Loader interface {
	// Load loads a chart from the specified path.
	// The path can be a directory or a .tgz file.
	// Returns the loaded chart or an error if loading fails.
	Load(chartPath string) (*chart.Chart, error)
}

// Ensure DefaultLoader implements analysis.ChartLoader
var _ analysis.ChartLoader = (*DefaultLoader)(nil)

// DefaultLoader is the standard implementation of Loader.
// It uses Helm's chart.loader package to load charts from the filesystem.
type DefaultLoader struct {
	fs fileutil.FS // Filesystem implementation to use
}

// NewDefaultLoader creates a new DefaultLoader instance with the provided
// filesystem implementation. If fs is nil, it uses the default filesystem.
func NewDefaultLoader(fs fileutil.FS) *DefaultLoader {
	if fs == nil {
		fs = fileutil.DefaultFS
	}
	return &DefaultLoader{fs: fs}
}

// SetFS replaces the filesystem used by the loader and returns a cleanup function
func (l *DefaultLoader) SetFS(fs fileutil.FS) func() {
	oldFS := l.fs
	l.fs = fs
	return func() {
		l.fs = oldFS
	}
}

// Load implements the Loader interface.
// It loads a chart from the specified path using Helm's loader.
// The path can be a directory containing an unpacked chart or a .tgz file.
// If the path is relative, it's resolved against the current working directory.
//
// Returns:
//   - *chart.Chart: The loaded chart with its values and dependencies
//   - error: An error if loading fails, which can happen if:
//   - The path doesn't exist
//   - The path doesn't contain a valid chart
//   - There are issues with the chart's structure or metadata
func (l *DefaultLoader) Load(chartPath string) (*chart.Chart, error) {
	debug.Printf("Loading chart from path: %s", chartPath)

	// Convert to absolute path if it's relative
	// Note: Although we're injecting our filesystem for testing,
	// we still need to use filepath.Abs here since Helm's loader
	// expects real filesystem paths
	absPath, err := filepath.Abs(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", chartPath, err)
	}
	debug.Printf("Absolute chart path: %s", absPath)

	// Verify the chart path exists using our injectable filesystem
	_, err = l.fs.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("chart path stat error %s: %w", absPath, err)
	}

	// Load the chart
	// Note: We're still using Helm's loader which uses the real filesystem
	// In a future refactoring, we could consider adapting Helm's loader to use our FS interface
	loadedChart, err := loader.Load(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart from %s: %w", absPath, err)
	}
	debug.Printf("Successfully loaded chart: %s (version: %s)", loadedChart.Name(), loadedChart.Metadata.Version)

	// Ensure chart has values
	if loadedChart.Values == nil {
		debug.Printf("Chart has no values, creating empty values map")
		loadedChart.Values = make(map[string]interface{})
	}

	// Output dependency information if present
	if len(loadedChart.Dependencies()) > 0 {
		debug.Printf("Chart has %d dependencies:", len(loadedChart.Dependencies()))
		for i, dep := range loadedChart.Dependencies() {
			debug.Printf("  [%d] %s (version: %s)", i, dep.Name(), dep.Metadata.Version)
		}
	} else {
		debug.Printf("Chart has no dependencies")
	}

	return loadedChart, nil
}

// --- Removed unused ChartData struct and helper functions ---
// - ChartData struct
// - loadChartFromDir
// - loadChartFromArchive
// - processChart
// - isDir
// - evaluateCondition
