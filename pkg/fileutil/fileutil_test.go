package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

// Special mock filesystem handler that properly deals with file not found errors in tests
type testFS struct {
	*AferoFS
	statFunc     func(name string) (os.FileInfo, error)
	mkdirAllFunc func(path string, perm os.FileMode) error
}

func newTestFS() *testFS {
	fsBase := NewAferoFS(afero.NewMemMapFs())
	fs := &testFS{
		AferoFS: fsBase,
	}

	// Set up default implementations that can be overridden
	fs.statFunc = func(name string) (os.FileInfo, error) {
		info, err := fsBase.fs.Stat(name)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, os.ErrNotExist // Use standard os.ErrNotExist directly
			}
			// Wrap other errors
			return nil, fmt.Errorf("testFS.statFunc: %w", err)
		}
		return info, nil
	}

	// Keep original MkdirAll implementation
	originalMkdirAll := fsBase.MkdirAll
	fs.mkdirAllFunc = originalMkdirAll

	return fs
}

// Stat overrides the AferoFS.Stat method to use our custom statFunc
func (t *testFS) Stat(name string) (os.FileInfo, error) {
	if t.statFunc != nil {
		// Return error from statFunc directly without wrapping
		return t.statFunc(name)
	}
	// Fallback to default behavior, return error directly
	return t.fs.Stat(name)
}

// MkdirAll overrides the AferoFS.MkdirAll method to use our custom mkdirAllFunc
func (t *testFS) MkdirAll(path string, perm os.FileMode) error {
	if t.mkdirAllFunc != nil {
		return t.mkdirAllFunc(path, perm)
	}
	// Fallback to default behavior if mkdirAllFunc hasn't been set
	return t.AferoFS.MkdirAll(path, perm)
}

// MockStat sets up a mock for the Stat method and returns a cleanup function
func (t *testFS) MockStat(mockFunc func(string) (os.FileInfo, error)) func() {
	original := t.statFunc
	t.statFunc = mockFunc
	return func() { t.statFunc = original }
}

// MockMkdirAll sets up a mock for the MkdirAll method and returns a cleanup function
func (t *testFS) MockMkdirAll(mockFunc func(string, os.FileMode) error) func() {
	original := t.mkdirAllFunc
	t.mkdirAllFunc = mockFunc
	return func() { t.mkdirAllFunc = original }
}

// testFileInfo implements os.FileInfo for testing (renamed from fakeFileInfo to avoid conflicts)
type testFileInfo struct {
	name string
	dir  bool
}

func (f *testFileInfo) Name() string       { return f.name }
func (f *testFileInfo) Size() int64        { return 0 }
func (f *testFileInfo) Mode() os.FileMode  { return 0o644 }
func (f *testFileInfo) ModTime() time.Time { return time.Now() }
func (f *testFileInfo) IsDir() bool        { return f.dir }
func (f *testFileInfo) Sys() interface{}   { return nil }

const (
	testFileName = "test.txt"
	testDirName  = "testdir"
)

// Helper for testing not found error variants for FileExists/DirExists
func testNotFoundErrorVariants(t *testing.T, existsFunc func(string) (bool, error), notExist1, notExist2, label string) {
	specialMockFS := newTestFS()

	specialMockFS.MockStat(func(name string) (os.FileInfo, error) {
		if name == notExist1 {
			return nil, fmt.Errorf("file does not exist")
		}
		if name == notExist2 {
			return nil, fmt.Errorf("no such file or directory")
		}
		return specialMockFS.fs.Stat(name)
	})

	specialCleanup := SetFS(specialMockFS)
	defer specialCleanup()

	exists, err := existsFunc(notExist1)
	if err != nil {
		if !strings.Contains(err.Error(), "file does not exist") {
			t.Errorf("%s() should handle 'file does not exist' error, but got unexpected error: %v", label, err)
		} else {
			t.Logf("Got non-nil error for 'file does not exist', but it contained the expected string: %v", err)
		}
	}
	if exists {
		t.Errorf("%s() with 'file does not exist' error should return false", label)
	}

	exists, err = existsFunc(notExist2)
	if err != nil {
		if !strings.Contains(err.Error(), "no such file or directory") {
			t.Errorf("%s() should handle 'no such file or directory' error, but got unexpected error: %v", label, err)
		} else {
			t.Logf("Got non-nil error for 'no such file or directory', but it contained the expected string: %v", err)
		}
	}
	if exists {
		t.Errorf("%s() with 'no such file or directory' error should return false", label)
	}
}

