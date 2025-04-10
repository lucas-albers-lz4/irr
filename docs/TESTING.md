# Helm Image Override Testing Plan

## Objective
Validate that the `helm-image-override` tool successfully redirects container images from public registries to a private registry while maintaining:
- Immutability of image versions/tags/digests
- Preservation of original chart versions
- Integrity of non-image related values
- Proper path strategy application
- Robust error handling and reporting

## Test Scope
✅ **Target Charts**: Top 50 Helm charts from Artifact Hub (sorted by popularity), plus curated examples of complex structures.
✅ **Critical Validation Points**:
1. Accurate image URI transformation (including tags and digests)
2. Strict version/tag/digest preservation
3. Non-destructive modification of values.yaml
4. Comprehensive registry pattern handling (including Docker Library normalization)
5. Proper subchart dependency handling (including alias resolution and deep nesting)
6. Correct handling of supported image value structures (map with registry/repo/tag, map with repo/tag, string value)
7. Clear reporting/failure for unsupported image value structures (especially with `--strict`)
8. Accurate exit codes for success and failure scenarios
9. Informative and structured error messages

---

## Test Strategy

Our testing strategy combines focused unit tests for deterministic functions with broader, outcome-focused tests for the heuristic-based core detection logic, supplemented by comprehensive integration tests against real Helm charts.

### 1. Unit and Focused Tests (Go Tests - `make test`)

These tests verify specific functions and isolated logic components:

*   **Strict Unit Tests:** Target deterministic functions with clear input/output relationships (e.g., `ParseImageReference`, `NormalizeRegistry`, `IsSourceRegistry`, `tryExtract...` functions). These tests precisely validate the expected output for given inputs.
*   **Outcome-Focused Tests:** Target the core `DetectImages` function and its interactions with different contexts (e.g., `TestDetectImages_ContextVariations`, `TestImageDetector_ContainerArrays`). Due to the heuristic nature of `DetectImages` in finding images within arbitrary YAML structures, these tests prioritize validating the *presence* of expected final image references (repository, tag, digest) rather than asserting the exact internal detection path or pattern. This approach reduces test fragility when refactoring the complex detection logic while still ensuring the function achieves its main goal in various scenarios.

### 2. Integration & Chart Validation Tests (`make test-charts`)

These tests provide end-to-end validation using real Helm charts:

*   Leverage the `test/tools/test-charts.sh` script.
*   Process a corpus of real-world charts (Top 50 from Artifact Hub + complex examples).
*   Validate the generated `override.yaml` by rendering Helm templates (`helm template ... -f override.yaml`).
*   Compare rendered manifests to ensure correct image relocation, version preservation, and non-destructive changes.
*   Verify behavior against different path strategies and registry filtering options.
*   Crucial for catching regressions and validating the tool's effectiveness in practical use cases.

### 3. Image Relocation Validation
**Regex Pattern Focus**:
```regex
# Tag-based
^(?:(?P<registry>docker\.io|quay\.io|gcr\.io|ghcr\.io)/)?(?P<repository>[a-zA-Z0-9\-_/.]+):(?P<tag>[a-zA-Z0-9\-.]+)$

# Digest-based
^(?P<registry>docker\.io|quay\.io|gcr\.io|ghcr\.io/)?(?P<repo>[a-zA-Z0-9\-_/.]+)(?:@(?P<digest>sha256:[a-fA-F0-9]{64}))?$
```

#### Test Matrix:

| Case Type | Original Image | Expected Output (`prefix-source-registry` strategy) |
|-----------|-----------------|---------------------------------------------------|
| Standard | docker.io/nginx:1.23 | myharbor.internal:5000/dockerio/nginx:1.23 |
| Nested Path | quay.io/project/img:v4.2 | myharbor.internal:5000/quayio/project/img:v4.2 |
| Implicit Registry | alpine:3.18 | myharbor.internal:5000/dockerio/library/alpine:3.18 |
| Registry+Repository | gcr.io/google-samples/hello-app:1.0 | myharbor.internal:5000/gcrio/google-samples/hello-app:1.0 |
| Digest | quay.io/prometheus/prometheus@sha256:abc... | myharbor.internal:5000/quayio/prometheus/prometheus@sha256:abc... |
| Excluded Registry | internal.repo/app:v1 | internal.repo/app:v1 |

