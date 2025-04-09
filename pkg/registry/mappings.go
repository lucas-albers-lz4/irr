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

	// Check if path is a directory
	fileInfo, err := os.Stat(path)
	if err == nil && fileInfo.IsDir() {
		return nil, fmt.Errorf("failed to read mappings file '%s': is a directory", path)
	}

	// Check file extension
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
	// Try both formats: map[string]string and Mappings struct
	var mappings Mappings

	// Try the new format first
	if err := yaml.Unmarshal(data, &mappings); err != nil {
		// Try the old format (map[string]string)
		var rawMappings map[string]string
		if err2 := yaml.Unmarshal(data, &rawMappings); err2 != nil {
			// Neither format worked
			return nil, WrapMappingFileParse(path, err)
		}
		// Convert the map into the expected []Mapping slice
		mappings.Entries = make([]Mapping, 0, len(rawMappings))
		for source, target := range rawMappings {
			mappings.Entries = append(mappings.Entries, Mapping{
				Source: strings.TrimSpace(source),
				Target: strings.TrimSpace(target),
			})
		}
	}

	// Trim whitespace from all entries
	for i := range mappings.Entries {
		mappings.Entries[i].Source = strings.TrimSpace(mappings.Entries[i].Source)
		mappings.Entries[i].Target = strings.TrimSpace(mappings.Entries[i].Target)
	}

	// Check if we have any mappings
	if len(mappings.Entries) == 0 {
		return nil, WrapMappingFileEmpty(path)
	}

	debug.Printf("LoadMappings: Successfully loaded and trimmed %d mappings from %s", len(mappings.Entries), path)
	return &mappings, nil
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

		// Normalize the *mapping* source for comparison against the already normalized input
		normalizedMappingSource := image.NormalizeRegistry(cleanedMappingSource)
		debug.Printf("GetTargetRegistry Loop: Comparing normalizedInput (%q) == normalizedMapping (%q)",
			normalizedSourceInput, normalizedMappingSource)

		// Special handling for docker.io variants
		if normalizedSourceInput == normalizedMappingSource ||
			(normalizedMappingSource == "docker.io" && strings.HasPrefix(source, "index.docker.io/")) {
			debug.Printf("GetTargetRegistry: Match found for '%s'! Returning target: '%s'", source, mapping.Target)
			return strings.TrimSpace(mapping.Target)
		}
	}

	debug.Printf("GetTargetRegistry: No match found for source '%s'.", source)
	return ""
}
