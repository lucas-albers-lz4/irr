package helm

import (
	"context"
	"fmt"
	"testing"

	internalhm "github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockGetCurrentNamespace(t *testing.T) {
	// Create a mock client with default namespace
	mockClient := internalhm.NewMockHelmClient()

	// Verify the default namespace is "default"
	namespace := mockClient.GetCurrentNamespace()
	assert.Equal(t, "default", namespace)
	assert.Equal(t, 1, mockClient.GetNamespaceCallCount, "GetNamespaceCallCount should be incremented")

	// Test with a custom namespace
	customNS := "test-namespace"
	mockClient.CurrentNamespace = customNS

	// Verify the custom namespace is returned
	namespace = mockClient.GetCurrentNamespace()
	assert.Equal(t, customNS, namespace)
	assert.Equal(t, 2, mockClient.GetNamespaceCallCount, "GetNamespaceCallCount should be incremented again")
}

func TestMockFindChartForRelease(t *testing.T) {
	t.Run("Default chart path", func(t *testing.T) {
		// Create a new mock client
		mockClient := internalhm.NewMockHelmClient()

		// Call FindChartForRelease without setting up a specific chart path
		chartPath, err := mockClient.FindChartForRelease(context.Background(), "test-release", "default")

		// Verify results
		require.NoError(t, err)
		assert.Equal(t, "/mock/helm/charts/test-release", chartPath, "Should return the default mock path")
		assert.Equal(t, 1, mockClient.FindChartCallCount, "FindChartCallCount should be incremented")
	})

	t.Run("Custom chart path", func(t *testing.T) {
		// Create a new mock client
		mockClient := internalhm.NewMockHelmClient()

		// Set up a custom chart path for a release
		expectedPath := "/custom/path/to/chart"
		mockClient.SetupMockChartPath("custom-release", "custom-namespace", expectedPath)

		// Call FindChartForRelease with the custom release
		chartPath, err := mockClient.FindChartForRelease(context.Background(), "custom-release", "custom-namespace")

		// Verify results
		require.NoError(t, err)
		assert.Equal(t, expectedPath, chartPath, "Should return the custom mock path")
		assert.Equal(t, 1, mockClient.FindChartCallCount, "FindChartCallCount should be incremented")
	})

	t.Run("Error case", func(t *testing.T) {
		// Create a new mock client with error
		mockClient := internalhm.NewMockHelmClient()
		mockClient.FindChartError = fmt.Errorf("chart not found")

		// Call FindChartForRelease
		chartPath, err := mockClient.FindChartForRelease(context.Background(), "test-release", "default")

		// Verify error is returned
		assert.Error(t, err)
		assert.Equal(t, "", chartPath)
		assert.Equal(t, 1, mockClient.FindChartCallCount, "FindChartCallCount should be incremented")
	})
}
