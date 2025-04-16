package chart

import (
	"errors"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/strategy"
	"github.com/stretchr/testify/assert"
)

func TestErrorSentinels(t *testing.T) {
	// Test error sentinel values
	assert.Equal(t, "strict mode validation failed: unsupported structures found", ErrStrictValidationFailed.Error())
	assert.Equal(t, "chart not found", ErrChartNotFound.Error())
	assert.Equal(t, "helm loader failed", ErrChartLoadFailed.Error())
}

func TestUnsupportedStructureError(t *testing.T) {
	path := []string{"image", "repository"}
	errorType := "map-registry-repository-tag"
	err := &UnsupportedStructureError{
		Path: path,
		Type: errorType,
	}

	// Test Error() method
	expectedMsg := "unsupported structure at path image.repository (type: map-registry-repository-tag)"
	assert.Equal(t, expectedMsg, err.Error())

	// Test Is() method
	assert.True(t, errors.Is(err, ErrStrictValidationFailed))
	assert.False(t, errors.Is(err, ErrChartNotFound))
}

func TestThresholdNotMetError(t *testing.T) {
	err := &ThresholdNotMetError{
		Actual:   70,
		Required: 80,
	}

	// Test Error() method
	expectedMsg := "processing threshold not met: required 80%, actual 70%"
	assert.Equal(t, expectedMsg, err.Error())

	// Test Is() method
	assert.True(t, errors.Is(err, strategy.ErrThresholdExceeded))
	assert.False(t, errors.Is(err, ErrChartNotFound))
}

func TestParsingError(t *testing.T) {
	innerErr := errors.New("file not found")
	err := &ParsingError{
		FilePath: "/path/to/chart.yaml",
		Message:  "Failed to parse chart manifest",
		Err:      innerErr,
	}

	// Test Error() method
	assert.Equal(t, "error parsing chart: file not found", err.Error())

	// Test Is() method with ErrChartNotFound
	assert.True(t, errors.Is(err, ErrChartNotFound))
	assert.True(t, errors.Is(err, ErrChartLoadFailed))
	assert.False(t, errors.Is(err, strategy.ErrThresholdExceeded))

	// Test Unwrap() method
	assert.Equal(t, innerErr, err.Unwrap())
	assert.True(t, errors.Is(err, innerErr))
}

func TestImageProcessingError(t *testing.T) {
	// Test with path and reference
	innerErr := errors.New("invalid image format")
	path := []string{"image", "repository"}
	ref := "docker.io/myapp:latest"
	err := &ImageProcessingError{
		Path: path,
		Ref:  ref,
		Err:  innerErr,
	}

	// Test Error() method with path
	errorMsg := err.Error()
	assert.True(t, strings.Contains(errorMsg, "image processing error at path 'image.repository'"))
	assert.True(t, strings.Contains(errorMsg, "ref: docker.io/myapp:latest"))
	assert.True(t, strings.Contains(errorMsg, "invalid image format"))

	// Test Unwrap() method
	assert.Equal(t, innerErr, err.Unwrap())
	assert.True(t, errors.Is(err, innerErr))

	// Test without path
	err = &ImageProcessingError{
		Path: []string{},
		Ref:  ref,
		Err:  innerErr,
	}
	errorMsg = err.Error()
	assert.True(t, strings.Contains(errorMsg, "image processing error (ref: docker.io/myapp:latest)"))
	assert.True(t, strings.Contains(errorMsg, "invalid image format"))
}
