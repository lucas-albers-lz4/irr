// Package helm provides internal utilities for interacting with Helm.
package helm

import (
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/log"
	"helm.sh/helm/v3/pkg/chart"
)

// ChartAnalysisContext encapsulates all information needed to analyze a chart's images.
type ChartAnalysisContext struct {
	// The Helm chart being analyzed
	Chart *chart.Chart

	// The final merged values that would be used for rendering
	Values map[string]interface{}

	// Origin tracking for values
	Origins map[string]ValueOrigin

	// Metadata about this analysis
	ChartName    string
	ChartVersion string
	ValuesFiles  []string
	SetValues    []string
}

// GetSourcePathForValue determines the source path for a given value based on its origin.
// This is crucial for generating correct source paths for images found in subchart values.
func (ctx *ChartAnalysisContext) GetSourcePathForValue(valuePath string) string {
	// Default to the base path if no origin info available
	if ctx.Origins == nil {
		return valuePath
	}

	origin, exists := ctx.Origins[valuePath]
	if !exists {
		// If exact path not found, try checking parent paths for aliasing/global overrides
		// This is a simplified check; more robust logic might be needed.
		parts := strings.Split(valuePath, ".")
		for i := len(parts) - 1; i > 0; i-- {
			parentPath := strings.Join(parts[:i], ".")
			if parentOrigin, parentExists := ctx.Origins[parentPath]; parentExists {
				// TODO: Add more sophisticated logic here if needed, e.g., alias handling
				// For now, use the origin of the nearest parent found.
				origin = parentOrigin
				exists = true
				break
			}
		}
		// If still no origin found after checking parents, return the original path
		if !exists {
			return valuePath
		}
	}

	// Handle different origin types
	switch origin.Type {
	case OriginChartDefault, OriginParentValues:
		// If the value originates from a different chart (subchart), prepend its name.
		if origin.ChartName != "" && origin.ChartName != ctx.ChartName {
			return origin.ChartName + "." + valuePath
		}
		// Otherwise, it's from the top-level chart, return the path as is.
		return valuePath

	case OriginUserFile, OriginUserSet:
		// User-provided values apply to the top-level context, don't change the path structure relative to that.
		return valuePath

	case OriginAlias:
		// TODO: Implement proper alias handling.
		// For now, return the path as is, assuming it might be correct or needs manual adjustment.
		// A more complete solution would involve tracing the alias back to the original chart
		// and prepending the alias name.
		log.Warn("Alias origin type detected, but full alias handling not yet implemented.", "path", valuePath, "originChart", origin.ChartName)
		return valuePath // Placeholder

	case OriginGlobal:
		// Globals are typically accessed directly, e.g., .Values.global.someValue
		// The tracked path should already include 'global.' prefix from the merging logic.
		// No modification needed here as the path should be correct as tracked.
		return valuePath

	default:
		log.Warn("Unknown value origin type encountered", "type", origin.Type, "path", valuePath)
		return valuePath
	}
}

// NewChartAnalysisContext creates a new context for chart analysis.
func NewChartAnalysisContext(chartData *chart.Chart, values map[string]interface{}, origins map[string]ValueOrigin, valuesFiles, setValues []string) *ChartAnalysisContext {
	return &ChartAnalysisContext{
		Chart:        chartData,
		Values:       values,
		Origins:      origins,
		ChartName:    chartData.Name(),
		ChartVersion: chartData.Metadata.Version,
		ValuesFiles:  valuesFiles,
		SetValues:    setValues,
	}
}

// NewChartAnalysisContextFromCoalesced creates a context from a chart and coalesced values.
func NewChartAnalysisContextFromCoalesced(chartData *chart.Chart, coalesced *CoalescedValues, valuesFiles, setValues []string) *ChartAnalysisContext {
	return &ChartAnalysisContext{
		Chart:        chartData,
		Values:       coalesced.Values,
		Origins:      coalesced.Origins,
		ChartName:    chartData.Name(),
		ChartVersion: chartData.Metadata.Version,
		ValuesFiles:  valuesFiles,
		SetValues:    setValues,
	}
}
