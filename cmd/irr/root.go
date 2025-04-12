// Package main implements the command-line interface for the irr (Image Relocation and Rewrite) tool.
// It provides commands for analyzing Helm charts and generating override values to redirect
// container image references from public registries to a target private registry.
//
// The main CLI commands are:
//   - analyze: Analyze a Helm chart to identify image references
//   - override: Generate override values to redirect images to a target registry
//
// Each command has various flags for configuration. See the help output for details.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	log "github.com/lalbers/irr/pkg/log"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// Global flag variables
var (
	cfgFile          string
	sourceRegistries []string
	outputFile       string
	debugEnabled     bool
	logLevel         string
	// analyze command flags
	outputFormat string
	// For analyze command
	// includePatterns []string
	// excludePatterns []string
	// knownPaths      []string

	// Output and mode flags
	registryFile string

	// Behavior flags
	verbose    bool
	strictMode bool

	// IntegrationTestMode controls behavior specific to integration tests
	integrationTestMode bool

	// Configuration variables (populated by flags or config file)
	// These seem unused according to the linter, removing them for now.
	// includePatterns []string
	// excludePatterns []string
	// knownPaths      []string
	// targetRegistry string
	// excludeRegistries []string
	// pathStrategy  string
	// printPatterns bool
	// templateMode  bool

	// TestAnalyzeMode is a global flag to enable test mode for analyze command
	TestAnalyzeMode bool
)

// Helper to panic on required flag errors (indicates programmer error)
func mustMarkFlagRequired(cmd *cobra.Command, flagName string) {
	if err := cmd.MarkFlagRequired(flagName); err != nil {
		panic(fmt.Sprintf("failed to mark flag '%s' as required: %v", flagName, err))
	}
}

const (
	// Standard file permissions
	defaultFilePerm       fs.FileMode = 0o600 // Read/write for owner
	defaultOutputFilePerm fs.FileMode = 0o644 // Read/write for owner, read for group/others
)

// AppFs defines the filesystem interface to use, allows mocking in tests.
var AppFs = afero.NewOsFs()

// ExitCodeError wraps an error with an exit code
type ExitCodeError struct {
	err      error
	exitCode int
}

func (e *ExitCodeError) Error() string {
	return e.err.Error()
}

// ExitCode returns the exit code stored in the error
func (e *ExitCodeError) ExitCode() int {
	return e.exitCode
}

