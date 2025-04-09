// Package integration provides test harnesses and utilities for running irr CLI integration tests.
package integration

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	// defaultDirPerm defines the default directory permissions (rwxr-x---)
	defaultDirPerm = 0o750
	// defaultFilePerm defines the default file permissions (rw-------)
	defaultFilePerm = 0o600
	// DefaultTargetRegistry is the registry used in tests when not specified.
	DefaultTargetRegistry = "test-target.local"
)

// Global variable to hold the path to the compiled binary
var buildOnce sync.Once
var buildErr error

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
	rootDir      string
	outputPath   string
	logger       *log.Logger
	cleanupFuncs []func()
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

	// Ensure overrides directory exists with correct permissions (using a fixed relative path)
	// G301 fix
	// REMOVED: Creation of test-data/overrides is handled elsewhere if needed.
	// if err := os.MkdirAll("../test-data/overrides", defaultDirPerm); err != nil {
	// 	require.NoError(t, err, "Failed to create test overrides directory: %v", err)
	// }

	// Set an environment variable to indicate testing mode if needed by the core logic
	if err := os.Setenv("IRR_TESTING", "true"); err != nil {
		t.Errorf("Failed to set IRR_TESTING env var: %v", err)
	}

	h := &TestHarness{
		t:            t,
		tempDir:      tempDir,
		rootDir:      rootDir, // Initialize rootDir field
		overridePath: filepath.Join(tempDir, "generated-overrides.yaml"),
		mappingsPath: "", // No mappings by default
		outputPath:   "", // No explicit output by default
		logger:       log.New(os.Stdout, fmt.Sprintf("[HARNESS %s] ", t.Name()), log.LstdFlags),
	}

	// Setup testing environment variable
	cleanupEnv := h.setTestingEnv()
	h.cleanupFuncs = append(h.cleanupFuncs, cleanupEnv)

	// Create a default registry mapping file during setup
	mappingsPath, err := h.createDefaultRegistryMappingFile() // Use internal helper
	require.NoError(t, err, "Failed to create default registry mapping file")
	h.mappingsPath = mappingsPath

	h.logger.Printf("Initialized harness in temp dir: %s", tempDir)
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
	// Explicitly unset IRR_DEBUG as well to avoid interference
	if err := os.Unsetenv("IRR_DEBUG"); err != nil {
		// Log this error but don't fail the test for it, as it might not have been set.
		h.t.Logf("Warning: Failed to unset IRR_DEBUG env var: %v (might not have been set)", err)
	}

	// Run registered cleanup functions
	for _, cleanup := range h.cleanupFuncs {
		cleanup()
	}
	h.cleanupFuncs = nil // Clear cleanup functions
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

// getProjectRoot finds the project root directory by searching upwards for go.mod
func getProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	dir := wd
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Found go.mod, this is the root
			return dir, nil
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root without finding go.mod
			return "", fmt.Errorf("failed to find project root (go.mod) starting from %s", wd)
		}
		dir = parent
	}
}

