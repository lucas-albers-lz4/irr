package registry

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testMappingFilePath = "/path/to/mappings.yaml"

// PathError is an interface for errors that contain a file path
type PathError interface {
	error // Embed the standard error interface
	GetPath() string
}

// testWrapperFunction is a helper to test wrapper functions that wrap errors
func testWrapperFunction(t *testing.T, path, errorMsg string, innerErr error,
	wrapFunc func(string, error) error, expectedErrType interface{}) {
	// Create the wrapped error
	err := wrapFunc(path, innerErr)

	// Verify error type
	assert.True(t, errors.As(err, expectedErrType), "Error type assertion failed")

	// Add debugging info
	t.Logf("Wrapped error: %v, Expected type: %T", err, expectedErrType)

	// Handle the specific error types separately
	switch typedPtr := expectedErrType.(type) {
	case **ErrMappingFileParse:
		if typedPtr == nil || *typedPtr == nil {
			t.Fatalf("ErrMappingFileParse pointer is nil")
		}
		// Check path if the error type has a GetPath method
		if pathGetter, ok := interface{}(*typedPtr).(interface{ GetPath() string }); ok {
			assert.Equal(t, path, pathGetter.GetPath(), "Path mismatch in ErrMappingFileParse")
		}
		assert.Equal(t, innerErr, (*typedPtr).Unwrap(), "Underlying error mismatch in ErrMappingFileParse")
	case **ErrMappingFileRead:
		if typedPtr == nil || *typedPtr == nil {
			t.Fatalf("ErrMappingFileRead pointer is nil")
		}
		if pathGetter, ok := interface{}(*typedPtr).(interface{ GetPath() string }); ok {
			assert.Equal(t, path, pathGetter.GetPath(), "Path mismatch in ErrMappingFileRead")
		}
		assert.Equal(t, innerErr, (*typedPtr).Unwrap(), "Underlying error mismatch in ErrMappingFileRead")
	case **ErrMappingFileNotExist:
		if typedPtr == nil || *typedPtr == nil {
			t.Fatalf("ErrMappingFileNotExist pointer is nil")
		}
		// Check path if the error type has a GetPath method (it does not)
		// Instead, access the struct field directly after type assertion.
		// No direct GetPath method, assert on struct fields if needed after type assertion
		// For now, just check unwrap.
		assert.Equal(t, innerErr, (*typedPtr).Unwrap(), "Underlying error mismatch in ErrMappingFileNotExist")
	default:
		// This default case might be overly generic or unnecessary if all expected
		// types are handled explicitly. For now, keep it as a fallback.
		t.Logf("Falling back to default case for type %T", expectedErrType)

		var pathErr PathError // Use the defined interface type
		if errors.As(err, &pathErr) {
			assert.Equal(t, path, pathErr.GetPath())
		}

		var unwrapErr interface{ Unwrap() error }
		if errors.As(err, &unwrapErr) {
			assert.Equal(t, innerErr, unwrapErr.Unwrap())
		} else {
			// If it doesn't fit the Unwrap interface, maybe it doesn't wrap?
			// Or the type assertion itself failed.
			t.Errorf("Failed to convert error to expected type with Unwrap method: %T", err)
		}
	}

	// Test Error() message formatting
	assert.Contains(t, err.Error(), errorMsg, "Error message mismatch")
	assert.Contains(t, err.Error(), path, "Error message should contain path")
	if innerErr != nil {
		assert.Contains(t, err.Error(), innerErr.Error(), "Error message should contain inner error message")
		// Test error unwrapping
		assert.True(t, errors.Is(err, innerErr), "errors.Is check failed")
	}
}

func TestErrMappingFileRead(t *testing.T) {
	// Create an underlying error
	innerErr := errors.New("file system error")

	// Create the error instance
	err := &ErrMappingFileRead{
		Path: testMappingFilePath,
		Err:  innerErr,
	}

	// Test Error() method
	expectedMsg := fmt.Sprintf("failed to read mappings file '%s': %v", testMappingFilePath, innerErr)
	assert.Equal(t, expectedMsg, err.Error())

	// Test Unwrap() method
	assert.Equal(t, innerErr, err.Unwrap())
	assert.True(t, errors.Is(errors.Unwrap(err), innerErr))
}

