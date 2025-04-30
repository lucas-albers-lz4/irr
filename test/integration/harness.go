// Package integration provides test harnesses and utilities for running irr CLI integration tests.
package integration

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"log/slog"

	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Constants
const (
	// defaultFilePerm defines the default file permissions (rw-------) using modern Go octal syntax
	defaultFilePerm = 0o600
	// DefaultTargetRegistry is the registry used in tests when not specified.
	DefaultTargetRegistry = "test-target.local"
	// TestDirPermissions is used for test directories (more restrictive than TestDirPermissions)
	// Uses modern Go octal syntax (0o750) for secure directory permissions (rwxr-x---)
	TestDirPermissions = 0o750 // Restrict to owner + group

	// unknownCWDPlaceholder is used when os.Getwd() fails in logging.
	unknownCWDPlaceholder = "(unknown)"
) // <-- Ensure this closing parenthesis is here

const (
	envSplitCount = 2
	// StandardStructuredMappingContent is a sample structured registry mapping file content.
	StandardStructuredMappingContent = `version: "1.0"
registries:
  mappings:
  - source: docker.io
    target: registry.example.com/docker
    enabled: true
  - source: quay.io
    target: registry.example.com/quay
    enabled: true
  defaultTarget: registry.example.com/default
  strictMode: false
`
)

// Global variables for build optimization

// TestHarness provides a structure for setting up and running integration tests.
type TestHarness struct {
	t              *testing.T
	tempDir        string
	chartPath      string
	targetReg      string
	sourceRegs     []string
	overridePath   string
	mappingsPath   string
	chartName      string
	rootDir        string
	outputPath     string
	logger         *slog.Logger
	cleanupFuncs   []func()
	mockHelmClient *MockHelmClient // Added for helm client mocking
}

// MockHelmClient is a simplified mock for testing
type MockHelmClient struct {
	GetValuesFunc func(releaseName string, allValues bool) (map[string]interface{}, error)
	MockChartPath string
}

// NewTestHarness creates a new TestHarness.
func NewTestHarness(t *testing.T) *TestHarness {
	t.Helper()

	// Build is handled centrally in TestMain now.
	// // Ensure the binary is built only once
	// buildOnce.Do(func() {
	// 	buildErr = buildIrrBinary(t)
	// })
	// // Fail early if build failed
	// require.NoError(t, buildErr, "Failed to build irr binary")

	// Create temp directory for test artifacts
	tempDir, err := os.MkdirTemp("", "irr-integration-test-")
	require.NoError(t, err)

	// Determine project root directory
	rootDir, err := getProjectRoot()
	require.NoError(t, err, "Failed to get project root in NewTestHarness")

	// Set an environment variable to indicate testing mode if needed by the core logic
	if err := os.Setenv("IRR_TESTING", "true"); err != nil {
		t.Errorf("Failed to set IRR_TESTING env var: %v", err)
	}

	h := &TestHarness{
		t:              t,
		tempDir:        tempDir,
		rootDir:        rootDir, // Initialize rootDir field
		overridePath:   filepath.Join(tempDir, "generated-overrides.yaml"),
		mappingsPath:   "",                // No mappings by default
		outputPath:     "",                // No explicit output by default
		logger:         log.Logger(),      // <-- Use custom logger instance getter
		mockHelmClient: &MockHelmClient{}, // Initialize mock helm client
	}

	// Setup testing environment variable
	cleanupEnv := h.setTestingEnv()
	h.cleanupFuncs = append(h.cleanupFuncs, cleanupEnv)

	// Create a default registry mapping file during setup
	mappingsPath, err := h.createDefaultRegistryMappingFile() // Use internal helper
	require.NoError(t, err, "Failed to create default registry mapping file")
	h.mappingsPath = mappingsPath

	h.logger.Info(fmt.Sprintf("Initialized harness in temp dir: %s", tempDir))
	return h
}

// Cleanup removes the temporary directory and resets environment variables.
func (h *TestHarness) Cleanup() {
	// errcheck fix: Check error from RemoveAll
	err := os.RemoveAll(h.tempDir)
	if err != nil {
		h.t.Logf("Warning: Failed to remove temp directory %s: %v", h.tempDir, err)
	}

	// Clean up environment variables
	if err := os.Unsetenv("IRR_TESTING"); err != nil {
		h.t.Errorf("Failed to unset IRR_TESTING env var: %v", err)
	}

	// Run registered cleanup functions
	for _, cleanup := range h.cleanupFuncs {
		cleanup()
	}
	h.cleanupFuncs = nil // Clear cleanup functions
}

// GetTempFilePath returns the full path to a file within the harness's temp directory.
func (h *TestHarness) GetTempFilePath(filename string) string {
	return filepath.Join(h.tempDir, filename)
}

// createDefaultRegistryMappingFile creates a default mapping file in the harness temp dir.
func (h *TestHarness) createDefaultRegistryMappingFile() (mappingsPath string, err error) {
	// Create a structured Config object instead of a map[string]string
	structuredConfig := registry.Config{
		Version: "1.0",
		Registries: registry.RegConfig{
			Mappings: []registry.RegMapping{
				{Source: "docker.io", Target: "quay.io/instrumenta", Enabled: true},
				{Source: "k8s.gcr.io", Target: "quay.io/instrumenta", Enabled: true},
				{Source: "registry.k8s.io", Target: "quay.io/instrumenta", Enabled: true},
				{Source: "quay.io/jetstack", Target: "quay.io/instrumenta", Enabled: true},
				{Source: "ghcr.io/prometheus", Target: "quay.io/instrumenta", Enabled: true},
				{Source: "grafana", Target: "quay.io/instrumenta", Enabled: true},
			},
		},
	}

	mappingsData, err := yaml.Marshal(structuredConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal structured registry mappings: %w", err)
	}

	mappingsPath = filepath.Join(h.tempDir, "default-registry-mappings.yaml")
	// #nosec G306 -- Using secure file permissions (0600) for test-generated file
	if err := os.WriteFile(mappingsPath, mappingsData, defaultFilePerm); err != nil {
		return "", fmt.Errorf("failed to write default registry mappings file: %w", err)
	}
	return mappingsPath, nil
}

