# TODO.md - Helm Image Override Implementation Plan

## 1. Project Setup

- [x] Initialize Go module: `go mod init <module-path>`
- [x] Set up basic directory layout: `cmd/helm-image-override/`, `pkg/`, `internal/`, `test/`
- [x] Add essential Go dependencies:
  - [x] `helm.sh/helm/v3/pkg/chartutil` (for chart loading utilities)
  - [x] `helm.sh/helm/v3/pkg/chartloader` (for loading chart archives/dirs)
  - [x] `sigs.k8s.io/yaml` (for YAML parsing/serialization)
  - [x] Standard libraries (`fmt`, `os`, `log`, `path/filepath`, `strings`, `regexp`)
- [x] Create Makefile with targets: `build`, `test`, `lint`, `clean`, `run`
- [x] Configure basic GitHub Actions CI workflow: linting, building, running unit tests

## 2. Core Implementation

### 2.1 Chart Loading

- [x] Implement chart loading from filesystem path (directory) using `chartloader.LoadDir`
- [x] Add support for `.tgz` archive loading using `chartloader.LoadFile`
- [x] Implement `values.yaml` parsing into a nested map structure (`map[string]interface{}`) using `sigs.k8s.io/yaml`
- [x] Implement `Chart.yaml` parsing to extract metadata and dependencies (using `chart.Chart` struct)
- [x] Implement recursive loading/processing logic for subcharts identified in parent `Chart.yaml` `dependencies` section (handling aliases)

### 2.2 Image Processing

- [x] Implement recursive value traversal function for the nested `values` map.
- [x] Implement image reference detection heuristics:
  - [x] Detect map containing `registry`, `repository`, `tag` string keys.
  - [x] Detect map containing `repository`, `tag` string keys (implies `docker.io` registry).
  - [x] Detect single string value assigned to a key named `image` (e.g., `image: myrepo/myimage:tag`).
  - [x] Add detection and warning for image-like structures within lists (initially unsupported for overrides).
  - [x] Explicitly handle and warn/error on unsupported structures (e.g., non-string tags, split keys).
- [x] Define and compile regex patterns for image parsing (as per DEVELOPMENT.md 6.1.3):
  - [x] Tag-based reference pattern: `^(?:(?P<registry>...)/)?(?P<repository>...):(?P<tag>...)$`
  - [x] Digest-based reference pattern: `^(?P<registry>.../)?(?P<repo>...)(?:@(?P<digest>sha256:...))?$`
- [x] Implement Docker Library image normalization (e.g., `nginx:latest` -> `docker.io/library/nginx:latest`). See DEVELOPMENT.md 6.1.4.
- [x] Implement source registry filtering logic based on user input (`--source-registries`).
- [x] Implement registry domain sanitization for path generation (remove `.`, preserve `-`, remove port, e.g., `docker.io` -> `dockerio`, `quay.io` -> `quayio`).

### 2.3 Path Strategy

- [x] Implement `prefix-source-registry` strategy (default):
  - [x] Construct target path: `targetRegistry / sanitizedSourceRegistry / originalRepositoryPath` (e.g., `harbor.home.arpa/dockerio/busybox`)
  - [x] Apply registry domain transformation rules (DEVELOPMENT.md 8.1).
  - [x] Ensure correct handling of Docker Library images (e.g., `.../dockerio/library/nginx...`).
- [x] Design and add framework/interface for easily adding future path strategies (e.g., `flat`).

### 2.4 Output Generation

- [x] Create override structure generator:
  - [x] Build a new nested map mirroring the original value's path.
  - [x] Include *only* the minimal required keys to redirect the image according to the chosen path strategy.
- [x] Implement subchart path mapping using dependency aliases from parent `Chart.yaml` (e.g., `subchartAlias.image.repository`).
- [x] Add YAML serialization for the generated override structure using `sigs.k8s.io/yaml`.
- [x] Implement output logic: write to `stdout` by default, or to specified `--output-file`.

### 2.5 Debugging and Logging
- [x] Implement debug package for structured logging
- [x] Add debug logging to key functions:
  - [x] IsSourceRegistry
  - [x] GenerateOverrides
  - [x] LoadChart
  - [x] GetStrategy
  - [x] OverridesToYAML
- [x] Add --debug flag to CLI interface

### 2.6 Bug Fixes and Improvements
- [x] Fix YAML output issues:
  - [x] Remove incorrect trailing colons in string values
  - [x] Fix quoting of string values
  - [x] Remove empty string keys in arrays
- ~~Fix non-image value transformation:~~ (Superseded by Refactoring below)
  - ~~[ ] Prevent transformation of configuration values (e.g., RuntimeDefault, linux, TCP)~~ (Old approach)
  - ~~[ ] Add better detection of non-image string values~~ (Old approach: Expanding `isNonImageValue` blacklist)
- [x] Improve error handling:
  - [x] Add better error messages for Helm template failures
  - [x] Add validation for generated YAML before applying
  - [x] Add checks for common Helm template issues

### 2.7 Refactor Image Detection Logic
- [x] **Goal:** Move away from value-based blacklisting (`isNonImageValue`) towards context-aware positive identification.
- [x] **Prioritize Structural Context:**
    - [x] Reliably identify explicit image maps (containing `repository`, `tag`, `registry` keys) via `tryExtractImageFromMap`.
    - [x] Define and use patterns for known *image-containing* keys/paths (e.g., `image`, `*.image`, `spec.containers[*].image`) to guide `tryExtractImageFromString`.
    - [x] Define and use patterns for known *non-image* configuration keys/paths (e.g., `*.enabled`, `*.annotations.*`, `*.labels.*`, `*.port`, `*.timeout`, `*.serviceAccountName`) to *prevent* attempting image parsing on their values.
- [x] **Implement Stricter String Parsing:**
    - [x] For string values under *ambiguous* keys (not clearly image or non-image paths), apply a strict format check *before* passing to `reference.ParseDockerRef`.
    - [x] The check should verify the string structure resembles `[host/]<repo-path>[:tag|@digest]`, potentially requiring `/` and (`:` or `@`) unless it matches a known Docker Library pattern.
    - [x] This should inherently filter out simple config values like `true`, `60s`, `/metrics`, `-v`, `-5` without needing a specific blacklist.
- [x] **Deprecate/Remove `isNonImageValue`:**
    - [x] Remove the large blacklist logic from `isNonImageValue`.
    - [x] Potentially keep minimal checks (e.g., empty string) or remove the function entirely if context and stricter parsing cover all cases.
- [x] **Update Relevant Functions:** Refactor `detectImagesRecursive`, `tryExtractImageFromString`, and potentially related helpers to implement this contextual logic.

### 2.8 Registry Mapping Support (New)
- [x] Add registry mapping configuration support:
  - [x] Create registry mappings package
  - [x] Implement YAML configuration loading
  - [x] Update path strategy to use mappings
  - [x] Add CLI flag for mappings file
  - [x] Add documentation and examples
  - [x] Add tests for mapping functionality
  - [x] Add verbose output for default mapping behavior

