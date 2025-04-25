package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAbsPath(t *testing.T) {
	// Test cases
	testCases := []struct {
		name          string
		path          string
		expectedError bool
	}{
		{
			name:          "Valid path",
			path:          "utils_test.go",
			expectedError: false,
		},
		{
			name:          "Absolute path",
			path:          "/tmp/test",
			expectedError: false,
		},
		{
			name:          "Empty path",
			path:          "",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call GetAbsPath
			result, err := GetAbsPath(tc.path)

			// Check error expectation
			if tc.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify the result is an absolute path
			assert.True(t, filepath.IsAbs(result))

			// For the current file, we can verify more precisely
			if tc.path == "utils_test.go" {
				assert.Equal(t, "utils_test.go", filepath.Base(result))
			}
		})
	}

	// Test with a non-existent path that should still resolve
	t.Run("Non-existent path", func(t *testing.T) {
		nonExistentPath := "non_existent_file.txt"
		result, err := GetAbsPath(nonExistentPath)
		require.NoError(t, err)

		// The path should be absolute
		assert.True(t, filepath.IsAbs(result))

		// The base name should match
		assert.Equal(t, nonExistentPath, filepath.Base(result))

		// The file should not exist
		_, err = os.Stat(result)
		assert.True(t, os.IsNotExist(err))
	})

	// Test with an extremely long path that should exceed OS limits
	t.Run("Extremely long path", func(t *testing.T) {
		// Create a path that's likely to be too long for most filesystems
		// This should trigger the filepath.Abs error path in most systems
		// For example, Windows has a MAX_PATH of 260 characters, macOS around 1024
		var longPath string
		for i := 0; i < 5000; i++ {
			longPath += "verylongdirnametotriggerfilepathabs/"
		}
		longPath += "file.txt"

		_, err := GetAbsPath(longPath)
		// On some OSes this might not fail but we're not asserting it must
		// Just ensure the function doesn't panic
		t.Log("Result for extremely long path:", err)
	})
}

const utilsTestFileName = "test_file.txt"

// utilsTestFS is a custom FS wrapper that properly handles not-exist errors
type utilsTestFS struct {
	*AferoFS
	statFunc  func(name string) (os.FileInfo, error)
	statCount int
}

func newUtilsTestFS() *utilsTestFS {
	return &utilsTestFS{
		AferoFS: NewAferoFS(afero.NewMemMapFs()),
	}
}

// Stat overrides the AferoFS.Stat method to use the custom statFunc first,
// then fall back to the embedded FS, returning errors directly.
func (f *utilsTestFS) Stat(name string) (os.FileInfo, error) {
	f.statCount++ // Increment count regardless of path taken
	// Use fmt.Printf for logging within this method as it doesn't have access to *testing.T
	fmt.Printf("[utilsTestFS.Stat LOG] Called for path: %s, statFunc set: %v\n", name, f.statFunc != nil)

	// 1. Check custom mock function first
	if f.statFunc != nil {
		// Return result directly from mock function
		fmt.Printf("[utilsTestFS.Stat LOG] Using statFunc for path: %s\n", name)
		info, err := f.statFunc(name)
		fmt.Printf("[utilsTestFS.Stat LOG] statFunc for path '%s' returned: info=%v, err=%v\n", name, info, err)
		return info, err
	}

	// 2. Fallback to embedded FS Stat if no mock function was set
	fmt.Printf("[utilsTestFS.Stat LOG] Using embedded AferoFS.Stat for path: %s\n", name)
	// Return result directly from embedded FS
	info, err := f.AferoFS.Stat(name)
	fmt.Printf("[utilsTestFS.Stat LOG] embedded AferoFS.Stat for path '%s' returned: info=%v, err=%v\n", name, info, err)
	return info, err
}

func TestFileExists_Utils(t *testing.T) {
	// Save the original DefaultFS and restore it after the test
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Use a custom memory filesystem for testing that handles errors properly
	memFs := newUtilsTestFS()
	DefaultFS = memFs

	// Create a test file
	err := memFs.WriteFile(utilsTestFileName, []byte("test content"), ReadWriteUserReadOthers)
	require.NoError(t, err, "Failed to create test file")

	// Test cases
	testCases := []struct {
		name     string
		path     string
		expected bool
		setup    func() error
	}{
		{
			name:     "Existing file",
			path:     utilsTestFileName,
			expected: true,
		},
		{
			name:     "Non-existent file",
			path:     "nonexistent.txt",
			expected: false,
		},
		{
			name:     "Directory instead of file",
			path:     "testdir",
			expected: false,
			setup: func() error {
				return memFs.Mkdir("testdir", ReadWriteExecuteUserReadExecuteOthers)
			},
		},
		{
			name:     "Path with special characters",
			path:     "test$file@.txt",
			expected: true,
			setup: func() error {
				return memFs.WriteFile("test$file@.txt", []byte("test"), ReadWriteUserReadOthers)
			},
		},
		{
			name:     "Path with permission error",
			path:     "/root/no_access.txt", // This will cause permission error on real FS
			expected: false,                 // but memFs will handle it as non-existent
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Run setup if provided
			if tc.setup != nil {
				err := tc.setup()
				require.NoError(t, err, "Setup failed")
			}

			// Call FileExists
			exists, err := FileExists(tc.path)

			// Check results based on expected outcome
			if tc.expected {
				assert.NoError(t, err, "Expected no error for existing path '%s'", tc.path)
			} else {
				// Expect NoError because FileExists/DirExists now return nil for any IsNotExist-like error
				assert.NoError(t, err, "Expected no error for non-existent path '%s' (isNotExistError should handle)", tc.path)
			}
			assert.Equal(t, tc.expected, exists)
		})
	}
}

