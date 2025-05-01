// Package helmtypes provides shared types and interfaces for Helm chart loading and value origin tracking.
package helmtypes

import (
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli/values"
)

// ValueOriginType defines the source of a specific value during Helm's value merging process.
type ValueOriginType int

const (
	// OriginUnknown indicates an unknown or untracked origin.
	OriginUnknown ValueOriginType = iota
	// OriginChartDefault indicates the value came from a chart's default values.yaml.
	OriginChartDefault
	// OriginUserFile indicates the value came from a user-supplied values file (-f).
	OriginUserFile
	// OriginUserSet indicates the value came from a --set, --set-string, or --set-file flag.
	OriginUserSet
	// OriginParentOverride indicates the value came from a parent chart overriding a subchart's default.
	OriginParentOverride
)

// ValueOrigin holds metadata about where a specific value originated from during Helm's value computation.
type ValueOrigin struct {
	Type      ValueOriginType `json:"type"`
	Path      string          `json:"path,omitempty"`
	ChartName string          `json:"chartName,omitempty"`
	Alias     string          `json:"alias,omitempty"`
	Key       string          `json:"key,omitempty"`
}

// ChartLoaderOptions contains the options for loading a chart.
type ChartLoaderOptions struct {
	ChartPath  string
	ValuesOpts values.Options
}

// ChartAnalysisContext holds the results of chart loading with value origin tracking.
type ChartAnalysisContext struct {
	LoadedChart  *chart.Chart           `json:"-"`
	MergedValues map[string]interface{} `json:"mergedValues"`
	Origins      map[string]ValueOrigin `json:"origins"`
	ChartRoot    string                 `json:"chartRoot"`
}

// NewChartAnalysisContext creates a new context for chart analysis.
func NewChartAnalysisContext(
	helmChart *chart.Chart,
	mergedValues map[string]interface{},
	origins map[string]ValueOrigin,
	chartRoot string,
) *ChartAnalysisContext {
	if origins == nil {
		origins = make(map[string]ValueOrigin)
	}
	if mergedValues == nil {
		mergedValues = make(map[string]interface{})
	}
	return &ChartAnalysisContext{
		LoadedChart:  helmChart,
		MergedValues: mergedValues,
		Origins:      origins,
		ChartRoot:    chartRoot,
	}
}

// ChartLoader defines the interface for loading Helm charts and their values.
// This allows mocking the chart loading process during testing.
type ChartLoader interface {
	// LoadChartWithValues loads a chart and computes its merged values based on provided options.
	LoadChartWithValues(opts *ChartLoaderOptions) (*chart.Chart, map[string]interface{}, error)
	// LoadChartAndTrackOrigins loads a chart and provides a context containing the merged values
	// and the origin of each value.
	LoadChartAndTrackOrigins(opts *ChartLoaderOptions) (*ChartAnalysisContext, error)
}
