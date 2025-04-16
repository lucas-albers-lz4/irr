// Package override_test contains tests for the override package, specifically focusing on path utilities.
package override

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeepCopy(t *testing.T) {
	tests := []struct {
		name string
		src  interface{}
	}{
		{
			name: "simple map",
			src: map[string]interface{}{
				"key": "value",
				"num": 42,
			},
		},
		{
			name: "nested map",
			src: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
		},
		{
			name: "array in map",
			src: map[string]interface{}{
				"items": []interface{}{
					"item1",
					map[string]interface{}{"key": "value"},
				},
			},
		},
		{
			name: "primitive types",
			src: map[string]interface{}{
				"string": "value",
				"int":    42,
				"float":  3.14,
				"bool":   true,
				"nil":    nil,
			},
		},
		{
			name: "complex nested structure",
			src: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"image": "nginx:latest",
							"env": []interface{}{
								map[string]interface{}{
									"name":  "DEBUG",
									"value": "true",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "empty map",
			src:  map[string]interface{}{},
		},
		{
			name: "nil value",
			src:  nil,
		},
		{
			name: "non-map value",
			src:  "just a string",
		},
		{
			name: "array with nil values",
			src: map[string]interface{}{
				"items": []interface{}{nil, "value", nil},
			},
		},
		{
			name: "map with empty arrays",
			src: map[string]interface{}{
				"empty": []interface{}{},
				"full":  []interface{}{"value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeepCopy(tt.src)

			// Verify the copy is equal but not the same instance
			if !reflect.DeepEqual(tt.src, result) {
				t.Errorf("DeepCopy() = %v, want %v", result, tt.src)
			}

			// For maps, verify it's a different instance
			if m, ok := tt.src.(map[string]interface{}); ok {
				if result != nil && reflect.ValueOf(result).Pointer() == reflect.ValueOf(m).Pointer() {
					t.Error("DeepCopy() returned same map instance")
				}
			}

			// For arrays, verify they're different instances
			srcMap, srcIsMap := tt.src.(map[string]interface{})
			resultMap, resultIsMap := result.(map[string]interface{})

			// Use guard clauses to reduce nesting when checking array instances
			if !srcIsMap || !resultIsMap {
				return // Skip if either source or result is not a map
			}

			for k, srcVal := range srcMap {
				srcArr, srcIsArr := srcVal.([]interface{})
				if !srcIsArr {
					continue // Skip if source value is not an array
				}

				resultVal, resultHasKey := resultMap[k]
				if !resultHasKey {
					t.Errorf("DeepCopy() result missing key %s", k)
					continue
				}

				resultArr, resultIsArr := resultVal.([]interface{})
				if !resultIsArr {
					t.Errorf("DeepCopy() value at key %s is not []interface{}", k)
					continue
				}

				// Check for same instance only if both arrays are non-empty
				if len(srcArr) == 0 || len(resultArr) == 0 {
					continue
				}

				if reflect.ValueOf(srcArr).Pointer() == reflect.ValueOf(resultArr).Pointer() {
					t.Errorf("DeepCopy() returned same array instance for key %s", k)
				}
			}
		})
	}
}

func TestSetValueAtPath(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]interface{}
		path      []string
		value     interface{}
		wantData  map[string]interface{}
		wantError bool
	}{
		{
			name:  "simple path",
			data:  map[string]interface{}{},
			path:  []string{"key"},
			value: "value",
			wantData: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name:  "nested path",
			data:  map[string]interface{}{},
			path:  []string{"outer", "inner"},
			value: "value",
			wantData: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
		},
		{
			name:  "array path",
			data:  map[string]interface{}{},
			path:  []string{"items[0]"},
			value: "value",
			wantData: map[string]interface{}{
				"items": []interface{}{"value"},
			},
		},
		{
			name:  "nested array path",
			data:  map[string]interface{}{},
			path:  []string{"spec", "containers[0]", "image"},
			value: "nginx:latest",
			wantData: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"image": "nginx:latest",
						},
					},
				},
			},
		},
		{
			name:  "multiple array indices",
			data:  map[string]interface{}{},
			path:  []string{"spec", "containers[1]", "image"},
			value: "nginx:latest",
			wantData: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{nil, map[string]interface{}{
						"image": "nginx:latest",
					}},
				},
			},
		},
		{
			name:      "empty path",
			data:      map[string]interface{}{},
			path:      []string{},
			value:     "value",
			wantError: true,
		},
		{
			name:      "nil data",
			data:      nil,
			path:      []string{"key"},
			value:     "value",
			wantError: true,
		},
		{
			name:      "invalid array index",
			data:      map[string]interface{}{},
			path:      []string{"items[a]"},
			value:     "value",
			wantError: true,
		},
		{
			name:      "negative array index",
			data:      map[string]interface{}{},
			path:      []string{"items[-1]"},
			value:     "value",
			wantError: true,
		},
		{
			name: "overwrite existing value",
			data: map[string]interface{}{
				"key": "old value",
			},
			path:  []string{"key"},
			value: "new value",
			wantData: map[string]interface{}{
				"key": "new value",
			},
		},
		{
			name: "overwrite existing map with value",
			data: map[string]interface{}{
				"key": map[string]interface{}{
					"inner": "value",
				},
			},
			path:  []string{"key"},
			value: "new value",
			wantData: map[string]interface{}{
				"key": "new value",
			},
		},
		{
			name: "overwrite existing value with map",
			data: map[string]interface{}{
				"key": "old value",
			},
			path: []string{"key"},
			value: map[string]interface{}{
				"inner": "value",
			},
			wantData: map[string]interface{}{
				"key": map[string]interface{}{
					"inner": "value",
				},
			},
		},
		{
			name: "extend existing array",
			data: map[string]interface{}{
				"items": []interface{}{"item1"},
			},
			path:  []string{"items[1]"},
			value: "item2",
			wantData: map[string]interface{}{
				"items": []interface{}{"item1", "item2"},
			},
		},
		{
			name:  "skip array indices",
			data:  map[string]interface{}{},
			path:  []string{"items[2]"},
			value: "value",
			wantData: map[string]interface{}{
				"items": []interface{}{nil, nil, "value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetValueAtPath(tt.data, tt.path, tt.value, false) // Pass false for debug

			if tt.wantError {
				if err == nil {
					t.Error("SetValueAtPath() expected error, got nil")
				}
				return // Expect error, so don't check data
			}

			if err != nil {
				t.Errorf("SetValueAtPath() unexpected error: %v", err)
				return
			}

			if !reflect.DeepEqual(tt.data, tt.wantData) {
				t.Errorf("SetValueAtPath() got = %v, want %v", tt.data, tt.wantData)
			}
		})
	}
}

func TestParsePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "simple path",
			path:     "key",
			expected: []string{"key"},
		},
		{
			name:     "nested path",
			path:     "outer.inner",
			expected: []string{"outer", "inner"},
		},
		{
			name:     "array path",
			path:     "items[0]",
			expected: []string{"items[0]"},
		},
		{
			name:     "complex path",
			path:     "spec.containers[0].image",
			expected: []string{"spec", "containers[0]", "image"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParsePath(tt.path)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParsePath() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseArrayPath(t *testing.T) {
	tests := []struct {
		name         string
		part         string
		wantKey      string
		wantIndex    int
		wantHasIndex bool
		wantErr      bool
	}{
		{
			name:         "simple key",
			part:         "image",
			wantKey:      "image",
			wantIndex:    -1,
			wantHasIndex: false,
			wantErr:      false,
		},
		{
			name:         "array index",
			part:         "containers[0]",
			wantKey:      "containers",
			wantIndex:    0,
			wantHasIndex: true,
			wantErr:      false,
		},
		{
			name:         "malformed array index - no closing bracket",
			part:         "containers[0",
			wantKey:      "",
			wantIndex:    0,
			wantHasIndex: false,
			wantErr:      true,
		},
		{
			name:         "malformed_array_index_-_no_opening_bracket",
			part:         "containers0]",
			wantKey:      "containers0]",
			wantIndex:    0,
			wantHasIndex: false,
			wantErr:      true,
		},
		{
			name:         "non-integer array index",
			part:         "containers[abc]",
			wantKey:      "",
			wantIndex:    0,
			wantHasIndex: false,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, index, hasIndex, err := parsePathPart(tt.part)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parsePathPart() expected error for invalid input '%s'", tt.part)
				}
				return
			}
			if err != nil {
				t.Errorf("parsePathPart() returned unexpected error for valid input '%s': %v", tt.part, err)
				return
			}
			if key != tt.wantKey {
				t.Errorf("parsePathPart() key = %v, want %v", key, tt.wantKey)
			}
			if hasIndex != tt.wantHasIndex {
				t.Errorf("parsePathPart() hasIndex = %v, want %v", hasIndex, tt.wantHasIndex)
			}
			if index != tt.wantIndex {
				t.Errorf("parsePathPart() index = %v, want %v", index, tt.wantIndex)
			}
		})
	}
}

