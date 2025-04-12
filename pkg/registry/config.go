// Package registry provides functionality for mapping container registry names.
package registry

import (
	"fmt"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/spf13/afero"
	"sigs.k8s.io/yaml"
)

// Config represents the top-level structure for the new structured config YAML format.
type Config struct {
	// Registries contains the registry mappings configuration
	Registries RegConfig `yaml:"registries"`
	// Version of the config format (for future compatibility)
	Version string `yaml:"version,omitempty"`
	// Compatibility flags for handling special cases
	Compatibility CompatibilityConfig `yaml:"compatibility,omitempty"`
}

// RegConfig holds registry-specific configuration
type RegConfig struct {
	// Mappings contains the source to target registry mappings
	Mappings []RegMapping `yaml:"mappings"`
	// DefaultTarget is the default target registry if no specific mapping is found
	DefaultTarget string `yaml:"defaultTarget,omitempty"`
	// StrictMode determines if unknown registries should fail (true) or use the default (false)
	StrictMode bool `yaml:"strictMode,omitempty"`
}

// RegMapping represents a single source to target registry mapping with additional metadata
type RegMapping struct {
	// Source is the source registry to be mapped (e.g., docker.io, quay.io)
	Source string `yaml:"source"`
	// Target is the target registry to map to (e.g., harbor.example.com/docker)
	Target string `yaml:"target"`
	// Description provides optional documentation about this mapping
	Description string `yaml:"description,omitempty"`
	// Enabled determines if this mapping is active (default: true)
	Enabled bool `yaml:"enabled,omitempty"`
}

// CompatibilityConfig holds compatibility flags for special handling
type CompatibilityConfig struct {
	// LegacyFlatFormat if true allows falling back to the old flat map[string]string format
	LegacyFlatFormat bool `yaml:"legacyFlatFormat,omitempty"`
	// IgnoreEmptyFields if true ignores empty fields in the structured format
	IgnoreEmptyFields bool `yaml:"ignoreEmptyFields,omitempty"`
}

// LoadStructuredConfig loads registry mappings from a YAML file using the new structured format.
// It returns a Config struct representing the parsed configuration, which can be converted
// to the legacy map[string]string format if needed for backward compatibility.
func LoadStructuredConfig(fs afero.Fs, path string, skipCWDRestriction bool) (*Config, error) {
	// Validate file path
	if err := validateConfigFilePath(fs, path, skipCWDRestriction); err != nil {
		return nil, err
	}

	// Read file content
	data, err := readConfigFileContent(fs, path)
	if err != nil {
		return nil, err
	}

	debug.Printf("LoadStructuredConfig: Attempting to parse file content:\n%s", string(data))

	// Parse the YAML content as structured Config
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		debug.Printf("LoadStructuredConfig: Failed to parse as structured config: %v", err)
		return nil, fmt.Errorf("failed to parse config file '%s' as structured format: %w", path, err)
	}

	// Validate the parsed config
	if err := validateStructuredConfig(&config, path); err != nil {
		return nil, err
	}

	debug.Printf("LoadStructuredConfig: Successfully loaded structured config from %s", path)
	return &config, nil
}

// validateStructuredConfig performs validation on the structured config
func validateStructuredConfig(config *Config, path string) error {
	// Check if registries.mappings is present and not empty
	if len(config.Registries.Mappings) == 0 {
		return WrapMappingFileEmpty(path)
	}

	// Set default for Enabled if not specified
	for i := range config.Registries.Mappings {
		// Default enabled to true if not specified
		if !config.Registries.Mappings[i].Enabled {
			config.Registries.Mappings[i].Enabled = true
		}

		// Validate source and target in each mapping
		source := config.Registries.Mappings[i].Source
		target := config.Registries.Mappings[i].Target

		// Skip disabled mappings
		if !config.Registries.Mappings[i].Enabled {
			continue
		}

		// Validate source (same validation as in LoadConfig)
		if source == "" {
			return fmt.Errorf("empty source registry in mapping at index %d in config file '%s'", i, path)
		}
		if len(source) > MaxKeyLength {
			return WrapKeyTooLong(path, source, len(source), MaxKeyLength)
		}
		if !isValidDomain(source) {
			return fmt.Errorf("invalid source registry domain '%s' in config file '%s'", source, path)
		}

		// Validate target
		if target == "" {
			return fmt.Errorf("empty target registry in mapping for source '%s' in config file '%s'", source, path)
		}
		if err := validateMappingValue(source, target, path); err != nil {
			return err
		}
	}

	// If StrictMode is enabled, DefaultTarget is not required
	// If StrictMode is disabled, DefaultTarget should be set
	if !config.Registries.StrictMode && config.Registries.DefaultTarget == "" {
		debug.Printf("Warning: StrictMode is disabled but DefaultTarget is not set in config file '%s'", path)
	}

	// If DefaultTarget is set, it should be valid
	if config.Registries.DefaultTarget != "" {
		if err := validateMappingValue("default", config.Registries.DefaultTarget, path); err != nil {
			return fmt.Errorf("invalid DefaultTarget in config file '%s': %w", path, err)
		}
	}

	return nil
}

// ConvertToLegacyFormat converts a structured Config to the legacy flat map[string]string format
func ConvertToLegacyFormat(config *Config) map[string]string {
	result := make(map[string]string)

	// Only process enabled mappings
	for _, mapping := range config.Registries.Mappings {
		if mapping.Enabled {
			result[mapping.Source] = mapping.Target
		}
	}

	return result
}

// ToMappings converts a structured Config to the Mappings format
func (c *Config) ToMappings() *Mappings {
	mappings := &Mappings{
		Entries: make([]Mapping, 0, len(c.Registries.Mappings)),
	}

	for _, mapping := range c.Registries.Mappings {
		if mapping.Enabled {
			mappings.Entries = append(mappings.Entries, Mapping{
				Source: mapping.Source,
				Target: mapping.Target,
			})
		}
	}

	return mappings
}
