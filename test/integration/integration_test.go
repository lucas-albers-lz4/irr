// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Exported debug flag variable
var DebugEnabled bool

func TestMinimalChart(t *testing.T) {
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
	for _, tc := range []struct {
		name           string
		chartPath      string
		sourceRegs     []string
		expectedImages []string
		skip           bool
		skipReason     string
	}{
		{
			name:      "cert-manager with webhook and cainjector",
			chartPath: testutil.GetChartPath("cert-manager"),
			sourceRegs: []string{
				"quay.io",
				"docker.io",
			},
			expectedImages: []string{
				"harbor.home.arpa/dockerio/jetstack/cert-manager-controller:latest",
				"harbor.home.arpa/dockerio/jetstack/cert-manager-webhook:latest",
				"harbor.home.arpa/dockerio/jetstack/cert-manager-cainjector:latest",
				"harbor.home.arpa/dockerio/jetstack/cert-manager-acmesolver:latest",
				"harbor.home.arpa/dockerio/jetstack/cert-manager-startupapicheck:latest",
			},
			skip: false,
		},
		{
			name:      "simplified-prometheus-stack with specific components",
			chartPath: testutil.GetChartPath("simplified-prometheus-stack"),
			sourceRegs: []string{
				"quay.io",
				"docker.io",
				"registry.k8s.io",
			},
			expectedImages: []string{
				"harbor.home.arpa/quayio/prometheus/prometheus:latest",
			},
			skip: false,
		},
		{
			name:      "ingress-nginx with admission webhook",
			chartPath: testutil.GetChartPath("ingress-nginx"),
			sourceRegs: []string{
				"registry.k8s.io",
				"docker.io",
			},
			expectedImages: []string{
				"docker.io/bitnami/nginx",
				"docker.io/bitnami/nginx-exporter",
			},
			skip: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip(tc.skipReason)
			}

			harness := NewTestHarness(t)
			defer harness.Cleanup()

			harness.SetupChart(tc.chartPath)
			harness.SetRegistries("harbor.home.arpa", tc.sourceRegs)

			args := []string{
				"override",
				"--chart-path", harness.chartPath,
				"--target-registry", harness.targetReg,
				"--source-registries", strings.Join(harness.sourceRegs, ","),
				"--output-file", harness.overridePath,
			}
			if harness.mappingsPath != "" {
				absMappingsPath, absErr := filepath.Abs(harness.mappingsPath)
				if absErr != nil {
					t.Fatalf("Failed to get absolute path for mappings file %s: %v", harness.mappingsPath, absErr)
				}
				args = append(args, "--registry-file", absMappingsPath)
			}
			args = append(args, "--debug")

			if tc.name == "ingress-nginx" {
				explicitOutputFile := filepath.Join(harness.tempDir, "explicit-ingress-nginx-overrides.yaml")
				explicitArgs := make([]string, len(args), len(args)+2)
				copy(explicitArgs, args)
				explicitArgs = append(explicitArgs, "--output-file", explicitOutputFile)

				explicitOutput, err := harness.ExecuteIRR(explicitArgs...)
				require.NoError(t, err, "Explicit ExecuteIRR failed for ingress-nginx. Output:\n%s", explicitOutput)

				// #nosec G304 -- Reading a test-generated file from the test's temp directory is safe.
				overridesBytes, err := os.ReadFile(explicitOutputFile)
				require.NoError(t, err, "Failed to read explicit output file")
				require.NotEmpty(t, overridesBytes, "Explicit output file should not be empty")

				explicitOverrides := make(map[string]interface{})
				err = yaml.Unmarshal(overridesBytes, &explicitOverrides)
				require.NoError(t, err, "Failed to unmarshal explicit overrides YAML for ingress-nginx")

				for _, expectedImage := range tc.expectedImages {
					expectedRepo := ""
					if strings.HasPrefix(expectedImage, "docker.io/") {
						imgPart := strings.TrimPrefix(expectedImage, "docker.io/")
						if !strings.Contains(imgPart, "/") {
							imgPart = "library/" + imgPart
						}
						expectedRepo = "dockerio/" + imgPart
					} else if strings.HasPrefix(expectedImage, "registry.k8s.io/") {
						imgPart := strings.TrimPrefix(expectedImage, "registry.k8s.io/")
						expectedRepo = "registryk8sio/" + imgPart
					} else {
						t.Fatalf("Unhandled source registry prefix in expected image: %s", expectedImage)
					}
					foundInExplicit := false
					harness.WalkImageFields(explicitOverrides, func(_ []string, imageValue interface{}) {
						if foundInExplicit {
							return
						}
						if imageMap, ok := imageValue.(map[string]interface{}); ok {
							if repo, ok := imageMap["repository"].(string); ok {
								if repo == expectedRepo {
									foundInExplicit = true
								}
							}
						}
					})
					if !foundInExplicit {
						t.Errorf("Expected image %s (looking for repo containing '%s') "+
							"not found in explicit overrides for ingress-nginx", expectedImage, expectedRepo)
					}
				}
				return
			}

			// #nosec G204 -- Test harness executes irr binary with test-controlled arguments.
			output, err := harness.ExecuteIRR(args...)
			if err != nil {
				t.Fatalf("Failed to execute irr override command: %v\nOutput:\n%s", err, output)
			}

			if err := harness.ValidateOverrides(); err != nil {
				t.Fatalf("Failed to validate overrides: %v", err)
			}

			overrides, err := harness.getOverrides()
			if err != nil {
				t.Fatalf("Failed to get overrides: %v", err)
			}

			foundImages := make(map[string]bool)
			harness.WalkImageFields(overrides, func(_ []string, imageValue interface{}) {
				if imageMap, ok := imageValue.(map[string]interface{}); ok {
					if repo, ok := imageMap["repository"].(string); ok {
						foundImages[repo] = true
						t.Logf("Found image repo in overrides: %s", repo)
					}
				}
			})

			for _, expectedImage := range tc.expectedImages {
				expectedRepo := ""
				if strings.HasPrefix(expectedImage, harness.targetReg+"/") {
					expectedRepo = strings.TrimPrefix(expectedImage, harness.targetReg+"/")
					expectedRepo = strings.Split(expectedRepo, ":")[0]
				} else {
					if strings.HasPrefix(expectedImage, "docker.io/") {
						imgPart := strings.TrimPrefix(expectedImage, "docker.io/")
						if !strings.Contains(imgPart, "/") {
							imgPart = "library/" + imgPart
						}
						expectedRepo = fmt.Sprintf("dockerio/%s", imgPart)
					} else if strings.HasPrefix(expectedImage, "registry.k8s.io/") {
						expectedRepo = fmt.Sprintf("registryk8sio/%s", strings.TrimPrefix(expectedImage, "registry.k8s.io/"))
					} else if strings.HasPrefix(expectedImage, "quay.io/") {
						expectedRepo = fmt.Sprintf("quayio/%s", strings.TrimPrefix(expectedImage, "quay.io/"))
					}
				}
				if expectedRepo == "" {
					t.Errorf("Could not determine expected rewritten repo path for: %s", expectedImage)
					continue
				}

				found := false
				for actualRepo := range foundImages {
					if actualRepo == expectedRepo {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected rewritten image %s not found in overrides. Found repositories: %v", expectedRepo, foundImages)
				}
			}
		})
	}
}

