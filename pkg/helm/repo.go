// Package helm provides interfaces and implementations for interacting with the Helm package manager.
// It includes repository management, chart loading, and other Helm-related functionality.
package helm

import (
	"fmt"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/repo"

	log "github.com/lalbers/irr/pkg/log"
)

// RepositoryManager handles Helm repository operations with caching
type RepositoryManager struct {
	settings *cli.EnvSettings
	cache    *repoCache
}

type repoCache struct {
	repos    *repo.File
	indexes  map[string]*repo.IndexFile
	lastSync time.Time
	mu       sync.RWMutex
}

const (
	// DefaultCacheDuration is how long to cache repository data
	DefaultCacheDuration = 5 * time.Minute
)

// NewRepositoryManager creates a new repository manager
func NewRepositoryManager(settings *cli.EnvSettings) *RepositoryManager {
	return &RepositoryManager{
		settings: settings,
		cache: &repoCache{
			indexes: make(map[string]*repo.IndexFile),
		},
	}
}

// GetRepositories returns the list of configured repositories
func (rm *RepositoryManager) GetRepositories() (*repo.File, error) {
	rm.cache.mu.RLock()
	if rm.cache.repos != nil && time.Since(rm.cache.lastSync) < DefaultCacheDuration {
		defer rm.cache.mu.RUnlock()
		return rm.cache.repos, nil
	}
	rm.cache.mu.RUnlock()

	// Need to refresh cache
	rm.cache.mu.Lock()
	defer rm.cache.mu.Unlock()

	// Double check after acquiring write lock
	if rm.cache.repos != nil && time.Since(rm.cache.lastSync) < DefaultCacheDuration {
		return rm.cache.repos, nil
	}

	// Load repositories from config
	repoFile := rm.settings.RepositoryConfig
	repos, err := repo.LoadFile(repoFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load repositories: %w", err)
	}

	rm.cache.repos = repos
	rm.cache.lastSync = time.Now()
	return repos, nil
}

// FindChartInRepositories searches for a chart across all repositories
func (rm *RepositoryManager) FindChartInRepositories(chartName string) (string, error) {
	repos, err := rm.GetRepositories()
	if err != nil {
		return "", err
	}

	// Check if repositories are configured
	if repos == nil || len(repos.Repositories) == 0 {
		return "", fmt.Errorf("no repositories configured")
	}

	for _, r := range repos.Repositories {
		index, err := rm.getRepositoryIndex(r)
		if err != nil {
			log.Warn("Failed to get index for repository", "repository", r.Name, "error", err)
			continue
		}

		if _, exists := index.Entries[chartName]; exists {
			return r.Name, nil
		}
	}

	return "", fmt.Errorf("chart %s not found in any repository", chartName)
}

// getRepositoryIndex returns the index file for a repository, using cache if available
func (rm *RepositoryManager) getRepositoryIndex(r *repo.Entry) (*repo.IndexFile, error) {
	rm.cache.mu.RLock()
	if index, ok := rm.cache.indexes[r.Name]; ok && time.Since(rm.cache.lastSync) < DefaultCacheDuration {
		defer rm.cache.mu.RUnlock()
		return index, nil
	}
	rm.cache.mu.RUnlock()

	// Need to refresh cache
	rm.cache.mu.Lock()
	defer rm.cache.mu.Unlock()

	// Double check after acquiring write lock
	if index, ok := rm.cache.indexes[r.Name]; ok && time.Since(rm.cache.lastSync) < DefaultCacheDuration {
		return index, nil
	}

	// Create new index
	index := repo.NewIndexFile()
	if index == nil {
		return nil, fmt.Errorf("failed to create index for repository %s", r.Name)
	}

	// Cache the index
	rm.cache.indexes[r.Name] = index
	rm.cache.lastSync = time.Now()

	return index, nil
}

// ClearCache clears the repository cache
func (rm *RepositoryManager) ClearCache() {
	rm.cache.mu.Lock()
	defer rm.cache.mu.Unlock()

	rm.cache.repos = nil
	rm.cache.indexes = make(map[string]*repo.IndexFile)
	rm.cache.lastSync = time.Time{}
}
