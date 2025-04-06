// Package registry provides functionality for managing container registry information.
package registry

import (
	"errors"
	"fmt"
)

// Registry mapping errors.
var (
	// ErrInvalidMappingPath is returned when the registry mapping file path is invalid.
	ErrInvalidMappingPath = errors.New("invalid mappings file path")

	// ErrMappingPathNotInWD is returned when the registry mapping file is not within the current working directory tree.
	ErrMappingPathNotInWD = errors.New("mappings file path must be within the current working directory tree")

	// ErrInvalidMappingExtension is returned when the registry mapping file does not have a valid extension (.yaml or .yml).
	ErrInvalidMappingExtension = errors.New("mappings file path must end with .yaml or .yml")

	// ErrMappingFileNotExist is returned when the registry mapping file does not exist.
	ErrMappingFileNotExist = errors.New("mappings file does not exist")

	// ErrMappingFileRead is returned when the registry mapping file cannot be read.
	ErrMappingFileRead = errors.New("failed to read mappings file")

	// ErrMappingFileParse is returned when the registry mapping file cannot be parsed.
	ErrMappingFileParse = errors.New("failed to parse mappings file")

	// ErrNoMappingsLoaded is returned when no registry mappings could be loaded.
	ErrNoMappingsLoaded = errors.New("no registry mappings loaded")

	// ErrNoTargetMapping is returned when no mapping exists for the specified source registry.
	ErrNoTargetMapping = errors.New("no target registry mapping found for source registry")
)

// WrapMappingPathNotInWD wraps ErrMappingPathNotInWD with the given path for context.
func WrapMappingPathNotInWD(path string) error {
	return fmt.Errorf("%w: %s", ErrMappingPathNotInWD, path)
}

// WrapMappingExtension wraps ErrInvalidMappingExtension with the given path for context.
func WrapMappingExtension(path string) error {
	return fmt.Errorf("%w: %s", ErrInvalidMappingExtension, path)
}

// WrapMappingFileNotExist wraps ErrMappingFileNotExist with the given path and original error for context.
func WrapMappingFileNotExist(path string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrMappingFileNotExist, path, err)
}

// WrapMappingFileRead wraps ErrMappingFileRead with the given path and original error for context.
func WrapMappingFileRead(path string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrMappingFileRead, path, err)
}

// WrapMappingFileParse wraps ErrMappingFileParse with the given path and original error for context.
func WrapMappingFileParse(path string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrMappingFileParse, path, err)
}

// WrapNoTargetMapping wraps ErrNoTargetMapping with the given source registry for context.
func WrapNoTargetMapping(sourceRegistry string) error {
	return fmt.Errorf("%w: %s", ErrNoTargetMapping, sourceRegistry)
}