### 4. Version Preservation Check

#### Validation Commands:

```bash
# Chart versions
diff <(yq eval '.version' original/Chart.yaml) <(yq eval '.version' migrated/Chart.yaml)

# App versions
diff <(yq eval '.appVersion' original/Chart.yaml) <(yq eval '.appVersion' migrated/Chart.yaml)

# Image tags/digests (requires parsing manifests or overrides)
# Example conceptual check:
# yq eval '.. | select(has("image")) | .image' overridden-manifest.yaml | grep '@sha256:' # Verify digests preserved
# yq eval '.. | select(has("repository")) | .tag' overrides.yaml # Check tags weren't added/removed inappropriately
```

### 5. Non-Destructive Change Verification

#### Checklist:
- ☐ No values.yaml changes except specified image references
- ☐ 100% template file parity (ignoring generated files)
- ☐ Matching Helm template output (excluding overridden image fields)

```bash
# Generate overrides (assuming ./chart is original)
helm-image-override --chart-path ./chart --target-registry myharbor.internal:5000 --source-registries docker.io,quay.io,gcr.io,ghcr.io --output-file ./overrides.yaml

# Compare manifests ignoring image lines
helm template ./chart > original.yaml
helm template ./chart -f overrides.yaml > migrated.yaml
diff --ignore-matching-lines='image:' --ignore-matching-lines='repository:' --ignore-matching-lines='registry:' original.yaml migrated.yaml
```

### 6. Path Strategy Testing

Test each supported path strategy (`prefix-source-registry`, potentially others) with various registry patterns and chart structures.

**Target Registry Constraint Testing**:
- Test `prefix-source-registry` with long original repo paths to check against potential target limits (e.g., Harbor project path depth).
- Test image names that might conflict with target registry naming rules (e.g., if ECR were a target, test paths with potentially problematic characters for the `flat` strategy if implemented).

### 7. Subchart and Complex Structure Testing
- Verify correct override path generation using dependency aliases (e.g., `parentchart.alias.image.repository`).
- Test charts with multiple levels of nesting (parent -> child -> grandchild).
- Include test cases with complex value structures:
    - Images nested within lists or multiple levels deep in maps.
    - Charts utilizing CRDs where image references might be less direct (though primary focus remains `values.yaml`).
    - StatefulSets or Deployments referencing multiple distinct images within the same resource block in `values.yaml`.

### 7.1. Complex Chart Testing Framework

For particularly complex charts like cert-manager, kube-prometheus-stack, and others with multiple distinct components, we use a specialized testing approach that maintains comprehensive coverage while providing better troubleshooting capabilities:

#### 7.1.1. Component-Group Testing Strategy

The strategy divides complex charts into logical component groups and tests each group as a subtest:

```go
// Example for cert-manager components
componentGroups := []struct {
    name               string    // Group name
    components         []string  // Components in this group
    threshold          int       // Success threshold percentage 
    expectedImages     int       // Expected number of images to find
    criticalComponents bool      // Whether this group contains critical components
}{
    {
        name:               "core_controllers",
        components:         []string{"cert-manager-controller", "cert-manager-webhook"},
        threshold:          100,
        expectedImages:     4,
        criticalComponents: true,
    },
    {
        name:               "supporting_services",
        components:         []string{"cert-manager-cainjector", "cert-manager-startupapicheck"},
        threshold:          95,
        expectedImages:     2,
        criticalComponents: false,
    },
}
```

#### 7.1.2. Testing Framework Integration

This approach integrates with our existing testing framework:

- **Run Method**: Each component group runs as a subtest (`t.Run()`)
- **Failure Handling**: Core/critical component failures can trigger test failure
- **Reporting**: Results are aggregated into the standard reporting format
- **Thresholds**: Different thresholds can be applied per component group
- **Debugging**: Debug logs segregated by component group

#### 7.1.3. Verification Points for Each Component Group

For each component group, we verify:

1. **Image Detection**: All expected images are found and correctly processed
2. **Path Generation**: Proper path strategy application across all components
3. **Version Integrity**: Tags/digests preserved across all components
4. **Registry Handling**: Correct source registry filtering (including exclusions)
5. **Error Reporting**: Structured error messages for any issues

#### 7.1.4. Selective Testing

This structure allows for selective testing during development:

```bash
# Test only the core controllers component group
go test -v ./... -run TestCertManager/core_controllers

# Test with debug logging for a specific component group
go test -v ./... -run TestCertManager/supporting_services -debug
```

#### 7.1.5. Implementation Guidelines

When implementing tests for complex charts:

1. **Group Definition**:
   - Group components based on functional relationship
   - Keep groups small enough to be manageable (2-4 components per group)
   - Define appropriate thresholds for each group
   - Document expected image counts for validation

2. **Test Structure**:
   - Use table-driven subtests with consistent structure
   - Implement proper test setup and teardown for each group
   - Use shared utilities for common verification tasks
   - Maintain thorough error context for debugging

3. **Failure Criteria**:
   - Define clear success/failure criteria for each component group
   - Differentiate between critical and non-critical failures
   - Apply appropriate thresholds based on component complexity
   - Include summary of all failures in test output

This approach balances the thoroughness of testing each component with the efficiency of maintaining a cohesive test structure.

### 8. Path Strategy and Registry Mapping Testing

**Path Strategy Testing:**
- Test each path strategy with various registry patterns
- Verify correct handling of Docker Hub library images (`nginx` → `library/nginx`)
- Ensure registry sanitization works correctly (`registry.k8s.io` → `registryk8sio`)
- Validate the path generation with and without registry mappings

**Registry Mapping File Handling:**
- **File Path Handling:**
  - Use `filepath.Abs()` in tests when creating temporary mapping files
  - Set the `IRR_TESTING=true` environment variable in tests to bypass working directory checks
  - Always use a unique temporary filename within the current working directory
  - Ensure proper cleanup with error checking: `defer func() { err := os.Remove(file); if err != nil { t.Logf("Warning: %v", err) } }()`

- **Format Testing:**
  - Test both supported formats:
    - Simple map: `docker.io: target-registry/docker-mirror`
    - Structured format with mappings array
  - Test edge cases:
    - Empty mapping file
    - Mapping file with no entries for test registries
    - Mapping file with inconsistent formatting (extra whitespace, etc.)

- **Integration Test Guidelines:**
  - Create mapping files in a temporary directory using the test harness
  - Use absolute paths when passing mapping files to commands
  - Validate that the generated override values correctly reflect the configured mappings

### 9. Command-Line Option Validation

Test all CLI options individually and in combination:
- `--chart-path` (directory and .tgz)
- `--target-registry` (with and without port)
- `--source-registries` (single, multiple, including potential edge cases)
- `--output-file` (writing to file vs stdout)
- `--path-strategy` (each implemented strategy)
- `--verbose` (check for increased output detail)
- `--dry-run` (verify no file output, only console preview)
- `--strict` (ensure failure on unsupported structures vs. warning)
- `--exclude-registries` (verify specified registries are skipped)
- `--threshold` (test behavior with different percentages)
- `--debug` (enable debug logging during test execution)

### 10. Debug Mode Testing

The `--debug` flag can be used during testing to enable detailed debug logging. This is particularly useful for:

- Troubleshooting test failures
- Understanding image detection paths
- Verifying registry mapping behavior
- Tracing override generation logic

To enable debug logging in tests:

```bash
# For all tests in a package
go test -v ./... -debug

# For a specific test
go test -v ./... -run TestSpecificTest -debug

# For integration tests
go test -v ./test/integration/... -debug
```

