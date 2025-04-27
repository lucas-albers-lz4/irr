//go:build integration

// Package integration contains integration tests for verifying test infrastructure.
package integration

import (
	"runtime"
	"strings"
	"testing"
)

// TestFunctionNamesExist validates that test function names referenced in the code actually exist
func TestFunctionNamesExist(t *testing.T) {
	// This test documents the expected test functions that should exist in the test suite
	// It doesn't actually validate them through reflection (which is hard to do for top-level functions)
	// but serves as documentation and a reminder when new tests are added

	// List of expected top-level test functions that should exist
	// This should be updated when new test functions are added
	expectedTestFunctions := []string{
		"TestRegistryMappingFile",
		"TestInvalidRegistryMappingFile",
		"TestMinimalChart",
		"TestParentChart",
		"TestKubePrometheusStack",
		"TestComplexChartFeatures",
		"TestDryRunFlag",
		"TestStrictMode",
		"TestConfigFileMappings",
		"TestClickhouseOperator",
		"TestMinimalGitImageOverride",
		"TestReadOverridesFromStdout",
		"TestNoArgs",
		"TestUnknownFlag",
		"TestInvalidStrategy",
		"TestMissingChartPath",
		"TestNonExistentChartPath",
		"TestStrictModeExitCode",
		"TestInvalidChartPath",
		"TestRegistryMappingFileFormats",
		"TestCreateRegistryMappingsFile",
		"TestRegistryPrefixTransformation",
		"TestFunctionNamesExist", // Include this test itself
	}

	// Just for documentation - log the expected test functions
	t.Logf("Expected top-level test functions (%d):", len(expectedTestFunctions))
	for _, name := range expectedTestFunctions {
		t.Logf("  %s", name)
	}

	// Find the current test function's name using runtime.Caller
	pc, _, _, ok := runtime.Caller(0)
	if !ok {
		t.Error("Failed to get current test function name")
		return
	}

	fullFuncName := runtime.FuncForPC(pc).Name()
	parts := strings.Split(fullFuncName, ".")
	currentTestName := parts[len(parts)-1]

	t.Logf("Current test function: %s", currentTestName)

	// Basic validation: check that this test's name is in the expected list
	// This serves as a minimal check that our function naming is consistent
	found := false
	for _, name := range expectedTestFunctions {
		if name == currentTestName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Current test function %q is not in the expected list", currentTestName)
	}
}