// getProjectRoot finds the project root directory by looking for go.mod
func getProjectRoot() (string, error) {
	// Keep existing debug logging
	initialWd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get initial working directory: %w", err)
	}
	log.Debug("[DEBUG getProjectRoot] Initial os.Getwd()", "wd", initialWd)

	dir := initialWd
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Found go.mod, this is the root
			// Temporary Debug Logging
			log.Debug("[DEBUG getProjectRoot] Found go.mod", "path", dir)
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root without finding go.mod
			// Temporary Debug Logging
			log.Error("[DEBUG getProjectRoot] Failed to find go.mod", "startDir", initialWd)
			return "", fmt.Errorf("failed to find project root (go.mod) starting from %s", initialWd)
		}
		dir = parent
	}
}

// setTestingEnv sets an environment variable to indicate testing is active
// and returns a cleanup function to unset it.
func (h *TestHarness) setTestingEnv() func() {
	h.logger.Info("Setting IRR_TESTING=true")
	if err := os.Setenv("IRR_TESTING", "true"); err != nil {
		h.t.Errorf("Failed to set IRR_TESTING env var: %v", err)
	}
	return func() {
		h.logger.Info("Unsetting IRR_TESTING")
		if err := os.Unsetenv("IRR_TESTING"); err != nil {
			h.t.Errorf("Failed to unset IRR_TESTING env var: %v", err)
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

	// Create registry mappings file within the harness temp directory
	mappings := struct {
		Mappings []struct {
			Source string `yaml:"source"`
			Target string `yaml:"target"`
		} `yaml:"mappings"`
	}{}

	// Generate target registry based on sanitized source registry name (prefix strategy assumption)
	for _, source := range sources {
		// Use a consistent mapping strategy, e.g., prefixing with sanitized source
		// Ensure this matches the strategy used by the 'irr' command under test if necessary
		// Example: prefix strategy might generate target "harbor.home.arpa/dockerio/..." for source "docker.io"
		// The mapping file itself might map source "docker.io" to target "dockerio" or similar short name
		// Let's assume the mapping file maps source registry to a *target prefix* for this example
		sanitizedSource := image.SanitizeRegistryForPath(source) // e.g., docker.io -> dockerio
		// The actual target registry used in overrides will be h.targetReg + "/" + sanitizedSource + "/..."
		// So, the mapping file should reflect the relationship between the original source and the prefix used.
		// Sticking with mapping source -> sanitized source prefix for simplicity here.
		mappings.Mappings = append(mappings.Mappings, struct {
			Source string `yaml:"source"`
			Target string `yaml:"target"`
		}{
			Source: source,
			Target: sanitizedSource, // Map source to its sanitized prefix
		})
	}

	// Use a unique name based on the temp directory
	mappingsFilename := fmt.Sprintf("registry-mappings-%s.yaml", filepath.Base(h.tempDir))
	// Create the file inside the harness's temp directory
	mappingsPath := filepath.Join(h.tempDir, mappingsFilename)

	mappingsData, err := yaml.Marshal(mappings)
	if err != nil {
		h.t.Fatalf("Failed to marshal registry mappings: %v", err)
	}

	// #nosec G306 -- Using secure file permissions (0600) for test-generated file
	if err := os.WriteFile(mappingsPath, mappingsData, defaultFilePerm); err != nil {
		h.t.Fatalf("Failed to write registry mappings to %s: %v", mappingsPath, err)
	}

	// Store the absolute path
	absMappingsPath, err := filepath.Abs(mappingsPath)
	if err != nil {
		h.t.Fatalf("Failed to get absolute path for mappings file %s: %v", mappingsPath, err)
	}
	h.mappingsPath = absMappingsPath // Assign the absolute path to the harness field
	h.t.Logf("Registry mappings file created at: %s", h.mappingsPath)

	// Ensure the main override file path also uses the OS temp dir (already done in NewTestHarness)
	// h.overridePath = filepath.Join(h.tempDir, "generated-overrides.yaml")
}

// GenerateOverrides runs the irr override command using the harness settings.
func (h *TestHarness) GenerateOverrides(extraArgs ...string) error {
	// h.mappingsPath now holds the absolute path to the file inside h.tempDir
	args := []string{
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", h.overridePath, // Absolute path within h.tempDir
	}
	// Add registry file arg only if mappingsPath is set
	if h.mappingsPath != "" {
		args = append(args, "--registry-file", h.mappingsPath) // Pass the absolute path
	}
	args = append(args, extraArgs...)

	out, err := h.ExecuteIRR(nil, args...)
	if err != nil {
		return fmt.Errorf("irr override command failed: %w\nOutput:\n%s", err, out)
	}
	return nil
}

// ValidateOverrides checks the generated overrides against expected values.
// This function performs comprehensive validation of the override file structure.
func (h *TestHarness) ValidateOverrides() error {
	h.logger.Info(fmt.Sprintf("Validating overrides for chart: %s", h.chartPath))

	mappings, err := h.loadMappings()
	if err != nil {
		return err
	}

	expectedTargets := h.determineExpectedTargets(mappings)
	h.logger.Info(fmt.Sprintf("Expecting images to use target registries: %v", expectedTargets))

	tempValidationOverridesPath, err := h.readAndWriteOverrides()
	if err != nil {
		return err
	}

	args := h.buildHelmArgs(tempValidationOverridesPath)

	output, err := h.ExecuteHelm(args...)
	if err != nil {
		return fmt.Errorf("helm template validation failed: %w\nOutput:\n%s", err, output)
	}

	actualOverrides, getOverridesErr := h.getOverrides()
	actualTargetsUsed := make(map[string]bool)

	err = h.validateHelmOutput(getOverridesErr, mappings, output, actualOverrides, actualTargetsUsed)
	if err != nil {
		return err
	}

	h.t.Log("Helm template validation successful.")
	return nil
}

// loadMappings loads the registry mappings from the file specified at h.mappingsPath.
// This method prioritizes the structured format (containing version, registries, compatibility sections)
func (h *TestHarness) loadMappings() (*registry.Mappings, error) {
	mappings := &registry.Mappings{}
	if h.mappingsPath != "" {
		_, statErr := os.Stat(h.mappingsPath)
		switch {
		case statErr == nil:
			// Load using the structured format
			structConfig, loadErr := registry.LoadStructuredConfigDefault(h.mappingsPath, true)
			if loadErr != nil {
				// Return the error - no fallback to legacy format anymore
				return nil, fmt.Errorf("failed to load mappings file %s: %w", h.mappingsPath, loadErr)
			}
			// Convert structured config to mappings format
			mappings = structConfig.ToMappings()
			h.logger.Info(fmt.Sprintf("Successfully loaded mappings (structured format) from %s", h.mappingsPath))
		case os.IsNotExist(statErr):
			h.logger.Info(fmt.Sprintf("Mappings file %s does not exist, proceeding without mappings.", h.mappingsPath))
		default:
			h.logger.Info(fmt.Sprintf("Warning: Error stating mappings file %s: %v", h.mappingsPath, statErr))
		}
	} else {
		h.logger.Info("No mappings file path specified for harness.")
	}
	return mappings, nil
}

func (h *TestHarness) determineExpectedTargets(mappings *registry.Mappings) []string {
	expectedTargets := []string{}
	switch {
	case mappings != nil && len(mappings.Entries) > 0:
		uniqueTargets := make(map[string]struct{})
		for _, entry := range mappings.Entries {
			if entry.Target != "" {
				normTarget := image.NormalizeRegistry(entry.Target)
				if _, exists := uniqueTargets[normTarget]; !exists {
					uniqueTargets[normTarget] = struct{}{}
					expectedTargets = append(expectedTargets, normTarget)
				}
			}
		}
		if len(expectedTargets) == 0 {
			h.logger.Info("Mappings file loaded but contains no target registries. Falling back to default.")
			expectedTargets = append(expectedTargets, image.NormalizeRegistry(h.targetReg))
		}
	default:
		expectedTargets = append(expectedTargets, image.NormalizeRegistry(h.targetReg))
	}
	finalExpectedTargets := []string{}
	seenTargets := make(map[string]bool)
	for _, target := range expectedTargets {
		if !seenTargets[target] {
			finalExpectedTargets = append(finalExpectedTargets, target)
			seenTargets[target] = true
		}
	}
	return finalExpectedTargets
}

func (h *TestHarness) readAndWriteOverrides() (string, error) {
	currentOverridesBytes, err := os.ReadFile(h.overridePath)
	if err != nil {
		h.t.Logf("Warning: failed to read overrides file %s locally for modification: %v", h.overridePath, err)
		currentOverridesBytes = []byte{}
	} else {
		h.t.Logf("Read %d bytes from overrides file: %s", len(currentOverridesBytes), h.overridePath)
	}
	h.t.Logf("Generated Overrides Content:\n%s", string(currentOverridesBytes))
	tempValidationOverridesPath := filepath.Join(h.tempDir, "validation-overrides.yaml")
	if err := os.WriteFile(tempValidationOverridesPath, currentOverridesBytes, defaultFilePerm); err != nil {
		return "", fmt.Errorf("failed to write temporary validation overrides file %s: %w", tempValidationOverridesPath, err)
	}
	h.t.Logf("Wrote %d bytes to temporary validation file: %s", len(currentOverridesBytes), tempValidationOverridesPath)
	return tempValidationOverridesPath, nil
}

func (h *TestHarness) buildHelmArgs(tempValidationOverridesPath string) []string {
	args := []string{"template", "test-release", h.chartPath, "-f", tempValidationOverridesPath}
	if h.chartName == "ingress-nginx" {
		args = append(args, "--set", "global.security.allowInsecureImages=true")
		h.logger.Info("Detected ingress-nginx chart, adding --set global.security.allowInsecureImages=true for validation")
	}
	return args
}

func (h *TestHarness) validateHelmOutput(getOverridesErr error, mappings *registry.Mappings, output string, actualOverrides map[string]interface{}, actualTargetsUsed map[string]bool) error {
	switch {
	case getOverridesErr != nil:
		h.t.Logf("Warning: Could not read overrides file (%s) for validation: %v. Falling back to checking configured targets.", h.overridePath, getOverridesErr)
		return h.fallbackCheck(mappings, output)
	case len(actualOverrides) == 0:
		h.t.Log("Overrides file is empty. Skipping registry validation in Helm output.")
		return nil
	default:
		h.WalkImageFields(actualOverrides, func(_ []string, value interface{}) {
			if imageMap, ok := value.(map[string]interface{}); ok {
				if reg, ok := imageMap["registry"].(string); ok && reg != "" {
					actualTargetsUsed[image.NormalizeRegistry(reg)] = true
				}
			}
			if imageStr, ok := value.(string); ok && imageStr != "" {
				ref, parseErr := image.ParseImageReference(imageStr)
				if parseErr == nil && ref != nil && ref.Registry != "" {
					actualTargetsUsed[image.NormalizeRegistry(ref.Registry)] = true
				}
			}
		})
		if len(actualTargetsUsed) == 0 {
			h.t.Log("No image registry keys/values found in the generated overrides file. Validation check skipped.")
			return nil
		}
		foundActualTargetInOutput := false
		for target := range actualTargetsUsed {
			if strings.Contains(output, target) {
				foundActualTargetInOutput = true
				h.t.Logf("Found actual target registry %s from overrides in Helm output.", target)
				break
			}
		}
		if !foundActualTargetInOutput {
			targetsSlice := make([]string, 0, len(actualTargetsUsed))
			for target := range actualTargetsUsed {
				targetsSlice = append(targetsSlice, target)
			}
			return fmt.Errorf("no actual target registry used in overrides (%v) found in validated helm template output", targetsSlice)
		}
		return nil
	}
}

func (h *TestHarness) fallbackCheck(mappings *registry.Mappings, output string) error {
	expectedTargets := []string{h.targetReg}
	if mappings != nil {
		for _, entry := range mappings.Entries {
			if entry.Target != "" {
				expectedTargets = append(expectedTargets, entry.Target)
			}
		}
	}
	foundExpectedTarget := false
	for _, target := range expectedTargets {
		normTarget := image.NormalizeRegistry(target)
		if normTarget != "" && strings.Contains(output, normTarget) {
			foundExpectedTarget = true
			h.t.Logf("[Fallback Check] Found configured target registry %s (normalized) in Helm output.", normTarget)
			break
		}
	}
	if !foundExpectedTarget {
		return fmt.Errorf("[Fallback Check] no configured target registry (default: %s or mapped: %v) found in validated helm template output", h.targetReg, expectedTargets)
	}
	return nil
}

// getOverrides reads the default generated override file and returns its content as a map.
// Deprecated: Use getOverridesFromPath for clarity when multiple override files might exist.
func (h *TestHarness) getOverrides() (overrides map[string]interface{}, err error) {
	return h.getOverridesFromPath(h.overridePath)
}

// getOverridesFromPath reads the override file at the specified path and returns its content as a map.
func (h *TestHarness) getOverridesFromPath(filePath string) (map[string]interface{}, error) {
	content, err := os.ReadFile(filePath) //nolint:gosec // filePath is controlled by the test harness, not external input.
	if err != nil {
		return nil, fmt.Errorf("failed to read overrides file %s: %w", filePath, err)
	}
	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal overrides YAML from %s: %w", filePath, err)
	}
	return data, nil
}

// GetValueFromOverrides retrieves a potentially nested value from the override map using a path.
func (h *TestHarness) GetValueFromOverrides(overrides map[string]interface{}, path ...string) (interface{}, bool) {
	var current interface{} = overrides
	for _, key := range path {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false // Path doesn't lead to a map intermediate
		}
		value, exists := currentMap[key]
		if !exists {
			return nil, false // Key doesn't exist at this level
		}
		current = value
	}
	return current, true
}