// setTestingEnv sets an environment variable to indicate testing is active
// and returns a cleanup function to unset it.
func (h *TestHarness) setTestingEnv() func() {
	h.logger.Printf("Setting IRR_TESTING=true")
	if err := os.Setenv("IRR_TESTING", "true"); err != nil {
		h.t.Errorf("Failed to set IRR_TESTING env var: %v", err)
	}
	return func() {
		h.logger.Printf("Unsetting IRR_TESTING")
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
	h.overridePath = filepath.Join(h.tempDir, "generated-overrides.yaml")
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
	h.logger.Printf("Validating overrides for chart: %s", h.chartPath)

	// Determine expected target registries based on mapping file
	expectedTargets := []string{image.NormalizeRegistry(h.targetReg)}
	if h.mappingsPath != "" {
		// #nosec G304 -- Reading a test-generated file from the test's temp directory is safe.
		mappingBytes, err := os.ReadFile(h.mappingsPath)
		if err != nil {
			return fmt.Errorf("failed to read mappings file %s: %w", h.mappingsPath, err)
		}
		var mappings map[string]string
		if err := yaml.Unmarshal(mappingBytes, &mappings); err != nil {
			return fmt.Errorf("failed to unmarshal mappings from %s: %w", h.mappingsPath, err)
		}
		// Clear default if mappings are used
		expectedTargets = []string{}
		for _, target := range mappings {
			normTarget := image.NormalizeRegistry(target)
			found := false
			for _, existing := range expectedTargets {
				if existing == normTarget {
					found = true
					break
				}
			}
			if !found {
				expectedTargets = append(expectedTargets, normTarget)
			}
		}
	}
	h.logger.Printf("Expecting images to use target registries: %v", expectedTargets)

	// Read the generated overrides file content
	currentOverrides, err := os.ReadFile(h.overridePath)
	if err != nil {
		h.t.Logf("Warning: failed to read overrides file %s locally for modification: %v", h.overridePath, err)
		currentOverrides = []byte{}
	} else {
		h.t.Logf("Read %d bytes from overrides file: %s", len(currentOverrides), h.overridePath)
	}

	// Write the potentially modified overrides to a *temporary* file for helm template validation
	tempValidationOverridesPath := filepath.Join(h.tempDir, "validation-overrides.yaml")
	if err := os.WriteFile(tempValidationOverridesPath, currentOverrides, defaultFilePerm); err != nil {
		return fmt.Errorf("failed to write temporary validation overrides file %s: %w", tempValidationOverridesPath, err)
	}
	h.t.Logf("Wrote %d bytes to temporary validation file: %s", len(currentOverrides), tempValidationOverridesPath)

	// Helm template command for validation (using tempValidationOverridesPath)
	args := []string{"template", "test-release", h.chartPath, "-f", tempValidationOverridesPath}
	// ... (Add bitnami flags etc.)

	output, err := h.ExecuteHelm(args...)
	if err != nil {
		return fmt.Errorf("helm template validation failed: %w\nOutput:\n%s", err, output)
	}

	// Load mappings to check against mapped targets as well
	mappings := &registry.Mappings{} // Initialize as non-nil
	if h.mappingsPath != "" {
		loadedMappings, loadErr := registry.LoadMappings(afero.NewOsFs(), h.mappingsPath)
		if loadErr != nil {
			h.t.Logf("Warning: could not load mappings file %s for validation: %v", h.mappingsPath, loadErr)
		} else {
			mappings = loadedMappings
		}
	}

	// Get the actual overrides generated to find the real target registries used.
	actualOverrides, getOverridesErr := h.getOverrides()
	actualTargetsUsed := make(map[string]bool)

	if getOverridesErr != nil {
		h.t.Logf("Warning: Could not read overrides file (%s) for validation: %v. Falling back to checking configured targets.", h.overridePath, getOverridesErr)

		// -- Fallback Check Logic --
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
			// Use image.NormalizeRegistry (assuming import added)
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
		// -- End Fallback Check --

	} else if len(actualOverrides) == 0 {
		h.t.Log("Overrides file is empty. Skipping registry validation in Helm output.")
	} else {
		// Successfully read overrides, find actual targets used.
		h.WalkImageFields(actualOverrides, func(_ []string, value interface{}) { // Fix: Mark path as unused with _
			if imageMap, ok := value.(map[string]interface{}); ok {
				if reg, ok := imageMap["registry"].(string); ok && reg != "" {
					actualTargetsUsed[image.NormalizeRegistry(reg)] = true // Normalize here too
				}
			}
			if imageStr, ok := value.(string); ok && imageStr != "" {
				ref, parseErr := image.ParseImageReference(imageStr, true)
				if parseErr == nil && ref != nil && ref.Registry != "" {
					actualTargetsUsed[image.NormalizeRegistry(ref.Registry)] = true // Normalize here too
				}
			}
		})

		if len(actualTargetsUsed) == 0 {
			h.t.Log("No image registry keys/values found in the generated overrides file. Validation check skipped.")
		} else {
			foundActualTargetInOutput := false
			for target := range actualTargetsUsed {
				if strings.Contains(output, target) { // Check against normalized target
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
		}
	}

	h.t.Log("Helm template validation successful.")
	return nil
}

// getOverrides reads and unmarshals the generated overrides file.
func (h *TestHarness) getOverrides() (map[string]interface{}, error) {
	// #nosec G304 -- Reading a test-generated file from the test's temp directory is safe.
	overridesBytes, err := os.ReadFile(h.overridePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read overrides file %s: %w", h.overridePath, err)
	}

	overrides := make(map[string]interface{})
	if err := yaml.Unmarshal(overridesBytes, &overrides); err != nil {
		return nil, fmt.Errorf("failed to unmarshal overrides YAML from %s: %w", h.overridePath, err)
	}
	return overrides, nil
}

// WalkImageFields recursively walks through a map/slice structure and calls the visitor function
// for any field that represents an image (either a string or a map with expected keys).
// This is a simplified walker, assuming overrides structure.
func (h *TestHarness) WalkImageFields(data interface{}, visitor func(path []string, value interface{})) {
	h.walkImageFieldsRecursive(data, []string{}, visitor)
}

func (h *TestHarness) walkImageFieldsRecursive(data interface{}, currentPath []string, visitor func(path []string, value interface{})) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this map potentially represents an image override.
		// Presence of "repository" is a strong indicator.
		if _, repoOk := v["repository"]; repoOk {
			// Consider it an image map if repository is present.
			// We might also check for tag/digest, but repository is key.
			visitor(currentPath, v)
			return // Don't recurse further into this identified image map structure.
		}
		// Not identified as an image map structure based on keys, recurse into its values.
		for key, value := range v {
			newPath := append(append([]string{}, currentPath...), key)
			h.walkImageFieldsRecursive(value, newPath, visitor)
		}
	case []interface{}:
		for i, item := range v {
			newPath := append(append([]string{}, currentPath...), fmt.Sprintf("[%d]", i))
			h.walkImageFieldsRecursive(item, newPath, visitor)
		}
	case string:
		// Check if the path itself suggests this string is an image.
		// This is more reliable than guessing based on string content.
		if len(currentPath) > 0 {
			lastKey := currentPath[len(currentPath)-1]
			// Simple check: if the key is "image" or ends with "Image", treat the string as an image override.
			// This aligns better with how overrides might be structured.
			if lastKey == "image" || strings.HasSuffix(lastKey, "Image") {
				visitor(currentPath, v)
			}
		}
		// Optional: Could add back the content heuristic as a fallback if needed,
		// but path-based checking is generally preferred for overrides.
		/*
			hasSeparator := strings.Contains(v, ":") || strings.Contains(v, "@")
			isNotPathOrURL := !strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "./") && !strings.HasPrefix(v, "../") && !strings.HasPrefix(v, "http")
			if hasSeparator && isNotPathOrURL {
				visitor(currentPath, v)
			}
		*/
	}
}

// ExecuteIRR runs the irr binary with the given arguments and returns the combined output.
func (h *TestHarness) ExecuteIRR(args ...string) (string, error) {
	binPath := h.getBinaryPath()
	// G204: Subprocess launched with variable - This is acceptable in test code
	// where args are controlled by the test setup.
	cmd := exec.Command(binPath, args...) // #nosec G204
	cmd.Dir = h.tempDir                   // Run from the temp directory
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	// ALWAYS log the full command for debugging purposes
	h.logger.Printf("[HARNESS EXECUTE_IRR] Command: %s %s", binPath, strings.Join(args, " "))

	// errcheck: Capture error, but test might focus on stderr content, so don't require.NoError here.
	err := cmd.Run()
	outputStr := out.String()
	stderrStr := stderr.String()

	// ALWAYS log the full output for debugging purposes
	if outputStr != "" {
		h.logger.Printf("[HARNESS EXECUTE_IRR] Stdout:\n%s", outputStr)
	}
	if stderrStr != "" {
		h.logger.Printf("[HARNESS EXECUTE_IRR] Stderr:\n%s", stderrStr)
	}

	if err != nil {
		// Return error along with the combined output for context
		combinedOutput := outputStr + stderrStr
		return combinedOutput, fmt.Errorf("irr command execution failed: %w\nOutput:\n%s", err, combinedOutput)
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

	h.logger.Printf("[HARNESS EXECUTE_HELM] Command: helm %s", strings.Join(args, " "))
	h.logger.Printf("[HARNESS EXECUTE_HELM] Full Output:\n%s", outputStr)

	if err != nil {
		return outputStr, fmt.Errorf("helm command execution failed: %w", err)
	}
	return outputStr, nil
}

// init ensures the binary path is determined early.
func init() {
	// Build is now handled lazily by buildOnce.Do in NewTestHarness
	// to avoid redundant builds and potential issues with init-phase failures.
	/*
		// Ensure the binary path is determined early, potentially building it.
		// We need this available before individual tests run.
		var err error
		irrBinaryPath, err = buildIrrBinary(nil) // Pass nil testing.T for init phase
		if err != nil {
			// Log the error and potentially panic if the build is critical for all tests
			log.Fatalf("FATAL: Failed to build irr binary during init: %v", err)
		}
	*/
}

// SetChartPath sets the chart path for the test harness.
func (h *TestHarness) SetChartPath(path string) {
	h.chartPath = path
}

// GetTestdataPath returns the absolute path to a file or directory within the testdata directory.
func (h *TestHarness) GetTestdataPath(relativePath string) string {
	absPath, err := filepath.Abs(filepath.Join("..", "testdata", relativePath))
	if err != nil {
		h.t.Fatalf("Failed to get absolute path for testdata: %v", err)
	}
	return absPath
}

// AssertExitCode runs the IRR binary with the given arguments and checks the exit code.
func (h *TestHarness) AssertExitCode(expected int, args ...string) {
	h.t.Helper()
	binPath := h.getBinaryPath()

	// Debug logging for path and CWD
	cwd, err := os.Getwd() // Check error now
	if err != nil {
		// Log the error but don't fail the test just for this
		h.logger.Printf("[ASSERT_EXIT_CODE WARNING] Failed to get current working directory: %v", err)
		cwd = "(unknown)" // Use placeholder
	}
	h.logger.Printf("[ASSERT_EXIT_CODE DEBUG] binPath: %s, CWD: %s", binPath, cwd)

	// G204: Subprocess launched with variable - Acceptable in test code.
	cmd := exec.Command(binPath, args...) // #nosec G204
	cmd.Dir = h.tempDir                   // Run from the temp directory
	outputBytes, runErr := cmd.CombinedOutput()
	outputStr := string(outputBytes)

	// Check exit code using exec.ExitError
	// errorlint: Use errors.As for checking the type
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		// It's an ExitError, check the code
		assert.Equal(h.t, expected, exitErr.ExitCode(),
			"Expected exit code %d but got %d\nArgs: %v\nOutput:\n%s", expected, exitErr.ExitCode(), args, outputStr)
	} else if runErr != nil && expected != 0 {
		// Command failed in a way other than ExitError (e.g., couldn't start)
		h.t.Fatalf("Command failed unexpectedly (expected exit code %d): %v\nArgs: %v\nOutput:\n%s", expected, runErr, args, outputStr)
	} else if runErr == nil && expected != 0 {
		// Command succeeded but an error code was expected
		h.t.Fatalf("Expected exit code %d but command succeeded.\nArgs: %v\nOutput:\n%s", expected, args, outputStr)
	} else if runErr != nil && expected == 0 {
		// Command failed but success (0) was expected
		h.t.Fatalf("Expected exit code 0 but command failed: %v\nArgs: %v\nOutput:\n%s", runErr, args, outputStr)
	}
	// If expected is 0 and err is nil, it's a pass, do nothing.
}

// AssertErrorContains checks if the stderr output contains the specified substring.
func (h *TestHarness) AssertErrorContains(substring string, args ...string) {
	h.t.Helper()
	binPath := h.getBinaryPath()

	// Debug logging for path and CWD
	cwd, err := os.Getwd() // Check error now
	if err != nil {
		// Log the error but don't fail the test just for this
		h.logger.Printf("[ASSERT_ERROR_CONTAINS WARNING] Failed to get current working directory: %v", err)
		cwd = "(unknown)" // Use placeholder
	}
	h.logger.Printf("[ASSERT_ERROR_CONTAINS DEBUG] binPath: %s, CWD: %s", binPath, cwd)

	// G204: Subprocess launched with variable - Acceptable in test code.
	cmd := exec.Command(binPath, args...) // #nosec G204
	cmd.Dir = h.tempDir                   // Run from the temp directory
	var stderr bytes.Buffer
	var stdout bytes.Buffer // Keep capturing stdout for context if needed, but don't check it
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	// Explicitly ignore error return value from cmd.Run().
	// This function's purpose is to assert content in stderr, regardless of exit code.
	_ = cmd.Run() //nolint:errcheck

	stderrStr := stderr.String()
	stdoutStr := stdout.String() // Log stdout too for debugging context

	h.logger.Printf("[ASSERT_ERROR_CONTAINS] Stderr:\n%s", stderrStr)
	if stdoutStr != "" {
		h.logger.Printf("[ASSERT_ERROR_CONTAINS] Stdout:\n%s", stdoutStr)
	}

	assert.Contains(h.t, stderrStr, substring,
		"Expected stderr to contain '%s'\nArgs: %v\nActual stderr:\n%s", substring, args, stderrStr)
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
	err := os.MkdirAll(filepath.Join(rootDir, "bin"), 0755)
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
		// errorlint: Use errors.As instead of type assertion
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// If it's an ExitError, log stderr for detailed Go build failure info
			h.t.Fatalf("BuildIRR failed with exit code %d: %v\nStderr:\n%s", exitErr.ExitCode(), err, string(output))
		} else {
			// For other errors, just log the error
			h.t.Fatalf("BuildIRR failed: %v\nStderr:\n%s", err, string(output))
		}
	}
	h.t.Logf("BuildIRR successful.")
}

// buildIrrBinary builds the irr binary for testing and returns its path.
// It ensures the build happens only once per test run.
// Note: This function does not use t.Helper() or t.Logf() as it might be called from TestMain.
func buildIrrBinary() error { // Removed t *testing.T argument
	rootDir, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	binDir := filepath.Join(rootDir, "bin")
	// Use 0755 for bin directory as it needs execute permissions
	err = os.MkdirAll(binDir, 0755) // #nosec G301
	if err != nil {
		return fmt.Errorf("failed to create bin directory %s: %w", binDir, err)
	}

	binPath := filepath.Join(binDir, "irr")
	fmt.Printf("Building irr binary at: %s\n", binPath) // Use fmt.Printf
	// #nosec G204 -- Building the project's own binary is safe.
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/irr")
	cmd.Dir = rootDir // Run build from project root
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's an ExitError using errors.As
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Include exit code in the error message for better context
			return fmt.Errorf("go build failed with exit code %d: %w\nOutput:\n%s", exitErr.ExitCode(), err, string(output))
		} else {
			// Generic build failure
			return fmt.Errorf("go build failed: %w\nOutput:\n%s", err, string(output))
		}
	}
	fmt.Printf("Build successful.\n") // Use fmt.Printf
	return nil
}
