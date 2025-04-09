package chart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"

	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
)

const (
	testChartPath         = "./test-chart"
	defaultTargetRegistry = "harbor.local"
)

// MockPathStrategy for testing
type MockPathStrategy struct {
	GeneratePathFunc func(originalRef *image.Reference, targetRegistry string) (string, error)
}

func (m *MockPathStrategy) GeneratePath(originalRef *image.Reference, targetRegistry string) (string, error) {
	if m.GeneratePathFunc != nil {
		return m.GeneratePathFunc(originalRef, targetRegistry)
	}
	return "mockpath/" + originalRef.Repository, nil
}

func TestNewGenerator(t *testing.T) {
	chartPath := "./test-chart"
	targetRegistry := "my.registry.com"
	sourceRegistries := []string{"docker.io"}
	excludeRegistries := []string{"internal.com"}
	var mockMappings *registry.Mappings // Updated type here
	mockStrategy := &MockPathStrategy{}
	strict := false
	threshold := 90
	// Use a mock loader for NewGenerator test (can be nil to test default)
	mockLoader := Loader(nil) // Test default loader case first

	generator := NewGenerator(
		chartPath,
		targetRegistry,
		sourceRegistries,
		excludeRegistries,
		mockStrategy,
		mockMappings,
		strict,
		threshold,
		mockLoader, // Pass the loader
	)

	require.NotNil(t, generator)
	assert.Equal(t, chartPath, generator.chartPath, "chartPath mismatch")
	assert.Equal(t, targetRegistry, generator.targetRegistry, "targetRegistry mismatch")
	assert.Equal(t, sourceRegistries, generator.sourceRegistries, "sourceRegistries mismatch")
	assert.Equal(t, excludeRegistries, generator.excludeRegistries, "excludeRegistries mismatch")
	assert.Equal(t, mockStrategy, generator.pathStrategy, "pathStrategy mismatch")
	assert.Equal(t, mockMappings, generator.mappings, "mappings mismatch")
	assert.Equal(t, strict, generator.strict, "strict mismatch")
	assert.Equal(t, threshold, generator.threshold, "threshold mismatch")
	assert.NotNil(t, generator.loader, "Loader should be initialized by default")
	assert.IsType(t, &helmLoader{}, generator.loader, "Default loader should be helmLoader type")

	// TODO: Add test case passing a non-nil mock loader
}

// --- Mocks for Generate Tests ---

// MockChartLoader (specific to generator tests if needed, or reuse from analysis)
type MockChartLoader struct {
	LoadFunc func(_ string) (*chart.Chart, error)
}

func (m *MockChartLoader) Load(_ string) (*chart.Chart, error) {
	if m.LoadFunc != nil {
		return m.LoadFunc("")
	}
	return nil, errors.New("MockChartLoader.LoadFunc was not set")
}

// --- Test Generate --- //

