package helm

import (
	"context"
	"fmt"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/stretchr/testify/mock"
	"helm.sh/helm/v3/pkg/chart"
)

// MockHelmClient implements ClientInterface for testing
type MockHelmClient struct {
	mock.Mock // Embed testify mock
	// Mock responses
	ReleaseValues    map[string]map[string]interface{} // releaseName -> values
	ReleaseCharts    map[string]*ChartMetadata         // releaseName -> chart metadata
	TemplateResults  map[string]string                 // chartPath -> manifest
	CurrentNamespace string
	MockReleases     []*ReleaseElement // List of mock releases for ListReleases

	// Track calls for assertions
	GetValuesCallCount    int
	GetChartCallCount     int
	TemplateCallCount     int
	GetNamespaceCallCount int
	FindChartCallCount    int
	ValidateCallCount     int
	ListReleasesCallCount int

	// Error simulation
	GetValuesError    error
	GetChartError     error
	TemplateError     error
	FindChartError    error
	ValidateError     error
	ListReleasesError error
	FindChartResults  map[string]string // releaseKey -> chartPath

	// Track calls
	TemplateChartCalled bool
	TemplateChartErr    error // Error to return from TemplateChart
	ReleaseValuesErr    error
}

// NewMockHelmClient creates a new MockHelmClient
func NewMockHelmClient() *MockHelmClient {
	return &MockHelmClient{
		ReleaseValues:    make(map[string]map[string]interface{}),
		ReleaseCharts:    make(map[string]*ChartMetadata),
		TemplateResults:  make(map[string]string),
		FindChartResults: make(map[string]string),
		CurrentNamespace: "default",
		MockReleases:     []*ReleaseElement{},
	}
}

// GetReleaseValues returns mocked values for a release
func (m *MockHelmClient) GetReleaseValues(_ context.Context, releaseName, namespace string) (map[string]interface{}, error) {
	m.GetValuesCallCount++

	if m.GetValuesError != nil {
		return nil, m.GetValuesError
	}

	releaseKey := releaseName
	if namespace != "" {
		releaseKey = fmt.Sprintf("%s/%s", namespace, releaseName)
	}

	values, exists := m.ReleaseValues[releaseKey]
	if !exists {
		return nil, fmt.Errorf("release %q not found", releaseKey)
	}

	return values, nil
}

// GetChartFromRelease implements ClientInterface.GetChartFromRelease
func (m *MockHelmClient) GetChartFromRelease(_ context.Context, releaseName, namespace string) (*ChartMetadata, error) {
	m.GetChartCallCount++

	if m.GetChartError != nil {
		return nil, m.GetChartError
	}

	releaseKey := releaseName
	if namespace != "" {
		releaseKey = fmt.Sprintf("%s/%s", namespace, releaseName)
	}

	chartMeta, exists := m.ReleaseCharts[releaseKey]
	if !exists {
		return nil, fmt.Errorf("release %q not found", releaseKey)
	}

	return chartMeta, nil
}

// TemplateChart mocks the TemplateChart method
func (m *MockHelmClient) TemplateChart(_ context.Context, releaseName, namespace, chartPath string, _ /* values */ map[string]interface{}) (string, error) {
	m.TemplateChartCalled = true // Mark as called

	// Return preconfigured error first, if any
	if m.TemplateChartErr != nil {
		return "", m.TemplateChartErr
	}

	// Return predefined success result based on namespace/release key
	key := fmt.Sprintf("%s/%s", namespace, releaseName)
	if result, ok := m.TemplateResults[key]; ok {
		return result, nil
	}

	// Return a simple default if no specific result or error is configured
	log.Debug("Mock TemplateChart returning default success value", "key", key, "chartPath", chartPath)
	return "---\napiVersion: v1\nkind: Pod\nmetadata:\n  name: mock-pod", nil
}

// GetCurrentNamespace returns the mocked current namespace
func (m *MockHelmClient) GetCurrentNamespace() string {
	m.GetNamespaceCallCount++
	return m.CurrentNamespace
}

