package helm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListReleases(t *testing.T) {
	// Create a mock client to test with
	mockClient := NewMockHelmClient()

	// Setup mock releases
	mockReleases := []*ReleaseElement{
		{Name: "release1", Namespace: "default"},
		{Name: "release2", Namespace: "default"},
		{Name: "release3", Namespace: "other-namespace"},
	}
	mockClient.SetupMockReleases(mockReleases)

	// Test case 1: List releases in current namespace
	t.Run("ListInCurrentNamespace", func(t *testing.T) {
		releases, err := mockClient.ListReleases(context.Background(), false)
		require.NoError(t, err)
		assert.Len(t, releases, 2, "Should return 2 releases in default namespace")
		assert.Equal(t, 1, mockClient.ListReleasesCallCount)

		// Verify the release names
		releaseNames := []string{releases[0].Name, releases[1].Name}
		assert.Contains(t, releaseNames, "release1")
		assert.Contains(t, releaseNames, "release2")
	})

	// Test case 2: List releases across all namespaces
	t.Run("ListAllNamespaces", func(t *testing.T) {
		releases, err := mockClient.ListReleases(context.Background(), true)
		require.NoError(t, err)
		assert.Len(t, releases, 3, "Should return all 3 releases across namespaces")
		assert.Equal(t, 2, mockClient.ListReleasesCallCount)

		// Verify the release names
		releaseNames := []string{releases[0].Name, releases[1].Name, releases[2].Name}
		assert.Contains(t, releaseNames, "release1")
		assert.Contains(t, releaseNames, "release2")
		assert.Contains(t, releaseNames, "release3")
	})

	// Test case 3: Error handling
	t.Run("ErrorHandling", func(t *testing.T) {
		mockClient.ListReleasesError = assert.AnError
		releases, err := mockClient.ListReleases(context.Background(), false)
		assert.Error(t, err)
		assert.Nil(t, releases)
		assert.Equal(t, 3, mockClient.ListReleasesCallCount)

		// Reset error for subsequent tests
		mockClient.ListReleasesError = nil
	})
}
