package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	// Use testify for assertions
	"github.com/stretchr/testify/assert"

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
	// "github.com/spf13/cobra"
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
	// Uses the real command structure but doesn't need mocks yet

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
			expectErr:      true,                         // Expect error from Generate() as it's not mocked
			stdErrContains: "error generating overrides", // Check for error from RunE
		},
		// --- Invalid Flag Values (where applicable) ---
		// Example: Invalid path strategy (though currently only one is supported)
		{
			name:           "invalid path strategy",
			args:           []string{"override", "--chart-path", "cp", "--target-registry", "tr", "--source-registries", "sr", "--path-strategy", "invalid-strat"},
			expectErr:      true,
			stdErrContains: "unsupported path strategy: invalid-strat",
		},
		// We don't have args validation for override command itself (it takes no direct args)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := newRootCmd() // Use the real command setup
			// We aren't mocking yet, so execution errors are expected for valid flags
			currentGeneratorFactory = defaultGeneratorFactory // Ensure default factory is used

			_, stderr, err := executeCommand(rootCmd, tt.args...)

			if tt.expectErr {
				assert.Error(t, err, "Expected an error")
				if tt.stdErrContains != "" {
					assert.Contains(t, stderr, tt.stdErrContains, "stderr should contain expected message")
				}
			} else {
				assert.NoError(t, err, "Did not expect an error")
				assert.Empty(t, stderr, "stderr should be empty on success")
			}
		})
	}
}

func TestOverrideCmdExecution(t *testing.T) {
	// Restore original generator factory after tests
	originalGeneratorFactory := currentGeneratorFactory // Keep this
	defer func() {
		currentGeneratorFactory = originalGeneratorFactory
	}()

	// Default args for successful execution tests
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
		// REMOVE mockLoadMappings field
		// REMOVE mockGetStrategy field
		expectErr      bool
		stdOutContains string
		stdErrContains string
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
		},
		// REMOVE test case for load mappings error
		// REMOVE test case for get strategy error
		{
			name: "success with output file (flow check)",
			args: append(defaultArgs, "--output-file", "override_output.txt"),
			mockGeneratorFunc: func() (*override.OverrideFile, error) {
				return &override.OverrideFile{Overrides: map[string]interface{}{"key": "value"}}, nil
			},
			expectErr:      false,
			stdOutContains: "",
			stdErrContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Generator Mock (only this mock is needed now)
			if tt.mockGeneratorFunc != nil {
				currentGeneratorFactory = func(chartPath, targetRegistry string, sourceRegistries, excludeRegistries []string, pathStrategy strategy.PathStrategy, mappings *registry.RegistryMappings, strict bool, threshold int, loader chart.Loader) GeneratorInterface {
					// We might need to pass dummy/nil values for strategy/mappings here if the factory expects them
					// but the mock generator itself might not use them.
					return &mockGenerator{GenerateFunc: tt.mockGeneratorFunc}
				}
			} else {
				currentGeneratorFactory = defaultGeneratorFactory
			}

			// REMOVE Setup for LoadMappings Mock
			// REMOVE Setup for GetStrategy Mock

			rootCmd := newRootCmd()
			stdout, stderr, err := executeCommand(rootCmd, tt.args...)

			// Assertions remain largely the same
			if tt.expectErr {
				assert.Error(t, err)
				if tt.stdErrContains != "" {
					assert.Contains(t, stderr, tt.stdErrContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.stdOutContains != "" {
					assert.Contains(t, stdout, tt.stdOutContains)
				}
				assert.Empty(t, stderr, "stderr should be empty on success")
			}

			// Cleanup
			if strings.Contains(strings.Join(tt.args, " "), "--output-file override_output.txt") {
				_ = os.Remove("override_output.txt")
			}

			// REMOVE Restore for LoadMappings/GetStrategy
			currentGeneratorFactory = originalGeneratorFactory // Restore generator factory
		})
	}
}
