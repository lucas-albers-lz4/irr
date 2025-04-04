package chart

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lalbers/helm-image-override/pkg/debug"
	"github.com/lalbers/helm-image-override/pkg/image"
	"github.com/lalbers/helm-image-override/pkg/override"
	"github.com/lalbers/helm-image-override/pkg/strategy"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"sigs.k8s.io/yaml"
)

// Package chart provides functionality for handling Helm charts and generating image override values.
// This package is responsible for processing Helm charts and generating override structures
// that can be used to modify image references in the chart's values.

// Key concepts:
// - ImageReference: Represents a container image reference (registry/repository:tag)
// - OverrideStructure: A nested map that mirrors the chart's values structure
// - PathStrategy: Defines how image paths are transformed in the override structure

// Type hints for map structures:
// map[string]interface{} can contain:
// - Nested maps (for structured data)
// - Arrays (for list values)
// - Strings (for image references and other values)
// - Numbers (for ports, replicas, etc.)
// - Booleans (for feature flags)

// @llm-helper This package uses reflection and type assertions extensively
// @llm-helper The override structure matches Helm's value override format
// @llm-helper Image references can be in multiple formats (string, map with repository/tag)

// ImageReference represents an image reference found in a chart
type ImageReference struct {
	Path      []string
	Reference *image.ImageReference
}

// Generator generates image overrides for a Helm chart
type Generator struct {
	chartPath         string
	targetRegistry    string
	sourceRegistries  []string
	excludeRegistries []string
	pathStrategy      strategy.PathStrategy
	strict            bool
	threshold         int
}

// NewGenerator creates a new Generator
func NewGenerator(chartPath, targetRegistry string, sourceRegistries, excludeRegistries []string, pathStrategy strategy.PathStrategy, strict bool, threshold int) *Generator {
	return &Generator{
		chartPath:         chartPath,
		targetRegistry:    targetRegistry,
		sourceRegistries:  sourceRegistries,
		excludeRegistries: excludeRegistries,
		pathStrategy:      pathStrategy,
		strict:            strict,
		threshold:         threshold,
	}
}

