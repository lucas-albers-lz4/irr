package helm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
)

// TestMockHelmClient_GetReleaseValues tests the MockHelmClient's GetReleaseValues method
func TestMockHelmClient_GetReleaseValues(t *testing.T) {
	// Create a mock client with custom mock function
	client := &MockHelmClient{
		MockGetReleaseValues: func(_ context.Context, releaseName, namespace string) (map[string]interface{}, error) {
			// Verify input parameters
			assert.Equal(t, "test-release", releaseName)
			assert.Equal(t, "test-namespace", namespace)

			// Return mock data
			return map[string]interface{}{
				"testKey": "testValue",
			}, nil
		},
	}

	// Call the method
	result, err := client.GetReleaseValues(context.Background(), "test-release", "test-namespace")

	// Verify result
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "testValue", result["testKey"])
}

// TestMockHelmClient_GetChartFromRelease tests the MockHelmClient's GetChartFromRelease method
func TestMockHelmClient_GetChartFromRelease(t *testing.T) {
	// Create expected chart
	expectedChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
	}

	// Create a mock client with custom mock function
	client := &MockHelmClient{
		MockGetChartFromRelease: func(_ context.Context, releaseName, namespace string) (*chart.Chart, error) {
			// Verify input parameters
			assert.Equal(t, "test-release", releaseName)
			assert.Equal(t, "test-namespace", namespace)

			// Return mock data
			return expectedChart, nil
		},
	}

	// Call the method
	result, err := client.GetChartFromRelease(context.Background(), "test-release", "test-namespace")

	// Verify result
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test-chart", result.Metadata.Name)
	assert.Equal(t, "1.0.0", result.Metadata.Version)
}

// TestMockHelmClient_GetReleaseMetadata tests the MockHelmClient's GetReleaseMetadata method
func TestMockHelmClient_GetReleaseMetadata(t *testing.T) {
	// Create expected metadata
	expectedMetadata := &chart.Metadata{
		Name:    "test-chart",
		Version: "1.0.0",
	}

	// Create a mock client with custom mock function
	client := &MockHelmClient{
		MockGetReleaseMetadata: func(_ context.Context, releaseName, namespace string) (*chart.Metadata, error) {
			// Verify input parameters
			assert.Equal(t, "test-release", releaseName)
			assert.Equal(t, "test-namespace", namespace)

			// Return mock data
			return expectedMetadata, nil
		},
	}

	// Call the method
	result, err := client.GetReleaseMetadata(context.Background(), "test-release", "test-namespace")

	// Verify result
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test-chart", result.Name)
	assert.Equal(t, "1.0.0", result.Version)
}

// TestMockHelmClient_TemplateChart tests the MockHelmClient's TemplateChart method
func TestMockHelmClient_TemplateChart(t *testing.T) {
	// Create a mock client with custom mock function
	client := &MockHelmClient{
		MockTemplateChart: func(_ context.Context, chartPath, releaseName, namespace string, values map[string]interface{}) (string, error) {
			// Verify input parameters
			assert.Equal(t, "test-chart-path", chartPath)
			assert.Equal(t, "test-release", releaseName)
			assert.Equal(t, "test-namespace", namespace)
			assert.Equal(t, "test-value", values["test-key"])

			// Return mock data
			return "templated-yaml-content", nil
		},
	}

	// Call the method
	result, err := client.TemplateChart(
		context.Background(),
		"test-chart-path",
		"test-release",
		"test-namespace",
		map[string]interface{}{"test-key": "test-value"},
	)

	// Verify result
	require.NoError(t, err)
	assert.Equal(t, "templated-yaml-content", result)
}

// TestNewMockHelmClient tests the NewMockHelmClient constructor
func TestNewMockHelmClient(t *testing.T) {
	// Create a new mock client using the constructor
	client := NewMockHelmClient()

	// Verify client is not nil
	require.NotNil(t, client)

	// Verify default mock functions
	values, err := client.GetReleaseValues(context.Background(), "some-release", "some-namespace")
	require.NoError(t, err)
	assert.NotNil(t, values)

	chartObj, err := client.GetChartFromRelease(context.Background(), "some-release", "some-namespace")
	require.NoError(t, err)
	assert.NotNil(t, chartObj)
	assert.Equal(t, "mock-chart", chartObj.Metadata.Name)

	metadata, err := client.GetReleaseMetadata(context.Background(), "some-release", "some-namespace")
	require.NoError(t, err)
	assert.NotNil(t, metadata)
	assert.Equal(t, "mock-chart", metadata.Name)

	templateOutput, err := client.TemplateChart(context.Background(), "some-path", "some-release", "some-namespace", nil)
	require.NoError(t, err)
	assert.Equal(t, "mock-template-output", templateOutput)
}

// TestRealHelmClient_GetReleaseValues_EmptyReleaseName tests error handling when release name is empty
func TestRealHelmClient_GetReleaseValues_EmptyReleaseName(t *testing.T) {
	// Create a real client
	client := NewRealHelmClient(cli.New())

	// Call the method with empty release name
	_, err := client.GetReleaseValues(context.Background(), "", "test-namespace")

	// Verify error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "release name is empty")
}

// TestRealHelmClient_GetChartFromRelease_EmptyReleaseName tests error handling when release name is empty
func TestRealHelmClient_GetChartFromRelease_EmptyReleaseName(t *testing.T) {
	// Create a real client
	client := NewRealHelmClient(cli.New())

	// Call the method with empty release name
	_, err := client.GetChartFromRelease(context.Background(), "", "test-namespace")

	// Verify error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "release name is empty")
}
