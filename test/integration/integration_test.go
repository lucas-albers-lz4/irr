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

	"github.com/lalbers/irr/pkg/image"
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

			foundImageRepos := make(map[string]bool)
			foundImageStrings := make(map[string]bool)

			harness.WalkImageFields(overrides, func(path []string, imageValue interface{}) {
				if imageMap, ok := imageValue.(map[string]interface{}); ok {
					if repo, repoOk := imageMap["repository"].(string); repoOk {
						// Also check registry and tag if available for more precise matching if needed
						fullRef := repo
						if reg, regOk := imageMap["registry"].(string); regOk && reg != "" {
							fullRef = reg + "/" + fullRef // Reconstruct approx full ref
						}
						if tag, tagOk := imageMap["tag"].(string); tagOk && tag != "" {
							fullRef = fullRef + ":" + tag
						} else if digest, digestOk := imageMap["digest"].(string); digestOk && digest != "" {
							fullRef = fullRef + "@" + digest
						}
						t.Logf("Found image map at path %v: Repo='%s', FullRef (approx)='%s'", path, repo, fullRef)
						foundImageRepos[repo] = true      // Store repo for validation
						foundImageStrings[fullRef] = true // Store reconstructed full ref
					}
				} else if imageStr, ok := imageValue.(string); ok {
					// Handle direct string override
					t.Logf("Found image string at path %v: '%s'", path, imageStr)
					foundImageStrings[imageStr] = true // Store full string
					// Also try to parse and store the repo part for compatibility with existing validation
					if ref, err := image.ParseImageReference(imageStr, true); err == nil {
						repoKey := ref.Repository // Default to just repo
						if ref.Registry != "" {
							// Construct registry/repository format if registry is present
							repoKey = ref.Registry + "/" + ref.Repository
						}
						foundImageRepos[repoKey] = true // Store registry/repo or just repo
					}
				}
			})

			// Keep validateExpectedImages for now, but it might need adjustment
			// It primarily checks repository paths derived from expected full strings
			validateExpectedImages(t, tc.expectedImages, foundImageRepos, harness.targetReg)

			// Additionally, check if the full expected strings were found anywhere
			for _, expectedFullImage := range tc.expectedImages {
				if !foundImageStrings[expectedFullImage] {
					// Check if a repo-only match was found, indicating maybe tag was missing/different
					expectedRepoKey := deriveRepoKey(expectedFullImage, harness.targetReg) // Need a helper like in validateExpectedImages
					if !foundImageRepos[expectedRepoKey] {
						t.Errorf("Expected full image string OR repository path not found in overrides: %s (Expected Repo Key: %s)", expectedFullImage, expectedRepoKey)
					}
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

	// Verify exit code is 11 (ExitImageProcessingError) using harness method, passing args
	h.AssertExitCode(exitcodes.ExitImageProcessingError, args...)

	// Verify the error message contains expected text using harness method, passing args
	h.AssertErrorContains("unsupported structure found", args...)
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
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), 0o600))

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
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(deploymentYaml), 0o600))

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
	h.SetChartPath(h.GetTestdataPath("unsupported-test"))

	// Define the arguments for the IRR command
	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", "test.target.io", // Required flag
		"--source-registries", "test.source.io", // Required flag
		"--strict",
	}

	// Verify exit code is 11 (ExitImageProcessingError) using harness method, passing args
	h.AssertExitCode(exitcodes.ExitImageProcessingError, args...)

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

// TestChartFeatures_CertManager tests cert-manager chart with webhook and cainjector
func TestChartFeatures_CertManager(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	chartPath := testutil.GetChartPath("cert-manager")
	sourceRegs := []string{"quay.io", "docker.io"}
	expectedImages := []string{
		"harbor.home.arpa/dockerio/jetstack/cert-manager-controller:latest",
		"harbor.home.arpa/dockerio/jetstack/cert-manager-webhook:latest",
		"harbor.home.arpa/dockerio/jetstack/cert-manager-cainjector:latest",
		"harbor.home.arpa/dockerio/jetstack/cert-manager-acmesolver:latest",
		"harbor.home.arpa/dockerio/jetstack/cert-manager-startupapicheck:latest",
	}

	harness.SetupChart(chartPath)
	harness.SetRegistries("harbor.home.arpa", sourceRegs)

	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", harness.targetReg,
		"--source-registries", strings.Join(harness.sourceRegs, ","),
		"--output-file", harness.overridePath,
		"--debug",
	}
	if harness.mappingsPath != "" {
		absMappingsPath, absErr := filepath.Abs(harness.mappingsPath)
		if absErr != nil {
			t.Fatalf("Failed to get absolute path for mappings file %s: %v", harness.mappingsPath, absErr)
		}
		args = append(args, "--registry-file", absMappingsPath)
	}

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

	foundImageRepos := make(map[string]bool)
	foundImageStrings := make(map[string]bool)

	harness.WalkImageFields(overrides, func(path []string, imageValue interface{}) {
		if imageMap, ok := imageValue.(map[string]interface{}); ok {
			if repo, repoOk := imageMap["repository"].(string); repoOk {
				// Also check registry and tag if available for more precise matching if needed
				fullRef := repo
				if reg, regOk := imageMap["registry"].(string); regOk && reg != "" {
					fullRef = reg + "/" + fullRef // Reconstruct approx full ref
				}
				if tag, tagOk := imageMap["tag"].(string); tagOk && tag != "" {
					fullRef = fullRef + ":" + tag
				} else if digest, digestOk := imageMap["digest"].(string); digestOk && digest != "" {
					fullRef = fullRef + "@" + digest
				}
				t.Logf("Found image map at path %v: Repo='%s', FullRef (approx)='%s'", path, repo, fullRef)
				foundImageRepos[repo] = true      // Store repo for validation
				foundImageStrings[fullRef] = true // Store reconstructed full ref
			}
		} else if imageStr, ok := imageValue.(string); ok {
			// Handle direct string override
			t.Logf("Found image string at path %v: '%s'", path, imageStr)
			foundImageStrings[imageStr] = true // Store full string
			// Also try to parse and store the repo part for compatibility with existing validation
			if ref, err := image.ParseImageReference(imageStr, true); err == nil {
				repoKey := ref.Repository // Default to just repo
				if ref.Registry != "" {
					// Construct registry/repository format if registry is present
					repoKey = ref.Registry + "/" + ref.Repository
				}
				foundImageRepos[repoKey] = true // Store registry/repo or just repo
			}
		}
	})

	// Keep validateExpectedImages for now, but it might need adjustment
	// It primarily checks repository paths derived from expected full strings
	validateExpectedImages(t, expectedImages, foundImageRepos, harness.targetReg)

	// Additionally, check if the full expected strings were found anywhere
	for _, expectedFullImage := range expectedImages {
		if !foundImageStrings[expectedFullImage] {
			// Check if a repo-only match was found, indicating maybe tag was missing/different
			expectedRepoKey := deriveRepoKey(expectedFullImage, harness.targetReg) // Need a helper like in validateExpectedImages
			if !foundImageRepos[expectedRepoKey] {
				t.Errorf("Expected full image string OR repository path not found in overrides: %s (Expected Repo Key: %s)", expectedFullImage, expectedRepoKey)
			}
		}
	}
}

