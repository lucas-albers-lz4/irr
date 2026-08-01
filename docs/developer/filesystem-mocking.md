# Filesystem Mocking in IRR

This document provides guidance on filesystem mocking in the IRR codebase, covering architectural tradeoffs, implementation approaches, and test patterns.

## Overview

IRR uses the [afero](https://github.com/spf13/afero) library to abstract filesystem operations, enabling:

- Consistent interfaces for file operations
- Simplified testing without touching the actual filesystem
- Easy mocking of filesystem behaviors and failures

## Architectural Approaches

IRR uses two primary approaches for filesystem abstraction:

### 1. Package-Level Variables (Non-Intrusive Approach)

```go
// Package uses a global filesystem variable
var fs = afero.NewOsFs()

// SetFs allows tests to swap the filesystem implementation
func SetFs(newFs afero.Fs) func() {
    oldFs := fs
    fs = newFs
    return func() { fs = oldFs } // Return cleanup function
}

// Functions use the package-level variable
func ReadConfigFile(path string) ([]byte, error) {
    return fs.ReadFile(path)
}
```

**When to use this approach:**
- For packages with existing, stable code where significant refactoring is undesirable
- When backward compatibility needs to be maintained
- For smaller, focused packages with limited filesystem interaction

### 2. Dependency Injection (DI Approach)

```go
// Struct with explicit dependency
type FileOperations struct {
    fs afero.Fs
}

// Constructor with default
func NewFileOperations(fs afero.Fs) *FileOperations {
    if fs == nil {
        fs = afero.NewOsFs()
    }
    return &FileOperations{fs: fs}
}

// Methods use the dependency
func (f *FileOperations) ReadConfig(path string) ([]byte, error) {
    return f.fs.ReadFile(path)
}
```

**When to use this approach:**
- For new code or major refactorings
- In packages with complex filesystem interactions
- When greater flexibility and explicit dependencies are desired
- For components that are frequently tested

## Test Patterns

### Standard Test Pattern for Package-Level Variables

```go
func TestWithMockFs(t *testing.T) {
    // Create mock filesystem
    mockFs := afero.NewMemMapFs()
    
    // Setup test files/directories
    afero.WriteFile(mockFs, "test.yaml", []byte("test: data"), 0644)
    
    // Replace package filesystem with mock
    reset := mypackage.SetFs(mockFs)
    defer reset() // Restore original filesystem
    
    // Run test with mock filesystem
    result, err := mypackage.ReadConfigFile("test.yaml")
    assert.NoError(t, err)
    assert.Equal(t, "test: data", string(result))
}
```

### Standard Test Pattern for Dependency Injection

```go
func TestWithInjectedFs(t *testing.T) {
    // Create mock filesystem
    mockFs := afero.NewMemMapFs()
    
    // Setup test files/directories
    afero.WriteFile(mockFs, "test.yaml", []byte("test: data"), 0644)
    
    // Create instance with mock filesystem
    ops := mypackage.NewFileOperations(mockFs)
    
    // Run test with mock filesystem
    result, err := ops.ReadConfig("test.yaml")
    assert.NoError(t, err)
    assert.Equal(t, "test: data", string(result))
}
```

## Common Testing Scenarios

### Testing File Existence and Content

```go
// Write test data
err := afero.WriteFile(mockFs, "config.yaml", []byte("key: value"), 0644)
require.NoError(t, err)

// Test file existence
exists, err := afero.Exists(mockFs, "config.yaml")
assert.NoError(t, err)
assert.True(t, exists)

// Test file content
content, err := afero.ReadFile(mockFs, "config.yaml")
assert.NoError(t, err)
assert.Contains(t, string(content), "key: value")
```

### Testing File Not Found Errors

```go
// Test non-existent file
_, err := mypackage.ReadFile("nonexistent.yaml")
assert.Error(t, err)
assert.True(t, os.IsNotExist(err))
```

### Testing Directory Operations

```go
// Create directories
err := mockFs.MkdirAll("dir1/dir2", 0755)
require.NoError(t, err)

// Test directory exists
info, err := mockFs.Stat("dir1/dir2")
assert.NoError(t, err)
assert.True(t, info.IsDir())

// Test directory listing
entries, err := afero.ReadDir(mockFs, "dir1")
assert.NoError(t, err)
assert.Len(t, entries, 1)
assert.Equal(t, "dir2", entries[0].Name())
```

## Best Practices

1. **Cleanup after tests**: Always use `defer` with the cleanup function returned by `SetFs()`
2. **Isolate tests**: Create a fresh filesystem for each test to prevent interference
3. **Use descriptive test cases**: Group related filesystem tests with readable names
4. **Test error conditions**: Explicitly test file not found, permission errors, etc.
5. **Use helper functions**: Create helpers for common filesystem setup in test packages
6. **Write idiomatic Go**: Follow Go conventions for error handling and control flow

## Compatibility Notes

- Some afero adapters may have different behavior than the real filesystem
- Permissions behave differently in memory filesystems vs. OS filesystems
- Be cautious with paths on different operating systems (use `filepath.Join` consistently)

## Implementation Progress

The filesystem mocking approach has been implemented in the following packages:

- `pkg/helm`: Injectable filesystem in SDK
- `pkg/fileutil`: Core filesystem utilities
- `pkg/chart`: Chart loader and generator
- `pkg/registry`: Registry mapping file operations
- `cmd/irr`: CLI commands and operations 

# Filesystem Mocking Guidelines for Testing

This document outlines the standard approach for filesystem mocking in the IRR codebase tests.

## Why Mock the Filesystem?

Mocking the filesystem provides several critical benefits:

1. **Test Reliability**: Tests run consistently without being affected by the actual filesystem state
2. **Test Speed**: In-memory operations are faster than disk I/O
3. **Test Isolation**: Tests can't interfere with each other or the host system
4. **Parallel Testing**: Tests can run in parallel without filesystem conflicts
5. **Cross-Platform**: Tests run consistently across different operating systems

## Standard Approach: Hybrid Model

The IRR codebase uses a hybrid approach to filesystem abstraction:

### 1. For New Code and Major Refactoring (Preferred)

Use the dependency injection pattern:

```go
// Define a struct with explicit filesystem dependency
type FileOperations struct {
    fs fileutil.FS // Standard interface defined in the codebase
}

// Constructor with default
func NewFileOperations(fs fileutil.FS) *FileOperations {
    if fs == nil {
        fs = afero.NewOsFs()
    }
    return &FileOperations{fs: fs}
}

// Methods use the injected filesystem
func (f *FileOperations) ReadConfig(path string) ([]byte, error) {
    return f.fs.ReadFile(path)
}
```

### 2. For Existing Code with Minimal Changes

Use the package-level variable pattern:

```go
// In package
var fs fileutil.FS = afero.NewOsFs()

// Helper for tests to replace the filesystem
func SetFs(newFs fileutil.FS) func() {
    oldFs := fs
    fs = newFs
    return func() { fs = oldFs } // Return a cleanup function
}

// Package functions use the global filesystem
func ReadConfigFile(path string) ([]byte, error) {
    return fs.ReadFile(path)
}
```

## Standard Filesystem Interface

All packages should use the standard filesystem interface defined in `pkg/fileutil`:

```go
// FS defines the filesystem operations needed by IRR
type FS interface {
    Open(name string) (File, error)
    Stat(name string) (os.FileInfo, error)
    Create(name string) (File, error)
    ReadFile(filename string) ([]byte, error)
    WriteFile(filename string, data []byte, perm os.FileMode) error
    MkdirAll(path string, perm os.FileMode) error
    Remove(name string) error
    RemoveAll(path string) error
    // Add other methods as needed
}

// Ensure afero.Fs implements this interface
var _ FS = afero.NewOsFs()
```

## Test Patterns

### 1. Testing with Dependency Injection

```go
func TestWithDependencyInjection(t *testing.T) {
    // Create mock filesystem
    mockFs := afero.NewMemMapFs()
    
    // Setup test files/directories
    afero.WriteFile(mockFs, "config.yaml", []byte("key: value"), 0644)
    
    // Create component with mock filesystem
    operations := NewFileOperations(mockFs)
    
    // Run test using the component
    data, err := operations.ReadConfig("config.yaml")
    
    // Assert results
    assert.NoError(t, err)
    assert.Equal(t, "key: value", string(data))
}
```

### 2. Testing with Package Variables

```go
func TestWithPackageVariables(t *testing.T) {
    // Create mock filesystem
    mockFs := afero.NewMemMapFs()
    
    // Setup test files/directories
    afero.WriteFile(mockFs, "config.yaml", []byte("key: value"), 0644)
    
    // Replace package filesystem with mock and get cleanup function
    cleanup := SetFs(mockFs)
    defer cleanup() // Restore original filesystem when test completes
    
    // Run test using package functions
    data, err := ReadConfigFile("config.yaml")
    
    // Assert results
    assert.NoError(t, err)
    assert.Equal(t, "key: value", string(data))
}
```

## Implementation Guidelines

### 1. Filesystem Operations

Always use the filesystem interface for all file operations:

```go
// Incorrect - uses os package directly
file, err := os.Open(path)

// Correct - uses filesystem interface
file, err := fs.Open(path)
```

### 2. File Permissions

Use consistent file permissions:

- Regular files: `0644`
- Executable files: `0755`
- Directories: `0755`

```go
// Example
fs.WriteFile(path, data, 0644)
fs.MkdirAll(dirPath, 0755)
```

### 3. Path Handling

Use the filepath package for path manipulations:

```go
fullPath := filepath.Join(dir, file)
```

### 4. Error Handling

Always check errors from filesystem operations:

```go
file, err := fs.Open(path)
if err != nil {
    return fmt.Errorf("failed to open file: %w", err)
}
defer file.Close()
```

### 5. Test Setup and Teardown

Use proper setup and teardown patterns:

```go
func TestFileOperations(t *testing.T) {
    // Setup
    mockFs := afero.NewMemMapFs()
    cleanup := SetFs(mockFs)
    defer cleanup()
    
    // Ensure test directories exist
    fs.MkdirAll("testdata", 0755)
    
    // Create test files
    fs.WriteFile("testdata/config.yaml", []byte("test: data"), 0644)
    
    // Run tests...
}
```

## Package-by-Package Migration Plan

The migration to consistent filesystem mocking should follow this priority order:

1. `pkg/fileutil` - Core filesystem utilities
2. `pkg/registry` - Registry mapping file handling 
3. `pkg/chart` - Chart loading and processing
4. `pkg/testutil` - Test utilities
5. `cmd/irr` - Command-line interface

Each package should adopt the appropriate pattern based on its structure and usage:

- Packages with many public functions: Use package variables
- Packages with structured types: Use dependency injection
- New packages: Always use dependency injection

## Key Packages Requiring Migration

### pkg/fileutil

This should be the first package migrated as it forms the foundation for filesystem operations:

```go
// Define the standard FS interface
type FS interface {
    // Methods as defined above
}

// Default implementation
var DefaultFs FS = afero.NewOsFs()

// Helper for tests
func SetFs(fs FS) func() {
    oldFs := DefaultFs
    DefaultFs = fs
    return func() { DefaultFs = oldFs }
}

// Utility functions use DefaultFs
func FileExists(path string) (bool, error) {
    _, err := DefaultFs.Stat(path)
    if err == nil {
        return true, nil
    }
    if os.IsNotExist(err) {
        return false, nil
    }
    return false, err
}
``` 