// Package mocks provides mock implementations of interfaces used for testing.
package mocks

import (
	"helm.sh/helm/v3/pkg/release"
)

// MockGetValuesFunc defines the signature for a function that can mock GetValues behavior.
type MockGetValuesFunc func(releaseName string, allValues bool) (map[string]interface{}, error)

// MockHelmClient is a mock implementation of the HelmClient interface.
type MockHelmClient struct {
	MockChartPath       string
	MockTemplateResult  string
	MockTemplateError   error
	MockGetValuesResult map[string]interface{}
	MockGetValuesError  error
	GetValuesFunc       MockGetValuesFunc
}

// GetValues mocks the GetValues method.
func (m *MockHelmClient) GetValues(releaseName string, allValues bool) (map[string]interface{}, error) {
	if m.GetValuesFunc != nil {
		// Use the custom function if provided
		return m.GetValuesFunc(releaseName, allValues)
	}

	// Fallback to simpler mock fields
	if m.MockGetValuesError != nil {
		return nil, m.MockGetValuesError
	}
	if m.MockGetValuesResult != nil {
		// Return a copy to prevent modification by the caller
		resultCopy := make(map[string]interface{})
		for k, v := range m.MockGetValuesResult {
			resultCopy[k] = v
		}
		return resultCopy, nil
	}

	// Default behavior if nothing specific is set
	return map[string]interface{}{}, nil
}

// Template mocks the Template method.
func (m *MockHelmClient) Template(_, _ string, _ map[string]interface{}, _ string) (string, error) {
	if m.MockTemplateError != nil {
		return "", m.MockTemplateError
	}
	// Optionally use values to slightly customize template result if needed
	// For now, just return the mocked result
	return m.MockTemplateResult, nil
}

// GetChartPath mocks the GetChartPath method.
func (m *MockHelmClient) GetChartPath(_ string) (string, error) {
	// Return a predefined path or error based on mock setup
	// For simplicity, let's assume it returns a mock path or error if needed
	if m.MockChartPath != "" {
		return m.MockChartPath, nil
	}
	return "/mock/chart/path", nil // Default mock path
}

// SearchRepos mocks the SearchRepos method.
func (m *MockHelmClient) SearchRepos(_ string) ([]interface{}, error) {
	// Return mock search results using a generic interface{} type
	return nil, nil
}

// ListReleases mocks the ListReleases method.
func (m *MockHelmClient) ListReleases(_ string) ([]*release.Release, error) {
	// Return mock list of releases
	return []*release.Release{}, nil
}

// EnsurePlugins mocks the EnsurePlugins method.
func (m *MockHelmClient) EnsurePlugins(_ []string) error {
	// Assume plugins are always ensured in mock
	return nil
}