func TestFileExists(t *testing.T) {
	// Create test filesystem
	mockFS := newTestFS()

	// Setup test files
	err := mockFS.WriteFile(testFileName, []byte("test content"), ReadWriteUserReadOthers)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	// Setup test directory (for testing the file vs directory path)
	err = mockFS.Mkdir(testDirName, ReadWriteExecuteUserReadExecuteOthers)
	if err != nil {
		t.Fatalf("Failed to set up test directory: %v", err)
	}

	// Replace default filesystem with mock
	cleanup := SetFS(mockFS)
	defer cleanup()

	// Test for existing file
	exists, err := FileExists(testFileName)
	if err != nil {
		t.Errorf("FileExists() error = %v, want nil", err)
	}
	if !exists {
		t.Errorf("FileExists() = %v, want true", exists)
	}

	// Test for non-existent file
	exists, err = FileExists("nonexistent.txt")
	// Allow nil error or IsNotExist error
	if err != nil && !os.IsNotExist(err) {
		t.Errorf("FileExists() for non-existent file returned unexpected error: %v", err)
	}
	if exists {
		t.Errorf("FileExists() = %v, want false", exists)
	}

	// Test for directory (should return false, as it's not a file)
	exists, err = FileExists(testDirName)
	if err != nil {
		t.Errorf("FileExists() error = %v, want nil", err)
	}
	if exists {
		t.Errorf("FileExists() = %v, want false for a directory", exists)
	}

	// Test for handling different "file not found" error messages
	t.Run("File not found error variants", func(t *testing.T) {
		testNotFoundErrorVariants(t, FileExists, "file-not-exist-1", "file-not-exist-2", "FileExists")
	})

	// Test other error cases that should be propagated
	t.Run("Non-NotExist error propagation", func(t *testing.T) {
		// Create special mock filesystem that returns a different error
		errorFS := newTestFS()

		// Setup custom Stat function
		errorFS.MockStat(func(_ string) (os.FileInfo, error) {
			return nil, fmt.Errorf("unexpected error: permission denied")
		})

		errorCleanup := SetFS(errorFS)
		defer errorCleanup()

		// Test error propagation
		_, err := FileExists("any-file")
		if err == nil || !strings.Contains(err.Error(), "unexpected error: permission denied") {
			t.Errorf("FileExists() should propagate unexpected errors, got %v", err)
		}
	})
}

func TestDirExists(t *testing.T) {
	// Create test filesystem
	mockFS := newTestFS()

	// Setup test directories
	err := mockFS.MkdirAll(testDirName, ReadWriteExecuteUserReadExecuteOthers)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	// Replace default filesystem with mock
	cleanup := SetFS(mockFS)
	defer cleanup()

	// Test for existing directory
	exists, err := DirExists(testDirName)
	if err != nil {
		t.Errorf("DirExists() error = %v, want nil", err)
	}
	if !exists {
		t.Errorf("DirExists() = %v, want true", exists)
	}

	// Test for non-existent directory
	exists, err = DirExists("nonexistentdir")
	// Allow nil error or IsNotExist error
	if err != nil && !os.IsNotExist(err) {
		t.Errorf("DirExists() for non-existent dir returned unexpected error: %v", err)
	}
	if exists {
		t.Errorf("DirExists() = %v, want false", exists)
	}

	// Test for file (not a directory)
	err = mockFS.WriteFile(testFileName, []byte("test content"), ReadWriteUserReadOthers)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	exists, err = DirExists(testFileName)
	if err != nil {
		t.Errorf("DirExists() error = %v, want nil", err)
	}
	if exists {
		t.Errorf("DirExists() = %v, want false for a file", exists)
	}

	// Test for handling different "file not found" error messages
	t.Run("Directory not found error variants", func(t *testing.T) {
		testNotFoundErrorVariants(t, DirExists, "dir-not-exist-1", "dir-not-exist-2", "DirExists")
	})

	// Test other error cases that should be propagated
	t.Run("Non-NotExist error propagation", func(t *testing.T) {
		// Create special mock filesystem that returns a different error
		errorFS := newTestFS()

		// Setup custom Stat function
		errorFS.MockStat(func(_ string) (os.FileInfo, error) {
			return nil, fmt.Errorf("unexpected error: permission denied")
		})

		errorCleanup := SetFS(errorFS)
		defer errorCleanup()

		// Test error propagation
		_, err := DirExists("any-dir")
		if err == nil || !strings.Contains(err.Error(), "unexpected error: permission denied") {
			t.Errorf("DirExists() should propagate unexpected errors, got %v", err)
		}
	})
}

