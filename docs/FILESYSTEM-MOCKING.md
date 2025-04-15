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