func wrapExitCodeError(err error, code int) error {
	if err == nil {
		return nil
	}
	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	return &ExitCodeError{err: err, exitCode: code}
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

// AnalyzerInterface defines the methods expected from an analyzer.
// This interface is used to allow mocking in tests and to provide a clean
// abstraction between the CLI and the analysis package.
type AnalyzerInterface interface {
	// Analyze performs chart analysis and returns the detected image patterns
	// or an error if the analysis fails.
	Analyze() (*analysis.ChartAnalysis, error)
}

// --- End Factory ---

// --- Factory for Generator ---

// GeneratorInterface defines the methods expected from a generator.
// This interface is used to allow mocking in tests and to provide a clean
// abstraction between the CLI and the chart processing logic.
type GeneratorInterface interface {
	// Generate performs image reference override generation and returns
	// the override file structure or an error if generation fails.
	Generate() (*override.File, error)
}

// Allows overriding for testing
type generatorFactoryFunc func(
	chartPath, targetRegistry string,
	sourceRegistries, excludeRegistries []string,
	pathStrategy strategy.PathStrategy,
	mappings *registry.Mappings,
	configMappings map[string]string,
	strict bool,
	threshold int,
	loader analysis.ChartLoader,
	includePatterns, excludePatterns, knownPaths []string,
) GeneratorInterface

// Default factory creates the real generator
var defaultGeneratorFactory generatorFactoryFunc = func(
	chartPath, targetRegistry string,
	sourceRegistries, excludeRegistries []string,
	pathStrategy strategy.PathStrategy,
	mappings *registry.Mappings,
	configMappings map[string]string,
	strict bool,
	threshold int,
	loader analysis.ChartLoader,
	includePatterns, excludePatterns, knownPaths []string,
) GeneratorInterface {
	return chart.NewGenerator(
		chartPath, targetRegistry,
		sourceRegistries, excludeRegistries,
		pathStrategy, mappings, configMappings, strict, threshold, loader,
		includePatterns, excludePatterns, knownPaths,
	)
}

// Keep track of the current factory (can be replaced in tests)
var currentGeneratorFactory = defaultGeneratorFactory

// Regular expression for validating registry names (simplified based on common usage)
// var registryRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9](:\\d+)?$`)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "irr",
	Short: "Image Registry Redirect - Helm chart image registry override tool",
	Long: `irr (Image Relocation and Rewrite) is a tool for generating Helm override values
that redirect container image references from public registries to a private registry.

It can analyze Helm charts to identify image references and generate override values 
files compatible with Helm, pointing images to a new registry according to specified strategies.
It also supports linting image references for potential issues.`,
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		// Setup logging before any command logic runs
		logLevelStr := logLevel      // Use the global variable
		debugEnabled := debugEnabled // Use the global variable

		level := log.LevelInfo // Default level
		if debugEnabled {
			level = log.LevelDebug
		} else if logLevelStr != "" { // Only check --log-level if --debug is not set
			parsedLevel, err := log.ParseLevel(logLevelStr)
			if err != nil {
				log.Warnf("Invalid log level specified: '%s'. Using default: %s. Error: %v", logLevelStr, level, err)
			} else {
				level = parsedLevel
			}
		}

		log.SetLevel(level)

		// Set debug.Enabled based on --debug flag OR IRR_DEBUG env var
		// Prioritize the command-line flag if set to true.
		if debugEnabled { // Check the flag first
			debug.Enabled = true
			log.SetLevel(log.LevelDebug)                        // Ensure log level is also debug
			debug.Printf("--debug flag enabled debug logging.") // Use debug.Printf
		} else { // If flag is not set, check the environment variable
			debugEnv := os.Getenv("IRR_DEBUG")
			// Only attempt to parse if the environment variable is actually set
			if debugEnv != "" {
				debugVal, err := strconv.ParseBool(debugEnv)
				if err != nil {
					// Only log the warning if in test mode or if debug is already enabled
					if integrationTestMode {
						log.Warnf("Invalid boolean value for IRR_DEBUG environment variable: '%s'. Defaulting to false.", debugEnv)
					}
					debug.Enabled = false
				} else {
					debug.Enabled = debugVal
					if debugVal { // If IRR_DEBUG=true, ensure log level is also debug
						log.SetLevel(log.LevelDebug)
						debug.Printf("IRR_DEBUG environment variable enabled debug logging.") // Use debug.Printf
					}
				}
			} else {
				// Default to false if neither flag nor env var is set and non-empty
				debug.Enabled = false
			}
		}

		log.Infof("Log level set to %s", level)                  // Use log.Infof for informational messages
		debug.Printf("Debug package enabled: %t", debug.Enabled) // This should confirm if it's set

		// Integration test mode check
		if integrationTestMode {
			log.Warnf("Integration test mode enabled.")
			// Perform actions specific to integration test mode if needed
		}

		if registryFile != "" {
			// Only reset the filesystem if it's not already an in-memory filesystem
			// This preserves the filesystem set up by tests
			_, isMemMapFs := AppFs.(*afero.MemMapFs)
			if !isMemMapFs {
				AppFs = afero.NewOsFs() // Ensure filesystem is initialized only if not in a test with MemMapFs
				debug.Printf("Using OS filesystem for registry mappings")
			} else {
				debug.Printf("Preserving in-memory filesystem for testing")
			}
			debug.Printf("Root command: Attempting to load mappings from %s", registryFile)
			// Only load to check for errors, don't need the result here.
			// Skip CWD restriction when in integration test mode
			_, err := registry.LoadMappings(AppFs, registryFile, integrationTestMode) // Pass integrationTestMode for skipCWDRestriction
			if err != nil {
				debug.Printf("Root command: Failed to load mappings: %v", err)
				// Use debug.Printf for logging warnings as well, assuming it handles levels
				debug.Printf("Warning: Failed to load registry mappings from %s: %v. Proceeding without mappings.", registryFile, err)
			}
		}

		return nil // PersistentPreRunE should return error
	},
	RunE: func(_ *cobra.Command, args []string) error {
		// If no arguments (subcommand) are provided, return an error.
		if len(args) == 0 {
			// Use Errorf for consistency
			log.Errorf("Error: a subcommand is required. Use 'irr --help' for available commands.")
			return errors.New("a subcommand is required")
		}
		// Otherwise, let Cobra handle the subcommand or help text.
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return fmt.Errorf("execute command: %w", err)
	}
	return nil
}

