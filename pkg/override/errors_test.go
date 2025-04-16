package override

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorSentinels(t *testing.T) {
	// Test error sentinel values
	assert.Equal(t, "data map cannot be nil", ErrNilDataMap.Error())
	assert.Equal(t, "empty path", ErrEmptyPath.Error())
	assert.Equal(t, "nil image reference", ErrNilImageReference.Error())
	assert.Equal(t, "error parsing path part", ErrPathParsing.Error())
	assert.Equal(t, "negative array index", ErrNegativeArrayIndex.Error())
	assert.Equal(t, "value is not an array", ErrNotAnArray.Error())
	assert.Equal(t, "cannot traverse through non-map", ErrNonMapTraversal.Error())
	assert.Equal(t, "invalid non-integer array index", ErrInvalidArrayIndex.Error())
	assert.Equal(t, "malformed array index syntax", ErrMalformedArrayIndex.Error())
	assert.Equal(t, "failed to marshal overrides to YAML", ErrMarshalOverrides.Error())
	assert.Equal(t, "failed to convert JSON to YAML", ErrJSONToYAML.Error())
	assert.Equal(t, "path not found", ErrPathNotFound.Error())
	assert.Equal(t, "array index out of bounds", ErrArrayIndexOutOfBounds.Error())
	assert.Equal(t, "cannot traverse through non-map or non-array", ErrNonMapOrArrayTraversal.Error())
}

func TestWrapPathParsing(t *testing.T) {
	innerErr := errors.New("inner error")
	part := "test[0]"
	err := WrapPathParsing(part, innerErr)

	// Test error message formatting
	assert.Contains(t, err.Error(), "error parsing path part")
	assert.Contains(t, err.Error(), part)
	assert.Contains(t, err.Error(), innerErr.Error())

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrPathParsing))
	assert.True(t, errors.Is(err, innerErr))
}

func TestWrapNegativeArrayIndex(t *testing.T) {
	index := -5
	err := WrapNegativeArrayIndex(index)

	// Test error message formatting
	assert.Contains(t, err.Error(), "negative array index")
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", index))

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrNegativeArrayIndex))
}

func TestWrapNotAnArray(t *testing.T) {
	key := "testKey"
	err := WrapNotAnArray(key)

	// Test error message formatting
	assert.Contains(t, err.Error(), "value is not an array")
	assert.Contains(t, err.Error(), key)

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrNotAnArray))
}

func TestWrapNonMapTraversalArray(t *testing.T) {
	index := 5
	value := "string value"
	err := WrapNonMapTraversalArray(index, value)

	// Test error message formatting
	assert.Contains(t, err.Error(), "cannot traverse through non-map")
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", index))
	assert.Contains(t, err.Error(), "string") // Type of value

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrNonMapTraversal))
}

func TestWrapNonMapTraversalMap(t *testing.T) {
	key := "testKey"
	err := WrapNonMapTraversalMap(key)

	// Test error message formatting
	assert.Contains(t, err.Error(), "cannot traverse through non-map")
	assert.Contains(t, err.Error(), key)

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrNonMapTraversal))
}

func TestWrapInvalidArrayIndex(t *testing.T) {
	indexStr := "abc"
	part := "test[abc]"
	err := WrapInvalidArrayIndex(indexStr, part)

	// Test error message formatting
	assert.Contains(t, err.Error(), "invalid non-integer array index")
	assert.Contains(t, err.Error(), indexStr)
	assert.Contains(t, err.Error(), part)

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrInvalidArrayIndex))
}

func TestWrapMalformedArrayIndex(t *testing.T) {
	part := "test["
	err := WrapMalformedArrayIndex(part)

	// Test error message formatting
	assert.Contains(t, err.Error(), "malformed array index syntax")
	assert.Contains(t, err.Error(), part)

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrMalformedArrayIndex))
}

func TestWrapMarshalOverrides(t *testing.T) {
	innerErr := errors.New("yaml marshal error")
	err := WrapMarshalOverrides(innerErr)

	// Test error message formatting
	assert.Contains(t, err.Error(), "failed to marshal overrides to YAML")
	assert.Contains(t, err.Error(), innerErr.Error())

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrMarshalOverrides))
	assert.True(t, errors.Is(err, innerErr))
}

func TestWrapJSONToYAML(t *testing.T) {
	innerErr := errors.New("json to yaml conversion error")
	err := WrapJSONToYAML(innerErr)

	// Test error message formatting
	assert.Contains(t, err.Error(), "failed to convert JSON to YAML")
	assert.Contains(t, err.Error(), innerErr.Error())

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrJSONToYAML))
	assert.True(t, errors.Is(err, innerErr))
}

func TestWrapPathNotFound(t *testing.T) {
	path := []string{"parent", "child", "value"}
	err := WrapPathNotFound(path)

	// Test error message formatting
	assert.Contains(t, err.Error(), "path not found")
	assert.Contains(t, err.Error(), "parent.child.value")

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrPathNotFound))
}

func TestWrapArrayIndexOutOfBounds(t *testing.T) {
	index := 10
	length := 5
	err := WrapArrayIndexOutOfBounds(index, length)

	// Test error message formatting
	assert.Contains(t, err.Error(), "array index out of bounds")
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", index))
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", length))

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrArrayIndexOutOfBounds))
}

func TestWrapNonMapOrArrayTraversal(t *testing.T) {
	path := []string{"parent", "child", "value"}
	err := WrapNonMapOrArrayTraversal(path)

	// Test error message formatting
	assert.Contains(t, err.Error(), "attempted to traverse non-map/non-array")
	assert.Contains(t, err.Error(), "parent.child.value")

	// Test error unwrapping
	assert.True(t, errors.Is(err, ErrNonMapOrArrayTraversal))
}