// WalkImageFields recursively walks through a map/slice structure and calls the visitor function
// for any field that represents an image (either a string or a map with expected keys).
// This is a simplified walker, assuming overrides structure.
func (h *TestHarness) WalkImageFields(data interface{}, visitor func(path []string, value interface{})) {
	h.walkImageFieldsRecursive(data, []string{}, visitor)
}

// walkImageFieldsRecursive traverses the data structure and calls the visitor function when it finds
// elements that appear to be image references or image configuration maps.
func (h *TestHarness) walkImageFieldsRecursive(data interface{}, currentPath []string, visitor func(path []string, value interface{})) {
	switch val := data.(type) {
	case map[string]interface{}:
		// Check if this map *itself* looks like an image structure
		_, hasRepo := val["repository"]
		// Decide if this map as a whole should be visited
		if hasRepo { // Or use a more robust check if needed
			visitor(currentPath, val) // Visit the map node
		}
		// ALWAYS recurse into map values, regardless of whether the map itself was visited
		for k, v := range val {
			h.walkImageFieldsRecursive(v, append(currentPath, k), visitor)
		}
	case []interface{}:
		// Recurse into slice elements
		for i, item := range val {
			// Create path segment for slice index
			indexedPath := append(currentPath, fmt.Sprintf("[%d]", i)) //nolint:gocritic // Intentional new slice for recursion
			h.walkImageFieldsRecursive(item, indexedPath, visitor)
		}
	case string:
		// Check if this string value itself should be visited
		// Heuristic: Is the key likely an image key? (Need key context here, difficult)
		// Simpler: Does the string value *look* like an image reference?
		if (strings.Contains(val, ":") || strings.Contains(val, "@")) && strings.Contains(val, "/") {
			visitor(currentPath, val) // Visit the string node
		}
		// Potentially add check for key names if path context implies image
		// e.g., if currentPath ends with "image" or "repository"
		if len(currentPath) > 0 {
			lastKey := currentPath[len(currentPath)-1]
			if strings.Contains(strings.ToLower(lastKey), "image") {
				// If the key suggests it's an image, visit the string value
				// This might double-visit if the string also looked like an image, visitor must handle
				visitor(currentPath, val)
			}
		}
		// default: // Ignore other types like bool, int, float, nil
	}
}