// FindChartForRelease returns a mocked chart path for a release
func (m *MockHelmClient) FindChartForRelease(_ context.Context, releaseName, namespace string) (string, error) {
	m.FindChartCallCount++

	if m.FindChartError != nil {
		return "", m.FindChartError
	}

	releaseKey := releaseName
	if namespace != "" {
		releaseKey = fmt.Sprintf("%s/%s", namespace, releaseName)
	}

	// If a specific result was configured, return it
	if path, exists := m.FindChartResults[releaseKey]; exists {
		return path, nil
	}

	// Otherwise, return a default path
	return fmt.Sprintf("/mock/helm/charts/%s", releaseName), nil
}

// ValidateRelease validates a release with overrides (mock implementation)
func (m *MockHelmClient) ValidateRelease(_ context.Context, releaseName, namespace string, _ []string, _ string) error {
	m.ValidateCallCount++

	if m.ValidateError != nil {
		return m.ValidateError
	}

	// Simply check if the release exists in our mock data
	releaseKey := releaseName
	if namespace != "" {
		releaseKey = fmt.Sprintf("%s/%s", namespace, releaseName)
	}

	_, valuesExist := m.ReleaseValues[releaseKey]
	_, chartExists := m.ReleaseCharts[releaseKey]

	if !valuesExist || !chartExists {
		return fmt.Errorf("release %q not found for validation", releaseKey)
	}

	return nil
}

// SetupMockRelease is a helper method to set up a mock release
func (m *MockHelmClient) SetupMockRelease(releaseName, namespace string, values map[string]interface{}, chartMetadata *ChartMetadata) {
	releaseKey := releaseName
	if namespace != "" {
		releaseKey = fmt.Sprintf("%s/%s", namespace, releaseName)
	}

	m.ReleaseValues[releaseKey] = values
	m.ReleaseCharts[releaseKey] = chartMetadata
}

// SetupMockTemplate configures the mock response for TemplateChart for a specific namespace/release key
func (m *MockHelmClient) SetupMockTemplate(namespace, releaseName, result string, err error) {
	if m.TemplateResults == nil {
		m.TemplateResults = make(map[string]string)
	}
	key := fmt.Sprintf("%s/%s", namespace, releaseName)
	m.TemplateResults[key] = result
	m.TemplateChartErr = err // Store the error to be returned by TemplateChart
}

// SetupMockChartPath is a helper method to set up a mock chart path for a release
func (m *MockHelmClient) SetupMockChartPath(releaseName, namespace, chartPath string) {
	releaseKey := releaseName
	if namespace != "" {
		releaseKey = fmt.Sprintf("%s/%s", namespace, releaseName)
	}

	if m.FindChartResults == nil {
		m.FindChartResults = make(map[string]string)
	}
	m.FindChartResults[releaseKey] = chartPath
}

// ListReleases returns a mocked list of Helm releases
func (m *MockHelmClient) ListReleases(_ context.Context, allNamespaces bool) ([]*ReleaseElement, error) {
	m.ListReleasesCallCount++

	if m.ListReleasesError != nil {
		return nil, m.ListReleasesError
	}

	// If allNamespaces is false, filter to only return releases from the current namespace
	if !allNamespaces {
		filteredReleases := make([]*ReleaseElement, 0)
		for _, release := range m.MockReleases {
			if release.Namespace == m.CurrentNamespace {
				filteredReleases = append(filteredReleases, release)
			}
		}
		return filteredReleases, nil
	}

	// Return all mock releases
	return m.MockReleases, nil
}

// SetupMockReleases is a helper method to configure mock releases for ListReleases
func (m *MockHelmClient) SetupMockReleases(releases []*ReleaseElement) {
	m.MockReleases = releases
}

// LoadChart is a mock implementation of the LoadChart method
func (m *MockHelmClient) LoadChart(_ /* chartPath */ string) (*chart.Chart, error) {
	// This is a mock implementation, so we can return a simple mock chart
	return &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "mock-chart",
			Version: "1.0.0",
		},
	}, nil
}

// GetReleaseChart is an alias for GetChartFromRelease to maintain backward compatibility
func (m *MockHelmClient) GetReleaseChart(_ context.Context, releaseName, namespace string) (*ChartMetadata, error) {
	return m.GetChartFromRelease(context.Background(), releaseName, namespace) // Use context.Background() or a relevant context
}
