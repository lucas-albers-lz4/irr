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

// GetPath returns the file path that caused the error
func (e *ErrMappingFileRead) GetPath() string {
	return e.Path
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

// GetPath returns the file path that caused the error
func (e *ErrMappingFileParse) GetPath() string {
	return e.Path
}

// WrapMappingFileParse creates a new ErrMappingFileParse error.
func WrapMappingFileParse(path string, err error) error {
	return &ErrMappingFileParse{Path: path, Err: err}
}

// ErrDuplicateRegistryKey indicates a duplicate registry key was found.
type ErrDuplicateRegistryKey struct {
	Path string
	Key  string
}

func (e *ErrDuplicateRegistryKey) Error() string {
	return fmt.Sprintf("duplicate registry key '%s' found in mappings file '%s'", e.Key, e.Path)
}

// WrapDuplicateRegistryKey creates a new ErrDuplicateRegistryKey error.
func WrapDuplicateRegistryKey(path, key string) error {
	return &ErrDuplicateRegistryKey{Path: path, Key: key}
}

// ErrInvalidPortNumber indicates an invalid port number in the registry mapping.
type ErrInvalidPortNumber struct {
	Path  string
	Key   string
	Value string
	Port  string
}

func (e *ErrInvalidPortNumber) Error() string {
	return fmt.Sprintf("invalid port number '%s' in target registry value '%s' for source '%s' in mappings file '%s': port must be between 1 and 65535", e.Port, e.Value, e.Key, e.Path)
}

// WrapInvalidPortNumber creates a new ErrInvalidPortNumber error.
func WrapInvalidPortNumber(path, key, value, port string) error {
	return &ErrInvalidPortNumber{Path: path, Key: key, Value: value, Port: port}
}

// ErrKeyTooLong indicates a registry key exceeds the maximum allowed length.
type ErrKeyTooLong struct {
	Path   string
	Key    string
	Length int
	Max    int
}

func (e *ErrKeyTooLong) Error() string {
	return fmt.Sprintf("registry key '%s' exceeds maximum length of %d characters in mappings file '%s'", e.Key, e.Max, e.Path)
}

// WrapKeyTooLong creates a new ErrKeyTooLong error.
func WrapKeyTooLong(path, key string, length, maxLength int) error {
	return &ErrKeyTooLong{Path: path, Key: key, Length: length, Max: maxLength}
}

// ErrValueTooLong indicates a registry value exceeds the maximum allowed length.
type ErrValueTooLong struct {
	Path   string
	Key    string
	Value  string
	Length int
	Max    int
}

func (e *ErrValueTooLong) Error() string {
	return fmt.Sprintf("registry value '%s' for key '%s' exceeds maximum length of %d characters in mappings file '%s'", e.Value, e.Key, e.Max, e.Path)
}

// WrapValueTooLong creates a new ErrValueTooLong error.
func WrapValueTooLong(path, key, value string, length, maxLength int) error {
	return &ErrValueTooLong{Path: path, Key: key, Value: value, Length: length, Max: maxLength}
}