func TestGenerate(t *testing.T) {
	// Common setup for tests
	targetRegistry := "harbor.local"
	sourceRegistries := []string{"docker.io"}
	var excludeRegistries []string
	mockStrategy := &MockPathStrategy{} // Use simple default mock strategy
	strict := false
	threshold := 100
	dummyChartPath := "./test-chart"

	t.Run("Simple Image Map Override", func(t *testing.T) {
		// 1. Mock Chart Loader
		mockChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "testchart"},
			Values: map[string]interface{}{
				"appImage": map[string]interface{}{
					"registry":   "docker.io", // Source registry
					"repository": "myorg/myapp",
					"tag":        "1.0.0",
				},
				"otherValue": 123,
			},
		}
		mockLoader := &MockChartLoader{LoadFunc: func(_ string) (*chart.Chart, error) {
			return mockChart, nil
		}}

		// 2. Create Generator with mocks
		generator := NewGenerator(
			dummyChartPath,
			targetRegistry,
			sourceRegistries,
			excludeRegistries,
			mockStrategy,
			nil,
			strict,
			threshold,
			mockLoader,
		)

		// 3. Call Generate
		overrideFile, err := generator.Generate()

		// +++ Add Debug Logging Here +++
		t.Logf("[TEST DEBUG] Generate returned err: %v", err)
		if overrideFile != nil {
			t.Logf("[TEST DEBUG] overrideFile.ChartName = %q", overrideFile.ChartName)
			overridesBytes, _ := yaml.Marshal(overrideFile.Overrides)
			t.Logf("[TEST DEBUG] overrideFile.Overrides =\n%s", string(overridesBytes))
		} else {
			t.Log("[TEST DEBUG] overrideFile is nil")
		}
		// +++ End Debug Logging +++

		// 4. Assert Results
		require.NoError(t, err, "Generate() should not return an error")
		require.NotNil(t, overrideFile, "Generated override file should not be nil")

		// Check chart metadata
		assert.Equal(t, dummyChartPath, overrideFile.ChartPath, "Chart path mismatch")
		assert.Equal(t, "testchart", overrideFile.ChartName, "Chart name mismatch")
		assert.Empty(t, overrideFile.Unsupported, "Should be no unsupported structures")

		// Check the generated override structure
		require.Contains(t, overrideFile.Overrides, "appImage", "Overrides should contain appImage key")
		appImage, ok := overrideFile.Overrides["appImage"].(map[string]interface{})
		require.True(t, ok, "appImage should be a map[string]interface{}")

		// Check image map fields
		assert.Equal(t, targetRegistry, appImage["registry"], "Registry mismatch")
		assert.Equal(t, "mockpath/myorg/myapp", appImage["repository"], "Repository mismatch")
		assert.Equal(t, "1.0.0", appImage["tag"], "Tag mismatch")

		// Verify non-image values are not included
		assert.NotContains(t, overrideFile.Overrides, "otherValue", "Overrides should not contain unmodified values")
	})

	t.Run("Simple Image String Override", func(t *testing.T) {
		// 1. Mock Chart Loader
		imageStringValue := "docker.io/myorg/stringapp:v2" // Belongs to source registry
		mockChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "stringchart"},
			Values: map[string]interface{}{
				"workerImage": imageStringValue,
			},
		}
		mockLoader := &MockChartLoader{LoadFunc: func(_ string) (*chart.Chart, error) {
			return mockChart, nil
		}}

		// 2. Create Generator
		generator := NewGenerator(
			dummyChartPath,
			targetRegistry,
			sourceRegistries,
			excludeRegistries,
			mockStrategy,
			nil,
			strict,
			threshold,
			mockLoader,
		)

		// 3. Call Generate
		overrideFile, err := generator.Generate()

		// +++ Add Debug Logging Here +++
		t.Logf("[TEST DEBUG] Generate returned err: %v", err)
		if overrideFile != nil {
			t.Logf("[TEST DEBUG] overrideFile.ChartName = %q", overrideFile.ChartName)
			overridesBytes, _ := yaml.Marshal(overrideFile.Overrides)
			t.Logf("[TEST DEBUG] overrideFile.Overrides =\n%s", string(overridesBytes))
		} else {
			t.Log("[TEST DEBUG] overrideFile is nil")
		}
		// +++ End Debug Logging +++

		// 4. Assert Results
		require.NoError(t, err, "Generate() should not return an error")
		require.NotNil(t, overrideFile, "Generated override file should not be nil")
		assert.Empty(t, overrideFile.Unsupported, "Should be no unsupported structures")

		// Check the generated override structure - Should be a STRING
		require.Contains(t, overrideFile.Overrides, "workerImage", "Overrides should contain workerImage key")
		workerImageValue, ok := overrideFile.Overrides["workerImage"].(string)
		require.True(t, ok, "workerImage override should be a string")

		// Check the string value (Construct expected value based on strategy)
		// Mock strategy prepends "mockpath/" to the REPOSITORY part only.
		// Original string was "docker.io/myorg/stringapp:v2"
		// Repository is "myorg/stringapp"
		expectedStringValue := targetRegistry + "/mockpath/myorg/stringapp:v2" // Corrected expectation
		assert.Equal(t, expectedStringValue, workerImageValue, "Override string value mismatch")
	})

	t.Run("Excluded Registry", func(t *testing.T) {
		excludedReg := "private.registry.io"
		localExcludeRegistries := []string{excludedReg} // Specific for this test

		mockChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "excludedchart"},
			Values: map[string]interface{}{
				"internalImage": map[string]interface{}{ // Image from excluded registry
					"registry":   excludedReg,
					"repository": "internal/tool",
					"tag":        "prod",
				},
				"publicImage": "docker.io/library/alpine:latest", // Should still be processed
			},
		}
		mockLoader := &MockChartLoader{LoadFunc: func(_ string) (*chart.Chart, error) {
			return mockChart, nil
		}}

		generator := NewGenerator(
			dummyChartPath,
			targetRegistry,
			sourceRegistries,
			localExcludeRegistries, // Use local exclude list
			mockStrategy,
			nil,
			strict,
			threshold,
			mockLoader,
		)

		// Call Generate
		overrideFile, err := generator.Generate()

		// +++ Add Debug Logging Here +++
		t.Logf("[TEST DEBUG] Generate returned err: %v", err)
		if overrideFile != nil {
			t.Logf("[TEST DEBUG] overrideFile.ChartName = %q", overrideFile.ChartName)
			overridesBytes, _ := yaml.Marshal(overrideFile.Overrides)
			t.Logf("[TEST DEBUG] overrideFile.Overrides =\n%s", string(overridesBytes))
		} else {
			t.Log("[TEST DEBUG] overrideFile is nil")
		}
		// +++ End Debug Logging +++

		// Assert Results
		require.NoError(t, err, "Generate() should not return an error")
		require.NotNil(t, overrideFile, "Generated override file should not be nil")
		assert.Empty(t, overrideFile.Unsupported, "Should be no unsupported structures")

		// Check overrides: ONLY publicImage should be present
		assert.NotContains(t, overrideFile.Overrides, "internalImage", "Overrides should NOT contain excluded image")
		require.Contains(t, overrideFile.Overrides, "publicImage", "Overrides should contain non-excluded image")

		// Check publicImage structure - Should be a STRING
		publicImageValue, ok := overrideFile.Overrides["publicImage"].(string)
		require.True(t, ok, "publicImage override should be a string")

		// Check the string value
		// Original: "docker.io/library/alpine:latest"
		// Mock strategy prepends "mockpath/" to repository "library/alpine"
		expectedStringValue := targetRegistry + "/mockpath/library/alpine:latest" // Corrected expectation
		assert.Equal(t, expectedStringValue, publicImageValue, "Override string value mismatch")
	})

	t.Run("Non-Source Registry", func(t *testing.T) {
		nonSourceReg := "quay.io" // Not in sourceRegistries = ["docker.io"]

		mockChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "nonsourcechart"},
			Values: map[string]interface{}{
				"quayImage": map[string]interface{}{ // Image from non-source registry
					"registry":   nonSourceReg,
					"repository": "coreos/etcd",
					"tag":        "v3.5",
				},
				"dockerImage": "docker.io/library/redis:alpine", // Should be processed
			},
		}
		mockLoader := &MockChartLoader{LoadFunc: func(_ string) (*chart.Chart, error) {
			return mockChart, nil
		}}

		generator := NewGenerator(
			dummyChartPath, targetRegistry, sourceRegistries, excludeRegistries,
			mockStrategy, nil, strict, threshold, mockLoader,
		)

		overrideFile, err := generator.Generate()

		// +++ Add Debug Logging Here +++
		t.Logf("[TEST DEBUG] Generate returned err: %v", err)
		if overrideFile != nil {
			t.Logf("[TEST DEBUG] overrideFile.ChartName = %q", overrideFile.ChartName)
			overridesBytes, _ := yaml.Marshal(overrideFile.Overrides)
			t.Logf("[TEST DEBUG] overrideFile.Overrides =\n%s", string(overridesBytes))
		} else {
			t.Log("[TEST DEBUG] overrideFile is nil")
		}
		// +++ End Debug Logging +++

		require.NoError(t, err)
		require.NotNil(t, overrideFile)
		assert.Empty(t, overrideFile.Unsupported)

		// Check overrides: ONLY dockerImage should be present
		assert.NotContains(t, overrideFile.Overrides, "quayImage", "Overrides should NOT contain non-source image")
		assert.Contains(t, overrideFile.Overrides, "dockerImage", "Overrides should contain source image")

		// Check dockerImage structure - Should be a STRING
		dockerImageValue, ok := overrideFile.Overrides["dockerImage"].(string)
		require.True(t, ok, "dockerImage override should be a string")

		// Check the string value
		// Original: "docker.io/library/redis:alpine"
		// Mock strategy prepends "mockpath/" to repository "library/redis"
		expectedStringValue := targetRegistry + "/mockpath/library/redis:alpine" // Corrected expectation
		assert.Equal(t, expectedStringValue, dockerImageValue, "Override string value mismatch")
	})

	t.Run("Prefix Source Registry Strategy", func(t *testing.T) {
		// Use the actual strategy
		actualStrategy := strategy.NewPrefixSourceRegistryStrategy()

		mockChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "strategychart"},
			Values: map[string]interface{}{
				"imgDocker": "docker.io/library/nginx:stable",
				"imgQuay":   map[string]interface{}{"registry": "quay.io", "repository": "prometheus/alertmanager", "tag": "v0.25"},
				"imgGcr":    "gcr.io/google-containers/pause:3.2",
			},
		}
		mockLoader := &MockChartLoader{LoadFunc: func(_ string) (*chart.Chart, error) {
			return mockChart, nil
		}}

		// Need to include quay.io and gcr.io as sources for this test
		localSourceRegistries := []string{"docker.io", "quay.io", "gcr.io"}

		generator := NewGenerator(
			dummyChartPath,
			targetRegistry,
			localSourceRegistries,
			excludeRegistries,
			actualStrategy, // Use the actual strategy instance
			nil,
			strict,
			threshold,
			mockLoader,
		)

		// Call Generate
		overrideFile, err := generator.Generate()

		// +++ Add Debug Logging Here +++
		t.Logf("[TEST DEBUG] Generate returned err: %v", err)
		if overrideFile != nil {
			t.Logf("[TEST DEBUG] overrideFile.ChartName = %q", overrideFile.ChartName)
			overridesBytes, _ := yaml.Marshal(overrideFile.Overrides)
			t.Logf("[TEST DEBUG] overrideFile.Overrides =\n%s", string(overridesBytes))
		} else {
			t.Log("[TEST DEBUG] overrideFile is nil")
		}
		// +++ End Debug Logging +++

		// Assert Results
		require.NoError(t, err, "Generate() should not return an error")
		require.NotNil(t, overrideFile, "Generated override file should not be nil")
		assert.Empty(t, overrideFile.Unsupported, "Should be no unsupported structures")
		require.Len(t, overrideFile.Overrides, 3, "Should have overrides for all 3 images")

		// Check imgDocker (string)
		imgDockerVal, okDocker := overrideFile.Overrides["imgDocker"].(string)
		require.True(t, okDocker, "imgDocker override should be a string")
		assert.Equal(t, "harbor.local/dockerio/library/nginx:stable", imgDockerVal, "imgDocker override value mismatch") // strategy includes source

		// Check imgGcr (string)
		imgGcrVal, okGcr := overrideFile.Overrides["imgGcr"].(string)
		require.True(t, okGcr, "imgGcr override should be a string")
		assert.Equal(t, "harbor.local/gcrio/google-containers/pause:3.2", imgGcrVal, "imgGcr override value mismatch") // strategy includes source

		// Check imgQuay (map - remains map)
		imgQuayVal, okQuay := overrideFile.Overrides["imgQuay"].(map[string]interface{})
		require.True(t, okQuay, "imgQuay override should be a map")
		assert.Equal(t, targetRegistry, imgQuayVal["registry"], "imgQuay registry mismatch")                       // Target registry
		assert.Equal(t, "quayio/prometheus/alertmanager", imgQuayVal["repository"], "imgQuay repository mismatch") // Strategy prepends sanitized source
		assert.Equal(t, "v0.25", imgQuayVal["tag"], "imgQuay tag mismatch")
	})

	t.Run("Chart with Dependencies", func(t *testing.T) {
		// 1. Define Parent and Child Chart structures
		parentChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "parentchart"},
			Values: map[string]interface{}{
				"parentImage": "docker.io/parent/app:v1",
				"child": map[string]interface{}{ // Values specifically for the child alias
					"image": map[string]interface{}{ // Child expects a map
						"repository": "my-child-repo", // Gets docker.io by default
						"tag":        "child-tag",
					},
				},
			},
		}
		childChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "childchart"},
			Values: map[string]interface{}{ // Default values for the child
				"image": map[string]interface{}{
					"repository": "original/child",
					"tag":        "default-tag",
				},
			},
		}
		parentChart.AddDependency(childChart) // Link child to parent

		mockLoader := &MockChartLoader{LoadFunc: func(_ string) (*chart.Chart, error) {
			return parentChart, nil
		}}

		// Use Prefix strategy for clearer path checking
		prefixStrategy := strategy.NewPrefixSourceRegistryStrategy()

		generator := NewGenerator(
			dummyChartPath,
			targetRegistry,
			sourceRegistries,
			excludeRegistries,
			prefixStrategy,
			nil,
			strict,
			threshold,
			mockLoader,
		)

		// Call Generate
		overrideFile, err := generator.Generate()

		// +++ Add Debug Logging Here +++
		t.Logf("[TEST DEBUG] Generate returned err: %v", err)
		if overrideFile != nil {
			t.Logf("[TEST DEBUG] overrideFile.ChartName = %q", overrideFile.ChartName)
			overridesBytes, _ := yaml.Marshal(overrideFile.Overrides)
			t.Logf("[TEST DEBUG] overrideFile.Overrides =\n%s", string(overridesBytes))
		} else {
			t.Log("[TEST DEBUG] overrideFile is nil")
		}
		// +++ End Debug Logging +++

		// Assert Results
		require.NoError(t, err, "Generate() should not return an error")
		require.NotNil(t, overrideFile, "Generated override file should not be nil")
		assert.Empty(t, overrideFile.Unsupported, "Should be no unsupported structures")

		// Check parent image (string)
		parentImageVal, okParent := overrideFile.Overrides["parentImage"].(string)
		require.True(t, okParent, "parentImage override should be a string")
		// Actual strategy: harbor.local/dockerio/parent/app:v1 (docker.io default)
		assert.Equal(t, "harbor.local/dockerio/parent/app:v1", parentImageVal, "parentImage override value mismatch") // Corrected expected value

		// Check child image (map), path should be prefixed with chart name ('child')
		childOverrides, okChild := overrideFile.Overrides["child"].(map[string]interface{})
		require.True(t, okChild, "child overrides should be a map[string]interface{}")

		require.Contains(t, childOverrides, "image", "Child overrides should contain image key")
		childImage, ok := childOverrides["image"].(map[string]interface{})
		require.True(t, ok, "child image should be a map[string]interface{}")

		// Check child image fields
		assert.Equal(t, targetRegistry, childImage["registry"], "Registry mismatch for child image")
		assert.Equal(t, "dockerio/library/my-child-repo", childImage["repository"], "Repository mismatch for child image")
		assert.Equal(t, "child-tag", childImage["tag"], "Tag mismatch for child image")
	})

	t.Run("WithRegistryMapping", func(t *testing.T) {
		// 1. Setup Mocks
		mockChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "mappingchart"},
			Values: map[string]interface{}{
				"imageFromDocker": "docker.io/library/nginx:stable",        // Should map to 'mapped-docker' target
				"imageFromQuay":   "quay.io/prometheus/alertmanager:v0.25", // Should map to 'mapped-quay' target
				"imageUnmapped":   "gcr.io/google-containers/pause:3.2",    // Should use default target 'harbor.local'
			},
		}
		mockLoader := &MockChartLoader{LoadFunc: func(_ string) (*chart.Chart, error) {
			return mockChart, nil
		}}
		mockMappings := &registry.Mappings{
			Entries: []registry.Mapping{
				{Source: "docker.io", Target: "mapped-docker.local"},
				{Source: "quay.io", Target: "mapped-quay.local"},
			},
		}

		// Use Prefix strategy which respects mappings
		prefixStrategy := strategy.NewPrefixSourceRegistryStrategy()
		sourceRegistries := []string{"docker.io", "quay.io", "gcr.io"} // Include all sources

		// 2. Create Generator
		generator := NewGenerator(
			dummyChartPath,
			targetRegistry, // Default target, should be overridden by mappings
			sourceRegistries,
			[]string{}, // No excludes
			prefixStrategy,
			mockMappings, // Provide the mappings
			strict,
			threshold,
			mockLoader,
		)

		// 3. Call Generate
		overrideFile, err := generator.Generate()

		// +++ Add Debug Logging Here +++
		t.Logf("[TEST DEBUG] Generate returned err: %v", err)
		if overrideFile != nil {
			t.Logf("[TEST DEBUG] overrideFile.ChartName = %q", overrideFile.ChartName)
			overridesBytes, _ := yaml.Marshal(overrideFile.Overrides)
			t.Logf("[TEST DEBUG] overrideFile.Overrides =\n%s", string(overridesBytes))
		} else {
			t.Log("[TEST DEBUG] overrideFile is nil")
		}
		// +++ End Debug Logging +++

		// 4. Assert Results
		require.NoError(t, err, "Generate() should not return an error")
		require.NotNil(t, overrideFile, "Generated override file should not be nil")
		assert.Empty(t, overrideFile.Unsupported, "Should be no unsupported structures")
		require.Len(t, overrideFile.Overrides, 3, "Should have 3 overrides")

		// Check imageFromDocker (string)
		valDocker, okDocker := overrideFile.Overrides["imageFromDocker"].(string)
		require.True(t, okDocker, "imageFromDocker override should be a string")
		assert.Equal(t, "mapped-docker.local/dockerio/library/nginx:stable", valDocker, "imageFromDocker value mismatch") // Mapped target + strategy path

		// Check imageFromQuay (string)
		valQuay, okQuay := overrideFile.Overrides["imageFromQuay"].(string)
		require.True(t, okQuay, "imageFromQuay override should be a string")
		assert.Equal(t, "mapped-quay.local/quayio/prometheus/alertmanager:v0.25", valQuay, "imageFromQuay value mismatch") // Mapped target + strategy path

		// Check imageUnmapped (string) - uses default strategy
		valUnmapped, okUnmapped := overrideFile.Overrides["imageUnmapped"].(string)
		require.True(t, okUnmapped, "imageUnmapped override should be a string")
		assert.Equal(t, "harbor.local/gcrio/google-containers/pause:3.2", valUnmapped, "imageUnmapped value mismatch") // Default target + strategy path
	})

	// TODO: Test Generate with strict mode + unsupported
	// TODO: Test Generate with threshold failure
}

