package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/lalbers/helm-image-override/pkg/analysis"
	"github.com/spf13/cobra"
)

func newAnalyzeCmd() *cobra.Command {
	var outputFormat string
	var outputFile string

	cmd := &cobra.Command{
		Use:   "analyze [chart-path]",
		Short: "Analyze a Helm chart for image patterns",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chartPath := args[0]

			// Create analyzer
			analyzer := analysis.NewAnalyzer(chartPath)

			// Perform analysis
			result, err := analyzer.Analyze()
			if err != nil {
				return fmt.Errorf("analysis failed: %w", err)
			}

			// Format output
			var output string
			if outputFormat == "json" {
				jsonBytes, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal JSON: %w", err)
				}
				output = string(jsonBytes)
			} else {
				output = formatTextOutput(result)
			}

			// Write output
			if outputFile != "" {
				if err := os.WriteFile(outputFile, []byte(output), 0644); err != nil {
					return fmt.Errorf("failed to write output file: %w", err)
				}
			} else {
				fmt.Println(output)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text or json)")
	cmd.Flags().StringVarP(&outputFile, "file", "f", "", "Output file (defaults to stdout)")

	return cmd
}

func formatTextOutput(analysis *analysis.ChartAnalysis) string {
	var sb strings.Builder
	sb.WriteString("Chart Analysis\n\n")

	sb.WriteString("Pattern Summary:\n")
	sb.WriteString(fmt.Sprintf("Total image patterns: %d\n", len(analysis.ImagePatterns)))
	sb.WriteString(fmt.Sprintf("Global patterns: %d\n", len(analysis.GlobalPatterns)))
	sb.WriteString("\n")

	if len(analysis.ImagePatterns) > 0 {
		sb.WriteString("Image Patterns:\n")
		w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
		_, err := fmt.Fprintln(w, "PATH\tTYPE\tDETAILS\tCOUNT")
		if err != nil {
			return fmt.Sprintf("Error writing output: %v", err)
		}
		for _, p := range analysis.ImagePatterns {
			details := ""
			if p.Type == "map" {
				reg := p.Structure["registry"]
				repo := p.Structure["repository"]
				tag := p.Structure["tag"]
				details = fmt.Sprintf("registry=%v, repository=%v, tag=%v", reg, repo, tag)
			} else {
				details = p.Value
			}
			_, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", p.Path, p.Type, details, p.Count)
			if err != nil {
				return fmt.Sprintf("Error writing output: %v", err)
			}
		}
		if err := w.Flush(); err != nil {
			return fmt.Sprintf("Error flushing output: %v", err)
		}
		sb.WriteString("\n")
	}

	if len(analysis.GlobalPatterns) > 0 {
		sb.WriteString("Global Patterns:\n")
		for _, p := range analysis.GlobalPatterns {
			sb.WriteString(fmt.Sprintf("- %s\n", p.Path))
		}
	}

	return sb.String()
}