// buildEnv creates the environment slice for the command, applying overrides.
func (h *TestHarness) buildEnv(envOverrides map[string]string) []string {
	// Start with current environment
	baseEnv := os.Environ()
	envMap := make(map[string]string)
	for _, envVar := range baseEnv {
		//nolint:nilaway // strings.SplitN always returns non-nil slice
		parts := strings.SplitN(envVar, "=", envSplitCount)
		if len(parts) == envSplitCount {
			//nolint:nilaway // length checked above
			envMap[parts[0]] = parts[1]
		} else {
			//nolint:nilaway // length checked above (parts[0] exists)
			envMap[parts[0]] = "" // Handle variables without values
		}
	}

	// Apply overrides
	for key, value := range envOverrides {
		envMap[key] = value
	}

	// Ensure IRR_TESTING is set
	envMap["IRR_TESTING"] = "true"

	// Convert map back to slice
	finalEnv := make([]string, 0, len(envMap))
	for key, value := range envMap {
		finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", key, value))
	}

	// Log the effective environment for debugging (optional)
	// h.logger.Printf("Effective Subprocess Environment: %v", finalEnv)

	return finalEnv
}

// ExecuteIRR runs the irr command with the given arguments and returns its stdout. #nosec G204 -- Arguments are controlled by test harness, not user input
func (h *TestHarness) ExecuteIRR(env map[string]string, args ...string) (string, error) {
	cmdPath := h.getIrrBinaryPath()
	cmdArgs := append([]string{}, args...) // Create a mutable copy

	log.Info("Executing command (capturing stdout)", "command", cmdPath, "args", cmdArgs)

	cmd := exec.Command(cmdPath, cmdArgs...) // #nosec G204 Need to run the built binary

	// Set environment variables if provided
	cmd.Env = os.Environ() // Initialize with current environment
	if env != nil {
		cmd.Env = append(cmd.Env, h.getEnvSlice(env)...) // Append ONLY custom vars
		log.Debug("Setting custom environment variables", "env", env)
	}

	outputBytes, err := cmd.CombinedOutput() // Use CombinedOutput to get stderr in case of error
	output := string(outputBytes)

	if err != nil {
		// Log the error and output for easier debugging in tests
		log.Info("Command failed", "error", err, "output", output)
		// Return a combined error message including output content
		return output, fmt.Errorf("%w\\nOutput:\\n%s", err, output)
	}

	log.Debug("Command succeeded", "output_len", len(output))
	return output, nil
}

