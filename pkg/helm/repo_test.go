package helm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/repo/repotest"

	// Use fileutil constants for permissions
	"github.com/lalbers/irr/pkg/fileutil"
	// Assuming log package is used/needed
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a temporary repository and returns the required test objects.
func setupTestRepo(t *testing.T) (tmpDir string, srv *repotest.Server, settings *cli.EnvSettings, rm *RepositoryManager, cleanup func()) {
	t.Helper() // Mark as test helper

	tmpDir, err := os.MkdirTemp("", "helm-repo-test-*")
	require.NoError(t, err)

	// Create a test repository server. Provide "test-chart" as required by the function,
	// but we will overwrite its index.yaml immediately.
	srv, err = repotest.NewTempServerWithCleanup(t, "test-chart")
	require.NoError(t, err)
	t.Logf("Test server running at: %s", srv.URL())
	t.Logf("Test server root: %s", srv.Root())

	// Manually create an index.yaml with our test chart entry
	indexContent := fmt.Sprintf(`
apiVersion: v1
entries:
  test-chart:
  - apiVersion: v2
    appVersion: 1.0.0
    created: "%s"
    description: A Helm chart for testing
    digest: sha256:abcdef123456
    name: test-chart
    type: application
    urls:
    - %s/test-chart-0.1.0.tgz
    version: 0.1.0
generated: "%s"
`, time.Now().UTC().Format(time.RFC3339Nano), srv.URL(), time.Now().UTC().Format(time.RFC3339Nano))

	// Ensure the server's root directory exists (it should, but belt-and-suspenders)
	err = os.MkdirAll(srv.Root(), fileutil.ReadWriteExecuteUserReadExecuteOthers) // Use constant 0755
	require.NoError(t, err, "Failed to ensure test server root directory exists")

	// Write the manual index.yaml to the server's root directory
	indexPath := filepath.Join(srv.Root(), "index.yaml")
	err = os.WriteFile(indexPath, []byte(indexContent), fileutil.ReadWriteUserReadOthers) // Use constant 0644
	require.NoError(t, err, "Failed to write manual index.yaml to test server root")
	t.Logf("Manually wrote index.yaml to %s", indexPath)

	// Create a test repo file pointing to the server
	repoFile := filepath.Join(tmpDir, "repositories.yaml")
	rf := repo.NewFile()
	rf.Add(&repo.Entry{
		Name: "test-repo",
		URL:  srv.URL(),
	})
	err = rf.WriteFile(repoFile, fileutil.ReadWriteUserReadOthers) // Use constant 0644
	require.NoError(t, err)

	// Create settings with our test repo file
	settings = cli.New()
	settings.RepositoryConfig = repoFile
	// Set cache dir within temp dir to avoid polluting user cache
	settings.RepositoryCache = filepath.Join(tmpDir, "helm-cache")
	err = os.MkdirAll(settings.RepositoryCache, fileutil.ReadWriteExecuteUserReadExecuteOthers) // Use constant 0755
	require.NoError(t, err)
	t.Logf("Using Helm cache directory: %s", settings.RepositoryCache)

	// Create repository manager
	rm = NewRepositoryManager(settings)

	// Return a cleanup function
	cleanup = func() {
		srv.Stop()
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Warning: Failed to remove temp dir %s: %v", tmpDir, err)
		}
	}

	return tmpDir, srv, settings, rm, cleanup
}

func TestRepositoryManager_GetRepositories(t *testing.T) {
	_, _, _, rm, cleanup := setupTestRepo(t) //nolint:dogsled // Assign unused returns to _ to satisfy linter
	defer cleanup()

	// Test GetRepositories
	repos, err := rm.GetRepositories()
	require.NoError(t, err)
	require.NotNil(t, repos)
	require.Len(t, repos.Repositories, 1)
	assert.Equal(t, "test-repo", repos.Repositories[0].Name)

	// Test caching
	repos2, err := rm.GetRepositories()
	require.NoError(t, err)
	assert.Same(t, repos, repos2, "Should return cached repositories")

	// Test cache expiration
	rm.cache.mu.Lock()
	rm.cache.lastSync = time.Now().Add(-DefaultCacheDuration - time.Second)
	rm.cache.mu.Unlock()
	repos3, err := rm.GetRepositories()
	require.NoError(t, err)
	assert.NotSame(t, repos, repos3, "Should refresh cache after expiration")
}

