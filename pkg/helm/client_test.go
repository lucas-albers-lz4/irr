package helm

import (
	"context"
	"errors"
	"fmt"
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

// TestMockHelmClient_GetReleaseValues_Error tests the MockHelmClient's GetReleaseValues method with an error
func TestMockHelmClient_GetReleaseValues_Error(t *testing.T) {
	// Create a mock client that returns an error
	expectedErr := errors.New("mock get values error")
	client := &MockHelmClient{
		MockGetReleaseValues: func(_ context.Context, releaseName, namespace string) (map[string]interface{}, error) {
			// Verify input parameters
			assert.Equal(t, "error-release", releaseName)
			assert.Equal(t, "error-namespace", namespace)

			// Return the predefined error
			return nil, expectedErr
		},
	}

	// Call the method
	result, err := client.GetReleaseValues(context.Background(), "error-release", "error-namespace")

	// Verify result
	require.Error(t, err, "Expected an error from GetReleaseValues")
	assert.Nil(t, result, "Expected nil result on error")
	assert.Equal(t, expectedErr, err, "Returned error should match the expected mock error")
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
	// Create a mock client
	client := &MockHelmClient{
		MockTemplateChart: func(_ context.Context, chartPath, releaseName, namespace string, _ map[string]interface{}, kubeVersion string) (string, error) {
			// Verify inputs
			assert.Equal(t, "test-chart-path", chartPath)
			assert.Equal(t, "test-release", releaseName)
			assert.Equal(t, "test-namespace", namespace)
			assert.Equal(t, "1.21.0", kubeVersion)

			// Return a mock output
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
		"1.21.0",
	)

	// Verify result
	require.NoError(t, err)
	assert.Equal(t, "templated-yaml-content", result)
}

// TestMockHelmClient_TemplateChart_Error tests the MockHelmClient's TemplateChart method with an error
func TestMockHelmClient_TemplateChart_Error(t *testing.T) {
	// Create a mock client that returns an error
	expectedErr := errors.New("mock template chart error")
	client := &MockHelmClient{
		MockTemplateChart: func(_ context.Context, chartPath, releaseName, namespace string, _ map[string]interface{}, _ string) (string, error) {
			// Verify inputs (optional, can be skipped if covered by success test)
			assert.Equal(t, "error-chart-path", chartPath)
			assert.Equal(t, "error-release", releaseName)
			assert.Equal(t, "error-namespace", namespace)

			// Return the predefined error
			return "", expectedErr
		},
	}

	// Call the method
	result, err := client.TemplateChart(
		context.Background(),
		"error-chart-path",
		"error-release",
		"error-namespace",
		map[string]interface{}{"key": "val"}, // Values don't matter for error path
		"1.20.0",                             // KubeVersion doesn't matter for error path
	)

	// Verify result
	require.Error(t, err, "Expected an error from TemplateChart")
	assert.Empty(t, result, "Expected empty string result on error")
	assert.Equal(t, expectedErr, err, "Returned error should match the expected mock error")
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

	templateOutput, err := client.TemplateChart(context.Background(), "some-path", "some-release", "some-namespace", nil, "")
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

// NOTE: Testing the actual Run() failure of action.NewGetValues requires more complex mocking
// or integration tests, which might be beyond the scope of simple unit tests.
// The existing mock tests cover the interface contract, and the init error test covers
// a key error path within the RealHelmClient method itself.

func TestGetHelmSettings(t *testing.T) {
	t.Run("Should return non-nil settings", func(t *testing.T) {
		settings := GetHelmSettings()
		assert.NotNil(t, settings, "GetHelmSettings should return a non-nil *cli.EnvSettings")
		// We can also check a default field if it's predictable and important
		// For example, checking if the default Debug flag is false
		// assert.False(t, settings.Debug, "Default Debug setting should be false")
	})
}

// Helper function to test common error scenarios for mock client methods
func testMockClientErrorScenario[T any](t *testing.T, methodName string, setupMock func(*MockHelmClient, error), callMethod func(*MockHelmClient) (T, error), expectedErr error) {
	t.Helper()
	client := &MockHelmClient{}
	setupMock(client, expectedErr)

	result, err := callMethod(client)
	require.Error(t, err, fmt.Sprintf("Expected an error from %s", methodName))
	var zero T // Get the zero value for the type T
	assert.Equal(t, zero, result, fmt.Sprintf("Expected zero value result on error from %s", methodName))
	assert.Equal(t, expectedErr, err, fmt.Sprintf("Returned error from %s should match the expected mock error", methodName))
}

func TestMockHelmClient_ErrorScenarios(t *testing.T) {
	testCases := []struct {
		name           string
		releaseName    string
		namespace      string
		setupMock      func(*MockHelmClient, error, string, string)
		callMethod     func(*MockHelmClient, string, string) (interface{}, error)
		expectedErrMsg string
	}{
		{
			name:           "GetChartFromRelease error",
			releaseName:    "error-release",
			namespace:      "error-namespace",
			expectedErrMsg: "mock get chart error",
			setupMock: func(client *MockHelmClient, err error, releaseName, namespace string) {
				client.MockGetChartFromRelease = func(_ context.Context, rn, ns string) (*chart.Chart, error) {
					assert.Equal(t, releaseName, rn)
					assert.Equal(t, namespace, ns)
					return nil, err
				}
			},
			callMethod: func(client *MockHelmClient, releaseName, namespace string) (interface{}, error) {
				return client.GetChartFromRelease(context.Background(), releaseName, namespace)
			},
		},
		{
			name:           "GetReleaseMetadata error",
			releaseName:    "error-release",
			namespace:      "error-namespace",
			expectedErrMsg: "mock get metadata error",
			setupMock: func(client *MockHelmClient, err error, releaseName, namespace string) {
				client.MockGetReleaseMetadata = func(_ context.Context, rn, ns string) (*chart.Metadata, error) {
					assert.Equal(t, releaseName, rn)
					assert.Equal(t, namespace, ns)
					return nil, err
				}
			},
			callMethod: func(client *MockHelmClient, releaseName, namespace string) (interface{}, error) {
				return client.GetReleaseMetadata(context.Background(), releaseName, namespace)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expectedErr := errors.New(tc.expectedErrMsg)

			// Create a new client for each test case
			client := NewMockHelmClient()

			// Setup the mock with the error
			tc.setupMock(client, expectedErr, tc.releaseName, tc.namespace)

			// Call the method and check the error
			_, err := tc.callMethod(client, tc.releaseName, tc.namespace)

			// Verify the error
			assert.Error(t, err)
			assert.Equal(t, expectedErr, err)
		})
	}
}
