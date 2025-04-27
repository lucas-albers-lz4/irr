package helm

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultNamespace = "default"
	testNamespace    = "test-namespace"
	testReleaseName  = "test-release"
)

func TestMockGetCurrentNamespace(t *testing.T) {
	// Create a mock client with default namespace
	mockClient := NewMockHelmClient()

	// Verify the default namespace is "default"
	namespace := mockClient.GetCurrentNamespace()
	assert.Equal(t, defaultNamespace, namespace)
	assert.Equal(t, 1, mockClient.GetNamespaceCallCount, "GetNamespaceCallCount should be incremented")

	// Test with a custom namespace
	customNS := testNamespace
	mockClient.CurrentNamespace = customNS

	// Verify the custom namespace is returned
	namespace = mockClient.GetCurrentNamespace()
	assert.Equal(t, customNS, namespace)
	assert.Equal(t, 2, mockClient.GetNamespaceCallCount, "GetNamespaceCallCount should be incremented again")
}

func TestMockFindChartForRelease(t *testing.T) {
	t.Run("Default chart path", func(t *testing.T) {
		// Create a new mock client
		mockClient := NewMockHelmClient()

		// Call FindChartForRelease without setting up a specific chart path
		chartPath, err := mockClient.FindChartForRelease(context.Background(), testReleaseName, defaultNamespace)

		// Verify results
		require.NoError(t, err)
		assert.Equal(t, "/mock/helm/charts/"+testReleaseName, chartPath, "Should return the default mock path")
		assert.Equal(t, 1, mockClient.FindChartCallCount, "FindChartCallCount should be incremented")
	})

	t.Run("Custom chart path", func(t *testing.T) {
		// Create a new mock client
		mockClient := NewMockHelmClient()

		// Set up a custom chart path for a release
		const customRelease = "custom-release"
		const customNamespace = "custom-namespace"
		expectedPath := "/custom/path/to/chart"
		mockClient.SetupMockChartPath(customRelease, customNamespace, expectedPath)

		// Call FindChartForRelease with the custom release
		chartPath, err := mockClient.FindChartForRelease(context.Background(), customRelease, customNamespace)

		// Verify results
		require.NoError(t, err)
		assert.Equal(t, expectedPath, chartPath, "Should return the custom mock path")
		assert.Equal(t, 1, mockClient.FindChartCallCount, "FindChartCallCount should be incremented")
	})

	t.Run("Error case", func(t *testing.T) {
		// Create a new mock client with error
		mockClient := NewMockHelmClient()
		mockClient.FindChartError = fmt.Errorf("chart not found")

		// Call FindChartForRelease
		chartPath, err := mockClient.FindChartForRelease(context.Background(), testReleaseName, defaultNamespace)

		// Verify error is returned
		assert.Error(t, err)
		assert.Equal(t, "", chartPath)
		assert.Equal(t, 1, mockClient.FindChartCallCount, "FindChartCallCount should be incremented")
	})
}

func TestMockSetupMockChartPath(t *testing.T) {
	mockClient := NewMockHelmClient()
	expectedPath := "/path/to/specific/chart"
	releaseKey := fmt.Sprintf("%s/%s", testNamespace, testReleaseName)

	mockClient.SetupMockChartPath(testReleaseName, testNamespace, expectedPath)

	// Check internal state
	storedPath, exists := mockClient.FindChartResults[releaseKey]
	assert.True(t, exists, "Release key should exist in FindChartResults map")
	assert.Equal(t, expectedPath, storedPath, "Stored path should match the expected path")

	// Check via FindChartForRelease method
	actualPath, err := mockClient.FindChartForRelease(context.Background(), testReleaseName, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, expectedPath, actualPath, "FindChartForRelease should return the configured path")
}

func TestMockValidateRelease(t *testing.T) {
	t.Run("Successful validation", func(t *testing.T) {
		mockClient := NewMockHelmClient()
		// Setup required mock data for validation to pass internally
		mockClient.SetupMockRelease(testReleaseName, defaultNamespace, map[string]interface{}{}, &ChartMetadata{})

		err := mockClient.ValidateRelease(context.Background(), testReleaseName, defaultNamespace, []string{}, "")

		assert.NoError(t, err)
		assert.Equal(t, 1, mockClient.ValidateCallCount, "ValidateCallCount should be 1 after successful call")
	})

	t.Run("Release not found error", func(t *testing.T) {
		mockClient := NewMockHelmClient()
		// Don't setup mock release

		err := mockClient.ValidateRelease(context.Background(), "non-existent-release", defaultNamespace, []string{}, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found for validation", "Error message should indicate release not found")
		assert.Equal(t, 1, mockClient.ValidateCallCount, "ValidateCallCount should be 1 even on error")
	})

	t.Run("Configured validation error", func(t *testing.T) {
		mockClient := NewMockHelmClient()
		// Setup required mock data
		mockClient.SetupMockRelease(testReleaseName, defaultNamespace, map[string]interface{}{}, &ChartMetadata{})

		// Configure the mock to return an error
		expectedErr := fmt.Errorf("simulated validate error")
		mockClient.ValidateError = expectedErr

		err := mockClient.ValidateRelease(context.Background(), testReleaseName, defaultNamespace, []string{}, "")

		assert.ErrorIs(t, err, expectedErr, "Should return the configured error")
		assert.Equal(t, 1, mockClient.ValidateCallCount, "ValidateCallCount should be 1 when returning configured error")
	})
}

func TestMockListReleases(t *testing.T) {
	t.Run("Return mock releases", func(t *testing.T) {
		// Create a new mock client
		mockClient := NewMockHelmClient()

		// Set up mock releases
		expectedReleases := []*ReleaseElement{
			{
				Name:      "release1",
				Namespace: "default",
			},
			{
				Name:      "release2",
				Namespace: "test",
			},
		}
		mockClient.SetupMockReleases(expectedReleases)

		// Call ListReleases
		releases, err := mockClient.ListReleases(context.Background(), true)

		// Verify results
		require.NoError(t, err)
		assert.Equal(t, expectedReleases, releases, "Should return the configured mock releases")
		assert.Equal(t, 1, mockClient.ListReleasesCallCount, "ListReleasesCallCount should be incremented")
	})

	t.Run("Error case", func(t *testing.T) {
		// Create a new mock client with error
		mockClient := NewMockHelmClient()
		expectedError := fmt.Errorf("failed to list releases")
		mockClient.ListReleasesError = expectedError

		// Call ListReleases
		releases, err := mockClient.ListReleases(context.Background(), true)

		// Verify error is returned
		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, releases)
		assert.Equal(t, 1, mockClient.ListReleasesCallCount, "ListReleasesCallCount should be incremented")
	})
}

func TestMockSetupMockReleases(t *testing.T) {
	mockClient := NewMockHelmClient()
	expectedReleases := []*ReleaseElement{
		{
			Name:      "release1",
			Namespace: "default",
		},
		{
			Name:      "release2",
			Namespace: "test",
		},
	}

	// Call the helper method
	mockClient.SetupMockReleases(expectedReleases)

	// Verify the releases are stored correctly
	assert.Equal(t, expectedReleases, mockClient.MockReleases, "MockReleases should contain the configured releases")

	// Verify we can retrieve them via ListReleases
	releases, err := mockClient.ListReleases(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, expectedReleases, releases, "ListReleases should return the configured releases")
}
