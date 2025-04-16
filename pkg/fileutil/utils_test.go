package fileutil

import (
	"path/filepath"
	"testing"
)

func TestGetAbsPath(t *testing.T) {
	// Test with an already absolute path
	absolutePath := filepath.Join("absolute", "path")
	if !filepath.IsAbs(absolutePath) {
		absolutePath = filepath.Join(string(filepath.Separator), absolutePath)
	}

	absPath, err := GetAbsPath(absolutePath)
	if err != nil {
		t.Errorf("GetAbsPath() error = %v, want nil", err)
	}
	if absPath != absolutePath {
		t.Errorf("GetAbsPath() = %v, want %v", absPath, absolutePath)
	}

	// Test with a relative path
	// Since the working directory can change depending on where the test is run,
	// we can only check that the result is an absolute path
	relativePath := filepath.Join("relative", "path")

	absPath, err = GetAbsPath(relativePath)
	if err != nil {
		t.Errorf("GetAbsPath() error = %v, want nil", err)
	}
	if !filepath.IsAbs(absPath) {
		t.Errorf("GetAbsPath() = %v, which is not an absolute path", absPath)
	}
}