## 3. CLI Interface

- [x] Set up command-line flag parsing (using `flag` package or a library like `cobra`):
  - [x] `--chart-path` (string, required)
  - [x] `--target-registry` (string, required)
  - [x] `--source-registries` (string slice/comma-separated, required)
  - [x] `--output-file` (string, optional, default: "")
  - [x] `--path-strategy` (string, optional, default: "prefix-source-registry")
  - [x] `--verbose` (bool, optional, default: false)
  - [x] `--dry-run` (bool, optional, default: false)
  - [x] `--strict` (bool, optional, default: false)
  - [x] `--exclude-registries` (string slice/comma-separated, optional)
  - [x] `--threshold` (int, optional, default: 100)
- [x] Implement input validation:
  - [x] Validate chart path existence and readability.
  - [x] Validate registry formats (basic check for invalid characters).
  - [x] Check for potential path traversal issues in file paths.
- [x] Implement error handling with specific exit codes (DEVELOPMENT.md 6.1.2):
  - [x] 0: Success
  - [x] 1: General runtime error
  - [x] 2: Input/Configuration Error
  - [x] 3: Chart Parsing Error
  - [x] 4: Image Processing Error
  - [x] 5: Unsupported Structure Error (`--strict` only)
  - [x] (Define code for threshold failure, e.g., 6)
- [x] Implement structured error messages (as per TESTING.md Section 7 format) for value-related issues.

## 4. Testing Implementation

### 4.1 Unit Tests

- [x] Test value traversal logic.
- [x] Test image detection heuristics for all supported and unsupported patterns.
- [x] Test image string parsing regex and extraction logic.
- [x] Test Docker Library normalization function.
- [x] Test registry domain sanitization function.
- [x] Test `prefix-source-registry` path generation logic.
- [x] Test override structure generation for various inputs (ensure minimal output).
- [x] Test subchart alias path construction.
- [x] Test YAML generation output format.

### 4.2 Integration Tests

- [x] **Core Use Case Test:**
    - [x] Add specific test using `kube-prometheus-stack` chart (or equivalent complex chart).
    - [x] Configure test to use `--source-registries docker.io,quay.io` and `--target-registry harbor.home.arpa`.
    - [x] **Validation:**
        - [x] Generate `override.yaml` using the tool.
        - [x] Run `helm template <chart> <original_values>` and capture image lines.
        - [x] Run `helm template <chart> <original_values> -f override.yaml` and capture image lines.
        - [x] Compare outputs to verify:
            - Images from `docker.io`, `quay.io` are redirected to `harbor.home.arpa` using the correct path strategy.
            - Tags/digests remain identical.
            - Images from other registries are unchanged.
            - Images excluded via `--exclude-registries` are unchanged.
        - [x] Verify the `helm template ... -f override.yaml` command completes successfully.

### 4.3 Bulk Chart Testing (New)

- [ ] **Diverse Chart Testing:**
    - [ ] Create simple test script that:
        - [ ] Takes a list of chart repositories
        - [ ] Downloads latest version of each chart
        - [ ] Runs our analyzer on each chart
        - [ ] Records simple success/failure metrics
    - [ ] Test against charts from diverse maintainers:
        - [ ] Bitnami charts (baseline)
        - [ ] Official Kubernetes charts
        - [ ] Cloud provider charts (AWS, Azure, GCP)
        - [ ] Community charts from Artifact Hub
    - [ ] Focus on fixing failures:
        - [ ] Identify common failure patterns
        - [ ] Update code to handle new patterns
        - [ ] Rerun tests to verify fixes
        - [ ] Track success rate improvement
    - [ ] Success metrics:
        - [ ] Number of charts processed
        - [ ] Number of successful overrides
        - [ ] Simple percentage success rate
        - [ ] Basic error categorization

### 4.4 Performance Testing

- [ ] Setup benchmark infrastructure (standardized environment).
- [ ] Create tests using charts of varying complexity.
- [ ] Measure execution time and peak memory usage for each complexity level.

## 5. Documentation

- [x] Create `README.md`: Overview, Installation, Quick Start, Basic Usage.
- [x] Add detailed CLI Reference section (Flags and Arguments).
- [x] Document Path Strategies Explained (include sanitization rules).
- [x] Add Examples / Tutorials section.
- [ ] Create Troubleshooting / Error Codes guide.
- [ ] Add Contributor Guide (basic setup, testing).

## 6. Release Process

- [ ] Set up Git tagging for versioning (e.g., SemVer).
- [ ] Create release builds for target platforms (Linux AMD64, macOS AMD64/ARM64).
- [ ] Publish binaries (e.g., GitHub Releases).
- [ ] Publish documentation (e.g., alongside code or separate site).
- [ ] Setup automated release pipeline using GitHub Actions (triggered by tags).

## 7. Stretch Goals (Post-MVP)

- [ ] Implement `flat` path strategy.
- [ ] Implement multi-strategy support (different strategy per source registry).
- [ ] Add configuration file support (`--config`) for defining source/target/exclusions/custom patterns.
- [ ] Enhance image identification heuristics (e.g., custom key patterns via config).
- [ ] Improve handling of digest-based references (more robust parsing).
- [ ] Add comprehensive private registry exclusion patterns (potentially beyond just source registry name).
- [ ] Implement validation of generated target URLs (basic format check).
- [ ] Explore support for additional target registries (Quay, ECR, GCR, ACR, GHCR) considering their specific path/naming constraints.

## 8. Post-Refactor Cleanup & Fixes

### 8.1 Solidify Normalization & Sanitization (Highest Priority)
- [x] Define clear roles for `NormalizeRegistry` (canonical name) and `SanitizeRegistryForPath` (path component).
- [x] Refine `SanitizeRegistryForPath` (`pkg/image/image.go`) to consistently remove ports and dots, and handle `docker.io` normalization.
- [x] Update `TestSanitizeRegistryForPath` (`pkg/image/parser_test.go`) to match refined behavior (removing dots, not using underscores).
- [x] Update `TestPrefixSourceRegistryStrategy` (`pkg/strategy/path_strategy_test.go`) to expect paths using sanitized names (e.g., `dockerio/`, `localhost/`).
- [x] Update `TestGenerateOverrideStructure` (`pkg/override/override_test.go`) to align with sanitized name expectations in generated paths.

### 8.2 Re-Investigate Override Structure & Schema
- [ ] After fixing normalization, re-run integration tests (`TestCertManagerOverrides`, `TestCertManagerIntegration`). (Ongoing - Led to Section 13)
- [ ] Debug `generateOverrideStructure` (`pkg/override/override.go`) again. (Ongoing - Led to Section 13)
- [ ] Verify the root-level structure and placement of the `image: {...}` block in the generated override map. (Identified as problematic)

