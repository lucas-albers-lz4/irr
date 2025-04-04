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

### 1. Image Relocation Validation
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

### 2. Version Preservation Check

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

### 3. Non-Destructive Change Verification

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

### 4. Path Strategy Testing

Test each supported path strategy (`prefix-source-registry`, potentially others) with various registry patterns and chart structures.

**Target Registry Constraint Testing**:
- Test `prefix-source-registry` with long original repo paths to check against potential target limits (e.g., Harbor project path depth).
- Test image names that might conflict with target registry naming rules (e.g., if ECR were a target, test paths with potentially problematic characters for the `flat` strategy if implemented).

### 5. Subchart and Complex Structure Testing
- Verify correct override path generation using dependency aliases (e.g., `parentchart.alias.image.repository`).
- Test charts with multiple levels of nesting (parent -> child -> grandchild).
- Include test cases with complex value structures:
    - Images nested within lists or multiple levels deep in maps.
    - Charts utilizing CRDs where image references might be less direct (though primary focus remains `values.yaml`).
    - StatefulSets or Deployments referencing multiple distinct images within the same resource block in `values.yaml`.
```yaml
# Example Complex Structure for Testing
global:
  registry: docker.io
someApp:
  image:
    primary:
      repository: myapp/server
      tag: 1.2.3
      # registry: # Uses global
    secondary:
      image: quay.io/utility/helper:latest # Full override needed
  sidecars:
    - name: agent
      image: gcr.io/monitoring/agent:v5
    - name: proxy
      repository: istio/proxyv2 # Implicit docker.io/library
      tag: 1.19.0
      registry: docker.io # Explicit docker.io
```

### 4.1 Unit Tests

- [ ] Test value traversal logic.
- [ ] Test image detection heuristics for all supported and unsupported patterns.
- [ ] Test image string parsing regex and extraction logic.
- [ ] Test Docker Library normalization function.
- [ ] Test registry domain sanitization function.
- [ ] Test `prefix-source-registry` path generation logic.
- [ ] Test override structure generation for various inputs (ensure minimal output).
- [ ] Test subchart alias path construction.
- [ ] Test YAML generation output format.

### 4.2 Integration Tests

- [ ] **Core Use Case Test:**
    - [ ] Add specific test using `kube-prometheus-stack` chart (or equivalent complex chart).
    - [ ] Configure test to use `--source-registries docker.io,quay.io` and `--target-registry harbor.home.arpa`.
    - [ ] **Validation:**
        - [ ] Generate `override.yaml` using the tool.
        - [ ] Run `helm template <chart> <original_values>` and capture image lines (e.g., `... | grep 'image:' > original_images.txt`).
        - [ ] Run `helm template <chart> <original_values> -f override.yaml` and capture image lines (e.g., `... | grep 'image:' > overridden_images.txt`).
        - [ ] Compare `original_images.txt` and `overridden_images.txt` to verify:
            - Images from `docker.io`, `quay.io` are redirected to `harbor.home.arpa` using the correct path strategy (`harbor.home.arpa/dockerio/...`, `harbor.home.arpa/quayio/...`).
            - Tags/digests remain identical.
            - Images from other registries (e.g., `registry.k8s.io`) are unchanged.
            - Images excluded via `--exclude-registries` are unchanged.
        - [ ] Verify the `helm template ... -f override.yaml` command completes successfully.
- [ ] Curate test chart corpus: Select top ~50 from Artifact Hub + specific complex examples (including the core use case chart).
- [ ] Implement validation test harness/script automating the comparison steps above:
  - [ ] **Image Relocation Validation:** Compare images in `helm template` output (original vs. overridden) using parsing/diffing. Verify target registry, path strategy application, tag/digest preservation.
  - [ ] **Version Preservation Check:** Diff `Chart.yaml` `version` and `appVersion`.
  - [ ] **Non-destructive Change Verification:** Diff `helm template` output ignoring expected image lines and ensuring no unrelated values are changed.
  - [ ] **Path Strategy Testing:** Run tests for each implemented strategy.
  - [ ] **Subchart Handling:** Use charts with multiple nesting levels and aliases. Verify correct override paths.
  - [ ] **Complex Structures:** Test charts with images in lists, nested maps, etc.
  - [ ] **CLI Options Testing:** Create test cases covering combinations of CLI flags.
  - [ ] **Error Handling Validation:** Test scenarios triggering each defined exit code and verify error message structure.
  - [ ] **Dry Run Test:** Verify no file output with `--dry-run`.
  - [ ] **Strict Mode Test:** Verify failure vs. warning behavior with `--strict`.
  - [ ] **Exclusion Test:** Verify `--exclude-registries` correctly skips processing images from the specified *source registries*. (e.g., `--exclude-registries docker.io` should not modify any `docker.io` images).
  - [ ] **Threshold Test:** Verify behavior with different `--threshold` values.

### 6. Command-Line Option Validation

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

## 7. Error Handling and Exit Code Testing

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

## 8. Performance Benchmarking

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
*Note: Actual charts, times, and memory usage to be filled in during testing.*

## 9. Debug Logging Testing

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

## 5. Documentation

- [ ] Create `README.md`: Overview, Installation, Quick Start, Basic Usage (including the core Prometheus->Harbor example).
- [ ] Add detailed CLI Reference section (Flags and Arguments).
- [ ] Document Path Strategies Explained (include sanitization rules).
- [ ] Add Examples / Tutorials section.
- [ ] Create Troubleshooting / Error Codes guide.
- [ ] Add Contributor Guide (basic setup, testing).

## 6. Release Process

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

The project includes several types of tests that can be run using different make targets:

### Unit Tests
```bash
make test
```
Runs the Go unit tests for all packages using `go test -v ./...`

### Chart Tests
```bash
make test-charts [TARGET_REGISTRY=your.registry.com]
```
Runs the chart testing script that validates the tool against a variety of Helm charts. This:
- Builds the binary if needed
- Creates required test directories (`test/results`, `test/overrides`)
- Runs `test-charts.sh` with the specified registry (defaults to harbor.home.arpa)
- Generates test results in `test/results/results.txt`
- Creates override files in `test/overrides/`

### Directory Structure
The testing framework uses several directories:
- `test-data/charts/`: Contains test chart fixtures
- `test/results/`: Contains test execution results
- `test/overrides/`: Contains generated override files
- `test/tools/`: Contains testing scripts

### Clean Up
```bash
make clean
```
Removes all generated test artifacts, including:
- Build directory
- Test charts directory
- Test results
- Generated overrides
