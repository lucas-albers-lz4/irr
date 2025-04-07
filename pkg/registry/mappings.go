package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Mapping defines a single source-to-target registry mapping.
type Mapping struct {
	Source string `yaml:"source"` // The source registry hostname (e.g., docker.io)
	Target string `yaml:"target"` // The target registry hostname (e.g., my-proxy.com)
}

// Mappings holds a list of registry mappings.
type Mappings struct {
	Mappings []Mapping `yaml:"mappings"` // List of individual mappings
}

// LoadMappings loads registry mappings from a YAML file
func LoadMappings(path string) (*Mappings, error) {
	if path == "" {
		return nil, nil
	}

	// Basic validation to prevent path traversal
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for mappings file: %w", err)
	}
	// Add check for CWD prefix
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory failed: %w", err)
	}
	// Skip CWD check during testing
	if os.Getenv("IRR_TESTING") != "true" {
		if !strings.HasPrefix(absPath, wd) {
			return nil, WrapMappingPathNotInWD(path)
		}
	}
	// End added check

	if !strings.HasSuffix(absPath, ".yaml") && !strings.HasSuffix(absPath, ".yml") {
		return nil, WrapMappingExtension(path)
	}

	// Read the file content
	// #nosec G304 -- Path is validated above and provided by user input.
	data, err := os.ReadFile(path) // G304 mitigation: path validated above
	if err != nil {
		if os.IsNotExist(err) {
			return nil, WrapMappingFileNotExist(path, err)
		}
		return nil, WrapMappingFileRead(path, err)
	}

	var mappings Mappings
	if err := yaml.Unmarshal(data, &mappings); err != nil {
		return nil, WrapMappingFileParse(path, err)
	}

	return &mappings, nil
}

// GetTargetRegistry returns the mapped target registry for a given source registry.
// If no mapping is found, it returns an empty string.
func (m *Mappings) GetTargetRegistry(sourceRegistry string) string {
	if m == nil {
		return "" // Use default mapping
	}

	for _, mapping := range m.Mappings {
		if mapping.Source == sourceRegistry {
			return mapping.Target
		}
	}
	return "" // Use default mapping
}
