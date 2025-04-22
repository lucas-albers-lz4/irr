// Package main is the entry point for the irr CLI application.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/exitcodes"
	log "github.com/lalbers/irr/pkg/log"
	// Removed cmd import to break cycle
)

// isHelmPlugin indicates if the application is running as a Helm plugin.
// This is determined by checking environment variables set by Helm.
// REMOVED - Declaration moved to root.go
// var isHelmPlugin bool

// BinaryVersion is replaced during build with the value from plugin.yaml.
// When you run make build or make dist, Go replaces this value in the compiled binary.
var BinaryVersion = "0.2.0"

// main is the entry point of the application.
// It calls the Execute function defined locally (likely in root.go) to set up and run the commands.
func main() {
	// Initialize debug based on the environment variable checked in its init()
	debug.Init(debug.Enabled)

	// Use stdLog for consistency, check if debug is enabled
	log.Debug("--- IRR BINARY VERSION:", "version", BinaryVersion)

	// Check for IRR_DEBUG environment variable for potential future debug setup
	if parseIrrDebugEnvVar() {
		// Place any IRR_DEBUG specific setup here if needed in the future
		fmt.Println("IRR_DEBUG environment variable detected, enabling debug logs.")
	}

	// Check if we're running as a Helm plugin
	// isHelmPlugin = isRunningAsHelmPlugin()

	// Log Helm environment variables when in debug mode
	log.Debug("### DETECTED RUNNING IN STANDALONE MODE ###")

	// Initialize Helm plugin if necessary
	// if isHelmPlugin {
	// 	log.Debugf("Running as Helm plugin")
	// 	// Check Helm version compatibility
	// 	if err := version.CheckHelmVersion(); err != nil {
	// 		log.Errorf("Helm version check failed: %v", err)
	// 		if code, ok := exitcodes.IsExitCodeError(err); ok {
	// 			os.Exit(code)
	// 		}
	// 		os.Exit(1)
	// 	}
	// 	log.Debugf("Helm version check passed")
	// 	// initHelmPlugin will be called in init() of the root.go file
	// }

	// Execute the root command (defined in root.go, package main)
	// Cobra's Execute() handles its own error printing. We check the returned
	// error to propagate the correct exit code.
	if err := Execute(); err != nil {
		// Check if the error is a custom ExitCodeError
		if code, ok := exitcodes.IsExitCodeError(err); ok {
			// Use the specific exit code from the error
			os.Exit(code)
		}
		// Cobra likely printed the error already, use a generic failure code
		os.Exit(1)
	}
}

// isRunningAsHelmPlugin checks if the program is being run as a Helm plugin
func isRunningAsHelmPlugin() bool {
	// Check for environment variables set by Helm when running a plugin
	return os.Getenv("HELM_PLUGIN_NAME") != "" || os.Getenv("HELM_PLUGIN_DIR") != ""
}

// parseIrrDebugEnvVar checks the IRR_DEBUG environment variable to determine if debugging is enabled
func parseIrrDebugEnvVar() bool {
	debugEnv := os.Getenv("IRR_DEBUG")
	if debugEnv == "" {
		return false
	}

	// Check for common "true" values
	debugEnv = strings.ToLower(debugEnv)
	return debugEnv == "1" || debugEnv == "true" || debugEnv == "yes"
}

// logHelmEnvironment logs Helm-related environment variables for debugging
func logHelmEnvironment() {
	helmEnvVars := []string{
		"HELM_PLUGIN_DIR",
		"HELM_PLUGIN_NAME",
		"HELM_NAMESPACE",
		"HELM_BIN",
		"HELM_DEBUG",
		"HELM_PLUGINS",
		"HELM_REGISTRY_CONFIG",
		"HELM_REPOSITORY_CACHE",
		"HELM_REPOSITORY_CONFIG",
	}

	log.Debug("Helm Environment Variables:")
	for _, envVar := range helmEnvVars {
		value := os.Getenv(envVar)
		if value != "" {
			log.Debug("Helm Env", "var", envVar, "value", value)
		}
	}
}

// MIGRATION NOTE: All legacy log.Debugf and log.IsDebugEnabled calls have been migrated to slog-based logging.