### 8.3 Fix Image Parsing
- [ ] Debug `ParseImageReference` (`pkg/image/parser.go`) to ensure errors are correctly returned for invalid image reference inputs.
- [ ] Fix the failing test case `TestParseImageReference/invalid_image_reference` (`pkg/image/parser_test.go`).

### 8.4 Clean Up Integration Test Environment
- [ ] Fix chart loading issue in `TestKubePrometheusStack` (ensure chart path is correct or chart is present).
- [ ] Resolve `executable file not found in $PATH` error in `TestDryRunFlag`.
- [ ] Fix argument/configuration error in `TestStrictMode`.

## 9. Post-Refactor Override Generation Debugging & Fix

### 9.1 Investigation Summary
- [x] Identified root cause of YAML generation issues:
  - [x] Confirmed issue was not invalid YAML syntax but incorrect override structure
  - [x] Verified that explicit registry/repository/tag structure is required
  - [x] Documented behavior differences between charts (nginx vs cert-manager)

### 9.2 Required Fix
- [x] **Modify Override Structure Generation:** (Initial flawed attempt completed)
  - [x] Updated `GenerateOverrideStructure` in `pkg/override/override.go` to generate complete image map structure
  - [x] Implemented explicit setting of registry, repository, and tag/digest fields
  - [x] Added proper nesting based on path detection
  - [x] Removed conditional logic that caused inconsistent structure generation
- [ ] **Testing and Validation:** (Failed, leading to Section 13)
  - [ ] Re-run integration tests to verify fixes
  - [ ] Add new test cases for various path structures
  - [ ] Document any remaining chart-specific validation issues

## 10. Systematic Helm Chart Analysis & Refinement

**Goal:** Proactively identify common patterns, edge cases, and potential issues across a diverse range of Helm charts to inform robust design and targeted refactoring.

### 10.1 Test Infrastructure Enhancement
- [ ] **Structured Result Collection:**
  - [ ] Define a stable JSON schema for individual test run results (e.g., `test-result-v1.schema.json`).
  - [ ] Include mandatory fields: `chartName`, `chartVersion`, `chartSource`, `testScenario` (e.g., `override_generation`, `helm_template_validation`), `timestamp`, `executionTime`, `peakMemory`, `outcome` (pass/fail), `errors` (list of structured errors), `generatedOverridePath`, `validationDetails` (diffs, schema errors).
  - [ ] Implement test harness logic to output results as JSON files (e.g., `results/<chart-name>/<version>/<scenario>.json`).

### 10.2 Chart Corpus Expansion & Management
- [x] **Diverse Chart Selection:** (List expanded in test script)
  - [ ] Target Top N (e.g., 30-50) charts from ArtifactHub based on recent download/star counts.
  - [ ] Explicitly include known complex charts (e.g., Istio, Prometheus Operator, GitLab, Airflow).
  - [ ] Ensure representation from major Helm chart providers (Bitnami, VMware Tanzu, community sources).
  - [ ] Include charts exercising different features (complex subcharts, CRDs with image refs, diverse `values.yaml` structures).
- [ ] **Corpus Maintenance:**
  - [ ] Document selection criteria, source URLs, and rationale for each chart included.
  - [ ] Implement a process/script to periodically check for new versions of corpus charts and update the test matrix.
  - [ ] Store test charts locally or fetch on demand during CI runs.

### 10.3 Automated Pattern Detection & Analysis
- [ ] **Value Structure Patterns:**
  - [ ] Implement detectors (regex, potentially Go AST parsing for Go templates within values) for:
    - Explicit image maps (`image.repository`, `*.image.tag`).
    - Simple image strings (`image: registry/repo:tag`).
    - Global registry variables (`global.imageRegistry`).
    - Image references within lists/arrays.
    - Common non-image key patterns (`*.enabled`, `*.port`, `*.serviceAccountName`).
- [ ] **Frequency & Correlation Analysis:**
  - [ ] Develop tools to count pattern occurrences across the corpus.
  - [ ] Identify correlations (e.g., charts using global registries often use simple image strings).
  - [ ] Generate reports highlighting common vs. rare patterns.

### 10.4 Schema Structure Analysis
- [ ] **Schema Extraction & Comparison:**
  - [ ] Build tools to automatically extract `values.schema.json` if present.
  - [ ] Compare schema structures, focusing on definitions related to `image`, `registry`, `repository`, `tag`, `pullPolicy`.
  - [ ] Identify common schema validation rules (e.g., required fields, `additionalProperties: false`, type constraints).
- [ ] **Provider Variations:**
  - [ ] Document schema variations across different chart providers (e.g., Bitnami common structure vs. custom).
  - [ ] Create a schema compatibility matrix (Chart Provider vs. Schema Features).

### 10.5 Data-Driven Refactoring Framework
- [ ] **Refactoring Metrics:**
  - [ ] Quantify pattern coverage (e.g., % of corpus image patterns handled by current logic).
  - [ ] Define complexity score (e.g., cyclomatic complexity, lines of code per pattern handled).
  - [ ] Establish backward compatibility impact assessment (e.g., number of tests broken by a change).
- [ ] **Decision Prioritization:**
  - [ ] Use a weighted decision matrix template (spreadsheet/tool) combining metrics.
  - [ ] Prioritize refactoring based on highest impact (covering common patterns) and lowest risk/complexity.
  - [ ] Estimate effort (developer days) for proposed refactoring tasks.

### 10.6 Container Array Pattern Support
- [ ] **Container Array Handling:**
  - [ ] Add explicit support for container arrays in pod templates:
    - [ ] Regular containers (`spec.containers[]`)
    - [ ] Init containers (`spec.initContainers[]`)
    - [ ] Sidecar containers (part of regular containers array)
  - [ ] Document common patterns found in popular charts:
    - [ ] Redis pattern (init containers for volume permissions)
    - [ ] Prometheus pattern (sidecar exporters)
    - [ ] Istio pattern (sidecar proxies)
  - [ ] Add test cases specifically for array-based container definitions

### 10.7 Image Reference Focus
- [x] **Scope Clarification:**
  - [x] Focus solely on registry location changes
  - [x] Preserve all other image-related configurations:
    - [x] Image pull policies
    - [x] Image pull secrets
    - [x] Container security contexts
    - [x] Resource limits/requests
  - [x] Document this focused scope in user documentation

## 11. Analyzer Refinement & Expanded Testing (New)

**Goal:** Improve analyzer robustness based on initial diverse chart testing and gather more data to inform potential future refactoring of the rewrite logic.

### 11.1 Analyzer Updates
- [x] **Handle Missing Registry:** Update `analyzeValues` (and `analyzeArray` if needed) in `pkg/analysis/analyzer.go` to default the `registry` to `"docker.io"` if it is missing, empty, or nil in an identified image map structure.
- [x] **Handle Empty/Non-String Tags:** Ensure the analyzer gracefully handles empty or non-string `tag` values (e.g., store as empty string `""`) without causing errors or warnings.

