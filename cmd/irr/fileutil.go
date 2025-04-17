package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/lalbers/irr/internal/helm"
	"github.com/lalbers/irr/pkg/exitcodes"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// getReleaseNameAndNamespaceCommon extracts and validates release name and namespace
func getReleaseNameAndNamespaceCommon(cmd *cobra.Command, args []string) (releaseName, namespace string, err error) {
	releaseName, err = cmd.Flags().GetString("release-name")
	if err != nil {
		return "", "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get release-name flag: %w", err),
		}
	}

	// Check for positional argument as release name if flag is not set and we're running as a plugin
	if releaseName == "" && isHelmPlugin && len(args) > 0 {
		releaseName = args[0]
		log.Infof("Using %s as release name from positional argument", releaseName)
	}

	namespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return "", "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("failed to get namespace flag: %w", err),
		}
	}

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
	err = AppFs.MkdirAll(filepath.Dir(outputFile), DirPermissions)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to create output directory: %w", err),
		}
	}

	// Write the file
	err = afero.WriteFile(AppFs, outputFile, content, FilePermissions)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to write output file: %w", err),
		}
	}

	// Log success message if provided
	if successMessage != "" {
		log.Infof(successMessage, outputFile)
	}

	return nil
}

// createHelmAdapter creates a new Helm client and adapter, handling errors consistently
func createHelmAdapter() (*helm.Adapter, error) {
	// Create a new Helm client
	helmClient, err := helm.NewHelmClient()
	if err != nil {
		return nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to initialize Helm client: %w", err),
		}
	}

	// Create adapter with the Helm client
	adapter := helm.NewAdapter(helmClient, AppFs, isHelmPlugin)
	return adapter, nil
}

// getCommandContext gets the context from a command or creates a background context if none exists
func getCommandContext(cmd *cobra.Command) context.Context {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx
}
