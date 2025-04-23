// Package registry provides functionality for mapping container registry names.
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"errors"

	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/log"
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
		return nil, fmt.Errorf("mappings file path is empty")
	}

	if err := validateConfigFilePath(fs, path, skipCWDRestriction); err != nil {
		return nil, err
	}

	// Read the file content using the provided filesystem
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, WrapMappingFileNotExist(path, err)
		}
		return nil, WrapMappingFileRead(path, err)
	}

	// Check for empty file content BEFORE parsing attempts
	if len(data) == 0 {
		return nil, WrapMappingFileEmpty(path)
	}

	log.Debug("LoadMappings: Attempting to parse file content:\n%s", string(data))

	// --- Attempt 1: Structured Format ---
	var config Config
	var structuredParseErr error
	var structuredValidationErr error

	structuredParseErr = yaml.Unmarshal(data, &config)
	if structuredParseErr == nil {
		// Parsing succeeded, now validate
		structuredValidationErr = validateStructuredConfig(&config, path)
		if structuredValidationErr == nil {
			// Validation also succeeded, check if mappings exist
			if len(config.Registries.Mappings) == 0 {
				// Valid structure but no mappings is treated as empty
				log.Debug("LoadMappings: Structured config parsed and validated, but contains no mapping entries")
				// We might still want to try legacy here if structured is valid but empty?
				// For now, treat as empty and proceed to legacy check.
				structuredValidationErr = WrapMappingFileEmpty(path)
			} else {
				// Success! Convert and return.
				log.Debug("LoadMappings: Successfully loaded %d mappings from structured format in %s", len(config.Registries.Mappings), path)
				return config.ToMappings(), nil
			}
		} else {
			log.Debug("LoadMappings: Structured config parsed but failed validation: %v", structuredValidationErr)
			// Fall through to legacy attempt
		}
	} else {
		log.Debug("LoadMappings: Failed to parse as structured format: %v", structuredParseErr)
		// Fall through to legacy attempt
	}

	// --- Attempt 2: Legacy Format (Key-Value) ---
	log.Debug("LoadMappings: Attempting to parse as legacy key-value format")
	var legacyFormat map[string]string
	legacyErr := yaml.Unmarshal(data, &legacyFormat)
	if legacyErr != nil {
		log.Debug("LoadMappings: Also failed to parse as legacy format: %v", legacyErr)
		// Both attempts failed. Prioritize structured error if available.
		finalErr := structuredParseErr // Start with the parse error
		if finalErr == nil {
			finalErr = structuredValidationErr // If parse ok, use validation error
		}
		if finalErr == nil {
			// If somehow both structured errors are nil, use legacy error
			finalErr = legacyErr
		}

		// Ensure we always return a parse error if both failed
		// Handle specific case where structured failed validation due to empty, but legacy also failed.
		if finalErr != nil && strings.Contains(finalErr.Error(), "mappings file is empty") {
			return nil, WrapMappingFileParse(path, fmt.Errorf("failed structured parse (%w) and legacy parse (%w)", finalErr, legacyErr))
		} else if finalErr != nil {
			return nil, WrapMappingFileParse(path, fmt.Errorf("failed structured parse/validation (%w) and legacy parse (%w)", finalErr, legacyErr))
		}
		// If we reached here, both structured and legacy parsing failed, but finalErr somehow ended up nil (should be unreachable).
		// Outdent this return as per revive suggestion (removed the wrapping else).
		return nil, WrapMappingFileParse(path, fmt.Errorf("failed to parse as structured or legacy format: %w", legacyErr))
	}

	// Check if legacy format contains any entries
	if len(legacyFormat) == 0 {
		log.Debug("LoadMappings: Legacy format was parsed but contains no entries")
		// If structured parse failed validation because it was empty, and legacy is also empty, report empty.
		if structuredValidationErr != nil && strings.Contains(structuredValidationErr.Error(), "mappings file is empty") {
			return nil, WrapMappingFileEmpty(path)
		}
		// Otherwise, if structured failed for other reasons and legacy is empty, report legacy empty.
		return nil, WrapMappingFileEmpty(path)
	}

	// Convert legacy format to mappings structure
	log.Debug("LoadMappings: Successfully parsed legacy key-value format, found %d entries", len(legacyFormat))
	mappings := Mappings{
		Entries: make([]Mapping, 0, len(legacyFormat)),
	}
	for source, target := range legacyFormat {
		mappings.Entries = append(mappings.Entries, Mapping{
			Source: strings.TrimSpace(source),
			Target: strings.TrimSpace(target),
		})
	}

	log.Debug("LoadMappings: Successfully loaded %d mappings from legacy format in %s", len(mappings.Entries), path)
	return &mappings, nil
}

