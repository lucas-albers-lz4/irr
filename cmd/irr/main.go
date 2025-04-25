// Package main is the entry point for the irr CLI application.
package main

import (
	"os"

	"github.com/lalbers/irr/pkg/exitcodes"
	log "github.com/lalbers/irr/pkg/log"
	// Removed cmd import to break cycle
)

// BinaryVersion is replaced during build with the value from plugin.yaml.
// When you run make build or make dist, Go replaces this value in the compiled binary.
var BinaryVersion = "0.2.0"

// main is the entry point of the application.
// It calls the Execute function defined locally (likely in root.go) to set up and run the commands.
func main() {
	// Use stdLog for consistency, check if debug is enabled
	log.Debug("--- IRR BINARY VERSION:", "version", BinaryVersion)

	// Log Helm environment variables when in debug mode
	// TODO: Re-evaluate if this standalone mode log is always accurate or needed.
	log.Debug("### DETECTED RUNNING IN STANDALONE MODE ###")

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

// logHelmEnvironment logs Helm-related environment variables for debugging
func logHelmEnvironment() {
	helmEnvVars := []string{
		"HELM_PLUGIN_DIR",
		"HELM_PLUGIN_NAME",
		"HELM_NAMESPACE",
		"HELM_BIN",
		"HELM_DEBUG", // Note: This is the Helm binary debug flag, not IRR's
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
