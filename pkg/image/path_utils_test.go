package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetValueAtPath(t *testing.T) {
	tests := []struct {
		name          string
		values        map[string]interface{}
		path          []string
		expected      interface{}
		expectedFound bool
	}{
		{
			name: "simple path",
			values: map[string]interface{}{
				"key": "value",
			},
			path:          []string{"key"},
			expected:      "value",
			expectedFound: true,
		},
		{
			name: "nested path",
			values: map[string]interface{}{
				"parent": map[string]interface{}{
					"child": "value",
				},
			},
			path:          []string{"parent", "child"},
			expected:      "value",
			expectedFound: true,
		},
		{
			name: "array path",
			values: map[string]interface{}{
				"items": []interface{}{
					"first",
					"second",
				},
			},
			path:          []string{"items", "1"},
			expected:      "second",
			expectedFound: true,
		},
		{
			name: "missing path",
			values: map[string]interface{}{
				"key": "value",
			},
			path:          []string{"missing"},
			expected:      nil,
			expectedFound: false,
		},
		{
			name: "invalid array index",
			values: map[string]interface{}{
				"items": []interface{}{
					"first",
				},
			},
			path:          []string{"items", "1"},
			expected:      nil,
			expectedFound: false,
		},
		{
			name: "invalid path type",
			values: map[string]interface{}{
				"key": "value",
			},
			path:          []string{"key", "invalid"},
			expected:      nil,
			expectedFound: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, found := GetValueAtPath(tc.values, tc.path)
			assert.Equal(t, tc.expectedFound, found)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSetValueAtPath(t *testing.T) {
	tests := []struct {
		name          string
		values        map[string]interface{}
		path          []string
		value         interface{}
		expected      map[string]interface{}
		expectedError bool
		errorContains string
	}{
		{
			name:          "set_simple_value",
			values:        map[string]interface{}{"key": "old"},
			path:          []string{"key"},
			value:         "new",
			expected:      map[string]interface{}{"key": "new"},
			expectedError: false,
		},
		{
			name:          "set_nested_value",
			values:        map[string]interface{}{"parent": map[string]interface{}{"child": "old"}},
			path:          []string{"parent", "child"},
			value:         "new",
			expected:      map[string]interface{}{"parent": map[string]interface{}{"child": "new"}},
			expectedError: false,
		},
		{
			name:          "create_nested_path",
			values:        map[string]interface{}{"existing": "data"},
			path:          []string{"new", "nested", "key"},
			value:         "value",
			expected:      map[string]interface{}{"existing": "data", "new": map[string]interface{}{"nested": map[string]interface{}{"key": "value"}}},
			expectedError: false,
		},
		{
			name:          "set_array_element",
			values:        map[string]interface{}{"key": []interface{}{0, 1, 2}},
			path:          []string{"key", "1"},
			value:         "new",
			expected:      map[string]interface{}{"key": []interface{}{0, "new", 2}},
			expectedError: false,
		},
		{
			name:          "invalid_array_index",
			values:        map[string]interface{}{"key": []interface{}{0}},
			path:          []string{"key", "10"},
			value:         "ignored",
			expected:      map[string]interface{}{"key": []interface{}{0}},
			expectedError: true,
			errorContains: "out of bounds invalid array index: 10 (array length: 1)",
		},
		{
			name:          "invalid_path_type",
			values:        map[string]interface{}{"key": "not a map"},
			path:          []string{"key", "invalid"},
			value:         "new",
			expected:      map[string]interface{}{"key": "not a map"},
			expectedError: true,
			errorContains: "is not a map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Operate directly on the test case map
			err := SetValueAtPath(tt.values, tt.path, tt.value)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				// Verify data wasn't changed on error
				assert.Equal(t, tt.expected, tt.values, "Data should not change on error")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, tt.values)
			}
		})
	}
}
