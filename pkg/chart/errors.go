// Package chart provides functionality for loading and processing Helm charts, including error definitions.
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

// ParsingError represents an error encountered during chart parsing.
type ParsingError struct {
	FilePath string
	Message  string
	Err      error
}

func (e *ParsingError) Error() string {
	return fmt.Sprintf("error parsing chart: %s", e.Err)
}

func (e *ParsingError) Unwrap() error {
	return e.Err
}

// ImageProcessingError indicates an error occurred during image detection or processing.
type ImageProcessingError struct {
	Path []string // Path within the values where the error occurred
	Ref  string   // Image reference string, if available
	Err  error
}

func (e *ImageProcessingError) Error() string {
	if len(e.Path) > 0 {
		return fmt.Sprintf("image processing error at path '%s' (ref: %s): %v", e.Path, e.Ref, e.Err)
	}
	return fmt.Sprintf("image processing error (ref: %s): %v", e.Ref, e.Err)
}

func (e *ImageProcessingError) Unwrap() error {
	return e.Err
}

// ThresholdError indicates processing failed because the success threshold wasn't met
// type ThresholdError struct {
// 	Message string
// }

// func (e *ThresholdError) Error() string {
// 	return e.Message
// }
