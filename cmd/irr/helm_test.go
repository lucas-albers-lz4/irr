package main

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setTestEnvVar sets an environment variable for testing and returns a function to restore the original value
func setTestEnvVar(t *testing.T, name, value string) func() {
	originalValue := os.Getenv(name)
	if err := os.Setenv(name, value); err != nil {
		t.Fatalf("Failed to set %s: %v", name, err)
	}

	return func() {
		if err := os.Setenv(name, originalValue); err != nil {
			t.Logf("Warning: Failed to restore %s: %v", name, err)
		}
	}
}

func TestGetReleaseValues_EmptyName(t *testing.T) {
	t.Run("returns error for empty release name", func(t *testing.T) {
		vals, err := GetReleaseValues(context.Background(), "", "test-ns")
		require.Error(t, err, "Expected an error for empty release name")
		assert.Nil(t, vals, "Expected nil values map on error")
		assert.Contains(t, err.Error(), "release name is empty", "Error message should indicate empty release name")
	})
}

func TestGetReleaseNamespace(t *testing.T) {
	testDefaultNs := validateTestNamespace // Use constant

	t.Run("flag has precedence", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("namespace", "", "Namespace")
		err := cmd.Flags().Set("namespace", "flag-ns")
		require.NoError(t, err)

		ns := GetReleaseNamespace(cmd)
		assert.Equal(t, "flag-ns", ns)
	})

	t.Run("namespace from env var when flag is empty", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("namespace", "", "Namespace")

		// Set environment variable
		defer setTestEnvVar(t, "HELM_NAMESPACE", "env-ns")()

		ns := GetReleaseNamespace(cmd)
		assert.Equal(t, "env-ns", ns)
	})

	t.Run("namespace from env var takes precedence over default", func(t *testing.T) {
		// This test is implicitly covered by the one above, but making it explicit
		cmd := &cobra.Command{}
		cmd.Flags().String("namespace", "", "Namespace")

		defer setTestEnvVar(t, "HELM_NAMESPACE", "env-ns-precedence")()

		ns := GetReleaseNamespace(cmd)
		assert.Equal(t, "env-ns-precedence", ns)
	})

	t.Run("default namespace when flag and env var are empty", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("namespace", "", "Namespace")

		// Ensure env var is empty or unset
		originalEnv, envSet := os.LookupEnv("HELM_NAMESPACE")
		if err := os.Unsetenv("HELM_NAMESPACE"); err != nil {
			t.Fatalf("Failed to unset HELM_NAMESPACE: %v", err)
		}
		defer func() {
			if envSet {
				if err := os.Setenv("HELM_NAMESPACE", originalEnv); err != nil {
					t.Logf("Warning: Failed to restore HELM_NAMESPACE: %v", err)
				}
			} else {
				if err := os.Unsetenv("HELM_NAMESPACE"); err != nil {
					t.Logf("Warning: Failed to unset HELM_NAMESPACE: %v", err)
				}
			}
		}()

		ns := GetReleaseNamespace(cmd)
		assert.Equal(t, testDefaultNs, ns)
	})

	t.Run("flag takes precedence over env var", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("namespace", "", "Namespace")
		err := cmd.Flags().Set("namespace", "flag-ns-precedence")
		require.NoError(t, err)

		// Set environment variable (should be ignored)
		defer setTestEnvVar(t, "HELM_NAMESPACE", "env-ns-ignored")()

		ns := GetReleaseNamespace(cmd)
		assert.Equal(t, "flag-ns-precedence", ns)
	})

	// Note: Testing the case where GetString fails is hard because
	// cobra/pflag usually handle flag definition errors earlier.
}

func TestGetChartPathFromRelease_EmptyName(t *testing.T) {
	t.Run("returns error for empty release name", func(t *testing.T) {
		path, err := GetChartPathFromRelease("")
		require.Error(t, err, "Expected an error for empty release name")
		assert.Empty(t, path, "Expected empty path on error")
		assert.Contains(t, err.Error(), "release name is empty", "Error message should indicate empty release name")
	})

	// NOTE: Testing the full success path requires extensive mocking of Helm actions
	// (Get, Pull, LocateChart) and filesystem operations, which is better suited
	// for integration tests or command-level tests using a mocked Helm client.
}

// TestGetHelmSettings verifies that GetHelmSettings returns a non-nil settings object.
func TestGetHelmSettings(t *testing.T) {
	settings := GetHelmSettings()
	assert.NotNil(t, settings, "GetHelmSettings should return a non-nil object")
	// We don't assert specific values within settings as they depend on the environment.
}

// TestInitHelmPlugin verifies that plugin-specific flags are made visible.
func TestInitHelmPlugin(t *testing.T) {
	// Need to ensure rootCmd has the flags defined (usually done in root.go init)
	// For isolated testing, create a temporary root command or ensure init ran.
	// Using the actual rootCmd assumes init() in root.go has run.
	cmd := getRootCmd() // Assuming getRootCmd() exists and returns the configured rootCmd

	// Ensure flags exist (redundant if getRootCmd ensures init)
	require.NotNil(t, cmd.PersistentFlags().Lookup("release-name"), "release-name flag missing")
	require.NotNil(t, cmd.PersistentFlags().Lookup("namespace"), "namespace flag missing")

	// Optional: Set flags to hidden initially to verify the change
	err := cmd.PersistentFlags().MarkHidden("release-name")
	require.NoError(t, err)
	err = cmd.PersistentFlags().MarkHidden("namespace")
	require.NoError(t, err)

	// Call the function
	initHelmPlugin()

	// Assert flags are no longer hidden
	assert.False(t, cmd.PersistentFlags().Lookup("release-name").Hidden, "release-name flag should be visible after initHelmPlugin")
	assert.False(t, cmd.PersistentFlags().Lookup("namespace").Hidden, "namespace flag should be visible after initHelmPlugin")
}

// TestRemoveHelmPluginFlags verifies that plugin-specific flags are hidden.
func TestRemoveHelmPluginFlags(t *testing.T) {
	// Create a dummy command with the flags
	cmd := &cobra.Command{}
	addReleaseFlag(cmd) // Use helper to add the flag
	addNamespaceFlag(cmd)

	// Ensure flags start visible (or their default state)
	require.False(t, cmd.PersistentFlags().Lookup("release-name").Hidden, "release-name should start visible")
	require.False(t, cmd.PersistentFlags().Lookup("namespace").Hidden, "namespace should start visible")

	// Call the function
	removeHelmPluginFlags(cmd)

	// Assert flags are now hidden
	assert.True(t, cmd.PersistentFlags().Lookup("release-name").Hidden, "release-name flag should be hidden after removeHelmPluginFlags")
	assert.True(t, cmd.PersistentFlags().Lookup("namespace").Hidden, "namespace flag should be hidden after removeHelmPluginFlags")

	// Test idempotency (calling again shouldn't error or change state)
	removeHelmPluginFlags(cmd)
	assert.True(t, cmd.PersistentFlags().Lookup("release-name").Hidden, "release-name flag should remain hidden")
	assert.True(t, cmd.PersistentFlags().Lookup("namespace").Hidden, "namespace flag should remain hidden")
}
