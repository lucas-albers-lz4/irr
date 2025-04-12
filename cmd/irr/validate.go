package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/cobra"
)

// newValidateCmd creates a new validate command
func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [flags]",
		Short: "Validate a Helm chart with override values",
		Long: "Validates a Helm chart after applying override values files. " +
			"This command executes 'helm template' with the specified values to ensure " +
			"that the chart can be successfully rendered with the provided overrides.",
		Args: cobra.NoArgs,
		RunE: runValidate,
	}

	// Add flags
	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball (required)")
	cmd.Flags().StringP("release-name", "n", "release", "Release name to use for templating")
	cmd.Flags().StringSliceP("values", "f", []string{}, "One or more values files to use for validation (can be specified multiple times)")
	cmd.Flags().StringSlice("set", []string{}, "Set values on the command line (can be specified multiple times)")
	cmd.Flags().StringP("output-file", "o", "", "Output file for helm template result (default: discard)")
	cmd.Flags().Bool("debug-template", false, "Show full helm template output even on success")

	// Mark required flags
	mustMarkFlagRequired(cmd, "chart-path")
	mustMarkFlagRequired(cmd, "values")

	return cmd
}

// runValidate implements the validate command logic
func runValidate(cmd *cobra.Command, _ []string) error {
	// Get chart path
	chartPath, err := cmd.Flags().GetString("chart-path")
	if err != nil || chartPath == "" {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitMissingRequiredFlag,
			Err:  fmt.Errorf("required flag \"chart-path\" not set"),
		}
	}

	// Normalize chart path
	chartPath, err = filepath.Abs(chartPath)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get absolute path for chart: %w", err),
		}
	}

	// Check if chart exists
	_, err = os.Stat(chartPath)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartNotFound,
			Err:  fmt.Errorf("chart path not found or inaccessible: %s", chartPath),
		}
	}

	// Get release name
	releaseName, err := cmd.Flags().GetString("release-name")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get release-name flag: %w", err),
		}
	}

	// Get values files
	valuesFiles, err := cmd.Flags().GetStringSlice("values")
	if err != nil || len(valuesFiles) == 0 {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitMissingRequiredFlag,
			Err:  fmt.Errorf("required flag \"values\" not set"),
		}
	}

	// Check that values files exist
	for _, valueFile := range valuesFiles {
		_, err := os.Stat(valueFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err:  fmt.Errorf("values file not found or inaccessible: %s", valueFile),
			}
		}
	}

	// Get set values
	setValues, err := cmd.Flags().GetStringSlice("set")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get set flag: %w", err),
		}
	}

	// Get output file
	outputFile, err := cmd.Flags().GetString("output-file")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-file flag: %w", err),
		}
	}

	// Get debug-template flag
	debugTemplate, err := cmd.Flags().GetBool("debug-template")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get debug-template flag: %w", err),
		}
	}

	// Check if we need to get values from a release
	// If release-name is specified and --get-release-values is not false, attempt to get values
	if releaseName != "release" && releaseName != "" {
		log.Infof("Using release name %s", releaseName)
	}

	// Build helm template command
	helmArgs := []string{"template", releaseName, chartPath}

	// Add values files
	for _, valueFile := range valuesFiles {
		helmArgs = append(helmArgs, "--values", valueFile)
	}

	// Add set values
	for _, setValue := range setValues {
		helmArgs = append(helmArgs, "--set", setValue)
	}

	log.Infof("Executing: helm %s", strings.Join(helmArgs, " "))
	// #nosec G204 -- We need to allow variable arguments to helm command
	cmd2 := exec.Command("helm", helmArgs...)

	var stdout, stderr bytes.Buffer
	cmd2.Stdout = &stdout
	cmd2.Stderr = &stderr

	// Execute helm template
	err = cmd2.Run()

	// Check if command succeeded
	if err != nil {
		log.Errorf("Helm template failed: %v", err)
		log.Errorf("Stderr: %s", stderr.String())

		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("helm template failed: %w\n%s", err, stderr.String()),
		}
	}

	// Handle output
	switch {
	case outputFile != "":
		if err := os.WriteFile(outputFile, stdout.Bytes(), fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write template output to file: %w", err),
			}
		}
		log.Infof("Template output written to %s", outputFile)
	case debugTemplate:
		fmt.Println(stdout.String())
	default:
		log.Infof("Helm template completed successfully")
	}

	return nil
}