The debug output will include detailed information about:
- Image detection process
- Registry mapping decisions
- Override generation steps
- File operations and validation

This is especially helpful when:
- Investigating why certain images aren't being detected
- Understanding how registry mappings are being applied
- Debugging path strategy application
- Tracing the flow of strict mode validation

### 11. Unsupported Image Handling

The tool must properly identify and handle unsupported image structures. This is particularly important when running in strict mode.

#### Unsupported Structure Types
The following types of structures should be identified and handled appropriately:

1. **Template Variables in Maps**
   - Detected via `UnsupportedTypeTemplateMap`
   - Example: When template variables are found in map-based image definitions

2. **Tag and Digest Conflicts**
   - Detected via `UnsupportedTypeMapTagAndDigest`
   - Example: When both tag and digest are present in the same image definition

3. **Non-Source Registry Images**
   - Detected via `UnsupportedTypeNonSourceImage`
   - Example: When an image is from a registry not in the source list (in strict mode)

4. **Invalid Map Structures**
   - Detected via `UnsupportedTypeMapParseError`
   - Example: When a map structure is invalid after normalization

5. **Template Variables in Strings**
   - Detected via `UnsupportedTypeTemplateString`
   - Example: When template variables are found in string-based image definitions

6. **String Parse Errors**
   - Detected via `UnsupportedTypeStringParseError`
   - Example: When a string-based image reference fails to parse

#### Testing Approach
For each unsupported structure type:
1. Create test cases that trigger the specific unsupported type
2. Verify correct error type is returned
3. Validate error message contains appropriate context
4. Confirm behavior differs between strict and non-strict modes
5. Check that unsupported structures are properly collected and reported

Example Test Structure:
```go
func TestUnsupportedStructures(t *testing.T) {
    cases := []struct {
        name           string
        input         map[string]interface{}
        expectedType  UnsupportedType
        expectedError string
        strictMode    bool
    }{
        {
            name: "template in map",
            input: map[string]interface{}{
                "image": map[string]interface{}{
                    "repository": "{{ .Values.image }}",
                },
            },
            expectedType: UnsupportedTypeTemplateMap,
            expectedError: "template variable detected",
            strictMode: true,
        },
        // Additional test cases for other unsupported types
    }
    // Test implementation
}
```

#### Validation Points
- Verify unsupported structures are properly detected
- Confirm error messages are clear and actionable
- Check that strict mode fails appropriately
- Validate non-strict mode continues processing
- Ensure all unsupported structures are reported
- Verify error context includes value path information

## Test Environment

### Core Toolchain:

```bash
# Generate overrides
helm-image-override \
  --chart-path ./original-chart \
  --target-registry myharbor.internal:5000 \
  --source-registries docker.io,quay.io,gcr.io,ghcr.io \
  --output-file ./overrides.yaml

# Apply overrides and generate manifests
helm template original-chart/ > original-manifest.yaml
helm template original-chart/ -f overrides.yaml > overridden-manifest.yaml

# Comparison & Validation
# Check that only specified registries were rewritten using manifest diffs or specific parsing
# Example: grep 'myharbor.internal:5000' overridden-manifest.yaml | grep -v 'quayio\|gcrio\|dockerio' # Should be empty if only source registries were targeted

# Sanity test
helm install --dry-run my-release original-chart/ -f overrides.yaml > /dev/null && echo "Validation OK" || echo "Validation FAILED"
```

### Automation Framework:

- Bulk processing script for test chart corpus (Top 50 + complex examples).
- Validation pipeline with stages:
  - Image transformation audit (correct registry, path, tag/digest)
  - Version integrity check (Chart.yaml versions)
  - Value integrity check (diff non-image values)
  - Installation sanity test (`helm install --dry-run`)
  - Exit code verification
  - Error message format verification (see Section 7)

### Advanced Environment Testing (CI/CD Focus)

