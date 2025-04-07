// Package chart provides functionality for loading and processing Helm charts.
package chart

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lalbers/irr/pkg/debug"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	// "helm.sh/helm/v3/pkg/chartutil" // Not needed after removing unused funcs
	// "sigs.k8s.io/yaml" // Not needed after removing unused funcs
)

// Loader defines the interface for loading Helm charts.
type Loader interface {
	Load(chartPath string) (*chart.Chart, error)
}

// helmLoader implements the Loader interface using the Helm library.
type helmLoader struct {
	// No fields needed for this implementation as it uses library functions
}

// NewLoader creates a new Loader instance that uses the Helm library.
func NewLoader() Loader {
	return &helmLoader{}
}

// Load loads a Helm chart from a directory or .tgz file using the Helm library.
func (l *helmLoader) Load(chartPath string) (*chart.Chart, error) {
	debug.FunctionEnter("[helmLoader] Load")
	defer debug.FunctionExit("[helmLoader] Load")

	debug.Printf("Loading chart from path: %s", chartPath)

	// Check if path exists using standard os.Stat
	_, err := os.Stat(chartPath)
	if err != nil {
		debug.Printf("Error checking chart path: %v", err)
		// Helm loader functions below will handle path errors with more context
	}

	// Load the chart using Helm's loader funcs
	var chartData *chart.Chart
	if filepath.Ext(chartPath) == ".tgz" {
		debug.Printf("Loading packaged chart from .tgz file using loader.Load")
		chartData, err = loader.Load(chartPath)
	} else {
		// Assume directory if not .tgz. loader.LoadDir handles non-dirs.
		debug.Printf("Loading chart from directory using loader.LoadDir")
		chartData, err = loader.LoadDir(chartPath)
	}

	if err != nil {
		debug.Printf("Error loading chart: %v", err)
		// Return the Helm library's error directly for better detail
		return nil, fmt.Errorf("helm chart load failed: %w", err)
	}

	debug.Printf("Successfully loaded chart: %s", chartData.Name())
	// Optional: Keep debug dumps if useful
	// debug.DumpValue("Chart Metadata", chartData.Metadata)
	// debug.DumpValue("Chart Values", chartData.Values)
	// debug.Printf("Number of dependencies: %d", len(chartData.Dependencies()))

	return chartData, nil
}

// --- Removed unused ChartData struct and helper functions ---
// - ChartData struct
// - loadChartFromDir
// - loadChartFromArchive
// - processChart
// - isDir
// - evaluateCondition