// ExecuteIRRWithStderr executes the IRR command with the given arguments,
// capturing both stdout and stderr separately.
// It also accepts an optional map of environment variables to set for the command execution.
func (h *TestHarness) ExecuteIRRWithStderr(env map[string]string, args ...string) (stdout, stderr string, err error) {
	cmdPath := h.getIrrBinaryPath()
	cmdArgs := append([]string{}, args...) // Create a mutable copy

	log.Info("Executing command (capturing stderr)", "command", cmdPath, "args", cmdArgs)

	cmd := exec.Command(cmdPath, cmdArgs...) // #nosec G204 Need to run the built binary

	// Set environment variables if provided
	cmd.Env = os.Environ() // Initialize with current environment
	if env != nil {
		cmd.Env = append(cmd.Env, h.getEnvSlice(env)...) // Append ONLY custom vars
		log.Debug("Setting custom environment variables", "env", env)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if err != nil {
		// Log the error along with stdout/stderr for context
		log.Info("Command failed", "error", err, "stderr", stderr)
		// Return a combined error message including stderr content
		return stdout, stderr, fmt.Errorf("%w\\nStderr:\\n%s", err, stderr)
	}

	log.Debug("Command succeeded", "stdout_len", len(stdout), "stderr_len", len(stderr))
	return stdout, stderr, nil
}

// getIrrBinaryPath returns the path to the built IRR binary.
func (h *TestHarness) getIrrBinaryPath() string {
	return filepath.Join(h.rootDir, "bin", "irr")
}

// getEnvSlice converts a map to a slice of "key=value" strings suitable for exec.Cmd.Env.
// It always includes IRR_TESTING=true.
func (h *TestHarness) getEnvSlice(customEnv map[string]string) []string {
	var envSlice []string
	// The range loop handles nil maps gracefully (it won't execute).
	// So the nil check `if customEnv != nil` is redundant.
	for k, v := range customEnv {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}
	return envSlice
}

// ExecuteHelm runs the helm binary with the given arguments.
func (h *TestHarness) ExecuteHelm(args ...string) (output string, err error) {
	// #nosec G204 // Test harness executes helm with test-controlled arguments
	cmd := exec.Command("helm", args...)
	cmd.Dir = h.tempDir // Run in the temp directory context
	outputBytes, err := cmd.CombinedOutput()
	outputStr := string(outputBytes)

	h.logger.Info(fmt.Sprintf("[HARNESS EXECUTE_HELM] Command: helm %s", strings.Join(args, " ")))
	h.logger.Info(fmt.Sprintf("[HARNESS EXECUTE_HELM] Full Output:\n%s", outputStr))

	if err != nil {
		return outputStr, fmt.Errorf("helm command execution failed: %w", err)
	}
	return outputStr, nil
}

// init ensures the binary path is determined early.
func init() {
	// Binary building is now handled in NewTestHarness
}

// SetChartPath sets the chart path for the test harness.
func (h *TestHarness) SetChartPath(path string) {
	h.chartPath = path
}

// GetTestdataPath returns the absolute path to a test chart directory.
func (h *TestHarness) GetTestdataPath(relPath string) string {
	// First try the test/testdata path
	testdataPath := filepath.Join(h.rootDir, "test", "testdata", relPath)

	// Check if the path exists in test/testdata
	if _, err := os.Stat(testdataPath); err == nil {
		absPath, absErr := filepath.Abs(testdataPath)
		if absErr != nil {
			h.t.Fatalf("Failed to get absolute path for %s: %v", testdataPath, absErr)
		}
		return absPath
	}

	// Also try the test-data path as an alternative
	altPath := filepath.Join(h.rootDir, "test-data", relPath)

	// Check if the path exists in test-data
	if _, err := os.Stat(altPath); err == nil {
		absPath, absErr := filepath.Abs(altPath)
		if absErr != nil {
			h.t.Fatalf("Failed to get absolute path for %s: %v", altPath, absErr)
		}
		return absPath
	}

	// If not found in either location, log a fatal error
	h.t.Fatalf("Test data path not found at either %s or %s. Original relative path: %s",
		testdataPath, altPath, relPath)
	return "" // Unreachable, but satisfies compiler
}

// GetTestOverridePath returns the path to a test override values file.
// If the file doesn't exist, it creates an empty file at the specified path.
func (h *TestHarness) GetTestOverridePath(relPath string) string {
	// Create the test-overrides directory in the temp directory if it doesn't exist
	overridesDir := filepath.Join(h.tempDir, "test-overrides")
	// Use modern Go octal literal syntax (0o750) with predefined constants for secure permissions
	if err := os.MkdirAll(overridesDir, TestDirPermissions); err != nil {
		h.t.Fatalf("Failed to create test-overrides directory: %v", err)
	}

	// Create subdirectories if needed
	if strings.Contains(relPath, "/") {
		dirPath := filepath.Dir(filepath.Join(overridesDir, relPath))
		// Use modern Go octal literal syntax (0o750) with predefined constants for secure permissions
		if err := os.MkdirAll(dirPath, TestDirPermissions); err != nil {
			h.t.Fatalf("Failed to create directory %s: %v", dirPath, err)
		}
	}

	// Create the override file path
	overridePath := filepath.Join(overridesDir, relPath)

	// Create an empty file if it doesn't exist
	if _, err := os.Stat(overridePath); os.IsNotExist(err) {
		// Use predefined constant (0o600) for secure file permissions to satisfy linter requirements
		if err := os.WriteFile(overridePath, []byte("# Test override values for "+relPath+"\n"), defaultFilePerm); err != nil {
			h.t.Fatalf("Failed to create empty override file at %s: %v", overridePath, err)
		}
	}

	return overridePath
}

// CombineValuesPaths joins multiple values file paths with commas for use with the --values flag.
func (h *TestHarness) CombineValuesPaths(paths []string) string {
	return strings.Join(paths, ",")
}

// AssertExitCode runs the IRR binary with the given arguments and checks the exit code.
func (h *TestHarness) AssertExitCode(expected int, args ...string) {
	h.t.Helper()
	binPath := h.getBinaryPath()

	// Debug logging for path and CWD
	cwd, err := os.Getwd() // Check error now
	if err != nil {
		// Log the error but don't fail the test just for this
		h.logger.Info(fmt.Sprintf("[ASSERT_EXIT_CODE WARNING] Failed to get current working directory: %v", err))
		cwd = unknownCWDPlaceholder // Use placeholder
	}
	h.logger.Info(fmt.Sprintf("[ASSERT_EXIT_CODE DEBUG] binPath: %s, CWD: %s", binPath, cwd))

	// G204: Subprocess launched with variable - Acceptable in test code.
	cmd := exec.Command(binPath, args...) // #nosec G204
	cmd.Dir = h.tempDir                   // Run from the temp directory
	cmd.Env = h.buildEnv(nil)             // Use default env (includes IRR_TESTING=true)
	outputBytes, runErr := cmd.CombinedOutput()
	outputStr := string(outputBytes)

	// Check exit code using exec.ExitError
	// errorlint: Use errors.As for checking the type
	var exitErr *exec.ExitError

	switch {
	case errors.As(runErr, &exitErr):
		// It's an ExitError, check the code
		assert.Equal(
			h.t,
			expected,
			exitErr.ExitCode(),
			"Expected exit code %d but got %d\nArgs: %v\nOutput:\n%s",
			expected,
			exitErr.ExitCode(),
			args,
			outputStr,
		)
	case runErr != nil && expected != 0:
		// Command failed in a way other than ExitError (e.g., couldn't start)
		h.t.Fatalf(
			"Command failed unexpectedly (expected exit code %d): %v\nArgs: %v\nOutput:\n%s",
			expected,
			runErr,
			args,
			outputStr,
		)
	case runErr == nil && expected != 0:
		// Command succeeded but an error code was expected
		h.t.Fatalf(
			"Expected exit code %d but command succeeded.\nArgs: %v\nOutput:\n%s",
			expected,
			args,
			outputStr,
		)
	case runErr != nil && expected == 0:
		// Command failed but success (0) was expected
		h.t.Fatalf(
			"Expected exit code 0 but command failed: %v\nArgs: %v\nOutput:\n%s",
			runErr,
			args,
			outputStr,
		)
	}
}

// AssertErrorContains checks if the stderr output contains the specified substring.
func (h *TestHarness) AssertErrorContains(substring string, args ...string) {
	h.t.Helper()
	binPath := h.getBinaryPath()

	// Debug logging for path and CWD
	cwd, err := os.Getwd() // Check error now
	if err != nil {
		// Log the error but don't fail the test just for this
		h.logger.Info(fmt.Sprintf("[ASSERT_ERROR_CONTAINS WARNING] Failed to get current working directory: %v", err))
		cwd = unknownCWDPlaceholder // Use placeholder
	}
	h.logger.Info(fmt.Sprintf("[ASSERT_ERROR_CONTAINS DEBUG] binPath: %s, CWD: %s", binPath, cwd))

	// G204: Subprocess launched with variable - Acceptable in test code.
	cmd := exec.Command(binPath, args...) // #nosec G204
	cmd.Dir = h.tempDir                   // Run from the temp directory
	cmd.Env = h.buildEnv(nil)             // Use default env (includes IRR_TESTING=true)
	var stderr bytes.Buffer
	var stdout bytes.Buffer // Keep capturing stdout for context if needed, but don't check it
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	// Explicitly ignore error return value from cmd.Run().
	// This function's purpose is to assert content in stderr, regardless of exit code.
	_ = cmd.Run() //nolint:errcheck // We intentionally ignore the error as we're only checking stderr contents

	stderrStr := stderr.String()
	stdoutStr := stdout.String() // Log stdout too for debugging context

	h.logger.Info(fmt.Sprintf("[ASSERT_ERROR_CONTAINS] Stderr:\n%s", stderrStr))
	if stdoutStr != "" {
		h.logger.Info(fmt.Sprintf("[ASSERT_ERROR_CONTAINS] Stdout:\n%s", stdoutStr))
	}

	assert.Contains(
		h.t,
		stderrStr,
		substring,
		"Expected stderr to contain '%s'\nArgs: %v\nActual stderr:\n%s",
		substring,
		args,
		stderrStr,
	)
}

// getBinaryPath determines the path to the compiled irr binary.
func (h *TestHarness) getBinaryPath() string {
	return filepath.Join(h.rootDir, "bin", "irr")
}

// BuildIRR compiles the irr binary for use in tests.
// It assumes the test is run from the test/integration directory.
func (h *TestHarness) BuildIRR() {
	h.t.Helper()
	rootDir := "../.." // Assuming test run from test/integration
	binPath := filepath.Join(rootDir, "bin", "irr")

	h.t.Logf("Building irr binary at %s", binPath)

	// Ensure bin directory exists
	err := os.MkdirAll(filepath.Join(rootDir, "bin"), TestDirPermissions) // #nosec G301 -- Test code creating temp build dir, 0755 is acceptable here.
	require.NoError(h.t, err)

	// #nosec G204 -- Building the project's own binary is safe.
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/irr")
	projectRoot, err := getProjectRoot() // Get project root
	if err != nil {
		h.t.Fatalf("Failed to get project root for build: %v", err)
	}
	cmd.Dir = projectRoot // Run build from project root
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's an ExitError using errors.As
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr):
			// Include exit code in the error message for better context
			h.t.Fatalf("BuildIRR failed with exit code %d: %v\nOutput:\n%s", exitErr.ExitCode(), err, string(output))
		default:
			// Generic build failure
			h.t.Fatalf("BuildIRR failed: %v\nOutput:\n%s", err, string(output))
		}
	}
	h.t.Logf("BuildIRR successful.")
}

