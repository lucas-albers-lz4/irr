package override

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

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
		key, index, hasIndex := parseArrayPath(part)

		if hasIndex {
			// Handle array indexing
			if _, exists := current[key]; !exists {
				// Create new array if it doesn't exist
				arr := make([]interface{}, index+1)
				// Initialize all elements as empty maps
				for j := range arr {
					arr[j] = make(map[string]interface{})
				}
				current[key] = arr
			}

			arr, ok := current[key].([]interface{})
			if !ok {
				return fmt.Errorf("path element %s exists but is not an array", key)
			}

			// Expand array if needed, initializing new elements as empty maps
			for len(arr) <= index {
				arr = append(arr, make(map[string]interface{}))
			}
			current[key] = arr

			if isLast {
				arr[index] = value
			} else {
				// Ensure we have a map at this index for further traversal
				if arr[index] == nil {
					arr[index] = make(map[string]interface{})
				}
				nextMap, ok := arr[index].(map[string]interface{})
				if !ok {
					return fmt.Errorf("cannot traverse through non-map at index %d", index)
				}
				current = nextMap
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
// Returns the key, index, and whether an index was found.
// Example: "containers[0]" -> "containers", 0, true
func parseArrayPath(part string) (string, int, bool) {
	start := strings.Index(part, "[")
	end := strings.Index(part, "]")

	if start != -1 && end != -1 && start < end {
		key := part[:start]
		indexStr := part[start+1 : end]
		if index, err := strconv.Atoi(indexStr); err == nil {
			return key, index, true
		}
	}
	return part, 0, false
}