func TestEnsureDirExists(t *testing.T) {
	// Create test filesystem
	mockFS := newTestFS()

	// Replace default filesystem with mock
	cleanup := SetFS(mockFS)
	defer cleanup()

	// Test creating a new directory
	testDir := "newdir"
	err := EnsureDirExists(testDir)
	assert.NoError(t, err, "EnsureDirExists() error = %v, want nil")

	// Verify directory was created
	exists, err := DirExists(testDir)
	assert.NoError(t, err, "DirExists() error = %v, want nil")
	assert.True(t, exists, "Directory was not created successfully")

	// Test with existing directory (should not error)
	err = EnsureDirExists(testDir)
	assert.NoError(t, err, "EnsureDirExists() with existing dir error = %v, want nil")

	// Test with nested directories
	nestedDir := "parent/child/grandchild"
	err = EnsureDirExists(nestedDir)
	assert.NoError(t, err, "EnsureDirExists() with nested dirs error = %v, want nil")

	// Verify nested directory was created
	exists, err = DirExists(nestedDir)
	assert.NoError(t, err, "DirExists() error = %v, want nil")
	assert.True(t, exists, "Nested directory was not created successfully")

	// Test with path that exists as a file (should error)
	t.Run("File instead of directory", func(t *testing.T) {
		// Setup a new test filesystem for this specific test
		fileCollisionFS := newTestFS()

		// Create a file with the path we'll try to make a directory
		fileInsteadOfDir := "file-instead-of-dir"
		err = fileCollisionFS.WriteFile(fileInsteadOfDir, []byte("test content"), ReadWriteUserReadOthers)
		if err != nil {
			t.Fatalf("Failed to set up test file: %v", err)
		}

		// Custom mock Stat to properly detect that file exists and is not a directory
		fileCollisionFS.MockStat(func(name string) (os.FileInfo, error) {
			if name == fileInsteadOfDir {
				// Return a fake file info showing this is a file, not a directory
				return &testFileInfo{name: filepath.Base(name), dir: false}, nil
			}
			// For other paths, use the default implementation
			return fileCollisionFS.AferoFS.Stat(name)
		})

		// Switch to the collision test filesystem and ensure we restore the original after
		collisionCleanup := SetFS(fileCollisionFS)
		defer collisionCleanup()

		// Now try to create a directory with the same name as the file - should error
		err := EnsureDirExists(fileInsteadOfDir)
		assert.Error(t, err, "EnsureDirExists() with path that exists as file should return error")
	})

	// Test DirExists error path by injecting error
	t.Run("DirExists error", func(t *testing.T) {
		// Create special mock filesystem for error cases
		specialMockFS := newTestFS()

		// Override Stat to return a specific error
		specialMockFS.MockStat(func(name string) (os.FileInfo, error) {
			if name == "special-error-dir" {
				return nil, fmt.Errorf("custom error that's not a NotExist error")
			}
			return nil, os.ErrNotExist
		})

		specialCleanup := SetFS(specialMockFS)
		defer specialCleanup()

		// Test error propagation
		err := EnsureDirExists("special-error-dir")
		if err == nil || !strings.Contains(err.Error(), "custom error that's not a NotExist error") {
			t.Errorf("EnsureDirExists() should propagate DirExists errors, got: %v", err)
		}
	})

	// Test Mkdir error path
	t.Run("Mkdir error", func(t *testing.T) {
		// Create a mock filesystem that fails on Mkdir
		failMkdirFS := newTestFS()

		// Setup MkdirAll to fail
		failMkdirFS.MockMkdirAll(func(path string, _ os.FileMode) error {
			return fmt.Errorf("failed to create directory: %s", path)
		})

		failCleanup := SetFS(failMkdirFS)
		defer failCleanup()

		// Test error propagation
		err := EnsureDirExists("should-fail")
		if err == nil {
			t.Errorf("EnsureDirExists() should propagate MkdirAll errors")
		} else if !strings.Contains(err.Error(), "failed to create directory: should-fail") {
			t.Errorf("EnsureDirExists() error didn't contain expected text, got: %v", err)
		}
	})
}