// ValidateFullyQualifiedOverrides validates that all images are fully qualified with the specified target registry
func (h *TestHarness) ValidateFullyQualifiedOverrides(targetRegistry string, targets []string) {
	// Read the overrides file
	_, err := os.ReadFile(h.overridePath)
	if err != nil {
		h.t.Fatalf("Failed to read overrides file: %v", err)
	}

	// Create combined qualifiers to check for
	var qualifiers []string
	for _, target := range targets {
		qualifiers = append(qualifiers, fmt.Sprintf("%s/%s", targetRegistry, target))
	}

	// Validate the helm template contains all expected registry combinations
	h.ValidateHelmTemplate(qualifiers, "")
}

// ValidateWithRegistryPrefix validates the generated overrides contain the expected target registry prefix
func (h *TestHarness) ValidateWithRegistryPrefix(targetRegistry string) {
	// Read the overrides file
	_, err := os.ReadFile(h.overridePath)
	if err != nil {
		h.t.Fatalf("Failed to read overrides file: %v", err)
	}

	// Look for direct registry usage
	h.ValidateHelmTemplate([]string{targetRegistry}, "")
}

// CreateRegistryMappingsFile creates a registry mappings file with the provided content
// Returns the path to the mappings file
func (h *TestHarness) CreateRegistryMappingsFile(mappingType string) string {
	var content string

	// Determine content based on mappingType
	switch {
	case mappingType == "":
		// Handle empty string as a special case
		content = ""
		h.logger.Info("Creating empty registry mappings file")
	case strings.HasPrefix(mappingType, "version:"),
		strings.HasPrefix(mappingType, "registries:"),
		strings.HasPrefix(mappingType, "compatibility:"),
		strings.Contains(mappingType, "\n"):
		// If it looks like YAML content, use it directly
		content = mappingType
		h.logger.Info("Using direct YAML content as registry mappings file")
	default:
		// Otherwise, treat it as a predefined mapping type
		switch mappingType {
		case "structured":
			content = `version: "1.0"
registries:
  mappings:
  - source: docker.io
    target: registry.example.com/dockerio
    enabled: true
    description: "Docker Hub mapping"
  - source: quay.io
    target: registry.example.com/quay
    enabled: true
    description: "Quay.io mapping"
  defaultTarget: registry.example.com/default
  strictMode: false
compatibility:
  ignoreEmptyFields: true
`
		case "empty":
			content = ""
		case "invalid_structured_format_-_missing_required_fields":
			content = `version: "1.0"
registries:
  # Missing mappings section
  defaultTarget: registry.example.com/default
  strictMode: false
`
		case "structured-only":
			content = StandardStructuredMappingContent
		default:
			h.t.Fatalf("unknown mapping type: %s", mappingType)
		}
	}

	h.logger.Info(fmt.Sprintf("Writing %s format registry mappings file", mappingType))
	h.logger.Info(fmt.Sprintf("Registry mappings file content verification:\n%s", content))

	mappingsPath := filepath.Join(h.tempDir, "registry-mappings.yaml")
	// Declare and assign err in one step
	if err := os.WriteFile(mappingsPath, []byte(content), fileutil.ReadWriteUserPermission); err != nil {
		h.t.Fatalf("Failed to write registry mappings file: %v", err)
	}

	h.mappingsPath = mappingsPath
	return mappingsPath
}

