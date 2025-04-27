package fileutil

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFileName = "test_fs_file.txt"
	testDirName  = "test_fs_dir"
)

func TestNewAferoFS(t *testing.T) {
	// Test with nil fs
	fs := NewAferoFS(nil)
	assert.NotNil(t, fs, "NewAferoFS should not return nil when given nil fs")

	// Test with non-nil fs
	memFs := afero.NewMemMapFs()
	fs = NewAferoFS(memFs)
	assert.NotNil(t, fs, "NewAferoFS should not return nil when given a valid fs")
}

func TestAferoFS_Create(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Test creating a file
	file, err := fs.Create(testFileName)
	assert.NoError(t, err, "Create should not return an error")
	assert.NotNil(t, file, "Created file should not be nil")

	// Verify the file was created
	exists, err := afero.Exists(memFs, testFileName)
	assert.NoError(t, err, "Exists should not return an error")
	assert.True(t, exists, "File should exist after Create")

	// Test writing to the created file
	testData := []byte("test data")
	n, err := file.Write(testData)
	assert.NoError(t, err, "Write should not return an error")
	assert.Equal(t, len(testData), n, "Write should return the number of bytes written")

	// Close the file
	err = file.Close()
	assert.NoError(t, err, "Close should not return an error")

	// Verify the file contents
	data, err := afero.ReadFile(memFs, testFileName)
	assert.NoError(t, err, "ReadFile should not return an error")
	assert.Equal(t, testData, data, "File contents should match written data")

	// Test error case with a read-only filesystem
	readOnlyFs := afero.NewReadOnlyFs(afero.NewMemMapFs())
	readOnlyAferoFs := NewAferoFS(readOnlyFs)
	_, err = readOnlyAferoFs.Create("should-fail.txt")
	assert.Error(t, err, "Create should return an error with read-only filesystem")
	assert.Contains(t, err.Error(), "failed to create file", "Error message should contain 'failed to create file'")
}

func TestAferoFS_Mkdir(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Test creating a directory
	testDir := "testdir"
	err := fs.Mkdir(testDir, ReadWriteExecuteUserReadExecuteOthers)
	assert.NoError(t, err, "Mkdir should not return an error")

	// Verify the directory was created
	exists, err := afero.DirExists(memFs, testDir)
	assert.NoError(t, err, "DirExists should not return an error")
	assert.True(t, exists, "Directory should exist after Mkdir")

	// Test creating a nested directory without parent - behavior depends on OS
	// On some OS (like Windows), this might fail, but on others it might create parent dirs
	// So we'll use MkdirAll for a more reliable test
	fs2 := NewAferoFS(afero.NewOsFs()) // Use OS filesystem for this test
	uniqueNestedDir := filepath.Join(os.TempDir(), "irr_mkdir_test", "nonexistentparent", "child")

	// First clean up if the directory exists from a previous test run
	if err := fs2.RemoveAll(filepath.Join(os.TempDir(), "irr_mkdir_test")); err != nil {
		t.Logf("Warning: Failed to clean up test directory: %v", err)
	}

	// Now try to create the nested dir with Mkdir - this should fail since the parent doesn't exist
	err = fs2.Mkdir(uniqueNestedDir, ReadWriteExecuteUserReadExecuteOthers)
	if err == nil {
		// Check if the parent was actually created (some filesystems might do this)
		parentExists, err := DirExists(filepath.Dir(uniqueNestedDir))
		if err != nil {
			t.Fatalf("Failed to check parent directory: %v", err)
		}
		if !parentExists {
			t.Error("Mkdir should return an error for nested directory without parent")
		}
	}

	// Clean up
	if err := fs2.RemoveAll(filepath.Join(os.TempDir(), "irr_mkdir_test")); err != nil {
		t.Logf("Warning: Failed to clean up test directory: %v", err)
	}
}

func TestAferoFS_MkdirAll(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Test creating nested directories
	nestedDir := "parent/child/grandchild"
	err := fs.MkdirAll(nestedDir, ReadWriteExecuteUserReadExecuteOthers)
	assert.NoError(t, err, "MkdirAll should not return an error")

	// Verify the directory was created
	exists, err := afero.DirExists(memFs, nestedDir)
	assert.NoError(t, err, "DirExists should not return an error")
	assert.True(t, exists, "Directory should exist after MkdirAll")

	// Test creating a directory that already exists
	err = fs.MkdirAll(nestedDir, ReadWriteExecuteUserReadExecuteOthers)
	assert.NoError(t, err, "MkdirAll should not return an error for existing directory")

	// Test error case with a read-only filesystem
	readOnlyFs := afero.NewReadOnlyFs(afero.NewMemMapFs())
	readOnlyAferoFs := NewAferoFS(readOnlyFs)
	err = readOnlyAferoFs.MkdirAll("should-fail", ReadWriteExecuteUserReadExecuteOthers)
	assert.Error(t, err, "MkdirAll should return an error with read-only filesystem")
	assert.Contains(t, err.Error(), "failed to create directory path", "Error message should contain 'failed to create directory path'")
}

