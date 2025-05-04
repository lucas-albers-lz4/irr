// Package analyzer_test contains tests for the analyzer package.
package analyzer

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Constants used in tests
const (
	DefaultRegistry = "docker.io"
)

// Helper function to sort ImagePatterns by path for consistent test results
func sortPatternsByPath(patterns []ImagePattern) {
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Path < patterns[j].Path
	})
}

func TestSimplePatternMatching(t *testing.T) {
	values := map[string]interface{}{
		"container1": map[string]interface{}{"image": "nginx:latest"},
		"container2": map[string]interface{}{"image": "quay.io/prometheus/node-exporter:v1.0.0"},
		"container3": map[string]interface{}{"image": "mysql:8.0"},
		"ignored":    "some other value",
	}
	config := &Config{}

	// Call the main analysis function
	patterns, err := AnalyzeHelmValues(values, config)
	require.NoError(t, err)

	// Expected patterns - Structure is nil for string types
	expectedPatterns := []ImagePattern{
		{Path: "container1.image", Type: "string", Value: "nginx:latest", Count: 1, Structure: nil},
		{Path: "container2.image", Type: "string", Value: "quay.io/prometheus/node-exporter:v1.0.0", Count: 1, Structure: nil},
		{Path: "container3.image", Type: "string", Value: "mysql:8.0", Count: 1, Structure: nil},
	}

	// Sort patterns for comparison
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Path < patterns[j].Path
	})
	sort.Slice(expectedPatterns, func(i, j int) bool {
		return expectedPatterns[i].Path < expectedPatterns[j].Path
	})

	assert.Len(t, patterns, len(expectedPatterns), "Number of patterns mismatch")

	for i, expected := range expectedPatterns {
		require.Less(t, i, len(patterns), "Index out of bounds accessing patterns")
		actual := patterns[i]
		assert.Equal(t, expected.Path, actual.Path, "Pattern %d: Path mismatch", i)
		assert.Equal(t, expected.Type, actual.Type, "Pattern %d: Type mismatch", i)
		assert.Equal(t, expected.Value, actual.Value, "Pattern %d: Value mismatch", i)
		assert.Equal(t, expected.Count, actual.Count, "Pattern %d: Count mismatch", i)
		// Expect Structure to be nil for string types
		if expected.Type == "string" {
			assert.Nil(t, actual.Structure, "Pattern %d: Structure should be nil for string type", i)
		} else {
			assert.Equal(t, expected.Structure, actual.Structure, "Pattern %d: Structure mismatch", i)
		}
	}
}

