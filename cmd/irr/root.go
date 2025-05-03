// Package main implements the irr CLI commands.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/lucas-albers-lz4/irr/pkg/log"

	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/override"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Constants
const (
	// unknownLogLevelSource is the initial value for log level source determination.
	unknownLogLevelSource = "unknown"
)

// Global flag variables
var (
	cfgFile      string
	debugEnabled bool
	logLevel     string
	// Previous analyze command flags (now integrated with inspect)
	// outputFormat string

	// Output and mode flags
	registryFile string

	// IntegrationTestMode controls behavior specific to integration tests
	integrationTestMode bool

	// TestAnalyzeMode is a global flag to enable test mode (originally for analyze command, now for inspect)
	TestAnalyzeMode bool
)

// AppFs defines the filesystem interface to use, allows mocking in tests.
var AppFs = afero.NewOsFs()

// SetFs replaces the current filesystem with the provided one and returns a function to restore it.
// This is primarily used for testing.
func SetFs(newFs afero.Fs) func() {
	oldFs := AppFs
	AppFs = newFs
	return func() { AppFs = oldFs }
}

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

// AnalyzerInterface defines methods for chart analysis.
// This interface allows for chart analysis functionality to be mocked in tests.
type AnalyzerInterface interface {
	// Analyze performs chart analysis and returns the detected image patterns
	// or an error if the analysis fails.
	Analyze() (*analysis.ChartAnalysis, error)
}

// --- Factory for Generator ---

// GeneratorInterface defines the methods expected from a generator.
// This interface is used to allow mocking in tests and to provide a clean
// abstraction between the CLI and the chart processing logic.
type GeneratorInterface interface {
	// Generate performs image reference override generation and returns
	// the override file structure or an error if generation fails.
	Generate() (*override.File, error)
}

// Regular expression for validating registry names (simplified based on common usage)
// var registryRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9](:\\d+)?$`)

// ChartSource represents the source information for a chart operation.
// It consolidates chart path, release name, and namespace information.
type ChartSource struct {
	// ChartPath is the path to the chart directory or tarball
	ChartPath string
	// ReleaseName is the name of the Helm release
	ReleaseName string
	// Namespace is the Kubernetes namespace
	Namespace string
	// SourceType indicates how the chart source was determined
	// Valid values: "chart", "release", "auto-detected"
	SourceType string
	// Message contains additional information about how the source was determined
	Message string
}

// getChartSource retrieves and standardizes chart source information from flags and arguments.
// It implements the unified logic for --chart-path and --release-name flags:
// - Both flags can be used together
// - Auto-detection when only one is provided
// - Default to --release-name in plugin mode; default to --chart-path in standalone mode
// - Namespace always defaults to "default" when not provided
//
// The function returns a ChartSource struct with all necessary information.
// func getChartSource(cmd *cobra.Command, args []string) (*ChartSource, error) {

