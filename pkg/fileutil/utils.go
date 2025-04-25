package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isNotExistError checks if an error is a standard os.IsNotExist error
// or contains common "not found" substrings.
func isNotExistError(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) {
		return true
	}
	// Check for common substrings for broader compatibility (e.g., with afero errors)
	lowerCaseError := strings.ToLower(err.Error())
	return strings.Contains(lowerCaseError, "file does not exist") || strings.Contains(lowerCaseError, "no such file or directory")
}

// FileExists checks if a file exists at the given path
func FileExists(path string) (bool, error) {
	info, err := DefaultFS.Stat(path)
	if err != nil {
		if isNotExistError(err) {
			return false, nil
		}
		// Wrap other Stat errors
		return false, fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	// Defensive check against nil FileInfo
	if info == nil {
		return false, fmt.Errorf("received nil FileInfo with no error from Stat for path %s", path)
	}

	return !info.IsDir(), nil
}

// DirExists checks if a directory exists at the given path
func DirExists(path string) (bool, error) {
	info, err := DefaultFS.Stat(path)
	if err != nil {
		if isNotExistError(err) {
			return false, nil
		}
		// Wrap other Stat errors
		return false, fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	// Defensive check against nil FileInfo
	if info == nil {
		return false, fmt.Errorf("received nil FileInfo with no error from Stat for path %s", path)
	}

	return info.IsDir(), nil
}

// EnsureDirExists ensures a directory exists at the given path
func EnsureDirExists(path string) error {
	exists, err := DirExists(path)
	// Use isNotExistError for checking the error from DirExists
	if err != nil && !isNotExistError(err) {
		return err
	}

	if !exists {
		fileExists, fileErr := FileExists(path)
		// Use isNotExistError for checking the error from FileExists
		if fileErr != nil && !isNotExistError(fileErr) {
			return fileErr
		}
		if fileExists {
			return fmt.Errorf("cannot create directory at %s: path exists as a file", path)
		}

		if mkdirErr := DefaultFS.MkdirAll(path, ReadWriteExecuteUserReadExecuteOthers); mkdirErr != nil {
			return fmt.Errorf("failed to create directory: %w", mkdirErr)
		}
	}
	return nil
}

// ReadFileString reads a file and returns its contents as a string
func ReadFileString(path string) (string, error) {
	data, err := DefaultFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(data), nil
}

// WriteFileString writes a string to a file
func WriteFileString(path, content string) error {
	err := DefaultFS.WriteFile(path, []byte(content), ReadWriteUserReadOthers)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// JoinPath joins path components using the OS-specific separator
func JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}

// GetAbsPath returns the absolute path of a file
func GetAbsPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}
	return abs, nil
}
