package image

import (
	"fmt"
	"strconv"
	"strings"
)

// GetValueAtPath retrieves a value from a nested map structure based on a path.
// It handles maps and slices/arrays within the path.
func GetValueAtPath(data map[string]interface{}, path []string) (interface{}, error) {
	current := interface{}(data)

	for i, key := range path {
		// Check if current level is a map
		if currentMap, ok := current.(map[string]interface{}); ok {
			val, exists := currentMap[key]
			if !exists {
				return nil, fmt.Errorf("path element '%s' (index %d) not found in map", key, i)
			}
			current = val
		} else if currentSlice, ok := current.([]interface{}); ok {
			// Handle slice/array index (e.g., "[0]")
			// nolint:staticcheck // Intentionally keeping current logic for readability
			if !(strings.HasPrefix(key, "[") && strings.HasSuffix(key, "]")) {
				return nil, fmt.Errorf("path element '%s' (index %d) is not an array index, but current value is a slice", key, i)
			}
			indexStr := key[1 : len(key)-1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil, fmt.Errorf("invalid array index '%s' at path index %d: %w", indexStr, i, err)
			}

			if index < 0 || index >= len(currentSlice) {
				return nil, fmt.Errorf("array index %d out of bounds (len %d) for path element '%s' (index %d)", index, len(currentSlice), key, i)
			}
			current = currentSlice[index]
		} else {
			// Current level is not a map or slice, but path continues
			return nil, fmt.Errorf("cannot traverse path further at element '%s' (index %d): value is not a map or slice, it's a %T", key, i, current)
		}
	}

	return current, nil
}

// SetValueAtPath sets a value in a nested map structure based on a path.
// It creates maps and handles slices/arrays as needed.
func SetValueAtPath(data map[string]interface{}, path []string, value interface{}) error {
	if len(path) == 0 {
		return fmt.Errorf("empty path")
	}

	current := data
	for i, key := range path[:len(path)-1] {
		// Check if key is an array index (e.g., "[0]")
		if strings.HasPrefix(key, "[") && strings.HasSuffix(key, "]") {
			indexStr := key[1 : len(key)-1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return fmt.Errorf("invalid array index '%s' at path index %d: %w", indexStr, i, err)
			}

			// Need the previous key to access the parent map/slice
			if i == 0 {
				return fmt.Errorf("cannot have array index as first path element")
			}
			parentKey := path[i-1]

			// Find the parent map that holds the key leading to the array
			parentMap := data // Start search from root
			for j := 0; j < i-1; j++ {
				parentMapElement, exists := parentMap[path[j]]
				if !exists {
					return fmt.Errorf("intermediate path element '%s' not found", path[j])
				}
				var ok bool
				parentMap, ok = parentMapElement.(map[string]interface{})
				if !ok {
					return fmt.Errorf("intermediate path element '%s' is not a map", path[j])
				}
			}

			var arr []interface{}
			if existing, ok := parentMap[parentKey].([]interface{}); ok {
				arr = existing
			} else {
				// If parentKey doesn't exist or isn't a slice, create/overwrite with a new slice
				arr = make([]interface{}, 0)
				parentMap[parentKey] = arr
			}

			// Ensure array is large enough, padding with nil or empty maps if necessary
			for len(arr) <= index {
				// Pad with empty maps if we are not at the second to last element,
				// as we need a map to continue traversing
				if i < len(path)-2 {
					arr = append(arr, make(map[string]interface{}))
				} else {
					arr = append(arr, nil) // Pad with nil otherwise
				}
			}
			parentMap[parentKey] = arr // Update the slice in the parent map

			// Prepare 'current' for the next iteration or final assignment
			if nextMap, ok := arr[index].(map[string]interface{}); ok {
				current = nextMap
			} else if i < len(path)-2 {
				// If we're not at the end and the element isn't a map, create one
				newMap := make(map[string]interface{})
				arr[index] = newMap
				current = newMap
			} else {
				// If we are at the second to last element, 'current' isn't needed for traversal
				// The final assignment will handle setting the value at arr[index]
				continue
			}
		} else { // Handle regular map key
			if next, ok := current[key].(map[string]interface{}); ok {
				current = next
			} else if current[key] == nil || i < len(path)-1 { // Create map if missing or if not the final segment
				newMap := make(map[string]interface{})
				current[key] = newMap
				current = newMap
			} else {
				// Key exists but is not a map, and it's not the final key - error
				return fmt.Errorf("path element '%s' exists but is not a map (%T)", key, current[key])
			}
		}
	}

	// Set the final value
	lastKey := path[len(path)-1]

	// Handle final array index assignment
	if strings.HasPrefix(lastKey, "[") && strings.HasSuffix(lastKey, "]") {
		indexStr := lastKey[1 : len(lastKey)-1]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			return fmt.Errorf("invalid array index '%s' at final path element: %w", indexStr, err)
		}
		parentKey := path[len(path)-2]

		// Find the parent map again
		parentMap := data
		for j := 0; j < len(path)-2; j++ {
			parentMapElement, exists := parentMap[path[j]]
			if !exists {
				return fmt.Errorf("intermediate path element '%s' not found", path[j])
			}
			var ok bool
			parentMap, ok = parentMapElement.(map[string]interface{})
			if !ok {
				return fmt.Errorf("intermediate path element '%s' is not a map", path[j])
			}
		}

		if arr, ok := parentMap[parentKey].([]interface{}); ok {
			if index >= 0 && index < len(arr) {
				arr[index] = value
				parentMap[parentKey] = arr // Update slice back into map
			} else {
				return fmt.Errorf("final array index %d out of bounds (len %d)", index, len(arr))
			}
		} else {
			return fmt.Errorf("path element '%s' is not a slice", parentKey)
		}
	} else { // Handle final map key assignment
		current[lastKey] = value
	}

	return nil
}
