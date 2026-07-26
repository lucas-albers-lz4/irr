// Package main contains the implementation for the irr CLI, including subcommands like inspect.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
)

// writeOutput writes the analysis to a file or stdout
func writeOutput(cmd *cobra.Command, analysisResult *ImageAnalysis, flags *InspectFlags) error {
	// Handle generate-config-skeleton flag
	if flags.GenerateConfigSkeleton {
		skeletonFile := flags.OutputFile
		if skeletonFile == "" {
			skeletonFile = DefaultConfigSkeletonFilename
		}

		// Check if the skeleton file exists
		exists, err := afero.Exists(AppFs, skeletonFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to check if skeleton file exists: %w", err),
			}
		}

		// If the file exists and overwriteSkeleton is false, return an error
		if exists && !flags.OverwriteSkeleton {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("output file %s already exists; use --overwrite-skeleton to overwrite", skeletonFile),
			}
		}

		// If overwriteSkeleton is true, we'll continue and overwrite the file
		if exists && flags.OverwriteSkeleton {
			log.Info("Overwriting existing skeleton file", "path", skeletonFile)
		}

		if err := createConfigSkeleton(analysisResult.Images, skeletonFile); err != nil {
			// Special handling for file exists error - should not happen now with the checks above
			var exitErr *exitcodes.ExitCodeError
			if errors.As(err, &exitErr) && strings.Contains(exitErr.Err.Error(), "already exists") {
				// This case should not occur now, but kept for robustness
				return &exitcodes.ExitCodeError{
					Code: exitcodes.ExitIOError,
					Err:  fmt.Errorf("output file %s already exists; use --overwrite-skeleton to overwrite", skeletonFile),
				}
			}

			// Other errors from createConfigSkeleton
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to create config skeleton: %w", err),
			}
		}
		return nil
	}

	// Determine output format (yaml or json)
	var output []byte
	var err error

	switch strings.ToLower(flags.OutputFormat) {
	case outputFormatJSON:
		output, err = json.Marshal(analysisResult)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to JSON: %w", err),
			}
		}
	default:
		// Default to YAML
		output, err = yaml.Marshal(analysisResult)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to YAML: %w", err),
			}
		}
	}

	// Write to file or stdout
	if flags.OutputFile != "" {
		if err := afero.WriteFile(AppFs, flags.OutputFile, output, fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write analysis to file: %w", err),
			}
		}
		log.Info("Analysis written to", flags.OutputFile)
	} else {
		// Use the command's out buffer instead of fmt.Println directly
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(output)); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write analysis to stdout: %w", err),
			}
		}
	}

	return nil
}

// extractUniqueRegistries extracts a set of unique registry names from image info
func extractUniqueRegistries(images []ImageInfo) map[string]bool {
	registries := make(map[string]bool)
	for _, img := range images {
		normalized := image.NormalizeRegistry(img.Registry)
		registries[normalized] = true
	}
	return registries
}

// outputRegistryConfigSuggestion prints suggestions for creating a registry mapping file
func outputRegistryConfigSuggestion(chartPath string, registries map[string]bool) {
	log.Info("\nSuggestion: Create a registry mapping file ('registry-mappings.yaml') to define target registries:")
	log.Info("Example structure:")
	log.Info("```yaml")
	log.Info("mappings:")

	uniqueRegistryList := make([]string, 0, len(registries))
	for reg := range registries {
		uniqueRegistryList = append(uniqueRegistryList, reg)
	}
	sort.Strings(uniqueRegistryList) // Sort for consistent output

	for _, reg := range uniqueRegistryList {
		log.Info(fmt.Sprintf("  - source: %s", reg))
		log.Info("    target: your-private-registry.com/path") // Example target
		log.Info("    # strategy: default (optional)")
	}
	log.Info("```")
	log.Info("Then use it with the 'override' command:")
	log.Info(fmt.Sprintf("  irr override --chart-path %s --config registry-mappings.yaml ...", chartPath)) // Recommend --config now
}

