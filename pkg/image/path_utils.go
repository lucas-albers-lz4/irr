package image

import (
	"fmt"
	"strconv"
)

// GetValueAtPath retrieves a value from a nested map structure based on a path.
// It handles maps and slices/arrays within the path.
// Returns the value and true if found, (nil, false) if not found.
func GetValueAtPath(data map[string]interface{}, path []string) (interface{}, bool) {
	current := interface{}(data)

	for _, key := range path {
		// Check if current level is a map
		if currentMap, ok := current.(map[string]interface{}); ok {
			val, exists := currentMap[key]
			if !exists {
				return nil, false
			}
			current = val
		} else if currentSlice, ok := current.([]interface{}); ok {
			// Handle slice/array index
			index, err := strconv.Atoi(key)
			if err != nil {
				return nil, false // Invalid index format, path doesn't match
			}

			if index < 0 || index >= len(currentSlice) {
				return nil, false // Index out of bounds
			}
			current = currentSlice[index]
		} else {
			// Current level is not a map or slice, but path continues
			return nil, false // Path mismatch
		}
	}

	return current, true
}

// SetValueAtPath sets a value in a nested map structure based on a path.
// It creates maps and handles slices/arrays as needed.
func SetValueAtPath(data map[string]interface{}, path []string, value interface{}) error {
	if len(path) == 0 {
		return ErrEmptyPath
	}

	current := data

	// Navigate through the path, creating maps as needed
	for i, key := range path[:len(path)-1] {
		// Check if next key is a number (array index)
		nextKey := path[i+1]
		if _, err := strconv.Atoi(nextKey); err == nil {
			// Current key leads to an array
			var arr []interface{}
			if existing, ok := current[key]; ok {
				// Check if the existing value is a slice
				_, isSlice := existing.([]interface{})
				if !isSlice {
					return fmt.Errorf("%w: %s", ErrPathElementNotSlice, key)
				}
				// If it exists and is a slice, we don't need to modify 'arr' or 'current[key]', just continue.
			} else {
				// If the key doesn't exist, create a new empty slice
				arr = make([]interface{}, 0)
				current[key] = arr
			}
			continue
		}

		// Handle regular map key
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else if existing, ok := current[key]; ok {
			// Key exists but is not a map
			return fmt.Errorf("%w: %s (found type %T)", ErrPathElementNotMap, key, existing)
		} else {
			// Create map if missing
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
		}
	}

	// Set the final value
	lastKey := path[len(path)-1]

	// Handle final array index assignment
	if index, err := strconv.Atoi(lastKey); err == nil {
		if len(path) < 2 {
			return ErrArrayIndexAsOnlyElement
		}
		parentKey := path[len(path)-2]

		// Get the array from the current map
		var arr []interface{}
		if existing, ok := current[parentKey]; ok {
			var isSlice bool
			arr, isSlice = existing.([]interface{})
			if !isSlice {
				return fmt.Errorf("%w: %s", ErrPathElementNotSlice, parentKey)
			}
		} else {
			return fmt.Errorf("array %w at path element '%s'", ErrPathNotFound, parentKey)
		}

		// Check array bounds
		if index < 0 {
			return fmt.Errorf("negative %w: %d", ErrInvalidArrayIndex, index)
		} else if index >= len(arr) {
			return fmt.Errorf("out of bounds %w: %d (array length: %d)", ErrInvalidArrayIndex, index, len(arr))
		}

		// Set the value
		arr[index] = value
		current[parentKey] = arr
	} else { // Handle final map key assignment
		if existing, ok := current[lastKey]; ok {
			if _, ok := existing.(map[string]interface{}); ok {
				return fmt.Errorf("%w at path element '%s'", ErrCannotOverwriteStructure, lastKey)
			}
			if _, ok := existing.([]interface{}); ok {
				return fmt.Errorf("%w at path element '%s'", ErrCannotOverwriteStructure, lastKey)
			}
		}
		current[lastKey] = value
	}

	return nil
}
