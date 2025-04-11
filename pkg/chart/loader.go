// Package chart provides functionality for loading and processing Helm charts.
package chart

import (
	"fmt"
	"path/filepath"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/debug"
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
type DefaultLoader struct{}

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
	absPath, err := filepath.Abs(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", chartPath, err)
	}
	debug.Printf("Absolute chart path: %s", absPath)

	// Load the chart
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
