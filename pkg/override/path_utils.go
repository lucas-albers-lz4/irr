package override

import (
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

// SetValueAtPath sets a value at a given path in a nested map structure.
// The path is specified as a slice of strings, where each element represents
// a key in the nested structure.
func SetValueAtPath(data map[string]interface{}, path []string, value interface{}) error {
	if data == nil {
		return ErrNilDataMap
	}

	if len(path) == 0 {
		return ErrEmptyPath
	}

	// Handle the last element separately
	lastIdx := len(path) - 1
	m := data

	// Traverse the path except for the last element
	for i := 0; i < lastIdx; i++ {
		part := path[i]
		key, arrayIndex, isArrayAccess, err := parsePathPart(part)
		if err != nil {
			return WrapPathParsing(part, err)
		}

		if isArrayAccess {
			// Verify that arrayIndex is not negative
			if arrayIndex < 0 {
				return WrapNegativeArrayIndex(arrayIndex)
			}

			// Handle array access - first get the array
			arrInterface, exists := m[key]
			if !exists {
				// Create a new array at the specified index
				m[key] = make([]interface{}, arrayIndex+1)
				arrInterface = m[key]
			}

			// Check if the value is actually an array
			arr, ok := arrInterface.([]interface{})
			if !ok {
				return WrapNotAnArray(key)
			}

			// Ensure the array is long enough
			for len(arr) <= arrayIndex {
				arr = append(arr, nil)
			}
			m[key] = arr

			// If the element at the index is a map, continue with it
			if tmp, ok := arr[arrayIndex].(map[string]interface{}); ok {
				m = tmp
			} else if arr[arrayIndex] == nil {
				// If it's nil, create a new map and continue with it
				tmp := make(map[string]interface{})
				arr[arrayIndex] = tmp
				m = tmp
			} else {
				// If it's not a map and not nil, we can't traverse through it
				return WrapNonMapTraversalArray(arrayIndex, arr[arrayIndex])
			}
		} else {
			// Handle regular map access
			if nextM, exists := m[key]; exists {
				if tmp, ok := nextM.(map[string]interface{}); ok {
					m = tmp
				} else {
					// If it exists but is not a map, we can't traverse through it
					return WrapNonMapTraversalMap(key)
				}
			} else {
				// Create a new map and continue with it
				tmp := make(map[string]interface{})
				m[key] = tmp
				m = tmp
			}
		}
	}

	// Handle the last path element
	lastPart := path[lastIdx]
	key, arrayIndex, isArrayAccess, err := parsePathPart(lastPart)
	if err != nil {
		return WrapPathParsing(lastPart, err)
	}

	if isArrayAccess {
		// First get or create the array
		arrInterface, exists := m[key]
		if !exists {
			// Create a new array
			m[key] = make([]interface{}, arrayIndex+1)
			arrInterface = m[key]
		}

		// Ensure it's an array
		arr, ok := arrInterface.([]interface{})
		if !ok {
			return WrapNotAnArray(key)
		}

		// Ensure the array is long enough
		for len(arr) <= arrayIndex {
			arr = append(arr, nil)
		}

		// Set the value at the specified index
		arr[arrayIndex] = value
		m[key] = arr
	} else {
		// Set the value directly in the map
		m[key] = value
	}

	return nil
}

// parsePathPart parses a path part which may include array access.
// Returns the key name, array index (if applicable), and whether it's an array access.
func parsePathPart(part string) (string, int, bool, error) {
	// Check for array access pattern: "key[index]"
	openBracket := strings.Index(part, "[")
	closeBracket := strings.Index(part, "]")

	// If we have an opening bracket, we expect a valid array index
	if openBracket >= 0 {
		if closeBracket <= openBracket {
			return part, 0, false, WrapMalformedArrayIndex(part)
		}
		key := part[:openBracket]
		indexStr := part[openBracket+1 : closeBracket]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			return part, 0, false, WrapInvalidArrayIndex(indexStr, part)
		}
		if index < 0 {
			return part, 0, false, WrapNegativeArrayIndex(index)
		}
		return key, index, true, nil
	}

	// No opening bracket, treat as regular key
	return part, 0, false, nil
}

// ParsePath splits a dot-notation path into segments.
// Handles array indices in the format "key[index]".
// Example: "spec.containers[0].image" -> ["spec", "containers[0]", "image"]
func ParsePath(path string) []string {
	return strings.Split(path, ".")
}
