// Package registry provides functionality for mapping container registry names.
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"errors"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/spf13/afero"
	"sigs.k8s.io/yaml"
)

const (
	// DefaultFilePermissions defines the permission mode for new files using modern octal literal style.
	DefaultFilePermissions = 0o644
	// MinDomainPartsForWildcard defines the minimum parts for a valid wildcard domain.
	MinDomainPartsForWildcard = 2
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

// ErrNoConfigSpecified indicates that no configuration file path was provided.
var ErrNoConfigSpecified = errors.New("no configuration file specified")

// LoadMappings loads registry mappings from a YAML file using the provided filesystem.
// skipCWDRestriction allows bypassing the check that the path must be within the CWD tree.
func LoadMappings(fs afero.Fs, path string, skipCWDRestriction bool) (*Mappings, error) {
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

	// Only skip path traversal check if explicitly allowed in test or via parameter
	if !skipCWDRestriction {
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

// LoadConfig loads registry mappings from a YAML file specified by the --config flag.
// It enforces strict validation on the format and content of the file:
// - The file must exist and be readable
// - The content must be a valid YAML map[string]string
// - Keys must be valid domain names
// - Values must contain at least one slash (registry/path format)
func LoadConfig(fs afero.Fs, path string, skipCWDRestriction bool) (map[string]string, error) {
	if path == "" {
		// No path provided, return the specific sentinel error
		return nil, ErrNoConfigSpecified
	}

	// Basic validation to prevent path traversal
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for config file '%s': %w", path, err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Only skip path traversal check if explicitly allowed in test or via parameter
	if !skipCWDRestriction {
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
		return nil, fmt.Errorf("failed to read config file '%s': is a directory", path)
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

	debug.Printf("LoadConfig: Attempting to parse file content:\n%s", string(data))

	// Parse the YAML content as map[string]string
	var config map[string]string
	if err := yaml.Unmarshal(data, &config); err != nil {
		debug.Printf("LoadConfig: Failed to parse as map[string]string: %v", err)
		return nil, WrapMappingFileParse(path, err)
	}

	// Strict validation of the config content
	validatedConfig := make(map[string]string)
	for source, target := range config {
		// Validate source (key) - must be a valid domain name
		source = strings.TrimSpace(source)
		if source == "" {
			debug.Printf("LoadConfig: Empty source key found, skipping entry")
			continue
		}

		// Simple domain validation - could be enhanced with regex
		if !isValidDomain(source) {
			return nil, fmt.Errorf("invalid source registry domain '%s' in config file '%s'", source, path)
		}

		// Validate target (value) - must contain at least one slash
		target = strings.TrimSpace(target)
		if target == "" {
			debug.Printf("LoadConfig: Empty target value for source '%s', skipping entry", source)
			continue
		}

		// Target must contain at least one slash (registry/path format)
		if !strings.Contains(target, "/") {
			return nil, fmt.Errorf("invalid target registry value '%s' for source '%s' in config file '%s': must contain at least one '/'", target, source, path)
		}

		validatedConfig[source] = target
	}

	// Check if we have any valid entries
	if len(validatedConfig) == 0 {
		debug.Printf("LoadConfig: No valid entries found after parsing")
		return nil, WrapMappingFileEmpty(path)
	}

	debug.Printf("LoadConfig: Successfully loaded %d mappings from %s", len(validatedConfig), path)
	return validatedConfig, nil
}

// isValidDomain performs a simple validation on a domain string
func isValidDomain(domain string) bool {
	// Simple validation: Allow alphanumeric, hyphens, dots, and wildcards
	// This is a basic check, could be enhanced with a proper regex for domains
	if domain == "" {
		return false
	}

	// Check for wildcards that would be valid for registry domains
	// Use TrimPrefix as suggested by staticcheck (S1017)
	domain = strings.TrimPrefix(domain, "*.")

	parts := strings.Split(domain, ".")
	if len(parts) < MinDomainPartsForWildcard {
		return false
	}

	for _, part := range parts {
		if part == "" {
			return false
		}

		for _, char := range part {
			// Validate character is alphanumeric or hyphen.
			isLower := char >= 'a' && char <= 'z'
			isUpper := char >= 'A' && char <= 'Z'
			isDigit := char >= '0' && char <= '9'
			isHyphen := char == '-'
			// Explicit De Morgan's law application for staticcheck
			if !isLower && !isUpper && !isDigit && !isHyphen {
				return false
			}
		}

		if strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return false
		}
	}

	return true
}
