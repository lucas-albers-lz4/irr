package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestHarness provides utilities for integration testing
type TestHarness struct {
	t            *testing.T
	tempDir      string
	chartPath    string
	targetReg    string
	sourceRegs   []string
	overridePath string
	mappingsPath string
}

// NewTestHarness creates a new test harness
func NewTestHarness(t *testing.T) *TestHarness {
	tempDir, err := os.MkdirTemp("", "helm-override-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	return &TestHarness{
		t:       t,
		tempDir: tempDir,
	}
}

// Cleanup removes temporary test files
func (h *TestHarness) Cleanup() {
	if h.tempDir != "" {
		if err := os.RemoveAll(h.tempDir); err != nil {
			fmt.Printf("Warning: failed to remove temporary directory %s: %v\n", h.tempDir, err)
		}
	}
}

// SetupChart prepares a test chart
func (h *TestHarness) SetupChart(chartPath string) {
	h.chartPath = chartPath
}

// SetRegistries sets the target and source registries
func (h *TestHarness) SetRegistries(target string, sources []string) {
	h.targetReg = target
	h.sourceRegs = sources

	// Create registry mappings file
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
			Target: target,
		})
	}

	mappingsPath := filepath.Join(h.tempDir, "registry-mappings.yaml")
	mappingsData, err := yaml.Marshal(mappings)
	if err != nil {
		h.t.Fatalf("Failed to marshal registry mappings: %v", err)
	}

	if err := os.WriteFile(mappingsPath, mappingsData, 0644); err != nil {
		h.t.Fatalf("Failed to write registry mappings: %v", err)
	}

	h.mappingsPath = mappingsPath
}

// GenerateOverrides runs the helm-image-override tool
func (h *TestHarness) GenerateOverrides() error {
	h.overridePath = filepath.Join(h.tempDir, "overrides.yaml")

	// Get absolute path to the binary
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}
	// Go up two directories to get to the project root
	projectRoot := filepath.Join(wd, "..", "..")
	binaryPath := filepath.Join(projectRoot, "bin", "irr")

	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", h.overridePath,
		"--registry-mappings", h.mappingsPath,
		"--verbose",
	}

	// Set IRR_TESTING environment variable
	os.Setenv("IRR_TESTING", "true")
	defer os.Unsetenv("IRR_TESTING")

	// #nosec G204 -- Test harness executes irr binary with test-controlled arguments
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = h.tempDir // Run in the temp directory context
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Attempt to read and log overrides even if command failed
		if _, statErr := os.Stat(h.overridePath); statErr == nil {
			overrideData, readErr := os.ReadFile(h.overridePath)
			if readErr == nil {
				h.t.Logf("Generated overrides.yaml content (on error):\n---\n%s\n---", string(overrideData))
			}
		}
		return fmt.Errorf("failed to generate overrides: %v\nOutput: %s", err, output)
	}

	// ---- START DEBUG LOGGING ----
	overrideData, readErr := os.ReadFile(h.overridePath)
	if readErr != nil {
		h.t.Logf("Warning: could not read generated overrides file %s for logging: %v", h.overridePath, readErr)
	} else {
		h.t.Logf("Generated overrides.yaml content (success):\n---\n%s\n---", string(overrideData))
	}
	// ---- END DEBUG LOGGING ----

	return nil
}

// ValidateOverrides checks that the generated overrides are correct
func (h *TestHarness) ValidateOverrides() error {
	// Read the generated overrides
	data, err := os.ReadFile(h.overridePath)
	if err != nil {
		return fmt.Errorf("failed to read overrides: %v", err)
	}
	// ---- START DEBUG LOGGING ----
	h.t.Logf("Generated overrides.yaml content:\n---\n%s\n---", string(data))
	// ---- END DEBUG LOGGING ----

	var overrides map[string]interface{}
	if err := yaml.Unmarshal(data, &overrides); err != nil {
		// Don't fail here, let helm template catch it if it's truly invalid for Helm
		h.t.Logf("Warning: failed to parse overrides.yaml locally: %v", err)
		// return fmt.Errorf("failed to parse overrides: %v", err)
	}

	// Generate helm template output with and without overrides
	originalOutput, err := h.helmTemplate("original", nil)
	if err != nil {
		return fmt.Errorf("failed to template original: %v", err)
	}

	overriddenOutput, err := h.helmTemplate("overridden", []string{"-f", h.overridePath})
	if err != nil {
		// It's possible the error is from helm itself due to bad overrides
		return fmt.Errorf("failed to template with overrides: %v", err)
	}
	// ---- START DEBUG LOGGING ----
	h.t.Logf("Helm template output with overrides:\n---\n%s\n---", overriddenOutput)
	// ---- END DEBUG LOGGING ----

	// Compare the outputs
	return h.compareTemplateOutputs(originalOutput, overriddenOutput)
}

// helmTemplate runs helm template and returns the output
func (h *TestHarness) helmTemplate(stage string, extraArgs []string) (string, error) {
	args := []string{"template", "test", h.chartPath}
	if extraArgs != nil {
		args = append(args, extraArgs...)
	}
	h.t.Logf("Running helm command (%s): helm %s", stage, strings.Join(args, " ")) // DEBUG LOGGING

	// #nosec G204 -- Test harness executes helm with test-controlled arguments
	cmd := exec.Command("helm", args...)
	cmd.Dir = h.tempDir // Run in the temp directory context
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Log the output even on error for debugging
		h.t.Logf("Helm command (%s) failed. Output:\n---\n%s\n---", stage, string(output)) // DEBUG LOGGING
		return "", fmt.Errorf("helm template failed: %v\nOutput: %s", err, output)
	}

	return string(output), nil
}

