// Package helm provides utilities for interacting with Helm charts and releases.
package helm

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"

	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
)

// ClientInterface defines the interface for interacting with Helm.
// This interface allows for real and mock implementations to be used interchangeably.
type ClientInterface interface {
	// GetReleaseValues retrieves the values from a Helm release.
	GetReleaseValues(_ context.Context, releaseName, namespace string) (map[string]interface{}, error)

	// GetChartFromRelease gets the chart from a Helm release.
	GetChartFromRelease(_ context.Context, releaseName, namespace string) (*chart.Chart, error)

	// GetReleaseMetadata gets the metadata from a Helm release.
	GetReleaseMetadata(_ context.Context, releaseName, namespace string) (*chart.Metadata, error)

	// TemplateChart templates a chart with the provided values.
	TemplateChart(_ context.Context, chartPath, releaseName, namespace string, values map[string]interface{}, kubeVersion string) (string, error)
}

// RealHelmClient implements ClientInterface using the real Helm SDK.
type RealHelmClient struct {
	settings *cli.EnvSettings
}

// NewRealHelmClient creates a new RealHelmClient with the provided settings.
func NewRealHelmClient(settings *cli.EnvSettings) *RealHelmClient {
	if settings == nil {
		settings = cli.New()
	}
	return &RealHelmClient{
		settings: settings,
	}
}

// GetReleaseValues implements ClientInterface.
func (c *RealHelmClient) GetReleaseValues(_ context.Context, releaseName, namespace string) (map[string]interface{}, error) {
	if releaseName == "" {
		return nil, fmt.Errorf("release name is empty")
	}

	// Create action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(c.settings.RESTClientGetter(), namespace, "", func(string, ...interface{}) {}); err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  fmt.Errorf("failed to initialize Helm action config: %w", err),
		}
	}

	// Create get values action
	getValues := action.NewGetValues(actionConfig)
	getValues.AllValues = true

	// Get the values
	vals, err := getValues.Run(releaseName)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  fmt.Errorf("failed to get values for release %s: %w", releaseName, err),
		}
	}

	return vals, nil
}

// GetChartFromRelease implements ClientInterface.
func (c *RealHelmClient) GetChartFromRelease(_ context.Context, releaseName, namespace string) (*chart.Chart, error) {
	if releaseName == "" {
		return nil, fmt.Errorf("release name is empty")
	}

	// Create action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(c.settings.RESTClientGetter(), namespace, "", func(string, ...interface{}) {}); err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  fmt.Errorf("failed to initialize Helm action config: %w", err),
		}
	}

	// Create get action
	get := action.NewGet(actionConfig)

	// Get the release
	rel, err := get.Run(releaseName)
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  fmt.Errorf("failed to get release %s: %w", releaseName, err),
		}
	}

	return rel.Chart, nil
}

// GetReleaseMetadata implements ClientInterface.
func (c *RealHelmClient) GetReleaseMetadata(_ context.Context, releaseName, namespace string) (*chart.Metadata, error) {
	chartObj, err := c.GetChartFromRelease(context.Background(), releaseName, namespace)
	if err != nil {
		return nil, err
	}

	return chartObj.Metadata, nil
}

// TemplateChart implements ClientInterface.
func (c *RealHelmClient) TemplateChart(_ context.Context, chartPath, releaseName, namespace string, values map[string]interface{}, kubeVersion string) (string, error) {
	// Create action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(c.settings.RESTClientGetter(), namespace, "", func(string, ...interface{}) {}); err != nil {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmInteractionError,
			Err:  fmt.Errorf("failed to initialize Helm action config: %w", err),
		}
	}

	// Create template action
	template := action.NewInstall(actionConfig)
	template.DryRun = true // Don't install the chart, just render templates
	template.ReleaseName = releaseName
	template.Namespace = namespace
	template.Replace = true      // Skip the name check
	template.ClientOnly = true   // Don't contact Kubernetes
	template.IncludeCRDs = false // Skip rendering CRDs to avoid warnings

	// Set Kubernetes version if specified
	if kubeVersion != "" {
		template.KubeVersion = &chartutil.KubeVersion{Version: kubeVersion}
	}

	// Load the chart
	chartPathResolved, err := template.LocateChart(chartPath, c.settings)
	if err != nil {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed,
			Err:  fmt.Errorf("failed to locate chart at path %s: %w", chartPath, err),
		}
	}

	// Load chart into memory
	loadedChart, err := loader.Load(chartPathResolved)
	if err != nil {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartLoadFailed,
			Err:  fmt.Errorf("failed to load chart from %s: %w", chartPath, err),
		}
	}

	// Create the release
	rel, err := template.Run(loadedChart, values)
	if err != nil {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmTemplateFailed,
			Err:  fmt.Errorf("failed to template chart %s: %w", chartPath, err),
		}
	}

	// Return the templates
	return rel.Manifest, nil
}

// MockHelmClient implements ClientInterface for testing.
type MockHelmClient struct {
	MockGetReleaseValues    func(_ context.Context, releaseName, namespace string) (map[string]interface{}, error)
	MockGetChartFromRelease func(_ context.Context, releaseName, namespace string) (*chart.Chart, error)
	MockGetReleaseMetadata  func(_ context.Context, releaseName, namespace string) (*chart.Metadata, error)
	MockTemplateChart       func(_ context.Context, chartPath, releaseName, namespace string, values map[string]interface{}, kubeVersion string) (string, error)
}

// NewMockHelmClient creates a new MockHelmClient with default mock functions.
func NewMockHelmClient() *MockHelmClient {
	return &MockHelmClient{
		MockGetReleaseValues: func(_ context.Context, _, _ string) (map[string]interface{}, error) {
			return map[string]interface{}{}, nil
		},
		MockGetChartFromRelease: func(_ context.Context, _, _ string) (*chart.Chart, error) {
			return &chart.Chart{
				Metadata: &chart.Metadata{
					Name:    "mock-chart",
					Version: "1.0.0",
				},
			}, nil
		},
		MockGetReleaseMetadata: func(_ context.Context, _, _ string) (*chart.Metadata, error) {
			return &chart.Metadata{
				Name:    "mock-chart",
				Version: "1.0.0",
			}, nil
		},
		MockTemplateChart: func(_ context.Context, _, _, _ string, _ map[string]interface{}, _ string) (string, error) {
			return "mock-template-output", nil
		},
	}
}

// GetReleaseValues implements ClientInterface.
func (m *MockHelmClient) GetReleaseValues(_ context.Context, releaseName, namespace string) (map[string]interface{}, error) {
	return m.MockGetReleaseValues(nil, releaseName, namespace)
}

// GetChartFromRelease implements ClientInterface.
func (m *MockHelmClient) GetChartFromRelease(_ context.Context, releaseName, namespace string) (*chart.Chart, error) {
	return m.MockGetChartFromRelease(nil, releaseName, namespace)
}

// GetReleaseMetadata implements ClientInterface.
func (m *MockHelmClient) GetReleaseMetadata(_ context.Context, releaseName, namespace string) (*chart.Metadata, error) {
	return m.MockGetReleaseMetadata(nil, releaseName, namespace)
}

// TemplateChart implements ClientInterface.
func (m *MockHelmClient) TemplateChart(_ context.Context, chartPath, releaseName, namespace string, values map[string]interface{}, kubeVersion string) (string, error) {
	return m.MockTemplateChart(nil, chartPath, releaseName, namespace, values, kubeVersion)
}

// GetHelmSettings returns the Helm CLI settings
func GetHelmSettings() *cli.EnvSettings {
	settings := cli.New()
	return settings
}