// TODO: Add tests for OverridesToYAML function
// TODO: Add tests for ValidateHelmTemplate function

func TestOverridesToYAML(t *testing.T) {
	overrideData := map[string]interface{}{
		"key1": "value1",
		"key2": map[string]interface{}{"nestedKey": "nestedValue"},
	}

	t.Run("Successful marshaling", func(t *testing.T) {
		yamlBytes, err := OverridesToYAML(overrideData)
		require.NoError(t, err, "OverridesToYAML should not fail for valid map")

		// Unmarshal back to verify structure (more robust than string comparison)
		var unmarshalledData map[string]interface{}
		err = yaml.Unmarshal(yamlBytes, &unmarshalledData)
		require.NoError(t, err, "Generated YAML should be unmarshallable")
		assert.Equal(t, overrideData, unmarshalledData, "Unmarshalled data mismatch")

		// Optionally, write to temp file for manual inspection or other tests
		tmpFile, err := os.CreateTemp("", "override-*.yaml")
		require.NoError(t, err, "Failed to create temp file")
		defer func() {
			err := os.Remove(tmpFile.Name())
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Logf("Warning: failed to remove temp file %s: %v", tmpFile.Name(), err)
			}
		}()
		err = os.WriteFile(tmpFile.Name(), yamlBytes, 0600)
		require.NoError(t, err, "Failed to write YAML to temp file")
	})

	// Add other test cases for OverridesToYAML if necessary...
}