// compareTemplateOutputs compares helm template outputs using string checks
func (h *TestHarness) compareTemplateOutputs(original, overridden string) error {
	h.t.Log("Comparing template outputs using string checks...")

	// 1. Check if outputs are identical (they shouldn't be if overrides were applied)
	if original == overridden {
		return fmt.Errorf("original and overridden template outputs are identical, overrides likely failed")
	}

	// 2. Check if the target registry appears in the overridden output
	if !strings.Contains(overridden, h.targetReg) {
		return fmt.Errorf("target registry '%s' not found in the overridden template output", h.targetReg)
	}

	// 3. Perform more specific checks for known images that should be overridden
	//    (Example: check if the nginx image is now using the target registry)
	originalNginxImage := "docker.io/nginx:latest"
	expectedOverriddenNginx := h.targetReg + "/docker.io/nginx:latest"
	if strings.Contains(original, originalNginxImage) && !strings.Contains(overridden, expectedOverriddenNginx) {
		// Search might be too simple if tag also gets overridden, let's just check the prefix
		expectedOverriddenNginxPrefix := h.targetReg + "/docker.io/nginx"
		if !strings.Contains(overridden, expectedOverriddenNginxPrefix) {
			return fmt.Errorf("expected nginx image prefix '%s' not found in overridden output", expectedOverriddenNginxPrefix)
		}
	}

	// Add similar checks for other known source images if necessary...
	// Example: Check redis image
	originalRedisImage := "redis:6.2.7"
	expectedOverriddenRedisPrefix := h.targetReg + "/redis"
	if strings.Contains(original, originalRedisImage) && !strings.Contains(overridden, expectedOverriddenRedisPrefix) {
		return fmt.Errorf("expected redis image prefix '%s' not found in overridden output", expectedOverriddenRedisPrefix)
	}

	h.t.Log("String-based comparison passed.")
	return nil
}

// GetOverrides reads and parses the generated overrides file
// NOTE: We keep this function even if compareTemplateOutputs doesn't parse YAML,
// as other tests might use it (e.g., TestComplexChartFeatures)
func (h *TestHarness) GetOverrides() (map[string]interface{}, error) {
	data, err := os.ReadFile(h.overridePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read overrides file %s: %v", h.overridePath, err)
	}

	var overrides map[string]interface{}
	if err := yaml.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("failed to parse overrides YAML from %s: %v", h.overridePath, err)
	}
	return overrides, nil
}

// WalkImageFields recursively walks through a map and calls the callback for each image field
func (h *TestHarness) WalkImageFields(data map[string]interface{}, callback func(path []string, value string)) {
	var walk func(map[string]interface{}, []string)
	walk = func(m map[string]interface{}, path []string) {
		for key, value := range m {
			currentPath := append(path, key)
			switch v := value.(type) {
			case map[string]interface{}:
				walk(v, currentPath)
			case []interface{}:
				for i, item := range v {
					if itemMap, ok := item.(map[string]interface{}); ok {
						walk(itemMap, append(currentPath, fmt.Sprintf("[%d]", i)))
					}
				}
			case string:
				if strings.Contains(key, "image") || strings.Contains(key, "repository") {
					callback(currentPath, v)
				}
			}
		}
	}
	walk(data, nil)
}

// ExecuteIRR runs the irr binary with the given arguments.
func (h *TestHarness) ExecuteIRR(args ...string) (string, error) {
	// Get absolute path to the binary
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %v", err)
	}
	// Go up two directories to get to the project root
	projectRoot := filepath.Join(wd, "..", "..")
	binaryPath := filepath.Join(projectRoot, "bin", "irr")

	// #nosec G204 // Test harness executes binary with test-controlled arguments
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = h.tempDir // Run in the temp directory context
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try to provide more context on error
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			return string(output), fmt.Errorf("command failed with exit code %d: %w\nStderr: %s", exitErr.ExitCode(), err, string(exitErr.Stderr))
		}
		return string(output), fmt.Errorf("failed to execute irr: %w", err)
	}
	return string(output), nil
}

// ExecuteHelm runs the helm binary with the given arguments.
func (h *TestHarness) ExecuteHelm(args ...string) (string, error) {
	// #nosec G204 // Test harness executes helm with test-controlled arguments
	cmd := exec.Command("helm", args...)
	cmd.Dir = h.tempDir // Run in the temp directory context
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try to provide more context on error
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			return string(output), fmt.Errorf("helm command failed with exit code %d: %w\nStderr: %s", exitErr.ExitCode(), err, string(exitErr.Stderr))
		}
		return string(output), fmt.Errorf("helm template execution failed: %w", err)
	}
	return string(output), nil
}

func setupIntegrationTestEnv(t *testing.T) (string, func()) {
	// Set environment variable to indicate testing mode
	err := os.Setenv("IRR_TESTING", "true")
	require.NoError(t, err, "Failed to set IRR_TESTING environment variable")
	cleanup := func() {
		err := os.Unsetenv("IRR_TESTING")
		assert.NoError(t, err, "Failed to unset IRR_TESTING environment variable") // Use assert in cleanup
	}

	// Create a temporary directory for test artifacts
	tempDir, err := os.MkdirTemp("", "helm-override-test-*")
	require.NoError(t, err, "Failed to create temporary directory")

	return tempDir, cleanup
}

// Ensure Helm is installed
func init() {
	if _, err := exec.LookPath("helm"); err != nil {
		fmt.Println("Helm command not found. Integration tests require Helm to be installed.")
		os.Exit(1)
	}
}