// Generate generates image overrides for the chart
func (g *Generator) Generate() (*override.OverrideFile, error) {
	debug.FunctionEnter("Generator.Generate")
	defer debug.FunctionExit("Generator.Generate")

	// Load the chart using the Helm SDK loader
	chart, err := loader.Load(g.chartPath)
	if err != nil {
		return nil, fmt.Errorf("error loading chart: %w", err)
	}

	// Create a deep copy of the chart values to preserve structure
	baseValues := override.DeepCopy(chart.Values).(map[string]interface{})
	debug.DumpValue("Base values", baseValues)

	// Get all image references from the chart
	images, unsupportedMatches, err := image.DetectImages(chart.Values, []string{}, g.sourceRegistries, g.excludeRegistries, g.strict)
	if err != nil {
		return nil, fmt.Errorf("error detecting images: %w", err)
	}
	debug.DumpValue("Found images", images)
	debug.DumpValue("Unsupported structures", unsupportedMatches)

	// Convert unsupported matches to unsupported structures
	var unsupported []override.UnsupportedStructure
	for _, match := range unsupportedMatches {
		unsupported = append(unsupported, override.UnsupportedStructure{
			Path: match.Path,
			Type: match.RefType,
		})
	}

	// Calculate success percentage
	totalImages := len(images)
	if totalImages == 0 {
		return nil, fmt.Errorf("no images found in chart")
	}

	// Process each image reference
	processedImages := 0
	modifiedValues := override.DeepCopy(baseValues).(map[string]interface{})

	for _, img := range images {
		// Skip if the image's registry is in the exclude list
		if g.isExcluded(img.ImageRef.Registry) {
			continue
		}

		// Skip if the image's registry is not in the source list
		if !g.isSourceRegistry(img.ImageRef.Registry) {
			continue
		}

		// Transform the image reference using the path strategy
		transformedPath := g.pathStrategy.Transform(img.ImageRef, g.targetRegistry)

		// Create the image configuration based on the original structure type
		var newValue interface{}
		switch img.RefType {
		case image.PatternRegistryRepoTag:
			newValue = map[string]interface{}{
				"registry":   g.targetRegistry,
				"repository": transformedPath,
				"tag":        img.ImageRef.Tag,
			}
			if img.ImageRef.Digest != "" {
				delete(newValue.(map[string]interface{}), "tag")
				newValue.(map[string]interface{})["digest"] = img.ImageRef.Digest
			}
		case image.PatternRepositoryTag:
			newValue = map[string]interface{}{
				"repository": transformedPath,
				"tag":        img.ImageRef.Tag,
			}
			if img.ImageRef.Digest != "" {
				delete(newValue.(map[string]interface{}), "tag")
				newValue.(map[string]interface{})["digest"] = img.ImageRef.Digest
			}
		case image.PatternImageString:
			if img.ImageRef.Digest != "" {
				newValue = fmt.Sprintf("%s/%s@%s", g.targetRegistry, transformedPath, img.ImageRef.Digest)
			} else {
				newValue = fmt.Sprintf("%s/%s:%s", g.targetRegistry, transformedPath, img.ImageRef.Tag)
			}
		default:
			debug.Printf("Skipping unsupported image reference type at path %v: %s", img.Path, img.RefType)
			continue
		}

		// Set the value at the correct path in the modified structure
		err := image.SetValueAtPath(modifiedValues, img.Path, newValue)
		if err != nil {
			debug.Printf("Error setting value at path %v: %v", img.Path, err)
			continue
		}

		processedImages++
	}

	// Calculate success percentage
	successPercentage := (processedImages * 100) / totalImages
	if successPercentage < g.threshold {
		return nil, fmt.Errorf("success percentage %d%% is below threshold %d%%", successPercentage, g.threshold)
	}

	// If strict mode is enabled and there are unsupported structures, return an error
	if g.strict && len(unsupported) > 0 {
		return nil, fmt.Errorf("found %d unsupported structures in strict mode", len(unsupported))
	}

	// Extract only the modified paths from the modified structure
	overrides := make(map[string]interface{})
	for _, img := range images {
		if g.isExcluded(img.ImageRef.Registry) || !g.isSourceRegistry(img.ImageRef.Registry) {
			continue
		}
		// Extract the modified subtree and merge it into the overrides
		subtree := extractSubtree(modifiedValues, img.Path)
		mergeOverrides(overrides, subtree)
	}

	return &override.OverrideFile{
		ChartPath:   g.chartPath,
		ChartName:   filepath.Base(g.chartPath),
		Overrides:   overrides,
		Unsupported: unsupported,
	}, nil
}

// extractSubtree extracts a subtree from a map structure based on a path
func extractSubtree(data map[string]interface{}, path []string) map[string]interface{} {
	if len(path) == 0 {
		return nil
	}

	result := make(map[string]interface{})
	current := result
	value := data

	// Build the path structure
	for i, key := range path[:len(path)-1] {
		// Handle array indices
		if strings.HasPrefix(key, "[") && strings.HasSuffix(key, "]") {
			// Extract index
			indexStr := key[1 : len(key)-1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil
			}

			// Get the array from the source data
			parentKey := path[i-1]
			if arr, ok := value[parentKey].([]interface{}); ok && index < len(arr) {
				// Create a new array in the result
				newArr := make([]interface{}, index+1)
				current[parentKey] = newArr

				// Move to the array element
				if mapValue, ok := arr[index].(map[string]interface{}); ok {
					value = mapValue
					newMap := make(map[string]interface{})
					newArr[index] = newMap
					current = newMap
				}
			}
			continue
		}

		// Handle regular map keys
		if nextValue, ok := value[key].(map[string]interface{}); ok {
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
			value = nextValue
		}
	}

	// Set the final value
	lastKey := path[len(path)-1]
	if finalValue, ok := value[lastKey]; ok {
		current[lastKey] = finalValue
	}

	return result
}

// isExcluded checks if a registry is in the exclude list
func (g *Generator) isExcluded(registry string) bool {
	for _, excluded := range g.excludeRegistries {
		if registry == excluded {
			return true
		}
	}
	return false
}

// isSourceRegistry checks if a registry is in the source list
func (g *Generator) isSourceRegistry(registry string) bool {
	for _, source := range g.sourceRegistries {
		if registry == source {
			return true
		}
	}
	return false
}

