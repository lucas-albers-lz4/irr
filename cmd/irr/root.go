// Package main implements the command-line interface for the irr (Image Relocation and Rewrite) tool.
// It provides commands for analyzing Helm charts and generating override values to redirect
// container image references from public registries to a target private registry.
//
// The main CLI commands are:
//   - inspect: Inspect a Helm chart to identify image references
//   - override: Generate override values to redirect images to a target registry
//   - validate: Validate generated overrides with Helm template
//
// Each command has various flags for configuration. See the help output for details.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lalbers/irr/pkg/exitcodes"
	log "github.com/lalbers/irr/pkg/log"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/helm"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
	isTestMode   bool
	isHelmPlugin bool
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
func getChartSource(cmd *cobra.Command, args []string) (*ChartSource, error) {
	// Initialize result
	result := &ChartSource{
		SourceType: "unknown",
	}

	// Get chart path
	chartPath, err := cmd.Flags().GetString("chart-path")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get chart-path flag: %w", err),
		}
	}
	result.ChartPath = chartPath
	chartPathProvided := chartPath != ""
	chartPathFlag := cmd.Flags().Changed("chart-path")

	// Get release name from --release-name flag or positional argument
	releaseNameFlag, err := cmd.Flags().GetString("release-name")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get release-name flag: %w", err),
		}
	}

	// Get release name from args or flag
	var releaseName string
	if len(args) > 0 {
		releaseName = args[0]
		result.Message = "Release name provided as argument"
	} else if releaseNameFlag != "" {
		releaseName = releaseNameFlag
		result.Message = "Release name provided via --release-name flag"
	}
	result.ReleaseName = releaseName
	releaseNameProvided := releaseName != ""
	releaseNameFlagSet := cmd.Flags().Changed("release-name")

	// Get namespace with default
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get namespace flag: %w", err),
		}
	}

	// Default namespace to "default" if not provided
	if namespace == "" {
		namespace = "default"
		debug.Printf("No namespace specified, using default: %s", namespace)
	}
	result.Namespace = namespace

	// Handle the case where neither is provided - attempt auto-detection
	if !chartPathProvided && !releaseNameProvided {
		// Try to detect chart in current directory - use the one from inspect.go
		detectedPath, err := detectChartInCurrentDirectory(AppFs, ".")
		if err != nil {
			// In plugin mode with no inputs, return clear error
			if isHelmPlugin {
				return nil, &exitcodes.ExitCodeError{
					Code: exitcodes.ExitInputConfigurationError,
					Err:  fmt.Errorf("either --chart-path or release name must be provided"),
				}
			}

			// In standalone mode with no inputs, return clear error
			return nil, &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("chart path not provided and could not auto-detect chart in current directory: %w", err),
			}
		}

		// Successfully auto-detected
		result.ChartPath = detectedPath
		result.SourceType = ChartSourceTypeAutoDetected
		result.Message = "Auto-detected chart in current directory"
		debug.Printf("Auto-detected chart path: %s", detectedPath)
		return result, nil
	}

	// If releaseName is provided but we're not in plugin mode, that's an error
	if releaseNameProvided && !chartPathProvided && !isHelmPlugin {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("release name provided but not running as Helm plugin. Use --chart-path in standalone mode"),
		}
	}

	// If the --release-name flag was explicitly set but we're not in plugin mode, that's an error
	if releaseNameFlagSet && !isHelmPlugin {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("the --release-name flag is only available when running as a Helm plugin (helm irr...)"),
		}
	}

	// If both are provided, use chart path primarily (with warning if there's potential conflict)
	if chartPathProvided && releaseNameProvided {
		if chartPathFlag && isHelmPlugin {
			// Both explicitly provided in plugin mode - prioritize chart path
			debug.Printf("Both chart path and release name provided, using chart path: %s", chartPath)
			result.SourceType = chartSourceTypeChart
			result.Message = "Using chart path (release name ignored)"
		} else {
			// In plugin mode without explicit chart path, prefer release name
			if isHelmPlugin && !chartPathFlag {
				result.SourceType = chartSourceTypeRelease
				result.Message = "Using release name in plugin mode"
			} else {
				// Default to chart path in other cases
				result.SourceType = chartSourceTypeChart
				result.Message = "Using chart path"
			}
		}
		return result, nil
	}

	// At this point, only one of chartPath or releaseName is provided
	switch {
	case chartPathProvided:
		result.SourceType = chartSourceTypeChart
		result.Message = "Using chart path"
	case releaseNameProvided && isHelmPlugin:
		result.SourceType = chartSourceTypeRelease
		result.Message = "Using release name in plugin mode"
	default:
		// This should be unreachable given the checks above
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("internal error: unable to determine chart source"),
		}
	}

	return result, nil
}

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
		// If debug is enabled, print environment, args, and plugin/debug status
		if debugEnabled || os.Getenv("IRR_DEBUG") == "1" || os.Getenv("IRR_DEBUG") == "true" {
			// Print all environment variables
			for _, e := range os.Environ() {
				fmt.Fprintf(os.Stderr, "[DEBUG] ENV: %s\n", e)
			}
			// Print all arguments
			fmt.Fprintf(os.Stderr, "[DEBUG] ARGS: %v\n", os.Args)
			// Print plugin/debug status
			fmt.Fprintf(os.Stderr, "[DEBUG] isHelmPlugin: %v, debugEnabled: %v\n", isHelmPlugin, debugEnabled)
		}

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
						debug.Printf("IRR_DEBUG environment variable enabled debug logging.") // Use debug.Printf
					}
				}
			} else {
				// Default to false if neither flag nor env var is set and non-empty
				debug.Enabled = false
			}
		}

		// Only log level in debug mode to avoid duplicate output
		if debug.Enabled {
			debug.Printf("Effective log level set to %s", level)
		}
		debug.Printf("Debug package enabled: %t", debug.Enabled)

		// Integration test mode check
		if integrationTestMode {
			log.Warnf("Integration test mode enabled.")
		}

		// Initialize Helm client if running as a Helm plugin
		if isHelmPlugin {
			settings := helm.GetHelmSettings()
			helmClient = helm.NewRealHelmClient(settings)
			debug.Printf("Initialized Helm client for plugin mode")
		}

		if registryFile != "" {
			_, isMemMapFs := AppFs.(*afero.MemMapFs)
			if !isMemMapFs {
				AppFs = afero.NewOsFs()
				debug.Printf("Using OS filesystem for registry mappings")
			} else {
				debug.Printf("Preserving in-memory filesystem for testing")
			}
			debug.Printf("Root command: Attempting to load mappings from %s", registryFile)
			_, err := registry.LoadMappings(AppFs, registryFile, integrationTestMode)
			if err != nil {
				debug.Printf("Root command: Failed to load mappings: %v", err)
				debug.Printf("Warning: Failed to load registry mappings from %s: %v. Proceeding without mappings.", registryFile, err)
			}
		}

		// Add debug log for execution mode detection
		pluginModeDetected := isRunningAsHelmPlugin()
		debug.Printf("Execution Mode Detected: %s", map[bool]string{true: "Plugin", false: "Standalone"}[pluginModeDetected])

		// Add a clear log message for plugin mode that will appear even at info level
		if pluginModeDetected {
			log.Infof("IRR v%s running as Helm plugin", BinaryVersion)
		} else {
			log.Infof("IRR v%s running in standalone mode", BinaryVersion)
		}

		return nil
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.irr.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debugEnabled, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "set log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&integrationTestMode, "integration-test", false, "enable integration test mode")
	// For testing purposes
	rootCmd.PersistentFlags().BoolVar(&TestAnalyzeMode, "test-analyze", false, "enable test mode (originally for analyze command, now for inspect)")

	// Hide the flags from regular usage
	if err := rootCmd.PersistentFlags().MarkHidden("integration-test"); err != nil {
		fmt.Printf("Warning: Failed to mark integration-test flag as hidden: %v\n", err)
	}
	if err := rootCmd.PersistentFlags().MarkHidden("test-analyze"); err != nil {
		fmt.Printf("Warning: Failed to mark test-analyze flag as hidden: %v\n", err)
	}

	// Add commands
	// rootCmd.AddCommand(newAnalyzeCmd()) // Removed as part of Phase 3
	rootCmd.AddCommand(newOverrideCmd())
	rootCmd.AddCommand(newInspectCmd())
	rootCmd.AddCommand(newValidateCmd())

	// Add release-name and namespace flags to root command for all modes
	// We'll check isHelmPlugin before using them in the command execution
	addReleaseFlag(rootCmd)
	addNamespaceFlag(rootCmd)

	// Check if running as Helm plugin
	if isHelmPlugin {
		// Initialize Helm plugin specific functionality
		initHelmPlugin()
	} else {
		// If not running as a plugin, hide the plugin-specific flags
		removeHelmPluginFlags(rootCmd)
	}

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
			exists, _ := afero.Exists(AppFs, projectConfigFile)
			if exists {
				viper.SetConfigFile(projectConfigFile)
			}
		}
	}
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
func initConfig() {
	// Check if IRR_TESTING is set
	if os.Getenv("IRR_TESTING") != "" {
		isTestMode = true
		log.Infof("IRR_TESTING environment variable is set. Running in test mode.")
	}

	// Determine if running as a Helm plugin
	if os.Getenv("HELM_BIN") != "" && os.Getenv("HELM_PLUGIN_NAME") == "irr" {
		isHelmPlugin = true
		// Optionally log Helm environment details for debugging plugin issues
		if log.IsDebugEnabled() {
			logHelmEnvironment()
		}
	}

	// Determine if integration test mode is active
	if os.Getenv("IRR_INTEGRATION_TEST") != "" {
		integrationTestMode = true
		log.Debugf("IRR_INTEGRATION_TEST environment variable is set.")
	}

	// Handle filesystem setup based on test/integration mode
	if isTestMode || integrationTestMode {
		// In test modes, assume AppFs is already set up by the test harness
		log.Debugf("Test mode detected, using pre-configured AppFs: %T", AppFs)
		if AppFs == nil {
			log.Warnf("Test mode active, but AppFs is nil. Defaulting to OS filesystem.")
			AppFs = afero.NewOsFs()
		}
	} else {
		// In normal operation, use the real OS filesystem
		AppFs = afero.NewOsFs()
	}

	// Get chart path and release name from flags if available
	// Note: This uses pflag directly as cobra binding might not be complete yet
	chartPathFlag := pflag.Lookup("chart-path")
	chartPathProvided := chartPathFlag != nil && chartPathFlag.Changed

	releaseNameFlag := pflag.Lookup("release-name")
	releaseNameProvided := releaseNameFlag != nil && releaseNameFlag.Changed

	// If chart-path and release-name are not provided, try to auto-detect
	if !chartPathProvided && !releaseNameProvided {
		// Try to detect chart in current directory - use the one from inspect.go
		// Pass "." as the starting directory
		detectedPath, err := detectChartInCurrentDirectory(AppFs, ".")
		if err != nil {
			// In plugin mode with no inputs, return clear error
			if isHelmPlugin {
				// Cobra handles this better in RunE, but good to log early if possible
				log.Debugf("Plugin mode active, but no chart path or release name provided, and auto-detect failed: %v", err)
			} else {
				// Standalone mode: Log if detection fails, command will likely fail later if path is required
				log.Debugf("Chart path not provided and auto-detect failed: %v", err)
			}
		} else {
			// If detected, potentially use this path for context?
			// For now, just log the detection.
			log.Debugf("Auto-detected chart directory: %s", detectedPath)
		}
	}

	// Initialize Helm client/adapter factory
	initializeHelmAdapterFactory()

	// Remove Helm plugin flags if not running as a plugin
	if !isHelmPlugin {
		removeHelmPluginFlags(rootCmd)
	}
}
