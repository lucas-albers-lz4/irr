// Package registry provides functionality for mapping container registry names.
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// MaxKeyLength defines the maximum allowed length for registry keys.
	MaxKeyLength = 253
	// MaxValueLength defines the maximum allowed length for registry values.
	MaxValueLength = 1024
	// SplitKeyValueParts defines the number of parts expected when splitting a key:value pair.
	SplitKeyValueParts = 2
)

// EmptyPathResult is a sentinel value returned when path is empty
var EmptyPathResult = map[string]string{}

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

	// Try the new format (with mappings key)
	var mappings Mappings
	var newFormat struct {
		Mappings []Mapping `yaml:"mappings"`
	}
	if err := yaml.Unmarshal(data, &newFormat); err != nil {
		debug.Printf("LoadMappings: Failed to parse as structured format: %v", err)
		return nil, WrapMappingFileParse(path, err)
	}

	if len(newFormat.Mappings) == 0 {
		debug.Printf("LoadMappings: No valid entries found after parsing")
		return nil, WrapMappingFileEmpty(path)
	}

	debug.Printf("LoadMappings: Successfully parsed structured format, found %d entries", len(newFormat.Mappings))
	mappings.Entries = make([]Mapping, len(newFormat.Mappings))
	for i, m := range newFormat.Mappings {
		mappings.Entries[i] = Mapping{
			Source: strings.TrimSpace(m.Source),
			Target: strings.TrimSpace(m.Target),
		}
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

// validateConfigFilePath validates path and performs basic integrity checks
func validateConfigFilePath(fs afero.Fs, path string, skipCWDRestriction bool) error {
	// Basic validation to prevent path traversal
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for config file '%s': %w", path, err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Only skip path traversal check if explicitly allowed in test or via parameter
	if !skipCWDRestriction {
		if !strings.HasPrefix(absPath, wd) {
			debug.Printf("Path traversal detected. Path: %s, WorkDir: %s", absPath, wd)
			return WrapMappingPathNotInWD(path)
		}
	}

	// Check if path is a directory using the provided filesystem
	fileInfo, err := fs.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WrapMappingFileNotExist(path, err)
		}
		return WrapMappingFileRead(path, err)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("failed to read config file '%s': is a directory", path)
	}

	// Check file extension
	if !strings.HasSuffix(absPath, ".yaml") && !strings.HasSuffix(absPath, ".yml") {
		return WrapMappingExtension(path)
	}

	return nil
}

// readConfigFileContent reads and performs basic validation on file content
func readConfigFileContent(fs afero.Fs, path string) ([]byte, error) {
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

	return data, nil
}

// validateMappingValue validates format and constraints for a target value
func validateMappingValue(source, target, path string) error {
	// Check value length
	if len(target) > MaxValueLength {
		return WrapValueTooLong(path, source, target, len(target), MaxValueLength)
	}

	// Target must contain at least one slash (registry/path format)
	if !strings.Contains(target, "/") {
		return fmt.Errorf("invalid target registry value '%s' for source '%s' in config file '%s': must contain at least one '/'",
			target, source, path)
	}

	// Validate port number if present
	hostPart := strings.Split(target, "/")[0]
	if strings.Contains(hostPart, ":") {
		hostAndPort := strings.Split(hostPart, ":")
		if len(hostAndPort) > 1 {
			portStr := hostAndPort[1]
			port, err := strconv.Atoi(portStr)
			if err != nil || port < 1 || port > 65535 {
				return WrapInvalidPortNumber(path, source, target, portStr)
			}
		}
	}

	return nil
}

// LoadConfig loads a configuration file using the provided filesystem.
// Returns a map[string]string for backward compatibility with existing code.
func LoadConfig(fs afero.Fs, path string, skipCWDRestriction bool) (map[string]string, error) {
	if path == "" {
		return EmptyPathResult, ErrNoConfigSpecified
	}

	// Try loading as structured format
	structuredConfig, err := LoadStructuredConfig(fs, path, skipCWDRestriction)
	if err != nil {
		debug.Printf("LoadConfig: Failed to load structured config: %v", err)
		return nil, err
	}

	// Convert structured format to map[string]string for backward compatibility
	debug.Printf("LoadConfig: Successfully loaded structured config with %d mappings", len(structuredConfig.Registries.Mappings))
	return ConvertToLegacyFormat(structuredConfig), nil
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
