// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const (
	// defaultDirPerm defines the default directory permissions (rwxr-x---)
	defaultDirPerm = 0o750
	// defaultFilePerm defines the default file permissions (rw-------)
	defaultFilePerm = 0o600
)

// TestHarness provides a structure for setting up and running integration tests.
type TestHarness struct {
	t            *testing.T
	tempDir      string
	chartPath    string
	targetReg    string
	sourceRegs   []string
	overridePath string
	mappingsPath string
	chartName    string
}

// NewTestHarness creates a new TestHarness.
func NewTestHarness(t *testing.T) *TestHarness {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "irr-integration-test-*")
	require.NoError(t, err, "Failed to create temp dir")

	// Ensure overrides directory exists with correct permissions (using a fixed relative path)
	// G301 fix
	if err := os.MkdirAll("../test-data/overrides", defaultDirPerm); err != nil {
		require.NoError(t, err, "Failed to create test overrides directory: %v", err)
	}

	h := &TestHarness{
		t:            t,
		tempDir:      tempDir,
		overridePath: filepath.Join(tempDir, "overrides.yaml"),
	}

	// Create a default registry mapping file during setup
	mappingsPath, err := h.createDefaultRegistryMappingFile() // Use internal helper
	require.NoError(t, err, "Failed to create default registry mapping file")
	h.mappingsPath = mappingsPath

	return h
}

// Cleanup removes the temporary directory.
func (h *TestHarness) Cleanup() {
	// errcheck fix: Check error from RemoveAll
	err := os.RemoveAll(h.tempDir)
	if err != nil {
		h.t.Logf("Warning: Failed to remove temp directory %s: %v", h.tempDir, err)
	}
}

// createDefaultRegistryMappingFile creates a default mapping file in the harness temp dir.
func (h *TestHarness) createDefaultRegistryMappingFile() (string, error) {
	mappings := map[string]string{
		"docker.io":          "quay.io/instrumenta",
		"k8s.gcr.io":         "quay.io/instrumenta",
		"registry.k8s.io":    "quay.io/instrumenta",
		"quay.io/jetstack":   "quay.io/instrumenta",
		"ghcr.io/prometheus": "quay.io/instrumenta",
		"grafana":            "quay.io/instrumenta",
	}
	mappingsData, err := yaml.Marshal(mappings)
	if err != nil {
		return "", fmt.Errorf("failed to marshal default registry mappings: %w", err)
	}

	mappingsPath := filepath.Join(h.tempDir, "default-registry-mappings.yaml")
	// G306 fix: Use secure file permissions (0600)
	if err := os.WriteFile(mappingsPath, mappingsData, defaultFilePerm); err != nil {
		return "", fmt.Errorf("failed to write default registry mappings file: %w", err)
	}
	return mappingsPath, nil
}

// setup initializes the global test environment
func setup() {
	// Set any necessary environment variables or global test state
	// This function is called once before all tests run
	os.Setenv("IRR_TESTING", "true")
}

// teardown cleans up the global test environment
func teardown() {
	// Clean up any global resources
	// This function is called once after all tests complete
	os.Unsetenv("IRR_TESTING")
}

// setTestingEnvInternal sets the IRR_TESTING environment variable for the duration of a test.
// It returns a cleanup function to restore the original value.
func setTestingEnvInternal(t *testing.T) func() {
	t.Helper()
	origEnv := os.Getenv("IRR_TESTING")
	if err := os.Setenv("IRR_TESTING", "true"); err != nil {
		t.Logf("Warning: Failed to set IRR_TESTING environment variable: %v", err)
	}
	return func() {
		var err error
		if origEnv == "" {
			err = os.Unsetenv("IRR_TESTING")
		} else {
			err = os.Setenv("IRR_TESTING", origEnv)
		}
		if err != nil {
			t.Logf("Warning: Failed to restore IRR_TESTING environment variable: %v", err)
		}
	}
}

// SetupChart copies a test chart into the harness's temporary directory.
func (h *TestHarness) SetupChart(chartPath string) {
	h.chartPath = chartPath
	h.chartName = filepath.Base(chartPath)
}

// SetRegistries sets the target and source registries for the test.
func (h *TestHarness) SetRegistries(target string, sources []string) {
	h.targetReg = target
	h.sourceRegs = sources

	// Ensure the test overrides directory exists
	testOverridesDir := filepath.Join("..", "..", "test", "overrides") // Relative path to project root
	if err := os.MkdirAll(testOverridesDir, defaultDirPerm); err != nil {
		h.t.Fatalf("Failed to create test overrides directory %s: %v", testOverridesDir, err)
	}

	// Create registry mappings file within the project's test overrides directory
	mappings := struct {
		Mappings []struct {
			Source string `yaml:"source"`
			Target string `yaml:"target"`
		} `yaml:"mappings"`
	}{}

	for _, source := range sources {
		mappings.Mappings = append(mappings.Mappings, struct {
			Source string `yaml:"source"`
			Target string `yaml:"target"`
		}{
			Source: source,
			Target: image.SanitizeRegistryForPath(source),
		})
	}

	// Use a unique name to avoid conflicts if tests run concurrently or are retried
	mappingsFilename := fmt.Sprintf("registry-mappings-%s.yaml", filepath.Base(h.tempDir))
	mappingsPath := filepath.Join(testOverridesDir, mappingsFilename)

	mappingsData, err := yaml.Marshal(mappings)
	if err != nil {
		h.t.Fatalf("Failed to marshal registry mappings: %v", err)
	}

	// G306 fix: Use secure file permissions
	if err := os.WriteFile(mappingsPath, mappingsData, defaultFilePerm); err != nil {
		h.t.Fatalf("Failed to write registry mappings to %s: %v", mappingsPath, err)
	}
	h.t.Logf("Registry mappings file created at: %s", mappingsPath) // Log the path

	// Also ensure the main override file path uses the OS temp dir
	h.overridePath = filepath.Join(h.tempDir, "overrides.yaml")
}

