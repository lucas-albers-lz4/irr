package chart

// Info represents basic information about a Helm chart
type Info struct {
	Name         string `json:"name" yaml:"name"`
	Version      string `json:"version" yaml:"version"`
	Path         string `json:"path" yaml:"path"`
	Dependencies int    `json:"dependencies" yaml:"dependencies"`
}

// ImageInfo holds information about an image reference
type ImageInfo struct {
	Registry   string `json:"registry" yaml:"registry"`
	Repository string `json:"repository" yaml:"repository"`
	Tag        string `json:"tag,omitempty" yaml:"tag,omitempty"`
	Digest     string `json:"digest,omitempty" yaml:"digest,omitempty"`
	Source     string `json:"source" yaml:"source"`
}

// ImageAnalysis holds the comprehensive analysis results
type ImageAnalysis struct {
	Chart    Info        `json:"chart" yaml:"chart"`
	Images   []ImageInfo `json:"images" yaml:"images"`
	Patterns interface{} `json:"patterns" yaml:"patterns"`
	Errors   []string    `json:"errors,omitempty" yaml:"errors,omitempty"`
	Skipped  []string    `json:"skipped,omitempty" yaml:"skipped,omitempty"`
}

// OverrideResult represents the results of an override operation
type OverrideResult struct {
	Chart          Info        `json:"chart" yaml:"chart"`
	TargetRegistry string      `json:"targetRegistry" yaml:"targetRegistry"`
	OverrideMap    interface{} `json:"overrideMap" yaml:"overrideMap"`
	ProcessedCount int         `json:"processedCount" yaml:"processedCount"`
	TotalCount     int         `json:"totalCount" yaml:"totalCount"`
	SuccessRate    float64     `json:"successRate" yaml:"successRate"`
	Errors         []string    `json:"errors,omitempty" yaml:"errors,omitempty"`
}
