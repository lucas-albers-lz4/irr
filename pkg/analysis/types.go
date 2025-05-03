// Package analysis defines types used during the chart analysis process.
// It provides data structures for representing the results of analyzing Helm charts,
// including the detection of container image patterns and global registry configurations.
package analysis

// PatternType represents the type of pattern found during chart analysis.
// This helps categorize different ways images can be defined in Helm charts.
type PatternType string

const (
	// PatternTypeMap represents an image defined as a map with registry, repository, tag
	// Example in values.yaml: image: {registry: "docker.io", repository: "nginx", tag: "1.19"}
	PatternTypeMap PatternType = "map"

	// PatternTypeString represents an image defined as a single string
	// Example in values.yaml: image: "docker.io/nginx:1.19"
	PatternTypeString PatternType = "string"

	// PatternTypeGlobal represents a global registry configuration
	// Example in values.yaml: global: {registry: "my-registry.example.com"}
	PatternTypeGlobal PatternType = "global"
)

// ImagePattern represents a discovered image pattern during chart analysis.
// It contains information about where the pattern was found, its type,
// and the specific image reference details.
type ImagePattern struct {
	Path      string                 // Path in values where pattern was found (e.g., "image" or "deployment.image")
	Type      PatternType            // Type of pattern (map or string)
	Structure map[string]interface{} `json:"structure,omitempty" yaml:"structure,omitempty"` // Detailed structure if Type is map
	Value     string                 // For string type, the image reference (e.g., "docker.io/nginx:1.19")
	Count     int                    `json:"count" yaml:"count"` // How many times this exact pattern was found
	// Added for context-aware analysis:
	OriginalRegistry string `json:"originalRegistry,omitempty" yaml:"originalRegistry,omitempty"` // Original registry from source chart if different
	SourceOrigin     string `json:"sourceOrigin,omitempty" yaml:"sourceOrigin,omitempty"`         // Originating file/path from context analysis
	// Added for subchart app version fallback:
	SourceChartAppVersion string `json:"sourceChartAppVersion,omitempty" yaml:"sourceChartAppVersion,omitempty"` // AppVersion of the originating chart
}

// GlobalPattern represents a global registry configuration found in the chart.
// Global patterns can be used to override registry settings for all images
// in a chart or subchart.
type GlobalPattern struct {
	Type PatternType // Type of pattern (always global)
	Path string      // Path in values where pattern was found (e.g., "global.registry")
}

// ChartAnalysis contains the results of analyzing a chart for image patterns.
// It stores both specific image patterns and global registry configurations
// that were detected during the analysis process.
type ChartAnalysis struct {
	ImagePatterns  []ImagePattern  // List of image patterns found in the chart
	GlobalPatterns []GlobalPattern // List of global registry configurations found
}

// NewChartAnalysis creates a new ChartAnalysis instance with empty pattern lists.
// This is used as the starting point for chart analysis.
func NewChartAnalysis() *ChartAnalysis {
	return &ChartAnalysis{
		ImagePatterns:  make([]ImagePattern, 0),
		GlobalPatterns: make([]GlobalPattern, 0),
	}
}

// Options configures the analysis process.
// It contains parameters that control how chart analysis is performed.
type Options struct {
	ChartPath string `yaml:"chartPath"` // Path to the Helm chart to analyze
	// Add other configuration fields as needed
}
