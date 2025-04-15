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

### pkg/registry

Registry mapping file operations should use the filesystem interface:

```go
// Either as a field in a struct
type MappingLoader struct {
    fs fileutil.FS
}

// Or using the package variable approach
var fs = fileutil.DefaultFs

func LoadMappings(path string) (Mappings, error) {
    data, err := fs.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read mapping file: %w", err)
    }
    // Parse data...
}
```

## Conclusion

Following these guidelines ensures consistent, reliable, and maintainable testing across the IRR codebase. The hybrid approach balances pragmatism with good design, allowing incremental adoption without requiring a complete rewrite of existing code.

Remember:
- Use dependency injection for new code and major refactors
- Use package variables for minimal changes to existing code
- Always provide test helpers for swapping filesystem implementations
- Consistently use the standard filesystem interface
- Follow proper test setup and teardown patterns 