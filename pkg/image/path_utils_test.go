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
	}{
		{
			name: "set simple value",
			values: map[string]interface{}{
				"key": "old",
			},
			path:  []string{"key"},
			value: "new",
			expected: map[string]interface{}{
				"key": "new",
			},
		},
		{
			name: "set nested value",
			values: map[string]interface{}{
				"parent": map[string]interface{}{
					"child": "old",
				},
			},
			path:  []string{"parent", "child"},
			value: "new",
			expected: map[string]interface{}{
				"parent": map[string]interface{}{
					"child": "new",
				},
			},
		},
		{
			name: "create nested path",
			values: map[string]interface{}{
				"existing": "value",
			},
			path:  []string{"new", "nested", "path"},
			value: "value",
			expected: map[string]interface{}{
				"existing": "value",
				"new": map[string]interface{}{
					"nested": map[string]interface{}{
						"path": "value",
					},
				},
			},
		},
		{
			name: "set array element",
			values: map[string]interface{}{
				"items": []interface{}{
					"first",
					"second",
				},
			},
			path:  []string{"items", "1"},
			value: "updated",
			expected: map[string]interface{}{
				"items": []interface{}{
					"first",
					"updated",
				},
			},
		},
		{
			name: "invalid array index",
			values: map[string]interface{}{
				"items": []interface{}{
					"first",
				},
			},
			path:          []string{"items", "1"},
			value:         "new",
			expectedError: true,
		},
		{
			name: "invalid path type",
			values: map[string]interface{}{
				"key": "value",
			},
			path:          []string{"key", "invalid"},
			value:         "new",
			expectedError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := SetValueAtPath(tc.values, tc.path, tc.value)
			if tc.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, tc.values)
		})
	}
}