### 11.2 Test Script Enhancement
- [x] **Expand Chart List:** Add more charts to the `repos` array in `test/tools/test-charts.sh`, aiming for broader coverage of maintainers and chart types (e.g., operators, databases, web apps, networking components).
- [ ] **Refine Error Reporting (Optional):** Consider adding more detail to the `FAILURE` message in `test/results.txt`, perhaps capturing the specific error output from the analyzer for failed charts.

### 11.3 Testing & Iteration Cycle
- [x] **Run Expanded Tests:** Execute the updated `test-charts.sh` script.
- [x] **Analyze Failures:** Examine the `results.txt` and the detailed analysis output (`test/charts/*-analysis.txt`) for any charts that still fail.
- [x] **Prioritize Fixes:** Focus on fixing analyzer errors that prevent successful processing. Failures indicate patterns our code doesn't handle.
- [x] **Iterate:** Repeat the cycle of fixing the analyzer and re-running tests until a high success rate is achieved across the diverse chart set. (Analyzer part seems stable now).

## 12. Override Generation Testing & Refinement (New)

**Goal:** Validate the override generation logic across the diverse chart corpus and refine the tool based on encountered errors or inconsistencies.

### 12.1 Test Script Adaptation
- [x] Modify `test/tools/test-charts.sh` to execute the `helm-image-override override` command (or equivalent Go function call) instead of `analyze`.
- [x] Configure the script to:
    - [x] Use appropriate flags (`--target-registry harbor.home.arpa`, `--source-registries docker.io,quay.io,gcr.io,ghcr.io`).
    - [x] Optionally save generated overrides to a directory (e.g., `test/overrides/`) for inspection using `--output-file`.
    - [x] Capture success/failure status for each chart in `test/results.txt`, noting override-specific errors.

### 12.2 Override Generation Execution
- [x] Run the adapted test script against the current list of successfully analyzed charts.

### 12.3 Results Analysis & Error Investigation
- [x] Analyze `test/results.txt` for any charts failing the override generation step.
- [x] For failed charts, investigate the error messages and potentially the generated override file (if saved).
- [x] Identify patterns in failures (e.g., specific value structures causing issues in `pkg/override/override.go`, complex subchart interactions, unsupported patterns not caught by analysis).

### 12.4 Override Logic Refinement
- [x] Based on the analysis, prioritize and fix issues in the override generation logic (`pkg/override/override.go`, path strategies `pkg/strategy/path_strategy.go`, etc.). (Attempted, led to Section 13)
- [ ] Ensure generated overrides are syntactically correct YAML.
- [ ] Verify that overrides correctly handle subchart aliases and paths.
- [ ] Ensure the minimal necessary structure is generated for overrides.

### 12.5 Iteration Cycle
- [x] Repeat the cycle of running tests, analyzing failures, and fixing the override logic until a high success rate is achieved. (Cycle completed, but failed, prompting Section 13)

## 13. Refactor Override Generation (Path-Based Modification)

### 13.1 Enhance Value Processing ✅
- [x] **Refine `processValues`:**
  - [x] Successfully captured precise paths for identified image maps and string values
  - [x] **Path Representation:** Implemented path storage as `[]string`
  - [x] Integrated path information with `ImageReference` data structure
  - [x] Added support for array indices in paths
  - [x] Implemented robust path tracking through nested structures

### 13.2 Base Structure Preparation ✅
- [x] **Clean Original Values:**
  - [x] Implemented deep copy function for `map[string]interface{}`
  - [x] Successfully integrated `cleanupTemplateVariables` with deep copied values
  - [x] Added validation to ensure original values remain unmodified
  - [x] Implemented proper handling of template variables

### 13.3 Implement Path-Based Modification ✅
- [x] **Develop Path Setter:**
  - [x] Implemented robust `setValueAtPath` function
  - [x] Successfully handled:
    - [x] Nested map creation
    - [x] Array index paths
    - [x] Type validation
    - [x] Error handling for invalid paths
  - [x] Added comprehensive test coverage
- [x] **Modify Base Structure:**
  - [x] Successfully implemented iteration through identified images
  - [x] Integrated path strategy for target reference generation
  - [x] Properly handled both string and map-based image references
  - [x] Added validation for modified structures

### 13.4 Generate Final Override Output ✅
- [x] **Output Strategy:**
  - [x] Implemented extraction of modified sub-trees
  - [x] Successfully preserved necessary parent keys
  - [x] Implemented proper YAML serialization
  - [x] Added validation for generated YAML
  - [x] Successfully handled complex nested structures

### 13.5 Testing & Validation ✅
- [x] **Comprehensive Testing:**
  - [x] Added and validated test cases for:
    - [x] Deep copy functionality
    - [x] Path-based modification
    - [x] Error handling
    - [x] Structure preservation
  - [x] Successfully validated with complex charts:
    - [x] cert-manager (268 lines generated)
    - [x] kube-prometheus-stack (2215 lines generated)
    - [x] ingress-nginx (411 lines generated)
    - [x] argo-cd (1516 lines generated)
    - [x] jaeger (1998 lines generated)

### 13.6 Results Summary ✅
- **Analysis Success Rate:** 98% (64/65 charts)
  - Single failure: bitnami/consul (timeout issue)
- **Override Generation:** Successfully generated 39 override files
  - Largest: kube-prometheus-stack (49KB)
  - Most complex: jaeger (1998 lines)
  - Average size: ~10KB
- **Validation:** All generated overrides are valid YAML and maintain proper structure

## 14. Image Detection and Override Generation Improvements

*(Refines image handling based on insights from Section 10-13 analysis)*

### 14.1 Image Detection Refinement (High Priority)
*Goal: Achieve consistent and accurate image identification across all code paths and handle common chart patterns.*

- [ ] **Fix inconsistency between analysis (`analyze`) and override (`override`) phases:**
    - [ ] **Consolidate Logic:** Migrate core image detection functions (`isImageMap`, `tryExtractImageFromString`, `tryExtractImageFromMap`, related helpers) into a shared location, likely `pkg/image/detection.go`. Update both `pkg/analysis` and `pkg/override` (or the shared value traversal logic used by `override`) to call these shared functions.
    - [ ] **Unified Test Suite:** Create shared test cases in `pkg/image/detection_test.go` that cover all supported patterns (maps, strings, partials, globals). Add specific integration tests (`TestAnalysisVsOverrideConsistency`) that run *both* `analyze` and `override` on the same complex chart (e.g., `cert-manager`, `kube-prometheus-stack`) and assert that the *set* of identified image references (original paths and parsed components) is identical.
    - [ ] **Refine `processValues` Interface:** Ensure the recursive value processing function (currently in `pkg/override/override.go`, potentially move to `pkg/values/traversal.go`?) accepts the shared detection functions/configuration as parameters to guarantee consistency if called from different commands.
    - [ ] **Logging:** Add specific `debug` logs within the shared detection functions detailing *why* a value was identified as an image (e.g., "Map matched structure", "String matched pattern under key 'image'"), and why it might be skipped (e.g., "Value is boolean", "Path matched non-image pattern '*.enabled'").