func TestAferoFS_Open(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Create a test file
	testData := []byte("test data")
	err := afero.WriteFile(memFs, testFileName, testData, ReadWriteUserReadOthers)
	require.NoError(t, err, "Failed to set up test file")

	// Test opening the file
	file, err := fs.Open(testFileName)
	assert.NoError(t, err, "Open should not return an error")
	assert.NotNil(t, file, "Opened file should not be nil")

	// Read from the file
	data, err := io.ReadAll(file)
	assert.NoError(t, err, "ReadAll should not return an error")
	assert.Equal(t, testData, data, "Read data should match original data")

	// Close the file
	err = file.Close()
	assert.NoError(t, err, "Close should not return an error")

	// Test opening a non-existent file
	_, err = fs.Open("nonexistent.txt")
	assert.Error(t, err, "Open should return an error for non-existent file")
}

func TestAferoFS_OpenFile(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Test creating a new file with OpenFile
	file, err := fs.OpenFile(testFileName, os.O_CREATE|os.O_WRONLY, ReadWriteUserReadOthers)
	assert.NoError(t, err, "OpenFile should not return an error")
	assert.NotNil(t, file, "Opened file should not be nil")

	// Write to the file
	testData := []byte("test data")
	n, err := file.Write(testData)
	assert.NoError(t, err, "Write should not return an error")
	assert.Equal(t, len(testData), n, "Write should return the number of bytes written")

	// Close the file
	err = file.Close()
	assert.NoError(t, err, "Close should not return an error")

	// Open the file for reading
	file, err = fs.OpenFile(testFileName, os.O_RDONLY, ReadWriteUserReadOthers)
	assert.NoError(t, err, "OpenFile should not return an error")

	// Read from the file
	data, err := io.ReadAll(file)
	assert.NoError(t, err, "ReadAll should not return an error")
	assert.Equal(t, testData, data, "Read data should match written data")

	// Close the file
	err = file.Close()
	assert.NoError(t, err, "Close should not return an error")

	// Test opening a non-existent file without O_CREATE
	_, err = fs.OpenFile("nonexistent.txt", os.O_RDONLY, ReadWriteUserReadOthers)
	assert.Error(t, err, "OpenFile should return an error for non-existent file without O_CREATE")
}

func TestAferoFS_Remove(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Create a test file
	err := afero.WriteFile(memFs, testFileName, []byte("test data"), ReadWriteUserReadOthers)
	require.NoError(t, err, "Failed to set up test file")

	// Test removing the file
	err = fs.Remove(testFileName)
	assert.NoError(t, err, "Remove should not return an error")

	// Verify the file was removed
	exists, err := afero.Exists(memFs, testFileName)
	assert.NoError(t, err, "Exists should not return an error")
	assert.False(t, exists, "File should not exist after Remove")

	// Test removing a non-existent file
	err = fs.Remove("nonexistent.txt")
	assert.Error(t, err, "Remove should return an error for non-existent file")
}

func TestAferoFS_RemoveAll(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Create a directory with files
	testDir := "testdir"
	testFile := "testdir/test.txt"
	err := memFs.MkdirAll(testDir, ReadWriteExecuteUserReadExecuteOthers)
	require.NoError(t, err, "Failed to set up test directory")
	err = afero.WriteFile(memFs, testFile, []byte("test data"), ReadWriteUserReadOthers)
	require.NoError(t, err, "Failed to set up test file")

	// Test removing the directory and its contents
	err = fs.RemoveAll(testDir)
	assert.NoError(t, err, "RemoveAll should not return an error")

	// Verify the directory was removed
	exists, err := afero.DirExists(memFs, testDir)
	assert.NoError(t, err, "DirExists should not return an error")
	assert.False(t, exists, "Directory should not exist after RemoveAll")

	// Test removing a non-existent directory
	err = fs.RemoveAll("nonexistent")
	assert.NoError(t, err, "RemoveAll should not return an error for non-existent path")

	// Test error case with a read-only filesystem
	// First create a directory in the underlying memfs
	memMapFs := afero.NewMemMapFs()
	err = memMapFs.MkdirAll("test-readonly", ReadWriteExecuteUserReadExecuteOthers)
	if err != nil {
		t.Fatalf("Failed to create test-readonly directory: %v", err)
	}
	roFs := afero.NewReadOnlyFs(memMapFs)
	roAferoFs := NewAferoFS(roFs)
	err = roAferoFs.RemoveAll("test-readonly")
	assert.Error(t, err, "RemoveAll should return an error with read-only filesystem")
	assert.Contains(t, err.Error(), "failed to remove path", "Error message should contain 'failed to remove path'")
}

