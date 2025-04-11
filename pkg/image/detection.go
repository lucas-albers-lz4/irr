// Package image provides core functionality for detecting, parsing, and normalizing container
// image references within Helm chart values.
//
// This package is responsible for:
// - Detecting image references in chart values (map and string formats)
// - Parsing image references into their component parts (registry, repository, tag, digest)
// - Normalizing references for consistent processing
// - Validating image reference components
// - Supporting template variable detection and handling
//
// Usage Example:
//
//	detector := image.NewDetector(image.DetectionContext{
//		SourceRegistries: []string{"docker.io", "quay.io"},
//		ExcludeRegistries: []string{"internal-registry.example.com"},
//		Strict: false,
//		TemplateMode: true,
//	})
//	detected, unsupported, err := detector.DetectImages(chartValues, []string{})
//
// The package uses a modular design with components separated into logical files.
package image

// This package was originally contained in a single detection.go file but has been refactored into multiple files:
//
// - types.go: Contains all type definitions including Reference, DetectionContext, LocationType, etc.
// - errors.go: Contains all error definitions for the package
// - detector.go: Contains the Detector implementation (DetectImages and related methods)
// - parser.go: Contains image reference parsing functions
// - normalization.go: Contains registry and reference normalization functions
// - validation.go: Contains validation functions for registry names, repository names, etc.
// - path_patterns.go: Contains path pattern definitions and compilation function
//
// The refactoring maintains the same functionality while making the codebase more modular and maintainable.

// Constants for image pattern types
const (
	PatternMap    = "map"    // Map-based image reference
	PatternString = "string" // Single string value (e.g., "nginx:latest")
	PatternGlobal = "global" // Global registry pattern
)
