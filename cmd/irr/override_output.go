package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/chart"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/strategy"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// handleGenerateError converts generator errors to appropriate exit code errors
func handleGenerateError(err error) error {
	switch {
	case errors.Is(err, strategy.ErrThresholdExceeded):
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitThresholdError,
			Err:  fmt.Errorf("failed to process chart: %w", err),
		}
	case errors.Is(err, chart.ErrChartNotFound) || errors.Is(err, chart.ErrChartLoadFailed):
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartParsingError,
			Err:  fmt.Errorf("failed to process chart: %w", err),
		}
	case errors.Is(err, chart.ErrUnsupportedStructure):
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitUnsupportedStructure,
			Err:  fmt.Errorf("failed to process chart: %w", err),
		}
	default:
		// Default to image processing error for any other errors
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitImageProcessingError,
			Err:  fmt.Errorf("failed to process chart: %w", err),
		}
	}
}

// outputOverrides handles writing the generated YAML or JSON to the correct destination
// (stdout or file) or logging it for dry-run.
func outputOverrides(cmd *cobra.Command, data []byte, outputFile string, dryRun bool) error {
	// Determine output format
	outputFormat, err := cmd.Flags().GetString("output-format")
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get output-format flag: %w", err),
		}
	}
	outputFormat = strings.ToLower(outputFormat)
	if outputFormat != outputFormatYAML && outputFormat != outputFormatJSON {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("unsupported output format %q; supported formats: yaml, json", outputFormat),
		}
	}

	// Marshal to the requested format if needed
	var output []byte
	if outputFormat == outputFormatJSON {
		var obj interface{}
		if err := yaml.Unmarshal(data, &obj); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to unmarshal YAML for JSON output: %w", err),
			}
		}
		output, err = json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal overrides to JSON: %w", err),
			}
		}
	} else {
		output = data // Already YAML
	}

	switch {
	case dryRun:
		log.Info("DRY RUN: Displaying generated override values (stdout)")
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(output)); err != nil {
			log.Error("Failed to write dry-run output to stdout", "error", err)
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write dry-run output to stdout: %w", err),
			}
		}
		return nil
	case outputFile == "":
		_, err := fmt.Fprintln(cmd.OutOrStdout(), string(output))
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to write overrides to stdout: %w", err),
			}
		}
		log.Info("Override values printed to stdout")
		return nil
	default:
		exists, err := afero.Exists(AppFs, outputFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to check if output file exists: %w", err),
			}
		}
		if exists {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("output file '%s' already exists", outputFile),
			}
		}
		dir := filepath.Dir(outputFile)
		if dir != "" && dir != "." {
			if mkDirErr := AppFs.MkdirAll(dir, fileutil.ReadWriteExecuteUserReadExecuteOthers); mkDirErr != nil {
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("failed to create output directory: %w", mkDirErr),
				}
			}
		}
		if writeErr := afero.WriteFile(AppFs, outputFile, output, fileutil.ReadWriteUserReadOthers); writeErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write output file '%s': %w", outputFile, writeErr),
			}
		}
		absPath, err := filepath.Abs(outputFile)
		if err == nil {
			log.Info("Override values written", "path", absPath)
		} else {
			log.Info("Override values written", "path", outputFile)
		}
		return nil
	}
}
