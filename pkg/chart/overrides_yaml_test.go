package chart

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverridesToYAML(t *testing.T) {
	t.Run("Simple Map", func(t *testing.T) {
		// Test with a simple map
		overrides := map[string]interface{}{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		}

		yaml, err := OverridesToYAML(overrides)
		require.NoError(t, err, "Converting simple map to YAML should not error")
		assert.Contains(t, string(yaml), "key1: value1")
		assert.Contains(t, string(yaml), "key2: 42")
		assert.Contains(t, string(yaml), "key3: true")
	})

	t.Run("Nested Map", func(t *testing.T) {
		// Test with a nested map
		overrides := map[string]interface{}{
			"top": map[string]interface{}{
				"nested": map[string]interface{}{
					"key": "value",
				},
			},
		}

		yaml, err := OverridesToYAML(overrides)
		require.NoError(t, err, "Converting nested map to YAML should not error")
		assert.Contains(t, string(yaml), "top:")
		assert.Contains(t, string(yaml), "nested:")
		assert.Contains(t, string(yaml), "key: value")
	})

	t.Run("Array Values", func(t *testing.T) {
		// Test with array values
		overrides := map[string]interface{}{
			"array": []string{"item1", "item2", "item3"},
		}

		yaml, err := OverridesToYAML(overrides)
		require.NoError(t, err, "Converting map with array values to YAML should not error")
		assert.Contains(t, string(yaml), "array:")
		assert.Contains(t, string(yaml), "- item1")
		assert.Contains(t, string(yaml), "- item2")
		assert.Contains(t, string(yaml), "- item3")
	})

	t.Run("Empty Map", func(t *testing.T) {
		// Test with an empty map
		overrides := map[string]interface{}{}

		yaml, err := OverridesToYAML(overrides)
		require.NoError(t, err, "Converting empty map to YAML should not error")
		assert.Equal(t, "{}\n", string(yaml))
	})

	t.Run("Complex Nested Structure", func(t *testing.T) {
		// Test with a complex nested structure
		overrides := map[string]interface{}{
			"global": map[string]interface{}{
				"security": map[string]interface{}{
					"allowInsecureImages": true,
				},
				"image": map[string]interface{}{
					"registry":   "example.com",
					"repository": "app",
					"tag":        "latest",
				},
			},
			"deployment": map[string]interface{}{
				"replicas": 3,
				"annotations": map[string]interface{}{
					"key1": "value1",
					"key2": "value2",
				},
			},
			"service": map[string]interface{}{
				"type": "ClusterIP",
				"ports": []map[string]interface{}{
					{
						"name": "http",
						"port": 80,
					},
					{
						"name": "https",
						"port": 443,
					},
				},
			},
		}

		yaml, err := OverridesToYAML(overrides)
		require.NoError(t, err, "Converting complex structure to YAML should not error")

		// Verify the top-level keys are present
		assert.Contains(t, string(yaml), "global:")
		assert.Contains(t, string(yaml), "deployment:")
		assert.Contains(t, string(yaml), "service:")

		// Verify some nested values
		assert.Contains(t, string(yaml), "allowInsecureImages: true")
		assert.Contains(t, string(yaml), "registry: example.com")
		assert.Contains(t, string(yaml), "replicas: 3")
		assert.Contains(t, string(yaml), "type: ClusterIP")
	})
}