// GetTargetRegistry returns the target registry for a given source registry
func (m *Mappings) GetTargetRegistry(source string) string {
	log.Debug("GetTargetRegistry: Looking for source '%s' in mappings", source)
	if m == nil || m.Entries == nil {
		log.Debug("GetTargetRegistry: Mappings are nil or empty.")
		return ""
	}

	// Clean and normalize the input source
	source = strings.TrimSpace(source)
	source = strings.TrimRight(source, "\r")
	normalizedSourceInput := image.NormalizeRegistry(source)
	log.Debug("GetTargetRegistry: Normalized source INPUT: '%s' -> '%s'", source, normalizedSourceInput)

	// Special case: if source starts with index.docker.io, normalize it
	if strings.HasPrefix(source, "index.docker.io/") {
		normalizedSourceInput = "docker.io"
		log.Debug("GetTargetRegistry: Special case - normalized index.docker.io to docker.io")
	}

	for _, mapping := range m.Entries {
		// Clean and normalize the mapping source
		mappingSource := strings.TrimSpace(mapping.Source)
		mappingSource = strings.TrimRight(mappingSource, "\r")
		normalizedMappingSource := image.NormalizeRegistry(mappingSource)
		log.Debug("GetTargetRegistry: Comparing normalized input '%s' with normalized mapping '%s'",
			normalizedSourceInput, normalizedMappingSource)

		if normalizedSourceInput == normalizedMappingSource {
			target := strings.TrimSpace(mapping.Target)
			log.Debug("GetTargetRegistry: Match found! Returning target: '%s'", target)
			return target
		}
	}

	log.Debug("GetTargetRegistry: No match found for source '%s'", source)
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

	// --- DEBUGGING: Log path validation values ---
	log.Debug("Validating config file path",
		"path", path,
		"absPath", absPath,
		"workDir", wd,
		"skipCWDRestriction", skipCWDRestriction,
		"isPrefix", strings.HasPrefix(absPath, wd),
	)
	// --- END DEBUGGING ---

	// Only skip path traversal check if explicitly allowed in test or via parameter
	if !skipCWDRestriction {
		if !strings.HasPrefix(absPath, wd) {
			log.Debug("Path traversal detected. Path: %s, WorkDir: %s", absPath, wd)
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
		return fmt.Errorf("failed to read mappings file '%s': is a directory", path)
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

	structuredConfig, err := tryLoadStructuredConfig(fs, path, skipCWDRestriction)
	if err == nil {
		if err := validateStructuredMappings(structuredConfig, path); err != nil {
			return nil, err
		}
		log.Debug("LoadConfig: Successfully loaded structured config with %d mappings", len(structuredConfig.Registries.Mappings))
		return ConvertToLegacyFormat(structuredConfig), nil
	}

	if isValidationError(err) {
		return nil, err
	}

	log.Debug("LoadConfig: Failed to load structured config: %v", err)

	log.Debug("LoadConfig: Attempting to load as legacy format")
	legacyFormat, err := tryLoadLegacyConfig(fs, path, skipCWDRestriction)
	if err != nil {
		return nil, err
	}
	if err := validateLegacyMappings(legacyFormat, path); err != nil {
		return nil, err
	}
	log.Debug("LoadConfig: Successfully loaded legacy format with %d mappings", len(legacyFormat))
	return legacyFormat, nil
}

func tryLoadStructuredConfig(fs afero.Fs, path string, skipCWDRestriction bool) (*Config, error) {
	return LoadStructuredConfig(fs, path, skipCWDRestriction)
}

func validateStructuredMappings(cfg *Config, path string) error {
	seen := make(map[string]struct{})
	for _, mapping := range cfg.Registries.Mappings {
		source := mapping.Source
		target := mapping.Target
		if len(source) > MaxKeyLength {
			return fmt.Errorf("registry key '%s' exceeds maximum length of %d characters in mappings file '%s'", source, MaxKeyLength, path)
		}
		if len(target) > MaxValueLength {
			return fmt.Errorf("registry value '%s' for key '%s' exceeds maximum length of %d characters in mappings file '%s'", target, source, MaxValueLength, path)
		}
		if !isValidDomain(source) {
			return fmt.Errorf("invalid source registry domain '%s' in config file '%s'", source, path)
		}
		if !strings.Contains(target, "/") {
			return fmt.Errorf("invalid target registry value '%s' for source '%s' in config file '%s': must contain at least one '/'", target, source, path)
		}
		hostPart := strings.Split(target, "/")[0]
		if strings.Contains(hostPart, ":") {
			hostAndPort := strings.Split(hostPart, ":")
			if len(hostAndPort) > 1 {
				portStr := hostAndPort[1]
				port, err := strconv.Atoi(portStr)
				if err != nil || port < 1 || port > 65535 {
					return fmt.Errorf("invalid port number '%s' in target registry value '%s' for source '%s' in mappings file '%s'", portStr, target, source, path)
				}
			}
		}
		if _, exists := seen[source]; exists {
			return fmt.Errorf("duplicate registry key '%s' found in mappings file '%s'", source, path)
		}
		seen[source] = struct{}{}
	}
	return nil
}

func isValidationError(err error) bool {
	validationErrs := []string{
		"invalid source registry domain",
		"invalid target registry value",
		"duplicate registry key",
		"invalid port number",
		"registry key",
		"registry value",
	}
	for _, substr := range validationErrs {
		if strings.Contains(err.Error(), substr) {
			return true
		}
	}
	return false
}

func tryLoadLegacyConfig(fs afero.Fs, path string, skipCWDRestriction bool) (map[string]string, error) {
	if err := validateConfigFilePath(fs, path, skipCWDRestriction); err != nil {
		return nil, err
	}
	data, err := readConfigFileContent(fs, path)
	if err != nil {
		return nil, err
	}
	var legacyFormat map[string]string
	if err := yaml.Unmarshal(data, &legacyFormat); err != nil {
		return nil, fmt.Errorf("failed to parse config file '%s': %w", path, err)
	}
	if len(legacyFormat) == 0 {
		log.Debug("LoadConfig: No mappings found in legacy format")
		return nil, WrapMappingFileEmpty(path)
	}
	return legacyFormat, nil
}

func validateLegacyMappings(legacyFormat map[string]string, path string) error {
	for source, target := range legacyFormat {
		if len(source) > MaxKeyLength {
			return fmt.Errorf("registry key too long in config file '%s'", path)
		}
		if len(target) > MaxValueLength {
			return fmt.Errorf("registry value too long in config file '%s'", path)
		}
		if !isValidDomain(source) {
			return fmt.Errorf("invalid source registry domain '%s' in config file '%s'", source, path)
		}
		if !strings.Contains(target, "/") {
			return fmt.Errorf("invalid target registry value '%s' for source '%s' in config file '%s': must contain at least one '/'", target, source, path)
		}
		hostPart := strings.Split(target, "/")[0]
		if strings.Contains(hostPart, ":") {
			hostAndPort := strings.Split(hostPart, ":")
			if len(hostAndPort) > 1 {
				portStr := hostAndPort[1]
				port, err := strconv.Atoi(portStr)
				if err != nil || port < 1 || port > 65535 {
					return fmt.Errorf("invalid port number '%s' in target registry value '%s' for source '%s' in config file '%s'", portStr, target, source, path)
				}
			}
		}
	}
	return nil
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

// LoadMappingsWithFS loads registry mappings from a YAML file using the provided fileutil.FS.
// skipCWDRestriction allows bypassing the check that the path must be within the CWD tree.
func LoadMappingsWithFS(fs fileutil.FS, path string, skipCWDRestriction bool) (*Mappings, error) {
	if fs == nil {
		fs = DefaultFS
	}

	// Convert to afero.Fs for backwards compatibility with existing implementation
	afs := GetAferoFS(fs)

	return LoadMappings(afs, path, skipCWDRestriction)
}

// LoadMappingsDefault loads registry mappings using the default filesystem.
func LoadMappingsDefault(path string, skipCWDRestriction bool) (*Mappings, error) {
	return LoadMappingsWithFS(DefaultFS, path, skipCWDRestriction)
}
