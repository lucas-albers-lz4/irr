// Package main implements the command-line interface for the irr (Image Relocation and Rewrite) tool.
package main

import (
	"fmt"

	"errors"

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
		// If file doesn't exist, log info and return nil (not an error for list)
		var notExistErr *registry.ErrMappingFileNotExist
		if errors.As(err, &notExistErr) {
			log.Info("No mappings found (file does not exist)", "file", configFile)
			return nil // Explicitly return nil, matching test expectation
		}
		// For other errors during loading, return them wrapped
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to load mappings from '%s': %w", configFile, err),
		}
	}

	// Check if no mappings exist after successful load
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
	// Need to handle both structured and legacy loading, similar to addUpdateMapping
	var mappings *registry.Mappings
	var err error
	var loadedConfig *registry.Config // Store the loaded structured config

	exists, err := afero.Exists(AppFs, configFile)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to check if file '%s' exists: %w", configFile, err),
		}
	}

	if exists {
		loadedConfig, err = registry.LoadStructuredConfig(AppFs, configFile, integrationTestMode)
		if err == nil {
			mappings = loadedConfig.ToMappings()
			log.Debug("Loaded existing config in structured format for removal")
		} else {
			log.Debug("Failed to load structured for removal, trying legacy/fallback", "error", err)
			mappings, err = registry.LoadMappings(AppFs, configFile, integrationTestMode)
			if err != nil {
				var notExistErr *registry.ErrMappingFileNotExist
				if errors.As(err, &notExistErr) {
					log.Info("Mappings file does not exist, nothing to remove", "file", configFile)
					return nil // Not an error if file doesn't exist
				}
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("failed to load mappings from '%s': %w", configFile, err),
				}
			}
			log.Debug("Loaded existing config using legacy/fallback LoadMappings for removal")
			loadedConfig = nil // Ensure we use default structure if loaded via legacy
		}
	} else {
		// File doesn't exist
		log.Info("Mappings file does not exist, nothing to remove", "file", configFile)
		return nil
	}

	// Check if no mappings exist after successful load
	if mappings == nil || len(mappings.Entries) == 0 {
		log.Info("No mappings found to remove", "file", configFile)
		return nil
	}

	// Remove the mapping if it exists
	found := false
	needsSave := false // Track if removal actually happened
	newEntries := make([]registry.Mapping, 0, len(mappings.Entries))
	for _, mapping := range mappings.Entries {
		if mapping.Source != configSource {
			newEntries = append(newEntries, mapping)
		} else {
			found = true
			needsSave = true // Mark that a change occurred
		}
	}

	// If mapping wasn't found, report it and exit (no save needed)
	if !found {
		log.Info("No mapping found for source", "source", configSource, "file", configFile)
		return nil
	}

	// Update mappings structure
	mappings.Entries = newEntries

	// Save updated mappings only if an item was actually removed
	if needsSave {
		log.Debug("Mapping removed, saving updated config...")
		if err := saveMappings(mappings, loadedConfig); err != nil { // Pass loadedConfig
			return err
		}
		log.Info("Successfully removed mapping", "source", configSource, "file", configFile)
	} else {
		// This branch should technically be unreachable due to the !found check above
		log.Warn("Internal state inconsistency: needsSave is false after finding mapping to remove", "source", configSource)
	}

	return nil
}