// --- Mocking for ValidateHelmTemplate ---

type MockCommandRunner struct {
	RunFunc func(_ string, _ ...string) ([]byte, error)
}

func (m *MockCommandRunner) Run(_ string, arg ...string) ([]byte, error) {
	if m.RunFunc != nil {
		return m.RunFunc("", arg...)
	}
	return nil, nil
}

// --- Remove old exec.Command mocking ---
// var originalExecCommand = exec.Command // Removed
// var mockExecCommand func(command string, args ...string) *exec.Cmd // Removed
// func setupMockExecCommand(t *testing.T, output string, exitCode int) { ... } // Removed
// func TestHelperProcess(t *testing.T) { ... } // Removed

func TestValidateHelmTemplate(t *testing.T) {
	// Create a temporary directory for the test setup
	tmpDir, err := os.MkdirTemp("", "helm-test-validate-*")
	require.NoError(t, err, "Failed to create temp directory")
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Logf("Warning: failed to remove temp directory %s: %v", tmpDir, err)
		}
	}()

	// Create a dummy chart structure within the temp directory
	dummyChartPath := filepath.Join(tmpDir, "test-chart")
	err = os.MkdirAll(filepath.Join(dummyChartPath, "templates"), 0750)
	require.NoError(t, err, "Failed to create dummy chart directories")
	err = os.WriteFile(filepath.Join(dummyChartPath, "Chart.yaml"), []byte("apiVersion: v2\nname: test-chart\nversion: 0.1.0"), 0600)
	require.NoError(t, err, "Failed to write dummy Chart.yaml")
	err = os.WriteFile(filepath.Join(dummyChartPath, "values.yaml"), []byte("replicaCount: 1"), 0600)
	require.NoError(t, err, "Failed to write dummy values.yaml")
	err = os.WriteFile(filepath.Join(dummyChartPath, "templates", "deployment.yaml"), []byte("apiVersion: apps/v1\nkind: Deployment"), 0600)
	require.NoError(t, err, "Failed to write dummy template")

	// Create dummy override content
	dummyOverrides := []byte("someKey: someValue\n")

	t.Run("Valid Template Output", func(t *testing.T) {
		validYAML := `--- 
# Source: chart/templates/service.yaml
apiVersion: v1
`
		mockRunner := &MockCommandRunner{
			RunFunc: func(_ string, _ ...string) ([]byte, error) {
				return []byte(validYAML), nil
			},
		}

		err := ValidateHelmTemplate(mockRunner, dummyChartPath, dummyOverrides)
		assert.NoError(t, err, "Validation should pass for valid YAML output")
	})

	t.Run("Helm Command Error", func(t *testing.T) {
		expectedErr := fmt.Errorf("helm process exited badly")
		mockRunner := &MockCommandRunner{
			RunFunc: func(_ string, _ ...string) ([]byte, error) {
				return []byte("Error: something went wrong executing helm"), expectedErr
			},
		}

		err := ValidateHelmTemplate(mockRunner, dummyChartPath, dummyOverrides)
		assert.Error(t, err, "Validation should fail when helm command fails")
		assert.ErrorContains(t, err, "helm template command failed")
	})

	// ... (keep other sub-tests for ValidateHelmTemplate like Invalid YAML Output if they existed) ...
}