func TestGetValueAtPath(t *testing.T) {
	// Test data structure with various nested elements
	data := map[string]interface{}{
		"simple":  "value",
		"number":  42,
		"boolean": true,
		"nested": map[string]interface{}{
			"inner": "nested-value",
			"deep": map[string]interface{}{
				"deepest": "very-deep",
			},
		},
		"array": []interface{}{
			"first",
			"second",
			map[string]interface{}{
				"name": "third-item",
			},
		},
		"mixed": map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{
					"prop": "item1",
				},
				map[string]interface{}{
					"prop": "item2",
				},
			},
		},
		"empty": map[string]interface{}{},
	}

	tests := []struct {
		name        string
		data        map[string]interface{}
		path        []string
		expected    interface{}
		expectError bool
		errorType   error
	}{
		{
			name:     "simple key",
			data:     data,
			path:     []string{"simple"},
			expected: "value",
		},
		{
			name:     "number value",
			data:     data,
			path:     []string{"number"},
			expected: 42,
		},
		{
			name:     "boolean value",
			data:     data,
			path:     []string{"boolean"},
			expected: true,
		},
		{
			name:     "nested key",
			data:     data,
			path:     []string{"nested", "inner"},
			expected: "nested-value",
		},
		{
			name:     "deeply nested key",
			data:     data,
			path:     []string{"nested", "deep", "deepest"},
			expected: "very-deep",
		},
		{
			name:     "array element by index",
			data:     data,
			path:     []string{"array[0]"},
			expected: "first",
		},
		{
			name:     "array element by index - second element",
			data:     data,
			path:     []string{"array[1]"},
			expected: "second",
		},
		{
			name:     "nested property in array element",
			data:     data,
			path:     []string{"array[2]", "name"},
			expected: "third-item",
		},
		{
			name:     "complex path with arrays and objects",
			data:     data,
			path:     []string{"mixed", "list[1]", "prop"},
			expected: "item2",
		},
		{
			name:     "empty path",
			data:     data,
			path:     []string{},
			expected: data, // The function returns the entire data map for empty path
		},
		{
			name:        "nil data",
			data:        nil,
			path:        []string{"key"},
			expectError: true,
			errorType:   ErrNilDataMap,
		},
		{
			name:        "non-existent key",
			data:        data,
			path:        []string{"nonexistent"},
			expectError: true,
			errorType:   ErrPathNotFound,
		},
		{
			name:        "non-existent nested key",
			data:        data,
			path:        []string{"nested", "nonexistent"},
			expectError: true,
			errorType:   ErrPathNotFound,
		},
		{
			name:        "array index out of bounds",
			data:        data,
			path:        []string{"array[99]"},
			expectError: true,
			errorType:   ErrArrayIndexOutOfBounds,
		},
		{
			name:        "invalid array index format",
			data:        data,
			path:        []string{"array[abc]"},
			expectError: true,
			errorType:   ErrPathParsing,
		},
		{
			name:        "traversal through primitive",
			data:        data,
			path:        []string{"simple", "deeper"},
			expectError: true,
			errorType:   ErrNonMapOrArrayTraversal,
		},
		{
			name:        "empty map traversal",
			data:        data,
			path:        []string{"empty", "nonexistent"},
			expectError: true,
			errorType:   ErrPathNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := GetValueAtPath(tt.data, tt.path)

			if tt.expectError {
				if assert.Error(t, err) {
					// Check that the error is of the expected type
					if tt.errorType != nil {
						assert.True(t, errors.Is(err, tt.errorType),
							"Expected error type %v, got %v", tt.errorType, err)
					}
				}
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, value)
		})
	}
}

// getSimpleMergeMapTestCases returns test cases for simple map merging scenarios
func getSimpleMergeMapTestCases() []struct {
	name     string
	dst      map[string]interface{}
	src      map[string]interface{}
	expected map[string]interface{}
} {
	return []struct {
		name     string
		dst      map[string]interface{}
		src      map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "simple merge - non-overlapping keys",
			dst: map[string]interface{}{
				"a": "valueA",
				"b": "valueB",
			},
			src: map[string]interface{}{
				"c": "valueC",
				"d": "valueD",
			},
			expected: map[string]interface{}{
				"a": "valueA",
				"b": "valueB",
				"c": "valueC",
				"d": "valueD",
			},
		},
		{
			name: "simple merge - overlapping keys",
			dst: map[string]interface{}{
				"a": "valueA",
				"b": "valueB",
			},
			src: map[string]interface{}{
				"b": "newValueB",
				"c": "valueC",
			},
			expected: map[string]interface{}{
				"a": "valueA",
				"b": "newValueB",
				"c": "valueC",
			},
		},
		{
			name: "nested merge - overlapping maps",
			dst: map[string]interface{}{
				"a": map[string]interface{}{
					"x": "valueX",
					"y": "valueY",
				},
			},
			src: map[string]interface{}{
				"a": map[string]interface{}{
					"y": "newValueY",
					"z": "valueZ",
				},
			},
			expected: map[string]interface{}{
				"a": map[string]interface{}{
					"x": "valueX",
					"y": "newValueY",
					"z": "valueZ",
				},
			},
		},
		{
			name: "type conflict - map vs primitive",
			dst: map[string]interface{}{
				"key": map[string]interface{}{
					"x": "valueX",
				},
			},
			src: map[string]interface{}{
				"key": "primitive",
			},
			expected: map[string]interface{}{
				"key": "primitive",
			},
		},
		{
			name: "type conflict - primitive vs map",
			dst: map[string]interface{}{
				"key": "primitive",
			},
			src: map[string]interface{}{
				"key": map[string]interface{}{
					"x": "valueX",
				},
			},
			expected: map[string]interface{}{
				"key": map[string]interface{}{
					"x": "valueX",
				},
			},
		},
	}
}