// addUpdateMapping adds or updates a mapping
func addUpdateMapping() error {
	// Load existing mappings if file exists
	var mappings *registry.Mappings
	var err error
	var loadedConfig *registry.Config // Store the loaded structured config if applicable

	exists, err := afero.Exists(AppFs, configFile)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to check if file '%s' exists: %w", configFile, err),
		}
	}

	if exists {
		// Try loading structured first
		loadedConfig, err = registry.LoadStructuredConfig(AppFs, configFile, integrationTestMode)
		if err == nil {
			// Convert structured to Mappings object for internal processing
			mappings = loadedConfig.ToMappings()
			log.Debug("Loaded existing config in structured format")
		} else {
			// If structured failed, try legacy Mappings (LoadMappings handles fallback internally)
			log.Debug("Failed to load as structured, trying legacy/fallback LoadMappings", "error", err)
			mappings, err = registry.LoadMappings(AppFs, configFile, integrationTestMode)
			if err != nil {
				// If both fail, return the error
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("failed to load mappings from '%s': %w", configFile, err),
				}
			}
			log.Debug("Loaded existing config using legacy/fallback LoadMappings")
			// If loaded via legacy, ensure loadedConfig is nil so save logic uses a fresh structure
			loadedConfig = nil
		}
	}

	// Initialize new mappings if none exist or file didn't exist
	if mappings == nil {
		mappings = &registry.Mappings{
			Entries: make([]registry.Mapping, 0),
		}
		log.Debug("Initialized new empty mappings structure")
	}

	// Check if this source already exists and if target needs update
	found := false
	needsSave := false                 // Flag to track if a change occurred
	updatedTargetValue := configTarget // Target value from flag

	for i, mapping := range mappings.Entries {
		if mapping.Source == configSource {
			found = true
			// Check if the target value actually needs changing
			if mapping.Target != updatedTargetValue {
				log.Debug("Updating target for existing source", "source", configSource, "old_target", mapping.Target, "new_target", updatedTargetValue)
				mappings.Entries[i].Target = updatedTargetValue
				needsSave = true // Mark that a change occurred
			} else {
				log.Debug("Source found, but target already matches. No update needed.", "source", configSource, "target", updatedTargetValue)
			}
			break
		}
	}

	// Add new mapping if not found
	if !found {
		log.Debug("Adding new mapping", "source", configSource, "target", updatedTargetValue)
		mappings.Entries = append(mappings.Entries, registry.Mapping{
			Source: configSource,
			Target: updatedTargetValue,
		})
		needsSave = true // Mark that a change occurred
	}

	// Save updated mappings ONLY if a change was made
	if needsSave {
		log.Debug("Change detected, saving mappings...")
		if err := saveMappings(mappings, loadedConfig); err != nil { // Pass loadedConfig to preserve structure
			return err
		}
		log.Info("Successfully saved changes to mappings", "file", configFile)
	} else {
		log.Info("No changes needed for mapping", "source", configSource, "target", updatedTargetValue, "file", configFile)
	}

	// Original log message (can be removed or kept depending on desired output)
	// action := "Updated"
	// if !found {
	// 	action = "Added"
	// }
	// log.Info("Mapping action", "action", action, "source", configSource, "target", configTarget, "file", configFile)

	return nil
}

// saveMappings saves the mappings to the specified file
// It now accepts the originally loaded config (if any) to preserve structure
func saveMappings(mappings *registry.Mappings, originalConfig *registry.Config) error {
	// Construct the full Config structure expected by the loader
	// If we loaded a structured config, use it as the base to preserve other fields
	config := registry.Config{}
	if originalConfig != nil {
		config = *originalConfig // Start with a copy of the original
		log.Debug("saveMappings: Using original loaded structured config as base")
	} else {
		// If no original config (new file or legacy load), create a default structure
		config.Version = "1.0" // Add a default version
		// Initialize nested structs if nil
		config.Registries = registry.RegConfig{}
		config.Compatibility = registry.CompatibilityConfig{}
		log.Debug("saveMappings: Creating default structured config base")
	}

	// Update the Mappings slice within the config struct
	config.Registries.Mappings = make([]registry.RegMapping, len(mappings.Entries))
	for i, entry := range mappings.Entries {
		// Preserve existing RegMapping fields if possible (like Description, Enabled)
		// This requires finding the corresponding mapping in originalConfig if it exists.
		var existingRegMapping *registry.RegMapping
		if originalConfig != nil {
			for j := range originalConfig.Registries.Mappings {
				if originalConfig.Registries.Mappings[j].Source == entry.Source {
					existingRegMapping = &originalConfig.Registries.Mappings[j]
					break
				}
			}
		}

		config.Registries.Mappings[i] = registry.RegMapping{
			Source: entry.Source,
			Target: entry.Target,
			// Preserve Enabled/Description from original if found, otherwise default
			Enabled:     true, // Default to true
			Description: "",   // Default to empty
		}
		if existingRegMapping != nil {
			config.Registries.Mappings[i].Enabled = existingRegMapping.Enabled
			config.Registries.Mappings[i].Description = existingRegMapping.Description
			log.Debug("Preserving existing details", "source", entry.Source, "enabled", existingRegMapping.Enabled, "desc", existingRegMapping.Description)
		}
	}

	// Marshal the full Config structure to YAML
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("failed to marshal config to YAML: %w", err),
		}
	}

	// Write file
	if err := afero.WriteFile(AppFs, configFile, yamlData, fileutil.ReadWriteUserPermission); err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitIOError,
			Err:  fmt.Errorf("failed to write mappings to file '%s': %w", configFile, err),
		}
	}
	return nil
}
