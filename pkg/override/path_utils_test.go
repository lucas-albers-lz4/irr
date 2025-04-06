package override

import (
	"reflect"
	"testing"
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
				if reflect.ValueOf(result).Pointer() == reflect.ValueOf(m).Pointer() {
					t.Error("DeepCopy() returned same map instance")
				}
			}

			// For arrays, verify they're different instances
			if m, ok := tt.src.(map[string]interface{}); ok {
				for k, v := range m {
					if arr, ok := v.([]interface{}); ok {
						resultMap, ok := result.(map[string]interface{})
						if !ok {
							t.Error("DeepCopy() result is not a map[string]interface{}")
							continue
						}
						resultVal, ok := resultMap[k]
						if !ok {
							t.Errorf("DeepCopy() result missing key %s", k)
							continue
						}
						resultArr, ok := resultVal.([]interface{})
						if !ok {
							t.Errorf("DeepCopy() value at key %s is not []interface{}", k)
							continue
						}
						if len(arr) > 0 && len(resultArr) > 0 {
							if reflect.ValueOf(arr).Pointer() == reflect.ValueOf(resultArr).Pointer() {
								t.Error("DeepCopy() returned same array instance")
							}
						}
					}
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
			err := SetValueAtPath(tt.data, tt.path, tt.value)

			if tt.wantError {
				if err == nil {
					t.Error("SetValueAtPath() expected error, got nil")
				}
				return
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
			wantIndex:    0,
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
			name:         "malformed array index - no opening bracket",
			part:         "containers0]",
			wantKey:      "containers0]",
			wantIndex:    0,
			wantHasIndex: false,
			wantErr:      false,
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
