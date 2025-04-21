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
	testDefaultNs := "default" // Default namespace for this test suite

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