// GenerateOverrides generates a map of Helm overrides for image references.
// @param chartData: The loaded Helm chart containing values and dependencies
// @param targetRegistry: The target registry where images should be pushed
// @param sourceRegistries: List of source registries to process
// @param excludeRegistries: List of registries to skip
// @param pathStrategy: Strategy for transforming image paths
// @param verbose: Enable verbose logging
// @returns: map[string]interface{} containing the override structure
// @returns: error if processing fails
// @llm-helper This is the main entry point for generating overrides
func GenerateOverrides(chartData *chart.Chart, targetRegistry string, sourceRegistries []string, excludeRegistries []string, pathStrategy strategy.PathStrategy, verbose bool) (map[string]interface{}, error) {
	debug.FunctionEnter("GenerateOverrides")
	defer debug.FunctionExit("GenerateOverrides")

	debug.DumpValue("Target Registry", targetRegistry)
	debug.DumpValue("Source Registries", sourceRegistries)
	debug.DumpValue("Exclude Registries", excludeRegistries)
	debug.DumpValue("Path Strategy", pathStrategy)

	result := make(map[string]interface{})

	// Process the main chart
	debug.Printf("Processing main chart: %s", chartData.Name())
	overrides, err := processChartForOverrides(chartData, targetRegistry, sourceRegistries, excludeRegistries, pathStrategy, verbose)
	if err != nil {
		return nil, fmt.Errorf("error processing main chart: %v", err)
	}
	mergeOverrides(result, overrides)
	debug.DumpValue("Main Chart Overrides", overrides)

	// Process dependencies
	debug.Printf("Processing %d dependencies", len(chartData.Dependencies()))
	for _, dep := range chartData.Dependencies() {
		debug.Printf("Processing dependency: %s", dep.Name())
		depOverrides, err := processChartForOverrides(dep, targetRegistry, sourceRegistries, excludeRegistries, pathStrategy, verbose)
		if err != nil {
			return nil, fmt.Errorf("error processing dependency %s: %v", dep.Name(), err)
		}
		mergeOverrides(result, depOverrides)
	}

	return result, nil
}

// processChartForOverrides processes a single chart and its values.
func processChartForOverrides(chartData *chart.Chart, targetRegistry string, sourceRegistries []string, excludeRegistries []string, pathStrategy strategy.PathStrategy, verbose bool) (map[string]interface{}, error) {
	debug.FunctionEnter("processChartForOverrides")
	defer debug.FunctionExit("processChartForOverrides")

	debug.Printf("Processing chart: %s", chartData.Name())
	debug.DumpValue("Chart Values", chartData.Values)

	// Create a deep copy of the chart values to preserve structure
	baseValues := override.DeepCopy(chartData.Values).(map[string]interface{})

	// Detect images in the chart's values
	detectedImages, unsupported, err := image.DetectImages(chartData.Values, []string{}, sourceRegistries, excludeRegistries, false)
	if err != nil {
		return nil, fmt.Errorf("error detecting images: %w", err)
	}

	debug.Printf("Detected %d images", len(detectedImages))
	debug.Printf("Found %d unsupported structures", len(unsupported))

	// Process each detected image
	for _, img := range detectedImages {
		debug.Printf("Processing image at path: %v", img.Path)
		debug.DumpValue("Image Reference", img.ImageRef)

		// Transform the image reference using the path strategy
		transformedRef := pathStrategy.Transform(img.ImageRef, targetRegistry)
		debug.DumpValue("Transformed Reference", transformedRef)

		// Create the image configuration
		imageConfig := map[string]interface{}{
			"registry":   targetRegistry,
			"repository": transformedRef,
		}
		if img.ImageRef.Tag != "" {
			imageConfig["tag"] = img.ImageRef.Tag
		}
		if img.ImageRef.Digest != "" {
			imageConfig["digest"] = img.ImageRef.Digest
		}

		// Set the value at the correct path
		err := override.SetValueAtPath(baseValues, img.Path, imageConfig)
		if err != nil {
			debug.Printf("Error setting value at path %v: %v", img.Path, err)
			continue
		}
	}

	debug.DumpValue("Final Chart Overrides", baseValues)
	return baseValues, nil
}

