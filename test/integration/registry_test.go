// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestRegistryMappingFileFormats tests different registry mapping file formats
func TestRegistryMappingFileFormats(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	setupMinimalTestChart(t, h)
	targetReg := "test.registry.io"
	sourceRegs := []string{"docker.io"}

	testCases := []struct {
		name           string
		mappingContent string
		shouldSucceed  bool
		expectedText   string
		errorSubstring string
	}{
		{
			name: "structured format",
			mappingContent: `version: "1.0"
registries:
  mappings:
  - source: docker.io
    target: registry.example.com/dockerio
    enabled: true
    description: "Docker Hub mapping"
  defaultTarget: registry.example.com/default
  strictMode: false
compatibility:
  ignoreEmptyFields: true
`,
			shouldSucceed: true,
			expectedText:  "dockerio",
		},
		{
			name: "legacy key-value format",
			mappingContent: `docker.io: registry.example.com/docker
quay.io: registry.example.com/quay
`,
			shouldSucceed: true,     // Changed from false to true as we now support legacy format
			expectedText:  "docker", // The expected text should contain "docker" for the repository
		},
		{
			name: "malformed YAML format",
			mappingContent: `version: "1.0"
registries:
  mappings:
  - source: docker.io
    target: registry.example.com/dockerio
    enabled: true
    description: Docker Hub mapping"  # Missing opening quote
  defaultTarget: registry.example.com/default
`,
			shouldSucceed: true, // YAML parser still handles this fine
			expectedText:  "dockerio",
		},
		{
			name:           "empty file",
			mappingContent: ``,
			shouldSucceed:  false,
			errorSubstring: "mappings file is empty", // Actual error message
		},
		{
			name: "invalid structured format - missing required fields",
			mappingContent: `version: "1.0"
registries:
  # Missing mappings section
  defaultTarget: registry.example.com/default
  strictMode: false
`,
			shouldSucceed:  false,
			errorSubstring: "failed to parse config file", // Updated to match actual error message
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create registry mapping file with the test content
			mappingFile := h.CreateRegistryMappingsFile(tc.mappingContent)
			require.FileExists(t, mappingFile, "Mapping file should be created")

			// For each test case, use a unique output file to avoid conflicts
			outputFile := filepath.Join(h.tempDir, fmt.Sprintf("output-%s.yaml", strings.ReplaceAll(tc.name, " ", "-")))

			// Run the override command with the mappings file
			args := []string{
				"override",
				"--chart-path", h.chartPath,
				"--target-registry", targetReg,
				"--source-registries", strings.Join(sourceRegs, ","),
				"--registry-file", mappingFile,
				"--output-file", outputFile,
			}

			output, stderr, err := h.ExecuteIRRWithStderr(args...)

			// Check if the command succeeded or failed as expected
			if tc.shouldSucceed {
				if !assert.NoError(t, err, "Command should succeed for valid format: %s", stderr) {
					t.Logf("Command failed unexpectedly. Output: %s", output)
					t.Logf("Stderr: %s", stderr)
					return
				}

				// Verify that the override file was created
				require.FileExists(t, outputFile, "Override file should be created")

				// Read the override file content directly
				fileBytes, readErr := os.ReadFile(outputFile) // #nosec G304 - test file created by this test
				if !assert.NoError(t, readErr, "Should be able to read override file") {
					return
				}

				content := string(fileBytes)
				t.Logf("Override content: %s", content)

				// Verify expected text is in the content
				assert.Contains(t, content, tc.expectedText,
					"Override should contain expected transformed text")
			} else {
				if !assert.Error(t, err, "Command should fail for invalid format") {
					t.Logf("Command succeeded unexpectedly. Output: %s", output)
					return
				}

				// Check for expected error message
				assert.Contains(t, stderr, tc.errorSubstring,
					"Error message should contain expected text: %s", tc.errorSubstring)
				t.Logf("Command failed as expected with error: %v", err)
				t.Logf("Stderr contained expected text: %s", stderr)
			}
		})
	}
}

// TestCreateRegistryMappingsFile tests the CreateRegistryMappingsFile method with different inputs
func TestCreateRegistryMappingsFile(t *testing.T) {
	testCases := []struct {
		name              string
		mappingContent    string
		shouldContain     []string
		expectedToBeEmpty bool
	}{
		{
			name: "structured format",
			mappingContent: `version: "1.0"
registries:
  mappings:
  - source: docker.io
    target: registry.example.com/dockerio
    enabled: true
    description: "Docker Hub mapping"
  defaultTarget: registry.example.com/default
  strictMode: false
compatibility:
  ignoreEmptyFields: true
`,
			shouldContain: []string{
				"version: \"1.0\"",
				"mappings:",
				"source: docker.io",
				"target: registry.example.com/dockerio",
				"enabled: true",
			},
			expectedToBeEmpty: false,
		},
		{
			name: "legacy key-value format",
			mappingContent: `docker.io: registry.example.com/docker
quay.io: registry.example.com/quay
`,
			shouldContain: []string{
				"docker.io: registry.example.com/docker",
				"quay.io: registry.example.com/quay",
			},
			expectedToBeEmpty: false,
		},
		{
			name:              "empty content",
			mappingContent:    ``,
			shouldContain:     []string{},
			expectedToBeEmpty: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewTestHarness(t)
			defer h.Cleanup()

			mappingFile := h.CreateRegistryMappingsFile(tc.mappingContent)
			require.NotEmpty(t, mappingFile, "Mapping file path should not be empty")

			// Verify file exists
			require.FileExists(t, mappingFile, "Registry mapping file should exist")

			// Read the file content
			data, err := os.ReadFile(mappingFile) // #nosec G304 - test file created by this test
			require.NoError(t, err, "Failed to read mapping file")
			content := string(data)

			if tc.expectedToBeEmpty {
				assert.Empty(t, strings.TrimSpace(content), "File content should be empty")
			} else {
				assert.NotEmpty(t, strings.TrimSpace(content), "File content should not be empty")

				// Check content has expected strings
				for _, expectedStr := range tc.shouldContain {
					assert.Contains(t, content, expectedStr, "File should contain expected string: %s", expectedStr)
				}
			}

			// If it's a structured format and not empty, try parsing it
			if strings.Contains(tc.mappingContent, "version:") && !tc.expectedToBeEmpty {
				// Set the mappings path for loadMappings to use
				h.mappingsPath = mappingFile

				mappings, err := h.loadMappings()
				if !assert.NoError(t, err, "Should be able to load mappings") {
					return
				}
				assert.NotNil(t, mappings, "Mappings should not be nil")
			}
		})
	}
}