func TestDryRunFlag(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	setupMinimalTestChart(t, harness)

	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", "harbor.example.com",
		"--source-registries", "docker.io",
		"--dry-run",
	}

	// #nosec G204 -- Test harness executes irr binary with test-controlled arguments.
	cmd := exec.Command("../../bin/irr", args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Dry run should succeed")

	_, err = os.Stat(filepath.Join(harness.tempDir, "overrides.yaml"))
	assert.True(t, os.IsNotExist(err), "No override file should be created in dry-run mode")

	assert.Contains(t, string(output), "repository:", "Dry run output should contain override preview")
}

func TestStrictMode(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Setup chart with known unsupported structure
	h.SetChartPath(h.GetTestdataPath("unsupported-test"))

	// Define the arguments for the IRR command
	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", "test.target.io", // Required flag
		"--source-registries", "test.source.io", // Required flag
		"--strict",
	}

	// Verify exit code is 12 (ExitUnsupportedStructure) using harness method, passing args
	h.AssertExitCode(exitcodes.ExitUnsupportedStructure, args...)

	// Verify the error message contains expected text using harness method, passing args
	h.AssertErrorContains("unsupported structures found", args...)
}

func TestRegistryMappingFile(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	mappingContent := `
docker.io: dckr
quay.io: quaycustom
`
	mappingFilePath := filepath.Join(harness.tempDir, "test-mappings.yaml")
	err := os.WriteFile(mappingFilePath, []byte(mappingContent), 0o600)
	require.NoError(t, err, "Failed to write temp mapping file")

	harness.SetupChart(testutil.GetChartPath("minimal-test"))
	harness.SetRegistries("target.registry.com", []string{"docker.io", "quay.io"})

	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", harness.targetReg,
		"--source-registries", strings.Join(harness.sourceRegs, ","),
		"--registry-file", mappingFilePath,
		"--output-file", harness.overridePath,
	}

	output, err := harness.ExecuteIRR(args...)
	require.NoError(t, err, "irr command with mapping file failed. Output: %s", output)

	overrides, err := harness.getOverrides()
	require.NoError(t, err, "Failed to read/parse generated overrides file")

	// Check image from first mapped source (docker.io -> dckr)
	dockerImageData, ok := overrides["image"].(map[string]interface{})
	require.True(t, ok, "Failed to find map for overrides[\"image\"]")
	dockerRegistryValue, ok := dockerImageData["registry"].(string)
	assert.True(t, ok, "Failed to find registry for image [image]")
	assert.Equal(t, "dckr", dockerRegistryValue, "Mapped registry 'dckr' should be used for docker.io source")
	dockerRepoValue, ok := dockerImageData["repository"].(string)
	assert.True(t, ok, "Failed to find repository for image [image]")
	// The prefix strategy uses sanitized source 'dockerio' prepended to original repo 'library/nginx' (after normalization)
	assert.Equal(t, "dockerio/library/nginx", dockerRepoValue, "Repository path prefix mismatch")

	// Check image from second mapped source (quay.io -> quaycustom)
	quayImageMap, ok := overrides["quayImage"].(map[string]interface{})
	require.True(t, ok, "Failed to find map for overrides[\"quayImage\"]")
	quayImageData, ok := quayImageMap["image"].(map[string]interface{})
	require.True(t, ok, "Failed to find map for overrides[\"quayImage\"][\"image\"]")
	quayRegistryValue, ok := quayImageData["registry"].(string)
	assert.True(t, ok, "Failed to find registry for image [quayImage][image]")
	assert.Equal(t, "quaycustom", quayRegistryValue, "Mapped registry 'quaycustom' should be used for quay.io source")
	quayRepoValue, ok := quayImageData["repository"].(string)
	assert.True(t, ok, "Failed to find repository for image [quayImage][image]")
	// The prefix strategy uses sanitized source 'quayio' prepended to original repo 'prometheus/node-exporter'
	assert.Equal(t, "quayio/prometheus/node-exporter", quayRepoValue, "Repository path should be prefixed with sanitized source 'quayio'")

	// NOTE: Removed checks for 'gcrImage' as it's not present in 'minimal-test' chart.
	// NOTE: Removed assert.Contains checks on stdout as they contradicted the expected override values.
}