func TestWrapMappingFileRead(t *testing.T) {
	innerErr := errors.New("permission denied")
	var mappingFileReadErr *ErrMappingFileRead

	testWrapperFunction(t, testMappingFilePath, "failed to read mappings file", innerErr,
		WrapMappingFileRead, &mappingFileReadErr)
}

func TestErrMappingFileParse_Unwrap(t *testing.T) {
	// Create an underlying error
	innerErr := errors.New("yaml parsing error")

	// Create the error instance
	err := &ErrMappingFileParse{
		Path: testMappingFilePath,
		Err:  innerErr,
	}

	// Test Error() method
	expectedMsg := fmt.Sprintf("failed to parse mappings file '%s': %v", testMappingFilePath, innerErr)
	assert.Equal(t, expectedMsg, err.Error())

	// Test Unwrap() method
	assert.Equal(t, innerErr, err.Unwrap())
	assert.True(t, errors.Is(errors.Unwrap(err), innerErr))
}

func TestWrapMappingFileParse(t *testing.T) {
	innerErr := errors.New("invalid yaml format")
	var parseErr *ErrMappingFileParse

	testWrapperFunction(t, testMappingFilePath, "failed to parse mappings file", innerErr,
		WrapMappingFileParse, &parseErr)
}

// TestWrapMappingPathNotInWD tests the WrapMappingPathNotInWD function.
func TestWrapMappingPathNotInWD(t *testing.T) {
	path := "../outside/mappings.yaml"
	err := WrapMappingPathNotInWD(path)

	// Check error type
	var targetErr *ErrMappingPathNotInWD
	assert.True(t, errors.As(err, &targetErr), "Error should be of type ErrMappingPathNotInWD")
	if targetErr != nil {
		assert.Equal(t, path, targetErr.Path, "Error path should match input path")
	}

	// Check error message
	expectedMsg := fmt.Sprintf("mappings file path '%s' must be within the current working directory tree", path)
	assert.Equal(t, expectedMsg, err.Error())
}

// TestWrapMappingExtension tests the WrapMappingExtension function.
func TestWrapMappingExtension(t *testing.T) {
	path := "/path/to/mappings.txt"
	err := WrapMappingExtension(path)

	// Check error type
	var targetErr *ErrMappingExtension
	assert.True(t, errors.As(err, &targetErr), "Error should be of type ErrMappingExtension")
	if targetErr != nil {
		assert.Equal(t, path, targetErr.Path, "Error path should match input path")
	}

	// Check error message
	expectedMsg := fmt.Sprintf("mappings file path must end with .yaml or .yml: %s", path)
	assert.Equal(t, expectedMsg, err.Error())
}

// TestWrapMappingFileEmpty tests the WrapMappingFileEmpty function.
func TestWrapMappingFileEmpty(t *testing.T) {
	path := "/path/to/empty.yaml"
	err := WrapMappingFileEmpty(path)

	// Check error type
	var targetErr *ErrMappingFileEmpty
	assert.True(t, errors.As(err, &targetErr), "Error should be of type ErrMappingFileEmpty")
	if targetErr != nil {
		assert.Equal(t, path, targetErr.Path, "Error path should match input path")
	}

	// Check error message
	expectedMsg := fmt.Sprintf("mappings file is empty: %s", path)
	assert.Equal(t, expectedMsg, err.Error())
}

// TestWrapDuplicateRegistryKey tests the WrapDuplicateRegistryKey function.
func TestWrapDuplicateRegistryKey(t *testing.T) {
	path := "/path/to/duplicates.yaml"
	key := "docker.io"
	err := WrapDuplicateRegistryKey(path, key)

	// Check error type
	var targetErr *ErrDuplicateRegistryKey
	assert.True(t, errors.As(err, &targetErr), "Error should be of type ErrDuplicateRegistryKey")
	if targetErr != nil {
		assert.Equal(t, path, targetErr.Path, "Error path should match input path")
		assert.Equal(t, key, targetErr.Key, "Error key should match input key")
	}

	// Check error message
	expectedMsg := fmt.Sprintf("duplicate registry key '%s' found in mappings file '%s'", key, path)
	assert.Equal(t, expectedMsg, err.Error())
}

