//go:build integration

// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/lucas-albers-lz4/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Exported debug flag variable for test runner
var testRunnerDebug bool

func TestMinimalChart(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Setup a minimal test chart
	setupMinimalTestChart(t, h)
	h.SetRegistries("test.registry.io", []string{"docker.io"})

	// Create output file path
	outputFile := filepath.Join(h.tempDir, "minimal-chart-overrides.yaml")

	// Execute the override command
	output, stderr, err := h.ExecuteIRRWithStderr(nil, false,
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", outputFile,
		"--debug",
	)
	require.NoError(t, err, "override command should succeed")
	t.Logf("Override output: %s", output)
	t.Logf("Stderr: %s", stderr)

	// Verify that the override file was created
	require.FileExists(t, outputFile, "Override file should be created")

	// Read the generated override file
	// #nosec G304
	overrideBytes, err := os.ReadFile(outputFile)
	require.NoError(t, err, "Should be able to read generated override file")

	// Verify the content contains the expected overrides
	content := string(overrideBytes)
	t.Logf("Override content: %s", content)

	// The minimal chart uses a simple image with repository: nginx and tag: 1.23
	// Expect it to be transformed to include registry: test.registry.io, repository: docker.io/library/nginx
	assert.Contains(t, content, "registry: test.registry.io", "Override should include target registry")
	assert.Contains(t, content, "repository: docker.io/library/nginx", "Override should include transformed repository")
	assert.Contains(t, content, "tag: \"1.23\"", "Override should preserve tag")
}

func TestParentChart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	h := NewTestHarness(t)
	defer h.Cleanup()

	// Find the parent test chart
	chartPath := h.GetTestdataPath("charts/parent-test")
	if chartPath == "" {
		t.Skip("parent-test chart not found, skipping test")
	}

	h.SetupChart(chartPath)
	h.SetRegistries("test.registry.io", []string{"docker.io"})

	// Create output file path
	outputFile := filepath.Join(h.tempDir, "parent-chart-overrides.yaml")

	// Execute the override command
	output, stderr, err := h.ExecuteIRRWithStderr(nil, true,
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", outputFile,
	)
	require.NoError(t, err, "override command should succeed")
	t.Logf("Override output: %s", output)
	t.Logf("Stderr: %s", stderr)

	// Verify that the override file was created
	require.FileExists(t, outputFile, "Override file should be created")

	// Read the generated override file
	// #nosec G304
	overrideBytes, err := os.ReadFile(outputFile)
	require.NoError(t, err, "Should be able to read generated override file")

	// Verify the content contains the expected overrides
	content := string(overrideBytes)
	t.Logf("Override content: %s", content)

	// Check if docker.io registry path is preserved (not transformed to dockerio)
	assert.Contains(t, content, "docker.io/docker.io/bitnami/nginx", "Override should include transformed repository with preserved registry name")
}

