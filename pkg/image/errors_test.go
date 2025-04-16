package image

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorDefinitions(t *testing.T) {
	// Map Structure Parsing Errors
	assert.NotNil(t, ErrInvalidImageMapRepo, "ErrInvalidImageMapRepo should be defined")
	assert.NotNil(t, ErrInvalidImageMapRegistryType, "ErrInvalidImageMapRegistryType should be defined")
	assert.NotNil(t, ErrInvalidImageMapTagType, "ErrInvalidImageMapTagType should be defined")
	assert.NotNil(t, ErrInvalidImageMapDigestType, "ErrInvalidImageMapDigestType should be defined")
	assert.NotNil(t, ErrRepoNotFound, "ErrRepoNotFound should be defined")
	assert.NotNil(t, ErrMissingRepoInImageMap, "ErrMissingRepoInImageMap should be defined")

	// String Parsing Errors
	assert.NotNil(t, ErrEmptyImageString, "ErrEmptyImageString should be defined")
	assert.NotNil(t, ErrEmptyImageReference, "ErrEmptyImageReference should be defined")
	assert.NotNil(t, ErrInvalidDigestFormat, "ErrInvalidDigestFormat should be defined")
	assert.NotNil(t, ErrInvalidTagFormat, "ErrInvalidTagFormat should be defined")
	assert.NotNil(t, ErrInvalidRepoName, "ErrInvalidRepoName should be defined")
	assert.NotNil(t, ErrInvalidImageRefFormat, "ErrInvalidImageRefFormat should be defined")
	assert.NotNil(t, ErrInvalidRegistryName, "ErrInvalidRegistryName should be defined")
	assert.NotNil(t, ErrInvalidImageString, "ErrInvalidImageString should be defined")
	assert.NotNil(t, ErrTemplateVariableInRepo, "ErrTemplateVariableInRepo should be defined")
	assert.NotNil(t, ErrInvalidTypeAssertion, "ErrInvalidTypeAssertion should be defined")

	// Common Validation Errors
	assert.NotNil(t, ErrUnsupportedImageType, "ErrUnsupportedImageType should be defined")
	assert.NotNil(t, ErrMissingTagOrDigest, "ErrMissingTagOrDigest should be defined")
	assert.NotNil(t, ErrTagAndDigestPresent, "ErrTagAndDigestPresent should be defined")
	assert.NotNil(t, ErrInvalidImageReference, "ErrInvalidImageReference should be defined")
	assert.NotNil(t, ErrAmbiguousStringPath, "ErrAmbiguousStringPath should be defined")
	assert.NotNil(t, ErrTemplateVariableDetected, "ErrTemplateVariableDetected should be defined")
	assert.NotNil(t, ErrSkippedTemplateDetection, "ErrSkippedTemplateDetection should be defined")

	// Path Manipulation Errors
	assert.NotNil(t, ErrEmptyPath, "ErrEmptyPath should be defined")
	assert.NotNil(t, ErrPathNotFound, "ErrPathNotFound should be defined")
	assert.NotNil(t, ErrPathElementNotMap, "ErrPathElementNotMap should be defined")
	assert.NotNil(t, ErrPathElementNotSlice, "ErrPathElementNotSlice should be defined")
	assert.NotNil(t, ErrArrayIndexOutOfBounds, "ErrArrayIndexOutOfBounds should be defined")
	assert.NotNil(t, ErrInvalidArrayIndex, "ErrInvalidArrayIndex should be defined")
	assert.NotNil(t, ErrCannotOverwriteStructure, "ErrCannotOverwriteStructure should be defined")
	assert.NotNil(t, ErrArrayIndexAsOnlyElement, "ErrArrayIndexAsOnlyElement should be defined")
}

func TestUnsupportedImageError(t *testing.T) {
	// Test UnsupportedImageError creation and string representation
	tests := []struct {
		name          string
		path          []string
		uType         UnsupportedType
		err           error
		expectedParts []string
	}{
		{
			name:          "error with underlying cause",
			path:          []string{"path", "to", "image"},
			uType:         UnsupportedTypeNonStringOrMap,
			err:           errors.New("underlying error"),
			expectedParts: []string{"path", "to", "image", "underlying error"},
		},
		{
			name:          "error without underlying cause",
			path:          []string{"another", "path"},
			uType:         UnsupportedTypeMapTagAndDigest,
			err:           nil,
			expectedParts: []string{"another", "path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsupportedErr := NewUnsupportedImageError(tt.path, tt.uType, tt.err)
			errStr := unsupportedErr.Error()

			// Check that the error string contains expected parts
			for _, part := range tt.expectedParts {
				assert.Contains(t, errStr, part, "Error string should contain path element")
			}

			// Test type assertion
			var imgErr *UnsupportedImageError
			assert.True(t, errors.As(unsupportedErr, &imgErr), "Should be able to assert to UnsupportedImageError")

			// Verify fields after assertion
			assert.Equal(t, tt.path, imgErr.Path, "Path should match")
			assert.Equal(t, tt.uType, imgErr.Type, "Type should match")
			if tt.err != nil {
				assert.Equal(t, tt.err.Error(), imgErr.Err.Error(), "Underlying error should match")
			} else {
				assert.Nil(t, imgErr.Err, "Underlying error should be nil")
			}
		})
	}
}

// TestErrorWrapping tests that errors can be properly wrapped and unwrapped
func TestErrorWrapping(t *testing.T) {
	// Create a wrapped error chain
	baseErr := errors.New("base error")

	// Check that errors.Is works for predefined errors
	isErr := ErrInvalidImageString

	// Create a properly wrapped error using fmt.Errorf with %w verb
	wrappedIsErr := fmt.Errorf("wrapped: %w", isErr)

	assert.True(t, errors.Is(isErr, ErrInvalidImageString), "errors.Is should work for identical errors")
	assert.True(t, errors.Is(wrappedIsErr, ErrInvalidImageString), "errors.Is should work for wrapped errors")

	// Check that errors.As works for custom error types
	customErr := NewUnsupportedImageError([]string{"path"}, UnsupportedTypeMapError, baseErr)

	// Create a properly wrapped error using fmt.Errorf with %w verb
	wrappedCustomErr := fmt.Errorf("wrapped: %w", customErr)

	var imgErr *UnsupportedImageError
	assert.True(t, errors.As(customErr, &imgErr), "errors.As should work for custom error types")
	assert.True(t, errors.As(wrappedCustomErr, &imgErr), "errors.As should work for properly wrapped custom errors")
}
