package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinimalChart(t *testing.T) {
	// t.Skip("Temporarily disabled")
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	harness.SetupChart(testutil.GetChartPath("minimal-test"))
	harness.SetRegistries("target.io", []string{"source.io", "docker.io"})

	if err := harness.GenerateOverrides(); err != nil {
		t.Fatalf("Failed to generate overrides: %v", err)
	}

	if err := harness.ValidateOverrides(); err != nil {
		t.Fatalf("Failed to validate overrides: %v", err)
	}
}

func TestParentChart(t *testing.T) {
	// t.Skip("Temporarily disabled")
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	harness.SetupChart(testutil.GetChartPath("parent-test"))
	harness.SetRegistries("target.io", []string{"source.io", "docker.io"})

	if err := harness.GenerateOverrides(); err != nil {
		t.Fatalf("Failed to generate overrides: %v", err)
	}

	if err := harness.ValidateOverrides(); err != nil {
		t.Fatalf("Failed to validate overrides: %v", err)
	}
}

func TestKubePrometheusStack(t *testing.T) {
	// t.Skip("Temporarily disabled")
	// t.Skip("Skipping test: kube-prometheus-stack chart not available in test-data/charts")
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	harness.SetupChart(testutil.GetChartPath("kube-prometheus-stack"))
	harness.SetRegistries("target.io", []string{"quay.io", "registry.k8s.io"})

	if err := harness.GenerateOverrides(); err != nil {
		t.Fatalf("Failed to generate overrides: %v", err)
	}

	if err := harness.ValidateOverrides(); err != nil {
		t.Fatalf("Failed to validate overrides: %v", err)
	}
}

func TestCertManagerIntegration(t *testing.T) {
	// t.Skip("Temporarily disabled")
	// Certificate manager is available as cert-manager in test-data/charts
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	harness.SetupChart(testutil.GetChartPath("cert-manager"))
	harness.SetRegistries("harbor.home.arpa", []string{"quay.io", "docker.io"})

	if err := harness.GenerateOverrides(); err != nil {
		t.Fatalf("Failed to generate overrides: %v", err)
	}

	if err := harness.ValidateOverrides(); err != nil {
		t.Fatalf("Failed to validate overrides: %v", err)
	}
}

func TestKubePrometheusStackIntegration(t *testing.T) {
	// t.Skip("Temporarily disabled")
	// t.Skip("Skipping test: kube-prometheus-stack chart not available in test-data/charts")
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	harness.SetupChart(testutil.GetChartPath("kube-prometheus-stack"))
	harness.SetRegistries("harbor.home.arpa", []string{"quay.io", "docker.io", "registry.k8s.io"})

	if err := harness.GenerateOverrides(); err != nil {
		t.Fatalf("Failed to generate overrides: %v", err)
	}

	if err := harness.ValidateOverrides(); err != nil {
		t.Fatalf("Failed to validate overrides: %v", err)
	}
}

func TestIngressNginxIntegration(t *testing.T) {
	// t.Skip("Temporarily disabled")
	// t.Skip("Skipping test: ingress-nginx chart not available in test-data/charts")
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	harness.SetupChart(testutil.GetChartPath("ingress-nginx"))
	harness.SetRegistries("harbor.home.arpa", []string{"registry.k8s.io", "docker.io"})

	if err := harness.GenerateOverrides(); err != nil {
		t.Fatalf("Failed to generate overrides: %v", err)
	}

	if err := harness.ValidateOverrides(); err != nil {
		t.Fatalf("Failed to validate overrides: %v", err)
	}
}

