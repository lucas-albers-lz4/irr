package fileutil

import (
	"os"
	"testing"

	"github.com/spf13/afero"
)

func TestFileExists(t *testing.T) {
	// Create mock filesystem
	mockFs := afero.NewMemMapFs()
	mockAferoFS := NewAferoFS(mockFs)

	// Setup test files
	testFile := "test.txt"
	err := mockAferoFS.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	// Replace default filesystem with mock
	cleanup := SetFS(mockAferoFS)
	defer cleanup()

	// Test for existing file
	exists, err := FileExists(testFile)
	if err != nil {
		t.Errorf("FileExists() error = %v, want nil", err)
	}
	if !exists {
		t.Errorf("FileExists() = %v, want true", exists)
	}

	// Test for non-existent file
	exists, err = FileExists("nonexistent.txt")
	if err != nil {
		t.Errorf("FileExists() error = %v, want nil", err)
	}
	if exists {
		t.Errorf("FileExists() = %v, want false", exists)
	}
}

func TestDirExists(t *testing.T) {
	// Create mock filesystem
	mockFs := afero.NewMemMapFs()
	mockAferoFS := NewAferoFS(mockFs)

	// Setup test directories
	testDir := "testdir"
	err := mockAferoFS.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	// Replace default filesystem with mock
	cleanup := SetFS(mockAferoFS)
	defer cleanup()

	// Test for existing directory
	exists, err := DirExists(testDir)
	if err != nil {
		t.Errorf("DirExists() error = %v, want nil", err)
	}
	if !exists {
		t.Errorf("DirExists() = %v, want true", exists)
	}

	// Test for non-existent directory
	exists, err = DirExists("nonexistentdir")
	if err != nil {
		t.Errorf("DirExists() error = %v, want nil", err)
	}
	if exists {
		t.Errorf("DirExists() = %v, want false", exists)
	}

	// Test for file (not a directory)
	testFile := "testfile.txt"
	err = mockAferoFS.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	exists, err = DirExists(testFile)
	if err != nil {
		t.Errorf("DirExists() error = %v, want nil", err)
	}
	if exists {
		t.Errorf("DirExists() = %v, want false for a file", exists)
	}
}

func TestEnsureDirExists(t *testing.T) {
	// Create mock filesystem
	mockFs := afero.NewMemMapFs()
	mockAferoFS := NewAferoFS(mockFs)

	// Replace default filesystem with mock
	cleanup := SetFS(mockAferoFS)
	defer cleanup()

	// Test creating a new directory
	testDir := "newdir"
	err := EnsureDirExists(testDir)
	if err != nil {
		t.Errorf("EnsureDirExists() error = %v, want nil", err)
	}

	// Verify directory was created
	exists, err := DirExists(testDir)
	if err != nil {
		t.Errorf("DirExists() error = %v, want nil", err)
	}
	if !exists {
		t.Errorf("Directory was not created successfully")
	}

	// Test with existing directory (should not error)
	err = EnsureDirExists(testDir)
	if err != nil {
		t.Errorf("EnsureDirExists() with existing dir error = %v, want nil", err)
	}

	// Test with nested directories
	nestedDir := "parent/child/grandchild"
	err = EnsureDirExists(nestedDir)
	if err != nil {
		t.Errorf("EnsureDirExists() with nested dirs error = %v, want nil", err)
	}

	// Verify nested directory was created
	exists, err = DirExists(nestedDir)
	if err != nil {
		t.Errorf("DirExists() error = %v, want nil", err)
	}
	if !exists {
		t.Errorf("Nested directory was not created successfully")
	}
}

func TestReadFileString(t *testing.T) {
	// Create mock filesystem
	mockFs := afero.NewMemMapFs()
	mockAferoFS := NewAferoFS(mockFs)

	// Setup test files
	testFile := "test.txt"
	testContent := "Hello, World!"
	err := mockAferoFS.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	// Replace default filesystem with mock
	cleanup := SetFS(mockAferoFS)
	defer cleanup()

	// Test reading existing file
	content, err := ReadFileString(testFile)
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
	// Create mock filesystem
	mockFs := afero.NewMemMapFs()
	mockAferoFS := NewAferoFS(mockFs)

	// Replace default filesystem with mock
	cleanup := SetFS(mockAferoFS)
	defer cleanup()

	// Test writing to a file
	testFile := "output.txt"
	testContent := "Test content for writing"

	err := WriteFileString(testFile, testContent)
	if err != nil {
		t.Errorf("WriteFileString() error = %v, want nil", err)
	}

	// Verify file was written correctly
	content, err := mockAferoFS.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read written file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("Written content = %q, want %q", string(content), testContent)
	}

	// Check file permissions
	info, err := mockAferoFS.Stat(testFile)
	if err != nil {
		t.Errorf("Failed to stat written file: %v", err)
	}
	if info.Mode().Perm() != ReadWriteUserReadOthers {
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
			expected: "dir" + string(os.PathSeparator) + "file.txt",
		},
		{
			name:     "multiple elements",
			elements: []string{"root", "parent", "child", "file.txt"},
			expected: "root" + string(os.PathSeparator) + "parent" + string(os.PathSeparator) + "child" + string(os.PathSeparator) + "file.txt",
		},
		{
			name:     "with empty elements",
			elements: []string{"dir", "", "file.txt"},
			expected: "dir" + string(os.PathSeparator) + "file.txt",
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
