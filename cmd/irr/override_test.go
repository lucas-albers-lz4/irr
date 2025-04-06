package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	// Use testify for assertions
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Need analysis types for mocking generator return value
	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	// Import necessary packages (bytes, os, strings might be needed later)
	// "bytes"
	// "os"
	// "strings"
	// Need cobra for command execution simulation
)

// executeCommand is defined in analyze_test.go - we might move it to a shared test utility later

// Mock Generator (implements GeneratorInterface from root.go)
type mockGenerator struct {
	GenerateFunc func() (*override.OverrideFile, error)
}

func (m *mockGenerator) Generate() (*override.OverrideFile, error) {
	if m.GenerateFunc != nil {
		return m.GenerateFunc()
	}
	// Default mock behavior
	return &override.OverrideFile{Overrides: make(map[string]interface{})}, nil
}

// REMOVE Mock Strategy definition - we won't mock GetStrategy directly here
// type mockStrategy struct { ... }
// func (m *mockStrategy) GeneratePath(...) { ... }

// REMOVE Backup original functions - not needed for this approach
// var originalLoadMappings = registry.LoadMappings
// var originalGetStrategy = strategy.GetStrategy

func TestOverrideCmdArgs(t *testing.T) {
	// Test cases focusing only on argument validation and required flags
	tests := []struct {
		name           string
		args           []string
		expectErr      bool
		stdErrContains string
	}{
		// --- Missing Required Flags ---
		{
			name:           "missing chart-path",
			args:           []string{"override", "--target-registry", "tr", "--source-registries", "sr"},
			expectErr:      true,
			stdErrContains: "required flag(s) \"chart-path\" not set",
		},
		{
			name:           "missing target-registry",
			args:           []string{"override", "--chart-path", "cp", "--source-registries", "sr"},
			expectErr:      true,
			stdErrContains: "required flag(s) \"target-registry\" not set",
		},
		{
			name:           "missing source-registries",
			args:           []string{"override", "--chart-path", "cp", "--target-registry", "tr"},
			expectErr:      true,
			stdErrContains: "required flag(s) \"source-registries\" not set",
		},
		{
			name:           "all required flags present (execution error expected)",
			args:           []string{"override", "--chart-path", "cp", "--target-registry", "tr", "--source-registries", "sr"},
			expectErr:      true,
			stdErrContains: "chart parsing failed for cp",
		},
		// --- Invalid Flag Values ---
		{
			name:           "invalid path strategy",
			args:           []string{"override", "--chart-path", "cp", "--target-registry", "tr", "--source-registries", "sr", "--path-strategy", "invalid-strat"},
			expectErr:      true,
			stdErrContains: "unsupported path strategy: invalid-strat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := newRootCmd()
			currentGeneratorFactory = defaultGeneratorFactory
			_, _, err := executeCommand(rootCmd, tt.args...)

			if tt.expectErr {
				assert.Error(t, err, "Expected an error")
				if tt.stdErrContains != "" {
					assert.Contains(t, err.Error(), tt.stdErrContains, "error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Did not expect an error")
			}
		})
	}
}

func TestOverrideCmdExecution(t *testing.T) {
	originalGeneratorFactory := currentGeneratorFactory
	defer func() {
		currentGeneratorFactory = originalGeneratorFactory
	}()

	defaultArgs := []string{
		"override",
		"--chart-path", "./fake/chart",
		"--target-registry", "mock-target.com",
		"--source-registries", "docker.io",
	}

	tests := []struct {
		name              string
		args              []string
		mockGeneratorFunc func() (*override.OverrideFile, error)
		expectErr         bool
		stdOutContains    string
		stdErrContains    string
		setupEnv          map[string]string
		postCheck         func(t *testing.T, testDir string)
	}{
		{
			name: "success execution to stdout",
			args: defaultArgs,
			mockGeneratorFunc: func() (*override.OverrideFile, error) {
				return &override.OverrideFile{
					Overrides: map[string]interface{}{"image": map[string]interface{}{"repository": "mock-target.com/dockerio/nginx"}},
				}, nil
			},
			expectErr:      false,
			stdOutContains: "repository: mock-target.com/dockerio/nginx",
			stdErrContains: "",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck:      nil,
		},
		{
			name: "success with dry run",
			args: append(defaultArgs, "--dry-run"),
			mockGeneratorFunc: func() (*override.OverrideFile, error) {
				return &override.OverrideFile{
					Overrides: map[string]interface{}{"image": "dry-run-image"},
				}, nil
			},
			expectErr:      false,
			stdOutContains: "image: dry-run-image",
			stdErrContains: "",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck:      nil,
		},
		{
			name: "generator returns error",
			args: defaultArgs,
			mockGeneratorFunc: func() (*override.OverrideFile, error) {
				return nil, fmt.Errorf("mock generator error")
			},
			expectErr:      true,
			stdOutContains: "",
			stdErrContains: "error generating overrides: mock generator error",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck:      nil,
		},
		{
			name: "success with output file (flow check)",
			args: defaultArgs, // Will append output file path in the test
			mockGeneratorFunc: func() (*override.OverrideFile, error) {
				return &override.OverrideFile{
					Overrides: map[string]interface{}{"image": map[string]interface{}{"repository": "mock-target.com/dockerio/nginx", "tag": "latest"}},
				}, nil
			},
			expectErr:      false,
			stdOutContains: "Overrides written to:",
			stdErrContains: "",
			setupEnv:       map[string]string{"IRR_SKIP_HELM_VALIDATION": "true"},
			postCheck: func(t *testing.T, testDir string) {
				outputPath := filepath.Join(testDir, "output.yaml")
				content, err := os.ReadFile(outputPath)
				require.NoError(t, err, "Should be able to read output file")
				assert.Contains(t, string(content), "repository: mock-target.com/dockerio/nginx")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()

			// Setup Generator Mock
			if tt.mockGeneratorFunc != nil {
				currentGeneratorFactory = func(chartPath, targetRegistry string, sourceRegistries, excludeRegistries []string, pathStrategy strategy.PathStrategy, mappings *registry.RegistryMappings, strict bool, threshold int, loader chart.Loader) GeneratorInterface {
					return &mockGenerator{GenerateFunc: tt.mockGeneratorFunc}
				}
			} else {
				currentGeneratorFactory = defaultGeneratorFactory
			}

			// Set up environment variables if specified
			if tt.setupEnv != nil {
				for k, v := range tt.setupEnv {
					os.Setenv(k, v)
				}
				defer func() {
					for k := range tt.setupEnv {
						os.Unsetenv(k)
					}
				}()
			}

			// Prepare args with output file if needed
			args := tt.args
			if tt.name == "success with output file (flow check)" {
				outputPath := filepath.Join(testDir, "output.yaml")
				args = append(args, "--output-file", outputPath, "--verbose")
			}

			// Get a fresh command instance
			rootCmd := newRootCmd()

			// Execute command
			stdout, _, err := executeCommand(rootCmd, args...)

			// Assertions
			if tt.expectErr {
				assert.Error(t, err, "Expected an error")
				if tt.stdErrContains != "" {
					assert.Contains(t, err.Error(), tt.stdErrContains, "error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Did not expect an error")
				if tt.stdOutContains != "" {
					assert.Contains(t, stdout, tt.stdOutContains)
				}
			}

			// Run post-check if specified
			if tt.postCheck != nil {
				tt.postCheck(t, testDir)
			}
		})
	}
}