// createConfigSkeleton generates a registry mapping config skeleton
func createConfigSkeleton(images []ImageInfo, outputFile string) error {
	// Use default filename if none specified
	if outputFile == "" {
		outputFile = DefaultConfigSkeletonFilename
		log.Info("No output file specified, using default:", outputFile)
	}

	// Note: File existence check is now done in writeOutput function
	// so we don't need to check here

	// Ensure the directory exists before trying to write the file
	dir := filepath.Dir(outputFile)
	if dir != "" && dir != "." {
		if err := AppFs.MkdirAll(dir, fileutil.ReadWriteExecuteUserReadExecuteOthers); err != nil {
			return fmt.Errorf("failed to create directory for config skeleton: %w", err)
		}
	}

	// Extract unique registries from images
	registries := make(map[string]bool)
	for _, img := range images {
		if img.Registry != "" {
			registries[img.Registry] = true
		}
	}

	// Sort registries for consistent output
	var registryList []string
	for registry := range registries {
		registryList = append(registryList, registry)
	}
	sort.Strings(registryList)

	// Create structured registry mappings
	mappings := make([]registry.RegMapping, 0, len(registryList))
	for _, reg := range registryList {
		log.Debug("CREATE_SKELETON: Creating mapping entry", "source_registry_key", reg)
		// Generate a sanitized target registry path
		targetPath := strings.ReplaceAll(reg, ".", "-")
		mappings = append(mappings, registry.RegMapping{
			Source:      reg,
			Target:      "registry.local/" + targetPath,
			Description: fmt.Sprintf("Mapping for %s", reg),
			Enabled:     true,
		})
	}

	// Create config structure using the registry package format
	config := registry.Config{
		Version: registry.DefaultConfigVersion,
		Registries: registry.RegConfig{
			Mappings:      mappings,
			DefaultTarget: "registry.local/default", // Example default target
			StrictMode:    false,                    // Default to false for better usability
		},
		Compatibility: registry.CompatibilityConfig{
			IgnoreEmptyFields: true,
		},
	}

	// Marshal to YAML
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config skeleton: %w", err)
	}

	// Add helpful comments
	yamlWithComments := fmt.Sprintf(`# IRR Configuration File
# 
# This file contains registry mappings for redirecting container images
# from public registries to your private registry. Update the target values
# to match your registry configuration.
#
# USAGE INSTRUCTIONS:
# 1. Update the 'target' fields with your actual registry paths
# 2. Use with 'irr override' command to generate image overrides
# 3. Validate generated overrides with 'irr validate'
#
# IMPORTANT NOTES:
# - This file uses the standard structured format which includes version, registries, 
#   and compatibility sections for enhanced functionality
# - The 'override' and 'validate' commands can run without this config, 
#   but image redirection correctness depends on your configuration
# - When using Harbor as a pull-through cache, ensure your target paths
#   match your Harbor project configuration
# - You can set or update mappings using 'irr config --source <reg> --target <path>'
# - This file was auto-generated from detected registries in your chart
#
%s`, string(configYAML))

	// Write the skeleton file
	err = afero.WriteFile(AppFs, outputFile, []byte(yamlWithComments), fileutil.ReadWriteUserPermission)
	if err != nil {
		return fmt.Errorf("failed to write config skeleton: %w", err)
	}

	absPath, err := filepath.Abs(outputFile)
	if err == nil {
		log.Info("Config skeleton written to", absPath)
	} else {
		log.Info("Config skeleton written to", outputFile)
	}

	log.Info("Update the target registry paths and use with 'irr config' to set up your configuration")
	return nil
}

// isValidRegistryHostname checks if a registry string looks like a valid hostname.
// Parameter renamed to avoid shadowing the 'registry' package.
func isValidRegistryHostname(hostname string) bool {
	// Basic checks: not empty, doesn't contain invalid characters, doesn't start with /
	if hostname == "" || strings.ContainsAny(hostname, " \t\n\r:/@") || strings.HasPrefix(hostname, "/") {
		return false
	}
	// Must contain a dot or a colon
	if !strings.Contains(hostname, ".") && !strings.Contains(hostname, ":") {
		return false
	}
	// Try to parse as IP - if successful, it's NOT a valid hostname registry (unless it has a port)
	if !strings.Contains(hostname, ":") { // Only check for pure IPs if no port is present
		if net.ParseIP(hostname) != nil {
			return false // It's a bare IP address
		}
	}

	// Basic check passed
	return true
}