- [ ] **Improve image map detection:**
    - [ ] **Partial Maps:** Modify `tryExtractImageFromMap` (in `pkg/image/detection.go`) to handle cases where `registry` or `tag` might be missing. If `registry` is missing, default to `docker.io` (consider Docker Library normalization). If `tag` is missing, store it as an empty string or a specific marker. Document this behavior clearly.
    - [ ] **Global Registry Handling:** Enhance the value traversal logic (`processValues`) to accept and track a `context` map. When encountering a `global.imageRegistry` (or similar common patterns), store it in the context. When `tryExtractImageFromMap` processes a partial map missing a `registry`, it should check the context for a global value.
    - [ ] **Support Variations:** Ensure the detection logic handles the examples provided (standard, partial, global, string) robustly. Add specific unit tests for each variation listed in the `TODO.md`.

- [ ] **Enhance template variable handling (Medium Priority):**
    - [ ] **Detection (Heuristic):** In `tryExtractImageFromString` and potentially `tryExtractImageFromMap` (for templated tag/registry values), detect strings containing `{{ ... }}`. Do *not* attempt to parse these as standard image references.
    - [ ] **Preservation:** When a templated string is detected in an image field (e.g., `tag: {{ .Chart.AppVersion }}`), treat the entire string as opaque. The `ImageReference` struct should store the original templated string. The override logic (`setValueAtPath`) must ensure this original templated string is preserved in the generated override YAML, not replaced by a potentially incorrect interpretation.
    - [ ] **Validation (Basic):** Add warnings if a template variable is detected in a part of the image reference that *must* be static for redirection (e.g., the repository name itself, if the strategy relies on it). Log clearly that the template logic within Helm will resolve the final value.
    - [ ] **Test Cases:** Add tests using chart snippets with common template patterns (`.Chart.AppVersion`, `.Values.global.version`, `default "..." ...`).

### 14.2 Value Processing Improvements (High Priority)
*Goal: Prevent misidentification of non-image values and ensure robust path handling.*

- [ ] **Fix boolean and numeric value handling:**
    - [ ] **Type Checking:** Within the recursive `processValues` function, add explicit type checks (`switch v := v.(type)`) *before* attempting image detection logic, especially for ambiguous keys. If a value is clearly `bool`, `float64`, `int`, etc., log it at debug level and skip image detection attempts *unless* the key/path is explicitly known to sometimes contain images represented numerically (highly unlikely and should be avoided).
    - [ ] **Contextual Skip:** Leverage the non-image path patterns defined in **Section 2.7** (e.g., `*.enabled`, `*.port`). If the current path matches a known non-image pattern, skip image detection entirely, regardless of the value type.
    - [ ] **Preservation:** Ensure the `setValueAtPath` function correctly handles setting boolean and numeric types when reconstructing parts of the structure for overrides (although this function primarily sets image strings/maps).

- [ ] **Enhance path resolution (Medium Priority):**
    - [ ] **Array Indexing:** Solidify the chosen convention for representing array indices in paths (e.g., `list[2]`) within `processValues` and ensure `setValueAtPath` parses and handles it correctly, including creating/expanding slices as needed. Add specific tests for paths involving arrays.
    - **Map Key Handling:** Ensure `setValueAtPath` correctly handles creating nested maps. Test edge cases like attempting to set a map key on a path element that already exists but is *not* a map (should return an error).

### 14.3 Override Generation Enhancement
*Goal: Improve the quality and accuracy of generated overrides, supporting more complex chart structures.*

- [ ] **Improve structure preservation (Low Priority - Best Effort):**
    - [ ] **Acknowledge Limitations:** Recognize that standard Go YAML libraries (`sigs.k8s.io/yaml`) *lose* comments and fine-grained formatting during parsing. Preserving these perfectly is likely infeasible without switching to a much more complex parser/emitter.
    - [ ] **Focus on Essentials:** Ensure the *structural* nesting (keys, indentation levels) is correctly mirrored in the output generated by the path extraction/merge logic (from 13.4).
    - [ ] **Handle Multi-line Strings:** Verify that the YAML emitter used correctly handles multi-line strings (e.g., certificates, scripts embedded in values) if they exist as siblings to overridden image values.

- [ ] **Add array-based image support (Medium Priority):**
    - [ ] **Detection:** Modify `processValues` to iterate through slices/arrays. When processing array elements, if the element is a map, recursively call `processValues` on it. If the element is a string, potentially apply `tryExtractImageFromString` if the array's key suggests it might contain images (e.g., `containerImages: ["img1:tag", "img2:tag"]`). Define clear heuristics for which arrays to process. Start with common patterns like `spec.containers`, `spec.initContainers`.
    - [ ] **Path Handling:** Ensure paths correctly include array indices (e.g., `deployment.spec.containers[0].image`).
    - [ ] **Override Generation:** Ensure `setValueAtPath` correctly modifies values within arrays using the indexed path.
    - [ ] **Test Cases:** Add test fixtures specifically for charts using `containers`, `initContainers`, and other list-based image definitions.

- [ ] **Implement context-aware override generation (Low Priority):**
    - [ ] **Global Context:** As per 14.1, pass down global values (like `global.imageRegistry`) during value traversal. Use this context in `tryExtractImageFromMap` when resolving partial image maps.
    - [ ] **Subchart Aliases:** Section 13's path-based modification already handles subchart aliases correctly by using the full path provided by the initial traversal. Ensure tests cover scenarios with aliases.


### 14.4 Testing Infrastructure (Medium Priority)
*Goal: Ensure robustness and prevent regressions.*

- [ ] **Add comprehensive test suite:**
    - [ ] **Pattern Tests:** Create specific unit tests in `pkg/image/detection_test.go` and `pkg/override/override_test.go` (or `pkg/values/traversal_test.go`) for *each* supported image pattern variation (standard map, partial map, global registry, string format, templated strings, images within arrays).
    - [ ] **Integration Tests:** Add new integration tests using real charts (or curated snippets) demonstrating the fixes for boolean/numeric handling, array support, and template variable preservation.
    - [ ] **Regression Tests:** For any significant bug fixed during this phase, add a specific test case that would have failed before the fix.

- [ ] **Create test fixtures:**
    - [ ] Develop small, focused test charts (`testdata/charts/`) demonstrating specific structures:
        - `partial-maps/`
        - `global-registry/`
        - `template-vars/`
        - `array-images/`
        - `mixed-types/` (booleans/numbers near images)

- [ ] **Implement validation tools:**
    - [ ] **Override Validator:** Enhance integration tests to not only check `helm template` success but also to parse the generated `override.yaml` and perform basic structural checks or compare against an expected minimal override structure for simple cases.
    - [ ] **Consistency Checker:** Implement the `TestAnalysisVsOverrideConsistency` integration test mentioned in 14.1.