// Helper function to create a test chart with a specific image
func createTestChartWithImage(chartDir, registry, repository string) error {
	if err := os.MkdirAll(chartDir, 0o750); err != nil {
		return fmt.Errorf("failed to create chart directory: %w", err)
	}

	chartYaml := `apiVersion: v2
name: prefix-test-chart
version: 0.1.0`
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0o600); err != nil {
		return fmt.Errorf("failed to write Chart.yaml: %w", err)
	}

	// Format image properly with separate registry and repository fields
	valuesYaml := `image:
  registry: ` + registry + `
  repository: ` + repository + `
  tag: "latest"`
	if err := os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), 0o600); err != nil {
		return fmt.Errorf("failed to write values.yaml: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(chartDir, "templates"), 0o750); err != nil {
		return fmt.Errorf("failed to create templates directory: %w", err)
	}

	deploymentYaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  template:
    spec:
      containers:
      - name: test-container
        image: {{ .Values.image.registry }}/{{ .Values.image.repository }}:{{ .Values.image.tag }}`
	if err := os.WriteFile(filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(deploymentYaml), 0o600); err != nil {
		return fmt.Errorf("failed to write deployment.yaml: %w", err)
	}

	return nil
}

// TestRegistryPrefixTransformation tests registry prefix transformation with different inputs
func TestRegistryPrefixTransformation(t *testing.T) {
	testCases := []struct {
		name           string
		sourceRegistry string
		targetRegistry string
		repository     string
		expectedRepo   string
	}{
		{
			name:           "quay.io to custom registry",
			sourceRegistry: "quay.io",
			targetRegistry: "harbor.example.com",
			repository:     "prometheus/prometheus",
			expectedRepo:   "quayio/prometheus/prometheus",
		},
		{
			name:           "registry.k8s.io to custom registry",
			sourceRegistry: "registry.k8s.io",
			targetRegistry: "harbor.example.com",
			repository:     "ingress-nginx/controller",
			expectedRepo:   "registryk8sio/ingress-nginx/controller",
		},
		{
			name:           "gcr.io to custom registry",
			sourceRegistry: "gcr.io",
			targetRegistry: "harbor.example.com",
			repository:     "google-samples/gb-frontend",
			expectedRepo:   "gcrio/google-samples/gb-frontend",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewTestHarness(t)
			defer h.Cleanup()

			// Create a minimal chart with the specified image
			chartDir := filepath.Join(h.tempDir, "prefix-test-chart")
			err := createTestChartWithImage(chartDir, tc.sourceRegistry, tc.repository)
			require.NoError(t, err, "Failed to create test chart")

			h.SetupChart(chartDir)
			h.SetRegistries(tc.targetRegistry, []string{tc.sourceRegistry})

			// For each test case, use a unique output file to avoid conflicts
			outputFile := filepath.Join(h.tempDir, fmt.Sprintf("output-%s.yaml", strings.ReplaceAll(tc.name, " ", "-")))

			// Run the override command
			args := []string{
				"override",
				"--chart-path", h.chartPath,
				"--target-registry", h.targetReg,
				"--source-registries", strings.Join(h.sourceRegs, ","),
				"--output-file", outputFile,
			}

			output, stderr, err := h.ExecuteIRRWithStderr(args...)
			require.NoError(t, err, "override command should succeed: %s", stderr)
			t.Logf("Override output: %s", output)
			t.Logf("Stderr: %s", stderr)

			// Read the raw override file to verify content
			fileBytes, err := os.ReadFile(outputFile) // #nosec G304 - test file created by this test
			require.NoError(t, err, "Should be able to read override file")

			fileContent := string(fileBytes)
			t.Logf("Override file content: %s", fileContent)

			// Check if override file contains the expected transformation
			// First check for the specific repository format
			assert.Contains(t, fileContent, tc.expectedRepo,
				"Override should contain transformed repository %s", tc.expectedRepo)

			// Try to parse the YAML for structured validation
			var overrides map[string]interface{}
			err = yaml.Unmarshal(fileBytes, &overrides)
			if err != nil {
				t.Logf("Warning: Could not parse override file as YAML: %v", err)
				return
			}

			// Use the Values key to check the image structure
			if values, ok := overrides["Values"].(map[string]interface{}); ok {
				if image, ok := values["image"].(map[string]interface{}); ok {
					if repo, ok := image["repository"].(string); ok {
						assert.Contains(t, repo, tc.expectedRepo,
							"Image repository should contain %s", tc.expectedRepo)
						t.Logf("Found repository in Values.image.repository: %s", repo)
					} else {
						t.Logf("No repository key found in image map")
					}
				} else {
					t.Logf("No image key found in Values map")
				}
			} else {
				t.Logf("No Values key found in overrides")
			}
		})
	}
}
