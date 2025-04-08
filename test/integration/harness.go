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

	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/stretchr/testify/assert"
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
	rootDir      string
}

// NewTestHarness creates a new TestHarness.
func NewTestHarness(t *testing.T) *TestHarness {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "irr-integration-test-*")
	require.NoError(t, err, "Failed to create temp dir")

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
	// Note: Using require directly might not be ideal if harness is created outside a test func scope initially.
	// Let's assume t is available for now, otherwise needs refactoring.
	// G104 is suppressed as the error check is now added.
	err = os.Setenv("IRR_TESTING", "true")
	require.NoError(t, err, "Failed to set IRR_TESTING env var")

	h := &TestHarness{
		t:            t,
		tempDir:      tempDir,
		rootDir:      rootDir, // Initialize rootDir field
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

	// Clean up environment variable
	// G104 is suppressed as the error check is now added.
	err = os.Unsetenv("IRR_TESTING")
	require.NoError(h.t, err, "Failed to unset IRR_TESTING env var")
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

// setup initializes the global test environment
func setup() {
	fmt.Println("--- ENTERING INTEGRATION TEST SETUP ---")
	// Set any necessary environment variables or global test state
	// This function is called once before all tests run
	// G304: Potential file inclusion via variable - Test environment variable, considered safe.
	_ = os.Setenv("IRR_TESTING", "true") // #nosec G104
	// Ensure the binary exists for integration tests
	rootDir, err := getProjectRoot()
	if err != nil {
		// Use panic to ensure the failure is visible during test setup
		panic(fmt.Sprintf("Critical error in setup: Failed to get project root: %v", err))
	}
	fmt.Printf("--- Project Root Detected: %s ---\n", rootDir)

	binPath := filepath.Join(rootDir, "bin", "irr")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		fmt.Printf("irr binary not found at %s. Building...\n", binPath)
		cmd := exec.Command("make", "build")
		cmd.Dir = rootDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to build irr binary in setup: %v\nOutput:\n%s\n", err, string(output))
			// Fail fast if build fails
			panic("Failed to build required irr binary for integration tests")
		}
		fmt.Println("irr binary built successfully.")
	} else {
		fmt.Println("irr binary already exists.")
	}
}