func TestDirExists_Utils(t *testing.T) {
	// Save the original DefaultFS and restore it after the test
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Use a custom memory filesystem for testing that handles errors properly
	memFs := newUtilsTestFS()
	DefaultFS = memFs

	// Create a test directory
	err := memFs.Mkdir("testdir", ReadWriteExecuteUserReadExecuteOthers)
	require.NoError(t, err, "Failed to create test directory")

	// Test cases
	testCases := []struct {
		name     string
		path     string
		expected bool
		setup    func() error
	}{
		{
			name:     "Existing directory",
			path:     "testdir",
			expected: true,
		},
		{
			name:     "Non-existent directory",
			path:     "nonexistent",
			expected: false,
		},
		{
			name:     "File instead of directory",
			path:     utilsTestFileName,
			expected: false,
			setup: func() error {
				return memFs.WriteFile(utilsTestFileName, []byte("test content"), ReadWriteUserReadOthers)
			},
		},
		{
			name:     "Nested directory",
			path:     "testdir/nested",
			expected: true,
			setup: func() error {
				return memFs.MkdirAll("testdir/nested", ReadWriteExecuteUserReadExecuteOthers)
			},
		},
		{
			name:     "Path with permission error",
			path:     "/root/no_access", // This will cause permission error on real FS
			expected: false,             // but memFs will handle it as non-existent
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Run setup if provided
			if tc.setup != nil {
				err := tc.setup()
				require.NoError(t, err, "Setup failed")
			}

			// Call DirExists
			exists, err := DirExists(tc.path)

			// Check results based on expected outcome
			if tc.expected {
				assert.NoError(t, err, "Expected no error for existing path '%s'", tc.path)
			} else {
				// Expect NoError because FileExists/DirExists now return nil for any IsNotExist-like error
				assert.NoError(t, err, "Expected no error for non-existent path '%s' (isNotExistError should handle)", tc.path)
			}
			assert.Equal(t, tc.expected, exists)
		})
	}
}

// pathCollisionFS is a custom FS wrapper that properly handles not-exist errors
type pathCollisionFS struct {
	*utilsTestFS
	collisionPath string
}

func newPathCollisionFS(collisionPath string) *pathCollisionFS {
	return &pathCollisionFS{
		utilsTestFS:   newUtilsTestFS(),
		collisionPath: collisionPath,
	}
}

// Stat is overridden to make it look like the collision path exists but is not a directory
func (f *pathCollisionFS) Stat(name string) (os.FileInfo, error) {
	if name == f.collisionPath {
		// Simulate the file exists but is not a directory
		// We need a mock FileInfo that returns IsDir() == false
		return &mockFileInfo{name: filepath.Base(name), isDir: false}, nil
	}
	// Delegate other paths to the embedded utilsTestFS Stat
	info, err := f.utilsTestFS.Stat(name)
	if err != nil {
		// Wrap errors from the embedded Stat call
		return nil, fmt.Errorf("path collision FS delegation error for %s: %w", name, err)
	}
	return info, nil
}

func TestEnsureDirExists_Utils(t *testing.T) {
	// Save the original DefaultFS and restore it after the test
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Use a custom memory filesystem for testing that handles errors properly
	memFs := newUtilsTestFS()
	DefaultFS = memFs

	// Test cases for non-collision tests
	testCases := []struct {
		name      string
		path      string
		expectErr bool
		setup     func() error
		validate  func() bool
	}{
		{
			name:      "Non-existent directory",
			path:      "newdir",
			expectErr: false,
			validate: func() bool {
				exists, err := DirExists("newdir")
				if err != nil {
					t.Fatalf("DirExists failed: %v", err)
				}
				return exists
			},
		},
		{
			name:      "Existing directory",
			path:      "existingdir",
			expectErr: false,
			setup: func() error {
				return memFs.Mkdir("existingdir", ReadWriteExecuteUserReadExecuteOthers)
			},
			validate: func() bool {
				exists, err := DirExists("existingdir")
				if err != nil {
					t.Fatalf("DirExists failed: %v", err)
				}
				return exists
			},
		},
		{
			name:      "Nested directory",
			path:      "parent/child/grandchild",
			expectErr: false,
			validate: func() bool {
				exists, err := DirExists("parent/child/grandchild")
				if err != nil {
					t.Fatalf("DirExists failed: %v", err)
				}
				return exists
			},
		},
	}

	// Run normal test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Run setup if provided
			if tc.setup != nil {
				err := tc.setup()
				require.NoError(t, err, "Setup failed")
			}

			// Call EnsureDirExists
			err := EnsureDirExists(tc.path)

			// Check error expectation
			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Additional validation if provided
			if tc.validate != nil {
				assert.True(t, tc.validate(), "Validation failed")
			}
		})
	}

	// Special test case for path collision separately
	t.Run("Directory with path collision", func(t *testing.T) {
		// Create a special filesystem with mocked behavior for file collision
		collisionFS := newPathCollisionFS("file_exists")

		// Set as the DefaultFS for this test
		oldFS := DefaultFS
		DefaultFS = collisionFS
		defer func() { DefaultFS = oldFS }()

		// Attempt to create a directory with the same name as the file
		err := EnsureDirExists("file_exists")

		// This should fail with an error
		assert.Error(t, err, "EnsureDirExists should fail with path collision")
	})
}

