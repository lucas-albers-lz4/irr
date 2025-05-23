// Package chart provides functionality for loading and processing Helm charts.
package chart

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/lucas-albers-lz4/irr/pkg/log"
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
	log.Debug("Loading chart from path", "path", chartPath)

	// Convert to absolute path if it's relative
	// Note: Although we're injecting our filesystem for testing,
	// we still need to use filepath.Abs here since Helm's loader
	// expects real filesystem paths
	absPath, err := filepath.Abs(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %w", chartPath, err)
	}
	log.Debug("Absolute chart path", "path", absPath)

	// Verify the chart path exists using our injectable filesystem
	if l.fs == nil {
		return nil, errors.New("internal error: chart loader has a nil filesystem")
	}
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
	// Add checks for nil chart and nil metadata
	if loadedChart == nil {
		return nil, fmt.Errorf("helm loader returned nil chart for path %s without error", absPath)
	}
	if loadedChart.Metadata == nil {
		// Log a warning but maybe proceed? Or return error?
		// Helm charts *should* always have metadata. Let's treat this as an error.
		chartName := absPath             // Use path as fallback if name isn't available
		if loadedChart.Metadata != nil { // Re-check might be redundant but safe if logic changes
			chartName = loadedChart.Name()
		}
		return nil, fmt.Errorf("loaded chart %s has nil metadata", chartName)
	}

	// Re-check Metadata before accessing Version to satisfy nilaway
	chartVersion := "<unknown>"
	if loadedChart.Metadata != nil {
		chartVersion = loadedChart.Metadata.Version
	}
	log.Debug("Successfully loaded chart", "name", loadedChart.Name(), "version", chartVersion)

	// Ensure chart has values
	if loadedChart.Values == nil {
		log.Debug("Chart has no values, creating empty values map")
		loadedChart.Values = make(map[string]interface{})
	}

	// Output dependency information if present
	if len(loadedChart.Dependencies()) > 0 {
		log.Debug("Chart has dependencies", "count", len(loadedChart.Dependencies()))
		for i, dep := range loadedChart.Dependencies() {
			// Check dependency metadata before accessing
			depVersion := "<unknown>"
			if dep != nil && dep.Metadata != nil {
				depVersion = dep.Metadata.Version
			}
			log.Debug("Dependency", "index", i, "name", dep.Name(), "version", depVersion)
		}
	} else {
		log.Debug("Chart has no dependencies")
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

// ---
// Logging migration progress note:
// - pkg/chart/loader.go: All debug logging migrated to slog-based logger (log.Debug, log.Error, log.Warn).
// - All debug.* calls replaced with slog style logging.
// - Next: Continue migration in other files using the debug package.
// ---
