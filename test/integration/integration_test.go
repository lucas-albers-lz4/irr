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
	"gopkg.in/yaml.v3"
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
			skip:       true,
			skipReason: "cert-manager chart has unique image structure that requires additional handling",
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
				// Images hardcoded in templates, not values.yaml
				// "registry.k8s.io/ingress-nginx/controller",
				// "registry.k8s.io/ingress-nginx/kube-webhook-certgen",
				// We should still expect docker.io images to be processed if present
				"docker.io/bitnami/nginx",
				"docker.io/bitnami/git",            // From cloneStaticSiteFromGit.image
				"docker.io/bitnami/nginx-exporter", // From metrics.image
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

			// --- Use ExecuteIRR instead of GenerateOverrides ---
			args := []string{
				"override",
				"--chart-path", harness.chartPath,
				"--target-registry", harness.targetReg,
				"--source-registries", strings.Join(harness.sourceRegs, ","),
				"--output-file", harness.overridePath,
				// Add mappings path if set in harness (though not used in these specific tests yet)
			}
			if harness.mappingsPath != "" {
				// Ensure absolute path for mappings if provided
				absMappingsPath, absErr := filepath.Abs(harness.mappingsPath)
				if absErr != nil {
					t.Fatalf("Failed to get absolute path for mappings file %s: %v", harness.mappingsPath, absErr)
				}
				args = append(args, "--registry-mappings", absMappingsPath)
			}
			args = append(args, "--debug") // Ensure debug is enabled

			if tt.chartName == "ingress-nginx" {
				// Special handling for ingress-nginx with explicit output file
				explicitOutputFile := filepath.Join(harness.tempDir, "explicit-ingress-nginx-overrides.yaml")
				explicitArgs := append(args, "--output-file", explicitOutputFile)

				// Execute IRR specifically for ingress-nginx
				explicitOutput, err := harness.ExecuteIRR(explicitArgs...)
				require.NoError(t, err, "Explicit ExecuteIRR failed for ingress-nginx. Output:\n%s", explicitOutput)

				// Load the overrides generated specifically for this subtest
				// #nosec G304 -- Test reads file path constructed from test data
				overridesBytes, err := os.ReadFile(explicitOutputFile)
				require.NoError(t, err, "Failed to read explicit output file: %s", explicitOutputFile)
				require.NotEmpty(t, overridesBytes, "Explicit output file should not be empty")

				explicitOverrides := make(map[string]interface{})
				err = yaml.Unmarshal(overridesBytes, &explicitOverrides)
				require.NoError(t, err, "Failed to unmarshal explicit overrides YAML for ingress-nginx")

				// Assert against the explicitOverrides
				for _, expectedImage := range tt.expectedImages {
					// Modify assertion slightly: check if rewritten repo path exists
					expectedRepo := ""
					if strings.HasPrefix(expectedImage, "docker.io/") {
						// Handle potential library prefix addition
						imgPart := strings.TrimPrefix(expectedImage, "docker.io/")
						if !strings.Contains(imgPart, "/") {
							imgPart = "library/" + imgPart // Assume library if no org
						}
						expectedRepo = "dockerio/" + imgPart
					} else if strings.HasPrefix(expectedImage, "registry.k8s.io/") {
						expectedRepo = "registryk8sio/" + strings.TrimPrefix(expectedImage, "registry.k8s.io/")
					} else if strings.HasPrefix(expectedImage, "quay.io/") {
						expectedRepo = "quayio/" + strings.TrimPrefix(expectedImage, "quay.io/")
					}
					if expectedRepo == "" {
						t.Errorf("Could not determine expected rewritten repo path for: %s", expectedImage)
						continue
					}
					found := false
					harness.WalkImageFields(explicitOverrides, func(imagePath []string, imageValue interface{}) {
						if found {
							return
						} // Short circuit if already found
						if repoStr, ok := imageValue.(string); ok {
							if strings.Contains(repoStr, expectedRepo) {
								t.Logf("[DEBUG FOUND STRING] Path: %v, Value: %s, ExpectedRepo: %s", imagePath, repoStr, expectedRepo)
								found = true
							}
						} else if imgMap, ok := imageValue.(map[string]interface{}); ok {
							if repo, repoOk := imgMap["repository"].(string); repoOk {
								if strings.Contains(repo, expectedRepo) {
									t.Logf("[DEBUG FOUND MAP] Path: %v, Repo: %s, ExpectedRepo: %s", imagePath, repo, expectedRepo)
									found = true
								}
							}
						}
					})
					if !found {
						t.Errorf("Expected image %s (looking for repo containing '%s') not found in explicit overrides for ingress-nginx", expectedImage, expectedRepo)
						t.Logf("Explicit Overrides content:\n%s", string(overridesBytes))
					}
				}
				// Skip the generic execution and validation for this specific test case
				return
			}

			// Generic execution for other test cases (outside the ingress-nginx specific block)
			// Note: The os.ReadFile check for explicitOutputFile was moved inside the ingress-nginx block above
			output, err := harness.ExecuteIRR(args...)
			if err != nil {
				t.Fatalf("Failed to execute irr override command: %v\nOutput:\n%s", err, output)
			}
			// --- End ExecuteIRR change ---

			if err := harness.ValidateOverrides(); err != nil {
				t.Fatalf("Failed to validate overrides: %v", err)
			}

			// Verify specific images are properly handled
			overrides, err := harness.GetOverrides()
			if err != nil {
				t.Fatalf("Failed to get overrides: %v", err)
			}

			for _, expectedImage := range tt.expectedImages {
				// Modify assertion slightly: check if rewritten repo path exists
				expectedRepo := ""
				if strings.HasPrefix(expectedImage, "docker.io/") {
					// Handle potential library prefix addition
					imgPart := strings.TrimPrefix(expectedImage, "docker.io/")
					if !strings.Contains(imgPart, "/") {
						imgPart = "library/" + imgPart // Assume library if no org
					}
					expectedRepo = "dockerio/" + imgPart
				} else if strings.HasPrefix(expectedImage, "registry.k8s.io/") {
					expectedRepo = "registryk8sio/" + strings.TrimPrefix(expectedImage, "registry.k8s.io/")
				} else if strings.HasPrefix(expectedImage, "quay.io/") {
					expectedRepo = "quayio/" + strings.TrimPrefix(expectedImage, "quay.io/")
				}

				if expectedRepo == "" {
					t.Errorf("Could not determine expected rewritten repo path for: %s", expectedImage)
					continue
				}

				found := false
				harness.WalkImageFields(overrides, func(imagePath []string, imageValue interface{}) {
					// Check if the repository value in a map matches, or if a string value matches
					if repoStr, ok := imageValue.(string); ok {
						if strings.Contains(repoStr, expectedRepo) {
							found = true
						}
					} else if imgMap, ok := imageValue.(map[string]interface{}); ok {
						if repo, repoOk := imgMap["repository"].(string); repoOk {
							if strings.Contains(repo, expectedRepo) {
								found = true
							}
						}
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

	// <<< ADDED: Set registries for the harness call >>>
	harness.SetRegistries("harbor.dummy.com", []string{"docker.io"}) // Values don't really matter for this test

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

// --- ADDING NEW TEST ---
func TestRegistryMappingFile(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// 1. Create a temporary mapping file
	mappingContent := `
docker.io: quay.io/instrumenta
k8s.gcr.io: quay.io/instrumenta
registry.k8s.io: quay.io/instrumenta
`
	mappingFilePath := filepath.Join(harness.tempDir, "test-mappings.yaml")
	// G306: Use secure file permissions (0600)
	err := os.WriteFile(mappingFilePath, []byte(mappingContent), 0600)
	require.NoError(t, err, "Failed to write temp mapping file")

	// 2. Setup chart (using minimal-test which has docker.io and quay.io images)
	harness.SetupChart(testutil.GetChartPath("minimal-test"))
	// Target registry doesn't matter much here, focus is on the prefix mapping
	harness.SetRegistries("target.registry.com", []string{"docker.io", "quay.io"})

	// 3. Execute override command with the mapping file flag
	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", harness.targetReg, // Use harness target registry
		"--source-registries", strings.Join(harness.sourceRegs, ","),
		"--registry-mappings", mappingFilePath, // Point to our custom file
		"--output-file", harness.overridePath, // Use harness output path
	}

	output, err := harness.ExecuteIRR(args...)
	require.NoError(t, err, "irr command with mapping file failed. Output: %s", output)

	// 4. Validate the generated overrides
	overrides, err := harness.GetOverrides()
	require.NoError(t, err, "Failed to read/parse generated overrides file")

	// Check docker.io image (should use 'dckr' prefix)
	dockerImageValue, ok := overrides["image"].(map[string]interface{})["repository"].(string)
	assert.True(t, ok, "Failed to find repository for image [image]")
	assert.Equal(t, "dckr/library/nginx", dockerImageValue, "docker.io image should use 'dckr' prefix from mapping file")

	// Check quay.io image (should use 'quaycustom' prefix)
	quayImageValue, ok := overrides["quayImage"].(map[string]interface{})["image"].(map[string]interface{})["repository"].(string)
	assert.True(t, ok, "Failed to find repository for image [quayImage][image]")
	assert.Equal(t, "quaycustom/prometheus/node-exporter", quayImageValue, "quay.io image should use 'quaycustom' prefix from mapping file")
}

// --- END NEW TEST ---

// +++ ADDING TEST FOR MINIMAL GIT IMAGE STRUCTURE +++
func TestMinimalGitImageOverride(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Setup chart
	harness.SetupChart(testutil.GetChartPath("minimal-git-image"))
	harness.SetRegistries("harbor.test.local", []string{"docker.io"})

	// Execute override command
	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", harness.targetReg,
		"--source-registries", strings.Join(harness.sourceRegs, ","),
		"--output-file", harness.overridePath,
		"--debug",
	}
	output, err := harness.ExecuteIRR(args...)
	require.NoError(t, err, "irr override failed for minimal-git-image chart. Output: %s", output)

	// Validate the generated overrides
	overrides, err := harness.GetOverrides()
	require.NoError(t, err, "Failed to read/parse generated overrides file")

	// Check the specific image override
	expectedRepo := "dockerio/bitnami/git"
	found := false
	harness.WalkImageFields(overrides, func(imagePath []string, imageValue interface{}) {
		if found {
			return
		}
		if imgMap, ok := imageValue.(map[string]interface{}); ok {
			if repo, repoOk := imgMap["repository"].(string); repoOk {
				if repo == expectedRepo {
					t.Logf("Found expected repo '%s' at path %v", expectedRepo, imagePath)
					found = true
				}
			}
		}
	})

	if !found {
		// errcheck: Check error from ReadFile before logging content
		overrideBytes, readErr := os.ReadFile(harness.overridePath)
		t.Errorf("Expected image repository '%s' not found in overrides", expectedRepo)
		if readErr != nil {
			t.Logf("Additionally, failed to read overrides file %s for debugging: %v", harness.overridePath, readErr)
		} else {
			t.Logf("Overrides content:\n%s", string(overrideBytes))
		}
	}
}

// +++ END TEST FOR MINIMAL GIT IMAGE STRUCTURE +++

// Helper functions

func setupMinimalTestChart(t *testing.T, h *TestHarness) {
	// G301: Use secure directory permissions (0750 or less)
	chartDir := filepath.Join(h.tempDir, "minimal-chart")
	require.NoError(t, os.MkdirAll(chartDir, 0750))

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: minimal-chart
version: 0.1.0`
	// G306: Use secure file permissions (0600)
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0600))

	// Create values.yaml
	valuesYaml := `image:
  repository: nginx
  tag: "1.23"`
	// G306: Use secure file permissions (0600)
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), 0600))

	h.chartPath = chartDir
}

// func setupChartWithUnsupportedStructure(t *testing.T, h *TestHarness) { // Keep the function, remove comment block
func setupChartWithUnsupportedStructure(t *testing.T, h *TestHarness) {
	t.Helper()
	chartPath := testutil.GetChartPath("unsupported-test")
	// G301: Use secure directory permissions (0750 or less)
	err := os.MkdirAll(filepath.Join(h.tempDir, chartPath), 0750)
	require.NoError(t, err, "Failed to create unsupported-test chart directory")

	// Create Chart.yaml
	chartYaml := `apiVersion: v2
name: unsupported-test
version: 0.1.0`
	// G306: Use secure file permissions (0600)
	require.NoError(t, os.WriteFile(filepath.Join(h.tempDir, chartPath, "Chart.yaml"), []byte(chartYaml), 0600))

	// Create values.yaml with unsupported structure
	valuesYaml := `image:
  name: nginx
  version: 1.23  # Using 'version' instead of 'tag'`
	// G306: Use secure file permissions (0600)
	require.NoError(t, os.WriteFile(filepath.Join(h.tempDir, chartPath, "values.yaml"), []byte(valuesYaml), 0600))

	h.chartPath = filepath.Join(h.tempDir, chartPath)
}

// */ // Remove end comment block

// nolint:unused // Kept for potential future uses
func chartExists(name string) bool {
	// Check if chart exists in test-data/charts
	_, err := os.Stat(filepath.Join("test-data", "charts", name))
	return err == nil
}

// Test reading overrides from standard output when --output-file is not provided
func TestReadOverridesFromStdout(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()
	h.SetupChart("minimal-test")
	h.SetRegistries("test.registry.io", []string{"docker.io"})

	// Run irr override without --output-file
	stdout, irrErr := h.ExecuteIRR("override", h.chartPath, "--target-registry", h.targetReg, "--source-registries", strings.Join(h.sourceRegs, ","))
	require.NoError(t, irrErr, "irr override command failed")
	require.NotEmpty(t, stdout, "stdout should contain the override YAML")

	// Parse the overrides from stdout
	var overrides map[string]interface{}
	err := yaml.Unmarshal([]byte(stdout), &overrides)
	require.NoError(t, err, "Failed to parse overrides from stdout")

	// Basic validation
	require.Contains(t, overrides, "image", "Overrides should contain the 'image' key")
	imageMap, ok := overrides["image"].(map[string]interface{})
	require.True(t, ok, "'image' key should be a map")
	assert.Equal(t, "test.registry.io/library/nginx", imageMap["repository"], "Repository mismatch")
	assert.Equal(t, "latest", imageMap["tag"], "Tag mismatch")

	// Check that the original override file path doesn't exist
	_, err = os.Stat(h.overridePath) // Use the harness's default override path
	assert.True(t, os.IsNotExist(err), "Override file should not exist when outputting to stdout")
}

// Helper function to compare override content
// func assertOverridesMatch(t *testing.T, harness *TestHarness, expectedRepo string) { // Keep commented out
/* Unused function
// Helper function to compare override content
func assertOverridesMatch(t *testing.T, harness *TestHarness, expectedRepo string) {
// ... (rest of assertOverridesMatch remains commented out)
}
*/
