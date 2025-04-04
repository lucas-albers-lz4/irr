package registry

import (
	"fmt"
	"os"

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

	data, err := os.ReadFile(path)
	if err != nil {
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