// --- End Mocking for ValidateHelmTemplate ---

// --- Test GenerateOverrides ---

func TestGenerateOverrides(t *testing.T) {
	// Enable debugging for this test
	t.Setenv("IRR_DEBUG", "true")
	defer t.Setenv("IRR_DEBUG", "") // Disable after test

	// Common setup
	targetRegistry := defaultTargetRegistry // Use constant
	sourceRegistries := []string{"docker.io"}
	var excludeRegistries []string
	actualStrategy := strategy.NewPrefixSourceRegistryStrategy() // Use the actual strategy
	verbose := false                                             // Keep test output clean

	// 1. Define the EXPECTED MERGED values structure Helm would create
	mergedValues := map[string]interface{}{
		"parentImage": "docker.io/parent/app:v1", // From parent
		"child": map[string]interface{}{ // Alias used as key
			// Values from parent's 'child:' block take precedence
			"image": map[string]interface{}{
				"repository": "my-child-repo", // Registry might be implicitly docker.io or global? Test assumes detection handles it.
				"tag":        "child-tag",
			},
			// Values only in child's defaults are merged in
			"anotherImage": "docker.io/another/child:stable",
		},
	}

	// Create a mock chart object containing ONLY the merged values
	mockMergedChart := &chart.Chart{
		Metadata: &chart.Metadata{Name: "merged-chart-for-test"},
		Values:   mergedValues,
	}

	// Create a new detector for the test, including detections for child images
	detector := &MockImageDetector{
		DetectedImages: []image.DetectedImage{
			{ // Parent Image
				Path: []string{"parentImage"},
				Reference: &image.Reference{
					Registry:   "docker.io",
					Repository: "parent/app",
					Tag:        "v1",
				},
				Pattern: image.PatternString, // Changed TypeString to PatternString constant
			},
			{ // Child Image (from parent override)
				Path: []string{"child", "image"}, // Path within merged values
				Reference: &image.Reference{
					// Assuming detector resolves missing registry to docker.io based on context or defaults
					Registry:   "docker.io",
					Repository: "my-child-repo",
					Tag:        "child-tag",
				},
				Pattern: image.PatternMap, // Changed TypeMapRegistryRepositoryTag to PatternMap constant
			},
			{ // Child Image (from child defaults)
				Path: []string{"child", "anotherImage"}, // Path within merged values
				Reference: &image.Reference{
					Registry:   "docker.io",
					Repository: "another/child",
					Tag:        "stable",
				},
				Pattern: image.PatternString, // Changed TypeString to PatternString constant
			},
		},
		Unsupported: []image.UnsupportedImage{},
		Error:       nil,
	}

	overrides, err := processChartForOverrides(
		mockMergedChart,
		targetRegistry,
		sourceRegistries,
		excludeRegistries,
		actualStrategy,
		verbose,
		detector,
	)
	require.NoError(t, err)
	require.NotNil(t, overrides)

	// Check parent override (now map)
	assert.Contains(t, overrides, "parentImage")
	if parentImage, ok := overrides["parentImage"].(map[string]interface{}); ok {
		assert.Equal(t, targetRegistry, parentImage["registry"], "Parent registry mismatch")
		assert.Equal(t, "dockerio/parent/app", parentImage["repository"], "Parent repository mismatch")
		assert.Equal(t, "v1", parentImage["tag"], "Parent tag mismatch")
	} else {
		t.Errorf("Parent image override is not a map")
	}

	// Check child override (under its alias/name)
	assert.Contains(t, overrides, "child", "Overrides should contain key for child chart")
	if childOverrides, ok := overrides["child"].(map[string]interface{}); ok {
		// Check image defined in parent values for the child
		assert.Contains(t, childOverrides, "image", "Child overrides should contain image key")
		if childImage, ok := childOverrides["image"].(map[string]interface{}); ok {
			assert.Equal(t, targetRegistry, childImage["registry"], "Child registry mismatch")
			// Repository name comes from parent values, prefixed by strategy, NOW INCLUDES library/
			expectedChildRepo := "dockerio/library/my-child-repo"
			assert.Equal(t, expectedChildRepo, childImage["repository"], "Child repository mismatch")
			assert.Equal(t, "child-tag", childImage["tag"], "Child tag mismatch")
		} else {
			t.Errorf("Child image override is not a map")
		}

		// Check image only defined in child's default values
		assert.Contains(t, childOverrides, "anotherImage", "Child overrides should contain anotherImage key")
		if anotherImage, ok := childOverrides["anotherImage"].(map[string]interface{}); ok {
			assert.Equal(t, targetRegistry, anotherImage["registry"])
			expectedAnotherRepo := "dockerio/another/child"
			assert.Equal(t, expectedAnotherRepo, anotherImage["repository"])
			assert.Equal(t, "stable", anotherImage["tag"])
		} else {
			t.Errorf("Child anotherImage override is not a map")
		}
	} else {
		t.Errorf("Child override section is not a map")
	}

	// Note: This test now directly tests processChartForOverrides with merged values,
	// implicitly covering how GenerateOverrides *should* work if Helm provides
	// correctly merged values to the initial Load call.
}

