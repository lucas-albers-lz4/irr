package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

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
	// Chart and registry related
	chartPath         string
	targetRegistry    string
	sourceRegistries  []string
	excludeRegistries []string
	registryFile      string // Registry mappings file
	imageRegistry     string
	globalRegistry    string

	// Output and mode flags
	outputFile   string // Used by multiple commands
	outputFormat string // Output format (text or json)
	pathStrategy string
	pathDepth    int

	// Behavior flags
	verbose      bool
	dryRun       bool
	strictMode   bool
	debugEnabled bool // Used by multiple commands
	templateMode bool
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

	// Tabwriter settings
	minWidth = 0
	tabWidth = 4
	padding  = 1
	padChar  = ' '
	flags    = 0
)

// AppFs provides an abstraction over the filesystem.
// Defaults to the OS filesystem, can be replaced with a memory map for tests.
var AppFs afero.Fs = afero.NewOsFs()

// ExitCodeError wraps an error with an exit code
type ExitCodeError struct {
	err      error
	exitCode int
}

func (e *ExitCodeError) Error() string {
	return e.err.Error()
}

func (e *ExitCodeError) ExitCode() int {
	return e.exitCode
}

func wrapExitCodeError(err error, code int) error {
	if err == nil {
		return nil
	}
	if exitErr, ok := err.(*ExitCodeError); ok {
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

// AnalyzerInterface mirrors the analysis.Analyzer interface for mocking.
// It defines the Analyze method expected by the command.
// AnalyzerInterface defines the methods expected from an analyzer.
type AnalyzerInterface interface {
	Analyze() (*analysis.ChartAnalysis, error)
}

// --- End Factory ---

// --- Factory for Generator ---

// GeneratorInterface mirrors the chart.Generator interface for mocking.
// It defines the Generate method expected by the command.
type GeneratorInterface interface {
	Generate() (*override.File, error)
}

// Allows overriding for testing
type generatorFactoryFunc func(
	chartPath, targetRegistry string,
	sourceRegistries, excludeRegistries []string,
	pathStrategy strategy.PathStrategy,
	mappings *registry.Mappings,
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
	strict bool,
	threshold int,
	loader analysis.ChartLoader,
	includePatterns, excludePatterns, knownPaths []string,
) GeneratorInterface {
	return chart.NewGenerator(
		chartPath, targetRegistry,
		sourceRegistries, excludeRegistries,
		pathStrategy, mappings, strict, threshold, loader,
		includePatterns, excludePatterns, knownPaths,
	)
}

// Keep track of the current factory (can be replaced in tests)
var currentGeneratorFactory = defaultGeneratorFactory

// Regular expression for validating registry names (simplified based on common usage)
var registryRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9](:\d+)?$`)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "irr",
	Short: "Image Registry Redirect - Helm chart image registry override tool",
	Long: `irr (Image Relocation and Rewrite) is a tool for generating Helm override values
that redirect container image references from public registries to a private registry.

It can analyze Helm charts to identify image references and generate override values 
files compatible with Helm, pointing images to a new registry according to specified strategies.
It also supports linting image references for potential issues.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Setup logging before any command logic runs
		logLevelStr, err := cmd.Flags().GetString("log-level")
		if err != nil {
			log.Errorf("Error getting log-level flag: %v", err) // Log error
		}
		debugEnabled, err := cmd.Flags().GetBool("debug") // Use debugEnabled consistently
		if err != nil {
			log.Errorf("Error getting debug flag: %v", err) // Log error
		}

		level := log.LevelInfo // Default level is now defined in pkg/log
		if debugEnabled {
			level = log.LevelDebug
		} else if logLevelStr != "" { // Only check --log-level if --debug is not set
			parsedLevel, err := log.ParseLevel(logLevelStr)
			if err != nil {
				// Use the package's Warnf directly
				log.Warnf("Invalid log level specified: '%s'. Using default: %s. Error: %v", logLevelStr, level, err)
			} else {
				level = parsedLevel
			}
		}

		log.SetLevel(level)

		// Final check to confirm debug status and log timestamp
		if log.IsDebugEnabled() {
			log.Debugf("Debug logging is enabled (Timestamp: %s)", time.Now().Format(time.RFC3339))
		}

		// Propagate the debug flag state to the debug package
		debug.Enabled = debugEnabled
	},
	// Disable automatic printing of usage on error
	SilenceUsage: true,
	// Disable automatic printing of errors
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no arguments (subcommand) are provided, return an error.
		if len(args) == 0 {
			return errors.New("a subcommand is required. Use 'irr help' for available commands")
		}
		// Otherwise, let Cobra handle the subcommand or help text.
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		if exitErr, ok := err.(*ExitCodeError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(exitcodes.ExitGeneralRuntimeError)
	}
}

// Flag to indicate integration test mode (hidden)
var integrationTestMode bool

