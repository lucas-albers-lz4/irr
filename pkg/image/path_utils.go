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
				return nil, false
			}

			if index < 0 || index >= len(currentSlice) {
				return nil, false
			}
			current = currentSlice[index]
		} else {
			// Current level is not a map or slice, but path continues
			return nil, false
		}
	}

	return current, true
}

// SetValueAtPath sets a value in a nested map structure based on a path.
// It creates maps and handles slices/arrays as needed.
func SetValueAtPath(data map[string]interface{}, path []string, value interface{}) error {
	if len(path) == 0 {
		return fmt.Errorf("empty path")
	}

	current := data
	for i, key := range path[:len(path)-1] {
		// Check if next key is a number (array index)
		nextKey := path[i+1]
		if _, err := strconv.Atoi(nextKey); err == nil {
			// Current key leads to an array
			var arr []interface{}
			if existing, ok := current[key]; ok {
				if arr, ok = existing.([]interface{}); !ok {
					return fmt.Errorf("path element '%s' is not a slice", key)
				}
			} else {
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
			return fmt.Errorf("path element '%s' exists but is not a map (%T)", key, existing)
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
			return fmt.Errorf("cannot have array index as only path element")
		}
		parentKey := path[len(path)-2]

		// Get the array from the current map
		var arr []interface{}
		if existing, ok := current[parentKey]; ok {
			if arr, ok = existing.([]interface{}); !ok {
				return fmt.Errorf("path element '%s' is not a slice", parentKey)
			}
		} else {
			return fmt.Errorf("array not found at path element '%s'", parentKey)
		}

		// Check array bounds
		if index < 0 || index >= len(arr) {
			return fmt.Errorf("array index %d out of bounds (len %d)", index, len(arr))
		}

		// Set the value
		arr[index] = value
		current[parentKey] = arr
	} else { // Handle final map key assignment
		if existing, ok := current[lastKey]; ok {
			if _, ok := existing.(map[string]interface{}); ok {
				return fmt.Errorf("cannot overwrite map at path element '%s'", lastKey)
			}
			if _, ok := existing.([]interface{}); ok {
				return fmt.Errorf("cannot overwrite array at path element '%s'", lastKey)
			}
		}
		current[lastKey] = value
	}

	return nil
}