func TestBasicValueTraversal(t *testing.T) {
	values := map[string]interface{}{
		"mainContainer":            map[string]interface{}{"image": "app:latest"},
		"initContainer":            map[string]interface{}{"image": "busybox:1.32"},
		"sidecarContainer":         map[string]interface{}{"image": "proxy:v2"},
		"jobTemplate":              map[string]interface{}{"spec": map[string]interface{}{"image": "batch-processor:stable"}},
		"configMapData":            map[string]interface{}{"imageUrl": "gcr.io/google-containers/pause:3.1"},
		"stringWithRegistryAndTag": "my.registry.com/path/to/image:tag1",
		"stringWithRegistryNoTag":  "another.registry/just/repo",
		"stringNoRegistryWithTag":  "simple-repo:tag2",
		"stringNoRegistryNoTag":    "very-simple",
		"withPort":                 map[string]interface{}{"image": "registry.local:5000/app/service:v3"},
		"imageMap": map[string]interface{}{
			"registry":   "explicit.registry",
			"repository": "map-repo",
			"tag":        "map-tag",
		},
		"nestedMap": map[string]interface{}{
			"innerImage": map[string]interface{}{"image": "nested:abc"},
		},
		"imageInArray": []interface{}{
			"array-image1:tagA",
			"quay.io/array/image2:tagB",
		},
		"mapInArray": []interface{}{
			map[string]interface{}{"image": "map-array-img:tagC"},
			map[string]interface{}{"anotherKey": "ignored"},
		},
		"mapWithImageKey":      map[string]interface{}{"image": "key-implies-image:tagD"},
		"mapWithRepositoryKey": map[string]interface{}{"repository": "key-implies-repo:tagE"},
		"notAnImage":           "plain string",
		"templateString":       "{{ .Values.someValue }}",
		"emptyString":          "",
		"nonString":            123,
		"nilValue":             nil,
	}

	config := &Config{}

	patterns, err := AnalyzeHelmValues(values, config)
	require.NoError(t, err)

	// Updated expected patterns - Adjusted based on analyzer logic
	expectedPatternsMap := map[string]ImagePattern{
		"mainContainer.image":    {Path: "mainContainer.image", Type: "string", Value: "app:latest", Structure: nil},
		"initContainer.image":    {Path: "initContainer.image", Type: "string", Value: "busybox:1.32", Structure: nil},
		"sidecarContainer.image": {Path: "sidecarContainer.image", Type: "string", Value: "proxy:v2", Structure: nil},
		"jobTemplate.spec.image": {Path: "jobTemplate.spec.image", Type: "string", Value: "batch-processor:stable", Structure: nil},
		"withPort.image":         {Path: "withPort.image", Type: "string", Value: "registry.local:5000/app/service:v3", Structure: nil},
		"imageMap": {
			Path:      "imageMap",
			Type:      "map",
			Value:     "repository=map-repo,registry=explicit.registry,tag=map-tag",
			Structure: &ImageStructure{Registry: "explicit.registry", Repository: "map-repo", Tag: "map-tag"},
		},
		"nestedMap.innerImage.image": {Path: "nestedMap.innerImage.image", Type: "string", Value: "nested:abc", Structure: nil},
		"mapInArray[0].image":        {Path: "mapInArray[0].image", Type: "string", Value: "map-array-img:tagC", Structure: nil},
		"mapWithImageKey.image":      {Path: "mapWithImageKey.image", Type: "string", Value: "key-implies-image:tagD", Structure: nil},
		"mapWithRepositoryKey": {
			Path:      "mapWithRepositoryKey",
			Type:      "map",
			Value:     "repository=library/key-implies-repo,registry=docker.io,tag=tagE",
			Structure: &ImageStructure{Registry: "docker.io", Repository: "library/key-implies-repo", Tag: "tagE"},
		},
	}

	// Count actual patterns found
	actualPatternsMap := make(map[string]ImagePattern)
	for _, p := range patterns {
		actualPatternsMap[p.Path] = p
	}

	assert.Equal(t, len(expectedPatternsMap), len(actualPatternsMap), "Incorrect number of image patterns found. Expected: %v, Got: %v", expectedPatternsMap, actualPatternsMap)

	// Verify specific patterns
	for path, expectedPattern := range expectedPatternsMap {
		actualPattern, found := actualPatternsMap[path]
		require.True(t, found, "Expected pattern with path '%s' not found", path)

		assert.Equal(t, expectedPattern.Type, actualPattern.Type, "%s: Type mismatch", path)
		assert.Equal(t, expectedPattern.Value, actualPattern.Value, "%s: Value mismatch", path)
		assert.Equal(t, expectedPattern.Structure, actualPattern.Structure, "%s: Structure mismatch", path)
	}

	// Verify specific cases are still correctly identified (or not)
	mainPattern, foundMain := findPatternByPath(patterns, "mainContainer.image")
	require.True(t, foundMain)
	assert.Nil(t, mainPattern.Structure)

	sidecarPattern, foundSidecar := findPatternByPath(patterns, "sidecarContainer.image")
	require.True(t, foundSidecar)
	assert.Nil(t, sidecarPattern.Structure)

	imageMapPattern, foundMap := findPatternByPath(patterns, "imageMap")
	require.True(t, foundMap)
	require.NotNil(t, imageMapPattern.Structure)

	_, foundStringWithTag := findPatternByPath(patterns, "stringWithRegistryAndTag")
	assert.False(t, foundStringWithTag, "Pattern 'stringWithRegistryAndTag' should NOT be found by legacy analyzer")

	_, foundArrayImage := findPatternByPath(patterns, "imageInArray[0]")
	assert.False(t, foundArrayImage, "Pattern 'imageInArray[0]' should NOT be found by legacy analyzer")

	_, foundConfigMapURL := findPatternByPath(patterns, "configMapData.imageUrl")
	assert.False(t, foundConfigMapURL, "Pattern 'configMapData.imageUrl' should NOT be found by legacy analyzer")

	mapRepoPattern, foundMapRepo := findPatternByPath(patterns, "mapWithRepositoryKey")
	require.True(t, foundMapRepo, "Pattern 'mapWithRepositoryKey' (map) should be found")
	assert.Equal(t, "map", mapRepoPattern.Type)
	require.NotNil(t, mapRepoPattern.Structure)
}

