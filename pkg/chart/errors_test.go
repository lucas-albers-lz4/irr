package chart

import (
	"errors"
	"testing"

	"github.com/lalbers/irr/pkg/strategy"
	"github.com/stretchr/testify/assert"
)

func TestUnsupportedStructureError(t *testing.T) {
	path := []string{"image", "repository", "tag"}
	structType := "map-registry-repository-tag"

	err := &UnsupportedStructureError{
		Path: path,
		Type: structType,
	}

	// Test Error() method
	errMsg := err.Error()
	assert.Contains(t, errMsg, "unsupported structure")
	assert.Contains(t, errMsg, "image.repository.tag")
	assert.Contains(t, errMsg, "map-registry-repository-tag")

	// Test Is() method
	assert.True(t, errors.Is(err, ErrStrictValidationFailed))
	assert.False(t, errors.Is(err, ErrChartNotFound))
}

func TestThresholdNotMetError(t *testing.T) {
	err := &ThresholdNotMetError{
		Actual:   75,
		Required: 80,
	}

	// Test Error() method
	errMsg := err.Error()
	assert.Contains(t, errMsg, "processing threshold not met")
	assert.Contains(t, errMsg, "required 80%")
	assert.Contains(t, errMsg, "actual 75%")

	// Test Is() method
	assert.True(t, errors.Is(err, strategy.ErrThresholdExceeded))
	assert.False(t, errors.Is(err, ErrChartNotFound))
}

func TestParsingError(t *testing.T) {
	// Test with ErrChartNotFound
	err1 := &ParsingError{
		FilePath: "/path/to/chart",
		Message:  "chart not found",
		Err:      ErrChartNotFound,
	}

	// Test Error() method
	errMsg1 := err1.Error()
	assert.Contains(t, errMsg1, "error parsing chart")
	assert.Contains(t, errMsg1, "chart not found")

	// Test Is() method
	assert.True(t, errors.Is(err1, ErrChartNotFound))
	assert.False(t, errors.Is(err1, strategy.ErrThresholdExceeded))

	// Test with ErrChartLoadFailed
	err2 := &ParsingError{
		FilePath: "/path/to/chart",
		Message:  "failed to load chart",
		Err:      ErrChartLoadFailed,
	}

	// Test Error() method
	errMsg2 := err2.Error()
	assert.Contains(t, errMsg2, "error parsing chart")
	assert.Contains(t, errMsg2, "helm loader failed")

	// Test Is() method
	assert.True(t, errors.Is(err2, ErrChartLoadFailed))

	// Test Unwrap() method
	unwrappedErr1 := errors.Unwrap(err1)
	assert.Equal(t, ErrChartNotFound, unwrappedErr1)

	unwrappedErr2 := errors.Unwrap(err2)
	assert.Equal(t, ErrChartLoadFailed, unwrappedErr2)
}

func TestImageProcessingError(t *testing.T) {
	// Test with path
	path := []string{"image", "repository"}
	ref := "nginx:latest"
	underlyingErr := errors.New("invalid image format")

	err1 := &ImageProcessingError{
		Path: path,
		Ref:  ref,
		Err:  underlyingErr,
	}

	// Test Error() method
	errMsg1 := err1.Error()
	assert.Contains(t, errMsg1, "image processing error")
	assert.Contains(t, errMsg1, "image.repository")
	assert.Contains(t, errMsg1, "nginx:latest")
	assert.Contains(t, errMsg1, "invalid image format")

	// Test without path
	err2 := &ImageProcessingError{
		Path: []string{},
		Ref:  ref,
		Err:  underlyingErr,
	}

	// Test Error() method for empty path
	errMsg2 := err2.Error()
	assert.Contains(t, errMsg2, "image processing error")
	assert.Contains(t, errMsg2, "nginx:latest")
	assert.Contains(t, errMsg2, "invalid image format")
	assert.NotContains(t, errMsg2, "path")

	// Test Unwrap() method
	unwrappedErr := errors.Unwrap(err1)
	assert.Equal(t, underlyingErr, unwrappedErr)
}

func TestErrorSentinels(t *testing.T) {
	// Test error sentinel values
	assert.Equal(t, "strict mode validation failed: unsupported structures found", ErrStrictValidationFailed.Error())
	assert.Equal(t, "chart not found", ErrChartNotFound.Error())
	assert.Equal(t, "helm loader failed", ErrChartLoadFailed.Error())
}
