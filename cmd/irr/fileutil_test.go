package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// testSuccessMessage is the success message used in tests
	testSuccessMessage = "Successfully wrote to %s"
	// testFilePerms is the permission set used for test files
	testFilePerms = 0o644
)

func TestWriteOutputFile(t *testing.T) {
	// Save original filesystem and restore after tests
	originalFs := AppFs
	defer func() { AppFs = originalFs }()

	t.Run("successful file write", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()

		// Test data
		outputFile := "test-output.yaml"
		content := []byte("test content")

		// Call the function
		err := writeOutputFile(outputFile, content, testSuccessMessage)

		// Assertions
		require.NoError(t, err)

		// Verify file exists and has the correct content
		exists, err := afero.Exists(AppFs, outputFile)
		require.NoError(t, err)
		assert.True(t, exists)

		// Read file content
		actualContent, err := afero.ReadFile(AppFs, outputFile)
		require.NoError(t, err)
		assert.Equal(t, content, actualContent)
	})

	t.Run("error when file already exists", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()

		// Test data
		outputFile := "existing-file.yaml"
		content := []byte("test content")

		// Create the file first
		err := afero.WriteFile(AppFs, outputFile, []byte("existing content"), testFilePerms)
		require.NoError(t, err)

		// Call the function
		err = writeOutputFile(outputFile, content, testSuccessMessage)

		// Assertions
		require.Error(t, err)

		// Verify it's the correct error type
		var exitErr *exitcodes.ExitCodeError
		assert.ErrorAs(t, err, &exitErr)
		assert.Equal(t, exitcodes.ExitIOError, exitErr.Code)

		// Verify file wasn't changed
		actualContent, err := afero.ReadFile(AppFs, outputFile)
		require.NoError(t, err)
		assert.Equal(t, []byte("existing content"), actualContent)
	})

	t.Run("creates directory if needed", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		AppFs = afero.NewMemMapFs()

		// Test data
		outputFile := filepath.Join("test-dir", "test-output.yaml")
		content := []byte("test content")

		// Call the function
		err := writeOutputFile(outputFile, content, testSuccessMessage)

		// Assertions
		require.NoError(t, err)

		// Verify directory was created
		dirInfo, err := AppFs.Stat("test-dir")
		require.NoError(t, err)
		assert.True(t, dirInfo.IsDir())

		// Verify file exists
		exists, err := afero.Exists(AppFs, outputFile)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("error when directory creation fails", func(t *testing.T) {
		// Create a new in-memory filesystem for testing
		mockFs := afero.NewMemMapFs()

		// Create a file with the same name as our desired directory
		err := afero.WriteFile(mockFs, "blocked-dir", []byte("not a directory"), testFilePerms)
		require.NoError(t, err)

		// Use a mocking wrapper that fails directory creation
		AppFs = &mockFailDirFs{
			Fs:          mockFs,
			failDirPath: "blocked-dir",
		}

		// Test data
		outputFile := filepath.Join("blocked-dir", "test-output.yaml")
		content := []byte("test content")

		// Call the function
		err = writeOutputFile(outputFile, content, testSuccessMessage)

		// Assertions
		require.Error(t, err)

		// Verify it's the correct error type
		var exitErr *exitcodes.ExitCodeError
		assert.ErrorAs(t, err, &exitErr)
		assert.Equal(t, exitcodes.ExitGeneralRuntimeError, exitErr.Code)

		// Verify file doesn't exist
		exists, err := afero.Exists(AppFs, outputFile)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

// mockFailDirFs is a wrapper for afero.Fs that fails MkdirAll for specific paths
type mockFailDirFs struct {
	Fs          afero.Fs
	failDirPath string
}

// Implement necessary methods from afero.Fs interface
func (m *mockFailDirFs) MkdirAll(path string, perm os.FileMode) error {
	if filepath.Dir(path) == m.failDirPath || path == m.failDirPath {
		return &os.PathError{Op: "mkdir", Path: path, Err: os.ErrInvalid}
	}
	return m.Fs.MkdirAll(path, perm) //nolint:wrapcheck // This is a mock implementation
}

// Delegate all other methods to the wrapped Fs
func (m *mockFailDirFs) Create(name string) (afero.File, error)    { return m.Fs.Create(name) }      //nolint:wrapcheck // Mock implementation
func (m *mockFailDirFs) Mkdir(name string, perm os.FileMode) error { return m.Fs.Mkdir(name, perm) } //nolint:wrapcheck // Mock implementation
func (m *mockFailDirFs) Open(name string) (afero.File, error)      { return m.Fs.Open(name) }        //nolint:wrapcheck // Mock implementation
func (m *mockFailDirFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return m.Fs.OpenFile(name, flag, perm) //nolint:wrapcheck // Mock implementation
}
func (m *mockFailDirFs) Remove(name string) error                  { return m.Fs.Remove(name) }             //nolint:wrapcheck // Mock implementation
func (m *mockFailDirFs) RemoveAll(path string) error               { return m.Fs.RemoveAll(path) }          //nolint:wrapcheck // Mock implementation
func (m *mockFailDirFs) Rename(oldname, newname string) error      { return m.Fs.Rename(oldname, newname) } //nolint:wrapcheck // Mock implementation
func (m *mockFailDirFs) Stat(name string) (os.FileInfo, error)     { return m.Fs.Stat(name) }               //nolint:wrapcheck // Mock implementation
func (m *mockFailDirFs) Name() string                              { return m.Fs.Name() }
func (m *mockFailDirFs) Chmod(name string, mode os.FileMode) error { return m.Fs.Chmod(name, mode) }     //nolint:wrapcheck // Mock implementation
func (m *mockFailDirFs) Chown(name string, uid, gid int) error     { return m.Fs.Chown(name, uid, gid) } //nolint:wrapcheck // Mock implementation
func (m *mockFailDirFs) Chtimes(name string, atime, mtime time.Time) error {
	return m.Fs.Chtimes(name, atime, mtime) //nolint:wrapcheck // Mock implementation
}

func TestGetReleaseNameAndNamespaceCommon(t *testing.T) {
	// Save original isHelmPlugin value and restore after test
	originalIsHelmPlugin := isHelmPlugin
	defer func() { isHelmPlugin = originalIsHelmPlugin }()

	t.Run("release name and namespace from flags", func(t *testing.T) {
		// Create a mock command with flags
		cmd := &cobra.Command{}
		cmd.Flags().String("release-name", "", "Release name")
		cmd.Flags().String("namespace", "", "Namespace")

		// Set flag values
		err := cmd.Flags().Set("release-name", "test-release")
		require.NoError(t, err)
		err = cmd.Flags().Set("namespace", "test-namespace")
		require.NoError(t, err)

		// Test with Helm plugin mode
		isHelmPlugin = true
		releaseName, namespace, err := getReleaseNameAndNamespaceCommon(cmd, []string{})
		require.NoError(t, err)
		assert.Equal(t, "test-release", releaseName)
		assert.Equal(t, "test-namespace", namespace)

		// Test with standalone mode
		isHelmPlugin = false
		releaseName, namespace, err = getReleaseNameAndNamespaceCommon(cmd, []string{})
		require.NoError(t, err)
		assert.Equal(t, "test-release", releaseName)
		assert.Equal(t, "test-namespace", namespace)
	})

	t.Run("release name from positional argument in plugin mode", func(t *testing.T) {
		// Create a mock command with namespace flag only
		cmd := &cobra.Command{}
		cmd.Flags().String("release-name", "", "Release name")
		cmd.Flags().String("namespace", "", "Namespace")

		// Set namespace flag only
		err := cmd.Flags().Set("namespace", "arg-namespace")
		require.NoError(t, err)

		// Test with Helm plugin mode and args
		isHelmPlugin = true
		releaseName, namespace, err := getReleaseNameAndNamespaceCommon(cmd, []string{"arg-release"})
		require.NoError(t, err)
		assert.Equal(t, "arg-release", releaseName)
		assert.Equal(t, "arg-namespace", namespace)

		// Test with standalone mode (should not use args for release name)
		isHelmPlugin = false
		releaseName, namespace, err = getReleaseNameAndNamespaceCommon(cmd, []string{"arg-release"})
		require.NoError(t, err)
		assert.Equal(t, "", releaseName)
		assert.Equal(t, "arg-namespace", namespace)
	})

	t.Run("error when flags cannot be retrieved", func(t *testing.T) {
		// Create a minimal command without the required flags
		cmd := &cobra.Command{}

		// This should error because the flags don't exist
		_, _, err := getReleaseNameAndNamespaceCommon(cmd, []string{})

		// Verify it's the correct error type
		require.Error(t, err)
		var exitErr *exitcodes.ExitCodeError
		assert.ErrorAs(t, err, &exitErr)
		assert.Equal(t, exitcodes.ExitInputConfigurationError, exitErr.Code)
	})
}
