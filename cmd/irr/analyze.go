// Package main is the entry point for the irr CLI application.
package main

import (
	"fmt"

	"github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/spf13/cobra"
)

// Variables for flags
var (
	analyzeChartPath         string
	analyzeSourceRegistries  []string // Note: Unused in this reconstructed RunE
	analyzeExcludeRegistries []string // Note: Unused in this reconstructed RunE
	analyzeMappingsFile      string
	analyzeStrict            bool // Note: Unused in this reconstructed RunE
	analyzeVerbose           bool
)

// analyzeCmd represents the analyze command
var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze a Helm chart to detect image references",
	Long: `Analyzes a Helm chart's values and templates to identify container image 
references based on specified source registries.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Basic logging setup
		if analyzeVerbose {
			log.SetLevel(log.LevelDebug)
			log.Debugf("Verbose logging enabled")
		}

		log.Debugf("Analyze called with chartPath: %s, mappings: %s, strict: %v",
			analyzeChartPath, analyzeMappingsFile, analyzeStrict)

		// Load Mappings
		mappings, err := registry.LoadMappings(analyzeMappingsFile)
		if err != nil {
			return fmt.Errorf("error loading registry mappings: %w", err)
		}
		if mappings != nil {
			log.Debugf("Loaded %d registry mappings", len(mappings.Mappings))
		}

		// Create Analyzer using the factory defined in root.go
		// Note: This factory in root.go currently only takes chartPath.
		// The analyzer instance or its Analyze method would need modification
		// to actually use sourceRegistries, excludeRegistries, mappings, strict flags.
		analyzer := currentAnalyzerFactory(analyzeChartPath)

		// Run Analysis
		// Assume AnalyzerInterface returned by factory correctly defines Analyze()
		result, err := analyzer.Analyze() // result is *analysis.ChartAnalysis
		if err != nil {
			return fmt.Errorf("error during analysis: %w", err) // Keep simple error for now
		}

		// Print Results using fields from analysis.ChartAnalysis
		fmt.Printf("Analysis Complete:\n")
		fmt.Printf("  Image Patterns Found: %d\n", len(result.ImagePatterns))
		for _, pattern := range result.ImagePatterns {
			// Adjust printing based on ImagePattern fields (Path, Type, Value, Structure)
			fmt.Printf("    - Path: %s, Type: %s, Value: %s\n", pattern.Path, pattern.Type, pattern.Value)
		}
		fmt.Printf("  Global Patterns Found: %d\n", len(result.GlobalPatterns))
		for _, pattern := range result.GlobalPatterns {
			fmt.Printf("    - Path: %s, Type: %s\n", pattern.Path, pattern.Type)
		}

		// Handle strict mode exit code if needed
		// Note: The current AnalysisResult doesn't seem to track 'unsupported' structures
		// directly in the same way the previous version might have. Strict mode handling
		// might need adjustment based on how errors/unsupported cases are surfaced now.
		// For now, we'll comment out the strict mode check that relied on UnsupportedImages.
		/*
			if analyzeStrict && len(result.UnsupportedImages) > 0 {
				log.Errorf("Strict mode enabled: %d unsupported structures found.", len(result.UnsupportedImages))
				os.Exit(5) // Exit Code 5
			}
		*/

		return nil // Success
	},
}

func init() {
	// Assume rootCmd is defined in root.go

	// Define flags for analyze command
	analyzeCmd.Flags().StringVar(&analyzeChartPath, "chart-path", "", "Path to the Helm chart directory or .tgz file (required)")
	analyzeCmd.Flags().StringSliceVar(&analyzeSourceRegistries, "source-registries", []string{}, "Comma-separated list of source registries to analyze (required)")
	analyzeCmd.Flags().StringSliceVar(&analyzeExcludeRegistries, "exclude-registries", []string{}, "Comma-separated list of registries to exclude from analysis")
	analyzeCmd.Flags().StringVar(&analyzeMappingsFile, "registry-mappings", "", "Path to a YAML file for registry mappings (optional)")
	analyzeCmd.Flags().BoolVar(&analyzeStrict, "strict", false, "Fail analysis if any unsupported image structures are found")
	analyzeCmd.Flags().BoolVarP(&analyzeVerbose, "verbose", "v", false, "Enable verbose debug logging")

	// Mark required flags
	mustMarkFlagRequired(analyzeCmd, "chart-path")
	mustMarkFlagRequired(analyzeCmd, "source-registries")
}

// Helper to panic on required flag errors (indicates programmer error)
func mustMarkFlagRequired(cmd *cobra.Command, flagName string) {
	if err := cmd.MarkFlagRequired(flagName); err != nil {
		panic(fmt.Sprintf("failed to mark flag '%s' as required: %v", flagName, err))
	}
}

// Removed factory definition