func TestExtractSubtree(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		path     []string
		expected map[string]interface{}
	}{
		{
			name: "empty path",
			data: map[string]interface{}{
				"key": "value",
			},
			path:     []string{},
			expected: nil,
		},
		{
			name: "simple path",
			data: map[string]interface{}{
				"parent": map[string]interface{}{
					"child": "value",
				},
			},
			path: []string{"parent", "child"},
			expected: map[string]interface{}{
				"parent": map[string]interface{}{
					"child": "value",
				},
			},
		},
		{
			name: "nested path",
			data: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "value",
					},
				},
			},
			path: []string{"level1", "level2", "level3"},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "value",
					},
				},
			},
		},
		{
			name: "array index path",
			data: map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"image": "nginx:latest",
					},
					map[string]interface{}{
						"image": "redis:latest",
					},
				},
			},
			path: []string{"containers", "[1]", "image"},
			expected: map[string]interface{}{
				"containers": []interface{}{
					nil,
					map[string]interface{}{
						"image": "redis:latest",
					},
				},
			},
		},
		{
			name: "invalid array index",
			data: map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"image": "nginx:latest",
					},
				},
			},
			path:     []string{"containers", "[invalid]", "image"},
			expected: nil,
		},
		{
			name: "path with non-existent key",
			data: map[string]interface{}{
				"key1": "value1",
			},
			path:     []string{"nonexistent", "key"},
			expected: map[string]interface{}{},
		},
		{
			name: "mixed types path",
			data: map[string]interface{}{
				"deployment": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"config": map[string]interface{}{
								"image": "test:latest",
							},
						},
					},
				},
			},
			path: []string{"deployment", "containers", "[0]", "config", "image"},
			expected: map[string]interface{}{
				"deployment": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"config": map[string]interface{}{
								"image": "test:latest",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSubtree(tt.data, tt.path)
			assert.Equal(t, tt.expected, result, "extractSubtree() result mismatch")
		})
	}
}