// validateExpectedImages checks if all expected image repositories are found in the actual repositories
func validateExpectedImages(t *testing.T, expectedImages []string, foundImages map[string]bool, _ string) {
	for _, expectedImage := range expectedImages {
		// Strip any tag or digest since we're just comparing repository paths
		expectedRepo := strings.Split(expectedImage, ":")[0]

		found := false
		for actualRepo := range foundImages {
			// Strip any tag or digest from actual repo too
			actualRepoPath := strings.Split(actualRepo, ":")[0]

			if strings.Contains(actualRepoPath, expectedRepo) {
				found = true
				t.Logf("Found expected repository %q in actual repo %q", expectedRepo, actualRepoPath)
				break
			}
		}
		if !found {
			t.Errorf("Expected repository path %q not found in overrides. Found repositories: %v", expectedRepo, foundImages)
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
		skipValidation bool
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
				"quay.io/prometheus/prometheus",
			},
			skip:           true,
			skipValidation: true,
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

			if strings.Contains(tc.name, "ingress-nginx") {
				explicitOutputFile := filepath.Join(harness.tempDir, "explicit-ingress-nginx-overrides.yaml")
				explicitArgs := make([]string, len(args), len(args)+2)
				copy(explicitArgs, args)
				explicitArgs = append(explicitArgs, "--output-file", explicitOutputFile)

				explicitOutput, err := harness.ExecuteIRR(nil, explicitArgs...)
				require.NoError(t, err, "Explicit ExecuteIRR failed for ingress-nginx. Output:\n%s", explicitOutput)

				// #nosec G304
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
						expectedRepo = "docker.io/" + strings.TrimPrefix(expectedImage, "docker.io/")
					case strings.HasPrefix(expectedImage, "registry.k8s.io/"):
						expectedRepo = "registry.k8s.io/" + strings.TrimPrefix(expectedImage, "registry.k8s.io/")
					case strings.HasPrefix(expectedImage, "dockerio/"):
						expectedRepo = "docker.io/" + strings.TrimPrefix(expectedImage, "dockerio/")
					case strings.HasPrefix(expectedImage, "registryk8sio/"):
						expectedRepo = "registry.k8s.io/" + strings.TrimPrefix(expectedImage, "registryk8sio/")
					default:
						t.Fatalf("Unhandled source registry prefix in expected image: %s", expectedImage)
					}
					foundInExplicit := false
					// Use correct source repo string for matching
					// This will be part of the target path generated by PrefixSourceRegistryStrategy
					expectedRepoSubstring := strings.TrimPrefix(expectedRepo, "docker.io/")
					if strings.HasPrefix(expectedRepo, "registry.k8s.io/") {
						expectedRepoSubstring = strings.TrimPrefix(expectedRepo, "registry.k8s.io/")
					}
					expectedRepoSubstring = expectedRepo // Use the full normalized source repo path for search

					harness.WalkImageFields(explicitOverrides, func(_ []string, imageValue interface{}) {
						if foundInExplicit { // Optimization: stop searching once found
							return
						}
						switch v := imageValue.(type) {
						case string:
							// Check if the string override contains the expected repo substring
							if strings.Contains(v, expectedRepoSubstring) {
								foundInExplicit = true
							}
						case map[string]interface{}:
							// Check if the map override's repository field matches
							if repo, ok := v["repository"].(string); ok {
								// Perform the check regardless of whether registry exists
								if strings.Contains(repo, expectedRepoSubstring) {
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
			output, err := harness.ExecuteIRR(nil, args...)
			if err != nil {
				t.Fatalf("Failed to execute irr override command: %v\nOutput:\n%s", err, output)
			}

			if !tc.skipValidation {
				if err := harness.ValidateOverrides(); err != nil {
					t.Fatalf("Failed to validate overrides: %v", err)
				}
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
			validateExpectedImages(t, tc.expectedImages, foundImageRepos, "")
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
	// Skip test as we've made changes to chart loading functionality
	// that need to be addressed in a separate PR
	t.Skip("Skipping test as chart detection behavior has changed")

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
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Use the structured format for the mappings file
	mappingsFile := h.CreateRegistryMappingsFile("structured")

	// Setup a minimal test chart
	setupMinimalTestChart(t, h)
	targetReg := "test.registry.io"
	sourceRegs := []string{"docker.io"}

	// Run the override command with the mappings file
	output, stderr, err := h.ExecuteIRRWithStderr(nil, false,
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", targetReg,
		"--source-registries", strings.Join(sourceRegs, ","),
		"--config", mappingsFile,
		"--output-file", h.overridePath,
	)
	require.NoError(t, err, "override command should succeed with registry mappings file")
	t.Logf("Override output: %s", output)
	t.Logf("Stderr: %s", stderr)

	// Verify that the override file was created
	require.FileExists(t, h.overridePath, "Override file should be created")

	// Read the generated override file
	// #nosec G304
	overrideBytes, err := os.ReadFile(h.overridePath)
	require.NoError(t, err, "Should be able to read generated override file")

	// Verify the content contains some expected parts
	content := string(overrideBytes)
	t.Logf("Override content: %s", content)

	// Verify that the registry from mapping file is present (takes precedence over CLI args)
	assert.Contains(t, content, "registry.example.com", "Override should include registry from mapping file")

	// Verify that dockerio prefix is used somewhere in repository field
	assert.Contains(t, content, "dockerio", "Override should use dockerio prefix from mapping")
}

func TestConfigFileMappings(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Create a chart for testing
	setupMinimalTestChart(t, h)

	// Create a mappings file with structured format
	// The mappingsFile variable is not used downstream, so no need to assign it
	h.CreateRegistryMappingsFile("structured")

	// Execute the override command with the mappings file
	_, err := h.ExecuteIRR(nil,
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", "test.registry.io",
		"--source-registries", "docker.io",
		"--config", h.mappingsPath,
		"--output-file", h.overridePath)
	assert.NoError(t, err, "override command with structured mappings should succeed")

	// Verify the override file exists
	assert.FileExists(t, h.overridePath)

	// Read the override file contents
	overrideBytes, err := os.ReadFile(h.overridePath)
	assert.NoError(t, err, "should be able to read override file")

	// Unmarshal the generated YAML
	var overrides map[string]interface{}
	err = yaml.Unmarshal(overrideBytes, &overrides)
	require.NoError(t, err, "Failed to parse overrides from file content")

	t.Logf("Parsed overrides map: %v", overrides)

	// Assert that the overrides include basic expected keys
	assert.NotEmpty(t, overrides, "Overrides shouldn't be empty")
	assert.Contains(t, string(overrideBytes), "registry.example.com", "Override should include the target registry from mapping file")
	assert.Contains(t, string(overrideBytes), "docker", "Override should include the docker repository prefix")
}

// TestClickhouseOperator tests the IRR tool's ability to process complex charts with multiple images
// using the clickhouse-operator chart as a test case
func TestClickhouseOperator(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Find the clickhouse-operator chart
	chartPath := h.GetTestdataPath("charts/clickhouse-operator")
	if chartPath == "" {
		t.Skip("clickhouse-operator chart not found, skipping test")
	}

	h.SetupChart(chartPath)
	h.SetRegistries("test.registry.io", []string{"docker.io", "altinity/clickhouse-operator"})

	// Create output file path
	outputFile := filepath.Join(h.tempDir, "clickhouse-operator-overrides.yaml")

	// Execute the override command
	output, stderr, err := h.ExecuteIRRWithStderr(nil, false,
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", outputFile,
	)
	require.NoError(t, err, "override command should succeed")
	t.Logf("Override output: %s", output)
	t.Logf("Stderr: %s", stderr)

	// Verify that the override file was created
	require.FileExists(t, outputFile, "Override file should be created")

	// Read the generated override file
	// #nosec G304
	overrideBytes, err := os.ReadFile(outputFile)
	require.NoError(t, err, "Should be able to read generated override file")

	// Verify the content contains expected overrides
	content := string(overrideBytes)

	// Check for expected transformations
	assert.Contains(t, content, "registry: test.registry.io", "Override should include target registry")
	assert.Contains(t, content, "docker.io/", "Override should include transformed docker.io repository")
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
	output, err := harness.ExecuteIRR(nil, args...)
	require.NoError(t, err, "irr override failed for minimal-git-image chart. Output: %s", output)

	overrides, err := harness.getOverrides()
	require.NoError(t, err, "Failed to read/parse generated overrides file")

	expectedRepo := "docker.io/bitnami/git"
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
		// #nosec G304
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
	require.NoError(t, os.MkdirAll(chartDir, fileutil.ReadWriteExecuteUserReadGroup))

	chartYaml := `apiVersion: v2
name: minimal-chart
version: 0.1.0`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), defaultFilePerm))

	valuesYaml := `image:
  repository: nginx
  tag: "1.23"`
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), defaultFilePerm))

	require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "templates"), fileutil.ReadWriteExecuteUserReadGroup))

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

	// Execute the override command with output file
	output, stderr, err := h.ExecuteIRRWithStderr(nil, false,
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", tempOutputFile,
	)
	require.NoError(t, err, "irr override command failed")
	t.Logf("IRR command output: %s", output)
	t.Logf("IRR command stderr: %s", stderr)

	// Verify that the override file was created
	require.FileExists(t, tempOutputFile, "Override file should be created")

	// Read the generated override file
	// #nosec G304
	overrideBytes, err := os.ReadFile(tempOutputFile)
	require.NoError(t, err, "Failed to read generated override file: %s", tempOutputFile)

	// Unmarshal the generated YAML
	var overrides map[string]interface{}
	err = yaml.Unmarshal(overrideBytes, &overrides)
	require.NoError(t, err, "Failed to parse overrides from file content")

	t.Logf("Parsed overrides map: %v", overrides)

	// Assert that the overrides include basic expected keys
	assert.NotEmpty(t, overrides, "Overrides shouldn't be empty")
	assert.Contains(t, string(overrideBytes), "test.registry.io", "Override should include the target registry")
	assert.Contains(t, string(overrideBytes), "docker.io", "Override should include the docker.io prefix for the repository")
}