func TestAferoFS_Rename(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Create a test file
	oldName := "old.txt"
	newName := "new.txt"
	testData := []byte("test data")
	err := afero.WriteFile(memFs, oldName, testData, ReadWriteUserReadOthers)
	require.NoError(t, err, "Failed to set up test file")

	// Test renaming the file
	err = fs.Rename(oldName, newName)
	assert.NoError(t, err, "Rename should not return an error")

	// Verify the file was renamed
	exists, err := afero.Exists(memFs, oldName)
	assert.NoError(t, err, "Exists should not return an error")
	assert.False(t, exists, "Old file should not exist after Rename")

	exists, err = afero.Exists(memFs, newName)
	assert.NoError(t, err, "Exists should not return an error")
	assert.True(t, exists, "New file should exist after Rename")

	// Verify the contents were preserved
	data, err := afero.ReadFile(memFs, newName)
	assert.NoError(t, err, "ReadFile should not return an error")
	assert.Equal(t, testData, data, "File contents should be preserved after Rename")

	// Test renaming a non-existent file
	err = fs.Rename("nonexistent.txt", "any.txt")
	assert.Error(t, err, "Rename should return an error for non-existent file")
}

func TestAferoFS_Stat(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Create a test file
	testData := []byte("test data")
	err := afero.WriteFile(memFs, testFileName, testData, ReadWriteUserReadOthers)
	require.NoError(t, err, "Failed to set up test file")

	// Test getting file info
	info, err := fs.Stat(testFileName)
	assert.NoError(t, err, "Stat should not return an error")
	assert.NotNil(t, info, "File info should not be nil")
	assert.Equal(t, testFileName, info.Name(), "File name should match")
	assert.Equal(t, int64(len(testData)), info.Size(), "File size should match data length")
	assert.False(t, info.IsDir(), "File should not be a directory")

	// Test getting info for a non-existent file
	_, err = fs.Stat("nonexistent.txt")
	assert.Error(t, err, "Stat should return an error for non-existent file")
}

func TestAferoFS_ReadFile(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Create a test file
	err := afero.WriteFile(memFs, testFileName, []byte("test data"), ReadWriteUserReadOthers)
	require.NoError(t, err, "Failed to set up test file")

	// Test reading the file
	data, err := fs.ReadFile(testFileName)
	assert.NoError(t, err, "ReadFile should not return an error")
	assert.Equal(t, []byte("test data"), data, "Read data should match original data")

	// Test reading a non-existent file
	_, err = fs.ReadFile("nonexistent.txt")
	assert.Error(t, err, "ReadFile should return an error for non-existent file")
}

func TestAferoFS_WriteFile(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Test writing a file
	testData := []byte("test data")
	err := fs.WriteFile(testFileName, testData, ReadWriteUserReadOthers)
	assert.NoError(t, err, "WriteFile should not return an error")

	// Verify the file was written correctly
	data, err := afero.ReadFile(memFs, testFileName)
	assert.NoError(t, err, "ReadFile should not return an error")
	assert.Equal(t, testData, data, "File contents should match written data")

	// Test overwriting an existing file
	newData := []byte("new data")
	err = fs.WriteFile(testFileName, newData, ReadWriteUserReadOthers)
	assert.NoError(t, err, "WriteFile should not return an error when overwriting")

	// Verify the file was updated
	data, err = afero.ReadFile(memFs, testFileName)
	assert.NoError(t, err, "ReadFile should not return an error")
	assert.Equal(t, newData, data, "File contents should match new data")

	// Test error case with a read-only filesystem
	roFs := afero.NewReadOnlyFs(afero.NewMemMapFs())
	roAferoFs := NewAferoFS(roFs)
	err = roAferoFs.WriteFile("should-fail.txt", []byte("test"), ReadWriteUserReadOthers)
	assert.Error(t, err, "WriteFile should return an error with read-only filesystem")
	assert.Contains(t, err.Error(), "failed to write file", "Error message should contain 'failed to write file'")
}

func TestSetFS(t *testing.T) {
	// Save the original DefaultFS
	originalFS := DefaultFS

	// Create a mock filesystem
	mockFS := NewAferoFS(afero.NewMemMapFs())

	// Replace the default filesystem
	cleanup := SetFS(mockFS)

	// Verify DefaultFS has been changed
	assert.Equal(t, mockFS, DefaultFS, "DefaultFS should be set to mockFS")

	// Call the cleanup function
	cleanup()

	// Verify DefaultFS has been restored
	assert.Equal(t, originalFS, DefaultFS, "DefaultFS should be restored to originalFS")
}

