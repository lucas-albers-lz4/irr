package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileExists checks if a file exists at the given path
func FileExists(path string) (bool, error) {
	info, err := DefaultFS.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		// Don't propagate "file not found" errors
		errMsg := err.Error()
		if errMsg == os.ErrNotExist.Error() ||
			errMsg == "file does not exist" ||
			errMsg == "no such file or directory" ||
			strings.Contains(errMsg, "file does not exist") ||
			strings.Contains(errMsg, "no such file or directory") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if file exists: %w", err)
	}

	// Defensive check against nil FileInfo
	if info == nil {
		return false, fmt.Errorf("received nil FileInfo with no error from Stat")
	}

	return !info.IsDir(), nil
}

// DirExists checks if a directory exists at the given path
func DirExists(path string) (bool, error) {
	info, err := DefaultFS.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		// Don't propagate "file not found" errors
		errMsg := err.Error()
		if errMsg == os.ErrNotExist.Error() ||
			errMsg == "file does not exist" ||
			errMsg == "no such file or directory" ||
			strings.Contains(errMsg, "file does not exist") ||
			strings.Contains(errMsg, "no such file or directory") {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat directory: %w", err)
	}

	// Defensive check against nil FileInfo
	if info == nil {
		return false, fmt.Errorf("received nil FileInfo with no error from Stat")
	}

	return info.IsDir(), nil
}

// EnsureDirExists ensures a directory exists at the given path
func EnsureDirExists(path string) error {
	exists, err := DirExists(path)
	if err != nil {
		return err
	}
	if !exists {
		// Check if path exists as a file (not a directory)
		fileExists, err := FileExists(path)
		if err != nil {
			return err
		}
		if fileExists {
			return fmt.Errorf("cannot create directory at %s: path exists as a file", path)
		}

		if err := DefaultFS.MkdirAll(path, ReadWriteExecuteUserReadExecuteOthers); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
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
