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
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

const (
	// DefaultPathDepth defines the maximum depth for recursive path traversal.
	DefaultPathDepth = 10

	// Standard file permissions
	defaultFilePerm       fs.FileMode = 0o600 // Read/write for owner
	defaultOutputFilePerm fs.FileMode = 0o644 // Read/write for owner, read for group/others

	// Tabwriter settings
	tabwriterMinWidth = 0
	tabwriterTabWidth = 0
	tabwriterPadding  = 2
	tabwriterPadChar  = ' '
	tabwriterFlags    = 0
)

// AppFs provides an abstraction over the filesystem.
// Defaults to the OS filesystem, can be replaced with a memory map for tests.
var AppFs afero.Fs = afero.NewOsFs()

// Global flag variables (consider scoping if appropriate)
var (
	chartPath         string
	targetRegistry    string
	sourceRegistries  []string
	outputFile        string // Used by multiple commands
	pathStrategy      string
	verbose           bool
	dryRun            bool
	strictMode        bool
	excludeRegistries []string
	pathDepth         int
	debugEnabled      bool   // Used by multiple commands
	registryFile      string // Renamed from registryMappingsFile
	imageRegistry     string
	globalRegistry    string
	templateMode      bool
)

// Exit codes (keep public if needed elsewhere, otherwise consider keeping private)
const (
	ExitSuccess                   = 0
	ExitGeneralRuntimeError       = 1
	ExitInputConfigurationError   = 2
	ExitChartParsingError         = 3
	ExitParsingError              = 3 // Added for image parsing
	ExitImageProcessingError      = 4
	ExitUnsupportedStructure      = 5 // Added for strict mode
	ExitProcessingThresholdNotMet = 6 // Added for threshold
	ExitCodeInvalidStrategy       = 7
	ExitHelmTemplateError         = 8
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
	Short: "Helm Image Relocation and Rewrite tool",
	Long: `irr (Image Relocation and Rewrite) is a CLI tool to help manage container image references 
within Helm charts, facilitating the process of relocating images to a target registry.

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
		// Cobra already prints the error, so we just exit
		os.Exit(1)
	}
}

// Flag to indicate integration test mode (hidden)
var integrationTestMode bool

func init() {
	// Define persistent flags available to all commands
	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logging (overrides log-level to debug)")
	rootCmd.PersistentFlags().BoolVar(&integrationTestMode, "integration-test-mode", false, "Enable integration test mode (internal use)")
	_ = rootCmd.PersistentFlags().MarkHidden("integration-test-mode") // Hide the flag from help output

	// Add subcommands
	rootCmd.AddCommand(newAnalyzeCmd())
	// rootCmd.AddCommand(newDefaultCmd()) // Default command logic likely moved or removed
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
}

// --- Default (Override) Command --- Moved from original main.go

func newDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "override [flags]",
		Short: "Generate Helm overrides for redirecting container images",
		Long: `Generate Helm value overrides that redirect container images from source registries
to a target registry. This is the original functionality of IRR. If no subcommand is specified, this command runs by default.`,
		RunE: runOverride,
		// Restore default silencing for production behavior
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return cmd
}

// runOverride implements the logic for the override command.
func runOverride(cmd *cobra.Command, args []string) error {
	debug.FunctionEnter("runOverride")
	defer debug.FunctionExit("runOverride")

	// Retrieve flags needed early
	chartPath, _ := cmd.Flags().GetString("chart-path")
	targetRegistry, _ := cmd.Flags().GetString("target-registry")
	sourceRegistries, _ := cmd.Flags().GetStringSlice("source-registries")
	outputFile, _ := cmd.Flags().GetString("output-file")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	registryFile, _ := cmd.Flags().GetString("registry-file") // Renamed from registryMappingsFile
	strategyName, _ := cmd.Flags().GetString("strategy")
	excludeRegistries, _ := cmd.Flags().GetStringSlice("exclude-registries")
	threshold, _ := cmd.Flags().GetInt("threshold") // Get threshold value
	validate, _ := cmd.Flags().GetBool("validate")  // Get validate flag

	// Validate required flags
	if chartPath == "" {
		return errors.New("chart-path flag is required")
	}
	if targetRegistry == "" {
		return errors.New("target-registry flag is required")
	}
	// source-registries is not strictly required, an empty list means process all non-excluded.
	// if len(sourceRegistries) == 0 {
	// 	return errors.New("source-registries flag is required")
	// }

	// Log the received flag values
	debug.Printf("Executing override command with flags:")
	debug.Printf("  Chart Path: %s", chartPath)
	debug.Printf("  Target Registry: %s", targetRegistry)
	debug.Printf("  Source Registries: %v", sourceRegistries)
	debug.Printf("  Exclude Registries: %v", excludeRegistries)
	debug.Printf("  Output File: %s", outputFile)
	debug.Printf("  Dry Run: %v", dryRun)
	debug.Printf("  Strategy: %s", strategyName)
	debug.Printf("  Registry File: %s", registryFile)
	debug.Printf("  Threshold: %d%%", threshold)
	debug.Printf("  Validate: %v", validate)

	// Load registry mappings if file is provided
	var mappings *registry.Mappings
	if registryFile != "" {
		var loadMappingsErr error
		mappings, loadMappingsErr = registry.LoadMappings(registryFile)
		if loadMappingsErr != nil {
			return wrapExitCodeError(ExitInputConfigurationError,
				fmt.Sprintf("failed to load registry mappings from %s", registryFile),
				loadMappingsErr)
		}
	}

	// Select Strategy
	selectedStrategy, strategyErr := strategy.GetStrategy(pathStrategy, mappings)
	if strategyErr != nil {
		return &ExitCodeError{Code: ExitCodeInvalidStrategy, Err: strategyErr}
	}
	debug.Printf("Using path strategy: %T", selectedStrategy)

	// Use the global flag variables like chartPath, targetRegistry, sourceRegistries etc.
	debug.Printf("Chart Path: %s", chartPath)
	debug.Printf("Target Registry: %s", targetRegistry)
	debug.Printf("Source Registries: %v", sourceRegistries)
	debug.Printf("Exclude Registries: %v", excludeRegistries)
	debug.Printf("Registry File: %s", registryFile)

	// Retrieve necessary flags just before use
	includePattern, err := cmd.Flags().GetStringSlice("include-pattern")
	if err != nil {
		log.Warnf("Error getting include-pattern flag: %v. Using default (nil).", err)
	}
	excludePattern, err := cmd.Flags().GetStringSlice("exclude-pattern")
	if err != nil {
		log.Warnf("Error getting exclude-pattern flag: %v. Using default (nil).", err)
	}
	knownPathsVal, err := cmd.Flags().GetStringSlice("known-image-paths")
	if err != nil {
		log.Warnf("Error getting known-image-paths flag: %v. Using default (nil).", err)
	}

	// Create generator using the factory
	// Match the updated factory signature
	var loader analysis.ChartLoader = &chart.HelmLoader{} // Correct loader type
	generator := currentGeneratorFactory(
		chartPath, targetRegistry,
		sourceRegistries, excludeRegistries,
		selectedStrategy, mappings, strictMode, pathDepth, // Note: pathDepth isn't used by NewGenerator, but is by factory? Keep for now.
		loader,         // Pass analysis.ChartLoader
		includePattern, // Pass include patterns
		excludePattern, // Pass exclude patterns
		knownPathsVal,  // Pass known paths
	)

	// --- Generation ---
	debug.Println("Generating overrides...")
	overrideFile, err := generator.Generate()
	if err != nil {
		debug.Printf("Error during override generation: %v", err)

		// Default exit code and error message
		exitCode := ExitImageProcessingError // Default unless overridden
		errMsg := fmt.Sprintf("error generating overrides: %v", err)

		// Check if the error IS the specific strict validation failure
		if errors.Is(err, chart.ErrStrictValidationFailed) {
			debug.Println("Strict mode violation detected (using errors.Is), returning exit code 5.")
			exitCode = ExitUnsupportedStructure // Use specific exit code 5
			// Use the specific error message from the wrapped error
			errMsg = err.Error() // The wrapped error already contains the detailed message
		} else {
			// Handle other potential errors from Generate()
			// Could check for chart.ParsingError, chart.ImageProcessingError etc. if needed
			// For now, stick with the default ExitImageProcessingError
			debug.Printf("Non-strict error encountered: %v", err)
		}

		return &ExitCodeError{Code: exitCode, Err: errors.New(errMsg)}
	}

	// Log the count of unsupported structures found
	debug.Printf("Successfully generated override data. Unsupported structures found: %d", len(overrideFile.Unsupported))

	// --- Handle Output ---
	if dryRun {
		fmt.Println("--- Dry Run: Generated Overrides ---")
		outputBytes, err := overrideFile.ToYAML()
		if err != nil {
			return wrapExitCodeError(ExitGeneralRuntimeError, "failed to marshal overrides to YAML for dry run", err)
		}
		fmt.Println(string(outputBytes))
		fmt.Println("--- End Dry Run ---")
	} else {
		// Determine output file path
		outputFilePath := outputFile // Use the flag value directly
		if outputFilePath == "" {
			// Use a static default filename
			outputFilePath = "chart-overrides.yaml"
			debug.Printf("Output file not specified, defaulting to: %s", outputFilePath)
		}

		debug.Printf("Writing overrides to file: %s", outputFilePath)
		yamlBytes, err := overrideFile.ToYAML()
		if err != nil {
			return wrapExitCodeError(ExitGeneralRuntimeError, "failed to marshal overrides to YAML for writing", err)
		}
		// Use afero to write the file
		if writeErr := afero.WriteFile(AppFs, outputFilePath, yamlBytes, defaultFilePerm); writeErr != nil {
			return wrapExitCodeError(ExitGeneralRuntimeError, fmt.Sprintf("failed to write overrides to file %s", outputFilePath), writeErr)
		}
		if verbose {
			fmt.Printf("Overrides written to: %s\n", outputFilePath)
		}
	}

	debug.Println("Override command finished successfully.")
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
				if err := afero.WriteFile(AppFs, analyzeOutputFile, []byte(output), defaultOutputFilePerm); err != nil {
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
	cmd.Flags().StringVar(&outputFormat, "output", "text", "Output format (text or json)")
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
		w := tabwriter.NewWriter(&sb, tabwriterMinWidth, tabwriterTabWidth, tabwriterPadding, tabwriterPadChar, tabwriterFlags)
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