func init() {
	// Define persistent flags available to all commands
	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logging (overrides log-level to debug)")
	rootCmd.PersistentFlags().BoolVar(&integrationTestMode, "integration-test-mode", false, "Enable integration test mode (internal use)")
	if err := rootCmd.PersistentFlags().MarkHidden("integration-test-mode"); err != nil {
		log.Warnf("Failed to mark integration-test-mode flag as hidden: %v", err)
	}

	// Add subcommands
	rootCmd.AddCommand(newAnalyzeCmd())
	rootCmd.AddCommand(newOverrideCmd()) // Re-enable the override command
	// rootCmd.AddCommand(newLintCmd())
	// rootCmd.AddCommand(newVersionCmd())

	// REMOVED Redundant Persistent Flags - These are defined locally in subcommands now.
	// rootCmd.PersistentFlags().StringVarP(&chartPath, "chart-path", "p", "", "Path to the Helm chart directory or archive")
	// rootCmd.PersistentFlags().StringVarP(&targetRegistry, "target-registry", "t", "", "Target container registry URL")
	// rootCmd.PersistentFlags().StringSliceVarP(&sourceRegistries, "source-registries", "s", []string{}, "Source container registry URLs")
	// rootCmd.PersistentFlags().StringSliceVarP(&excludeRegistries, "exclude-registries", "e", []string{}, "Source registries to exclude")
	// rootCmd.PersistentFlags().BoolVar(&strictMode, "strict", false, "Enable strict mode")
	// rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	// rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview changes instead of writing file")
	// rootCmd.PersistentFlags().IntVar(&pathDepth, "path-depth", DefaultPathDepth, "Maximum recursion depth for values traversal")
	// rootCmd.PersistentFlags().StringVar(&imageRegistry, "image-registry", "",
	// 	"Global image registry override (DEPRECATED, use global-registry)")
	// rootCmd.PersistentFlags().StringVar(&globalRegistry, "global-registry", "", "Global image registry override")
	// rootCmd.PersistentFlags().StringVar(&registryFile, "registry-file", "", "Path to YAML file containing registry mappings")
	// rootCmd.PersistentFlags().StringVarP(&outputFile, "output-file", "o", "", "Path to the output override file")
	// rootCmd.PersistentFlags().StringVar(&pathStrategy, "path-strategy", "prefix-source-registry", "Path generation strategy")
	// rootCmd.PersistentFlags().BoolVar(&templateMode, "template-mode", true, "Enable template variable detection")

	// Add analyze command
	analyzeCmd := &cobra.Command{
		Use:   "analyze [flags] CHART",
		Short: "Analyze a Helm chart for image references",
		Long: `Analyze a Helm chart to identify container image references.
		
This command will scan a Helm chart for container image references and report them.
It can output in text or JSON format and supports filtering by source registry.`,
		Args: cobra.ExactArgs(1),
		RunE: runAnalyze,
	}

	// Add flags
	analyzeCmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text or json)")
	analyzeCmd.Flags().StringVarP(&outputFile, "output-file", "f", "", "File to write output to (defaults to stdout)")
	analyzeCmd.Flags().StringVarP(&registryFile, "mappings", "m", "", "Registry mappings file")
	analyzeCmd.Flags().BoolVarP(&strictMode, "strict", "s", false, "Enable strict mode")
	analyzeCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	analyzeCmd.Flags().StringSliceVarP(&sourceRegistries, "source-registries", "r", nil, "Source registries to analyze")

	// Mark required flags
	mustMarkFlagRequired(analyzeCmd, "source-registries")

	rootCmd.AddCommand(analyzeCmd)

	// Add analyze command flags using the global variables
	analyzeCmd.Flags().StringVarP(&chartPath, "chart", "c", "", "Path to the chart directory or archive")
	analyzeCmd.Flags().StringSliceVarP(&sourceRegistries, "source-registries", "s", []string{}, "Source registries to analyze")
	analyzeCmd.Flags().StringSliceVarP(&excludeRegistries, "exclude-registries", "e", []string{}, "Registries to exclude from analysis")
	analyzeCmd.Flags().StringVarP(&registryFile, "mappings", "m", "", "Path to registry mappings file")
	analyzeCmd.Flags().BoolVarP(&strictMode, "strict", "", false, "Enable strict validation mode")
	analyzeCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	analyzeCmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text or json)")
	analyzeCmd.Flags().StringVarP(&outputFile, "output-file", "f", "", "File to write output to")

	// Mark required flags
	_ = analyzeCmd.MarkFlagRequired("chart")
}

// --- Default (Override) Command --- Moved from original main.go

func newDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "irr",
		Short: "Image Registry Redirect - Helm chart image registry override tool",
		Long: `Image Registry Redirect (irr) is a tool for generating Helm override values
that redirect container image references from public registries to a private registry.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("a subcommand is required")
		},
	}
	return cmd
}

// --- Analyze Command --- Moved from analyze.go

func newAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze [flags] CHART",
		Short: "Analyze a Helm chart for image references",
		Long: `Analyze a Helm chart to identify container image references.
		
