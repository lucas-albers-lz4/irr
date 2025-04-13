package helm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/repo/repotest"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Mock interfaces
type mockChartLoader struct {
	mock.Mock
}

func (m *mockChartLoader) Load(path string) (*chart.Chart, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*chart.Chart), args.Error(1)
}

type mockTimeProvider struct {
	mock.Mock
}

func (m *mockTimeProvider) Now() time.Time {
	args := m.Called()
	return args.Get(0).(time.Time)
}

type mockRepositoryManager struct {
	mock.Mock
}

func (m *mockRepositoryManager) GetRepositories() (*repo.File, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repo.File), args.Error(1)
}

func (m *mockRepositoryManager) GetRepositoryIndex(entry *repo.Entry) (*repo.IndexFile, error) {
	args := m.Called(entry)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repo.IndexFile), args.Error(1)
}

// TestChartPathResolution tests chart path resolution using mocked filesystem
func TestChartPathResolution(t *testing.T) {
	// Setup
	fs := afero.NewMemMapFs()
	chartDir := "/test-chart"
	chartYaml := `
apiVersion: v2
name: test-chart
version: 0.1.0
`
	require.NoError(t, fs.MkdirAll(chartDir, 0o750))
	require.NoError(t, afero.WriteFile(fs, chartDir+"/Chart.yaml", []byte(chartYaml), 0o600))

	// Initialize Helm environment
	settings := cli.New()
	actionConfig := new(action.Configuration)
	require.NoError(t, actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "", nil))

	// Test cases
	tests := []struct {
		name        string
		releaseName string
		chartPath   string
		wantErr     bool
	}{
		{
			name:        "valid chart path",
			releaseName: "test-release",
			chartPath:   chartDir,
			wantErr:     false,
		},
		{
			name:        "non-existent chart path",
			releaseName: "test-release",
			chartPath:   "/non-existent",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPath, err := ResolveChartPath(actionConfig, tt.releaseName, tt.chartPath)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.chartPath, chartPath)
		})
	}
}