// detectChartInCurrentDirectory is defined in inspect.go to prevent duplicate functions

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "irr",
	Short: "Image Relocation and Rewrite tool for Helm Charts and K8s YAML",
	Long: `irr (Image Relocation and Rewrite) is a tool for generating Helm override values
that redirect container image references from public registries to a private registry.

It can analyze Helm charts to identify image references and generate override values 
files compatible with Helm, pointing images to a new registry according to specified strategies.
It also supports linting image references for potential issues.`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// --- Determine Final Log Level Based on Precedence --- START ---
		logLevelFlagStr := logLevel              // Value from --log-level flag
		debugFlagEnabled := debugEnabled         // Value from --debug flag
		envLogLevelStr := os.Getenv("LOG_LEVEL") // Value from env var

		// +++ Raw Debugging Output +++
		log.Debug("[PRE-RUN] Raw inputs",
			"debug", debugFlagEnabled,
			"log_level", logLevelFlagStr,
			"log_level_changed", cmd.Flags().Changed("log-level"),
			"env_log_level", envLogLevelStr)

		var finalLevel log.Level
		levelSource := unknownLogLevelSource // Initialize level source

		// 1. --debug flag has highest precedence
		if debugFlagEnabled {
			finalLevel = log.LevelDebug
			levelSource = "--debug flag"
		} else {
			// 2. --log-level flag is next, ONLY if it was explicitly set
			if cmd.Flags().Changed("log-level") && logLevelFlagStr != "" { // Check cmd.Flags().Changed()
				parsedLevel, err := log.ParseLevel(logLevelFlagStr)
				if err == nil {
					finalLevel = log.Level(parsedLevel)
					levelSource = "--log-level flag"
				} else {
					// Invalid flag, log warning later, proceed to check env var
					log.Debug("[PRE-RUN WARN] Invalid log level flag", "value", logLevelFlagStr)
				}
			}

			// 3. LOG_LEVEL env var is next (if flags didn't set a valid level)
			if levelSource == unknownLogLevelSource && envLogLevelStr != "" {
				parsedLevel, err := log.ParseLevel(envLogLevelStr)
				if err == nil {
					finalLevel = log.Level(parsedLevel)
					levelSource = "LOG_LEVEL env var"
				} else {
					// Invalid env var, log warning later, proceed to default
					log.Debug("[PRE-RUN WARN] Invalid LOG_LEVEL env var", "value", envLogLevelStr)
				}
			}

			// 4. Default level if nothing else set it
			if levelSource == unknownLogLevelSource { // Check against initial value
				// Check flags AND the environment variable set by the test harness
				isTestRun := integrationTestMode || TestAnalyzeMode || (os.Getenv("IRR_TESTING") == "true")
				if isTestRun {
					finalLevel = log.LevelInfo // Default to Info for test runs
				} else {
					// Change default for normal runs to Info
					finalLevel = log.LevelInfo // Default to Info for normal/plugin runs
				}
				levelSource = "mode default"
			}
		}

		// +++ Raw Debugging Output +++
		log.Debug("[PRE-RUN] Determined final level",
			"level", finalLevel.String(),
			"source", levelSource)

		// --- Apply Final Log Level --- START ---
		log.SetLevel(finalLevel)
		// Log the final effective level and its source *at debug level* for clarity
		log.Debug("Effective log level set", "level", finalLevel.String(), "source", levelSource)
		// --- Apply Final Log Level --- END ---

		// --- Remaining PreRun Setup ---

		// Integration test mode warning (still useful to know it's active)
		if integrationTestMode {
			// This warning should respect the final log level set above
			log.Warn("Integration test mode enabled.")
		}

		if registryFile != "" {
			_, isMemMapFs := AppFs.(*afero.MemMapFs)
			if !isMemMapFs {
				AppFs = afero.NewOsFs()
				log.Debug("Using OS filesystem for registry mappings")
			} else {
				log.Debug("Preserving in-memory filesystem for testing")
			}

			// Determine if we should skip CWD check based on flags OR env var
			shouldSkipCWDRestriction := integrationTestMode || (os.Getenv("IRR_TESTING") == "true") // <-- Use combined check

			log.Debug("Root command: Attempting to load mappings", "file", registryFile, "skipCWDRestriction", shouldSkipCWDRestriction)
			// Load structured config first, fall back to legacy
			_, err := registry.LoadStructuredConfigDefault(registryFile, shouldSkipCWDRestriction) // <-- Pass the combined check result
			if err != nil {
				log.Debug("Failed loading structured config, trying legacy LoadMappings", "error", err)
				// We need to load the legacy Mappings object to proceed
				_, legacyErr := registry.LoadMappings(AppFs, registryFile, shouldSkipCWDRestriction) // <-- Pass the combined check result here too
				if legacyErr != nil {
					// Log as debug because this happens early, might not be fatal
					log.Debug("Failed to load registry mappings (both formats). Proceeding without mappings.", "file", registryFile, "structured_error", err, "legacy_error", legacyErr)
				}
			} else {
				log.Debug("Successfully loaded structured registry config", "file", registryFile)
			}
		}

		// Add debug log for execution mode detection
		pluginModeDetected := isRunningAsHelmPlugin()
		log.Debug("Execution Mode Detected", "mode", map[bool]string{true: "Plugin", false: "Standalone"}[pluginModeDetected])

		// Add a clear log message for plugin mode that will appear even at info level
		if pluginModeDetected {
			log.Info("IRR running as Helm plugin", "version", BinaryVersion)
		} else {
			log.Info("IRR running in standalone mode", "version", BinaryVersion)
		}

		return nil
	},
	RunE: func(_ *cobra.Command, args []string) error {
		// If no arguments (subcommand) are provided, return an error.
		if len(args) == 0 {
			log.Error("A subcommand is required. Use 'irr --help' for available commands.")
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.irr.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debugEnabled, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "set log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&integrationTestMode, "integration-test", false, "enable integration test mode")
	// For testing purposes
	rootCmd.PersistentFlags().BoolVar(&TestAnalyzeMode, "test-analyze", false, "enable test mode (originally for analyze command, now for inspect)")

	// Hide the flags from regular usage
	if err := rootCmd.PersistentFlags().MarkHidden("integration-test"); err != nil {
		log.Warn("Failed to mark integration-test flag as hidden", "error", err)
	}
	if err := rootCmd.PersistentFlags().MarkHidden("test-analyze"); err != nil {
		log.Warn("Failed to mark test-analyze flag as hidden", "error", err)
	}

	// Add commands
	// rootCmd.AddCommand(newAnalyzeCmd()) // Removed as part of Phase 3
	rootCmd.AddCommand(newOverrideCmd())
	rootCmd.AddCommand(newInspectCmd())
	rootCmd.AddCommand(newValidateCmd())

	// Add release-name and namespace flags to root command for all modes
	addReleaseFlag(rootCmd)
	addNamespaceFlag(rootCmd)

	// Find and read the config file
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Search config in home directory with name ".irr" (without extension).
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)
		viper.SetConfigName(".irr")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(home)

		// Attempt to find chart path for context-aware config loading
		// We need to do this *before* viper reads the config, but Viper doesn't expose the fs easily.
		// So, we use our own detection logic with AppFs.
		var relativePath string                                                   // Declare relativePath
		chartDir, relativePath, chartDetectErr := detectChartIfNeeded(AppFs, ".") // Start search from "."
		if chartDetectErr == nil {
			// Use chartDir (absolute path) for joining with config file name
			projectConfigFile := filepath.Join(chartDir, ".irr.yaml")
			log.Debug("Checking for project-specific config", "path", projectConfigFile, "basedOnDetectedChartDir", chartDir, "relativeChartPath", relativePath)
			exists, err := afero.Exists(AppFs, projectConfigFile)
			if err != nil {
				log.Warn("Failed to check if project config file exists", "error", err)
			} else if exists {
				viper.SetConfigFile(projectConfigFile)
			}
		}
	}

	// Add build version info
	rootCmd.Version = BinaryVersion

	viper.SetDefault("logLevel", "info")
}

