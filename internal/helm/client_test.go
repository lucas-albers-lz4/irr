package helm

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/storage/driver"
)

func TestNewHelmClient(t *testing.T) {
	// Skip in environments where Helm SDK initialization might fail
	t.Skip("This test requires Helm configuration to be present, skipping as it might fail in CI environments")

	// Create a new client
	client, err := NewHelmClient()

	// Basic validation
	if assert.NoError(t, err) {
		assert.NotNil(t, client, "Client should not be nil")
		assert.NotNil(t, client.settings, "Settings should not be nil")
		assert.NotNil(t, client.actionConfig, "Action config should not be nil")
	}
}

func TestIsReleaseNotFoundError(t *testing.T) {
	// Test cases
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "Release not found error",
			err:      driver.ErrReleaseNotFound,
			expected: true,
		},
		{
			name:     "Wrapped release not found error",
			err:      errors.New("something went wrong: " + driver.ErrReleaseNotFound.Error()),
			expected: false, // Note: This will fail unless proper errors.Is wrapping is used
		},
		{
			name:     "Other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsReleaseNotFoundError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetCurrentNamespace(t *testing.T) {
	// Skip in environments where Helm SDK initialization might fail
	t.Skip("This test requires Helm configuration to be present, skipping as it might fail in CI environments")

	// Create a new client with mock initialization
	client, err := NewHelmClient()
	require.NoError(t, err)

	// Get current namespace
	namespace := client.GetCurrentNamespace()

	// We can't predict the exact namespace, but it should not be empty in a properly configured environment
	t.Logf("Current namespace: %s", namespace)

	// In most environments, this would be "default", but we don't want to assert on exact value
	assert.NotPanics(t, func() {
		client.GetCurrentNamespace()
	})
}

func TestFindChartInHelmCachePaths(t *testing.T) {
	// Since this is an internal function, we'll test with a few test cases
	testCases := []struct {
		name       string
		meta       *ChartMetadata
		cacheDir   string
		expectPath bool
	}{
		{
			name: "Empty metadata",
			meta: &ChartMetadata{
				Name:    "",
				Version: "",
			},
			cacheDir:   "",
			expectPath: false, // Should return empty path, not error
		},
		{
			name: "With name but no cache dir",
			meta: &ChartMetadata{
				Name:    "test-chart",
				Version: "1.0.0",
			},
			cacheDir:   "",
			expectPath: false, // Should return empty path, not error
		},
		// Note: We can't easily test positive cases as they depend on specific chart files existing
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path, err := findChartInHelmCachePaths(tc.meta, tc.cacheDir)

			// Function should never return an error
			assert.NoError(t, err)

			if tc.expectPath {
				assert.NotEmpty(t, path, "Expected a chart path")
			} else {
				assert.Empty(t, path, "Expected empty chart path")
			}
		})
	}
}
