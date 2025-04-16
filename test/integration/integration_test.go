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

	"github.com/lalbers/irr/pkg/testutil"

	"github.com/lalbers/irr/pkg/exitcodes"
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

// validateExpectedImages is a helper function to validate found images against expected ones
func validateExpectedImages(t *testing.T, expectedImages []string, foundImages map[string]bool, targetReg string) {
	for _, expectedImage := range expectedImages {
		expectedRepo := ""
		if strings.HasPrefix(expectedImage, targetReg+"/") {
			expectedRepo = strings.TrimPrefix(expectedImage, targetReg+"/")
			expectedRepo = strings.Split(expectedRepo, ":")[0]
		} else {
			switch {
			case strings.HasPrefix(expectedImage, "docker.io/"):
				imgPart := strings.TrimPrefix(expectedImage, "docker.io/")
				if !strings.Contains(imgPart, "/") {
					imgPart = "library/" + imgPart
				}
				expectedRepo = fmt.Sprintf("dockerio/%s", imgPart)
			case strings.HasPrefix(expectedImage, "registry.k8s.io/"):
				expectedRepo = fmt.Sprintf("registryk8sio/%s", strings.TrimPrefix(expectedImage, "registry.k8s.io/"))
			case strings.HasPrefix(expectedImage, "quay.io/"):
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
}

// collectImageInfo populates maps of found image repositories and string values from overrides
func collectImageInfo(t *testing.T, harness *TestHarness, overrides map[string]interface{}) (repos, stringVals map[string]bool) {
	foundImageRepos := make(map[string]bool)
	foundImageStrings := make(map[string]bool)

	// Walk the image fields in the overrides object and collect info
	harness.WalkImageFields(overrides, func(path []string, imageValue interface{}) {
		t.Logf("DEBUG: Found image at %s: %#v", strings.Join(path, "."), imageValue)

		switch typedValue := imageValue.(type) {
		case map[string]interface{}:
			if repo, ok := typedValue["repository"].(string); ok {
				foundImageRepos[repo] = true
			}
		case string:
			foundImageStrings[typedValue] = true
		}
	})

	return foundImageRepos, foundImageStrings
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
				"harbor.home.arpa/quayio/jetstack/cert-manager-controller:v1.17.1",
				"harbor.home.arpa/quayio/jetstack/cert-manager-webhook:v1.17.1",
				"harbor.home.arpa/quayio/jetstack/cert-manager-cainjector:v1.17.1",
				"harbor.home.arpa/quayio/jetstack/cert-manager-acmesolver:v1.17.1",
				"harbor.home.arpa/quayio/jetstack/cert-manager-startupapicheck:v1.17.1",
			},
			skip:       true,
			skipReason: "cert-manager chart is complex and requires separate targeted testing approach",
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
				"harbor.home.arpa/dockerio/bitnami/nginx:1.27.4-debian-12-r6",
				"harbor.home.arpa/dockerio/bitnami/git:2.48.1-debian-12-r9",
				"harbor.home.arpa/dockerio/bitnami/nginx-exporter:1.4.1-debian-12-r9",
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
					switch {
					case strings.HasPrefix(expectedImage, "docker.io/"):
						imgPart := strings.TrimPrefix(expectedImage, "docker.io/")
						if !strings.Contains(imgPart, "/") {
							imgPart = "library/" + imgPart
						}
						expectedRepo = "dockerio/" + imgPart
					case strings.HasPrefix(expectedImage, "registry.k8s.io/"):
						imgPart := strings.TrimPrefix(expectedImage, "registry.k8s.io/")
						expectedRepo = "registryk8sio/" + imgPart
					default:
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
			require.NoError(t, err, "Failed to read/parse generated overrides file")

			foundImageRepos, foundImageStrings := collectImageInfo(t, harness, overrides)

			// TEMPORARY: Print found images to debug expected paths
			t.Logf("DEBUG: Found images map for %s:\n%#v", tc.name, foundImageRepos)
			if len(foundImageStrings) > 0 {
				t.Logf("DEBUG: Found image strings for %s:\n%#v", tc.name, foundImageStrings)
			}

			// Validate that the expected images are present in the generated overrides
			validateExpectedImages(t, tc.expectedImages, foundImageRepos, harness.targetReg)
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
	h.SetupChart(testutil.GetChartPath("unsupported-test"))

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
	h.AssertErrorContains("unsupported structure found", args...)
}

func TestRegistryMappingFile(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	mappingContent := `mappings:
  - source: docker.io
    target: dckr
  - source: quay.io
    target: quaycustom
`
	mappingFilePath := filepath.Join(harness.tempDir, "test-mappings.yaml")
	err := os.WriteFile(mappingFilePath, []byte(mappingContent), defaultFilePerm)
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

func TestConfigFileMappings(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Create registry mappings content
	registryMappingsContent := `registries:
  mappings:
    - source: docker.io
      target: my-registry.io/custom/nginx-mirror
    - source: quay.io
      target: my-registry.io/monitoring/prometheus
`

	// Write the config file
	configPath := filepath.Join(harness.tempDir, "test-config.yaml")
	// #nosec G306 -- Using secure permissions (0600) for test-generated file
	err := os.WriteFile(configPath, []byte(registryMappingsContent), defaultFilePerm)
	require.NoError(t, err, "Failed to write test config file")

	output, err := harness.ExecuteIRR(
		"override",
		"--chart-path", testutil.GetChartPath("minimal-test"),
		"--target-registry", "target.registry.com",
		"--source-registries", "docker.io,quay.io",
		"--config", configPath,
		"--output-file", harness.overridePath,
		"--debug",
	)
	require.NoError(t, err, "IRR execution failed: %v\nOutput: %s", err, output)

	// Verify the overrides were written properly
	// #nosec G304 -- Reading a test-generated file from the test's temp directory is safe.
	overridesBytes, err := os.ReadFile(harness.overridePath)
	require.NoError(t, err, "Failed to read overrides file")
	require.NotEmpty(t, overridesBytes, "Overrides file should not be empty")
}

// TestClickhouseOperator tests the IRR tool's ability to process complex charts with multiple images
// using the clickhouse-operator chart as a test case
func TestClickhouseOperator(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	// Use the clickhouse-operator chart from the standard test charts location
	chartPath := testutil.GetChartPath("clickhouse-operator")
	_, err := os.Stat(chartPath)
	require.NoError(t, err, "Clickhouse-operator chart not found at %s", chartPath)

	// Set up the test harness with the clickhouse-operator chart
	harness.SetupChart(chartPath)

	// Set target registry as "harbor.home.arpa" and source registry as "docker.io"
	harness.SetRegistries("harbor.home.arpa", []string{"docker.io"})

	// Expected images to be found in the overrides
	expectedImages := []string{
		"harbor.home.arpa/dockerio/bitnami/clickhouse:25.3.2-debian-12-r3",
		"harbor.home.arpa/dockerio/bitnami/clickhouse-operator:0.24.5-debian-12-r3",
		"harbor.home.arpa/dockerio/bitnami/clickhouse-keeper:25.3.2-debian-12-r6",
		"harbor.home.arpa/dockerio/bitnami/clickhouse-operator-metrics-exporter:0.24.5-debian-12-r1",
	}

	// Generate overrides using the IRR tool
	if err := harness.GenerateOverrides(); err != nil {
		t.Fatalf("Failed to generate overrides: %v", err)
	}

	// Read and parse the overrides file
	overridesBytes, err := os.ReadFile(harness.overridePath)
	require.NoError(t, err, "Failed to read overrides file")
	require.NotEmpty(t, overridesBytes, "Overrides file should not be empty")

	overrides := make(map[string]interface{})
	err = yaml.Unmarshal(overridesBytes, &overrides)
	require.NoError(t, err, "Failed to unmarshal overrides YAML")

	// Collect image repositories from the overrides
	repos, _ := collectImageInfo(t, harness, overrides)
	require.Equal(t, 4, len(repos), "Expected 4 image repositories in overrides, got %d", len(repos))

	// Validate that the expected images are present in the overrides
	for _, expectedImage := range expectedImages {
		parts := strings.Split(expectedImage, "/")
		expectedRepo := strings.Join(parts[1:], "/")
		// Remove the tag part
		if strings.Contains(expectedRepo, ":") {
			expectedRepo = strings.Split(expectedRepo, ":")[0]
		}

		found := false
		for actualRepo := range repos {
			if strings.HasSuffix(actualRepo, expectedRepo) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected image %s not found in overrides. Found repositories: %v", expectedRepo, repos)
		}
	}

	// Add global.security.allowInsecureImages=true to the overrides to avoid Bitnami's validation error
	if globalMap, ok := overrides["global"].(map[string]interface{}); ok {
		if securityMap, ok := globalMap["security"].(map[string]interface{}); ok {
			securityMap["allowInsecureImages"] = true
		} else {
			globalMap["security"] = map[string]interface{}{
				"allowInsecureImages": true,
			}
		}
	} else {
		overrides["global"] = map[string]interface{}{
			"security": map[string]interface{}{
				"allowInsecureImages": true,
			},
		}
	}

	// Write updated overrides back to file
	updatedOverridesBytes, err := yaml.Marshal(overrides)
	require.NoError(t, err, "Failed to marshal updated overrides YAML")
	// #nosec G306 -- Using secure permissions (0600) for test-generated file
	err = os.WriteFile(harness.overridePath, updatedOverridesBytes, defaultFilePerm)
	require.NoError(t, err, "Failed to write updated overrides file")

	// Validate the overrides by running helm template
	if err := harness.ValidateOverrides(); err != nil {
		t.Fatalf("Failed to validate overrides: %v", err)
	}
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
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), defaultFilePerm))

	valuesYaml := `image:
  repository: nginx
  tag: "1.23"`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), defaultFilePerm))

	require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "templates"), 0o750))

	deploymentYaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: {{ .Values.image.repository }}:{{ .Values.image.tag }}`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(deploymentYaml), defaultFilePerm))

	h.chartPath = chartDir
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
	flag.Parse() // Parse flags

	// Build the binary once before running tests.
	fmt.Println("Building irr binary for integration tests...")
	if err := buildIrrBinary(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to build irr binary: %v\n", err)
		os.Exit(1) // Exit if build fails
	}
	fmt.Println("Build successful.")

	// Run the actual tests
	code := m.Run()

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
	h.SetupChart(testutil.GetChartPath("unsupported-test"))

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
	h.AssertErrorContains("unsupported structure found", args...)
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

// Note: The previous TestCertManagerComponents has been moved to cert_manager_test.go
// and implemented as TestCertManager with the component-group testing approach.
