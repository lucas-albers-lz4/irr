// Package override provides types and functions for creating and manipulating Helm override structures.
package override

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/image"
	"github.com/lucas-albers-lz4/irr/pkg/log"
	"sigs.k8s.io/yaml"
)

// ImageLocation represents a location of an image in a Helm chart
type ImageLocation struct {
	Path              []string
	ImageRef          string
	OriginalReference string
}

// ChartDependency represents a Helm chart dependency with optional alias
type ChartDependency struct {
	Name  string
	Alias string
}

// File represents the generated overrides for a single Helm chart.
type File struct {
	ChartPath      string                 `yaml:"-"` // Original path to the chart
	ChartName      string                 `yaml:"-"` // Base name of the chart directory
	Values         map[string]interface{} `yaml:"overrides"`
	Unsupported    []UnsupportedStructure
	ProcessedCount int     `yaml:"-"` // Number of images successfully processed
	TotalCount     int     `yaml:"-"` // Total number of images detected
	SuccessRate    float64 `yaml:"-"` // Percentage of images successfully processed
}

// UnsupportedStructure represents a structure that could not be processed
type UnsupportedStructure struct {
	Path []string
	Type string
}

// GenerateOverrides generates override values for a single image.
func GenerateOverrides(ref *image.Reference, path []string) (map[string]interface{}, error) {
	if ref == nil {
		return nil, fmt.Errorf("cannot generate overrides for nil image reference")
	}

	if len(path) == 0 {
		return nil, ErrEmptyPath
	}

	// Create a new map to hold the override values
	overrides := make(map[string]interface{})

	// Create a representation that matches the intended format
	// This will be stored at the location specified by 'path'
	ref = normalizeRegistry(ref)
	valueToSet := map[string]interface{}{
		"registry":   ref.Registry,
		"repository": ref.Repository,
	}

	if ref.Tag != "" {
		valueToSet["tag"] = ref.Tag
	}

	if ref.Digest != "" {
		valueToSet["digest"] = ref.Digest
	}

	// Set the value at the specified path in the overrides map
	err := SetValueAtPath(overrides, path, valueToSet)
	if err != nil {
		return nil, fmt.Errorf("failed to set value at path: %w", err)
	}

	return overrides, nil
}

// normalizeRegistry ensures the registry is in the expected format for override generation.
func normalizeRegistry(ref *image.Reference) *image.Reference {
	if ref == nil {
		return nil
	}

	// Create a copy to avoid modifying the original
	result := *ref

	// Docker Hub special case - convert 'docker.io' to registry.hub.docker.com
	// which is how Helm charts frequently represent it
	if ref.Registry == "docker.io" {
		result.Registry = "registry.hub.docker.com"
	}

	return &result
}

// GenerateYAMLOverrides generates YAML content for the given overrides map.
// If the format is 'values', it returns a plain YAML document.
// If the format is 'json', it returns a JSON-formatted string.
// If the format is 'helm-set', it returns a list of --set arguments.
func GenerateYAMLOverrides(overrides map[string]interface{}, format string) ([]byte, error) {
	switch format {
	case "values":
		// Convert directly to YAML
		yamlBytes, err := yaml.Marshal(overrides)
		if err != nil {
			return nil, WrapMarshalOverrides(err)
		}
		return yamlBytes, nil

	case "json":
		// Convert to JSON
		jsonBytes, err := json.Marshal(overrides)
		if err != nil {
			return nil, WrapMarshalOverrides(err)
		}
		return jsonBytes, nil

	case "helm-set":
		// Convert to --set format
		jsonBytes, err := json.Marshal(overrides)
		if err != nil {
			return nil, WrapMarshalOverrides(err)
		}

		// Convert JSON back to YAML for easier parsing
		yamlContent, err := yaml.JSONToYAML(jsonBytes)
		if err != nil {
			return nil, WrapJSONToYAML(err)
		}

		// Parse YAML and flatten to --set format
		var helmSets []string
		if err := flattenYAMLToHelmSet("", yamlContent, &helmSets); err != nil {
			return nil, err
		}

		// Join all --set arguments with newlines
		return []byte(strings.Join(helmSets, "\n")), nil
	}

	// Invalid format
	return nil, fmt.Errorf("invalid format: %s", format)
}

