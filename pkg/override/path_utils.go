// Package override provides utility functions for working with nested map paths used in Helm overrides.
package override

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
)

//nolint:unused // Kept for potential future uses
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

	if m == nil {
		m = make(map[string]interface{})
	}

	// Debug logging before setting the value
	log.Debug(fmt.Sprintf("[DEBUG irr SPATH] Target Map (m) before setting key '%s': %#v", key, m))
	log.Debug(fmt.Sprintf("[DEBUG irr SPATH] Path: %v, Key: %s, IsArray: %v, ArrayIndex: %d", path, key, isArrayAccess, arrayIndex))
	log.Debug(fmt.Sprintf("[DEBUG irr SPATH] Value to set: %#v", value))

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
		// Set the value in the map, merging if both old and new are maps
		existingVal, exists := m[key]
		if exists {
			existingMap, existingIsMap := existingVal.(map[string]interface{})
			newMap, newIsMap := value.(map[string]interface{})

			if existingIsMap && newIsMap {
				// Both are maps, merge the new one into the existing one
				m[key] = mergeMaps(existingMap, newMap)
			} else {
				// Existing is not a map, OR new value is not a map.
				// Overwrite the existing value.
				m[key] = value
			}
		} else {
			// Key doesn't exist, set the value directly
			m[key] = value
		}
	}

	// Debug logging after setting the value
	log.Debug(fmt.Sprintf("[DEBUG irr SPATH] Target Map (m) AFTER setting key '%s': %#v", key, m))

	// Debug logging for the final state of the target map for the current key
	log.Debug(fmt.Sprintf("[DEBUG irr SPATH] FINAL Target Map (m) state for key '%s': %#v", key, m))

	return nil
}

// mergeMaps recursively merges src map into dst map.
// It overwrites primitive values in dst with values from src.
// Nested maps are merged recursively.
func mergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	for key, srcVal := range src {
		if dstVal, exists := dst[key]; exists {
			srcMap, srcIsMap := srcVal.(map[string]interface{})
			dstMap, dstIsMap := dstVal.(map[string]interface{})
			if srcIsMap && dstIsMap {
				// Both are maps, recurse
				dst[key] = mergeMaps(dstMap, srcMap)
			} else {
				// Overwrite dst with src value (including overwriting map with primitive or vice versa)
				dst[key] = srcVal
			}
		} else {
			// Key doesn't exist in dst, add it
			dst[key] = srcVal
		}
	}
	return dst
}

// parsePathPart parses a single path component, detecting array access.
// It expects array access in the format "key[index]".
func parsePathPart(part string) (key string, index int, isArray bool, err error) {
	hasLeftBracket := strings.Contains(part, "[")
	hasRightBracket := strings.HasSuffix(part, "]")

	switch {
	case hasLeftBracket && hasRightBracket:
		// Potential array access, validate structure
		leftBracketPos := strings.LastIndex(part, "[")
		// Ensure brackets are ordered correctly and not adjacent
		if leftBracketPos == -1 || leftBracketPos >= len(part)-2 { // Need at least one char for index
			err = fmt.Errorf("invalid array access format in %s: malformed brackets", part)
			return
		}

		key = part[:leftBracketPos]
		indexStr := part[leftBracketPos+1 : len(part)-1]
		index, err = strconv.Atoi(indexStr)
		if err != nil {
			err = fmt.Errorf("invalid array index '%s' in %s: %w", indexStr, part, ErrInvalidArrayIndex)
			return
		}
		if index < 0 {
			err = fmt.Errorf("negative array index %d in %s: %w", index, part, ErrInvalidArrayIndex)
			return
		}
		isArray = true
	case !hasLeftBracket && !hasRightBracket:
		// Simple key, no brackets
		key = part
		isArray = false
		index = -1 // Convention for non-array parts
	default:
		// Mismatched brackets (e.g., "key[" or "key]") - treat as error
		err = fmt.Errorf("invalid array access format in %s: mismatched brackets", part)
		// Explicitly set return values for clarity, though error is primary
		key = ""
		index = -1
		isArray = false
	}
	return // Returns key, index, isArray, err
}

// GetValueAtPath retrieves a value from a nested map structure at a given path.
func GetValueAtPath(data map[string]interface{}, path []string) (interface{}, error) {
	if data == nil {
		return nil, ErrNilDataMap
	}
	if len(path) == 0 {
		return data, nil // Return the whole map if path is empty
	}

	current := interface{}(data)

	for i, part := range path {
		key, arrayIndex, isArrayAccess, err := parsePathPart(part)
		if err != nil {
			return nil, WrapPathParsing(part, err)
		}

		switch typedCurrent := current.(type) {
		case map[string]interface{}:
			nextVal, exists := typedCurrent[key]
			if !exists {
				return nil, WrapPathNotFound(path[:i+1])
			}

			if isArrayAccess {
				if arrayVal, ok := nextVal.([]interface{}); ok {
					if arrayIndex < 0 || arrayIndex >= len(arrayVal) {
						return nil, WrapArrayIndexOutOfBounds(arrayIndex, len(arrayVal))
					}
					current = arrayVal[arrayIndex]
				} else {
					return nil, WrapNotAnArray(key)
				}
			} else {
				current = nextVal
			}
		case []interface{}:
			// Handle accessing elements within an array directly
			index, convErr := strconv.Atoi(key) // Assuming key is the index string
			if convErr != nil || index < 0 || index >= len(typedCurrent) {
				return nil, WrapArrayIndexOutOfBounds(index, len(typedCurrent))
			}
			current = typedCurrent[index]
		default:
			// Cannot traverse further if not a map or array
			return nil, WrapNonMapOrArrayTraversal(path[:i])
		}
	}

	return current, nil
}

// ParsePath splits a dot-notation path into segments.
// Handles array indices in the format "key[index]".
// Example: "spec.containers[0].image" -> ["spec", "containers[0]", "image"]
func ParsePath(path string) []string {
	return strings.Split(path, ".")
}
