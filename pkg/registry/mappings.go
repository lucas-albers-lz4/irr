// Package registry provides functionality for mapping container registry names.
package registry // Updated package name

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"sigs.k8s.io/yaml"
)

// Mapping represents a single source to target registry mapping
type Mapping struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// Mappings holds a collection of registry mappings
type Mappings struct {
	Entries []Mapping `yaml:"mappings"`
}

// LoadMappings loads registry mappings from a YAML file
func LoadMappings(path string) (*Mappings, error) { // Updated return type
	if path == "" {
		// Returning nil, nil is intentional when path is empty (no mappings, no error).
		return nil, nil //nolint:nilnil // Intentional: Empty path means no mappings loaded, not an error.
	}

	// Basic validation to prevent path traversal
	absPath, err := filepath.Abs(path)
	if err != nil {
		// TODO: Consider creating a specific wrapped error in errors.go
		return nil, fmt.Errorf("failed to get absolute path for mappings file '%s': %w", path, err)
	}
	wd, err := os.Getwd()
	if err != nil {
		// TODO: Consider creating a specific wrapped error in errors.go
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	// Skip CWD check during testing
	if os.Getenv("IRR_TESTING") != "true" {
		if !strings.HasPrefix(absPath, wd) {
			// Use canonical error from pkg/registry/errors.go
			return nil, WrapMappingPathNotInWD(path)
		}
	}
	if !strings.HasSuffix(absPath, ".yaml") && !strings.HasSuffix(absPath, ".yml") {
		// Use canonical error from pkg/registry/errors.go
		return nil, WrapMappingExtension(path)
	}

	// Read the file content
	// #nosec G304 -- Path is validated above and provided by user input.
	data, err := os.ReadFile(path) // G304 mitigation: path validated above
	if err != nil {
		if os.IsNotExist(err) {
			// Use canonical error from pkg/registry/errors.go
			return nil, WrapMappingFileNotExist(path, err)
		}
		// Use canonical error from pkg/registry/errors.go
		return nil, WrapMappingFileRead(path, err)
	}

	// Check for empty file content
	if len(data) == 0 {
		// Use canonical error (assuming WrapMappingFileEmpty exists or needs creation)
		return nil, WrapMappingFileEmpty(path)
	}

	// --- PARSING LOGIC adopted from previous implementation ---
	// Unmarshal into a temporary map first, as the input format is map[string]string
	var rawMappings map[string]string
	if err := yaml.Unmarshal(data, &rawMappings); err != nil {
		// Use canonical error from pkg/registry/errors.go
		return nil, WrapMappingFileParse(path, err)
	}

	// Convert the map into the expected []Mapping slice
	finalMappings := &Mappings{ // Updated type
		Entries: make([]Mapping, 0, len(rawMappings)), // Updated type
	}

	for source, target := range rawMappings {
		trimmedSource := strings.TrimSpace(source)
		trimmedTarget := strings.TrimSpace(target)
		finalMappings.Entries = append(finalMappings.Entries, Mapping{ // Updated type
			Source: trimmedSource,
			Target: trimmedTarget,
		})
		debug.Printf("LoadMappings: Parsed and trimmed mapping: Source='%s', Target='%s'", trimmedSource, trimmedTarget)
	}
	// --- END PARSING LOGIC ---

	debug.Printf("LoadMappings: Successfully loaded and trimmed %d mappings from %s", len(finalMappings.Entries), path)
	return finalMappings, nil
}

// GetTargetRegistry returns the target registry for a given source registry
func (m *Mappings) GetTargetRegistry(source string) string { // Updated receiver type
	debug.Printf("GetTargetRegistry: Looking for source '%s' in mappings: %+v", source, m)
	if m == nil || m.Entries == nil {
		debug.Printf("GetTargetRegistry: Mappings are nil or empty.")
		return ""
	}
	normalizedSourceInput := image.NormalizeRegistry(source)
	debug.Printf("GetTargetRegistry: Normalized source INPUT: '%s'", normalizedSourceInput)

	for _, mapping := range m.Entries {
		// Explicitly trim \r from the mapping source
		cleanedMappingSource := strings.TrimRight(mapping.Source, "\r")

		// --- SIMPLIFIED COMPARISON ---
		// Normalize the *mapping* source for comparison against the already normalized input
		normalizedMappingSource := image.NormalizeRegistry(cleanedMappingSource)
		debug.Printf("GetTargetRegistry Loop: Comparing normalizedInput (%q) == normalizedMapping (%q)",
			normalizedSourceInput, normalizedMappingSource)

		if normalizedSourceInput == normalizedMappingSource {
			debug.Printf("GetTargetRegistry: Match found for '%s'! Returning target: '%s'", source, mapping.Target)
			return mapping.Target
		}
	}

	debug.Printf("GetTargetRegistry: No match found for source '%s'.", source)
	return ""
}