func TestRecursiveAnalysisWithNestedStructures(t *testing.T) {
	values := map[string]interface{}{
		"level1": map[string]interface{}{
			"image": "image1:v1",
			"level2": map[string]interface{}{
				"image": "image2:v2",
				"level3": map[string]interface{}{
					"image": "image3:v3",
				},
			},
			"siblings": map[string]interface{}{
				"sibling1": "sibling-repo1",
				"sibling2": "sibling-repo2:stable",
			},
		},
		"unrelated": map[string]interface{}{"image": "unrelated:abc"},
		"mixed": map[string]interface{}{
			"plainValue": "not-an-image",
			"nested":     "mixed-repo:v1",
		},
		"mapImage": map[string]interface{}{
			"registry":   "reg.com",
			"repository": "repo1",
			"tag":        "t1",
		},
		"nestedMapImage": map[string]interface{}{
			"inner": map[string]interface{}{
				"registry":   "nested.reg",
				"repository": "nested-repo",
				"tag":        "t2",
			},
		},
	}

	config := &Config{}

	// Call the main analysis function
	patterns, err := AnalyzeHelmValues(values, config)
	require.NoError(t, err)

	// Updated expected patterns based on key heuristics and map detection
	expectedPatternsMap := map[string]ImagePattern{
		"level1.image":               {Path: "level1.image", Type: "string", Value: "image1:v1", Structure: nil},
		"level1.level2.image":        {Path: "level1.level2.image", Type: "string", Value: "image2:v2", Structure: nil},
		"level1.level2.level3.image": {Path: "level1.level2.level3.image", Type: "string", Value: "image3:v3", Structure: nil},
		"unrelated.image":            {Path: "unrelated.image", Type: "string", Value: "unrelated:abc", Structure: nil},
		"mapImage":                   {Path: "mapImage", Type: "map", Value: "repository=repo1,registry=reg.com,tag=t1", Structure: &ImageStructure{Registry: "reg.com", Repository: "repo1", Tag: "t1"}},
		"nestedMapImage.inner": {
			Path:      "nestedMapImage.inner",
			Type:      "map",
			Value:     "repository=nested-repo,registry=nested.reg,tag=t2",
			Structure: &ImageStructure{Registry: "nested.reg", Repository: "nested-repo", Tag: "t2"},
		},
	}

	// Count actual patterns found
	actualPatternsMap := make(map[string]ImagePattern)
	for _, p := range patterns {
		actualPatternsMap[p.Path] = p
	}

	assert.Equal(t, len(expectedPatternsMap), len(actualPatternsMap), "Incorrect number of image patterns found. Expected: %v, Got: %v", expectedPatternsMap, actualPatternsMap)

	// Verify specific patterns
	for path, expectedPattern := range expectedPatternsMap {
		actualPattern, found := actualPatternsMap[path]
		require.True(t, found, "Expected pattern with path '%s' not found", path)

		assert.Equal(t, expectedPattern.Type, actualPattern.Type, "%s: Type mismatch", path)
		assert.Equal(t, expectedPattern.Value, actualPattern.Value, "%s: Value mismatch", path)
		assert.Equal(t, expectedPattern.Structure, actualPattern.Structure, "%s: Structure mismatch", path)
	}

	// Verify specific missed patterns
	_, foundSibling1 := findPatternByPath(patterns, "level1.siblings.sibling1")
	assert.False(t, foundSibling1, "Pattern 'level1.siblings.sibling1' should NOT be found")
	_, foundMixedNested := findPatternByPath(patterns, "mixed.nested")
	assert.False(t, foundMixedNested, "Pattern 'mixed.nested' should NOT be found")
}

