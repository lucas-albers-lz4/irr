// Package image provides core functionality for detecting, parsing, and normalizing container image references within Helm chart values.
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
