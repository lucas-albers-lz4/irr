package override

import (
	"fmt"
	"strings"

	"github.com/lalbers/helm-image-override/pkg/image"
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

// GenerateOverrideStructure creates a map structure for overriding image values
// based on the provided ImageReference and path.
func GenerateOverrideStructure(ref *image.ImageReference, path []string) (map[string]interface{}, error) {
	if ref == nil {
		return nil, fmt.Errorf("nil image reference")
	}

	if len(path) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	// Create the image map with the correct structure
	imageMap := make(map[string]interface{})
	imageMap["registry"] = ref.Registry
	imageMap["repository"] = ref.Repository
	if ref.Digest != "" {
		imageMap["digest"] = ref.Digest
	} else if ref.Tag != "" {
		imageMap["tag"] = ref.Tag
	}

	// Create the override structure and set the image map at the specified path
	overrides := make(map[string]interface{})
	err := SetValueAtPath(overrides, path, imageMap)
	if err != nil {
		return nil, fmt.Errorf("failed to set value at path: %w", err)
	}

	return overrides, nil
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

// ToYAML converts the override file to YAML format
func (o *OverrideFile) ToYAML() ([]byte, error) {
	// Convert directly to YAML
	yamlData, err := yaml.Marshal(o.Overrides)
	if err != nil {
		return nil, fmt.Errorf("error converting to YAML: %w", err)
	}

	return yamlData, nil
}

// JSONToYAML converts JSON data to YAML format
func JSONToYAML(jsonData []byte) ([]byte, error) {
	var yamlData []byte
	var err error

	if yamlData, err = yaml.JSONToYAML(jsonData); err != nil {
		return nil, err
	}

	return yamlData, nil
}