### 14.5 Documentation Updates (Medium Priority)
*Goal: Keep documentation aligned with features.*

- [ ] **Add detailed documentation:**
    - [ ] Update `README.md` or create `docs/image_patterns.md` detailing *exactly* which image value structures are supported, including partial maps, global registry interactions, string formats, and array handling.
    - [ ] Create `docs/template_handling.md` explaining how template variables are detected and preserved (treated as opaque strings).
    - [ ] Update CLI reference for any new flags or modified behavior.

- [ ] **Create troubleshooting guide:**
    - [ ] Add entries for common errors related to new features (e.g., "Warning: Template variable detected in image repository field", "Error: Unsupported value type found at path X").


### 14.6 Implementation Priority *(No Changes Needed)*
1. High Priority (Critical Fixes):
   - Fix inconsistency between analysis and override phases
   - Fix boolean and numeric value handling
   - Improve basic image map detection

2. Medium Priority (Enhancement):
   - Implement template variable handling
   - Add array-based image support
   - Enhance path resolution (focus on array/edge cases)
   - Add focused testing infrastructure

3. Low Priority (Nice to Have):
   - Add structure preservation (comments/formatting)
   - Implement context-aware override generation (beyond globals/aliases)
   - Expand test suite significantly beyond core fixes

## 15. Chart Testing Improvements

*(Based on analysis of test run results from Python test script)*

### 15.1 Command Syntax Correction (Highest Priority) ✓
*Goal: Fix the 0% override success rate by correcting helm-image-override invocation.*

- [x] **Update `test_chart_override` function in `test/tools/test-charts.py`:**
  - [x] Replace current subprocess invocation with proper command syntax:
    ```python
    result = subprocess.run(
        [
            str(helm_override_binary),
            "override",  # Add the required 'override' subcommand
            "--chart-path", str(chart_path),  # Change from '--chart' to '--chart-path'
            "--target-registry", target_registry,
            "--source-registries", "docker.io,quay.io,gcr.io,ghcr.io,k8s.gcr.io,registry.k8s.io",  # Add comprehensive registry list
            "--output-file", str(TEST_OUTPUT_DIR / f"{chart_name}-values.yaml")
        ],
        # ...existing code...
    )
    ```
  - [x] Remove the `--values` flag from the command (not used by the `override` subcommand)
  - [x] Add test validation to verify successful command execution

### 15.2 Default Values Enhancement (High Priority) ✓
*Goal: Fix the 63 Bitnami-specific errors by enabling non-standard image repositories.*

- [x] **Update `DEFAULT_VALUES_CONTENT` in `test/tools/test-charts.py`:**
  - [x] Add explicit security allowance for Bitnami charts:
    ```yaml
    global:
      imageRegistry: harbor.home.arpa/docker
      imagePullSecrets: []
      storageClass: ""
      security:
        allowInsecureImages: true  # Required for Bitnami charts
    ```

### 15.3 Error Categorization Improvement (Medium Priority) ✓
*Goal: Better identify and differentiate error types for more accurate statistics.*

- [x] **Enhance `categorize_error` function in `test/tools/test-charts.py`:**
  - [x] Add new error category for command syntax issues
  - [x] Update categorization logic to detect command errors
  - [x] Refine rate limit detection to catch all variants of rate limit errors
  - [x] Add more specific checks for Bitnami chart errors

### 15.4 Rate Limit and Performance Improvements (Medium Priority) ✓
*Goal: Reduce rate limit errors through better caching and processing control.*

- [x] **Implement Chart Caching System:**
  - [x] Add `CHART_CACHE_DIR` for persistent storage of downloaded charts
  - [x] Implement `get_cached_chart` function to check cache before downloading
  - [x] Add `--no-cache` flag to optionally disable caching
  - [x] Preserve downloaded charts between runs

- [x] **Rate Limit Mitigation:**
  - [x] Lower parallel processing limits to conservative values
  - [x] Add QPS and burst limits to Helm commands
  - [x] Implement incremental backoff for retries
  - [x] Add delays between repository operations

### 15.5 Targeted Testing (Low Priority) ✓
*Goal: Allow focused testing of specific charts or subsets of charts for efficient debugging.*

- [x] **Add filtering options to `test/tools/test-charts.py`:**
  - [x] Implement `--chart-filter` for pattern matching
  - [x] Add `--max-charts` for limiting test scope
  - [x] Add `--skip-charts` for excluding specific charts
  - [x] Update documentation with new options

### 15.6 Results Analysis Enhancement (Low Priority)
*Goal: Provide more detailed insights into test results for ongoing improvement.*

- [ ] **Improve summary generation in `test/tools/test-charts.py`:**
  - [ ] Add timing information (total and per-chart average)
  - [ ] Generate a list of most common error patterns
  - [ ] Create a simple chart category analysis (e.g., by repository, by error type)
  - [ ] Save raw test data to JSON for external analysis
  - [ ] Add a flag to generate an HTML report with visualizations

### 15.7 New Improvements
*Goal: Further enhance the testing process based on recent findings.*

- [ ] **Repository-Specific Optimizations:**
  - [ ] Add repository-specific rate limit configurations
  - [ ] Implement smart retry logic based on repository response codes
  - [ ] Add support for authenticated registry access

- [ ] **Cache Management:**
  - [ ] Add cache cleanup/maintenance functionality
  - [ ] Implement cache versioning for chart updates
  - [ ] Add cache statistics to summary report

- [ ] **Documentation:**
  - [ ] Create a dedicated document for the test script (`docs/chart_testing.md`)
  - [ ] Add troubleshooting guide for common issues
  - [ ] Document caching behavior and configuration

### 15.8 Implementation Status
1. **Completed:**
   - Command syntax correction
   - Default values enhancement
   - Error categorization improvement
   - Rate limit mitigation
   - Chart caching system
   - Targeted testing capabilities

2. **In Progress:**
   - Results analysis enhancement
   - Repository-specific optimizations

3. **Planned:**
   - Cache management improvements
   - Documentation updates

## 16. Hybrid Chart Classification for Test Configuration (Python Script)

**Goal:** Reduce template errors in `test/tools/test-charts.py` by applying tailored default `values.yaml` content based on chart characteristics, improving the override success rate.

### 16.1 Strategy Overview
Implement a tiered classification system within the Python test script (`test/tools/test-charts.py`) to select the most appropriate base configuration for the `helm template` validation step. The order of precedence prioritizes more specific indicators:

1.  **Dependency Analysis:** Check `Chart.yaml` for dependencies on known common libraries (e.g., `bitnami/common`). Apply a template optimized for that ecosystem.
2.  **`values.yaml` Structural Analysis:** If no common library is detected, analyze the chart's `values.yaml` for common image definition patterns (e.g., presence of `global.imageRegistry`, `image.repository` map, `image` as string). Select a corresponding standard template.
3.  **Default Fallback:** If neither analysis yields a clear classification, use a general-purpose default template and log a warning.

### 16.2 Implementation Steps (in `test/tools/test-charts.py`)