// mergeOverrides merges the source map into the destination map recursively.
// @param dest: Destination map to merge into
// @param src: Source map to merge from
// @llm-helper This function handles nested maps and preserves existing values
// @llm-helper Arrays are replaced, not merged
// @llm-helper Map values are merged recursively
func mergeOverrides(dest, src map[string]interface{}) {
	debug.FunctionEnter("mergeOverrides")
	defer debug.FunctionExit("mergeOverrides")

	debug.DumpValue("Destination Map", dest)
	debug.DumpValue("Source Map", src)

	for k, v := range src {
		debug.Printf("Processing key: %s", k)

		if destV, exists := dest[k]; exists {
			debug.Printf("Key %s exists in destination", k)
			// If both values are maps, merge them recursively
			if destMap, ok := destV.(map[string]interface{}); ok {
				if srcMap, ok := v.(map[string]interface{}); ok {
					debug.Printf("Both values are maps, merging recursively")
					mergeOverrides(destMap, srcMap)
					continue
				}
			}
		}

		// Otherwise, just overwrite the value
		debug.Printf("Setting key %s in destination", k)
		dest[k] = v
	}

	debug.DumpValue("Merged Result", dest)
}

// validateHelmTemplate validates the generated overrides by attempting a helm template.
func validateHelmTemplate(chartPath string, overrides []byte) error {
	debug.FunctionEnter("validateHelmTemplate")
	defer debug.FunctionExit("validateHelmTemplate")

	// Create a temporary file for the overrides
	tmpFile, err := os.CreateTemp("", "helm-override-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for validation: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write the overrides to the temporary file
	if err := os.WriteFile(tmpFile.Name(), overrides, 0644); err != nil {
		return fmt.Errorf("failed to write overrides to temporary file: %v", err)
	}

	// Run helm template with the overrides
	cmd := exec.Command("helm", "template", "test", chartPath, "-f", tmpFile.Name())
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Parse the error output to provide more helpful messages
		errorMsg := string(output)

		// Check for common error patterns
		switch {
		case strings.Contains(errorMsg, "could not find template"):
			return fmt.Errorf("helm template error: missing template file - this may indicate a problem with the chart's structure: %v", err)

		case strings.Contains(errorMsg, "parse error"):
			return fmt.Errorf("helm template error: YAML parsing failed - the generated overrides may be malformed: %v", err)

		case strings.Contains(errorMsg, "function \"include\""):
			return fmt.Errorf("helm template error: template include failed - this may indicate a problem with template helpers: %v", err)

		case strings.Contains(errorMsg, "undefined variable"):
			return fmt.Errorf("helm template error: undefined variable - the chart may require additional values not present in the overrides: %v", err)

		case strings.Contains(errorMsg, "no matches for kind"):
			return fmt.Errorf("helm template error: unknown resource kind - the chart may be using custom resource definitions (CRDs) that need to be installed: %v", err)

		default:
			// Extract the relevant portion of the error message
			lines := strings.Split(errorMsg, "\n")
			relevantLines := []string{}
			for _, line := range lines {
				if strings.Contains(line, "Error:") || strings.Contains(line, "error:") {
					relevantLines = append(relevantLines, line)
				}
			}

			if len(relevantLines) > 0 {
				return fmt.Errorf("helm template error: %s", strings.Join(relevantLines, "\n"))
			}
			return fmt.Errorf("helm template failed with error: %v\nOutput: %s", err, errorMsg)
		}
	}

	// Validate the output YAML
	if err := validateYAML(output); err != nil {
		return fmt.Errorf("generated template validation failed: %v", err)
	}

	return nil
}

