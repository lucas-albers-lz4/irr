package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOverrideFallbackTriggeredAndSucceeds tests the basic functionality
// of the fallback chart.
func TestOverrideFallbackTriggeredAndSucceeds(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Define the chart path
	chartPath := h.GetTestdataPath("charts/fallback-test")
	if chartPath == "" {
		t.Skip("fallback-test chart not found, skipping test")
	}

	// Run a simplified test using direct override command with --chart-path
	stdout, stderr, err := h.ExecuteIRRWithStderr(nil,
		"override",
		"--chart-path", chartPath,
		"--target-registry", "my-target-registry.com",
		"--source-registries", "docker.io",
		"--log-level", "debug",
	)
	require.NoError(t, err, "override command should succeed")
	t.Logf("Stderr output: %s", stderr)

	// Verify the content contains expected overrides
	assert.Contains(t, stdout, "registry: my-target-registry.com", "Output should include the target registry")
	assert.Contains(t, stdout, "repository: docker.io/library/nginx", "Output should include the image repository")
	assert.Contains(t, stdout, "tag: latest", "Output should include the image tag")
}