func TestMergeOverrides(t *testing.T) {
	tests := []struct {
		name     string
		dest     map[string]interface{}
		src      map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "merge empty maps",
			dest:     map[string]interface{}{},
			src:      map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "merge into empty destination",
			dest: map[string]interface{}{},
			src: map[string]interface{}{
				"key": "value",
			},
			expected: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name: "merge non-overlapping maps",
			dest: map[string]interface{}{
				"key1": "value1",
			},
			src: map[string]interface{}{
				"key2": "value2",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "merge overlapping primitive values",
			dest: map[string]interface{}{
				"key": "old_value",
			},
			src: map[string]interface{}{
				"key": "new_value",
			},
			expected: map[string]interface{}{
				"key": "new_value",
			},
		},
		{
			name: "merge nested maps",
			dest: map[string]interface{}{
				"nested": map[string]interface{}{
					"key1": "value1",
					"key2": "old_value",
				},
			},
			src: map[string]interface{}{
				"nested": map[string]interface{}{
					"key2": "new_value",
					"key3": "value3",
				},
			},
			expected: map[string]interface{}{
				"nested": map[string]interface{}{
					"key1": "value1",
					"key2": "new_value",
					"key3": "value3",
				},
			},
		},
		{
			name: "merge mixed types",
			dest: map[string]interface{}{
				"string": "old_string",
				"number": 42,
				"nested": map[string]interface{}{
					"key": "value",
				},
			},
			src: map[string]interface{}{
				"string": "new_string",
				"number": 84,
				"nested": map[string]interface{}{
					"new_key": "new_value",
				},
			},
			expected: map[string]interface{}{
				"string": "new_string",
				"number": 84,
				"nested": map[string]interface{}{
					"key":     "value",
					"new_key": "new_value",
				},
			},
		},
		{
			name: "merge map with non-map",
			dest: map[string]interface{}{
				"key": map[string]interface{}{
					"nested": "value",
				},
			},
			src: map[string]interface{}{
				"key": "simple_value",
			},
			expected: map[string]interface{}{
				"key": "simple_value",
			},
		},
		{
			name: "deep nested merge",
			dest: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"key": "old_value",
						},
					},
				},
			},
			src: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"key":     "new_value",
							"new_key": "value",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"key":     "new_value",
							"new_key": "value",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeOverrides(tt.dest, tt.src)
			assert.Equal(t, tt.expected, tt.dest, "mergeOverrides() result mismatch")
		})
	}
}

// MockImageDetector for testing
type MockImageDetector struct {
	DetectedImages []image.DetectedImage
	Unsupported    []image.UnsupportedImage
	Error          error
}

func (m *MockImageDetector) DetectImages(_ interface{}, _ []string) ([]image.DetectedImage, []image.UnsupportedImage, error) {
	return m.DetectedImages, m.Unsupported, m.Error
}

