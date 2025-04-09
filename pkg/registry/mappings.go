// Package registry provides functionality for mapping container registry names.
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/spf13/afero"
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

// LoadMappings loads registry mappings from a YAML file using the provided filesystem.
func LoadMappings(fs afero.Fs, path string) (*Mappings, error) {
	if path == "" {
		return nil, nil //nolint:nilnil // Intentional: Empty path means no mappings loaded, not an error.
	}

	// Basic validation to prevent path traversal
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for mappings file '%s': %w", path, err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Only skip path traversal check if explicitly allowed in test
	if os.Getenv("IRR_ALLOW_PATH_TRAVERSAL") != "true" {
		if !strings.HasPrefix(absPath, wd) {
			debug.Printf("Path traversal detected. Path: %s, WorkDir: %s", absPath, wd)
			return nil, WrapMappingPathNotInWD(path)
		}
	}

	// Check if path is a directory using the provided filesystem
	fileInfo, err := fs.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, WrapMappingFileNotExist(path, err)
		}
		return nil, WrapMappingFileRead(path, err)
	}
	if fileInfo.IsDir() {
		return nil, fmt.Errorf("failed to read mappings file '%s': is a directory", path)
	}

	// Check file extension
	if !strings.HasSuffix(absPath, ".yaml") && !strings.HasSuffix(absPath, ".yml") {
		return nil, WrapMappingExtension(path)
	}

	// Read the file content using the provided filesystem
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, WrapMappingFileNotExist(path, err)
		}
		return nil, WrapMappingFileRead(path, err)
	}

	// Check for empty file content
	if len(data) == 0 {
		return nil, WrapMappingFileEmpty(path)
	}

	debug.Printf("LoadMappings: Attempting to parse file content:\n%s", string(data))

	// Try both formats: map[string]string and Mappings struct
	var mappings Mappings

	// Try the new format first (with mappings key)
	var newFormat struct {
		Mappings []Mapping `yaml:"mappings"`
	}
	if err := yaml.Unmarshal(data, &newFormat); err == nil && len(newFormat.Mappings) > 0 {
		debug.Printf("LoadMappings: Successfully parsed new format, found %d entries", len(newFormat.Mappings))
		mappings.Entries = make([]Mapping, len(newFormat.Mappings))
		for i, m := range newFormat.Mappings {
			mappings.Entries[i] = Mapping{
				Source: strings.TrimSpace(m.Source),
				Target: strings.TrimSpace(m.Target),
			}
		}
		return &mappings, nil
	}
	debug.Printf("LoadMappings: Failed to parse new format or no entries found, trying old format")

	// Try the old format (map[string]string)
	var rawMappings map[string]string
	if err := yaml.Unmarshal(data, &rawMappings); err != nil {
		debug.Printf("LoadMappings: Failed to parse as map[string]string: %v", err)
		return nil, WrapMappingFileParse(path, err)
	}

	// Convert the map into the expected []Mapping slice
	mappings.Entries = make([]Mapping, 0, len(rawMappings))
	for source, target := range rawMappings {
		if source = strings.TrimSpace(source); source == "" {
			continue // Skip empty source keys
		}
		if target = strings.TrimSpace(target); target == "" {
			continue // Skip empty target values
		}
		mappings.Entries = append(mappings.Entries, Mapping{
			Source: source,
			Target: target,
		})
	}

	// Check if we have any mappings
	if len(mappings.Entries) == 0 {
		debug.Printf("LoadMappings: No valid entries found after parsing")
		return nil, WrapMappingFileEmpty(path)
	}

	debug.Printf("LoadMappings: Successfully loaded %d mappings from %s", len(mappings.Entries), path)
	return &mappings, nil
}

// GetTargetRegistry returns the target registry for a given source registry
func (m *Mappings) GetTargetRegistry(source string) string {
	debug.Printf("GetTargetRegistry: Looking for source '%s' in mappings", source)
	if m == nil || m.Entries == nil {
		debug.Printf("GetTargetRegistry: Mappings are nil or empty.")
		return ""
	}

	// Clean and normalize the input source
	source = strings.TrimSpace(source)
	source = strings.TrimRight(source, "\r")
	normalizedSourceInput := image.NormalizeRegistry(source)
	debug.Printf("GetTargetRegistry: Normalized source INPUT: '%s' -> '%s'", source, normalizedSourceInput)

	// Special case: if source starts with index.docker.io, normalize it
	if strings.HasPrefix(source, "index.docker.io/") {
		normalizedSourceInput = "docker.io"
		debug.Printf("GetTargetRegistry: Special case - normalized index.docker.io to docker.io")
	}

	for _, mapping := range m.Entries {
		// Clean and normalize the mapping source
		mappingSource := strings.TrimSpace(mapping.Source)
		mappingSource = strings.TrimRight(mappingSource, "\r")
		normalizedMappingSource := image.NormalizeRegistry(mappingSource)
		debug.Printf("GetTargetRegistry: Comparing normalized input '%s' with normalized mapping '%s'",
			normalizedSourceInput, normalizedMappingSource)

		if normalizedSourceInput == normalizedMappingSource {
			target := strings.TrimSpace(mapping.Target)
			debug.Printf("GetTargetRegistry: Match found! Returning target: '%s'", target)
			return target
		}
	}

	debug.Printf("GetTargetRegistry: No match found for source '%s'", source)
	return ""
}
