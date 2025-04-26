package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewValidateCommand tests the creation and flag setup of the validate command
func TestNewValidateCommand(t *testing.T) {
	cmd := newValidateCmd()

	// Check if flags are correctly defined
	assert.NotNil(t, cmd.Flags().Lookup("chart-path"), "chart-path flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("release-name"), "release-name flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("values"), "values flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("namespace"), "namespace flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("output-file"), "output-file flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("strict"), "strict flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("kube-version"), "kube-version flag should be defined")

	// Check default values
	chartPath, err := cmd.Flags().GetString("chart-path")
	require.NoError(t, err, "Failed to get chart-path flag")
	assert.Equal(t, "", chartPath, "Default chart-path should be empty")

	namespace, err := cmd.Flags().GetString("namespace")
	require.NoError(t, err, "Failed to get namespace flag")
	assert.Equal(t, "default", namespace, "Default namespace should be 'default'")

	strict, err := cmd.Flags().GetBool("strict")
	require.NoError(t, err, "Failed to get strict flag")
	assert.False(t, strict, "Default strict mode should be false")
}