// TestWrapInvalidPortNumber tests the WrapInvalidPortNumber function.
func TestWrapInvalidPortNumber(t *testing.T) {
	path := "/path/to/ports.yaml"
	key := "quay.io"
	value := "new-reg.com:70000"
	port := "70000"
	err := WrapInvalidPortNumber(path, key, value, port)

	// Check error type
	var targetErr *ErrInvalidPortNumber
	assert.True(t, errors.As(err, &targetErr), "Error should be of type ErrInvalidPortNumber")
	if targetErr != nil {
		assert.Equal(t, path, targetErr.Path)
		assert.Equal(t, key, targetErr.Key)
		assert.Equal(t, value, targetErr.Value)
		assert.Equal(t, port, targetErr.Port)
	}

	// Check error message
	expectedMsg := fmt.Sprintf("invalid port number '%s' in target registry value '%s' for source '%s' in mappings file '%s': port must be between 1 and 65535", port, value, key, path)
	assert.Equal(t, expectedMsg, err.Error())
}

// TestWrapKeyTooLong tests the WrapKeyTooLong function.
func TestWrapKeyTooLong(t *testing.T) {
	path := "/path/to/longkeys.yaml"
	// Generate a long key string
	key := ""
	for i := 0; i < 300; i++ {
		key += "a"
	}
	maxLength := 255
	length := len(key)
	err := WrapKeyTooLong(path, key, length, maxLength)

	// Check error type
	var targetErr *ErrKeyTooLong
	assert.True(t, errors.As(err, &targetErr), "Error should be of type ErrKeyTooLong")
	if targetErr != nil {
		assert.Equal(t, path, targetErr.Path)
		assert.Equal(t, key, targetErr.Key)
		assert.Equal(t, length, targetErr.Length)
		assert.Equal(t, maxLength, targetErr.Max)
	}

	// Check error message
	expectedMsg := fmt.Sprintf("registry key '%s' exceeds maximum length of %d characters in mappings file '%s'", key, maxLength, path)
	assert.Equal(t, expectedMsg, err.Error())
}

// TestWrapValueTooLong tests the WrapValueTooLong function.
func TestWrapValueTooLong(t *testing.T) {
	path := "/path/to/longvalues.yaml"
	key := "docker.io"
	// Generate a long value string
	value := ""
	for i := 0; i < 1100; i++ {
		value += "b"
	}
	maxLength := 1024
	length := len(value)
	err := WrapValueTooLong(path, key, value, length, maxLength)

	// Check error type
	var targetErr *ErrValueTooLong
	assert.True(t, errors.As(err, &targetErr), "Error should be of type ErrValueTooLong")
	if targetErr != nil {
		assert.Equal(t, path, targetErr.Path)
		assert.Equal(t, key, targetErr.Key)
		assert.Equal(t, value, targetErr.Value)
		assert.Equal(t, length, targetErr.Length)
		assert.Equal(t, maxLength, targetErr.Max)
	}

	// Check error message
	expectedMsg := fmt.Sprintf("registry value '%s' for key '%s' exceeds maximum length of %d characters in mappings file '%s'", value, key, maxLength, path)
	assert.Equal(t, expectedMsg, err.Error())
}

// TestWrapMappingFileNotExist tests the WrapMappingFileNotExist function.
func TestWrapMappingFileNotExist(t *testing.T) {
	innerErr := errors.New("no such file or directory")
	var notExistErr *ErrMappingFileNotExist

	testWrapperFunction(t, testMappingFilePath, "mappings file does not exist", innerErr,
		WrapMappingFileNotExist, &notExistErr)
}
