package main

import (
	"fmt"
	"os"

	// No need to import commands package
	"github.com/lalbers/irr/pkg/debug" // Ensure debug package is imported
)

// main is the entry point of the application.
// It calls the Execute function defined locally (likely in root.go) to set up and run the commands.
func main() {
	// Add a clear indicator that this version of the binary is running
	fmt.Println("--- IRR BINARY VERSION: DEBUG v1 ---")

	// Check for IRR_DEBUG environment variable for potential future debug setup
	if os.Getenv("IRR_DEBUG") != "" {
		// Place any IRR_DEBUG specific setup here if needed in the future
		debug.Init(true) // Correctly enable debug logging using Init()
		fmt.Println("IRR_DEBUG environment variable detected, enabling debug logs.")
	}

	// Always execute the main command logic (defined in the same package)
	Execute()
}