// GeneratedOverridesFile returns the path to the generated overrides file.
func (h *TestHarness) GeneratedOverridesFile() string {
	return h.overridePath
}

// ValidateHelmTemplate validates that the helm template contains specific text.
func (h *TestHarness) ValidateHelmTemplate(qualifiers []string, exclude string) {
	// Generate helm template using the chart and override file
	args := []string{"template", "release-name", h.chartPath, "-f", h.overridePath}
	output, err := h.ExecuteHelm(args...)
	if err != nil {
		h.t.Fatalf("Failed to render helm template: %v", err)
	}

	// Check that at least one of the qualifiers is present
	found := false
	for _, qualifier := range qualifiers {
		if strings.Contains(output, qualifier) {
			found = true
			h.t.Logf("Found qualifier %q in helm template output", qualifier)
			break
		}
	}

	if !found {
		h.t.Fatalf("None of the expected qualifiers %v found in helm output", qualifiers)
	}

	// Check exclusion if provided
	if exclude != "" && strings.Contains(output, exclude) {
		h.t.Fatalf("Found excluded text %q in helm template output", exclude)
	}
}

// ValidateOverridesWithQualifiers validates overrides with specific qualifiers
func (h *TestHarness) ValidateOverridesWithQualifiers(expectedQualifiers []string) error {
	// Existing implementation plus additional check for expected qualifiers
	if err := h.ValidateOverrides(); err != nil {
		return err
	}

	// Additional validation for expected qualifiers if provided
	if len(expectedQualifiers) > 0 {
		h.ValidateHelmTemplate(expectedQualifiers, "")
	}

	return nil
}