// outputMultiReleaseAnalysis formats and outputs the analysis results for multiple releases
func outputMultiReleaseAnalysis(cmd *cobra.Command, results []*ReleaseAnalysisResult, skipped []string, flags *InspectFlags) error {
	// Create a combined output structure
	type CombinedAnalysisResult struct {
		Releases []*ReleaseAnalysisResult `json:"releases" yaml:"releases"`
		Skipped  []string                 `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	}

	combinedResult := CombinedAnalysisResult{
		Releases: results,
		Skipped:  skipped,
	}

	// Determine output format (yaml or json)
	var output []byte
	var marshalErr error

	switch strings.ToLower(flags.OutputFormat) {
	case outputFormatJSON:
		output, marshalErr = json.Marshal(combinedResult)
		if marshalErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to JSON: %w", marshalErr),
			}
		}
	default:
		// Default to YAML
		output, marshalErr = yaml.Marshal(combinedResult)
		if marshalErr != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitGeneralRuntimeError,
				Err:  fmt.Errorf("failed to marshal analysis to YAML: %w", marshalErr),
			}
		}
	}

	// Write to file or stdout
	if flags.OutputFile != "" {
		if err := afero.WriteFile(AppFs, flags.OutputFile, output, fileutil.ReadWriteUserPermission); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write analysis to file: %w", err),
			}
		}
		log.Info("Analysis written to", flags.OutputFile)
	} else {
		// Use the command's out buffer instead of fmt.Println directly
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(output)); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to write analysis to stdout: %w", err),
			}
		}
	}

	// Log summary information
	if len(skipped) > 0 {
		log.Warn("Some releases were skipped during analysis:", "count", len(skipped))
		for _, skippedRelease := range skipped {
			log.Warn("  - " + skippedRelease)
		}
	}

	log.Info(fmt.Sprintf("Successfully analyzed %d releases", len(results)))
	return nil
}

// inspectAllNamespaces handles inspection of all Helm releases across all namespaces
func inspectAllNamespaces(cmd *cobra.Command, flags *InspectFlags) error {
	log.Info("Inspecting all Helm releases across all namespaces...")

	// Get all releases
	releases, helmAdapter, err := getAllReleases()
	if err != nil {
		return err
	}

	// Process all releases
	results, skippedReleases, skeletonImages, err := processAllReleases(releases, helmAdapter, flags)
	if err != nil && !flags.GenerateConfigSkeleton {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartProcessingFailed,
			Err:  err,
		}
	}

	// Handle skeleton generation
	if flags.GenerateConfigSkeleton {
		log.Info("Generating config skeleton from all releases...")

		// If we have no images but we're in skeleton mode, return an error
		if len(skeletonImages) == 0 {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartProcessingFailed,
				Err:  errors.New("no registries found for skeleton generation"),
			}
		}

		// Generate skeleton file
		skeletonFile := flags.OutputFile
		if skeletonFile == "" {
			skeletonFile = DefaultConfigSkeletonFilename
		}

		// Check if the skeleton file exists
		exists, err := afero.Exists(AppFs, skeletonFile)
		if err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to check if skeleton file exists: %w", err),
			}
		}

		// If the file exists and overwriteSkeleton is false, return an error
		if exists && !flags.OverwriteSkeleton {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("output file %s already exists; use --overwrite-skeleton to overwrite", skeletonFile),
			}
		}

		if err := createConfigSkeleton(skeletonImages, skeletonFile); err != nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitIOError,
				Err:  fmt.Errorf("failed to create config skeleton: %w", err),
			}
		}

		log.Info("Config skeleton generated successfully", "file", skeletonFile)
		return nil
	}

	// Output analysis results
	return outputMultiReleaseAnalysis(cmd, results, skippedReleases, flags)
}
