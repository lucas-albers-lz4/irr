package fileutil

import (
	"os"
	"path/filepath"
)

// FileExists checks if a file exists at the given path
func FileExists(path string) (bool, error) {
	_, err := DefaultFS.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// DirExists checks if a directory exists at the given path
func DirExists(path string) (bool, error) {
	info, err := DefaultFS.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
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
		return DefaultFS.MkdirAll(path, 0755)
	}
	return nil
}

// ReadFileString reads a file and returns its contents as a string
func ReadFileString(path string) (string, error) {
	data, err := DefaultFS.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFileString writes a string to a file
func WriteFileString(path, content string) error {
	return DefaultFS.WriteFile(path, []byte(content), ReadWriteUserReadOthers)
}

// JoinPath joins path components using the OS-specific separator
func JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}

// GetAbsPath returns the absolute path of a file
func GetAbsPath(path string) (string, error) {
	return filepath.Abs(path)
}
