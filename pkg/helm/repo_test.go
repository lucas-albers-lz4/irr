package helm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/repo/repotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a temporary repository and returns the required test objects.
// The caller should check the errors returned.
func setupTestRepo(t *testing.T) (tmpDir string, srv *repotest.Server, settings *cli.EnvSettings, rm *RepositoryManager, cleanup func()) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "helm-repo-test-*")
	require.NoError(t, err)

	// Create a test repository
	srv, err = repotest.NewTempServerWithCleanup(t, "test-chart")
	require.NoError(t, err)

	// Create a test repo file
	repoFile := filepath.Join(tmpDir, "repositories.yaml")
	rf := repo.NewFile()
	rf.Add(&repo.Entry{
		Name: "test-repo",
		URL:  srv.URL(),
	})
	err = rf.WriteFile(repoFile, 0o644)
	require.NoError(t, err)

	// Create settings with our test repo file
	settings = cli.New()
	settings.RepositoryConfig = repoFile

	// Create repository manager
	rm = NewRepositoryManager(settings)

	// Return a cleanup function
	cleanup = func() {
		srv.Stop()
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Failed to remove temp dir %s: %v", tmpDir, err)
		}
	}

	return tmpDir, srv, settings, rm, cleanup
}

func TestRepositoryManager_GetRepositories(t *testing.T) {
	_, _, _, rm, cleanup := setupTestRepo(t)
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
	rm.cache.lastSync = time.Now().Add(-DefaultCacheDuration - time.Second)
	repos3, err := rm.GetRepositories()
	require.NoError(t, err)
	assert.NotSame(t, repos, repos3, "Should refresh cache after expiration")
}

func TestRepositoryManager_FindChartInRepositories(t *testing.T) {
	_, _, _, rm, cleanup := setupTestRepo(t)
	defer cleanup()

	// Test finding non-existent chart (should return error)
	_, err := rm.FindChartInRepositories("non-existent-chart")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in any repository")

	// Test with empty repository list
	rm.cache.repos = &repo.File{
		Repositories: []*repo.Entry{},
	}
	// Make sure cache doesn't expire during test
	rm.cache.lastSync = time.Now()
	_, err = rm.FindChartInRepositories("any-chart")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no repositories configured")

	// Test with nil repository list - we'll use a mock repoFile that doesn't exist
	rm.settings.RepositoryConfig = "/does/not/exist.yaml"
	rm.cache.repos = nil
	rm.cache.lastSync = time.Time{} // Expired cache, force reload
	_, err = rm.FindChartInRepositories("any-chart")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load repositories")
}

func TestRepositoryManager_ClearCache(t *testing.T) {
	_, _, _, rm, cleanup := setupTestRepo(t)
	defer cleanup()

	// Get repositories to populate cache
	repos, err := rm.GetRepositories()
	require.NoError(t, err)
	require.NotNil(t, repos)

	// Clear cache
	rm.ClearCache()

	// Verify cache is cleared
	assert.Nil(t, rm.cache.repos)
	assert.Empty(t, rm.cache.indexes)
	assert.True(t, rm.cache.lastSync.IsZero())
}
