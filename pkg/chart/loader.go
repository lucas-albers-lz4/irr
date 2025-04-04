package chart

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lalbers/helm-image-override/pkg/debug"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"sigs.k8s.io/yaml"
)

// ChartData represents a loaded Helm chart with its values and dependencies
type ChartData struct {
	Name         string
	Path         string
	Values       map[string]interface{}
	Dependencies []*ChartData
}

// LoadChart loads a Helm chart from a directory or .tgz file
func LoadChart(chartPath string) (*chart.Chart, error) {
	debug.FunctionEnter("LoadChart")
	defer debug.FunctionExit("LoadChart")

	debug.Printf("Loading chart from path: %s", chartPath)

	// Check if path exists
	_, err := os.Stat(chartPath)
	if err != nil {
		debug.Printf("Error accessing chart path: %v", err)
		return nil, fmt.Errorf("error accessing chart path: %v", err)
	}

	// Load the chart
	var chartData *chart.Chart
	if filepath.Ext(chartPath) == ".tgz" {
		debug.Printf("Loading packaged chart from .tgz file")
		chartData, err = loader.Load(chartPath)
	} else {
		debug.Printf("Loading chart from directory")
		chartData, err = loader.LoadDir(chartPath)
	}

	if err != nil {
		debug.Printf("Error loading chart: %v", err)
		return nil, fmt.Errorf("error loading chart: %v", err)
	}

	debug.Printf("Successfully loaded chart: %s", chartData.Name())
	debug.DumpValue("Chart Metadata", chartData.Metadata)
	debug.DumpValue("Chart Values", chartData.Values)
	debug.Printf("Number of dependencies: %d", len(chartData.Dependencies()))

	return chartData, nil
}

// loadChartFromDir loads a chart from a local directory
func loadChartFromDir(dirPath string) (*ChartData, error) {
	helmChart, err := loader.LoadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart from directory: %w", err)
	}

	return processChart(helmChart, dirPath)
}

// loadChartFromArchive loads a chart from a .tgz archive
func loadChartFromArchive(archivePath string) (*ChartData, error) {
	helmChart, err := loader.LoadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart from archive: %w", err)
	}

	return processChart(helmChart, archivePath)
}

// processChart extracts values and dependencies from the Helm chart
func processChart(helmChart *chart.Chart, chartPath string) (*ChartData, error) {
	// Extract values from chart
	valuesBytes, err := yaml.Marshal(helmChart.Values)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal values from chart: %w", err)
	}

	// Convert YAML to map
	values, err := chartutil.ReadValues(valuesBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse values: %w", err)
	}

	chartData := &ChartData{
		Name:   helmChart.Name(),
		Path:   chartPath,
		Values: values,
	}

	// Process dependencies if any exist
	if helmChart.Metadata != nil && len(helmChart.Metadata.Dependencies) > 0 {
		chartData.Dependencies = make([]*ChartData, 0, len(helmChart.Metadata.Dependencies))

		// For TGZ files, dependencies are already included
		// For directory charts, we need to load them from the charts/ directory
		if isDir(chartPath) {
			chartsDir := filepath.Join(chartPath, "charts")
			if _, err := os.Stat(chartsDir); err == nil {
				for _, dep := range helmChart.Metadata.Dependencies {
					// Skip dependencies that may be disabled via condition
					if dep.Condition != "" && !evaluateCondition(dep.Condition, values) {
						continue
					}

					// Check if there's a subdirectory or tgz file matching the dependency
					depName := dep.Name
					if dep.Alias != "" {
						depName = dep.Alias
					}

					depPath := filepath.Join(chartsDir, depName)
					if _, err := os.Stat(depPath); err == nil && isDir(depPath) {
						// Directory found, load it
						depChart, err := loadChartFromDir(depPath)
						if err != nil {
							return nil, fmt.Errorf("failed to load dependency %s: %w", depName, err)
						}
						chartData.Dependencies = append(chartData.Dependencies, depChart)
						continue
					}

					// Try as .tgz archive
					depPath = filepath.Join(chartsDir, fmt.Sprintf("%s-%s.tgz", depName, dep.Version))
					if _, err := os.Stat(depPath); err == nil {
						depChart, err := loadChartFromArchive(depPath)
						if err != nil {
							return nil, fmt.Errorf("failed to load dependency archive %s: %w", depName, err)
						}
						chartData.Dependencies = append(chartData.Dependencies, depChart)
						continue
					}

					// If we get here, we couldn't find the dependency
					return nil, fmt.Errorf("dependency %s not found in charts directory", depName)
				}
			}
		} else {
			// For .tgz files, use the parsed subcharts
			for _, subChart := range helmChart.Dependencies() {
				if subChart.Metadata == nil {
					continue
				}

				// Extract subchart alias from parent's dependencies
				alias := subChart.Name()
				for _, dep := range helmChart.Metadata.Dependencies {
					if dep.Name == subChart.Metadata.Name && dep.Alias != "" {
						alias = dep.Alias
						break
					}
				}

				// Process the subchart
				subValuesBytes, err := yaml.Marshal(subChart.Values)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal values from subchart %s: %w", alias, err)
				}

				parsedValues, err := chartutil.ReadValues(subValuesBytes)
				if err != nil {
					return nil, fmt.Errorf("failed to parse values from subchart %s: %w", alias, err)
				}

				depChart := &ChartData{
					Name:   alias, // Use alias or name
					Path:   fmt.Sprintf("%s/%s", chartPath, alias),
					Values: parsedValues,
				}

				// Recursively process this subchart's dependencies
				if len(subChart.Dependencies()) > 0 {
					depChart.Dependencies = make([]*ChartData, 0, len(subChart.Dependencies()))
					// Note: For simplicity, we're not recursively processing nested dependencies
					// in the archive case in this initial version
				}

				chartData.Dependencies = append(chartData.Dependencies, depChart)
			}
		}
	}

	return chartData, nil
}

// Helper function to check if path is a directory
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Very basic condition evaluator (simplified for this implementation)
// In a production version, this needs to be more robust to handle complex conditions
func evaluateCondition(condition string, values map[string]interface{}) bool {
	// This is a simplistic placeholder. A real implementation would need to:
	// 1. Parse the condition expression
	// 2. Evaluate it against the values map
	// 3. Return true/false based on evaluation

	// For now, we'll assume all dependencies are enabled
	return true
}
