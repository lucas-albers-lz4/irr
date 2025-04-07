// Package registrymapping provides functionality for mapping container registry names.
package registrymapping

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
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

	// --- UPDATED PARSING LOGIC ---
	// Unmarshal into a temporary map first, as the input format is map[string]string
	var rawMappings map[string]string
	if err := yaml.Unmarshal(data, &rawMappings); err != nil {
		return nil, fmt.Errorf("failed to parse mappings file as map[string]string: %v", err)
	}

	// Convert the map into the expected []RegistryMapping slice
	finalMappings := &RegistryMappings{
		Mappings: make([]RegistryMapping, 0, len(rawMappings)),
	}

	for source, target := range rawMappings {
		trimmedSource := strings.TrimSpace(source)
		trimmedTarget := strings.TrimSpace(target)
		finalMappings.Mappings = append(finalMappings.Mappings, RegistryMapping{
			Source: trimmedSource,
			Target: trimmedTarget,
		})
		debug.Printf("LoadMappings: Parsed and trimmed mapping: Source='%s', Target='%s'", trimmedSource, trimmedTarget)
	}
	// --- END UPDATED PARSING LOGIC ---

	debug.Printf("LoadMappings: Successfully loaded and trimmed %d mappings from %s", len(finalMappings.Mappings), path)
	return finalMappings, nil
}

// GetTargetRegistry returns the target registry for a given source registry
func (rm *RegistryMappings) GetTargetRegistry(source string) string {
	debug.Printf("GetTargetRegistry: Looking for source '%s' in mappings: %+v", source, rm)
	if rm == nil || rm.Mappings == nil {
		debug.Printf("GetTargetRegistry: Mappings are nil or empty.")
		return ""
	}
	normalizedSourceInput := image.NormalizeRegistry(source)
	debug.Printf("GetTargetRegistry: Normalized source INPUT: '%s'", normalizedSourceInput)

	for _, mapping := range rm.Mappings {
		// Explicitly trim \r from the mapping source
		cleanedMappingSource := strings.TrimRight(mapping.Source, "\r")

		// --- SIMPLIFIED COMPARISON ---
		// Normalize the *mapping* source for comparison against the already normalized input
		normalizedMappingSource := image.NormalizeRegistry(cleanedMappingSource)
		debug.Printf("GetTargetRegistry Loop: Comparing normalizedInput (%q) == normalizedMapping (%q)", normalizedSourceInput, normalizedMappingSource)

		if normalizedSourceInput == normalizedMappingSource {
			debug.Printf("GetTargetRegistry: Match found for '%s'! Returning target: '%s'", source, mapping.Target)
			return mapping.Target
		}
	}

	debug.Printf("GetTargetRegistry: No match found for source '%s'.", source)
	return ""
}
