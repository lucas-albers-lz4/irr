package main

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetReleaseValues_EmptyName(t *testing.T) {
	t.Run("returns error for empty release name", func(t *testing.T) {
		vals, err := GetReleaseValues(context.Background(), "", "test-ns")
		require.Error(t, err, "Expected an error for empty release name")
		assert.Nil(t, vals, "Expected nil values map on error")
		assert.Contains(t, err.Error(), "release name is empty", "Error message should indicate empty release name")
	})
}

func TestGetReleaseNamespace(t *testing.T) {
	const testDefaultNs = "default"

	t.Run("namespace from flag", func(t *testing.T) {
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
		originalEnv := os.Getenv("HELM_NAMESPACE")
		_ = os.Setenv("HELM_NAMESPACE", "env-ns")
		defer func() { _ = os.Setenv("HELM_NAMESPACE", originalEnv) }() // Restore original value

		ns := GetReleaseNamespace(cmd)
		assert.Equal(t, "env-ns", ns)
	})

	t.Run("namespace from env var takes precedence over default", func(t *testing.T) {
		// This test is implicitly covered by the one above, but making it explicit
		cmd := &cobra.Command{}
		cmd.Flags().String("namespace", "", "Namespace")

		originalEnv := os.Getenv("HELM_NAMESPACE")
		_ = os.Setenv("HELM_NAMESPACE", "env-ns-precedence")
		defer func() { _ = os.Setenv("HELM_NAMESPACE", originalEnv) }()

		ns := GetReleaseNamespace(cmd)
		assert.Equal(t, "env-ns-precedence", ns)
	})

	t.Run("default namespace when flag and env var are empty", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("namespace", "", "Namespace")

		// Ensure env var is empty or unset
		originalEnv, envSet := os.LookupEnv("HELM_NAMESPACE")
		_ = os.Unsetenv("HELM_NAMESPACE")
		defer func() {
			if envSet {
				_ = os.Setenv("HELM_NAMESPACE", originalEnv)
			} else {
				_ = os.Unsetenv("HELM_NAMESPACE") // Ensure it remains unset if originally unset
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
		originalEnv := os.Getenv("HELM_NAMESPACE")
		_ = os.Setenv("HELM_NAMESPACE", "env-ns-ignored")
		defer func() { _ = os.Setenv("HELM_NAMESPACE", originalEnv) }()

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