- **Air-Gapped Simulation**:
    - Test against charts where all expected source images are *pre-mirrored* to the target registry.
    - Ensure the tool correctly generates overrides pointing to the local mirror (`--target-registry`).
    - Validate that no attempts are made to reference external source registries in the overrides.
    - *Note*: Requires environment setup (e.g., using `skopeo sync`) separate from the tool itself. Test assumes mirroring is complete.
- **Custom CA Bundles**:
    - *If* a feature like `--ca-bundle` is implemented for potential future template analysis or validation features, test its usage with a local registry using self-signed certificates.
- **Authentication**:
    - Tool assumes environment (Docker client, Kubeconfig) handles target registry authentication. Tool itself does not handle credentials. Testing confirms overrides work in an authenticated context.

## 9. Error Handling and Exit Code Testing

Verify correct exit codes and informative, structured error messages for various scenarios:

| Scenario | Expected Exit Code | Error Message Detail Level |
|----------|-------------------|----------------------------|
| Success | 0 | Minimal / Verbose option |
| General runtime error | 1 | Specific internal error source |
| Input/configuration error (bad path, invalid registry format) | 2 | Clear indication of faulty input |
| Chart parsing error (malformed YAML) | 3 | File and line number if possible |
| Image processing error (unparsable reference string) | 4 | Path in values, original value, parsing issue |
| Unsupported structure error (with --strict) | 5 | Path in values, description of unsupported structure |
| Threshold not met (--threshold) | (Define specific code, e.g., 6) | Summary of match rate vs threshold |

**Error Message Format Standard**:
Errors related to specific values should ideally follow a structured format to aid parsing and debugging:
```text
Error: <General error description>
- Path: <dot.notation.path.in.values>
  Original: "<original_value>"
  Issue: <Specific problem (e.g., "Invalid image format", "Unsupported structure")>
  Code: <Internal error code, optional (e.g., IMG-PARSE-001)>
  Fix Suggestion: <Optional hint (e.g., "Ensure image format is 'repo:tag' or 'repo@digest'")>
```
Test cases should validate that errors conform to this structure where applicable.

## 10. Performance Benchmarking

Establish baseline performance metrics to understand resource requirements and scalability.

**Methodology**:
- Run the tool against charts of varying complexity (measured by number of subcharts, size of `values.yaml`, total number of image references).
- Execute on standardized test environments (e.g., specific cloud instance types or local machine specs).
- Measure execution time and peak memory usage.

**Target Metrics**:
```markdown
| Chart Complexity        | Example Chart(s)        | Processing Time (avg ± stddev) | Peak Memory Usage (avg) | Test Env Spec | Notes                                   |
|-------------------------|-------------------------|--------------------------------|-------------------------|---------------|-----------------------------------------|
| Simple (0-2 Subcharts)  | `bitnami/nginx`         | < 1s                           | < 50MB                  | `t3.medium`   | Baseline                                |
| Medium (5-15 Subcharts) | `prometheus-community/kube-prometheus-stack` | ~2-5s                        | ~100-200MB              | `t3.medium`   | Representative common use case        |
| Complex (20+ Subcharts) | (Identify large charts) | ~10-30s                        | ~250-500MB              | `t3.large`    | Stress test, potential memory limits |
| Large Values File       | (Chart w/ >5k lines YAML) | TBD                            | TBD                     | `t3.large`    | Test YAML parsing efficiency            |
```
*Note: The charts listed above are examples and may not match the actual charts available in `test-data/charts/`. Actual charts, times, and memory usage to be filled in during testing.*

## 11. Debug Logging Testing

### Debug Output Validation
Test the debug logging functionality with the `--debug` flag:

| Test Case | Expected Debug Output |
|-----------|---------------------|
| Function Entry/Exit | Verify entry/exit logs for key functions (IsSourceRegistry, GenerateOverrides, etc.) |
| Value Dumps | Check detailed value dumps at critical processing points |
| Error Context | Ensure debug logs provide additional context for errors |
| Performance Impact | Measure overhead of debug logging when enabled |

