// Package registry handles registry mapping logic and associated errors.
package registry

import (
	"fmt"
)

// Define specific error types for registry mapping issues.

// ErrMappingPathNotInWD indicates the mappings file path is outside the current working directory tree.
type ErrMappingPathNotInWD struct {
	Path string
}

func (e *ErrMappingPathNotInWD) Error() string {
	return fmt.Sprintf("mappings file path '%s' must be within the current working directory tree", e.Path)
}

// WrapMappingPathNotInWD creates a new ErrMappingPathNotInWD error.
func WrapMappingPathNotInWD(path string) error {
	return &ErrMappingPathNotInWD{Path: path}
}

// ErrMappingExtension indicates the mappings file path has an invalid extension.
type ErrMappingExtension struct {
	Path string
}

func (e *ErrMappingExtension) Error() string {
	return fmt.Sprintf("mappings file path must end with .yaml or .yml: %s", e.Path)
}

// WrapMappingExtension creates a new ErrMappingExtension error.
func WrapMappingExtension(path string) error {
	return &ErrMappingExtension{Path: path}
}

// ErrMappingFileNotExist indicates the mappings file does not exist.
type ErrMappingFileNotExist struct {
	Path string
	Err  error
}

func (e *ErrMappingFileNotExist) Error() string {
	return fmt.Sprintf("mappings file does not exist: %s (%v)", e.Path, e.Err)
}

func (e *ErrMappingFileNotExist) Unwrap() error {
	return e.Err
}

// WrapMappingFileNotExist creates a new ErrMappingFileNotExist error.
func WrapMappingFileNotExist(path string, err error) error {
	return &ErrMappingFileNotExist{Path: path, Err: err}
}

// ErrMappingFileRead indicates an error occurred while reading the mappings file.
type ErrMappingFileRead struct {
	Path string
	Err  error
}

func (e *ErrMappingFileRead) Error() string {
	// Include the underlying error message, which might contain "is a directory"
	return fmt.Sprintf("failed to read mappings file '%s': %v", e.Path, e.Err)
}

func (e *ErrMappingFileRead) Unwrap() error {
	return e.Err
}

// WrapMappingFileRead creates a new ErrMappingFileRead error.
func WrapMappingFileRead(path string, err error) error {
	return &ErrMappingFileRead{Path: path, Err: err}
}

// ErrMappingFileEmpty indicates the mappings file is empty.
type ErrMappingFileEmpty struct {
	Path string
}

func (e *ErrMappingFileEmpty) Error() string {
	return fmt.Sprintf("mappings file is empty: %s", e.Path)
}

// WrapMappingFileEmpty creates a new ErrMappingFileEmpty error.
func WrapMappingFileEmpty(path string) error {
	return &ErrMappingFileEmpty{Path: path}
}

// ErrMappingFileParse indicates an error occurred while parsing the mappings file content.
type ErrMappingFileParse struct {
	Path string
	Err  error
}

func (e *ErrMappingFileParse) Error() string {
	return fmt.Sprintf("failed to parse mappings file '%s': %v", e.Path, e.Err)
}

func (e *ErrMappingFileParse) Unwrap() error {
	return e.Err
}

// WrapMappingFileParse creates a new ErrMappingFileParse error.
func WrapMappingFileParse(path string, err error) error {
	return &ErrMappingFileParse{Path: path, Err: err}
}