// getComplexMergeMapTestCases returns test cases for complex map merging scenarios
func getComplexMergeMapTestCases() []struct {
	name     string
	dst      map[string]interface{}
	src      map[string]interface{}
	expected map[string]interface{}
} {
	return []struct {
		name     string
		dst      map[string]interface{}
		src      map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "array handling",
			dst: map[string]interface{}{
				"array": []interface{}{1, 2, 3},
			},
			src: map[string]interface{}{
				"array": []interface{}{4, 5, 6},
			},
			expected: map[string]interface{}{
				"array": []interface{}{4, 5, 6},
			},
		},
		{
			name: "deep nesting (3+ levels)",
			dst: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"a": "valueA",
							"b": "valueB",
						},
					},
				},
			},
			src: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"b": "newValueB",
							"c": "valueC",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"a": "valueA",
							"b": "newValueB",
							"c": "valueC",
						},
					},
				},
			},
		},
		{
			name: "edge case - nil source map",
			dst: map[string]interface{}{
				"a": "valueA",
			},
			src: nil,
			expected: map[string]interface{}{
				"a": "valueA",
			},
		},
		{
			name: "edge case - empty source map",
			dst: map[string]interface{}{
				"a": "valueA",
			},
			src: map[string]interface{}{},
			expected: map[string]interface{}{
				"a": "valueA",
			},
		},
		{
			name: "edge case - empty destination map",
			dst:  map[string]interface{}{},
			src: map[string]interface{}{
				"a": "valueA",
			},
			expected: map[string]interface{}{
				"a": "valueA",
			},
		},
		{
			name: "complex merge with multiple types",
			dst: map[string]interface{}{
				"string": "value",
				"number": 42,
				"bool":   true,
				"array":  []interface{}{1, 2, 3},
				"map": map[string]interface{}{
					"key": "value",
				},
				"nested": map[string]interface{}{
					"array": []interface{}{
						map[string]interface{}{
							"name": "item1",
						},
					},
				},
			},
			src: map[string]interface{}{
				"string": "newValue",
				"array":  []interface{}{4, 5, 6},
				"map": map[string]interface{}{
					"key":    "newValue",
					"newKey": "value",
					"submap": map[string]interface{}{
						"key": "value",
					},
				},
				"nested": map[string]interface{}{
					"array": []interface{}{
						map[string]interface{}{
							"name": "item2",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"string": "newValue",
				"number": 42,
				"bool":   true,
				"array":  []interface{}{4, 5, 6},
				"map": map[string]interface{}{
					"key":    "newValue",
					"newKey": "value",
					"submap": map[string]interface{}{
						"key": "value",
					},
				},
				"nested": map[string]interface{}{
					"array": []interface{}{
						map[string]interface{}{
							"name": "item2",
						},
					},
				},
			},
		},
	}
}

func TestMergeMaps(t *testing.T) {
	// Combine all test cases
	tests := append(getSimpleMergeMapTestCases(), getComplexMergeMapTestCases()...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of dst to avoid modifying the test case
			dstCopyInterface := DeepCopy(tt.dst)
			dstCopy, ok := dstCopyInterface.(map[string]interface{})
			if !ok && dstCopyInterface != nil {
				t.Fatalf("Expected DeepCopy result to be map[string]interface{} or nil, got %T", dstCopyInterface)
			}

			// Test the function
			result := mergeMaps(dstCopy, tt.src)

			// Check the result
			assert.Equal(t, tt.expected, result, "Maps should be merged correctly")

			// Since mergeMaps modifies the dst map in place, dstCopy and result should be equal
			assert.Equal(t, dstCopy, result, "Result should equal the modified destination map")
		})
	}
}
