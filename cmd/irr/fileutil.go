package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// Global variable for injecting mock Helm client during tests
// Removed dependency on mocks package

// Variables to allow mocking for tests
var (
	// helmAdapterFactory is a function that creates a Helm adapter.
	// It can be replaced in tests to inject a mock adapter.
	helmAdapterFactory = defaultHelmAdapterFactory // Initially point to the real factory
)

// defaultHelmAdapterFactory is the real implementation of creating a Helm adapter
func defaultHelmAdapterFactory() (*helm.Adapter, error) {
	// Create a new Helm client
	helmClient, err := helm.NewHelmClient()
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to initialize Helm client: %w", err),
		}
	}

	// Create adapter with the Helm client
	adapter := helm.NewAdapter(helmClient, AppFs, isRunningAsHelmPlugin())
	return adapter, nil
}

// createHelmAdapter creates a new Helm client and adapter, handling errors consistently
func createHelmAdapter() (*helm.Adapter, error) {
	return helmAdapterFactory()
}

// getCommandContext gets the context from a command or creates a background context if none exists
func getCommandContext(cmd *cobra.Command) context.Context {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx
}

// getReleaseNameAndNamespaceCommon extracts and validates release name and namespace
func getReleaseNameAndNamespaceCommon(cmd *cobra.Command, args []string) (releaseName, namespace string, err error) {
	releaseName, err = cmd.Flags().GetString("release-name")
	if err != nil {
		return "", "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get release-name flag: %w", err),
		}
	}

	// Check for positional argument as release name if flag is not set
	if releaseName == "" && len(args) > 0 {
		releaseName = args[0]
		log.Info("Using release name from positional argument", "releaseName", releaseName)
	}

	namespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return "", "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get namespace flag: %w", err),
		}
	}

	// --- Helm Plugin Namespace Correction ---
	// If running as a plugin and the namespace flag is still the default,
	// try getting the namespace from the HELM_NAMESPACE env var.
	// NOTE: Uses the 'defaultNamespace' constant (expected to be "default")
	if isRunningAsHelmPlugin() && namespace == defaultNamespace {
		envNamespace := os.Getenv("HELM_NAMESPACE")
		if envNamespace != "" {
			log.Debug("Namespace flag was default in plugin mode, using HELM_NAMESPACE env var instead", "env_namespace", envNamespace)
			namespace = envNamespace
		} else {
			log.Debug("Namespace flag was default in plugin mode, but HELM_NAMESPACE env var is also empty. Using default.")
		}
	}
	// --- End Helm Plugin Namespace Correction ---

	return releaseName, namespace, nil
}

// writeOutputFile handles writing content to a file with proper error handling and directory creation
func writeOutputFile(outputFile string, content []byte, successMessage string) error {
	// Check if file exists
	exists, err := afero.Exists(AppFs, outputFile)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to check if output file exists: %w", err),
		}
	}
	if exists {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("output file '%s' already exists", outputFile),
		}
	}

	// Create the directory if it doesn't exist
	err = AppFs.MkdirAll(filepath.Dir(outputFile), fileutil.ReadWriteExecuteUserReadExecuteOthers)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to create output directory: %w", err),
		}
	}

	// Write the file
	err = afero.WriteFile(AppFs, outputFile, content, fileutil.ReadWriteUserReadOthers)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to write output file: %w", err),
		}
	}

	// Log success message if provided
	if successMessage != "" {
		log.Info("Output file written", "file", outputFile)
	}

	return nil
}
