// Package helm provides interfaces and implementations for interacting with the Helm package manager.
// It includes repository management, chart loading, and other Helm-related functionality.
package helm

import (
	"fmt"
	"os"
	"sync"
	"time"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// RepositoryManager handles Helm repository operations with caching
type RepositoryManager struct {
	settings *cli.EnvSettings
	cache    *repoCache
	getters  getter.Providers // Add getter providers
}

type repoCache struct {
	repos    *repo.File                 // Cached repository file (repositories.yaml)
	indexes  map[string]*repo.IndexFile // Cached index files for each repository name
	lastSync time.Time                  // Timestamp of the last successful sync (applies to both repos and indexes)
	mu       sync.RWMutex
}

const (
	// DefaultCacheDuration is how long to cache repository data
	DefaultCacheDuration = 5 * time.Minute
	// maxSampleRepoEntries defines the maximum number of sample repo entries to log.
	maxSampleRepoEntries = 3
)

// NewRepositoryManager creates a new repository manager
func NewRepositoryManager(settings *cli.EnvSettings) *RepositoryManager {
	// Initialize getters using the provided settings
	getters := getter.All(settings)
	log.Debug("Initialized RepositoryManager", "getters_count", len(getters))
	return &RepositoryManager{
		settings: settings,
		getters:  getters, // Store getters
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
		log.Debug("Returning cached repositories file")
		return rm.cache.repos, nil
	}
	rm.cache.mu.RUnlock()

	// Need to refresh cache
	rm.cache.mu.Lock()
	defer rm.cache.mu.Unlock()

	// Double check after acquiring write lock
	if rm.cache.repos != nil && time.Since(rm.cache.lastSync) < DefaultCacheDuration {
		log.Debug("Returning cached repositories file (double check)")
		return rm.cache.repos, nil
	}

	// Load repositories from config
	repoFile := rm.settings.RepositoryConfig
	log.Debug("Loading repositories file", "path", repoFile)
	repos, err := repo.LoadFile(repoFile)
	if err != nil {
		// If the file doesn't exist, return an empty repo file instead of error
		if os.IsNotExist(err) {
			log.Warn("Repositories file not found, returning empty list", "path", repoFile)
			newFile := repo.NewFile()
			rm.cache.repos = newFile
			rm.cache.lastSync = time.Now()
			return newFile, nil
		}
		return nil, fmt.Errorf("failed to load repositories: %w", err)
	}

	log.Debug("Successfully loaded repositories file", "path", repoFile, "count", len(repos.Repositories))
	rm.cache.repos = repos
	rm.cache.lastSync = time.Now()
	return repos, nil
}

// FindChartInRepositories searches for a chart across all repositories
func (rm *RepositoryManager) FindChartInRepositories(chartName string) (string, error) {
	log.Debug("Finding chart in repositories", "chartName", chartName)
	repos, err := rm.GetRepositories()
	if err != nil {
		return "", err
	}

	// *** END ADDED DEBUG LOG ***

	// Check if repositories are configured
	if repos == nil {
		log.Debug("Repositories list (repos) is nil")
	} else {
		// Linter (staticcheck) flags the log line below as potential nil dereference,
		// but the outer check ensures repos is not nil, and the subsequent check
		// ensures repos.Repositories is not nil before accessing Entries, making the log line safe.
		log.Debug("Repositories loaded/retrieved", "count", len(repos.Repositories), "is_nil", repos == nil)
		if len(repos.Repositories) > 0 {
			log.Debug("First repo entry details", "name", repos.Repositories[0].Name, "url", repos.Repositories[0].URL)
		}
	}

	// Actual check to prevent panic if repos or Repositories is nil
	if repos == nil || len(repos.Repositories) == 0 {
		log.Warn("No repositories configured or loaded")
		return "", fmt.Errorf("no repositories configured")
	}

	log.Debug("Searching repositories", "count", len(repos.Repositories))
	for _, r := range repos.Repositories {
		log.Debug("Checking repository", "repoName", r.Name, "repoURL", r.URL)
		index, err := rm.getRepositoryIndex(r)
		if err != nil {
			log.Warn("Failed to get or load index for repository", "repository", r.Name, "error", err)
			continue // Try next repository
		}

		if index == nil {
			log.Warn("Index file is nil, skipping repository", "repository", r.Name)
			continue
		}

		if _, exists := index.Entries[chartName]; exists {
			log.Debug("Chart found in repository", "chartName", chartName, "repoName", r.Name)
			return r.Name, nil // Found
		}
		log.Debug("Chart not found in this repository", "chartName", chartName, "repoName", r.Name)
	}

	log.Warn("Chart not found in any repository", "chartName", chartName)
	return "", fmt.Errorf("chart '%s' not found in any repository", chartName)
}

// getRepositoryIndex returns the index file for a repository, using cache if available
// or downloading and loading it otherwise.
func (rm *RepositoryManager) getRepositoryIndex(r *repo.Entry) (*repo.IndexFile, error) {
	// *** ENTRY LOG (Info) ***
	log.Info("--- ENTER getRepositoryIndex ---", "repoName", r.Name)

	rm.cache.mu.RLock()
	if index, ok := rm.cache.indexes[r.Name]; ok && time.Since(rm.cache.lastSync) < DefaultCacheDuration {
		defer rm.cache.mu.RUnlock()
		log.Info("Returning cached index", "repoName", r.Name)
		return index, nil
	}
	rm.cache.mu.RUnlock()

	// Need to refresh cache or download index
	rm.cache.mu.Lock()
	defer rm.cache.mu.Unlock()

	// Double check after acquiring write lock
	if index, ok := rm.cache.indexes[r.Name]; ok && time.Since(rm.cache.lastSync) < DefaultCacheDuration {
		log.Info("Returning cached index (double check)", "repoName", r.Name)
		return index, nil
	}

	// --- Download and Load Index --- //
	log.Info("Preparing to download index file", "repoName", r.Name, "repoURL", r.URL)
	chartRepo, err := repo.NewChartRepository(r, rm.getters)
	if err != nil {
		return nil, fmt.Errorf("failed to create chart repository object for %s: %w", r.Name, err)
	}

	// Set Helm specific options if available in Entry (e.g., credentials, TLS)
	// Log the constructed ChartRepository config for debugging
	log.Info("Constructed ChartRepository Config", "repoName", r.Name, "config", chartRepo.Config)
	chartRepo.Config.Username = r.Username
	chartRepo.Config.Password = r.Password
	chartRepo.Config.CertFile = r.CertFile
	chartRepo.Config.KeyFile = r.KeyFile
	chartRepo.Config.CAFile = r.CAFile
	chartRepo.Config.InsecureSkipTLSverify = r.InsecureSkipTLSverify
	chartRepo.Config.PassCredentialsAll = r.PassCredentialsAll

	// Log right before download attempt
	log.Info("Attempting index file download", "repoName", r.Name, "url", r.URL+"/index.yaml") // Approximate URL
	indexPath, err := chartRepo.DownloadIndexFile()
	if err != nil {
		// Log specific download error but allow continuing if other repos might work
		log.Warn("Failed to download index file", "repoName", r.Name, "error", err)
		// Return specific error for this repo, FindChartInRepositories will handle skipping
		return nil, fmt.Errorf("failed to download index for %s: %w", r.Name, err)
	}
	log.Info("Index file downloaded", "repoName", r.Name, "path", indexPath)

	// Load the downloaded index file
	loadedIndex, err := repo.LoadIndexFile(indexPath)
	if err != nil {
		// Log specific load error but allow continuing
		log.Warn("Failed to load index file from path", "repoName", r.Name, "path", indexPath, "error", err)
		// Return specific error for this repo
		return nil, fmt.Errorf("failed to load index file for %s from %s: %w", r.Name, indexPath, err)
	}

	log.Info("Successfully loaded index file", "repoName", r.Name, "apiVersion", loadedIndex.APIVersion, "entries_count", len(loadedIndex.Entries))
	if len(loadedIndex.Entries) > 0 {
		keys := make([]string, 0, maxSampleRepoEntries)
		count := 0
		for k := range loadedIndex.Entries {
			if count >= maxSampleRepoEntries {
				break
			}
			keys = append(keys, k)
			count++
		}
		log.Info("Sample entries in loaded index", "repoName", r.Name, "sample_keys", keys)
	}

	// Cache the loaded index
	rm.cache.indexes[r.Name] = loadedIndex
	// Update lastSync time since we successfully fetched an index
	// If GetRepositories was called recently, this extends the cache validity for *all* indexes.
	// If GetRepositories cache expired, this effectively resets it.
	// This might need refinement if per-repo caching is desired.
	rm.cache.lastSync = time.Now()

	return loadedIndex, nil
}

// ClearCache clears the repository cache
func (rm *RepositoryManager) ClearCache() {
	rm.cache.mu.Lock()
	defer rm.cache.mu.Unlock()

	log.Debug("Clearing repository cache")
	rm.cache.repos = nil
	rm.cache.indexes = make(map[string]*repo.IndexFile)
	rm.cache.lastSync = time.Time{}
}