func TestAggregatePatterns(t *testing.T) {
	// Test dataset with duplicate patterns
	patterns := []ImagePattern{
		{Path: "image", Type: "string", Value: "nginx:latest", Count: 1},
		{Path: "image", Type: "string", Value: "nginx:latest", Count: 1}, // Duplicate
		{Path: "other.image", Type: "string", Value: "redis:alpine", Count: 1},
		{Path: "service.image", Type: "map", Value: "mysql:8.0", Count: 1},
		{Path: "service.image", Type: "map", Value: "mysql:8.0", Count: 1}, // Duplicate
		{Path: "service.image", Type: "map", Value: "mysql:8.0", Count: 1}, // Another duplicate
	}

	// Expected result after aggregation
	expected := []ImagePattern{
		{Path: "image", Type: "string", Value: "nginx:latest", Count: 2},
		{Path: "other.image", Type: "string", Value: "redis:alpine", Count: 1},
		{Path: "service.image", Type: "map", Value: "mysql:8.0", Count: 3},
	}

	// Run the aggregation
	aggregated := aggregatePatterns(patterns)

	// Sort both for consistent comparison
	sortPatternsByPath(aggregated)
	sortPatternsByPath(expected)

	// Check the result
	if len(aggregated) != len(expected) {
		t.Errorf("Expected %d patterns after aggregation, got %d", len(expected), len(aggregated))
	}

	// Compare each pattern
	for i, exp := range expected {
		if i >= len(aggregated) {
			t.Errorf("Missing expected pattern at index %d: %+v", i, exp)
			continue
		}
		got := aggregated[i]
		if got.Path != exp.Path {
			t.Errorf("Pattern %d: expected path %q, got %q", i, exp.Path, got.Path)
		}
		if got.Count != exp.Count {
			t.Errorf("Pattern %d: expected count %d, got %d", i, exp.Count, got.Count)
		}
		if got.Value != exp.Value {
			t.Errorf("Pattern %d: expected value %q, got %q", i, exp.Value, got.Value)
		}
	}
}

func TestConfigWithIncludeExcludePatterns(t *testing.T) {
	// Create a test structure with various paths
	values := map[string]interface{}{
		"include": map[string]interface{}{
			"this": map[string]interface{}{
				"image": "include:this",
			},
		},
		"exclude": map[string]interface{}{
			"this": map[string]interface{}{
				"image": "exclude:this",
			},
		},
		"mixed": map[string]interface{}{
			"include": map[string]interface{}{
				"image": "mixed:include",
			},
			"exclude": map[string]interface{}{
				"image": "mixed:exclude",
			},
		},
	}

	// Config with include/exclude patterns
	config := &Config{
		IncludePatterns: []string{"include.*", "mixed.include.*"},
		ExcludePatterns: []string{"exclude.*", "mixed.exclude.*"},
	}

	// Run the analyzer
	patterns, err := AnalyzeHelmValues(values, config)
	if err != nil {
		t.Fatalf("AnalyzeHelmValues failed: %v", err)
	}

	// Expected patterns with their paths
	expectedPaths := map[string]bool{
		"include.this.image":  true,
		"mixed.include.image": true,
	}

	unexpectedPaths := map[string]bool{
		"exclude.this.image":  true,
		"mixed.exclude.image": true,
	}

	// Check that expected paths are found
	for _, pattern := range patterns {
		if unexpectedPaths[pattern.Path] {
			t.Errorf("Found pattern that should have been excluded: %s", pattern.Path)
		}

		if expectedPaths[pattern.Path] {
			// Mark as found
			expectedPaths[pattern.Path] = false
		} else {
			t.Errorf("Unexpected pattern found: %s", pattern.Path)
		}
	}

	// Check for missing expected patterns
	for path, notFound := range expectedPaths {
		if notFound {
			t.Errorf("Expected pattern not found: %s", path)
		}
	}
}

