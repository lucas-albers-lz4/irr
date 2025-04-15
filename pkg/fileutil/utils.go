package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileExists checks if a file exists at the given path
func FileExists(path string) (bool, error) {
	info, err := DefaultFS.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		// Don't wrap "file not found" errors
		if err.Error() == os.ErrNotExist.Error() ||
			err.Error() == "file does not exist" ||
			err.Error() == "no such file or directory" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if file exists: %w", err)
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
		// Don't wrap "file not found" errors
		if err.Error() == os.ErrNotExist.Error() ||
			err.Error() == "file does not exist" ||
			err.Error() == "no such file or directory" {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat directory: %w", err)
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
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}
	return abs, nil
}
