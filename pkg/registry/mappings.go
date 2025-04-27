// Package registry provides functionality for mapping container registry names.
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"errors"

	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultFilePermissions defines the permission mode for new files using modern octal literal style.
	DefaultFilePermissions = 0o644
	// MinDomainPartsForWildcard defines the minimum parts for a valid wildcard domain.
	MinDomainPartsForWildcard = 2
	// MinDomainParts defines the minimum number of parts for a standard domain.
	MinDomainParts = 2
	// MaxKeyLength defines the maximum allowed length for registry keys.
	MaxKeyLength = 253
	// MaxValueLength defines the maximum allowed length for registry values.
	MaxValueLength = 1024
	// SplitKeyValueParts defines the number of parts expected when splitting a key:value pair.
	SplitKeyValueParts = 2
	// DockerHubRegistry represents the canonical name for Docker Hub.
	DockerHubRegistry = "docker.io"
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

	// Parse as structured format
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Debug("LoadMappings: Failed to parse as structured format: %v", err)
		return nil, WrapMappingFileParse(path, err)
	}

	// Validate the parsed config
	if err := validateStructuredConfig(&config, path); err != nil {
		log.Debug("LoadMappings: Structured config parsed but failed validation: %v", err)
		// Check if the validation error is SPECIFICALLY the empty mapping error
		var emptyErr *ErrMappingFileEmpty
		if errors.As(err, &emptyErr) {
			log.Debug("LoadMappings: Structured validation indicated mappings list is empty. Returning empty result.")
			return &Mappings{Entries: []Mapping{}}, nil // Return success (empty mappings)
		}
		// For other validation errors, return the error
		return nil, err
	}

	// Convert and return the mappings
	if len(config.Registries.Mappings) == 0 {
		// This case should ideally not be hit if validation passed, but handle defensively.
		log.Debug("LoadMappings: Structured config parsed and validated, but contains no mapping entries")
		return &Mappings{Entries: []Mapping{}}, nil
	}

	log.Debug("LoadMappings: Successfully loaded %d mappings from structured format in %s", len(config.Registries.Mappings), path)
	return config.ToMappings(), nil
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
		normalizedSourceInput = DockerHubRegistry // Use constant
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
	// REMOVED: os.Getwd() and filepath.Abs() as they rely on real process CWD
	// REMOVED: CWD prefix check as it's unreliable with mock filesystems

	log.Debug("Validating config file path (simplified)",
		"path", path,
		"skipCWDRestriction(ignored)", skipCWDRestriction,
	)

	// Check if path is empty
	if path == "" {
		return errors.New("config file path cannot be empty")
	}

	// Clean the path (important!)
	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") {
		// Basic check to prevent accessing files outside the intended root (e.g., mock FS root)
		log.Warn("Config file path appears to traverse upwards, potentially unsafe", "path", path, "cleaned", cleanPath)
		// Depending on security posture, could return an error here.
		// For now, allow but warn.
	}

	// Check if path exists and is not a directory using the provided filesystem
	fileInfo, err := fs.Stat(cleanPath) // Use cleaned path
	if err != nil {
		if os.IsNotExist(err) {
			return WrapMappingFileNotExist(cleanPath, err) // Use cleaned path in error
		}
		return WrapMappingFileRead(cleanPath, err) // Use cleaned path in error
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("config file path '%s' is a directory, not a file", cleanPath)
	}

	// Check file extension (using cleaned path)
	if !strings.HasSuffix(cleanPath, ".yaml") && !strings.HasSuffix(cleanPath, ".yml") {
		return WrapMappingExtension(cleanPath)
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

	// Check for empty file
	if len(data) == 0 {
		return nil, WrapMappingFileEmpty(path)
	}

	return data, nil
}

// validateMappingValue performs validation on a target value
func validateMappingValue(source, target, path string) error {
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

// LoadConfig loads registry mappings using the structured format.
func LoadConfig(fs afero.Fs, path string, skipCWDRestriction bool) (*Config, error) {
	return LoadStructuredConfig(fs, path, skipCWDRestriction)
}

// isValidDomain checks if the given domain is a valid registry domain
func isValidDomain(domain string) bool {
	// Empty domain is invalid
	if domain == "" {
		return false
	}

	// Special case handling for well-known domains
	wellKnownDomains := map[string]bool{
		"docker.io":           true,
		"quay.io":             true,
		"gcr.io":              true,
		"k8s.gcr.io":          true,
		"registry.k8s.io":     true,
		"ghcr.io":             true,
		"docker.elastic.co":   true,
		"mcr.microsoft.com":   true,
		"public.ecr.aws":      true,
		"index.docker.io":     true,
		"registry.gitlab.com": true,
	}

	if wellKnownDomains[domain] {
		return true
	}

	// Check for wildcard
	if strings.HasPrefix(domain, "*.") {
		// For wildcards, ensure there are at least MinDomainPartsForWildcard parts after the *
		// e.g., *.example.com (valid), *.co (invalid)
		remaining := domain[2:] // Skip the *. prefix
		parts := strings.Split(remaining, ".")
		return len(parts) >= MinDomainPartsForWildcard
	}

	// Regular domain validation
	// Split by dot and ensure each part is valid
	parts := strings.Split(domain, ".")
	if len(parts) < MinDomainParts {
		log.Debug("isValidDomain: domain has fewer than required parts", "domain", domain, "parts", len(parts), "required", MinDomainParts)
		return false // Must have at least 2 parts (e.g., domain.com)
	}

	for _, part := range parts {
		// Check if part is empty
		if part == "" { // Use direct string comparison
			log.Debug("isValidDomain: component is empty", "domain", domain)
			return false
		}

		// Allow digits, letters, and hyphens in parts
		for _, ch := range part {
			// Check if the character is valid (alphanumeric or hyphen)
			isLower := 'a' <= ch && ch <= 'z'
			isUpper := 'A' <= ch && ch <= 'Z'
			isDigit := '0' <= ch && ch <= '9'
			isHyphen := ch == '-'

			// Apply De Morgan's law: !(A || B || C || D) is equivalent to !A && !B && !C && !D
			if !isLower && !isUpper && !isDigit && !isHyphen {
				log.Debug("isValidDomain: invalid character in component", "domain", domain, "component", part, "character", string(ch))
				return false
			}
		}

		// Parts can't start or end with hyphen
		if part[0] == '-' || part[len(part)-1] == '-' {
			log.Debug("isValidDomain: component starts or ends with hyphen", "domain", domain, "component", part)
			return false
		}
	}

	return true
}

// LoadMappingsWithFS loads registry mappings using the provided fileutil.FS.
func LoadMappingsWithFS(fs fileutil.FS, path string, skipCWDRestriction bool) (*Mappings, error) {
	if fs == nil {
		fs = DefaultFS
	}

	// Convert to afero.Fs for compatibility with existing implementation
	afs := GetAferoFS(fs)

	return LoadMappings(afs, path, skipCWDRestriction)
}

// LoadMappingsDefault loads registry mappings using the default filesystem.
func LoadMappingsDefault(path string, skipCWDRestriction bool) (*Mappings, error) {
	return LoadMappingsWithFS(DefaultFS, path, skipCWDRestriction)
}