// TestAnalyzeInterfaceValue tests the analyzeInterfaceValue function
func TestAnalyzeInterfaceValue(t *testing.T) {
	config := &Config{}

	testCases := []struct {
		name             string
		values           map[string]interface{}
		expectedPatterns int
		checkPattern     func(t *testing.T, p ImagePattern)
	}{
		{
			name:             "Interface value as string",
			values:           map[string]interface{}{"image": interface{}("ubuntu:latest")},
			expectedPatterns: 1,
			checkPattern: func(t *testing.T, p ImagePattern) {
				assert.Equal(t, "image", p.Path)
				assert.Equal(t, "string", p.Type)
				assert.Equal(t, "ubuntu:latest", p.Value)
				assert.Nil(t, p.Structure)
			},
		},
		{
			name: "Interface value as map",
			values: map[string]interface{}{"image": interface{}(map[string]interface{}{
				"repository": "nginx",
				"tag":        "latest",
			})},
			expectedPatterns: 1,
			checkPattern: func(t *testing.T, p ImagePattern) {
				assert.Equal(t, "image", p.Path)
				assert.Equal(t, "map", p.Type)
				require.NotNil(t, p.Structure)
				assert.Equal(t, "docker.io", p.Structure.Registry)
				assert.Equal(t, "library/nginx", p.Structure.Repository)
				assert.Equal(t, "latest", p.Structure.Tag)
			},
		},
		{
			name:             "Interface value as int",
			values:           map[string]interface{}{"image": interface{}(123)},
			expectedPatterns: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the main analysis function
			patterns, err := AnalyzeHelmValues(tc.values, config)
			require.NoError(t, err)
			assert.Len(t, patterns, tc.expectedPatterns)
			if tc.expectedPatterns > 0 && tc.checkPattern != nil {
				require.NotEmpty(t, patterns)
				tc.checkPattern(t, patterns[0])
			}
		})
	}
}

// TestAnalyzeInterfaceValueDirect tests the analyzeInterfaceValue function directly
func TestAnalyzeInterfaceValueDirect(t *testing.T) {
	config := &Config{}
	testCases := []struct {
		name             string
		value            interface{}
		expectedPatterns int
		checkPattern     func(t *testing.T, p ImagePattern)
		expectedErr      bool
	}{
		{
			name:             "Interface containing map",
			value:            map[string]interface{}{"image": map[string]interface{}{"repository": "nginx", "tag": "latest"}},
			expectedPatterns: 1,
			checkPattern: func(t *testing.T, p ImagePattern) {
				assert.Equal(t, "image", p.Path)
				assert.Equal(t, "map", p.Type)
				require.NotNil(t, p.Structure)
				assert.Equal(t, "docker.io", p.Structure.Registry)
				assert.Equal(t, "library/nginx", p.Structure.Repository)
				assert.Equal(t, "latest", p.Structure.Tag)
			},
		},
		{
			name:             "Interface containing string",
			value:            map[string]interface{}{"image": "redis:alpine"},
			expectedPatterns: 1,
			checkPattern: func(t *testing.T, p ImagePattern) {
				assert.Equal(t, "image", p.Path)
				assert.Equal(t, "string", p.Type)
				assert.Equal(t, "redis:alpine", p.Value)
				assert.Nil(t, p.Structure)
			},
		},
		{
			name:             "Interface containing int",
			value:            map[string]interface{}{"image": 123},
			expectedPatterns: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valuesMap, ok := tc.value.(map[string]interface{})
			require.True(t, ok, "Test case value must be a map[string]interface{}")
			// Call the main analysis function
			patterns, err := AnalyzeHelmValues(valuesMap, config)

			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, patterns, tc.expectedPatterns)
				if tc.expectedPatterns > 0 && tc.checkPattern != nil {
					require.NotEmpty(t, patterns)
					tc.checkPattern(t, patterns[0])
				}
			}
		})
	}
}

// Helper function to find a pattern by path
func findPatternByPath(patterns []ImagePattern, path string) (ImagePattern, bool) {
	for _, p := range patterns {
		if p.Path == path {
			return p, true
		}
	}
	return ImagePattern{}, false
}