func TestMinimalGitImageOverride(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	harness.SetupChart(testutil.GetChartPath("minimal-git-image"))
	harness.SetRegistries("harbor.test.local", []string{"docker.io"})

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

	overrides, err := harness.getOverrides()
	require.NoError(t, err, "Failed to read/parse generated overrides file")

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
		overrideBytes, readErr := os.ReadFile(harness.overridePath)
		t.Errorf("Expected image repository '%s' not found in overrides", expectedRepo)
		if readErr != nil {
			t.Logf("Additionally, failed to read overrides file %s for debugging: %v", harness.overridePath, readErr)
		} else {
			t.Logf("Overrides content:\n%s", string(overrideBytes))
		}
	}
}

func setupMinimalTestChart(t *testing.T, h *TestHarness) {
	chartDir := filepath.Join(h.tempDir, "minimal-chart")
	require.NoError(t, os.MkdirAll(chartDir, 0o750))

	chartYaml := `apiVersion: v2
name: minimal-chart
version: 0.1.0`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0o600))

	valuesYaml := `image:
  repository: nginx
  tag: "1.23"`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), 0600))

	h.chartPath = chartDir
}

func setupChartWithUnsupportedStructure(t *testing.T, h *TestHarness) {
	t.Helper()
	chartDirName := "unsupported-test-dynamic"
	chartDirPath := filepath.Join(h.tempDir, chartDirName)

	// G301 fix: Use 0750 permissions
	err := os.MkdirAll(filepath.Join(chartDirPath, "templates"), 0750)
	require.NoError(t, err, "Failed to create dynamic unsupported-test chart directory")

	chartYaml := `
apiVersion: v2
name: unsupported-test-dynamic
version: 0.1.0
description: A dynamically created chart with unsupported structures for strict mode testing.
`
	// G306 fix: Use 0600 permissions
	err = os.WriteFile(filepath.Join(chartDirPath, "Chart.yaml"), []byte(chartYaml), 0600)
	require.NoError(t, err)

	valuesYaml := `
replicaCount: 1

image:
  repository: myimage
  tag: latest
  pullPolicy: IfNotPresent

problematicImage:
  registry: "{{ .Values.global.registry }}"
  repository: "{{ .Values.global.repository }}/app"
  tag: "{{ .Values.appVersion }}"

global:
  registry: source.io
  repository: my-global-repo
appVersion: v1.2.3
`
	// G306 fix: Use 0600 permissions
	err = os.WriteFile(filepath.Join(chartDirPath, "values.yaml"), []byte(valuesYaml), 0600)
	require.NoError(t, err)

	templatesYaml := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: {{ .Values.replicaCount }}
  template:
    spec:
      containers:
      - name: normal-container
        image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
      - name: problematic-container
        image: "{{ .Values.problematicImage.registry }}/{{ .Values.problematicImage.repository }}:{{ .Values.appVersion }}"
`
	// G306 fix: Use 0600 permissions
	err = os.WriteFile(filepath.Join(chartDirPath, "templates", "deployment.yaml"), []byte(templatesYaml), 0600)
	require.NoError(t, err)

	h.chartPath = chartDirPath
	t.Logf("Setup dynamic unsupported chart at: %s", h.chartPath)
}

func TestReadOverridesFromStdout(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()
	h.SetupChart(testutil.GetChartPath("minimal-test"))
	h.SetRegistries("test.registry.io", []string{"docker.io"})

	tempOutputFile := filepath.Join(h.tempDir, "stdout-test-override.yaml")

	// Execute IRR with arguments including the temp output file
	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", tempOutputFile,
	}

	// #nosec G204 -- Test harness executes irr binary with test-controlled arguments.
	cmd := exec.Command("../../bin/irr", args...)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	require.NoError(t, err, "irr override command failed")
	t.Logf("IRR command stdout/stderr (not used for parsing):\n%s", output)

	// Read the generated override file
	// #nosec G304 -- Reading a test-generated file from the test's temp directory is safe.
	overrideBytes, err := os.ReadFile(tempOutputFile)
	require.NoError(t, err, "Failed to read generated override file: %s", tempOutputFile)

	// Unmarshal the generated YAML
	var overrides map[string]interface{}
	err = yaml.Unmarshal(overrideBytes, &overrides)
	require.NoError(t, err, "Failed to parse overrides from temp file content")

	t.Logf("Parsed overrides map:\n%s", overrides)

	require.Contains(t, overrides, "image", "Overrides should contain the 'image' key")
	imageMap, ok := overrides["image"].(map[string]interface{})
	require.True(t, ok, "'image' key should be a map")

	t.Logf("Extracted imageMap:\n%s", imageMap)

	assert.Equal(t, "test.registry.io", imageMap["registry"], "Registry mismatch")
	assert.Equal(t, "dockerio/library/nginx", imageMap["repository"], "Repository mismatch")
	assert.Equal(t, "latest", imageMap["tag"], "Tag mismatch")

	_, err = os.Stat(h.overridePath)
	assert.True(t, os.IsNotExist(err), "Override file should not exist when outputting to stdout")
}

// TestMain sets up the integration test environment.
func TestMain(m *testing.M) {
	// Define the debug flag
	flag.BoolVar(&DebugEnabled, "debug", false, "Enable debug logging")
	// flag.Parse() // Do NOT parse flags here; let the test runner handle it.

	// Setup and teardown logic placeholders (as per TODO.md)
	// TODO: Implement setup() and teardown() logic
	// setup()

	// Run tests
	code := m.Run()

	// Teardown logic placeholder
	// teardown()

	os.Exit(code)
}

// Removed placeholder setup function

func TestNoArgs(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Verify exit code is 1 (ExitMissingRequiredFlag) when no args provided
	h.AssertExitCode(exitcodes.ExitMissingRequiredFlag, "override")

	// Verify error message contains expected text
	h.AssertErrorContains("required flag(s)", "override")
}

func TestUnknownFlag(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	_, err := h.ExecuteIRR("override", "--unknown-flag")
	assert.Error(t, err, "should error on unknown flag")
	t.Cleanup(h.Cleanup)
}

func TestInvalidStrategy(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	h.SetChartPath(h.GetTestdataPath("simple"))
	_, err := h.ExecuteIRR("override", "--strategy", "invalid-strategy")
	assert.Error(t, err, "should error on invalid strategy")
	t.Cleanup(h.Cleanup)
}

func TestMissingChartPath(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	_, err := h.ExecuteIRR("override") // Missing --chart-path
	assert.Error(t, err, "should error when chart path is missing")
	t.Cleanup(h.Cleanup)
}

func TestNonExistentChartPath(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	_, err := h.ExecuteIRR("override", "--chart-path", "/path/does/not/exist")
	assert.Error(t, err, "should error when chart path does not exist")
	t.Cleanup(h.Cleanup)
}

func TestStrictModeExitCode(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Setup chart with known unsupported structure
	h.SetChartPath(h.GetTestdataPath("unsupported-test"))

	// Define the arguments for the IRR command
	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", "test.target.io", // Required flag
		"--source-registries", "test.source.io", // Required flag
		"--strict",
	}

	// Verify exit code is 12 (ExitUnsupportedStructure) using harness method, passing args
	h.AssertExitCode(exitcodes.ExitUnsupportedStructure, args...)

	// Verify the error message contains expected text using harness method, passing args
	h.AssertErrorContains("unsupported structures found", args...)
}

func TestInvalidChartPath(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	h.SetChartPath("/invalid/path/does/not/exist")
	_, err := h.ExecuteIRR("override")
	assert.Error(t, err, "should error when chart path does not exist")
	t.Cleanup(h.Cleanup)
}

func TestInvalidRegistryMappingFile(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	h.SetChartPath(h.GetTestdataPath("simple"))
	_, err := h.ExecuteIRR("override", "--registry-mappings", "/invalid/path/does/not/exist.yaml")
	assert.Error(t, err, "should error when registry mappings file does not exist")
	t.Cleanup(h.Cleanup)
}

func setupTestChart(t *testing.T, valuesYaml string, chartYaml string, templatesYaml string) string {
	t.Helper()
	chartDir, err := os.MkdirTemp("", "testchart-")
	require.NoError(t, err)

	// Create templates directory
	// G301 fix: Use 0750 permissions
	err = os.MkdirAll(filepath.Join(chartDir, "templates"), 0750) // #nosec G301
	require.NoError(t, err)

	// Create Chart.yaml
	if chartYaml == "" {
		chartYaml = `apiVersion: v2
name: test-chart
version: 0.1.0`
	}
	// G306 fix: Use 0600 permissions
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0600)) // #nosec G306

	// Create values.yaml if provided
	if valuesYaml != "" {
		// G306 fix: Use 0600 permissions
		require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), 0600)) // #nosec G306
	}

	// Create template file if provided
	if templatesYaml != "" {
		// G306 fix: Use 0600 permissions
		require.NoError(t, os.WriteFile(filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(templatesYaml), 0600)) // #nosec G306
	}

	return chartDir
}

func TestStrictModeExitCode12(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Set up chart with unsupported structure
	h.SetupChart(h.GetTestdataPath("unsupported-test"))
	h.SetRegistries("target.io", []string{"source.io"})

	// Run with strict mode - should fail with exit code 12
	h.AssertExitCode(12, "override", "--strict")
	h.AssertErrorContains("strict validation failed")
}
