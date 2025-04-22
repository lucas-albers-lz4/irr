package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/lalbers/irr/pkg/log"

	"log/slog"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/helm"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Constants
const (
	expectedEnvVarParts = 2
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

	// Helm client
	helmClient helm.ClientInterface

	// New variables for initConfig
	isTestMode bool
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
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		// --- Early Debug Logging (Before Main Slog Init) ---
		if debugEnabled {
			// Create a temporary, basic JSON logger to stderr for early debug info
			// This won't interfere with the main logger configured later by pkg/log
			tempLogger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelDebug, // Ensure debug messages are logged
			}))

			// Log environment variables (consider logging only specific ones if needed)
			envVars := make(map[string]string)
			for _, e := range os.Environ() {
				pair := strings.SplitN(e, "=", expectedEnvVarParts)
				if len(pair) == expectedEnvVarParts {
					envVars[pair[0]] = pair[1]
				}
			}
			tempLogger.Debug("Early debug info", slog.Any("environment", envVars))

			// Log arguments
			tempLogger.Debug("Early debug info", slog.Any("arguments", os.Args))

			// Log plugin status
			tempLogger.Debug("Early debug info", slog.Bool("isHelmPlugin", isRunningAsHelmPlugin()))
		}
		// --- End Early Debug Logging ---

		// --- Setup main logging using pkg/log ---
		logLevelStr := logLevel          // From --log-level flag
		debugFlagEnabled := debugEnabled // From --debug flag

		// Determine the target level based on flags and env vars (IRR_DEBUG handled by pkg/log/init)
		var targetLevel log.Level
		var parseErr error

		if debugFlagEnabled { // --debug flag takes highest precedence
			targetLevel = log.LevelDebug
		} else if logLevelStr != "" { // Then --log-level flag
			targetLevel, parseErr = log.ParseLevel(logLevelStr)
			if parseErr != nil {
				if integrationTestMode { // Only warn in test mode
					// Use slog for warnings, assuming it might be configured by now (or default)
					log.Warn("Invalid log level specified via flag. Using default.", "input", logLevelStr, "default", log.LevelInfo)
				}
				targetLevel = log.LevelInfo // Default to Info on parse error
			} // If no parse error, targetLevel is set correctly
		} else {
			// If neither --debug nor --log-level is set, the level is determined
			// solely by pkg/log/init based on LOG_LEVEL and IRR_DEBUG env vars.
			// We don't need to call SetLevel here in that case, but we can retrieve the
			// current level for logging purposes if needed.
			targetLevel = log.Level(log.CurrentLevel()) // Reflect the level set by init()
		}

		// Set the level explicitly *if* a flag determined it.
		// Otherwise, let the level determined by init() stand.
		if debugFlagEnabled || logLevelStr != "" {
			log.SetLevel(targetLevel)
		}

		// --- End Logging Setup ---

		// Integration test mode check
		if integrationTestMode {
			log.Warn("Integration test mode enabled.")
		}

		// Initialize Helm client if running as a Helm plugin
		if isRunningAsHelmPlugin() {
			settings := helm.GetHelmSettings()
			helmClient = helm.NewRealHelmClient(settings)
			log.Debug("Initialized Helm client for plugin mode")
		}

		if registryFile != "" {
			_, isMemMapFs := AppFs.(*afero.MemMapFs)
			if !isMemMapFs {
				AppFs = afero.NewOsFs()
				log.Debug("Using OS filesystem for registry mappings")
			} else {
				log.Debug("Preserving in-memory filesystem for testing")
			}
			log.Debug("Root command: Attempting to load mappings", "file", registryFile)
			_, err := registry.LoadMappings(AppFs, registryFile, integrationTestMode)
			if err != nil {
				log.Debug("Root command: Failed to load mappings", "error", err)
				log.Debug("Warning: Failed to load registry mappings. Proceeding without mappings.", "file", registryFile, "error", err)
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
		chartDir, chartDetectErr := detectChartInCurrentDirectory(AppFs, ".") // Start search from "."
		if chartDetectErr == nil {
			projectConfigFile := filepath.Join(chartDir, ".irr.yaml")
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
}

// --- Analyze Command Functionality --- Now integrated into inspect command

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

// initConfig reads in config file and ENV variables if set.
//
//nolint:unused // initConfig is called by cobra.OnInitialize in init()
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
