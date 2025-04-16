package registry

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testMappingFilePath = "/path/to/mappings.yaml"

// testWrapperFunction is a helper to test wrapper functions that wrap errors
func testWrapperFunction(t *testing.T, path, errorMsg string, innerErr error,
	wrapFunc func(string, error) error, expectedErrType interface{}) {
	// Create the wrapped error
	err := wrapFunc(path, innerErr)

	// Verify error type
	assert.True(t, errors.As(err, expectedErrType))

	// Add debugging info
	t.Logf("Expected error type: %T", expectedErrType)

	// Handle the specific error types separately
	switch typedPtr := expectedErrType.(type) {
	case **ErrMappingFileParse:
		if typedPtr == nil || *typedPtr == nil {
			t.Fatalf("ErrMappingFileParse pointer is nil")
		}
		assert.Equal(t, path, (*typedPtr).GetPath())
		assert.Equal(t, innerErr, (*typedPtr).Unwrap())
	case **ErrMappingFileRead:
		if typedPtr == nil || *typedPtr == nil {
			t.Fatalf("ErrMappingFileRead pointer is nil")
		}
		assert.Equal(t, path, (*typedPtr).GetPath())
		assert.Equal(t, innerErr, (*typedPtr).Unwrap())
	default:
		// For other types, use the original approach
		typedErr, ok := expectedErrType.(interface {
			Error() string
			Unwrap() error
		})
		if !ok {
			t.Fatalf("Failed to convert error to expected type")
		}

		// Verify error fields (assuming the error has Path and Err fields)
		if pathGetter, ok := typedErr.(interface{ GetPath() string }); ok {
			assert.Equal(t, path, pathGetter.GetPath())
		}
		assert.Equal(t, innerErr, typedErr.Unwrap())
	}

	// Test Error() message formatting
	assert.Contains(t, err.Error(), errorMsg)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), innerErr.Error())

	// Test error unwrapping
	assert.True(t, errors.Is(err, innerErr))
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