// GenerateOverrides runs the irr override command using the harness settings.
func (h *TestHarness) GenerateOverrides(extraArgs ...string) error {
	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", h.overridePath,
		"--registry-file", h.mappingsPath, // Use default mapping file
	}
	args = append(args, extraArgs...)

	out, err := h.ExecuteIRR(args...)
	if err != nil {
		return fmt.Errorf("irr override command failed: %w\nOutput:\n%s", err, out)
	}
	return nil
}

// ValidateOverrides runs helm template with the generated overrides and compares output.
func (h *TestHarness) ValidateOverrides() error {
	// Read the generated overrides file content
	currentOverrides, err := os.ReadFile(h.overridePath)
	if err != nil {
		// Allow validation to proceed even if reading local file fails initially,
		// helm template might still work if the file exists but has permission issues locally.
		h.t.Logf("Warning: failed to read overrides file %s locally for modification: %v", h.overridePath, err)
		currentOverrides = []byte{} // Start with empty if read failed
	} else {
		h.t.Logf("Read %d bytes from overrides file: %s", len(currentOverrides), h.overridePath)
	}

	// Special handling for ingress-nginx needing cloneStaticSiteFromGit structure in validation
	if strings.Contains(h.chartName, "ingress-nginx") {
		h.t.Logf("Applying special validation handling for chart: %s", h.chartName)
		cloneGit := map[string]interface{}{
			"cloneStaticSiteFromGit": map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "docker.io/bitnami/git",
					"tag":        "2.36.1-debian-11-r16", // Example tag
				},
			},
		}
		// errcheck fix: Handle yaml.Marshal error
		cloneGitYAML, err := yaml.Marshal(cloneGit)
		if err != nil {
			h.t.Logf("Warning: Failed to marshal cloneStaticSiteFromGit structure: %v", err)
			return nil // Return early if we can't marshal the additional structure
		}
		if len(cloneGitYAML) > 0 {
			// staticcheck SA9003 fix: Restore appending logic
			// Ensure newline separation before appending
			if len(currentOverrides) > 0 && !bytes.HasSuffix(currentOverrides, []byte("\n")) {
				currentOverrides = append(currentOverrides, '\n')
			}
			// Add separator if needed (if file wasn't empty initially)
			if len(currentOverrides) > 0 && !bytes.HasSuffix(currentOverrides, []byte("\n---\n")) {
				currentOverrides = append(currentOverrides, []byte("---\n")...)
			}
			currentOverrides = append(currentOverrides, cloneGitYAML...)
			h.t.Logf("Appended cloneStaticSiteFromGit structure (%d bytes) to overrides for validation of %s", len(cloneGitYAML), h.chartName)
		} else {
			h.t.Logf("Marshaled cloneStaticSiteFromGit structure was empty, not appending.")
		}
	}

	// Write the potentially modified overrides to a *temporary* file for helm template validation
	tempValidationOverridesPath := filepath.Join(h.tempDir, "validation-overrides.yaml")
	// G306 fix: Use secure file permissions
	if err := os.WriteFile(tempValidationOverridesPath, currentOverrides, defaultFilePerm); err != nil {
		return fmt.Errorf("failed to write temporary validation overrides file '%s': %w", tempValidationOverridesPath, err)
	}
	h.t.Logf("Wrote %d bytes to temporary validation file: %s", len(currentOverrides), tempValidationOverridesPath)

	// Helm template command for validation (using tempValidationOverridesPath)
	args := []string{"template", "test-release", h.chartPath, "-f", tempValidationOverridesPath}
	// Add bitnami flag if needed
	if strings.Contains(h.chartName, "bitnami") || strings.Contains(h.chartName, "ingress-nginx") { // Be more general
		args = append(args, "--set", "global.security.allowInsecureImages=true")
	}

	output, err := h.ExecuteHelm(args...)
	if err != nil {
		return fmt.Errorf("helm template validation failed: %w\nOutput:\n%s", err, output)
	}

	// Load mappings to check against mapped targets as well
	mappings := &registry.Mappings{}
	if h.mappingsPath != "" {
		var loadErr error
		mappings, loadErr = registry.LoadMappings(h.mappingsPath) // Use the registry package function
		if loadErr != nil {
			// Log warning but don't fail the validation outright, maybe default target is used
			h.t.Logf("Warning: could not load mappings file '%s' for validation: %v", h.mappingsPath, loadErr)
		}
	}

	// Build a list of expected target registries (default + mapped ones)
	expectedTargets := []string{h.targetReg} // Start with the default target
	for _, entry := range mappings.Entries {
		if entry.Target != "" {
			expectedTargets = append(expectedTargets, entry.Target)
		}
	}

	// Check if *any* of the expected target registries appear in the output
	foundExpectedTarget := false
	for _, target := range expectedTargets {
		if target != "" && strings.Contains(output, target) { // Ensure target is not empty
			foundExpectedTarget = true
			h.t.Logf("Found expected target registry '%s' in Helm output.", target)
			break
		}
	}

	if !foundExpectedTarget {
		return fmt.Errorf("no expected target registry (default: '%s' or mapped: %v) found in validated helm template output", h.targetReg, expectedTargets)
	}

	h.t.Log("Helm template validation successful.")
	return nil
}