// --- Analyze Command Functionality --- Now integrated into inspect command

// Get the root command - useful for testing
func getRootCmd() *cobra.Command {
	return rootCmd
}

// executeCommand is a helper for testing Cobra commands
func executeCommand(root *cobra.Command, args ...string) (stdout, stderr string, err error) {
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	root.SetOut(stdoutBuf)
	root.SetErr(stderrBuf)

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

	return stdoutBuf.String(), stderrBuf.String(), err
}

// initConfig reads in config file and ENV variables if set.
// Called by cobra.OnInitialize in init()
//
//nolint:unused // Called by cobra.OnInitialize, but linter doesn't detect it.
func initConfig() {
	// Only run initConfig once
	if viper.IsSet("config.read") {
		return
	}

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err) // This will exit on error

		// Search config in home directory with name ".irr" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".irr")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// Attempt to read the config file
	if err := viper.ReadInConfig(); err == nil {
		// Use slog for logging config file usage
		log.Debug("Using configuration file", "file", viper.ConfigFileUsed())
	} else {
		// Log the error only if it's not a "file not found" error, or if a specific file was requested
		if !errors.As(err, &viper.ConfigFileNotFoundError{}) || cfgFile != "" {
			// Use slog for logging config file errors
			log.Warn("Error reading configuration file", "file", viper.ConfigFileUsed(), "error", err)
		} else {
			// Use slog for logging when no config file is found (debug level)
			log.Debug("No configuration file found or specified, using defaults.")
		}
	}

	// Mark config as read to prevent re-running
	viper.Set("config.read", true)
}

// setupLogging configures the logger based on settings in Viper.
// Note: This function is currently unused as logging setup is handled in PersistentPreRunE.
/*
func setupLogging(v *viper.Viper) error {
	logLevelStr := v.GetString("logLevel")
	logFormat := v.GetString("logFormat")

	logLevel, err := log.ParseLevel(logLevelStr)
	if err != nil {
		log.Error("Invalid log level specified", "level", logLevelStr, "error", err)
		// Default to info if invalid
		logLevel = log.LevelInfo
		fmt.Fprintf(os.Stderr, "Warning: Invalid log level '%s'. Defaulting to %s.\n", logLevelStr, logLevel.String())
	}

	log.SetLevel(logLevel)
	log.SetFormat(logFormat) // Assumes SetFormat exists in your log package

	log.Debug("Logging configured", "level", logLevel.String(), "format", logFormat)
	return nil
}
*/
