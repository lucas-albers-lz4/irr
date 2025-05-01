package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestInspectParentChart(t *testing.T) {
	// Set up test environment (Removed call to non-existent testutil.SetupTestEnv)
	// testutil.SetupTestEnv(t)
	cmd := rootCmd
	parentChartPath := filepath.Join("..", "..", "test-data", "charts", "parent-test")

	// Ensure the chart path exists
	_, err := os.Stat(parentChartPath)
	require.NoError(t, err, "Parent test chart path does not exist: %s", parentChartPath)

	// Execute the inspect command, capturing stdout and ignoring stderr for now
	// Assuming executeCommand returns (stdout string, stderr string, error)
	output, _, err := executeCommand(cmd, "--chart-path", parentChartPath)
	require.NoError(t, err, "Inspect command failed")

	// Define expected images (canonical format)
	// Note: Order doesn't matter due to parsing into map, but list canonical forms.
	// The analyzer now produces canonical forms (e.g., docker.io/library/nginx:latest)
	expectedImages := []string{
		"docker.io/library/nginx:1.14.2",           // From parent values.yaml
		"docker.io/library/busybox:1.28",           // From child values.yaml
		"docker.io/library/another-image:1.0",      // From another-child values.yaml
		"docker.io/global/image:latest",            // From global definition
		"my-registry.com/parent/override:tag",      // Override from parent's child section
		"docker.io/another-child/direct-image:1.0", // Directly defined in another-child section
	}

	// Parse the YAML output
	var result map[string]interface{}
	err = yaml.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "Failed to unmarshal inspect output YAML")

	// Check if images key exists and is a list
	imagesIntf, ok := result["images"]
	require.True(t, ok, "Output YAML missing 'images' key")
	imagesList, ok := imagesIntf.([]interface{})
	require.True(t, ok, "'images' key is not a list")

	// Convert actual images to a slice of strings for comparison
	actualImages := make([]string, len(imagesList))
	for i, imgIntf := range imagesList {
		imgStr, ok := imgIntf.(string)
		require.True(t, ok, fmt.Sprintf("Image list item %d is not a string: %v", i, imgIntf))
		actualImages[i] = imgStr
	}

	// Use ElementsMatch for order-independent comparison
	assert.ElementsMatch(t, expectedImages, actualImages, "Inspect output images mismatch")
}

// Helper function executeCommand is assumed to be defined elsewhere (e.g., root_test.go)
