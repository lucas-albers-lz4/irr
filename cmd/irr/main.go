// Package main is the entry point for the irr CLI application.
package main

import (
	"fmt"
	"os"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/exitcodes"
	log "github.com/lalbers/irr/pkg/log"
	// Removed cmd import to break cycle
)

var version = "DEBUG v1"
var isHelmPlugin bool

// main is the entry point of the application.
// It calls the Execute function defined locally (likely in root.go) to set up and run the commands.
func main() {
	// Initialize debug based on the environment variable checked in its init()
	debug.Init(debug.Enabled)

	// Use stdLog for consistency, check if debug is enabled
	if log.IsDebugEnabled() {
		log.Debugf("--- IRR BINARY VERSION: %s ---", version)
	}

	// Check for IRR_DEBUG environment variable for potential future debug setup
	if os.Getenv("IRR_DEBUG") != "" {
		// Place any IRR_DEBUG specific setup here if needed in the future
		fmt.Println("IRR_DEBUG environment variable detected, enabling debug logs.")
	}

	// Check if we're running as a Helm plugin
	isHelmPlugin = isRunningAsHelmPlugin()

	// Initialize Helm plugin if necessary
	if isHelmPlugin {
		log.Debugf("Running as Helm plugin")
		// initHelmPlugin will be called in init() of the root.go file
	}

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
