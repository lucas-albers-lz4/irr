package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// getRootCmd resets and returns the root command for testing
func getRootCmd() *cobra.Command {
	// Create a NEW root command instance for each test to avoid state pollution
	newRootCmd := &cobra.Command{
		Use:   "irr",
		Short: "Image Registry Redirect - Helm chart image registry override tool",
		Long: `irr (Image Relocation and Rewrite) is a tool for generating Helm override values
that redirect container image references from public registries to a private registry.

It can analyze Helm charts to identify image references and generate override values 
files compatible with Helm, pointing images to a new registry according to specified strategies.
It also supports linting image references for potential issues.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// If no arguments (subcommand) are provided, return an error.
			if len(args) == 0 {
				return errors.New("a subcommand is required. Use 'irr help' for available commands")
			}
			// Otherwise, let Cobra handle the subcommand or help text.
			return nil
		},
	}

	// Add persistent flags (copy from root.go init)
	newRootCmd.PersistentFlags().StringP("log-level", "l", "info", "Log level (debug, info, warn, error)")
	newRootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logging (overrides log-level to debug)")

	// Add subcommands by calling their constructors
	analyzeCmd := newAnalyzeCmd()
	overrideCmd := newOverrideCmd()

	// Do not set default values for required flags - let tests handle this
	newRootCmd.AddCommand(analyzeCmd)
	newRootCmd.AddCommand(overrideCmd)

	return newRootCmd
}

// executeCommand executes a cobra command with the given arguments
func executeCommand(cmd *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return buf.String(), err
}

// setupTestFS creates a temporary test directory and returns a memory filesystem
func setupTestFS(t *testing.T) (afero.Fs, string) {
	t.Helper()
	fs := afero.NewMemMapFs()
	testDir := filepath.Join(os.TempDir(), "irr-test")
	err := fs.MkdirAll(testDir, 0o755)
	require.NoErrorf(t, err, "failed to create test directory %s: %v", testDir, err)
	return fs, testDir
}

// createDummyChart creates a minimal Helm chart structure for testing
func createDummyChart(fs afero.Fs, chartDir string) error {
	if fs == nil {
		return fmt.Errorf("nil filesystem provided")
	}
	if chartDir == "" {
		return fmt.Errorf("empty chart directory path provided")
	}

	// Create Chart.yaml
	chartYaml := []byte(`apiVersion: v2
name: test-chart
version: 0.1.0
description: A test chart for irr
`)
	if err := afero.WriteFile(fs, filepath.Join(chartDir, "Chart.yaml"), chartYaml, 0o644); err != nil {
		return fmt.Errorf("failed to create Chart.yaml in %s: %w", chartDir, err)
	}

	// Create values.yaml with test image references
	valuesYaml := []byte(`image:
  repository: nginx
  tag: latest
  registry: docker.io

sidecar:
  image: busybox:latest

initContainer:
  image:
    repository: alpine
    tag: "3.14"
    registry: docker.io
`)
	if err := afero.WriteFile(fs, filepath.Join(chartDir, "values.yaml"), valuesYaml, 0o644); err != nil {
		return fmt.Errorf("failed to create values.yaml in %s: %w", chartDir, err)
	}

	// Create templates directory
	templatesDir := filepath.Join(chartDir, "templates")
	if err := fs.MkdirAll(templatesDir, 0o755); err != nil {
		return fmt.Errorf("failed to create templates directory %s: %w", templatesDir, err)
	}

	// Create a deployment.yaml template
	deploymentYaml := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-test
spec:
  template:
    spec:
      containers:
      - name: main
        image: "{{ .Values.image.registry }}/{{ .Values.image.repository }}:{{ .Values.image.tag }}"
      - name: sidecar
        image: "{{ .Values.sidecar.image }}"
      initContainers:
      - name: init
        image: "{{ .Values.initContainer.image.registry }}/{{ .Values.initContainer.image.repository }}:{{ .Values.initContainer.image.tag }}"
`)
	if err := afero.WriteFile(fs, filepath.Join(templatesDir, "deployment.yaml"), deploymentYaml, 0o644); err != nil {
		return fmt.Errorf("failed to create deployment.yaml in %s: %w", templatesDir, err)
	}

	return nil
}