- [ ] **Define Configuration Templates:**
    - [ ] Create distinct string constants or load separate `.yaml` files representing different base configurations:
        - `VALUES_TEMPLATE_BITNAMI`: Optimized for Bitnami charts (includes `global.imageRegistry`, `commonLabels`, `allowInsecureImages`).
        - `VALUES_TEMPLATE_STANDARD_MAP`: Assumes `image.repository`, `image.tag` map structure.
        - `VALUES_TEMPLATE_STANDARD_STRING`: Assumes `image: registry/repo:tag` string structure (might need less common).
        - `VALUES_TEMPLATE_DEFAULT`: A general-purpose fallback (similar to the last improved version in Section 15).
    - [ ] Ensure templates use consistent placeholder values (e.g., `TARGET_REGISTRY_PLACEHOLDER`) that can be replaced dynamically.

- [ ] **Implement `get_chart_classification(chart_path: Path) -> str` function:**
    - [ ] **Dependency Check:**
        - [ ] Parse `Chart.yaml` located at `chart_path / "Chart.yaml"`.
        - [ ] Check the `dependencies` list for entries with `name: common` and `repository` containing `bitnami`.
        - [ ] If found, return `"BITNAMI"`.
    - [ ] **`values.yaml` Analysis:**
        - [ ] Parse `values.yaml` located at `chart_path / "values.yaml"`. Handle potential parsing errors gracefully.
        - [ ] Check for `global.imageRegistry`: If present (and maybe a string), return `"GLOBAL_REGISTRY"`. (This might overlap with Bitnami, refine logic).
        - [ ] Check for `image` key:
            - If `image` is a map containing `repository` (and maybe `tag`), return `"STANDARD_MAP"`.
            - If `image` is a string, return `"STANDARD_STRING"`.
        - [ ] Add checks for other common patterns as identified through error analysis (e.g., presence of `commonLabels`, `commonAnnotations`).
    - [ ] **Fallback:** If no classification is determined, return `"DEFAULT"`.

- [ ] **Implement `get_values_content(classification: str, target_registry: str) -> str` function:**
    - [ ] Takes the classification string and the target registry URL.
    - [ ] Returns the appropriate template string (from step 1) with placeholders replaced.
    - [ ] Example:
      ```python
      if classification == "BITNAMI":
          template = VALUES_TEMPLATE_BITNAMI
      elif classification == "STANDARD_MAP":
          template = VALUES_TEMPLATE_STANDARD_MAP
      # ... other classifications
      else: # DEFAULT
          template = VALUES_TEMPLATE_DEFAULT
      # Replace placeholder - Ensure placeholder is unique and defined in templates
      # return template.replace("TARGET_REGISTRY_PLACEHOLDER", target_registry)
      # Consider using a more robust templating method if needed
      return template # Placeholder replacement needs implementation
      ```

- [ ] **Modify `test_chart_override` function:**
    - [ ] Before running `helm template` validation (inside the `try` block after generating the override):
        - [ ] Call `classification = get_chart_classification(chart_path)`
        - [ ] Call `values_content = get_values_content(classification, target_registry)`
        - [ ] Create a temporary directory or use `tempfile` to write `values_content` to a temporary values file (e.g., `temp_class_values.yaml`). Ensure proper cleanup.
        - [ ] Update the `helm template` command to use *both* the generated `override.yaml` (`-f output_file`) and the temporary classification-based values file (`-f temp_class_values.yaml`). Ensure the override file (`output_file`) comes *last* so its values take precedence for the actual image fields.
          ```python
          # Example helm template command update
          temp_values_path = # path to temp_class_values.yaml
          template_result = subprocess.run(
              [
                  "helm", "template", str(chart_path),
                  "-f", str(temp_values_path), # Base values based on classification
                  "-f", str(output_file)      # Our generated image overrides
              ],
              # ... rest of subprocess call ...
          )
          ```
    - [ ] Add logging to indicate which classification was used for each chart (e.g., `print(f"Chart {chart}: Classified as {classification}, using corresponding template.")`).

- [ ] **Refine Templates and Logic:**
    - [ ] Analyze the remaining `TEMPLATE_ERROR` charts after initial implementation.
    - [ ] Refine the classification logic (`get_chart_classification`) and the content of the `VALUES_TEMPLATE_*` constants based on error patterns.
    - [ ] Consider adding more specific classifications or templates if needed (e.g., differentiate `STANDARD_MAP` based on whether `registry` key is present alongside `repository`).

### 16.3 Testing and Validation
- [ ] Run the updated `test-charts.py` script.
- [ ] Monitor the `TEMPLATE_ERROR` count in the summary.

## 17. Comprehensive Test Case Improvements

**Goal:** Strengthen our testing coverage with focused unit tests targeting key functionality areas, edge cases, and potential vulnerabilities.

### 17.1 Image Detection Test Expansion (`pkg/image/detection_test.go`)

- [ ] **`TestDetectImages_ComplexStructures`:**
  - [ ] Deep nested maps and arrays with images at various nesting levels
  - [ ] Maps with keys resembling image parts (e.g., `config: { repository: "abc", tag: "v1" }`) but not actual images
  - [ ] Lists containing mixed valid and invalid image references
  - [ ] Expected: Only correct images identified with precise paths

- [ ] **`TestDetectImages_ContextVariations`:**
  - [ ] Run standard tests with `Strict: true` to catch ambiguous strings
  - [ ] Test with `TemplateMode: false` to verify template variable handling behavior
  - [ ] Test global registry precedence (global vs. map-specific vs. default `docker.io`)
  - [ ] Expected: Context settings appropriately influence detection behavior

- [ ] **`TestTryExtractImageFromString_EdgeCases`:**
  - [ ] Invalid image formats (e.g., invalid tags, invalid digests, invalid characters)
  - [ ] Various registry formats (with/without port, localhost, private domains)
  - [ ] Docker Library image references (e.g., `nginx` without registry/repository)
  - [ ] Expected: Correct parsing or appropriate nil/error responses

- [ ] **`TestTryExtractImageFromMap_PartialMaps`:**
  - [ ] Maps with missing `tag` field
  - [ ] Maps with missing `registry` field (should use global or default)
  - [ ] Maps with template variables in fields
  - [ ] Expected: Proper handling with defaults applied correctly

- [ ] **`TestIsImagePath_RegexAccuracy`:**
  - [ ] Known image paths from real charts
  - [ ] Known non-image paths
  - [ ] Edge cases and ambiguous paths
  - [ ] Expected: Correct classification of paths

- [ ] **`TestNormalizeImageReference_DockerLibrary`:**
  - [ ] Docker Library normalization for registry/repository
  - [ ] Non-Docker registries (should remain unchanged)
  - [ ] Multi-component repository paths
  - [ ] Expected: Consistent normalization behavior

### 17.2 Override Generation Test Enhancement (`pkg/override/override_test.go`)

