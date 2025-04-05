package override

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// nolint:unused // Kept for potential future uses
var arrayIndexPattern = regexp.MustCompile(`^(.*)\[(\d+)\]$`)

// DeepCopy creates a deep copy of a map[string]interface{} structure.
// It handles nested maps, slices, and primitive values.
func DeepCopy(src interface{}) interface{} {
	switch v := src.(type) {
	case map[string]interface{}:
		dst := make(map[string]interface{}, len(v))
		for key, value := range v {
			dst[key] = DeepCopy(value)
		}
		return dst
	case []interface{}:
		dst := make([]interface{}, len(v))
		for i, value := range v {
			dst[i] = DeepCopy(value)
		}
		return dst
	// Handle primitive types that don't need deep copying
	case nil, string, bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, float32, float64:
		return v
	default:
		// For any other types, return as is (should be rare in YAML structures)
		return v
	}
}

// SetValueAtPath sets a value in a nested map structure using a path.
// The path is a slice of strings representing the keys to traverse.
// It automatically creates intermediate maps if they don't exist.
// For array access, use the format "key[index]" in the path element.
func SetValueAtPath(data map[string]interface{}, path []string, value interface{}) error {
	if data == nil {
		return fmt.Errorf("data map cannot be nil")
	}
	if len(path) == 0 {
		return fmt.Errorf("empty path")
	}

	current := data
	for i, part := range path {
		isLast := i == len(path)-1

		// Check if this path part contains an array index
		key, index, hasIndex, err := parseArrayPath(part)
		if err != nil {
			return fmt.Errorf("error parsing path part '%s': %w", part, err)
		}

		if hasIndex {
			// Validate index is valid
			if index < 0 {
				return fmt.Errorf("negative array index: %d", index)
			}

			// Check if key exists
			if _, exists := current[key]; !exists {
				// Create new array if it doesn't exist, initialize with nils
				arr := make([]interface{}, index+1)
				current[key] = arr
			}

			arr, ok := current[key].([]interface{})
			if !ok {
				return fmt.Errorf("path element %s exists but is not an array", key)
			}

			// Expand array if needed, padding with nil
			for len(arr) <= index {
				arr = append(arr, nil) // Pad with nil
			}
			current[key] = arr // Update the map with the potentially resized array

			if isLast {
				arr[index] = value // Set the final value
			} else {
				// If not the last element, we need to traverse into this index.
				// Ensure the element at the current index is a map.
				if arr[index] == nil {
					// If it's nil (because it was just padded), initialize it as a map.
					arr[index] = make(map[string]interface{})
				}

				nextMap, ok := arr[index].(map[string]interface{})
				if !ok {
					// If it exists but isn't a map, and we need to traverse, it's an error.
					return fmt.Errorf("cannot traverse through non-map at index %d which holds value %T", index, arr[index])
				}
				current = nextMap // Continue traversal into the map at the current index
			}
		} else {
			// Handle regular map keys
			if isLast {
				current[key] = value
			} else {
				if _, exists := current[key]; !exists {
					current[key] = make(map[string]interface{})
				}
				nextMap, ok := current[key].(map[string]interface{})
				if !ok {
					return fmt.Errorf("cannot traverse through non-map at key %s", key)
				}
				current = nextMap
			}
		}
	}
	return nil
}

// ParsePath splits a dot-notation path into segments.
// Handles array indices in the format "key[index]".
// Example: "spec.containers[0].image" -> ["spec", "containers[0]", "image"]
func ParsePath(path string) []string {
	return strings.Split(path, ".")
}

// parseArrayPath extracts the key and index from a path segment that may contain an array index.
// Returns the key, index, whether an index was found, and an error if syntax is invalid.
func parseArrayPath(part string) (string, int, bool, error) {
	start := strings.Index(part, "[")
	end := strings.Index(part, "]")

	if start != -1 && end != -1 && start < end && end == len(part)-1 {
		key := part[:start]
		indexStr := part[start+1 : end]
		index, err := strconv.Atoi(indexStr)
		if err == nil {
			return key, index, true, nil
		} else {
			return part, 0, false, fmt.Errorf("invalid non-integer array index '%s' in path part '%s'", indexStr, part)
		}
	} else if start != -1 || end != -1 {
		return part, 0, false, fmt.Errorf("malformed array index syntax in path part '%s'", part)
	}

	return part, 0, false, nil
}