### Integration with Existing Tests
- Add debug output validation to existing test cases:
  - Verify debug logs during image detection
  - Check debug output during override generation
  - Validate debug context in error scenarios
  - Ensure debug logs respect verbosity levels

### Debug Log Format
Debug messages should follow a consistent format:
```text
[DEBUG] FunctionName: Message
[DEBUG] Value dump: <structured_data>
[DEBUG] Error context: <error_details>
```

Test cases should verify this format is maintained across all debug output.

## Success Criteria

- **Critical**:
  - 100% of regex-matched, non-excluded images from specified source registries relocated correctly (respecting path strategy).
  - 0 version/tag/digest modifications in any chart image reference unless intended by strategy.
  - 100% of successfully processed charts pass `helm install --dry-run`.
  - Correct exit codes produced for all defined test scenarios.
  - Error messages are informative and follow the specified structure for value-related issues.
- **Warning/Threshold**:
  - Configurable image match rate threshold (e.g., `--threshold 98`) can allow runs to pass with warnings if some complex/unsupported images are skipped (requires explicit flag). Default threshold is 100%.

## Risk Analysis

### Potential Challenges

❗ **Complex/Non-Standard Image References**
Charts using:
- Dynamic tags based heavily on `tpl` functions within values (e.g., `tag: {{ include "mychart.imagetag" . }}`)
- Obscure value structures not matching standard patterns.
- Images defined entirely outside `values.yaml` (e.g., hardcoded in templates - *out of scope but good to note*).

❗ **Composite Charts & Dependencies**
- Aliases that conflict or are ambiguous.
- Conditional dependency enablement (`condition` field in `Chart.yaml`) affecting which values are active.

### Mitigation Plan

- Strict adherence to processing only clearly identified patterns in `values.yaml`.
- Comprehensive testing with diverse real-world charts (Top 50 +).
- Clear documentation on supported vs. unsupported structures.
- Implement `--strict` flag for users needing guaranteed processing or failure.
- Add specific test cases for alias resolution and conditional dependencies.

## Reporting Format

### Summary Table:

```markdown
| Chart Name    | Total Images Found | Relocated | Skipped (Excluded) | Skipped (Unsupported) | Install Test | Exit Code | Notes |
|---------------|--------------------|-----------|--------------------|-----------------------|--------------|-----------|-------|
| nginx-ingress | 5                  | 5         | 0                  | 0                     | ✅ Pass      | 0         |       |
| cert-manager  | 3                  | 2         | 0                  | 1                     | ✅ Pass      | 0 (or 6 if threshold used) | Unsupported structure in values |
| complex-chart | 10                 | 8         | 1 (private)        | 1                     | ⚠️ Fail      | 3         | Chart parsing error |
```

### Detailed Findings (Example JSON Output from Test Runner):

```json
{
  "chart": "redis",
  "status": "ERROR",
  "exit_code": 4,
  "summary": {
    "total_images": 3,
    "relocated": 2,
    "skipped_excluded": 0,
    "skipped_unsupported": 1
  },
  "errors": [
    {
      "path": "cluster.slave.image",
      "original": "redislabs/redis:latest:invalid", // Example invalid format
      "issue": "Invalid image format",
      "code": "IMG-PARSE-001"
    }
  ],
  "install_check": "N/A (Processing Failed)"
}
```

## Implementation Answers (Reference from Design Doc)

*These sections remain relevant context for testing.*

### 1. Image Digests Handling
*Covered in Relocation Validation and Version Preservation.*

### 2. Private Dependency Verification
*Covered by `--exclude-registries` testing and test matrix.*

### 3. Success Thresholds
*Testing covered by `--threshold` flag validation and Success Criteria.*

### 4. Docker Library Image Handling
*Covered in Relocation Validation test matrix.*

## 12. Documentation

