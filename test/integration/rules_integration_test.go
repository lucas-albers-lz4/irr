package integration

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRulesSystemIntegration(t *testing.T) {
	// Create a temporary harness just to access GetTestdataPath early
	tempHarness := NewTestHarness(t)
	defer tempHarness.Cleanup() // Cleanup needed even if only used for path

	testCases := []struct {
		name           string
		chartPath      string
		sourceRegs     []string
		useRules       bool   // Flag to control rules application
		validateChart  bool   // Flag to control validation
		expectedFail   bool   // Whether the override/validation is expected to fail
		expectedErrMsg string // Expected error message snippet if expectedFail is true
	}{
		// Test case 1: Bitnami chart with rules enabled (should succeed)
		{
			name:          "Bitnami_RulesEnabled",
			chartPath:     tempHarness.GetTestdataPath("charts/clickhouse-operator"), // Use h.GetTestdataPath
			sourceRegs:    []string{"docker.io"},
			useRules:      true,
			validateChart: false, // Don't validate templating here, focus on override generation
			expectedFail:  false,
		},
		// Test case 2: Bitnami chart with rules disabled (should succeed, different output potentially)
		{
			name:          "Bitnami_RulesDisabled",
			chartPath:     tempHarness.GetTestdataPath("charts/clickhouse-operator"), // Use h.GetTestdataPath
			sourceRegs:    []string{"docker.io"},
			useRules:      false,
			validateChart: false,
			expectedFail:  false,
		},
		// Test case 3: Bitnami chart with validation (might fail due to known issues)
		{
			name:           "Bitnami_ValidationSucceeds",
			chartPath:      tempHarness.GetTestdataPath("charts/clickhouse-operator"), // Use h.GetTestdataPath
			sourceRegs:     []string{"docker.io"},
			useRules:       true,
			validateChart:  true,
			expectedFail:   false,                        // ADJUST if validation is known to fail for this chart
			expectedErrMsg: "template validation failed", // Example if failure expected
		},
		// Add more test cases as needed...
	}

	// Loop through test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewTestHarness(t)
			defer h.Cleanup()

			// Skip if chart path is empty (chart not found)
			require.NotEmpty(t, tc.chartPath, "Chart path cannot be empty for test case: %s", tc.name)

			h.SetupChart(tc.chartPath)
			h.SetRegistries("test.registry.io", tc.sourceRegs)

			outputFile := filepath.Join(h.tempDir, tc.name+"-overrides.yaml")

			args := []string{
				"override",
				"--chart-path", h.chartPath,
				"--target-registry", h.targetReg,
				"--source-registries", strings.Join(h.sourceRegs, ","),
				"--output-file", outputFile,
			}

			if !tc.useRules {
				args = append(args, "--disable-rules")
			}

			if !tc.validateChart {
				args = append(args, "--no-validate")
			}

			// Execute with context-aware flag enabled for this test - Corrected call signature
			_, stderr, err := h.ExecuteIRRWithStderr(nil, tc.useRules, args...)

			if tc.expectedFail {
				require.Error(t, err, "Expected command to fail. Stderr: %s", stderr)
				if tc.expectedErrMsg != "" {
					assert.Contains(t, stderr, tc.expectedErrMsg, "Stderr should contain expected error message")
				}
			} else {
				require.NoError(t, err, "Expected command to succeed. Stderr: %s", stderr)
				// Add further assertions on the generated file content if needed for success cases
			}
		})
	} // End of loop
}
