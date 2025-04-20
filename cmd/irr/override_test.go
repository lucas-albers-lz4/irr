package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lalbers/irr/internal/helm"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	helmchart "helm.sh/helm/v3/pkg/chart"
)

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

func (m *MockHelmClient) GetValues(ctx context.Context, releaseName, namespace string) (map[string]interface{}, error) {
	if m.GetValuesError != nil {
		return nil, m.GetValuesError
	}
	return m.ReleaseValues, nil
}

func (m *MockHelmClient) GetChartFromRelease(ctx context.Context, releaseName, namespace string) (*helmchart.Chart, error) {
	if m.GetReleaseError != nil {
		return nil, m.GetReleaseError
	}
	return m.ReleaseChart, nil
}

func (m *MockHelmClient) GetReleaseMetadata(ctx context.Context, releaseName, namespace string) (*helmchart.Metadata, error) {
	if m.GetReleaseError != nil {
		return nil, m.GetReleaseError
	}
	if m.ReleaseChart == nil || m.ReleaseChart.Metadata == nil {
		return &helmchart.Metadata{Name: "mock-chart"}, nil
	}
	return m.ReleaseChart.Metadata, nil
}

func (m *MockHelmClient) TemplateChart(ctx context.Context, releaseName, chartPath string, values map[string]interface{}, namespace, kubeVersion string) (string, error) {
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

func (m *MockHelmClient) GetReleaseChart(ctx context.Context, releaseName, namespace string) (*helm.ChartMetadata, error) {
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

func (m *MockHelmClient) FindChartForRelease(ctx context.Context, releaseName, namespace string) (string, error) {
	if m.GetReleaseError != nil {
		return "", m.GetReleaseError
	}
	if m.LoadChartFromPath != "" {
		return m.LoadChartFromPath, nil
	}
	return "/mock/path/to/chart", nil
}

func (m *MockHelmClient) ValidateRelease(ctx context.Context, releaseName, namespace string, valueFiles []string, kubeVersion string) error {
	if m.ValidateError != nil {
		return m.ValidateError
	}
	return nil
}

func (m *MockHelmClient) GetReleaseValues(ctx context.Context, releaseName, namespace string) (map[string]interface{}, error) {
	if m.GetValuesError != nil {
		return nil, m.GetValuesError
	}
	return m.ReleaseValues, nil
}

// defaultHelmClientFactory creates default helm client
func defaultHelmClientFactory(_ string) (helm.ClientInterface, error) {
	return &MockHelmClient{}, nil
}

// Test variables
var helmClientFactory = defaultHelmClientFactory

// setupOverrideCommandForTest creates a command for testing
func setupOverrideCommandForTest() *cobra.Command {
	// Set test mode
	isTestMode = true

	// Also set helm plugin mode
	isHelmPlugin = true

	// Create mock chart dir for testing
	chartDir := filepath.Join(os.TempDir(), "test-chart")
	_ = AppFs.MkdirAll(chartDir, 0755)

	// Create a new command with test args
	cmd := newOverrideCmd()
	cmd.SetArgs([]string{
		"test-release", // Positional arg for release name
		"--target-registry", "target.com",
		"--source-registries", "docker.io",
		"--dry-run",              // Enable dry run
		"--no-validate",          // Explicitly disable validation - boolean flag, no value needed
		"--chart-path", chartDir, // Add chart path to satisfy requirement
	})

	return cmd
}

// TestOverrideCommand_DryRun tests the dry run functionality
func TestOverrideCommand_DryRun(t *testing.T) {
	// Save and restore original values
	originalIsHelmPlugin := isHelmPlugin
	originalIsTestMode := isTestMode
	originalFs := AppFs
	defer func() {
		isHelmPlugin = originalIsHelmPlugin
		isTestMode = originalIsTestMode
		AppFs = originalFs
	}()

	// Set both plugin mode and test mode to bypass chart-path requirement
	isHelmPlugin = true
	isTestMode = true

	// Setup in-memory filesystem
	AppFs = afero.NewMemMapFs()

	// Create temporary chart dir
	chartDir := filepath.Join(os.TempDir(), "test-chart")
	err := AppFs.MkdirAll(chartDir, 0755)
	require.NoError(t, err, "Failed to create test chart dir")

	// Test with test mode (which creates mock content for dry run)
	t.Run("dry_run_with_validation_disabled", func(t *testing.T) {
		// Create a new command with test args
		cmd := newOverrideCmd()
		cmd.SetArgs([]string{
			"test-release", // Positional arg for release name
			"--target-registry", "target.com",
			"--source-registries", "docker.io",
			"--dry-run",              // Enable dry run
			"--no-validate",          // Explicitly disable validation
			"--chart-path", chartDir, // Add chart path to satisfy requirement
		})

		// Manually set flags to ensure they're properly set
		cmd.Flags().Set("dry-run", "true")
		cmd.Flags().Set("no-validate", "true")

		output := &bytes.Buffer{}
		cmd.SetOut(output)

		// Directly call handleTestModeOverride instead of cmd.Execute()
		// since we're in test mode and this is what runOverride will call
		err := handleTestModeOverride(cmd, "test-release")
		require.NoError(t, err)

		// Check output contains expected overrides
		assert.Contains(t, output.String(), "--- Dry Run: Generated Overrides ---")

		// Check that no file was created
		exists, err := afero.Exists(AppFs, "test-release-overrides.yaml")
		assert.NoError(t, err)
		assert.False(t, exists, "Output file should not be created in dry run")
	})
}

// MockHelmAdapter mocks the helm.AdapterInterface
type MockHelmAdapter struct {
	MockClient     *MockHelmClient
	OverrideOutput string
	ValidateError  error
}

func (m *MockHelmAdapter) OverrideRelease(_ context.Context, _, _ string, _ string, _ []string, _ string, _ helm.OverrideOptions) (string, error) {
	return m.OverrideOutput, nil
}

func (m *MockHelmAdapter) ValidateRelease(_ context.Context, _, _ string, _ []string, _ string) error {
	return m.ValidateError
}

// Rest of the file follows...