func TestReadFileString(t *testing.T) {
	// Create test filesystem
	mockFS := newTestFS()

	// Setup test files
	testContent := "Hello, World!"
	err := mockFS.WriteFile(testFileName, []byte(testContent), ReadWriteUserReadOthers)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	// Replace default filesystem with mock
	cleanup := SetFS(mockFS)
	defer cleanup()

	// Test reading existing file
	content, err := ReadFileString(testFileName)
	if err != nil {
		t.Errorf("ReadFileString() error = %v, want nil", err)
	}
	if content != testContent {
		t.Errorf("ReadFileString() = %q, want %q", content, testContent)
	}

	// Test reading non-existent file
	_, err = ReadFileString("nonexistent.txt")
	if err == nil {
		t.Errorf("ReadFileString() error = nil, want error")
	}
}

func TestWriteFileString(t *testing.T) {
	// Create test filesystem
	mockFS := newTestFS()

	// Replace default filesystem with mock
	cleanup := SetFS(mockFS)
	defer cleanup()

	// Test writing to a file
	testFile := "output.txt"
	testContent := "Test content for writing"

	err := WriteFileString(testFile, testContent)
	if err != nil {
		t.Errorf("WriteFileString() error = %v, want nil", err)
	}

	// Verify file was written correctly
	content, err := mockFS.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read written file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("Written content = %q, want %q", string(content), testContent)
	}

	// Check file permissions
	info, err := mockFS.Stat(testFile)
	if err != nil {
		t.Errorf("Failed to stat written file: %v", err)
	}
	if info == nil {
		t.Errorf("File info is nil")
	} else if info.Mode().Perm() != ReadWriteUserReadOthers {
		t.Errorf("File permissions = %v, want %v", info.Mode().Perm(), ReadWriteUserReadOthers)
	}
}

func TestJoinPath(t *testing.T) {
	testCases := []struct {
		name     string
		elements []string
		expected string
	}{
		{
			name:     "single element",
			elements: []string{"file.txt"},
			expected: "file.txt",
		},
		{
			name:     "two elements",
			elements: []string{"dir", "file.txt"},
			expected: filepath.Join("dir", "file.txt"),
		},
		{
			name:     "multiple elements",
			elements: []string{"root", "parent", "child", "file.txt"},
			expected: filepath.Join("root", "parent", "child", "file.txt"),
		},
		{
			name:     "with empty elements",
			elements: []string{"dir", "", "file.txt"},
			expected: filepath.Join("dir", "file.txt"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := JoinPath(tc.elements...)
			if result != tc.expected {
				t.Errorf("JoinPath() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestFileOperations(t *testing.T) {
	// Create a temporary directory for the test
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, testFileName)

	// Create test filesystem
	mockFS := newTestFS()

	// Setup test files
	err := mockFS.WriteFile(testFile, []byte("test content"), ReadWriteUserReadOthers)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	// Replace default filesystem with mock
	cleanup := SetFS(mockFS)
	defer cleanup()

	// Test for existing file
	exists, err := FileExists(testFile)
	if err != nil {
		t.Errorf("FileExists() error = %v, want nil", err)
	}
	if !exists {
		t.Errorf("FileExists() = %v, want true", exists)
	}

	// Create a temporary directory for the test
	tempDir = t.TempDir()
	testDir := filepath.Join(tempDir, testDirName)

	// Setup test directories
	err = mockFS.MkdirAll(testDir, ReadWriteExecuteUserReadExecuteOthers)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	// Test for existing directory
	exists, err = DirExists(testDir)
	if err != nil {
		t.Errorf("DirExists() error = %v, want nil", err)
	}
	if !exists {
		t.Errorf("DirExists() = %v, want true", exists)
	}
}