// validateYAML performs additional validation on the generated YAML.
func validateYAML(yamlData []byte) error {
	// Split the YAML into documents
	docs := bytes.Split(yamlData, []byte("\n---\n"))

	for i, doc := range docs {
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		// Try to parse each document
		var obj map[string]interface{}
		if err := yaml.Unmarshal(doc, &obj); err != nil {
			return fmt.Errorf("invalid YAML in document %d: %v", i+1, err)
		}

		// Check for required Kubernetes fields
		if kind, ok := obj["kind"].(string); ok {
			// Skip empty documents
			if kind == "" {
				continue
			}

			// Validate required fields
			if _, ok := obj["apiVersion"].(string); !ok {
				return fmt.Errorf("missing apiVersion in document %d (kind: %s)", i+1, kind)
			}

			if _, ok := obj["metadata"].(map[string]interface{}); !ok {
				return fmt.Errorf("missing metadata in document %d (kind: %s)", i+1, kind)
			}
		}

		// Check for common issues
		if err := validateCommonIssues(obj); err != nil {
			return fmt.Errorf("validation failed in document %d: %v", i+1, err)
		}
	}

	return nil
}

// validateCommonIssues checks for common Kubernetes manifest issues.
func validateCommonIssues(obj map[string]interface{}) error {
	// Check for invalid null values in required fields
	var checkNulls func(map[string]interface{}, []string) error
	checkNulls = func(m map[string]interface{}, path []string) error {
		for k, v := range m {
			currentPath := append(path, k)
			pathStr := strings.Join(currentPath, ".")

			switch val := v.(type) {
			case nil:
				// Some fields should never be null
				if strings.HasSuffix(k, "Name") ||
					strings.HasSuffix(k, "Path") ||
					k == "key" ||
					k == "value" {
					return fmt.Errorf("field %s cannot be null", pathStr)
				}
			case map[string]interface{}:
				if err := checkNulls(val, currentPath); err != nil {
					return err
				}
			case []interface{}:
				for i, item := range val {
					if m, ok := item.(map[string]interface{}); ok {
						if err := checkNulls(m, append(currentPath, fmt.Sprintf("[%d]", i))); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	}

	return checkNulls(obj, nil)
}

// cleanupTemplateVariables removes or simplifies Helm template variables
func cleanupTemplateVariables(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		// If string contains template variables, return appropriate default
		if strings.Contains(v, "{{") || strings.Contains(v, "${") {
			// For image-related fields, return empty string
			if strings.Contains(strings.ToLower(v), "image") ||
				strings.Contains(strings.ToLower(v), "repository") ||
				strings.Contains(strings.ToLower(v), "registry") ||
				strings.Contains(strings.ToLower(v), "tag") {
				return ""
			}
			// For address fields, return empty string
			if strings.Contains(strings.ToLower(v), "address") {
				return ""
			}
			// For name fields, return empty string
			if strings.Contains(strings.ToLower(v), "name") {
				return ""
			}
			// For path fields, return empty string
			if strings.Contains(strings.ToLower(v), "path") {
				return ""
			}
			// For boolean fields containing "enabled" or "disabled", return false
			if strings.Contains(strings.ToLower(v), "enabled") || strings.Contains(strings.ToLower(v), "disabled") {
				return false
			}
			// Try to extract default value if present
			if strings.Contains(v, "| default") {
				parts := strings.Split(v, "| default")
				if len(parts) > 1 {
					defaultVal := strings.TrimSpace(parts[1])
					defaultVal = strings.TrimSuffix(defaultVal, "}}")
					defaultVal = strings.TrimSpace(defaultVal)
					// Try to convert to appropriate type
					if defaultVal == "true" {
						return true
					}
					if defaultVal == "false" {
						return false
					}
					if i, err := strconv.Atoi(defaultVal); err == nil {
						return i
					}
					return defaultVal
				}
			}
			return ""
		}
		return v
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			if val == nil {
				continue // Skip nil values
			}
			result[key] = cleanupTemplateVariables(val)
		}
		return result
	case []interface{}:
		result := make([]interface{}, 0, len(v))
		for _, item := range v {
			if item == nil {
				continue // Skip nil values
			}
			result = append(result, cleanupTemplateVariables(item))
		}
		return result
	case float64:
		// Convert float64 to int if it's a whole number
		if float64(int(v)) == v {
			return int(v)
		}
		return v
	case nil:
		return nil
	default:
		return v
	}
}

// OverridesToYAML converts a map of overrides to YAML format
func OverridesToYAML(overrides map[string]interface{}) ([]byte, error) {
	return yaml.Marshal(overrides)
}