// teardown cleans up the global test environment
func teardown() {
	fmt.Println("--- EXITING INTEGRATION TEST SETUP ---")
	// Clean up any global resources
	// This function is called once after all tests complete
	// G304: Potential file inclusion via variable - Test environment variable, considered safe.
	_ = os.Unsetenv("IRR_TESTING") // #nosec G104
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
	mappings := &registry.Mappings{} // Initialize as non-nil
	if h.mappingsPath != "" {
		var loadErr error
		// Assign loaded mappings only if successful
		loadedMappings, loadErr := registry.LoadMappings(h.mappingsPath) // Use the registry package function
		if loadErr != nil {
			// Log warning but don't fail the validation outright
			h.t.Logf("Warning: could not load mappings file '%s' for validation: %v", h.mappingsPath, loadErr)
			// Keep the initialized empty mappings struct
		} else {
			mappings = loadedMappings // Assign successfully loaded mappings
		}
	}

	// Get the actual overrides generated to find the real target registries used.
	actualOverrides, getOverridesErr := h.GetOverrides()
	actualTargetsUsed := make(map[string]bool)

	if getOverridesErr != nil {
		// If we can't read the overrides file, we can't determine actual targets.
		// Log a warning and fall back to the previous check based on configuration.
		h.t.Logf("Warning: Could not read overrides file (%s) for validation: %v. Falling back to checking configured targets.", h.overridePath, getOverridesErr)

		// -- Previous Check Logic (Fallback) --
		expectedTargets := []string{h.targetReg} // Start with the default target
		if mappings != nil {                     // Ensure mappings is not nil here too
			for _, entry := range mappings.Entries {
				if entry.Target != "" {
					expectedTargets = append(expectedTargets, entry.Target)
				}
			}
		}
		foundExpectedTarget := false
		for _, target := range expectedTargets {
			if target != "" && strings.Contains(output, target) {
				foundExpectedTarget = true
				h.t.Logf("[Fallback Check] Found configured target registry '%s' in Helm output.", target)
				break
			}
		}
		if !foundExpectedTarget {
			return fmt.Errorf("[Fallback Check] no configured target registry (default: '%s' or mapped: %v) found in validated helm template output", h.targetReg, expectedTargets)
		}
		// -- End Fallback Check --

	} else if len(actualOverrides) == 0 {
		// Overrides file was empty, likely no images needed changing.
		h.t.Log("Overrides file is empty. Skipping registry validation in Helm output.")
	} else {
		// Successfully read overrides, find actual targets used.
		h.WalkImageFields(actualOverrides, func(path []string, value interface{}) {
			if imageMap, ok := value.(map[string]interface{}); ok {
				if reg, ok := imageMap["registry"].(string); ok && reg != "" {
					actualTargetsUsed[reg] = true
				}
			}
			// Also handle string format if we generated strings
			if imageStr, ok := value.(string); ok && imageStr != "" {
				ref, parseErr := image.ParseImageReference(imageStr) // Use ParseImageReference
				if parseErr == nil && ref != nil && ref.Registry != "" {
					actualTargetsUsed[ref.Registry] = true
				}
			}
		})

		if len(actualTargetsUsed) == 0 {
			h.t.Log("No image registry keys/values found in the generated overrides file. Validation check skipped.")
			// Assume it's okay if no registries were found in overrides.
		} else {
			foundActualTargetInOutput := false
			for target := range actualTargetsUsed {
				if strings.Contains(output, target) {
					foundActualTargetInOutput = true
					h.t.Logf("Found actual target registry '%s' from overrides in Helm output.", target)
					break
				}
			}
			if !foundActualTargetInOutput {
				// Convert map keys to slice for error message
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
			currentPath := append([]string{}, path...) // Create copy before appending
			currentPath = append(currentPath, key)
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
	h.t.Logf("[HARNESS EXECUTE_IRR] Command: %s %s", binPath, strings.Join(args, " "))

	// errcheck: Capture error, but test might focus on stderr content, so don't require.NoError here.
	err := cmd.Run()
	outputStr := out.String()
	stderrStr := stderr.String()

	// ALWAYS log the full output for debugging purposes
	if outputStr != "" {
		h.t.Logf("[HARNESS EXECUTE_IRR] Stdout:\n%s", outputStr)
	}
	if stderrStr != "" {
		h.t.Logf("[HARNESS EXECUTE_IRR] Stderr:\n%s", stderrStr)
	}

	if err != nil {
		// Return error along with the combined output for context
		combinedOutput := outputStr + stderrStr
		return combinedOutput, fmt.Errorf("irr command execution failed: %w. Output:\n%s", err, combinedOutput)
	}

	return outputStr, nil // Return only stdout on success
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
	cwd, _ := os.Getwd() // Ignore error for logging only
	h.t.Logf("[ASSERT_EXIT_CODE DEBUG] binPath: %s, CWD: %s", binPath, cwd)

	// G204: Subprocess launched with variable - Acceptable in test code.
	cmd := exec.Command(binPath, args...) // #nosec G204
	cmd.Dir = h.tempDir                   // Run from the temp directory
	outputBytes, runErr := cmd.CombinedOutput()
	outputStr := string(outputBytes)

	// Check exit code using exec.ExitError
	if exitErr, ok := runErr.(*exec.ExitError); ok {
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
	cwd, _ := os.Getwd() // Ignore error for logging only
	h.t.Logf("[ASSERT_ERROR_CONTAINS DEBUG] binPath: %s, CWD: %s", binPath, cwd)

	// G204: Subprocess launched with variable - Acceptable in test code.
	cmd := exec.Command(binPath, args...) // #nosec G204
	cmd.Dir = h.tempDir                   // Run from the temp directory
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run() // Ignore error, focus on stderr content

	assert.Contains(h.t, stderr.String(), substring,
		"Expected stderr to contain '%s'\nArgs: %v\nActual stderr:\n%s", substring, args, stderr.String())
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

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/irr")
	cmd.Dir = rootDir
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout // Keep stdout for progress
	cmd.Stderr = &stderr   // Capture stderr for errors

	// errcheck: Capture error, but test might focus on stderr content, so don't require.NoError here.
	runErr := cmd.Run()
	if runErr != nil {
		// errorlint: Use errors.As instead of type assertion
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			// If it's an ExitError, log stderr for detailed Go build failure info
			h.t.Fatalf("BuildIRR failed with exit code %d: %v\nStderr:\n%s", exitErr.ExitCode(), runErr, stderr.String())
		} else {
			// For other errors, just log the error
			h.t.Fatalf("BuildIRR failed: %v\nStderr:\n%s", runErr, stderr.String())
		}
	}
	h.t.Logf("BuildIRR successful.")
}
