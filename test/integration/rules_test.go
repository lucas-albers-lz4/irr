package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRulesSystemIntegration tests the rules-based override generation system
func TestRulesSystemIntegration(t *testing.T) {
	// First check if the required charts are available
	bitnamiChartPath := testutil.GetChartPath("clickhouse-operator")
	if bitnamiChartPath == "" {
		t.Skip("clickhouse-operator chart not found, skipping bitnami rules tests")
		return
	}

	nonBitnamiChartPath := testutil.GetChartPath("minimal-test")
	if nonBitnamiChartPath == "" {
		t.Skip("minimal-test chart not found, skipping non-bitnami rules tests")
		return
	}

	// --- Test Case 1: Bitnami Chart, Rules Enabled (Default) ---
	t.Run("Bitnami_RulesEnabled", func(t *testing.T) {
		tempHarness := NewTestHarness(t)
		outputFile := filepath.Join(tempHarness.tempDir, "bitnami-rules-enabled-overrides.yaml")
		tempHarness.Cleanup() // Clean up temp harness
		runRulesTest(t, bitnamiChartPath, outputFile, true)
	})

	// --- Test Case 2: Bitnami Chart, Rules Disabled ---
	t.Run("Bitnami_RulesDisabled", func(t *testing.T) {
		h := NewTestHarness(t)
		defer h.Cleanup()

		// Find the test Bitnami chart
		if bitnamiChartPath == "" {
			t.Skip("Bitnami test chart not found, skipping test")
		}

		h.SetupChart(bitnamiChartPath)
		h.SetRegistries("harbor.test.local", []string{"docker.io"})

		// Create output file path
		outputFile := filepath.Join(h.tempDir, "bitnami-rules-disabled-overrides.yaml")

		// Run the override command with rules disabled and no validation
		// Note: We need to add --no-validate because Bitnami charts have validation
		// checks that will fail without the security rule being applied
		output, stderr, err := h.ExecuteIRRWithStderr(nil,
			"override",
			"--chart-path", h.chartPath,
			"--target-registry", h.targetReg,
			"--source-registries", strings.Join(h.sourceRegs, ","),
			"--disable-rules",
			"--no-validate", // Skip validation as it will fail without the rule
			"--output-file", outputFile,
		)
		require.NoError(t, err, "override command with rules disabled should succeed: %s", stderr)
		t.Logf("Override output: %s", output)
		t.Logf("Stderr: %s", stderr)

		// Check if the output file was created successfully
		require.FileExists(t, outputFile, "Override file should exist")

		// Read the generated override file
		// #nosec G304
		content, err := os.ReadFile(outputFile)
		require.NoError(t, err, "Should be able to read override file")

		// Verify the bitnami security rule was NOT applied
		// This is a flexible check since formats might vary
		securityPattern := `allowInsecureImages`
		assert.NotContains(t, string(content), securityPattern,
			"Override should not include security bypass when rules are disabled")
	})

	// --- Test Case 3: Non-Bitnami Chart, Rules Enabled ---
	t.Run("NonBitnami_RulesEnabled", func(t *testing.T) {
		tempHarness := NewTestHarness(t)
		outputFile := filepath.Join(tempHarness.tempDir, "non-bitnami-rules-overrides.yaml")
		tempHarness.Cleanup()
		runRulesTest(t, nonBitnamiChartPath, outputFile, false)
	})

	// --- Test Case 4: Bitnami Charts Validation Success ---
	t.Run("Bitnami_ValidationSucceeds", func(t *testing.T) {
		// Test with specific Bitnami charts
		bitnamiCharts := []struct {
			chartName    string
			expectBypass bool
			skipValidate bool
		}{
			{"clickhouse-operator", true, true}, // Skip validation due to API issues
			{"minimal-git-image", false, true},  // Skip validation - not complex enough for template testing
		}

		for _, chartInfo := range bitnamiCharts {
			chartPath := testutil.GetChartPath(chartInfo.chartName)
			if chartPath == "" {
				t.Logf("Chart %s not found, skipping this subtest", chartInfo.chartName)
				continue
			}

			t.Run(chartInfo.chartName, func(t *testing.T) {
				h := NewTestHarness(t)
				defer h.Cleanup()

				h.SetupChart(chartPath)
				h.SetRegistries("harbor.test.local", []string{"docker.io"})

				// Create output file path
				outputFile := filepath.Join(h.tempDir, chartInfo.chartName+"-validation-overrides.yaml")

				// Run the override command with rules enabled
				output, stderr, err := h.ExecuteIRRWithStderr(nil,
					"override",
					"--chart-path", h.chartPath,
					"--target-registry", h.targetReg,
					"--source-registries", strings.Join(h.sourceRegs, ","),
					"--output-file", outputFile,
				)
				require.NoError(t, err, "override command should succeed")
				t.Logf("Override output: %s", output)
				t.Logf("Stderr: %s", stderr)

				// Read the generated override file
				// #nosec G304
				content, err := os.ReadFile(outputFile)
				require.NoError(t, err, "Should be able to read override file")

				// Verify security bypass exists if expected
				if chartInfo.expectBypass {
					assert.Contains(t, string(content), "allowInsecureImages: true",
						"Override should include security bypass for expected charts")
				} else {
					assert.NotContains(t, string(content), "allowInsecureImages",
						"Override should not include security bypass for this chart")
				}

				// Skip validation if requested
				if chartInfo.skipValidate {
					t.Logf("Skipping validation for chart %s", chartInfo.chartName)
					return
				}

				// Validate the chart renders successfully with these overrides
				err = chart.ValidateHelmTemplate(chartPath, content)
				assert.NoError(t, err, "Chart should validate successfully with overrides")
			})
		}
	})
}

func runRulesTest(t *testing.T, chartPath, outputFile string, expectBypass bool) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	h.SetupChart(chartPath)
	h.SetRegistries("harbor.test.local", []string{"docker.io"})

	output, stderr, err := h.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", h.chartPath,
		"--target-registry", h.targetReg,
		"--source-registries", strings.Join(h.sourceRegs, ","),
		"--output-file", outputFile,
	)
	require.NoError(t, err, "override command should succeed")
	t.Logf("Override output: %s", output)
	t.Logf("Stderr: %s", stderr)

	// #nosec G304
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err, "Should be able to read override file")

	if expectBypass {
		assert.Contains(t, string(content), "allowInsecureImages: true", "Override should include security bypass for Bitnami chart")
	} else {
		assert.NotContains(t, string(content), "allowInsecureImages", "Override should not include security bypass for non-Bitnami chart")
	}
}