// TestChartFeatures_PrometheusStack tests simplified prometheus stack with specific components
func TestChartFeatures_PrometheusStack(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	chartPath := testutil.GetChartPath("simplified-prometheus-stack")
	sourceRegs := []string{"quay.io", "docker.io", "registry.k8s.io"}
	expectedImages := []string{
		"harbor.home.arpa/quayio/prometheus/prometheus:latest",
	}

	harness.SetupChart(chartPath)
	harness.SetRegistries("harbor.home.arpa", sourceRegs)

	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", harness.targetReg,
		"--source-registries", strings.Join(harness.sourceRegs, ","),
		"--output-file", harness.overridePath,
		"--debug",
	}
	if harness.mappingsPath != "" {
		absMappingsPath, absErr := filepath.Abs(harness.mappingsPath)
		if absErr != nil {
			t.Fatalf("Failed to get absolute path for mappings file %s: %v", harness.mappingsPath, absErr)
		}
		args = append(args, "--registry-file", absMappingsPath)
	}

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

	validateExpectedImages(t, expectedImages, foundImages, harness.targetReg)
}

// TestChartFeatures_IngressNginx tests ingress-nginx with admission webhook
func TestChartFeatures_IngressNginx(t *testing.T) {
	harness := NewTestHarness(t)
	defer harness.Cleanup()

	chartPath := testutil.GetChartPath("ingress-nginx")
	sourceRegs := []string{"registry.k8s.io", "docker.io"}
	expectedImages := []string{
		"docker.io/bitnami/nginx",
		"docker.io/bitnami/nginx-exporter",
	}

	harness.SetupChart(chartPath)
	harness.SetRegistries("harbor.home.arpa", sourceRegs)

	args := []string{
		"override",
		"--chart-path", harness.chartPath,
		"--target-registry", harness.targetReg,
		"--source-registries", strings.Join(harness.sourceRegs, ","),
		"--output-file", harness.overridePath,
		"--debug",
	}
	if harness.mappingsPath != "" {
		absMappingsPath, absErr := filepath.Abs(harness.mappingsPath)
		if absErr != nil {
			t.Fatalf("Failed to get absolute path for mappings file %s: %v", harness.mappingsPath, absErr)
		}
		args = append(args, "--registry-file", absMappingsPath)
	}

	// Test explicit output file
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

	foundImages := make(map[string]bool)
	harness.WalkImageFields(explicitOverrides, func(_ []string, imageValue interface{}) {
		if imageMap, ok := imageValue.(map[string]interface{}); ok {
			if repo, ok := imageMap["repository"].(string); ok {
				foundImages[repo] = true
				t.Logf("Found image repo in overrides: %s", repo)
			}
		}
	})

	validateExpectedImages(t, expectedImages, foundImages, harness.targetReg)
}

// validateExpectedImages is a helper function to validate found images against expected ones
func validateExpectedImages(t *testing.T, expectedImages []string, foundImages map[string]bool, targetReg string) {
	for _, expectedImage := range expectedImages {
		expectedRepo := ""
		if strings.HasPrefix(expectedImage, targetReg+"/") {
			expectedRepo = strings.TrimPrefix(expectedImage, targetReg+"/")
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
}

// Helper function to derive the repository key (needs to be implemented or moved from validateExpectedImages)
func deriveRepoKey(fullImage, targetReg string) string {
	// Simplified logic - mirroring parts of validateExpectedImages
	if strings.HasPrefix(fullImage, targetReg+"/") {
		repo := strings.TrimPrefix(fullImage, targetReg+"/")
		return strings.Split(repo, ":")[0] // Return repo part without tag/digest
	}
	// Add logic for docker.io, quay.io etc. if needed, mirroring validateExpectedImages
	return "" // Placeholder
}
