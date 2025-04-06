package chart

import (
	"fmt"
	"strings"
)

// Custom error types for the chart package

// UnsupportedStructureError indicates an image reference was found in a structure
// that the tool cannot currently process for overrides.
type UnsupportedStructureError struct {
	Path []string
	Type string // e.g., "map-registry-repository-tag", "string"
}

func (e *UnsupportedStructureError) Error() string {
	return fmt.Sprintf("unsupported structure at path %s (type: %s)", strings.Join(e.Path, "."), e.Type)
}

// ThresholdNotMetError indicates that the percentage of successfully processed
// images did not meet the required threshold.
type ThresholdNotMetError struct {
	Actual   int
	Required int
}

func (e *ThresholdNotMetError) Error() string {
	return fmt.Sprintf("processing threshold not met: required %d%%, actual %d%%", e.Required, e.Actual)
}

// ChartParsingError indicates a failure during the loading or initial parsing
// of the Helm chart itself (e.g., malformed Chart.yaml or values.yaml).
type ChartParsingError struct {
	FilePath string
	Err      error // Underlying error (e.g., from Helm loader or YAML parser)
}

func (e *ChartParsingError) Error() string {
	return fmt.Sprintf("chart parsing failed for %s: %v", e.FilePath, e.Err)
}
func (e *ChartParsingError) Unwrap() error { return e.Err } // Allow unwrapping

// ImageProcessingError indicates a failure during the processing of a specific
// image reference after it has been detected.
type ImageProcessingError struct {
	Path []string
	Ref  string // The problematic image reference string
	Err  error  // Underlying error (e.g., from path strategy, normalization)
}

func (e *ImageProcessingError) Error() string {
	return fmt.Sprintf("image processing failed at path %s for ref '%s': %v", strings.Join(e.Path, "."), e.Ref, e.Err)
}
func (e *ImageProcessingError) Unwrap() error { return e.Err } // Allow unwrapping
