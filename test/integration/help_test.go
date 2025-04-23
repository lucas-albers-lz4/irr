// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertHelpDefault checks if the help output for a given subcommand and flag
// contains the expected default value string.
func assertHelpDefault(t *testing.T, subcommand string, flagName string, expectedDefault string) {
	t.Helper()

	h := NewTestHarness(t) // Create a harness for execution context
	// No need to defer cleanup for help commands, they don't create files.

	output, stderr, err := h.ExecuteIRRWithStderr(nil, subcommand, "--help")
	require.NoError(t, err, "Executing --help for %s should succeed. Stderr: %s", subcommand, stderr)

	// Construct the expected default value string
	expectedDefaultText := fmt.Sprintf(`(default "%s")`, expectedDefault)
	// Construct the string to find the flag (e.g., "--log-level" or "-n, --namespace")
	// We are less strict here, just need to find the flag name itself.
	flagIdentifier := fmt.Sprintf("--%s", flagName)

	lines := strings.Split(output, "\n")
	foundFlagLine := ""
	found := false
	for _, line := range lines {
		// Check if the line contains the flag identifier
		if strings.Contains(line, flagIdentifier) {
			foundFlagLine = line
			found = true
			break // Assume first occurrence is the definition
		}
	}

	require.True(t, found, "Flag '%s' not found in help output for '%s'\nOutput:\n%s", flagIdentifier, subcommand, output)
	assert.Contains(t, foundFlagLine, expectedDefaultText, "Help text for --%s should contain default '%s' in line: %s", flagName, expectedDefault, foundFlagLine)
}

// TestHelpDefaults verifies that default values are shown in help text.
func TestHelpDefaults(t *testing.T) {
	t.Run("override command defaults", func(t *testing.T) {
		t.Parallel()
		assertHelpDefault(t, "override", "log-level", "info")
		assertHelpDefault(t, "override", "namespace", "default")
		// Add other override flags with defaults here if any, e.g., strategy?
		// assertHelpDefault(t, "override", "strategy", "prefix-source-registry") // Check actual default
		// assertHelpDefault(t, "override", "config", "") // Need to check how empty default is represented
	})

	t.Run("inspect command defaults", func(t *testing.T) {
		t.Parallel()
		assertHelpDefault(t, "inspect", "log-level", "info")
		assertHelpDefault(t, "inspect", "output-format", "yaml") // Corrected flag name
	})

	t.Run("validate command defaults", func(t *testing.T) {
		t.Parallel()
		assertHelpDefault(t, "validate", "log-level", "info")
		assertHelpDefault(t, "validate", "namespace", "default")
	})

	t.Run("config command defaults", func(t *testing.T) {
		t.Parallel()
		assertHelpDefault(t, "config", "log-level", "info")
		// Config file default is complex (env var, home dir), might not show explicitly
		// assertHelpDefault(t, "config", "config", "$HOME/.irr.yaml") // Verify actual representation
	})

	// Global flags (tested implicitly by checking one command, assuming they are consistent)
	t.Run("global flags defaults", func(t *testing.T) {
		t.Parallel()
		assertHelpDefault(t, "override", "log-level", "info") // Global flag tested via override
		// Add other global flags with defaults if any
	})
}