func TestProcessChartForOverrides(t *testing.T) {
	// Common test setup
	targetRegistry := "harbor.local"
	sourceRegistries := []string{"docker.io", "quay.io"}
	excludeRegistries := []string{"internal.registry"}
	actualStrategy := strategy.NewPrefixSourceRegistryStrategy()

	tests := []struct {
		name           string
		chartData      *chart.Chart
		detectedImages []image.DetectedImage
		unsupported    []image.UnsupportedImage
		detectError    error
		expected       map[string]interface{}
		expectError    bool
	}{
		{
			name: "simple chart with one image",
			chartData: &chart.Chart{
				Metadata: &chart.Metadata{Name: "test-chart"},
				Values: map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "docker.io",
						"repository": "nginx",
						"tag":        "latest",
					},
				},
			},
			detectedImages: []image.DetectedImage{
				{
					Path: []string{"image"},
					Reference: &image.Reference{
						Registry:   "docker.io",
						Repository: "nginx",
						Tag:        "latest",
					},
				},
			},
			expected: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   targetRegistry,
					"repository": "dockerio/library/nginx",
					"tag":        "latest",
				},
			},
		},
		{
			name: "chart with excluded registry",
			chartData: &chart.Chart{
				Metadata: &chart.Metadata{Name: "test-chart"},
				Values: map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "internal.registry",
						"repository": "app",
						"tag":        "v1",
					},
				},
			},
			detectedImages: []image.DetectedImage{
				{
					Path: []string{"image"},
					Reference: &image.Reference{
						Registry:   "internal.registry",
						Repository: "app",
						Tag:        "v1",
					},
				},
			},
			expected: map[string]interface{}{}, // No overrides for excluded registry
		},
		{
			name: "chart with non-source registry",
			chartData: &chart.Chart{
				Metadata: &chart.Metadata{Name: "test-chart"},
				Values: map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "gcr.io",
						"repository": "app",
						"tag":        "v1",
					},
				},
			},
			detectedImages: []image.DetectedImage{
				{
					Path: []string{"image"},
					Reference: &image.Reference{
						Registry:   "gcr.io",
						Repository: "app",
						Tag:        "v1",
					},
				},
			},
			expected: map[string]interface{}{}, // No overrides for non-source registry
		},
		{
			name: "chart with multiple images",
			chartData: &chart.Chart{
				Metadata: &chart.Metadata{Name: "test-chart"},
				Values: map[string]interface{}{
					"app": map[string]interface{}{
						"image": map[string]interface{}{
							"registry":   "docker.io",
							"repository": "app",
							"tag":        "v1",
						},
					},
					"sidecar": map[string]interface{}{
						"image": map[string]interface{}{
							"registry":   "quay.io",
							"repository": "helper",
							"tag":        "latest",
						},
					},
				},
			},
			detectedImages: []image.DetectedImage{
				{
					Path: []string{"app", "image"},
					Reference: &image.Reference{
						Registry:   "docker.io",
						Repository: "app",
						Tag:        "v1",
					},
				},
				{
					Path: []string{"sidecar", "image"},
					Reference: &image.Reference{
						Registry:   "quay.io",
						Repository: "helper",
						Tag:        "latest",
					},
				},
			},
			expected: map[string]interface{}{
				"app": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   targetRegistry,
						"repository": "dockerio/library/app",
						"tag":        "v1",
					},
				},
				"sidecar": map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   targetRegistry,
						"repository": "quayio/helper",
						"tag":        "latest",
					},
				},
			},
		},
		{
			name: "chart with detection error",
			chartData: &chart.Chart{
				Metadata: &chart.Metadata{Name: "test-chart"},
				Values:   map[string]interface{}{},
			},
			detectError: fmt.Errorf("detection failed"),
			expectError: true,
		},
		{
			name: "chart with digest instead of tag",
			chartData: &chart.Chart{
				Metadata: &chart.Metadata{Name: "test-chart"},
				Values: map[string]interface{}{
					"image": map[string]interface{}{
						"registry":   "docker.io",
						"repository": "app",
						"digest":     "sha256:1234567890",
					},
				},
			},
			detectedImages: []image.DetectedImage{
				{
					Path: []string{"image"},
					Reference: &image.Reference{
						Registry:   "docker.io",
						Repository: "app",
						Digest:     "sha256:1234567890",
					},
				},
			},
			expected: map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   targetRegistry,
					"repository": "dockerio/library/app",
					"digest":     "sha256:1234567890",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new detector for the test
			detector := &MockImageDetector{
				DetectedImages: tt.detectedImages,
				Unsupported:    tt.unsupported,
				Error:          tt.detectError,
			}

			result, err := processChartForOverrides(
				tt.chartData,
				targetRegistry,
				sourceRegistries,
				excludeRegistries,
				actualStrategy,
				true, // verbose
				detector,
			)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result, "processChartForOverrides() result mismatch")
		})
	}
}

func TestGenerateOverrides_Integration(_ *testing.T) {
	// Integration test for generating overrides
	targetRegistry := defaultTargetRegistry // Use constant
	sourceRegistries := []string{"docker.io"}
	excludeRegistries := []string{}
	actualStrategy := strategy.NewPrefixSourceRegistryStrategy() // Use the actual strategy
	strict := true
	threshold := 100
	chartPath := testChartPath // Use constant

	// Create test chart with various image patterns
	mockChart := &chart.Chart{
		Metadata: &chart.Metadata{Name: "test-chart"},
		Values: map[string]interface{}{
			"image": "docker.io/myapp:v1",
			"sidecar": map[string]interface{}{
				"image": map[string]interface{}{
					"registry":   "docker.io",
					"repository": "helper",
					"tag":        "latest",
				},
			},
			"nonImage": "some-value",
		},
	}

	mockLoader := &MockChartLoader{
		LoadFunc: func(_ string) (*chart.Chart, error) {
			return mockChart, nil
		},
	}

	generator := NewGenerator(
		chartPath,
		targetRegistry,
		sourceRegistries,
		excludeRegistries,
		actualStrategy, // Pass the actual strategy
		nil,
		strict,
		threshold,
		mockLoader,
	)

	overrideFile, err := generator.Generate()
	if err != nil {
		panic(err)
	}

	// Verify overrides were generated correctly
	if overrideFile == nil {
		panic("overrideFile is nil")
	}

	// Check chart metadata
	if overrideFile.ChartPath != chartPath {
		panic("chartPath mismatch")
	}
	if overrideFile.ChartName != "test-chart" {
		panic("chartName mismatch")
	}

	// Check overrides
	if len(overrideFile.Overrides) != 2 {
		panic("unexpected number of overrides")
	}

	// Check image string override - Expect a STRING
	if imageValue, ok := overrideFile.Overrides["image"].(string); ok {
		// Construct expected value based on actual strategy
		expectedValue := targetRegistry + "/dockerio/library/myapp:v1"
		if imageValue != expectedValue {
			panic(fmt.Sprintf("image override value mismatch: got %s, want %s", imageValue, expectedValue))
		}
	} else {
		panic("image override not found or not a string")
	}

	// Check nested image map override
	if sidecar, ok := overrideFile.Overrides["sidecar"].(map[string]interface{}); ok {
		if sidecarImage, ok := sidecar["image"].(map[string]interface{}); ok {
			if sidecarImage["registry"] != targetRegistry {
				panic("sidecar registry mismatch")
			}
			if sidecarImage["repository"] != "dockerio/library/helper" {
				panic("sidecar repository mismatch")
			}
			if sidecarImage["tag"] != "latest" {
				panic("sidecar tag mismatch")
			}
		} else {
			panic("sidecar image not found or wrong type")
		}
	} else {
		panic("sidecar override not found or wrong type")
	}

	// Verify non-image values were not included
	if _, exists := overrideFile.Overrides["nonImage"]; exists {
		panic("non-image value should not be in overrides")
	}
}