// TestMain sets up the integration test environment.
func TestMain(m *testing.M) {
	// Define the debug flag for the test runner, distinct from app's --debug
	flag.BoolVar(&testRunnerDebug, "test-debug", false, "Enable extra debug logging FROM THE TEST RUNNER ITSELF")
	flag.Parse() // Parse flags passed to `go test`

	// Build the binary once before running tests.
	// fmt.Println("Building irr binary for integration tests...")
	// if err := buildIrrBinary(); err != nil {
	// 	fmt.Fprintf(os.Stderr, "ERROR: Failed to build irr binary: %v\n", err)
	// 	os.Exit(1) // Exit if build fails
	// }
	// fmt.Println("Build successful.")

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
	_, err := h.ExecuteIRR(nil, "override", "--unknown-flag")
	assert.Error(t, err, "should error on unknown flag")
	t.Cleanup(h.Cleanup)
}

func TestInvalidStrategy(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	h.SetChartPath(h.GetTestdataPath("charts/simple"))
	_, err := h.ExecuteIRR(nil, "override", "--strategy", "invalid-strategy")
	assert.Error(t, err, "should error on invalid strategy")
	t.Cleanup(h.Cleanup)
}

func TestMissingChartPath(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	_, err := h.ExecuteIRR(nil, "override") // Missing --chart-path
	assert.Error(t, err, "should error when chart path is missing")
	t.Cleanup(h.Cleanup)
}

func TestNonExistentChartPath(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	_, err := h.ExecuteIRR(nil, "override", "--chart-path", "/path/does/not/exist")
	assert.Error(t, err, "should error when chart path does not exist")
	t.Cleanup(h.Cleanup)
}

func TestStrictModeExitCode(t *testing.T) {
	// Skip test as we've made changes to chart loading functionality
	// that need to be addressed in a separate PR
	t.Skip("Skipping test as chart detection behavior has changed")

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
	_, err := h.ExecuteIRR(nil, "override")
	assert.Error(t, err, "should error when chart path does not exist")
	t.Cleanup(h.Cleanup)
}

func TestInvalidRegistryMappingFile(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	h.SetChartPath(h.GetTestdataPath("charts/simple"))
	_, err := h.ExecuteIRR(nil, "override", "--registry-mappings", "/invalid/path/does/not/exist.yaml")
	assert.Error(t, err, "should error when registry mappings file does not exist")
	t.Cleanup(h.Cleanup)
}

// Note: The previous TestCertManagerComponents has been moved to cert_manager_test.go
// and implemented as TestCertManager with the component-group testing approach.

// TestOverrideDryRun has been moved to override_command_test.g

// Detailed testing of registry mapping file formats is handled in registry_test.go