func TestComplexChartFeatures(t *testing.T) {
	// TODO: Add more focused integration tests for complex subchart scenarios.
	// This test uses large, real-world charts which can be brittle.
	// Consider creating specific, smaller test cases for subchart value propagation,
	// default value handling, and other complex Helm features.

	// t.Skip("Temporarily disabled")
	tests := []struct {
		name           string
		chartName      string
		sourceRegs     []string
		expectedImages []string
		skip           bool
		skipReason     string
	}{
		{
			name:      "cert-manager with webhook and cainjector",
			chartName: "cert-manager",
			sourceRegs: []string{
				"quay.io",
				"docker.io",
			},
			expectedImages: []string{
				"quay.io/jetstack/cert-manager-controller",
				"quay.io/jetstack/cert-manager-webhook",
				"quay.io/jetstack/cert-manager-cainjector",
			},
			skip:       false,
			skipReason: "",
		},
		{
			name:      "simplified-prometheus-stack with specific components",
			chartName: "simplified-prometheus-stack",
			sourceRegs: []string{
				"quay.io",
				"docker.io",
				"registry.k8s.io",
			},
			expectedImages: []string{
				"quay.io/prometheus/prometheus",
				// "quay.io/prometheus/alertmanager", // Not used in minimal template
				// "quay.io/prometheus/node-exporter", // Not used in minimal template
				// "registry.k8s.io/kube-state-metrics/kube-state-metrics", // Not used in minimal template
				// "docker.io/grafana/grafana", // Not used in minimal template
			},
			skip: false,
			// skipReason: "kube-prometheus-stack chart not available in test-data/charts",
		},
		{
			name:      "ingress-nginx with admission webhook",
			chartName: "ingress-nginx",
			sourceRegs: []string{
				"registry.k8s.io",
				"docker.io",
			},
			expectedImages: []string{
				"registry.k8s.io/ingress-nginx/controller",
				"registry.k8s.io/ingress-nginx/kube-webhook-certgen",
			},
			skip: false,
			// skipReason: "ingress-nginx chart not available in test-data/charts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip(tt.skipReason)
			}

			harness := NewTestHarness(t)
			defer harness.Cleanup()

			harness.SetupChart(testutil.GetChartPath(tt.chartName))
			harness.SetRegistries("harbor.home.arpa", tt.sourceRegs)

			if err := harness.GenerateOverrides(); err != nil {
				t.Fatalf("Failed to generate overrides: %v", err)
			}

			if err := harness.ValidateOverrides(); err != nil {
				t.Fatalf("Failed to validate overrides: %v", err)
			}

			// Verify specific images are properly handled
			overrides, err := harness.GetOverrides()
			if err != nil {
				t.Fatalf("Failed to get overrides: %v", err)
			}

			for _, expectedImage := range tt.expectedImages {
				found := false
				harness.WalkImageFields(overrides, func(imagePath []string, imageValue string) {
					if strings.Contains(imageValue, strings.TrimPrefix(expectedImage, tt.sourceRegs[0]+"/")) {
						found = true
					}
				})
				if !found {
					t.Errorf("Expected image %s not found in overrides", expectedImage)
				}
			}
		})
	}
}

func TestDryRunFlag(t *testing.T) {
	// t.Skip("Temporarily disabled")
	// t.Skip("Skipping test: Requires binary to be built with \'make build\' first")
	// return
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Setup minimal test chart
	setupMinimalTestChart(t, harness)

	// Set the --dry-run flag
	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", "harbor.example.com",
		"--source-registries", "docker.io",
		"--dry-run",
	}

	// #nosec G204 // Test command uses test-controlled arguments
	cmd := exec.Command("../../bin/irr", args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Dry run should succeed")

	// Verify no file was created
	_, err = os.Stat(filepath.Join(harness.tempDir, "overrides.yaml"))
	assert.True(t, os.IsNotExist(err), "No override file should be created in dry-run mode")

	// Verify output contains preview
	assert.Contains(t, string(output), "repository:", "Dry run output should contain override preview")
}

func TestStrictMode(t *testing.T) {
	// t.Skip("Temporarily disabled")
	// t.Skip("Skipping test: Requires binary to be built with \'make build\' first")
	// return
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Setup chart with unsupported structure
	setupChartWithUnsupportedStructure(t, harness)

	// Test without --strict
	err := harness.GenerateOverrides()
	assert.NoError(t, err, "Should succeed with warning without --strict")

	// Test with --strict
	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", "harbor.example.com",
		"--source-registries", "docker.io",
		"--strict",
	}

	// #nosec G204 -- Test command uses test-controlled arguments
	cmd := exec.Command("../../bin/irr", args...)
	_, err = cmd.CombinedOutput()
	assert.Error(t, err, "Should fail in strict mode")
}

// Helper functions

func setupMinimalTestChart(t *testing.T, h *TestHarness) {
	chartDir := filepath.Join(h.tempDir, "minimal-chart")
	require.NoError(t, os.MkdirAll(chartDir, 0750))

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: minimal-chart
version: 0.1.0`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0644))

	// Create values.yaml
	valuesYaml := `image:
  repository: nginx
  tag: 1.23`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), 0644))

	h.chartPath = chartDir
}

func setupChartWithUnsupportedStructure(t *testing.T, h *TestHarness) {
	t.Helper()
	// t.Skip("Temporarily disabled - Need to create unsupported-test chart")
	chartPath := testutil.GetChartPath("unsupported-test")
	err := os.MkdirAll(filepath.Join(h.tempDir, chartPath), 0755)
	require.NoError(t, err, "Failed to create unsupported-test chart directory")

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: unsupported-test
version: 0.1.0`
	require.NoError(t, os.WriteFile(filepath.Join(h.tempDir, chartPath, "Chart.yaml"), []byte(chartYaml), 0644))

	// Create values.yaml with unsupported structure
	valuesYaml := `image:
  name: nginx
  version: 1.23  # Using 'version' instead of 'tag'`
	require.NoError(t, os.WriteFile(filepath.Join(h.tempDir, chartPath, "values.yaml"), []byte(valuesYaml), 0644))

	h.chartPath = filepath.Join(h.tempDir, chartPath)
}

// nolint:unused // Kept for potential future uses
func chartExists(name string) bool {
	// Check if chart exists in test-data/charts
	_, err := os.Stat(filepath.Join("test-data", "charts", name))
	return err == nil
}