// init sets up the root command and its flags.
func init() {
	cobra.OnInitialize()

	// Add global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default is $HOME/.irr.yaml)")
	rootCmd.PersistentFlags().StringSliceVarP(&sourceRegistries, "source-registries", "s", []string{}, "Source container registry URLs (comma-separated or multiple flags)")
	rootCmd.PersistentFlags().StringVarP(&outputFile, "output-file", "o", "", "Output file path (default: stdout)")
	rootCmd.PersistentFlags().BoolVarP(&debugEnabled, "debug", "d", false, "Enable debug output")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Set log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&registryFile, "registry-file", "", "Path to registry mappings file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&strictMode, "strict", false, "Enable strict validation")
	rootCmd.PersistentFlags().BoolVar(&integrationTestMode, "integration-test", false, "Enable integration test mode")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output-format", "yaml", "Output format (yaml, json)")

	// Add commands to root
	rootCmd.AddCommand(newInspectCmd())
	rootCmd.AddCommand(newOverrideCmd())
	rootCmd.AddCommand(newValidateCmd())

	// Hidden testing flags
	rootCmd.PersistentFlags().BoolVar(&TestAnalyzeMode, "test-analyze", false, "Enable test mode for analyze command")
	if err := rootCmd.PersistentFlags().MarkHidden("test-analyze"); err != nil {
		// Non-fatal error, just log
		fmt.Printf("Warning: Failed to mark test-analyze flag as hidden: %v\n", err)
	}
	if err := rootCmd.PersistentFlags().MarkHidden("integration-test"); err != nil {
		// Non-fatal error, just log
		fmt.Printf("Warning: Failed to mark integration-test flag as hidden: %v\n", err)
	}
}

// --- Analyze Command --- Moved from analyze.go

func newAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze [flags] CHART",
		Short: "Analyze a Helm chart for image references",
		Long: `Analyze a Helm chart to identify container image references.
		
This command will scan a Helm chart for container image references and report them.
It can output in text or JSON format and supports filtering by source registry.`,
		Args: func(cmd *cobra.Command, args []string) error {
			// Skip argument validation in test mode
			testAnalyze, err := cmd.Flags().GetBool("test-analyze")
			if err != nil {
				return fmt.Errorf("failed to get test-analyze flag: %w", err)
			}
			if testAnalyze {
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: runAnalyze,
	}

	// Add flags
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text or json)")
	cmd.Flags().StringVarP(&outputFile, "output-file", "f", "", "File to write output to (defaults to stdout)")
	cmd.Flags().StringVarP(&registryFile, "mappings", "m", "", "Registry mappings file")
	cmd.Flags().BoolVarP(&strictMode, "strict", "s", false, "Enable strict mode")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().StringSliceVarP(&sourceRegistries, "source-registries", "r", nil, "Source registries to analyze")
	cmd.Flags().StringP("chart-path", "c", "", "Path to the Helm chart directory or tarball (required)")

	// Mark required flags
	mustMarkFlagRequired(cmd, "source-registries")
	mustMarkFlagRequired(cmd, "chart-path")

	return cmd
}

// formatJSONOutput formats the analysis result as JSON
func formatJSONOutput(result *analysis.ChartAnalysis) (string, error) {
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", wrapExitCodeError(err, exitcodes.ExitGeneralRuntimeError)
	}
	return string(jsonBytes) + "\n", nil // Add newline for JSON output
}

