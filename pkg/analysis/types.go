// Package analysis defines types used during the chart analysis process.
package analysis

// PatternType represents the type of pattern found
type PatternType string

const (
	// PatternTypeMap represents an image defined as a map with registry, repository, tag
	PatternTypeMap PatternType = "map"
	// PatternTypeString represents an image defined as a single string
	PatternTypeString PatternType = "string"
	// PatternTypeGlobal represents a global registry configuration
	PatternTypeGlobal PatternType = "global"
)

// ImagePattern represents a discovered image pattern
type ImagePattern struct {
	Path      string                 // Path in values where pattern was found
	Type      PatternType            // Type of pattern (map or string)
	Structure map[string]interface{} // For map type, the full structure
	Value     string                 // For string type, the image reference
	Count     int                    // Number of occurrences
}

// GlobalPattern represents a global registry configuration
type GlobalPattern struct {
	Type PatternType // Type of pattern (always global)
	Path string      // Path in values where pattern was found
}

// ChartAnalysis contains the results of analyzing a chart
type ChartAnalysis struct {
	ImagePatterns  []ImagePattern  // List of image patterns found
	GlobalPatterns []GlobalPattern // List of global patterns found
}

// NewChartAnalysis creates a new ChartAnalysis
func NewChartAnalysis() *ChartAnalysis {
	return &ChartAnalysis{
		ImagePatterns:  make([]ImagePattern, 0),
		GlobalPatterns: make([]GlobalPattern, 0),
	}
}

// Options configures the analysis process.
type Options struct {
	ChartPath string `yaml:"chartPath"`
	// Add other configuration fields as needed
}
