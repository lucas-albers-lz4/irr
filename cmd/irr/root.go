package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// AppFs provides an abstraction over the filesystem.
// Defaults to the OS filesystem, can be replaced with a memory map for tests.
var AppFs afero.Fs = afero.NewOsFs()

// Global flag variables (consider scoping if appropriate)
var (
	chartPath            string
	targetRegistry       string
	sourceRegistries     string
	outputFile           string // Used by multiple commands
	pathStrategy         string
	verbose              bool
	dryRun               bool
	strictMode           bool
	excludeRegistries    string
	threshold            int
	debugEnabled         bool // Used by multiple commands
	registryMappingsFile string
)

// Exit codes (keep public if needed elsewhere, otherwise consider keeping private)
const (
	ExitSuccess                 = 0
	ExitGeneralRuntimeError     = 1
	ExitInputConfigurationError = 2
	ExitChartParsingError       = 3
	ExitImageProcessingError    = 4
	ExitUnsupportedStructError  = 5
	ExitThresholdNotMetError    = 6
	ExitCodeInvalidStrategy     = 7
	ExitHelmTemplateError       = 8
)

// ExitCodeError struct (keep public if needed)
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	return e.Err.Error()
}

func (e *ExitCodeError) Unwrap() error {
	return e.Err
}

// wrapExitCodeError helper (keep public if needed)
func wrapExitCodeError(code int, baseMsg string, originalErr error) error {
	combinedMsg := fmt.Sprintf("%s: %s", baseMsg, originalErr.Error())
	return &ExitCodeError{Code: code, Err: errors.New(combinedMsg)}
}

// --- Factory for Analyzer ---
// Allows overriding for testing
type analyzerFactoryFunc func(chartPath string) AnalyzerInterface

// Default factory creates the real analyzer
var defaultAnalyzerFactory analyzerFactoryFunc = func(chartPath string) AnalyzerInterface {
	// Pass nil loader to use the default Helm loader
	return analysis.NewAnalyzer(chartPath, nil)
}

// Keep track of the current factory (can be replaced in tests)
var currentAnalyzerFactory = defaultAnalyzerFactory

// Interface matching analysis.Analyzer for mocking
type AnalyzerInterface interface {
	Analyze() (*analysis.ChartAnalysis, error)
}

// --- End Factory ---

// --- Factory for Generator ---
// Interface matching chart.Generator for mocking
type GeneratorInterface interface {
	Generate() (*override.OverrideFile, error)
}

// Allows overriding for testing
type generatorFactoryFunc func(chartPath, targetRegistry string, sourceRegistries, excludeRegistries []string, pathStrategy strategy.PathStrategy, mappings *registry.RegistryMappings, strict bool, threshold int, loader chart.Loader) GeneratorInterface

// Default factory creates the real generator
var defaultGeneratorFactory generatorFactoryFunc = func(chartPath, targetRegistry string, sourceRegistries, excludeRegistries []string, pathStrategy strategy.PathStrategy, mappings *registry.RegistryMappings, strict bool, threshold int, loader chart.Loader) GeneratorInterface {
	return chart.NewGenerator(chartPath, targetRegistry, sourceRegistries, excludeRegistries, pathStrategy, mappings, strict, threshold, loader)
}

// Keep track of the current factory (can be replaced in tests)
var currentGeneratorFactory = defaultGeneratorFactory

// --- End Generator Factory ---

// newRootCmd creates the base command when called without any subcommands
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "irr",
		Short: "Tool for generating Helm overrides to redirect container images",
		Long: `IRR (Image Registry Rewrite) helps migrate container images between registries by generating
Helm value overrides that redirect image references to a new registry.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Initialize debug logging early based on the persistent flag
			// Find the debug flag value - check if it exists and is true
			if debugFlag := cmd.Flags().Lookup("debug"); debugFlag != nil {
				if debugEnabledVal, err := cmd.Flags().GetBool("debug"); err == nil && debugEnabledVal {
					debug.Init(true)
				}
			}
		},
		// Disable automatic printing of usage on error
		SilenceUsage: true,
		// Disable automatic printing of errors
		SilenceErrors: true,
	}

	// Add persistent flags available to all commands
	rootCmd.PersistentFlags().BoolVar(&debugEnabled, "debug", false, "Enable debug logging")

	// Add subcommands
	rootCmd.AddCommand(newDefaultCmd()) // Renamed to overrideCmd?
	rootCmd.AddCommand(newAnalyzeCmd())

	return rootCmd
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		// Check if the error is an ExitCodeError and use its code
		var exitCodeErr *ExitCodeError
		exitCode := ExitGeneralRuntimeError // Default exit code
		if errors.As(err, &exitCodeErr) {
			exitCode = exitCodeErr.Code
		}
		fmt.Fprintln(os.Stderr, err) // Print the error message regardless
		os.Exit(exitCode)
	}
}

// --- Default (Override) Command --- Moved from original main.go

func newDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "override [flags]",
		Short: "Generate Helm overrides for redirecting container images (default action)",
		Long: `Generate Helm value overrides that redirect container images from source registries