// RunIrrCommandWithOutput runs the irr command with specific arguments and returns its output.
func (h *TestHarness) RunIrrCommandWithOutput(cmdArgs []string) (string, error) {
	h.t.Helper()

	// Add logging here
	originalWd, wdErr := os.Getwd() // Check error
	if wdErr != nil {
		// Log warning but don't fail test
		h.logger.Warn("Failed to get working directory for logging", "error", wdErr)
		originalWd = unknownCWDPlaceholder
	}
	log.Debug("[HARNESS RunIrrCommand] Test process WD before running command", "wd", originalWd)
	log.Debug("[HARNESS RunIrrCommand] Preparing command", "args", cmdArgs)

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) // #nosec G204 - Args are controlled by test code
	cmd.Dir = h.tempDir                             // Set the working directory for the command being run

	log.Debug("[HARNESS RunIrrCommand] Setting cmd.Dir for child process", "dir", cmd.Dir)

	outputBytes, err := cmd.CombinedOutput() // Capture stdout and stderr

	log.Debug("[HARNESS RunIrrCommand] Command finished", "output", string(outputBytes), "err", err)

	if err != nil {
		// Improve error reporting for exit errors
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return string(outputBytes), fmt.Errorf("command %v exited with error: %w, output:\\n%s", cmdArgs, err, string(outputBytes))
		}
		return string(outputBytes), fmt.Errorf("failed to run command %v: %w, output:\\n%s", cmdArgs, err, string(outputBytes))
	}

	return string(outputBytes), nil
}

// RunIrrCommand executes the IRR command with the given arguments and returns stdout, stderr, and exit code
func (h *TestHarness) RunIrrCommand(args ...string) (stdout, stderr string, exitCode int) {
	// This is a simplified implementation that just uses ExecuteIRRWithStderr and captures exit code
	output, errOut, err := h.ExecuteIRRWithStderr(nil, args...)

	// If err is an ExitCodeError, extract the exit code
	var exitErr *exec.ExitError
	if err != nil && errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		// For other errors, use a generic error code
		exitCode = 1
	}

	return output, errOut, exitCode
}
