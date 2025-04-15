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
	"strconv"

	log "github.com/lalbers/irr/pkg/log"

	"github.com/lalbers/irr/pkg/analysis"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/override"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
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

	// Check if running as Helm plugin
	if isHelmPlugin {
		initHelmPlugin()
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
