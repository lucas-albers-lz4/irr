//go:build test

// This file contains test helpers that are only used during testing

package main

import (
	"bytes"
	"context"

	"github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/spf13/cobra"
	helmchart "helm.sh/helm/v3/pkg/chart"
)

// Variable to store the original helmAdapterFactory during testing
var originalHelmAdapterFactory func() (*helm.Adapter, error)

// TestAnalyzeMode is a global flag to enable test mode
var TestAnalyzeMode bool

// MockHelmClient implements the helm.ClientInterface for testing
type MockHelmClient struct {
	ReleaseValues     map[string]interface{}
	ReleaseChart      *helmchart.Chart
	ReleaseNamespace  string
	TemplateOutput    string
	TemplateError     error
	GetValuesError    error
	GetReleaseError   error
	ValidateError     error
	LoadChartFromPath string
	LoadChartError    error
}

func (m *MockHelmClient) GetValues(_ context.Context, _, _ string) (map[string]interface{}, error) {
	if m.GetValuesError != nil {
		return nil, m.GetValuesError
	}
	return m.ReleaseValues, nil
}

func (m *MockHelmClient) GetChartFromRelease(_ context.Context, _, _ string) (*helm.ChartMetadata, error) {
	if m.GetReleaseError != nil {
		return nil, m.GetReleaseError
	}

	// Convert Chart to ChartMetadata
	var meta *helm.ChartMetadata
	if m.ReleaseChart != nil && m.ReleaseChart.Metadata != nil {
		meta = &helm.ChartMetadata{
			Name:    m.ReleaseChart.Metadata.Name,
			Version: m.ReleaseChart.Metadata.Version,
			Path:    m.LoadChartFromPath,
		}

		// Extract repository if available
		if m.ReleaseChart.Metadata.Sources != nil && len(m.ReleaseChart.Metadata.Sources) > 0 {
			meta.Repository = m.ReleaseChart.Metadata.Sources[0]
		}
	} else {
		// Default metadata if none is available
		meta = &helm.ChartMetadata{
			Name:    "mock-chart",
			Version: "1.0.0",
			Path:    m.LoadChartFromPath,
		}
	}

	return meta, nil
}

func (m *MockHelmClient) GetReleaseMetadata(_ context.Context, _, _ string) (*helmchart.Metadata, error) {
	if m.GetReleaseError != nil {
		return nil, m.GetReleaseError
	}
	if m.ReleaseChart == nil || m.ReleaseChart.Metadata == nil {
		return &helmchart.Metadata{Name: "mock-chart"}, nil
	}
	return m.ReleaseChart.Metadata, nil
}

func (m *MockHelmClient) TemplateChart(_ context.Context, releaseName, namespace, chartPath string, values map[string]interface{}) (string, error) {
	if m.TemplateError != nil {
		return "", m.TemplateError
	}
	return m.TemplateOutput, nil
}

func (m *MockHelmClient) GetHelmSettings() (map[string]string, error) {
	return map[string]string{}, nil
}

func (m *MockHelmClient) GetCurrentNamespace() string {
	return m.ReleaseNamespace
}

func (m *MockHelmClient) GetReleaseChart(_ context.Context, _, _ string) (*helm.ChartMetadata, error) {
	if m.GetReleaseError != nil {
		return nil, m.GetReleaseError
	}
	return &helm.ChartMetadata{
		Name:       "mock-chart",
		Version:    "1.0.0",
		Repository: "https://charts.example.com",
		Path:       m.LoadChartFromPath,
	}, nil
}

func (m *MockHelmClient) FindChartForRelease(_ context.Context, _, _ string) (string, error) {
	if m.GetReleaseError != nil {
		return "", m.GetReleaseError
	}
	if m.LoadChartFromPath != "" {
		return m.LoadChartFromPath, nil
	}
	return "/mock/path/to/chart", nil
}

func (m *MockHelmClient) ValidateRelease(_ context.Context, _, _ string, _ []string, _ string) error {
	if m.ValidateError != nil {
		return m.ValidateError
	}
	return nil
}

func (m *MockHelmClient) GetReleaseValues(_ context.Context, _, _ string) (map[string]interface{}, error) {
	if m.GetValuesError != nil {
		return nil, m.GetValuesError
	}
	return m.ReleaseValues, nil
}

func (m *MockHelmClient) LoadChart(chartPath string) (*helmchart.Chart, error) {
	if m.LoadChartError != nil {
		return nil, m.LoadChartError
	}
	return m.ReleaseChart, nil
}

// ListReleases implements helm.ClientInterface and returns an empty list of releases
func (m *MockHelmClient) ListReleases(_ context.Context, _ bool) ([]*helm.ReleaseElement, error) {
	// For simplicity in tests, return an empty list or could be enhanced to return mock releases
	return []*helm.ReleaseElement{}, nil
}

// executeCommand is a helper function for testing Cobra commands
func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)

	// Use a clean argument slice for each test
	root.SetArgs(args)

	err = root.Execute()
	return buf.String(), err
}

// getRootCmd returns the root command for testing purposes
func getRootCmd() *cobra.Command {
	return newRootCmd()
}