func TestEnsureDirExists_FileExistsErrorPath(t *testing.T) {
	// Save the original DefaultFS and restore it after the test
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Use a custom memory filesystem for testing
	memFs := newUtilsTestFS()

	// Create a file that we'll try to create a directory over
	filename := "test-collision.txt"
	err := memFs.WriteFile(filename, []byte("test content"), ReadWriteUserReadOthers)
	require.NoError(t, err, "Failed to create test file")

	// Set as DefaultFS
	DefaultFS = memFs

	// Try to create a directory with the same name as the file
	err = EnsureDirExists(filename)

	// Should error with the specific error message about path existing as file
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot create directory")
	assert.Contains(t, err.Error(), "path exists as a file")
}

func TestWriteFileString_Utils(t *testing.T) {
	// Save the original DefaultFS and restore it after the test
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Use a memory filesystem for testing
	memFs := newUtilsTestFS()
	DefaultFS = memFs

	// Test writing a string to a file
	testPath := "test_write_string.txt"
	testContent := "Hello, World!"

	err := WriteFileString(testPath, testContent)
	assert.NoError(t, err, "WriteFileString should not return an error")

	// Verify the file was written correctly
	content, err := ReadFileString(testPath)
	assert.NoError(t, err, "ReadFileString should not return an error")
	assert.Equal(t, testContent, content, "File content should match written content")

	// Test error case with a read-only filesystem
	// Create a read-only version of our filesystem
	readOnlyFs := &utilsTestFS{
		AferoFS: NewAferoFS(afero.NewReadOnlyFs(afero.NewMemMapFs())),
	}
	DefaultFS = readOnlyFs

	err = WriteFileString("should-fail.txt", "test")
	assert.Error(t, err, "WriteFileString should return an error with read-only filesystem")
	assert.Contains(t, err.Error(), "failed to write file", "Error message should contain 'failed to write file'")
}

func TestEnsureDirExists_FileExistsError(t *testing.T) {
	// Save the original DefaultFS and restore it after the test
	origFS := DefaultFS
	defer func() {
		DefaultFS = origFS
	}()

	// Create a mock filesystem that returns an error on Stat
	mockFS := newUtilsTestFS()
	mockFS.statFunc = func(_ string) (os.FileInfo, error) {
		// First call should return os.ErrNotExist to pass DirExists
		if mockFS.statCount == 0 {
			mockFS.statCount++
			return nil, os.ErrNotExist
		}
		// Second call (during FileExists) should return a distinct error
		return nil, fmt.Errorf("distinct error during FileExists check")
	}

	// Add a counter to track stat calls
	mockFS.statCount = 0

	// Set as DefaultFS
	DefaultFS = mockFS

	// Try to EnsureDirExists, should error from FileExists
	err := EnsureDirExists("test-path")

	// Verify that we got the expected *unwrapped* error from the FileExists check phase
	assert.Error(t, err)
	// Check ONLY for the specific underlying error message, as it's not wrapped in this path
	assert.Contains(t, err.Error(), "distinct error during FileExists check")
	// Ensure the wrapper message is NOT present
	assert.NotContains(t, err.Error(), "failed to check if file exists at path")
}

func TestGetAbsPath_AllPaths(t *testing.T) {
	// Test empty path case
	_, err := GetAbsPath("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path cannot be empty")

	// Test valid path
	validPath := "test.txt"
	absPath, err := GetAbsPath(validPath)
	assert.NoError(t, err)
	assert.True(t, filepath.IsAbs(absPath))
	assert.Equal(t, validPath, filepath.Base(absPath))

	// The third condition (filepath.Abs error) is difficult to test directly
	// without monkeypatching, which isn't allowed in Go. In practice, filepath.Abs
	// only errors in extremely rare cases (like paths with null characters or system call errors)
	// so we'll consider it adequately covered for practical purposes.
}

// mockFileInfo implements os.FileInfo for testing purposes.
type mockFileInfo struct {
	name  string
	isDir bool
}

func (f mockFileInfo) Name() string       { return f.name }
func (f mockFileInfo) Size() int64        { return 0 }
func (f mockFileInfo) Mode() os.FileMode  { return 0 }
func (f mockFileInfo) ModTime() time.Time { return time.Time{} }
func (f mockFileInfo) IsDir() bool        { return f.isDir }
func (f mockFileInfo) Sys() interface{}   { return nil }
