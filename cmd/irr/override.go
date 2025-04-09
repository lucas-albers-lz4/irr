package main

import (
	"github.com/spf13/cobra"
)

// newOverrideCmd creates the cobra command for the 'override' operation.
func newOverrideCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "override",
		Short: "Analyzes a Helm chart and generates image override values",
		Long: "Analyzes a Helm chart to find all container image references (both direct string values " +
			"and map-based structures like 'image.repository', 'image.tag'). It then generates a " +
			"Helm-compatible values file that overrides these references to point to a specified " +
			"target registry, using a defined path strategy.\n\n" +
			"Supports filtering images based on source registries and excluding specific registries. " +
			"Can also utilize a registry mapping file for more complex source-to-target mappings." +
			"Includes options for dry-run, strict validation, and success thresholds.",
		Args: cobra.NoArgs, // Override command does not take positional arguments
		RunE: func(cmd *cobra.Command, args []string) error {
			// Flag retrieval and core logic are handled by runOverride in root.go for now.
			// Ideally, runOverride logic specific to this command would move here.
			return runOverride(cmd, args) // Call the existing logic function
		},
	}

	// Define flags specific to the override command
	// Required flags
	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball (required)")
	cmd.Flags().StringP("target-registry", "t", "", "Target container registry URL (required)")
	cmd.Flags().StringSliceP("source-registries", "s", []string{}, "Source container registry URLs to relocate (required, comma-separated or multiple flags)")

	// Optional flags
	cmd.Flags().StringP("output-file", "o", "", "Output file path for the generated overrides YAML (default: stdout)")
	cmd.Flags().StringP("strategy", "p", "prefix-source-registry", "Path generation strategy ('prefix-source-registry')")
	cmd.Flags().Bool("dry-run", false, "Perform analysis and print overrides to stdout without writing to file")
	cmd.Flags().Bool("strict", false, "Enable strict mode (fail on any image parsing/processing error)")
	cmd.Flags().StringSlice("exclude-registries", []string{}, "Container registry URLs to exclude from relocation (comma-separated or multiple flags)")
	cmd.Flags().Int("threshold", 0, "Minimum percentage of images successfully processed for the command to succeed (0-100, 0 disables)")
	cmd.Flags().String("registry-file", "", "Path to a YAML file containing registry mappings (source: target)")
	cmd.Flags().Bool("validate", false, "Run 'helm template' with generated overrides to validate chart renderability")

	// Mark required flags
	_ = cmd.MarkFlagRequired("chart-path")
	_ = cmd.MarkFlagRequired("target-registry")
	// Note: source-registries is logically required but an empty list has meaning (process all non-excluded),
	// so we don't mark it strictly required by Cobra here. Validation happens in runOverride.

	return cmd
}

// --- Helper functions previously in root.go, potentially move here later ---

// Example: Placeholder for potentially moving parts of runOverride here
// func executeOverrideLogic(cmd *cobra.Command, args []string) error { ... }
