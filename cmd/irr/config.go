// Package main implements the command-line interface for the irr (Image Relocation and Rewrite) tool.
package main

import (
	"fmt"
	"os"

	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

var (
	// Config command flags
	configSource     string
	configTarget     string
	configFile       string
	configListOnly   bool
	configRemoveOnly bool
)

func init() {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Configure registry mappings",
		Long: `Configure registry mappings for image redirects.
This command allows you to view, add, update, or remove registry mappings.
Mappings are stored in a YAML file and used by the 'override' command.

IMPORTANT NOTES:
- The 'override' and 'validate' commands can run without a config file,
  but image redirection correctness depends on your configuration.
- When using Harbor as a pull-through cache, ensure your target paths
  match your Harbor project configuration.
- For best results, first use 'irr inspect --generate-config-skeleton'
  to create a base config with detected registries.`,
		Example: `  # Add or update a mapping
  irr config --source quay.io --target harbor.local/quay

  # List all configured mappings
  irr config --list

  # Remove a mapping
  irr config --source quay.io --remove

  # Specify a custom config file
  irr config --file ./my-mappings.yaml --source docker.io --target registry.local/docker

  # Workflow example
  irr inspect --chart-path ./my-chart --generate-config-skeleton
  irr config --source docker.io --target registry.example.com/docker
  irr override --chart-path ./my-chart --target-registry registry.example.com --source-registries docker.io`,
		RunE: configCmdRun,
	}

	// Add config-specific flags
	configCmd.Flags().StringVar(&configSource, "source", "", "Source registry to map from (e.g., docker.io, quay.io)")
	configCmd.Flags().StringVar(&configTarget, "target", "", "Target registry to map to (e.g., harbor.example.com/docker)")
	configCmd.Flags().StringVar(&configFile, "file", "registry-mappings.yaml", "Path to the registry mappings file (default \"registry-mappings.yaml\")")
	configCmd.Flags().BoolVar(&configListOnly, "list", false, "List all configured mappings")
	configCmd.Flags().BoolVar(&configRemoveOnly, "remove", false, "Remove the specified source mapping")

	// Add to root command
	rootCmd.AddCommand(configCmd)
}

// configCmdRun executes the config command
func configCmdRun(_ *cobra.Command, _ []string) error {
	// Check command options
	if configListOnly {
		return listMappings()
	}

	if configRemoveOnly {
		if configSource == "" {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitMissingRequiredFlag,
				Err:  fmt.Errorf("--source is required when using --remove"),
			}
		}
		return removeMapping()
	}

	// For add/update, both source and target are required
	if configSource == "" || configTarget == "" {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitMissingRequiredFlag,
			Err:  fmt.Errorf("both --source and --target are required"),
		}
	}

	return addUpdateMapping()
}

// listMappings lists all configured registry mappings
func listMappings() error {
	mappings, err := registry.LoadMappings(AppFs, configFile, integrationTestMode)
	if err != nil {
		// If file doesn't exist, report empty mappings
		if os.IsNotExist(err) {
			log.Info("No mappings found (file does not exist)", "file", configFile)
			return nil
		}
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to load mappings from '%s': %w", configFile, err),
		}
	}

	// Check if no mappings exist
	if mappings == nil || len(mappings.Entries) == 0 {
		log.Info("No mappings configured", "file", configFile)
		return nil
	}

	// Display mappings
	log.Info("Registry mappings", "file", configFile)
	for _, mapping := range mappings.Entries {
		log.Info("Mapping", "source", mapping.Source, "target", mapping.Target)
	}

	return nil
}

// removeMapping removes a mapping for the specified source registry
func removeMapping() error {
	// Load existing mappings
	mappings, err := registry.LoadMappings(AppFs, configFile, integrationTestMode)
	if err != nil {
		// If file doesn't exist, report error
		if os.IsNotExist(err) {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("mappings file '%s' does not exist", configFile),
			}
		}
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to load mappings from '%s': %w", configFile, err),
		}
	}

	// Check if no mappings exist
	if mappings == nil || len(mappings.Entries) == 0 {
		log.Info("No mappings found to remove", "file", configFile)
		return nil
	}

	// Remove the mapping if it exists
	found := false
	newEntries := make([]registry.Mapping, 0, len(mappings.Entries))
	for _, mapping := range mappings.Entries {
		if mapping.Source != configSource {
			newEntries = append(newEntries, mapping)
		} else {
			found = true
		}
	}

	// If mapping wasn't found, report it
	if !found {
		log.Info("No mapping found for source", "source", configSource, "file", configFile)
		return nil
	}

	// Update mappings
	mappings.Entries = newEntries

	// Save updated mappings
	if err := saveMappings(mappings); err != nil {
		return err
	}

	log.Info("Successfully removed mapping", "source", configSource, "file", configFile)
	return nil
}

// addUpdateMapping adds or updates a mapping
func addUpdateMapping() error {
	// Load existing mappings if file exists
	var mappings *registry.Mappings
	var err error

	exists, err := afero.Exists(AppFs, configFile)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to check if file '%s' exists: %w", configFile, err),
		}
	}

	if exists {
		mappings, err = registry.LoadMappings(AppFs, configFile, integrationTestMode)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to load mappings from '%s': %w", configFile, err),
			}
		}
	}

	// Initialize new mappings if none exist
	if mappings == nil {
		mappings = &registry.Mappings{
			Entries: make([]registry.Mapping, 0),
		}
	}

	// Check if this source already exists
	found := false
	for i, mapping := range mappings.Entries {
		if mapping.Source == configSource {
			// Update existing mapping
			mappings.Entries[i].Target = configTarget
			found = true
			break
		}
	}

	// Add new mapping if not found
	if !found {
		mappings.Entries = append(mappings.Entries, registry.Mapping{
			Source: configSource,
			Target: configTarget,
		})
	}

	// Save updated mappings
	if err := saveMappings(mappings); err != nil {
		return err
	}

	action := "Updated"
	if !found {
		action = "Added"
	}
	log.Info("Mapping action", "action", action, "source", configSource, "target", configTarget, "file", configFile)
	return nil
}

// saveMappings saves the mappings to the specified file
func saveMappings(mappings *registry.Mappings) error {
	// Create file structure for saving
	fileStruct := struct {
		Mappings []registry.Mapping `yaml:"mappings"`
	}{
		Mappings: mappings.Entries,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(fileStruct)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal mappings to YAML: %w", err),
		}
	}

	// Write to file
	log.Debug("Writing mappings to file", "file", configFile)
	err = afero.WriteFile(AppFs, configFile, data, fileutil.ReadWriteUserReadOthers)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to write mappings to '%s': %w", configFile, err),
		}
	}

	return nil
}