// GenerateYAML generates YAML output for the override structure
func GenerateYAML(overrides map[string]interface{}) ([]byte, error) {
	// Wrap the error from the external YAML library
	yamlBytes, err := yaml.Marshal(overrides)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
	}
	return yamlBytes, nil
}

// ConstructSubchartPath converts a chart path to use aliases defined in dependencies
func ConstructSubchartPath(deps []ChartDependency, path string) (string, error) {
	parts := strings.Split(path, ".")
	result := make([]string, len(parts))
	copy(result, parts)

	// Build a map of chart names to aliases for quick lookup
	aliases := make(map[string]string)
	for _, dep := range deps {
		if dep.Alias != "" {
			aliases[dep.Name] = dep.Alias
		}
	}

	// Replace chart names with aliases where they exist
	for i, part := range parts {
		if alias, ok := aliases[part]; ok {
			result[i] = alias
		}
	}

	return strings.Join(result, "."), nil
}

// VerifySubchartPath verifies that the generated subchart path is valid
// This provides a basic sanity check for generated paths
func VerifySubchartPath(path string, deps []ChartDependency) error {
	if path == "" {
		return fmt.Errorf("empty path provided for verification")
	}

	// Build a map of chart names and aliases
	chartNames := make(map[string]bool)
	chartAliases := make(map[string]bool)

	for _, dep := range deps {
		chartNames[dep.Name] = true
		if dep.Alias != "" {
			chartAliases[dep.Alias] = true
		}
	}

	// Parse the path and verify parts
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return fmt.Errorf("invalid empty path")
	}

	// Check if first part is a known chart name or alias
	firstPart := parts[0]
	_, isName := chartNames[firstPart]
	_, isAlias := chartAliases[firstPart]

	// If we have chart dependencies and first part isn't recognized,
	// it might indicate a potential path issue
	if len(deps) > 0 && (!isName && !isAlias) {
		log.Debug("Warning: Generated path starts with unknown chart name/alias", "path", path, "firstPart", firstPart)
	}

	return nil
}

// ToYAML serializes the override structure to YAML.
func (f *File) ToYAML() ([]byte, error) {
	log.Debug("Marshaling override.File to YAML")
	yamlBytes, err := yaml.Marshal(f.Values)
	if err != nil {
		// Wrap error from external YAML library
		return nil, fmt.Errorf("failed to marshal override content to YAML: %w", err)
	}
	return yamlBytes, nil
}

// JSONToYAML converts JSON data to YAML format
func JSONToYAML(jsonData []byte) ([]byte, error) {
	// Attempt to convert JSON bytes (potentially from Helm output) to YAML
	yamlBytes, err := yaml.JSONToYAML(jsonData)
	if err != nil {
		// Wrap the error from JSONToYAML
		return nil, fmt.Errorf("failed to convert JSON to YAML: %w", err)
	}
	return yamlBytes, nil
}

// flattenYAMLToHelmSet recursively flattens YAML content into Helm --set format
func flattenYAMLToHelmSet(prefix string, content []byte, sets *[]string) error {
	var data interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	return flattenValue(prefix, data, sets)
}

// flattenValue recursively processes values and converts them to --set format
func flattenValue(prefix string, value interface{}, sets *[]string) error {
	switch v := value.(type) {
	case map[interface{}]interface{}:
		for k, val := range v {
			key := fmt.Sprintf("%v", k)
			var newPrefix string
			if prefix == "" {
				newPrefix = key
			} else {
				newPrefix = prefix + "." + key
			}
			if err := flattenValue(newPrefix, val, sets); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		for k, val := range v {
			var newPrefix string
			if prefix == "" {
				newPrefix = k
			} else {
				newPrefix = prefix + "." + k
			}
			if err := flattenValue(newPrefix, val, sets); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, val := range v {
			newPrefix := fmt.Sprintf("%s[%d]", prefix, i)
			if err := flattenValue(newPrefix, val, sets); err != nil {
				return err
			}
		}
	default:
		*sets = append(*sets, fmt.Sprintf("--set %s=%v", prefix, v))
	}
	return nil
}

// SetValueAtPath is defined in path_utils.go
