// Package registrymapping provides functionality for mapping container registry names.
package registrymapping

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// RegistryMapping represents a single source to target registry mapping
type RegistryMapping struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// RegistryMappings holds a collection of registry mappings
type RegistryMappings struct {
	Mappings []RegistryMapping `yaml:"mappings"`
}

// LoadMappings loads registry mappings from a YAML file
func LoadMappings(path string) (*RegistryMappings, error) {
	if path == "" {
		return nil, nil
	}

	// Basic validation to prevent path traversal
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for mappings file: %w", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	if !strings.HasPrefix(absPath, wd) {
		return nil, fmt.Errorf("invalid mappings file path: must be within the current working directory tree")
	}
	if !strings.HasSuffix(absPath, ".yaml") && !strings.HasSuffix(absPath, ".yml") {
		return nil, fmt.Errorf("invalid mappings file path: must end with .yaml or .yml")
	}

	// Read the file content
	// #nosec G304 -- Path is validated above and provided by user input.
	data, err := os.ReadFile(path) // G304 mitigation: path validated above
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("mappings file does not exist: %v", err)
		}
		return nil, fmt.Errorf("failed to read mappings file: %v", err)
	}

	var mappings RegistryMappings
	if err := yaml.Unmarshal(data, &mappings); err != nil {
		return nil, fmt.Errorf("failed to parse mappings file: %v", err)
	}

	return &mappings, nil
}

// GetTargetRegistry returns the target registry for a given source registry
func (m *RegistryMappings) GetTargetRegistry(source string) string {
	if m == nil {
		return "" // Use default mapping
	}

	for _, mapping := range m.Mappings {
		if mapping.Source == source {
			return mapping.Target
		}
	}
	return "" // Use default mapping
}