to a target registry. This is the original functionality of IRR. If no subcommand is specified, this command runs by default.`,
		RunE: runDefault,
		// Restore default silencing for production behavior
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Add flags specific to the override command
	f := cmd.Flags()
	f.StringVar(&chartPath, "chart-path", "", "Path to the Helm chart (directory or .tgz archive)")
	f.StringVar(&targetRegistry, "target-registry", "", "Target registry URL (e.g., harbor.example.com:5000)")
	f.StringVar(&sourceRegistries, "source-registries", "", "Comma-separated list of source registries to rewrite (e.g., docker.io,quay.io)")
	f.StringVar(&outputFile, "output-file", "", "Output file path for overrides (default: stdout)")
	f.StringVar(&pathStrategy, "path-strategy", "prefix-source-registry", "Path strategy to use (currently only prefix-source-registry is supported)")
	f.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	f.BoolVar(&dryRun, "dry-run", false, "Preview changes without writing file")
	f.BoolVar(&strictMode, "strict", false, "Fail on unrecognized image structures")
	f.StringVar(&excludeRegistries, "exclude-registries", "", "Comma-separated list of registries to exclude from processing")
	f.IntVar(&threshold, "threshold", 100, "Success threshold percentage (0-100)")
	f.StringVar(&registryMappingsFile, "registry-mappings", "", "Path to YAML file containing registry mappings")

	// Mark required flags for override command
	if err := cmd.MarkFlagRequired("chart-path"); err != nil {
		// This should never happen, but log it in case it does
		fmt.Fprintf(os.Stderr, "Error marking chart-path as required: %v\n", err)
	}
	if err := cmd.MarkFlagRequired("target-registry"); err != nil {
		fmt.Fprintf(os.Stderr, "Error marking target-registry as required: %v\n", err)
	}
	if err := cmd.MarkFlagRequired("source-registries"); err != nil {
		fmt.Fprintf(os.Stderr, "Error marking source-registries as required: %v\n", err)
	}

	return cmd
}

func runDefault(cmd *cobra.Command, args []string) error {
	debug.FunctionEnter("runDefault (overrideCmd)")
	defer debug.FunctionExit("runDefault (overrideCmd)")

	// Parse flags, load mappings, get strategy...
	// ... (parsing logic remains the same) ...
	sourceRegistriesList := strings.Split(sourceRegistries, ",")
	var excludeRegistriesList []string
	if excludeRegistries != "" {
		excludeRegistriesList = strings.Split(excludeRegistries, ",")
	}

	debug.DumpValue("Source Registries", sourceRegistriesList)
	debug.DumpValue("Exclude Registries", excludeRegistriesList)

	if pathStrategy != "prefix-source-registry" {
		return fmt.Errorf("unsupported path strategy: %s", pathStrategy)
	}

	var registryMappings *registry.RegistryMappings
	if registryMappingsFile != "" {
		var mapErr error
		// TODO: Mock registry.LoadMappings in tests
		registryMappings, mapErr = registry.LoadMappings(registryMappingsFile)
		if mapErr != nil {
			return wrapExitCodeError(ExitInputConfigurationError, "error loading registry mappings", mapErr)
		}
		if verbose {
			fmt.Printf("Loaded registry mappings from: %s\n", registryMappingsFile)
		}
	} else if verbose {
		fmt.Println("No registry mappings file provided.")
	}

	// TODO: Mock strategy.GetStrategy in tests
	strategyImpl, strategyErr := strategy.GetStrategy(pathStrategy, registryMappings)
	if strategyErr != nil {
		return wrapExitCodeError(ExitCodeInvalidStrategy, "error getting path strategy", strategyErr)
	}

	// Create the generator using the factory
	loader := chart.NewLoader() // Still need a loader instance for the factory
	generator := currentGeneratorFactory(
		chartPath,
		targetRegistry,
		sourceRegistriesList,
		excludeRegistriesList,
		strategyImpl,
		registryMappings,
		strictMode,
		threshold,
		loader,
	)

	// Generate overrides using the interface
	overrideFile, err := generator.Generate()
	if err != nil {
		exitCode := ExitImageProcessingError
		return wrapExitCodeError(exitCode, "error generating overrides", err)
	}

	// TODO: Re-implement threshold check if needed.

	// Generate YAML output
	yamlOutput, err := overrideFile.ToYAML()
	if err != nil {
		return wrapExitCodeError(ExitGeneralRuntimeError, "error generating YAML output", err)
	}

	// Restore Output results logic
	if dryRun {
		debug.Println("Dry run enabled")
		// Explicitly ignore both return values for stdout write
		n, err := fmt.Fprintln(cmd.OutOrStdout(), "Dry run enabled, printing overrides to stdout:")
		_ = n   // Ignore bytes written
		_ = err // Ignore error

		_, err = cmd.OutOrStdout().Write(yamlOutput) // Keep this error check as it involves the main output
		if err != nil {
			return fmt.Errorf("writing dry run output to stdout: %w", err)
		}
	} else if outputFile != "" {
		// Use afero WriteFile
		err = afero.WriteFile(AppFs, outputFile, yamlOutput, 0644)
		if err == nil && verbose {
			debug.Printf("Overrides written to: %s", outputFile)
			// Explicitly ignore both return values for stdout write
			n, err := fmt.Fprintf(cmd.OutOrStdout(), "Overrides written to: %s\n", outputFile)
			_ = n   // Ignore bytes written
			_ = err // Ignore error
		}
	} else {
		debug.Println("Writing overrides to stdout")
		_, err = cmd.OutOrStdout().Write(yamlOutput)
		if err != nil {
			return fmt.Errorf("writing overrides to stdout: %w", err)
		}
		// Explicitly ignore both return values for writing the final newline to stdout
		n, err := cmd.OutOrStdout().Write([]byte("\n")) // Ignore error here
		_ = n                                           // Ignore bytes written
		_ = err                                         // Ignore error
	}
	return nil
}

// --- Analyze Command --- Moved from analyze.go

func newAnalyzeCmd() *cobra.Command {
	// Define flags specific to analyze command *within* this function scope
	var outputFormat string
	var analyzeOutputFile string // Use a different var name to avoid collision

	cmd := &cobra.Command{
		Use:   "analyze [chart-path]",
		Short: "Analyze a Helm chart for image patterns",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chartPath := args[0]

			debug.FunctionEnter("runAnalyze")
			defer debug.FunctionExit("runAnalyze")

			// Use the factory to create the analyzer
			analyzer := currentAnalyzerFactory(chartPath)

			// Perform analysis
			debug.Printf("Analyzing chart: %s", chartPath)
			result, err := analyzer.Analyze() // Use the interface method
			if err != nil {
				return wrapExitCodeError(ExitChartParsingError, "analysis failed", err)
			}
			debug.DumpValue("Analysis Result", result)

			// Format output
			var output string
			if outputFormat == "json" {
				jsonBytes, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return wrapExitCodeError(ExitGeneralRuntimeError, "failed to marshal JSON", err)
				}
				output = string(jsonBytes)
			} else {
				output = formatTextOutput(result) // formatTextOutput needs to be in this file now
			}

			// Write output
			if analyzeOutputFile != "" {
				debug.Printf("Writing analysis output to file: %s", analyzeOutputFile)
				// Use afero WriteFile
				if err := afero.WriteFile(AppFs, analyzeOutputFile, []byte(output), 0644); err != nil {
					return wrapExitCodeError(ExitGeneralRuntimeError, "failed to write analysis output", err)
				}
			} else {
				debug.Println("Writing analysis output to stdout")
				_, err = cmd.OutOrStdout().Write([]byte(output))
				if err != nil {
					return fmt.Errorf("writing analysis output: %w", err)
				}
				// Explicitly ignore both return values for writing the final newline to stdout
				n, err := cmd.OutOrStdout().Write([]byte("\n")) // Ignore error here
				_ = n                                           // Ignore bytes written
				_ = err                                         // Ignore error
			}

			return nil
		},
	}

	// Add flags specific to the analyze command
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text or json)")
	cmd.Flags().StringVarP(&analyzeOutputFile, "file", "f", "", "Output file (defaults to stdout)")
	// Note: Using different variable `analyzeOutputFile` for the flag binding

	return cmd
}

// formatTextOutput needs to be moved here from analyze.go
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