- [ ] Create `README.md`: Overview, Installation, Quick Start, Basic Usage (including the core Prometheus->Harbor example).
- [ ] Add detailed CLI Reference section (Flags and Arguments).
- [ ] Document Path Strategies Explained (include sanitization rules).
- [ ] Add Examples / Tutorials section.
- [ ] Create Troubleshooting / Error Codes guide.
- [ ] Add Contributor Guide (basic setup, testing).

## 13. Release Process

- [ ] Set up Git tagging for versioning (e.g., SemVer).
- [ ] Create release builds for target platforms (Linux AMD64, macOS AMD64/ARM64).
- [ ] Publish binaries (e.g., GitHub Releases).
- [ ] Publish documentation (e.g., alongside code or separate site).
- [ ] Setup automated release pipeline using GitHub Actions (triggered by tags).

## Next Steps

- Establish test chart corpus (Top 50 list + curated complex examples).
- Implement baseline validation scripts (manifest diffing, exit code checks).
- Develop automated reporting system (generating summary/detailed findings).
- Implement performance benchmark test runs.
- Schedule manual audit sessions for complex chart failures.

## Running Tests

## Test Suites

The project has two complementary test suites:

### 1. Unit and Integration Tests (`make test`)
These tests run as part of the standard Go test suite and include:
- Unit tests for individual packages
  - Uses in-memory filesystem (afero.MemMapFs) for file operations to ensure test isolation and reliability
  - Particularly important for registry mapping tests and file operations
- Integration tests using controlled test fixtures in `test-data/charts/`
- Tests for specific functionality like:
  - Basic chart processing
  - Parent/child chart relationships
  - Complex chart handling
  - Dry-run functionality
  - Strict mode behavior
  - Registry mapping file handling with in-memory filesystem
These tests are fast, deterministic, and suitable for CI/CD pipelines.

### Test Environment Best Practices

#### Using In-Memory Filesystem
For tests involving file operations (especially in `pkg/registry`), use afero's MemMapFs:

```go
// Create a memory-backed filesystem for testing
fs := afero.NewMemMapFs()

// Set up test directories/files in memory
require.NoError(t, fs.MkdirAll("/tmp", 0o755))
require.NoError(t, afero.WriteFile(fs, "/tmp/test.yaml", []byte("content"), 0o644))

// Use the memory filesystem in your tests
result, err := YourFunction(fs, "/tmp/test.yaml")
```

Benefits:
- No real filesystem interaction
- Faster test execution
- Consistent across different environments
- No cleanup needed
- No permission issues
- No path traversal concerns in tests

#### Test File Operations
When testing functions that work with files:
1. Always use `afero.Fs` interface in your production code
2. Use `afero.NewMemMapFs()` in tests
3. Use `afero.NewOsFs()` in production
4. Properly handle permissions (0o644 for files, 0o755 for directories)
5. Clean up resources even in memory filesystem (good practice)

### 2. Chart Validation Tests (`make test-charts`)
These tests use the `test/tools/test-charts.sh` script to validate against real-world charts:
- Tests against multiple chart types
- Validates actual chart rendering
- Tests with Harbor registry integration
- Generates detailed test results
These tests take longer to run and are better suited for validation testing.

### Test Directory Structure
- `test-data/charts/`: Contains test fixtures used by integration tests
- `test/results/`: Contains results from chart validation tests
- `test/overrides/`: Contains generated override files from tests
- `test/tools/`: Contains testing scripts
- `test/integration/`: Contains Go integration tests

### Running Tests
```bash
# Run unit and integration tests
make test

# Run chart validation tests (optionally specify target registry)
make test-charts [TARGET_REGISTRY=your.registry.com]

# Clean all test artifacts
make clean
```

### Adding New Test Charts
To add new charts for testing:
1. Place the chart in the `test-data/charts/` directory
2. Ensure the chart follows the expected structure
3. If needed, update the integration tests in `test/integration/` to use the new chart

When adding tests that use specific charts, always check if the chart exists and skip the test if it doesn't, to ensure the test suite can run successfully regardless of which charts are available.