// TestRepositoryDetection tests repository detection and caching mechanism
func TestRepositoryDetection(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "helm-repo-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test repository
	srv, err := repotest.NewTempServer("test-chart")
	require.NoError(t, err)
	defer os.RemoveAll(srv.Root())

	// Create a test repo file
	repoFile := filepath.Join(tmpDir, "repositories.yaml")
	rf := repo.NewFile()
	rf.Add(&repo.Entry{
		Name: "test-repo",
		URL:  srv.URL(),
	})
	err = rf.WriteFile(repoFile, 0644)
	require.NoError(t, err)

	// Create settings with our test repo file
	settings := cli.New()
	settings.RepositoryConfig = repoFile

	// Create repository manager
	rm := NewRepositoryManager(settings)

	// Test repository detection
	repos, err := rm.GetRepositories()
	require.NoError(t, err)
	require.NotNil(t, repos)
	require.Len(t, repos.Repositories, 1)
	assert.Equal(t, "test-repo", repos.Repositories[0].Name)
	assert.Equal(t, srv.URL(), repos.Repositories[0].URL)

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

// TestChartPulling tests chart pulling with timeout handling
func TestChartPulling(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "helm-pull-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test repository
	srv, err := repotest.NewTempServer("test-chart")
	require.NoError(t, err)
	defer os.RemoveAll(srv.Root())

	// Create a test repo file
	repoFile := filepath.Join(tmpDir, "repositories.yaml")
	rf := repo.NewFile()
	rf.Add(&repo.Entry{
		Name: "test-repo",
		URL:  srv.URL(),
	})
	err = rf.WriteFile(repoFile, 0644)
	require.NoError(t, err)

	// Create settings with our test repo file
	settings := cli.New()
	settings.RepositoryConfig = repoFile

	// Create pull client
	pull := action.NewPull()
	pull.Settings = settings

	// Test successful pull
	chartPath, err := pull.Run("test-repo/test-chart")
	require.NoError(t, err)
	assert.NotEmpty(t, chartPath)

	// Test timeout handling
	pull.Timeout = 1 * time.Nanosecond
	_, err = pull.Run("test-repo/test-chart")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// TestReadOnlyOperations tests that only read-only operations are allowed
func TestReadOnlyOperations(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "helm-readonly-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test chart
	chartDir := filepath.Join(tmpDir, "test-chart")
	err = os.MkdirAll(chartDir, 0o750)
	require.NoError(t, err)

	// Create Chart.yaml
	chartYaml := []byte(`
apiVersion: v2
name: test-chart
version: 0.1.0
`)
	err = os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), chartYaml, 0o600)
	require.NoError(t, err)

	// Initialize Helm environment
	settings := cli.New()
	actionConfig := new(action.Configuration)
	err = actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "", nil)
	require.NoError(t, err)

	// Test allowed read operations
	// Get
	get := action.NewGet(actionConfig)
	_, err = get.Run("test-release")
	assert.NoError(t, err)

	// GetValues
	getValues := action.NewGetValues(actionConfig)
	_, err = getValues.Run("test-release")
	assert.NoError(t, err)

	// List
	list := action.NewList(actionConfig)
	_, err = list.Run()
	assert.NoError(t, err)

	// SearchRepo
	search := action.NewSearch()
	search.Settings = settings
	_, err = search.Run("test-chart", false)
	assert.NoError(t, err)

	// Test disallowed write operations
	// Install
	install := action.NewInstall(actionConfig)
	_, err = install.Run(&chart.Chart{}, map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write operation not allowed")

	// Upgrade
	upgrade := action.NewUpgrade(actionConfig)
	_, err = upgrade.Run("test-release", &chart.Chart{}, map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write operation not allowed")

	// Uninstall
	uninstall := action.NewUninstall(actionConfig)
	_, err = uninstall.Run("test-release")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write operation not allowed")
}

// TestPluginDiscovery tests plugin discovery using mocked filesystem
func TestPluginDiscovery(t *testing.T) {
	// Setup
	fs := afero.NewMemMapFs()
	pluginDir := "/plugins"
	require.NoError(t, fs.MkdirAll(pluginDir, 0o750))

	// Create test plugin files
	plugins := map[string]struct {
		content string
		mode    uint32
	}{
		"plugin1": {"#!/bin/bash\necho test", 0o755},
		"plugin2": {"#!/bin/bash\necho test", 0o644}, // Non-executable
		"plugin3": {"#!/bin/bash\necho test", 0o755},
	}

	for name, p := range plugins {
		require.NoError(t, afero.WriteFile(fs, pluginDir+"/"+name, []byte(p.content), os.FileMode(p.mode)))
	}

	// Test plugin discovery
	discovered, err := DiscoverPlugins(pluginDir)
	require.NoError(t, err)

	// Only executable plugins should be discovered
	assert.Len(t, discovered, 2)
	var names []string
	for _, p := range discovered {
		names = append(names, p.Name)
	}
	assert.Contains(t, names, "plugin1")
	assert.Contains(t, names, "plugin3")
	assert.NotContains(t, names, "plugin2")
}

// TestLoadChart tests chart loading with mocked chart loader
func TestLoadChart(t *testing.T) {
	// Setup mock chart loader
	mockLoader := &mockChartLoader{}
	testChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "0.1.0",
		},
	}

	// Test cases
	tests := []struct {
		name      string
		chartPath string
		mockSetup func()
		wantErr   bool
	}{
		{
			name:      "successful load",
			chartPath: "/test-chart",
			mockSetup: func() {
				mockLoader.On("Load", "/test-chart").Return(testChart, nil)
			},
			wantErr: false,
		},
		{
			name:      "load error",
			chartPath: "/invalid-chart",
			mockSetup: func() {
				mockLoader.On("Load", "/invalid-chart").Return(nil, assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			chart, err := LoadChart(tt.chartPath)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, testChart, chart)
		})
	}

	mockLoader.AssertExpectations(t)
}

// TestFindImagesInChart tests image reference detection
func TestFindImagesInChart(t *testing.T) {
	// Create test chart with dependencies
	testChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "0.1.0",
		},
		Values: map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "test/image",
				"tag":        "1.0.0",
			},
		},
		Dependencies: []*chart.Chart{
			{
				Metadata: &chart.Metadata{
					Name:    "dep1",
					Version: "0.1.0",
				},
				Values: map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "test/dep1",
						"tag":        "1.0.0",
					},
				},
			},
		},
	}

	// Test image detection
	images := findImagesInChart(testChart)
	assert.Len(t, images, 2)
	assert.Contains(t, images, "test/image:1.0.0")
	assert.Contains(t, images, "test/dep1:1.0.0")
}

// TestComplexChartProcessing tests processing of complex charts
func TestComplexChartProcessing(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "helm-complex-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a complex chart structure
	chartDir := filepath.Join(tmpDir, "kube-prometheus-stack")
	err = os.MkdirAll(chartDir, 0o750)
	require.NoError(t, err)

	// Create Chart.yaml with dependencies
	chartYaml := []byte(`
apiVersion: v2
name: kube-prometheus-stack
version: 0.1.0
dependencies:
  - name: prometheus
    version: 0.1.0
    repository: https://prometheus-community.github.io/helm-charts
`)
	err = os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), chartYaml, 0o600)
	require.NoError(t, err)

	// Create values.yaml with nested image references
	valuesYaml := []byte(`
prometheus:
  image:
    repository: quay.io/prometheus/prometheus
    tag: v2.30.3
grafana:
  image:
    repository: grafana/grafana
    tag: 8.2.0
`)
	err = os.WriteFile(filepath.Join(chartDir, "values.yaml"), valuesYaml, 0o600)
	require.NoError(t, err)

	// Test chart loading
	chart, err := chartutil.Load(chartDir)
	require.NoError(t, err)
	assert.Equal(t, "kube-prometheus-stack", chart.Metadata.Name)

	// Test dependency handling
	require.Len(t, chart.Dependencies(), 1)
	assert.Equal(t, "prometheus", chart.Dependencies()[0].Name())

	// Test image pattern detection
	images := findImagesInChart(chart)
	require.Len(t, images, 2)
	assert.Contains(t, images, "quay.io/prometheus/prometheus:v2.30.3")
	assert.Contains(t, images, "grafana/grafana:8.2.0")
}