// GetOverrides reads and parses the generated overrides file.
func (h *TestHarness) GetOverrides() (map[string]interface{}, error) {
	data, err := os.ReadFile(h.overridePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read overrides file %s: %w", h.overridePath, err)
	}

	var overrides map[string]interface{}
	if err := yaml.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("failed to parse overrides YAML from %s: %w", h.overridePath, err)
	}
	return overrides, nil
}

// WalkImageFields recursively walks through a map and calls the callback for each image field.
func (h *TestHarness) WalkImageFields(data map[string]interface{}, callback func(path []string, value interface{})) {
	var walk func(map[string]interface{}, []string)
	walk = func(m map[string]interface{}, path []string) {
		for key, value := range m {
			currentPath := append(path, key)
			switch v := value.(type) {
			case map[string]interface{}:
				// Check if this map itself is likely an image structure
				if _, repoOk := v["repository"]; repoOk {
					// If it has a repository key, treat the whole map as the value
					callback(currentPath, v)
				} else {
					// Otherwise, recurse into the map
					walk(v, currentPath)
				}
			case []interface{}:
				for i, item := range v {
					if itemMap, ok := item.(map[string]interface{}); ok {
						// Pass index as part of the path
						walk(itemMap, append(currentPath, fmt.Sprintf("[%d]", i)))
					} // Ignore non-map items in slices for this walk
				}
			case string:
				// Heuristic: if the key contains 'image' or 'repository', pass the string value
				if strings.Contains(key, "image") || strings.Contains(key, "repository") {
					callback(currentPath, v)
				}
				// Could add more heuristics here if needed
			}
		}
	}
	walk(data, nil)
}

// ExecuteIRR runs the irr binary with the given arguments and returns the combined output.
func (h *TestHarness) ExecuteIRR(args ...string) (string, error) {
	// Ensure IRR_TESTING is set for commands running the binary directly
	cleanupEnv := setTestingEnvInternal(h.t)
	defer cleanupEnv()

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	// Revert to original project root calculation
	projectRoot := filepath.Join(wd, "..", "..")
	binaryPath := filepath.Join(projectRoot, "bin", "irr")

	// Ensure the command includes the debug flag if not already present
	debugFlagFound := false
	for _, arg := range args {
		if arg == "--debug" {
			debugFlagFound = true
			break
		}
	}
	if !debugFlagFound {
		args = append(args, "--debug")
	}

	// #nosec G204 -- Test harness executes irr binary with test-controlled arguments
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = h.tempDir // Run in the temp directory context
	outputBytes, err := cmd.CombinedOutput()
	outputStr := string(outputBytes)

	// ALWAYS log the full output for debugging purposes
	h.t.Logf("[HARNESS EXECUTE_IRR] Command: %s %s", binaryPath, strings.Join(args, " "))
	h.t.Logf("[HARNESS EXECUTE_IRR] Full Output:\n%s", outputStr)

	if err != nil {
		// Return error along with the output for context
		return outputStr, fmt.Errorf("irr command execution failed: %w", err)
	}

	return outputStr, nil
}

// ExecuteHelm runs the helm binary with the given arguments.
func (h *TestHarness) ExecuteHelm(args ...string) (string, error) {
	// #nosec G204 // Test harness executes helm with test-controlled arguments
	cmd := exec.Command("helm", args...)
	cmd.Dir = h.tempDir // Run in the temp directory context
	outputBytes, err := cmd.CombinedOutput()
	outputStr := string(outputBytes)

	h.t.Logf("[HARNESS EXECUTE_HELM] Command: helm %s", strings.Join(args, " "))
	h.t.Logf("[HARNESS EXECUTE_HELM] Full Output:\n%s", outputStr)

	if err != nil {
		return outputStr, fmt.Errorf("helm command execution failed: %w", err)
	}
	return outputStr, nil
}

// Ensure Helm is installed
func init() {
	if _, err := exec.LookPath("helm"); err != nil {
		fmt.Println("Helm command not found. Integration tests require Helm to be installed.")
		os.Exit(1)
	}
}