This command will scan a Helm chart for container image references and report them.
It can output in text or JSON format and supports filtering by source registry.`,
		Args: cobra.ExactArgs(1),
		RunE: runAnalyze,
	}

	// Add flags
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text or json)")
	cmd.Flags().StringVarP(&outputFile, "output-file", "f", "", "File to write output to (defaults to stdout)")
	cmd.Flags().StringVarP(&registryFile, "mappings", "m", "", "Registry mappings file")
	cmd.Flags().BoolVarP(&strictMode, "strict", "s", false, "Enable strict mode")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().StringSliceVarP(&sourceRegistries, "source-registries", "r", nil, "Source registries to analyze")

	// Mark required flags
	mustMarkFlagRequired(cmd, "source-registries")

	return cmd
}

// runAnalyze implements the analyze command functionality
func runAnalyze(cmd *cobra.Command, args []string) error {
	chartPath := args[0]

	debug.FunctionEnter("runAnalyze")
	defer debug.FunctionExit("runAnalyze")

	// Basic logging setup (using global var)
	if verbose {
		log.SetLevel(log.LevelDebug)
		log.Debugf("Verbose logging enabled")
	}

	// Load Mappings (using global var and passing AppFs)
	mappings, err := registry.LoadMappings(AppFs, registryFile)
	if err != nil {
		return fmt.Errorf("error loading registry mappings: %w", err)
	}
	if mappings != nil {
		log.Debugf("Loaded %d registry mappings", len(mappings.Entries))
	}

	// Use the factory to create the analyzer
	analyzer := currentAnalyzerFactory(chartPath)

	// Perform analysis
	debug.Printf("Analyzing chart: %s", chartPath)
	result, err := analyzer.Analyze() // Use the interface method
	if err != nil {
		return wrapExitCodeError(err, exitcodes.ExitChartParsingError)
	}
	debug.DumpValue("Analysis Result", result)

	// Format output
	var output string
	if outputFormat == "json" {
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return wrapExitCodeError(err, exitcodes.ExitGeneralRuntimeError)
		}
		output = string(jsonBytes) + "\n" // Add newline for JSON output
	} else {
		// Text output format matching test expectations
		var sb strings.Builder
		sb.WriteString("Chart Analysis\n\n")
		sb.WriteString(fmt.Sprintf("Total image patterns: %d\n\n", len(result.ImagePatterns)))

		for _, pattern := range result.ImagePatterns {
			sb.WriteString(fmt.Sprintf("Path: %s\n", pattern.Path))
			sb.WriteString(fmt.Sprintf("Type: %s\n", pattern.Type))
			sb.WriteString(fmt.Sprintf("Value: %s\n", pattern.Value))
			sb.WriteString("\n")
		}

		if len(result.GlobalPatterns) > 0 {
			sb.WriteString(fmt.Sprintf("\nGlobal patterns: %d\n", len(result.GlobalPatterns)))
			for _, pattern := range result.GlobalPatterns {
				sb.WriteString(fmt.Sprintf("  Path: %s\n", pattern.Path))
			}
		}
		output = sb.String()
	}

	// Write output
	if outputFile != "" {
		debug.Printf("Writing analysis output to file: %s", outputFile)
		if err := afero.WriteFile(AppFs, outputFile, []byte(output), defaultOutputFilePerm); err != nil {
			return wrapExitCodeError(err, exitcodes.ExitGeneralRuntimeError)
		}
	} else {
		debug.Println("Writing analysis output to stdout")
		_, err = cmd.OutOrStdout().Write([]byte(output))
		if err != nil {
			return fmt.Errorf("writing analysis output: %w", err)
		}
	}

	return nil
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
		w := tabwriter.NewWriter(&sb, minWidth, tabWidth, padding, padChar, flags)
		if _, err := fmt.Fprintln(w, "PATH\tTYPE\tDETAILS\tCOUNT"); err != nil {
			return fmt.Sprintf("Error writing header to text output: %v", err)
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
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", p.Path, p.Type, details, p.Count); err != nil {
				return fmt.Sprintf("Error writing row to text output: %v", err)
			}
		}
		if err := w.Flush(); err != nil {
			return fmt.Sprintf("Error flushing text output: %v", err)
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

func formatStringSlice(slice []string) string {
	var sb strings.Builder
	for i, s := range slice {
		sb.WriteString(s)
		if i < len(slice)-1 {
			sb.WriteString(", ")
		}
	}
	return sb.String()
}

// initConfig reads in config file and ENV variables if set.
// NOTE: We are not currently using a config file or environment variables beyond LOG_LEVEL/IRR_DEBUG (handled in packages).
func initConfig() {
	// Original viper logic commented out as it's not used
	// if cfgFile != "" {
	// 	 // Use config file from the flag.
	// 	 viper.SetConfigFile(cfgFile)
	// } else {
	// 	 // Find home directory.
	// 	 home, err := os.UserHomeDir()
	// 	 cobra.CheckErr(err)
	//
	// 	 // Search config in home directory with name ".irr" (without extension).
	// 	 viper.AddConfigPath(home)
	// 	 viper.SetConfigType("yaml")
	// 	 viper.SetConfigName(".irr")
	// }
	//
	// viper.AutomaticEnv()
	//
	// if err := viper.ReadInConfig(); err == nil {
	// 	 fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	// }
}

// Override command implementation moved to override.go