- [ ] **`TestSetValueAtPath_ComplexPaths`:**
  - [ ] Nested map creation for non-existent paths
  - [ ] Type conflict handling (e.g., trying to set map field on a string)
  - [ ] Special characters in map keys
  - [ ] Expected: Correct value setting or appropriate errors

- [ ] **`TestCleanupMap_NestedStructures`:**
  - [ ] Deep nested structures where only deeper values are needed
  - [ ] Pruning of empty maps and arrays
  - [ ] Handling of non-map values
  - [ ] Expected: Minimal output structure with only necessary paths

- [ ] **`TestGenerateOverrideStructure_Minimalism`:**
  - [ ] Charts with only a subset of images requiring overrides
  - [ ] Mix of map and string image specifications
  - [ ] Complex nested structures
  - [ ] Expected: Minimal override YAML containing only necessary structure

- [ ] **`TestDeepCopy_Nested`:**
  - [ ] Complex nested structure with maps, arrays, and primitive values
  - [ ] Modification of copy should not affect original
  - [ ] Expected: Identical copy that's safely modifiable

### 17.3 Path Strategy Tests (`pkg/strategy/path_strategy_test.go`)

- [ ] **`TestPrefixSourceRegistryStrategy_RegistryVariations`:**
  - [ ] Various source registry formats
  - [ ] Registry sanitization edge cases
  - [ ] Docker Library handling
  - [ ] Expected: Correctly formatted target paths following all rules

- [ ] **`TestRegistryMappingIntegration`:**
  - [ ] Mapping file with various registry mappings
  - [ ] Source registries defined and undefined in mapping
  - [ ] Expected: Correct application of mapping rules

### 17.4 CLI and Integration Tests (`cmd/helm-image-override/main_test.go`)

- [ ] **`TestCLI_FlagValidation`:**
  - [ ] Missing required flags
  - [ ] Invalid flag values
  - [ ] Flag interactions and conflicts
  - [ ] Expected: Appropriate error messages and exit codes

- [ ] **`TestCLI_ExitCodes`:**
  - [ ] Various error conditions triggering specific exit codes
  - [ ] Charts with unsupported structures in strict mode
  - [ ] Expected: Correct exit codes for different error conditions

- [ ] **`TestCLI_OutputToFile`:**
  - [ ] Redirect output to file
  - [ ] Verify file contents
  - [ ] Expected: Correct file creation and content

### 17.5 Implementation Strategy

1. Prioritize test cases that improve core functionality coverage first:
   - Complex structure detection
   - Partial map handling
   - Path-based modification
   - Template variable preservation

2. Add regression tests for specific bugs fixed during development:
   - Docker library normalization
   - Registry sanitization
   - Path generation for special cases

3. Ensure edge cases and potential future issues are covered:
   - Type conflicts in maps
   - Invalid image references
   - Complex nesting and array handling

4. Target a minimum 80% test coverage for all core packages.

## 18. Python Test Script (`test-charts.py`) Stabilization

**Goal:** Ensure `test/tools/test-charts.py` can reliably extract various chart structures, execute override generation, validate results with `helm template`, and handle chart pulling errors gracefully.

**Note:** Items 18.1, 18.2, and 18.3 must be addressed together. Fixing extraction (18.1) without implementing override/validation (18.2, 18.3) previously led to tests halting. Implementing override/validation without fixing extraction leads to the current state where tests fail early.

### 18.1 Fix Chart Extraction Logic (High Priority)
- [ ] **Modify `test_chart_override`:** Re-implement robust logic to find the correct chart directory after extraction.
  - [ ] Check for `Chart.yaml` directly within the `temp_dir`.
  - [ ] If not found, reliably iterate through *potential* subdirectories within `temp_dir` to locate the one containing `Chart.yaml`.
  - [ ] Gracefully handle errors if `Chart.yaml` cannot be located after extraction.

### 18.2 Complete Override Generation Testing (High Priority)
- [ ] **Modify `test_chart_override`:** Implement the full process of running the `irr override` command after successful chart extraction:
  - [ ] Construct the `irr override` command using the *correctly identified* `extracted_chart_dir`, `--chart-path`, `--target-registry`, `--source-registries`, and `--output-file` arguments.
  - [ ] Execute the `irr override` command using `asyncio.create_subprocess_exec`, capturing stdout/stderr.
  - [ ] Check the exit code of the `irr` process. Report failure clearly if non-zero, including stderr.
  - [ ] Verify the specified `output_file` (e.g., `TEST_OUTPUT_DIR / f"{chart_name}-override.yaml"`) exists *and* is not empty after a successful `irr` command execution. Report error if not.

### 18.3 Implement Template Validation Step (Medium Priority)
- [ ] **Modify `test_chart_override`:** Add the `helm template` validation step *only after* successful override file generation (18.2):
  - [ ] Determine chart `classification` using `get_chart_classification`.
  - [ ] Generate the appropriate temporary values file (`temp_class_values.yaml`) using `get_values_content`.
  - [ ] Construct the `helm template` command using the *correctly identified* `extracted_chart_dir`, the temporary classification values (`-f temp_class_values.yaml`), and the generated override file (`-f output_file`). Ensure the override file is the *last* `-f` argument.
  - [ ] Execute the `helm template` command using `asyncio.create_subprocess_exec`, capturing stdout/stderr.
  - [ ] Check the exit code of the `helm` process. Report failure clearly if non-zero, including stderr.

### 18.4 Improve Chart Pulling Robustness (Medium Priority)
- [ ] **Enhance `main` / `pull_chart`:** (Separate from extraction/override logic)
  - [ ] Ensure `helm repo update` is called reliably before chart listing/pulling begins (consider adding it in `main`).
  - [ ] Refine error handling in `pull_chart` to better differentiate between potentially recoverable errors (timeouts, rate limits) and unrecoverable ones (404s, chart not found).
  - [ ] (Optional) Implement a simple retry mechanism for `helm pull` on specific, transient error types.
  - [ ] Clearly log and potentially skip charts that consistently fail to pull after updates/retries, reporting these skips in the summary.

### 18.5 Refine Error Reporting (Low Priority)
- [ ] **Enhance `categorize_error` and `TestResult`:**
  - [ ] Ensure distinct statuses/categories exist for failures during extraction, override generation, and template validation (e.g., `EXTRACTION_ERROR`, `OVERRIDE_CMD_FAILED`, `OVERRIDE_FILE_MISSING`, `TEMPLATE_VALIDATION_FAILED`).
  - [ ] Include relevant command output (stderr primarily) in `TestResult.details` for easier debugging.

### 18.6 (Optional) Add Verbose Logging
- [ ] Add `print` statements within `test_chart_override` to log key steps:
    - Path to the identified extracted chart directory.
    - Chart classification result.
    - Specific `irr override` command being executed.
    - Specific `helm template` command being executed.
    - Outcome of each command execution (success/failure + exit code).
    - Path to the generated override file.
    - Path to the temporary classification values file used.

   