func TestAferoFile(t *testing.T) {
	memFs := afero.NewMemMapFs()
	fs := NewAferoFS(memFs)

	// Create a file
	fileName := "test.txt"
	file, err := fs.Create(fileName)
	require.NoError(t, err, "Failed to create test file")

	// Test writing a string
	testStr := "test string"
	n, err := file.WriteString(testStr)
	assert.NoError(t, err, "WriteString should not return an error")
	assert.Equal(t, len(testStr), n, "WriteString should return the number of bytes written")

	// Close the file
	err = file.Close()
	assert.NoError(t, err, "Close should not return an error")

	// Open the file again
	file, err = fs.Open(fileName)
	require.NoError(t, err, "Failed to open test file")

	// Test reading the file
	buf := make([]byte, 100)
	n, err = file.Read(buf)
	assert.NoError(t, err, "Read should not return an error")
	assert.Equal(t, len(testStr), n, "Read should return the number of bytes read")
	assert.Equal(t, testStr, string(buf[:n]), "Read data should match written data")

	// Test seeking
	_, err = file.Seek(0, io.SeekStart)
	assert.NoError(t, err, "Seek should not return an error")

	// Test reading at offset
	buf2 := make([]byte, 4)
	n, err = file.ReadAt(buf2, 5)
	assert.NoError(t, err, "ReadAt should not return an error")
	assert.Equal(t, 4, n, "ReadAt should return the number of bytes read")
	assert.Equal(t, "stri", string(buf2), "ReadAt data should match expected substring")

	// Test getting file info
	info, err := file.Stat()
	assert.NoError(t, err, "Stat should not return an error")
	assert.Equal(t, fileName, info.Name(), "File name should match")
	assert.Equal(t, int64(len(testStr)), info.Size(), "File size should match string length")

	// Close the file
	err = file.Close()
	assert.NoError(t, err, "Close should not return an error")
}

func TestGetUnderlyingFs(t *testing.T) {
	// Create different types of filesystems
	osFs := afero.NewOsFs()
	memFs := afero.NewMemMapFs()

	// Test with regular OsFs
	underlyingFs := GetUnderlyingFs(osFs)
	assert.Equal(t, osFs, underlyingFs, "OsFs should return itself as the underlying fs")

	// Test with regular MemMapFs
	underlyingFs = GetUnderlyingFs(memFs)
	assert.Equal(t, memFs, underlyingFs, "MemMapFs should return itself as the underlying fs")

	// Test with a BasePathFs
	basePathFs := afero.NewBasePathFs(memFs, "/base")
	underlyingFs = GetUnderlyingFs(basePathFs)
	assert.Equal(t, memFs, underlyingFs, "BasePathFs should return its underlying MemMapFs")

	// Test with nested BasePathFs
	nestedBasePathFs := afero.NewBasePathFs(basePathFs, "/nested")
	underlyingFs = GetUnderlyingFs(nestedBasePathFs)
	assert.Equal(t, memFs, underlyingFs, "Nested BasePathFs should return the root underlying MemMapFs")

	// Test with ReadOnlyFs
	readOnlyFs := afero.NewReadOnlyFs(memFs)
	underlyingFs = GetUnderlyingFs(readOnlyFs)
	assert.Equal(t, memFs, underlyingFs, "ReadOnlyFs should return its underlying MemMapFs")

	// Test with complex nesting
	complexFs := afero.NewReadOnlyFs(afero.NewBasePathFs(afero.NewReadOnlyFs(memFs), "/complex"))
	underlyingFs = GetUnderlyingFs(complexFs)
	assert.Equal(t, memFs, underlyingFs, "Complex nested Fs should return the root underlying MemMapFs")
}

func TestAferoFS_GetUnderlyingFs(t *testing.T) {
	// Create different types of filesystems
	osFs := afero.NewOsFs()
	memFs := afero.NewMemMapFs()

	// Create AferoFS instances
	osAferoFS := NewAferoFS(osFs)
	memAferoFS := NewAferoFS(memFs)

	// Test with OsFs
	underlyingFs := osAferoFS.GetUnderlyingFs()
	assert.Equal(t, osFs, underlyingFs, "AferoFS.GetUnderlyingFs should return the original OsFs")

	// Test with MemMapFs
	underlyingFs = memAferoFS.GetUnderlyingFs()
	assert.Equal(t, memFs, underlyingFs, "AferoFS.GetUnderlyingFs should return the original MemMapFs")

	// Test with nil fs (should return default OsFs)
	nilAferoFS := NewAferoFS(nil)
	underlyingFs = nilAferoFS.GetUnderlyingFs()
	assert.NotNil(t, underlyingFs, "AferoFS.GetUnderlyingFs should never return nil")
	_, isOsFs := underlyingFs.(*afero.OsFs)
	assert.True(t, isOsFs, "AferoFS created with nil should have OsFs as underlying fs")
}
