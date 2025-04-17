package helm

import (
	"context"
	"fmt"
)

// MockHelmClient implements ClientInterface for testing
type MockHelmClient struct {
	// Mock responses
	ReleaseValues    map[string]map[string]interface{} // releaseName -> values
	ReleaseCharts    map[string]*ChartMetadata         // releaseName -> chart metadata
	TemplateResults  map[string]string                 // chartPath -> manifest
	CurrentNamespace string

	// Track calls for assertions
	GetValuesCallCount    int
	GetChartCallCount     int
	TemplateCallCount     int
	GetNamespaceCallCount int

	// Error simulation
	GetValuesError error
	GetChartError  error
	TemplateError  error
}

// NewMockHelmClient creates a new MockHelmClient
func NewMockHelmClient() *MockHelmClient {
	return &MockHelmClient{
		ReleaseValues:    make(map[string]map[string]interface{}),
		ReleaseCharts:    make(map[string]*ChartMetadata),
		TemplateResults:  make(map[string]string),
		CurrentNamespace: "default",
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

// GetReleaseChart returns mocked chart metadata for a release
func (m *MockHelmClient) GetReleaseChart(_ context.Context, releaseName, namespace string) (*ChartMetadata, error) {
	m.GetChartCallCount++

	if m.GetChartError != nil {
		return nil, m.GetChartError
	}

	releaseKey := releaseName
	if namespace != "" {
		releaseKey = fmt.Sprintf("%s/%s", namespace, releaseName)
	}

	chart, exists := m.ReleaseCharts[releaseKey]
	if !exists {
		return nil, fmt.Errorf("release %q not found", releaseKey)
	}

	return chart, nil
}

// TemplateChart returns a mocked template result
func (m *MockHelmClient) TemplateChart(_ context.Context, releaseName, chartPath string, _ map[string]interface{}, namespace string) (string, error) {
	m.TemplateCallCount++

	if m.TemplateError != nil {
		return "", m.TemplateError
	}

	result, exists := m.TemplateResults[chartPath]
	if !exists {
		// Return a default templated output if none configured
		return fmt.Sprintf("# Templated output for chart %s with release %s in namespace %s",
			chartPath, releaseName, namespace), nil
	}

	return result, nil
}

// GetCurrentNamespace returns the mocked current namespace
func (m *MockHelmClient) GetCurrentNamespace() string {
	m.GetNamespaceCallCount++
	return m.CurrentNamespace
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

// SetupMockTemplate is a helper method to set up a mock template result
func (m *MockHelmClient) SetupMockTemplate(chartPath, result string) {
	m.TemplateResults[chartPath] = result
}