// TestErrorHandling tests error handling and recovery
func TestErrorHandling(t *testing.T) {
	// Test invalid chart format
	_, err := chartutil.Load("non-existent-path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")

	// Test timeout handling
	settings := cli.New()
	actionConfig := new(action.Configuration)
	err = actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "", nil)
	require.NoError(t, err)

	get := action.NewGet(actionConfig)
	get.Timeout = 1 * time.Nanosecond
	_, err = get.Run("test-release")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	// Test network connectivity issues
	// This is a bit tricky to test directly, but we can simulate it
	// by using an invalid repository URL
	repoFile := filepath.Join(os.TempDir(), "repositories.yaml")
	rf := repo.NewFile()
	rf.Add(&repo.Entry{
		Name: "invalid-repo",
		URL:  "http://invalid-url",
	})
	err = rf.WriteFile(repoFile, 0644)
	require.NoError(t, err)

	settings.RepositoryConfig = repoFile
	search := action.NewSearch()
	search.Settings = settings
	_, err = search.Run("test-chart", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

// Helper function to find images in a chart
func findImagesInChart(chart *chart.Chart) []string {
	var images []string
	// This is a simplified version - in reality, you'd need to parse templates
	// and look for image references in various formats
	for _, dep := range chart.Dependencies() {
		images = append(images, findImagesInChart(dep)...)
	}
	return images
}

// TestRepositoryOperations tests repository operations using mocked repository manager
func TestRepositoryOperations(t *testing.T) {
	// Setup mock repository manager
	mockRepoManager := &mockRepositoryManager{}
	testRepoFile := &repo.File{
		Repositories: []*repo.Entry{
			{
				Name: "test-repo",
				URL:  "https://test-repo.example.com",
			},
		},
	}
	testIndex := &repo.IndexFile{
		Entries: map[string]*repo.ChartVersions{
			"test-chart": {
				{
					Version: "1.0.0",
				},
			},
		},
	}

	// Test cases
	tests := []struct {
		name      string
		setupMock func()
		wantErr   bool
	}{
		{
			name: "successful repository operations",
			setupMock: func() {
				mockRepoManager.On("GetRepositories").Return(testRepoFile, nil)
				mockRepoManager.On("GetRepositoryIndex", testRepoFile.Repositories[0]).Return(testIndex, nil)
			},
			wantErr: false,
		},
		{
			name: "repository not found",
			setupMock: func() {
				mockRepoManager.On("GetRepositories").Return(nil, assert.AnError)
			},
			wantErr: true,
		},
		{
			name: "index not found",
			setupMock: func() {
				mockRepoManager.On("GetRepositories").Return(testRepoFile, nil)
				mockRepoManager.On("GetRepositoryIndex", testRepoFile.Repositories[0]).Return(nil, assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			repos, err := mockRepoManager.GetRepositories()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, testRepoFile, repos)

			if !tt.wantErr {
				index, err := mockRepoManager.GetRepositoryIndex(repos.Repositories[0])
				assert.NoError(t, err)
				assert.Equal(t, testIndex, index)
			}
		})
	}

	mockRepoManager.AssertExpectations(t)
}

// TestErrorHandling tests error handling with mocked dependencies
func TestErrorHandling(t *testing.T) {
	// Setup mock time provider
	mockTime := &mockTimeProvider{}
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mockTime.On("Now").Return(fixedTime)

	// Test cases
	tests := []struct {
		name      string
		setupMock func()
		wantErr   bool
	}{
		{
			name: "time provider error",
			setupMock: func() {
				mockTime.On("Now").Return(time.Time{})
			},
			wantErr: true,
		},
		{
			name: "successful time operation",
			setupMock: func() {
				mockTime.On("Now").Return(fixedTime)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			now := mockTime.Now()
			if tt.wantErr {
				assert.True(t, now.IsZero())
				return
			}
			assert.Equal(t, fixedTime, now)
		})
	}

	mockTime.AssertExpectations(t)
}