// formatTextOutput formats the analysis result as human-readable text
func formatTextOutput(result *analysis.ChartAnalysis) string {
	var sb strings.Builder
	sb.WriteString("Chart Analysis\n\n")
	sb.WriteString(fmt.Sprintf("Total image patterns found: %d\n", len(result.ImagePatterns)))
	sb.WriteString(fmt.Sprintf("Total global patterns found: %d\n\n", len(result.GlobalPatterns)))

	if len(result.ImagePatterns) > 0 {
		sb.WriteString("Detected Image Patterns:\n")
		for _, pattern := range result.ImagePatterns {
			sb.WriteString(fmt.Sprintf("  - Path: %s\n", pattern.Path))
			sb.WriteString(fmt.Sprintf("    Type: %s\n", pattern.Type))
			// Use %+v for potentially complex values like maps
			formattedValue := fmt.Sprintf("%+v", pattern.Value)
			sb.WriteString(fmt.Sprintf("    Value: %s\n", formattedValue))
		}
		sb.WriteString("\n")
	}

	if len(result.GlobalPatterns) > 0 {
		sb.WriteString("Detected Global Patterns:\n")
		for _, pattern := range result.GlobalPatterns {
			sb.WriteString(fmt.Sprintf("  - Path: %s\n", pattern.Path))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// writeAnalysisOutput writes the analysis output to a file or stdout
func writeAnalysisOutput(cmd *cobra.Command, output, outputFile string) error {
	if outputFile != "" {
		debug.Printf("Writing analysis output to file: %s", outputFile)
		if err := afero.WriteFile(AppFs, outputFile, []byte(output), defaultOutputFilePerm); err != nil {
			return wrapExitCodeError(err, exitcodes.ExitGeneralRuntimeError)
		}
		return nil
	}

	debug.Println("Writing analysis output to stdout")
	_, err := cmd.OutOrStdout().Write([]byte(output))
	if err != nil {
		return fmt.Errorf("writing analysis output: %w", err)
	}
	return nil
}

// runAnalyze implements the analyze command functionality
func runAnalyze(cmd *cobra.Command, args []string) error {
	// Check if we're in test mode
	testAnalyze, err := cmd.Flags().GetBool("test-analyze")
	if err != nil {
		return fmt.Errorf("failed to get test-analyze flag: %w", err)
	}
	debug.Printf("Running analyze command in test mode: %v", testAnalyze)

	// Check required args in normal mode
	if !testAnalyze && len(args) != 1 {
		return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
	}

	// Check required flags in normal mode
	if !testAnalyze {
		sourceRegistries, err := cmd.Flags().GetStringSlice("source-registries")
		if err != nil || len(sourceRegistries) == 0 {
			return fmt.Errorf("required flag(s) \"source-registries\" not set")
		}

		chartPathFlag, err := cmd.Flags().GetString("chart-path")
		if err != nil || chartPathFlag == "" {
			return fmt.Errorf("required flag(s) \"chart-path\" not set")
		}
	}

	// Set up the analyzer
	var chartPath string
	switch {
	case len(args) > 0:
		chartPath = args[0]
	case testAnalyze:
		// Use a placeholder in test mode if no args provided
		chartPath = "test-chart"
	default:
		return fmt.Errorf("chart path argument required")
	}

	analyzer := currentAnalyzerFactory(chartPath)

	// Get output format (text or json)
	outputFormat, err := cmd.Flags().GetString("output")
	if err != nil {
		return fmt.Errorf("failed to get output flag: %w", err)
	}
	outputFile, err := cmd.Flags().GetString("output-file")
	if err != nil {
		return fmt.Errorf("failed to get output-file flag: %w", err)
	}

	// Perform analysis
	result, err := analyzer.Analyze()
	if err != nil {
		return wrapExitCodeError(err, exitcodes.ExitChartParsingError)
	}

	// Format output based on format flag
	var output string
	if outputFormat == "json" {
		output, err = formatJSONOutput(result)
		if err != nil {
			return err
		}
	} else {
		output = formatTextOutput(result)
	}

	// Write output to file or stdout
	return writeAnalysisOutput(cmd, output, outputFile)
}

// initConfig reads in config file and ENV variables if set.
// NOTE: We are not currently using a config file or environment variables beyond LOG_LEVEL/IRR_DEBUG (handled in packages).
// func initConfig() {
// ... implementation ...
// }

// Override command implementation moved to override.go

// Get the root command - useful for testing
func getRootCmd() *cobra.Command {
	return rootCmd
}

// executeCommand is a helper for testing Cobra commands
func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)

	// Ensure test-analyze flag is set when TestAnalyzeMode is true
	if TestAnalyzeMode {
		hasTestFlag := false
		for _, arg := range args {
			if arg == "--test-analyze" {
				hasTestFlag = true
				break
			}
		}
		if !hasTestFlag {
			args = append(args, "--test-analyze")
		}
	}

	root.SetArgs(args)
	err = root.Execute()

	return buf.String(), err
}

/* // Unused
// Function to initialize file system (moved from root execution)
func initFS() afero.Fs {
	// Example: Initialize based on a flag or environment variable if needed
	return afero.NewOsFs()
}
*/

/* // Unused
// Function to load mappings (consider moving to a shared location or helper)
func loadMappingsIfNeeded(fs afero.Fs, registryFile string) (*registry.Mappings, error) {
	if registryFile == "" {
		return nil, nil
	}
	// Pass false for skipCWDRestriction in normal execution path
	return registry.LoadMappings(fs, registryFile, false)
}
*/
