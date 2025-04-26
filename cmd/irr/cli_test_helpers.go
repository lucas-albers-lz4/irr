//go:build test
// +build test

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

func (m *MockHelmClient) GetChartFromRelease(_ context.Context, _, _ string) (*helmchart.Chart, error) {
	if m.GetReleaseError != nil {
		return nil, m.GetReleaseError
	}
	return m.ReleaseChart, nil
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

func (m *MockHelmClient) TemplateChart(_ context.Context, _, _ string, _ map[string]interface{}, _, _ string) (string, error) {
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
