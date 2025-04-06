package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
		return fmt.Errorf("failed to generate overrides: %v\nOutput: %s", err, output)
	}

	return nil
}

// ValidateOverrides checks that the generated overrides are correct
func (h *TestHarness) ValidateOverrides() error {
	// Read the generated overrides
	data, err := os.ReadFile(h.overridePath)
	if err != nil {
		return fmt.Errorf("failed to read overrides: %v", err)
	}

	var overrides map[string]interface{}
	if err := yaml.Unmarshal(data, &overrides); err != nil {
		return fmt.Errorf("failed to parse overrides: %v", err)
	}

	// Generate helm template output with and without overrides
	originalOutput, err := h.helmTemplate(nil)
	if err != nil {
		return fmt.Errorf("failed to template original: %v", err)
	}

	overriddenOutput, err := h.helmTemplate([]string{"-f", h.overridePath})
	if err != nil {
		return fmt.Errorf("failed to template with overrides: %v", err)
	}

	// Compare the outputs
	return h.compareTemplateOutputs(originalOutput, overriddenOutput)
}

// helmTemplate runs helm template and returns the output
func (h *TestHarness) helmTemplate(extraArgs []string) (string, error) {
	args := []string{"template", "test", h.chartPath}
	if extraArgs != nil {
		args = append(args, extraArgs...)
	}

	// #nosec G204 -- Test harness executes helm with test-controlled arguments
	cmd := exec.Command("helm", args...)
	cmd.Dir = h.tempDir // Run in the temp directory context
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("helm template failed: %v\nOutput: %s", err, output)
	}

	return string(output), nil
}

// compareTemplateOutputs compares helm template outputs
func (h *TestHarness) compareTemplateOutputs(original, overridden string) error {
	// Parse both outputs into structured format
	var originalDocs, overriddenDocs []map[string]interface{}

	// Split the outputs into individual YAML documents
	originalParts := strings.Split(original, "---")
	overriddenParts := strings.Split(overridden, "---")

	for _, part := range originalParts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		var doc map[string]interface{}
		if err := yaml.Unmarshal([]byte(part), &doc); err != nil {
			return fmt.Errorf("failed to parse original doc: %v", err)
		}
		originalDocs = append(originalDocs, doc)
	}

	for _, part := range overriddenParts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		var doc map[string]interface{}
		if err := yaml.Unmarshal([]byte(part), &doc); err != nil {
			return fmt.Errorf("failed to parse overridden doc: %v", err)
		}
		overriddenDocs = append(overriddenDocs, doc)
	}

	// Compare the documents
	assert.Equal(h.t, len(originalDocs), len(overriddenDocs), "Number of documents should match")

	// Compare each document, focusing on image-related fields
	for i := range originalDocs {
		h.compareImageFields(originalDocs[i], overriddenDocs[i])
	}

	return nil
}

// compareImageFields recursively compares image-related fields
func (h *TestHarness) compareImageFields(original, overridden map[string]interface{}) {
	for key, value := range original {
		switch v := value.(type) {
		case map[string]interface{}:
			if overValue, ok := overridden[key].(map[string]interface{}); ok {
				h.compareImageFields(v, overValue)
			}
		case string:
			if strings.Contains(key, "image") {
				// Verify that images from source registries are properly rewritten
				if h.isSourceRegistryImage(v) {
					overValue, ok := overridden[key].(string)
					assert.True(h.t, ok, "Image field should exist in overridden output")
					assert.True(h.t, strings.HasPrefix(overValue, h.targetReg),
						"Overridden image should use target registry")
				}
			}
		}
	}
}

// isSourceRegistryImage checks if an image is from one of the source registries
func (h *TestHarness) isSourceRegistryImage(image string) bool {
	for _, reg := range h.sourceRegs {
		if strings.HasPrefix(image, reg) {
			return true
		}
	}
	return false
}

// GetOverrides reads and parses the generated overrides file
func (h *TestHarness) GetOverrides() (map[string]interface{}, error) {
	data, err := os.ReadFile(h.overridePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read overrides: %v", err)
	}

	var overrides map[string]interface{}
	if err := yaml.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("failed to parse overrides: %v", err)
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
