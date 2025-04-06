package override

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lalbers/irr/pkg/image"
	"sigs.k8s.io/yaml"
)

// ImageLocation represents a location of an image in a Helm chart
type ImageLocation struct {
	Path              []string
	ImageRef          string
	OriginalReference string
}

// ImageReference represents a parsed container image reference
type ImageReference struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

// ParseImageReference parses an image reference string into its components
func ParseImageReference(ref string) (*ImageReference, error) {
	result := &ImageReference{}

	// Split registry and rest
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 && strings.Contains(parts[0], ".") {
		result.Registry = parts[0]
		ref = parts[1]
	} else {
		ref = strings.Join(parts, "/")
	}

	// Handle digest
	if digestParts := strings.SplitN(ref, "@", 2); len(digestParts) == 2 {
		ref = digestParts[0]
		result.Digest = digestParts[1]
	}

	// Handle tag
	if tagParts := strings.SplitN(ref, ":", 2); len(tagParts) == 2 {
		ref = tagParts[0]
		result.Tag = tagParts[1]
	}

	result.Repository = ref
	return result, nil
}

// ChartDependency represents a Helm chart dependency with optional alias
type ChartDependency struct {
	Name  string
	Alias string
}

// Override represents a single override value and its path
type Override struct {
	Path  []string
	Value interface{}
}

// OverrideFile represents a complete override file
type OverrideFile struct {
	ChartPath   string
	ChartName   string
	Overrides   map[string]interface{}
	Unsupported []UnsupportedStructure
}

// UnsupportedStructure represents a structure that could not be processed
type UnsupportedStructure struct {
	Path []string
	Type string
}

// GenerateOverrides generates override values for a single image.
func GenerateOverrides(ref *image.ImageReference, path []string) (map[string]interface{}, error) {
	if ref == nil {
		return nil, ErrNilImageReference
	}

	if len(path) == 0 {
		return nil, ErrEmptyPath
	}

	// Create a new map to hold the override values
	overrides := make(map[string]interface{})

	// Create a representation that matches the intended format
	// This will be stored at the location specified by 'path'
	ref = normalizeRegistry(ref)
	var valueToSet interface{}

	// TypeMapRegistryTag is most common, but also support TypeMapRegistry and TypeMapTag
	// These type constants are defined in the image package
	valueToSet = map[string]interface{}{
		"registry":   ref.Registry,
		"repository": ref.Repository,
	}

	if ref.Tag != "" {
		valueToSet.(map[string]interface{})["tag"] = ref.Tag
	}

	if ref.Digest != "" {
		valueToSet.(map[string]interface{})["digest"] = ref.Digest
	}

	// Set the value at the specified path in the overrides map
	err := SetValueAtPath(overrides, path, valueToSet)
	if err != nil {
		return nil, fmt.Errorf("failed to set value at path: %w", err)
	}

	return overrides, nil
}

// normalizeRegistry ensures the registry is in the expected format for override generation.
func normalizeRegistry(ref *image.ImageReference) *image.ImageReference {
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

// ConstructPath constructs the path for the override structure
func ConstructPath(path []string) []string {
	return path
}

// GenerateYAML generates YAML output for the override structure
func GenerateYAML(overrides map[string]interface{}) ([]byte, error) {
	return yaml.Marshal(overrides)
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

// MergeInto merges this override into the target map
func (o *Override) MergeInto(target map[string]interface{}) map[string]interface{} {
	current := target
	for _, key := range o.Path[:len(o.Path)-1] {
		next, exists := current[key]
		if !exists {
			next = make(map[string]interface{})
			current[key] = next
		}
		if nextMap, ok := next.(map[string]interface{}); ok {
			current = nextMap
		} else {
			// Convert the existing value to a map
			nextMap := make(map[string]interface{})
			current[key] = nextMap
			current = nextMap
		}
	}
	current[o.Path[len(o.Path)-1]] = o.Value
	return target
}

// ToYAML serializes the override structure to YAML.
func (o *OverrideFile) ToYAML() ([]byte, error) {
	yamlData, err := yaml.Marshal(o.Overrides)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
	}
	return yamlData, nil
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
			if strings.HasPrefix(key, prefix) {
				// Use prefix directly or construct new one if needed
				newPrefix := prefix
				if len(key) > len(prefix) {
					newPrefix = key[:strings.LastIndex(key, ".")]
				}
				// Use newPrefix in subsequent operations
				if err := flattenValue(newPrefix, val, sets); err != nil {
					return err
				}
			}
		}
	case map[string]interface{}:
		for k, val := range v {
			if strings.HasPrefix(k, prefix) {
				// Use prefix directly or construct new one if needed
				newPrefix := prefix
				if len(k) > len(prefix) {
					newPrefix = k[:strings.LastIndex(k, ".")]
				}
				// Use newPrefix in subsequent operations
				if err := flattenValue(newPrefix, val, sets); err != nil {
					return err
				}
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