func TestRepositoryManager_FindChartInRepositories(t *testing.T) {
	_, _, _, rm, cleanup := setupTestRepo(t) //nolint:dogsled // Assign unused returns to _ to satisfy linter
	defer cleanup()

	// Test finding an existing chart (provided by manual index.yaml)
	repoName, err := rm.FindChartInRepositories("test-chart")
	require.NoError(t, err, "Should not error when finding existing chart 'test-chart'")
	assert.Equal(t, "test-repo", repoName, "Should find chart 'test-chart' in the correct repository")

	// Test finding non-existent chart (should return error)
	_, err = rm.FindChartInRepositories("non-existent-chart")
	assert.Error(t, err, "Should error when chart is not found")
	assert.Contains(t, err.Error(), "not found in any repository", "Error message should indicate chart not found")

	// Test with empty repository list (force cache update)
	rm.cache.mu.Lock()
	rm.cache.repos = &repo.File{
		Repositories: []*repo.Entry{},
	}
	rm.cache.lastSync = time.Now() // Ensure cache is fresh but empty
	rm.cache.mu.Unlock()

	_, err = rm.FindChartInRepositories("any-chart")
	assert.Error(t, err, "Should error when no repositories are configured")
	assert.Contains(t, err.Error(), "no repositories configured", "Error message should indicate no repos configured")

	// Test with nil repository list - force reload from non-existent file
	rm.settings.RepositoryConfig = filepath.Join(t.TempDir(), "non-existent-repositories.yaml") // Use temp dir
	rm.ClearCache()                                                                             // Clear cache completely to force reload

	_, err = rm.FindChartInRepositories("any-chart")
	assert.Error(t, err, "Should error when repository file cannot be loaded")
	// Check if the underlying error is os.ErrNotExist
	assert.True(t, errors.Is(err, os.ErrNotExist), "Expected os.ErrNotExist, got %v", err)

	// Test case where index download fails for a repo
	// We need to setup a new repo manager pointing to a bad URL
	badRepoFile := filepath.Join(t.TempDir(), "bad-repositories.yaml")
	badRf := repo.NewFile()
	badRf.Add(&repo.Entry{
		Name: "bad-repo",
		URL:  "http://localhost:1", // Invalid URL likely to fail download fast
	})
	err = badRf.WriteFile(badRepoFile, fileutil.ReadWriteUserReadOthers) // Use constant 0644
	require.NoError(t, err)
	badSettings := cli.New()
	badSettings.RepositoryConfig = badRepoFile
	badSettings.RepositoryCache = filepath.Join(t.TempDir(), "bad-cache")
	err = os.MkdirAll(badSettings.RepositoryCache, fileutil.ReadWriteExecuteUserReadExecuteOthers) // Use constant 0755
	require.NoError(t, err)
	badRm := NewRepositoryManager(badSettings)

	_, err = badRm.FindChartInRepositories("any-chart")
	assert.Error(t, err, "Should error if index download fails for all repos")
	// Error message might vary, check for known part
	assert.Contains(t, err.Error(), "chart 'any-chart' not found in any repository", "Final error should indicate chart not found after trying repos")

	// --- Test cache behavior for indexes ---
	rm.ClearCache()                         // Start fresh
	_, _, _, rm, cleanup = setupTestRepo(t) //nolint:dogsled // Assign unused returns to _ to satisfy linter
	defer cleanup()

	// 1. Find chart, populating index cache
	_, err = rm.FindChartInRepositories("test-chart")
	require.NoError(t, err)
	rm.cache.mu.RLock()
	_, indexCached := rm.cache.indexes["test-repo"]
	lastSyncTime1 := rm.cache.lastSync
	rm.cache.mu.RUnlock()
	assert.True(t, indexCached, "Index for test-repo should be cached after successful find")

	// 2. Find again, should use cache
	_, err = rm.FindChartInRepositories("test-chart")
	require.NoError(t, err)
	rm.cache.mu.RLock()
	lastSyncTime2 := rm.cache.lastSync
	rm.cache.mu.RUnlock()
	assert.Equal(t, lastSyncTime1, lastSyncTime2, "lastSync time should not change when index cache is hit")

	// 3. Expire cache and find again
	rm.cache.mu.Lock()
	rm.cache.lastSync = time.Now().Add(-DefaultCacheDuration - time.Second)
	rm.cache.mu.Unlock()

	_, err = rm.FindChartInRepositories("test-chart")
	require.NoError(t, err)
	rm.cache.mu.RLock()
	lastSyncTime3 := rm.cache.lastSync
	rm.cache.mu.RUnlock()
	assert.True(t, lastSyncTime3.After(lastSyncTime1), "lastSync time should update after cache expires and index is refetched")
}

func TestRepositoryManager_ClearCache(t *testing.T) {
	_, _, _, rm, cleanup := setupTestRepo(t) //nolint:dogsled // Assign unused returns to _ to satisfy linter
	defer cleanup()

	// Get repositories to populate cache
	_, err := rm.GetRepositories() // Call GetRepositories to populate repo cache
	require.NoError(t, err)
	_, err = rm.FindChartInRepositories("test-chart") // Call FindChart to populate index cache
	require.NoError(t, err)

	// Verify cache is populated before clearing
	rm.cache.mu.RLock()
	assert.NotNil(t, rm.cache.repos, "Cache should have repos before clearing")
	assert.NotEmpty(t, rm.cache.indexes, "Cache should have indexes before clearing")
	assert.False(t, rm.cache.lastSync.IsZero(), "Cache should have sync time before clearing")
	rm.cache.mu.RUnlock()

	// Clear cache
	rm.ClearCache()

	// Verify cache is cleared
	rm.cache.mu.RLock()
	assert.Nil(t, rm.cache.repos, "Cache should not have repos after clearing")
	assert.Empty(t, rm.cache.indexes, "Cache should not have indexes after clearing")
	assert.True(t, rm.cache.lastSync.IsZero(), "Cache should have zero sync time after clearing")
	rm.cache.mu.RUnlock()
}
