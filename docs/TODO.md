# TODO.md - Helm Image Override Implementation Plan

## 1. Project Setup (Completed)
*Initial project setup, Go modules, directory structure, dependencies, Makefile, and basic CI are complete.*

## 2. Core Implementation

### 2.1 Chart Loading (Completed)
*Loading charts from directories and archives, parsing `values.yaml` and `Chart.yaml`, and recursive subchart handling are implemented.*

### 2.2 Image Processing (Completed - Refined in Later Sections)
*Initial image detection heuristics, regex parsing, normalization, registry filtering, and sanitization were implemented. This was significantly refactored later (see Sections 2.7, 14).*

### 2.3 Path Strategy (Completed - Base Implementation)
*Implemented `prefix-source-registry` strategy and designed framework for future strategies. Refinements occurred later.*

### 2.4 Output Generation (Completed - Refined in Later Sections)
*Initial override structure generation and YAML output implemented. Significantly refactored later (see Sections 9, 13).*

### 2.5 Debugging and Logging (Mostly Completed)
- [x] Implemented debug package and added logging to most key functions.
- [ ] Add debug logging to `OverridesToYAML` function.
- [x] Added `--debug` flag to CLI.

### 2.6 Bug Fixes and Improvements (Completed - Historical)
*Addressed initial YAML output issues and basic error handling improvements. Non-image value transformation was superseded by Section 2.7.*

### 2.7 Refactor Image Detection Logic (Completed)
*Refactored image detection away from blacklisting towards context-aware positive identification using structural context and stricter string parsing. Deprecated `isNonImageValue`.*

### 2.8 Registry Mapping Support (Completed - Refined in Section 23)
*Added initial support for registry mappings via CLI flag and YAML file. This functionality is being consolidated and tested thoroughly in Section 23.*

## 3. CLI Interface (Completed - Base Implementation)
*Implemented core CLI flags (`cobra`), input validation, exit codes, and basic error messaging. Specific flags like `--dry-run` and `--strict` need further testing/implementation (see Section 19).*

## 4. Testing Implementation

### 4.1 Unit Tests (Completed - Base Coverage)
*Initial unit tests covering core logic (value traversal, detection, parsing, normalization, path strategy, override generation, YAML output) were implemented. More comprehensive tests added/planned in later sections (e.g., Sections 17, 23).*

### 4.2 Integration Tests (Completed - Base Coverage)
*Core use case integration test (`kube-prometheus-stack`) implemented and validated. Further integration test improvements and fixes are tracked in Section 23.*

### 4.3 Bulk Chart Testing (In Progress - Python Script)
*Initial Python script (`test-charts.py`) created for testing against diverse charts. Further development and stabilization tracked in Sections 10, 18.*
- [ ] Refine test script for stability and better error reporting (See Section 18 tasks if script needs further work beyond stabilization).
- [ ] Expand chart corpus and analyze results systematically (See Section 10).

### 4.4 Performance Testing (Pending)
- [ ] Setup benchmark infrastructure (e.g., using `go test -bench` and standard test environment).
- [ ] Create benchmark tests for key functions (`LoadChart`, `DetectImages`, `GenerateOverrides`, `LoadMappings`) using charts/data of varying complexity.
- [ ] Measure execution time (`time/op`) and memory usage (`allocs/op`, `B/op`).
- [ ] Establish baseline performance metrics.

## 5. Documentation (Partially Completed)
- [x] Core documentation (`README.md`, CLI Reference, Path Strategies, Examples) created.
- [ ] Create Troubleshooting / Error Codes guide (Leverage errors defined in `pkg/*/errors.go`).
- [ ] Add comprehensive Contributor Guide (`CONTRIBUTING.md` - setup, testing, contribution process - see also Section 23 Preventive Measures).
- [ ] Update documentation to reflect recent refactoring and consolidation (Ongoing - ensure accuracy after Section 23 completion).

## 6. Release Process (Pending)
- [ ] Set up Git tagging strategy (e.g., SemVer `vX.Y.Z`).
- [ ] Create release build automation using GitHub Actions (triggered by tags).
- [ ] Publish cross-platform binaries (Linux AMD64, macOS AMD64/ARM64) to GitHub Releases.
- [ ] Ensure documentation is up-to-date and published with release.

## 7. Stretch Goals (Post-MVP - Pending)
*Potential future enhancements after stabilization.* 
- [ ] Implement `flat` path strategy.
- [ ] Implement multi-strategy support (different strategy per source registry).
- [ ] Add configuration file support (`--config`) for defining source/target/exclusions/custom patterns.
- [ ] Enhance image identification heuristics (e.g., custom key patterns via config).
- [ ] Improve handling of digest-based references (more robust parsing).
- [ ] Add comprehensive private registry exclusion patterns (potentially beyond just source registry name).
- [ ] Implement validation of generated target URLs (basic format check).
- [ ] Explore support for additional target registries (Quay, ECR, GCR, ACR, GHCR) considering their specific path/naming constraints.

## 8. Post-Refactor Cleanup & Fixes (Completed - Historical)
*Addressed specific normalization, sanitization, parsing, and test environment issues identified after initial refactoring. Subsequent issues tracked in later sections.*

## 9. Post-Refactor Override Generation Debugging & Fix (Completed - Historical)
*Investigated and implemented an initial fix for override generation structure issues. Superseded by the more robust path-based modification in Section 13.*

## 10. Systematic Helm Chart Analysis & Refinement (In Progress)
*Focuses on data-driven improvement by analyzing a large corpus of Helm charts.*
- [ ] **Test Infrastructure Enhancement:** Implement structured JSON result collection for `test-charts.py`.
- [x] **Chart Corpus Expansion:** Expanded chart list in `test/tools/test-charts.py`.
- [ ] **Corpus Maintenance:** Document chart selection criteria, implement automated version update checks.
- [ ] **Automated Pattern Detection:** Implement detectors (regex/AST?) for value structures (explicit maps, strings, globals, lists, non-image patterns) in `test-charts.py` or a separate Go tool.
- [ ] **Frequency & Correlation Analysis:** Develop tools/scripts to count patterns and identify correlations across the corpus results.
- [ ] **Schema Structure Analysis:** Implement tools to automatically extract and compare `values.schema.json` where available. Document common patterns and provider variations.
- [ ] **Data-Driven Refactoring Framework:** Define metrics (coverage, complexity, compatibility), create decision matrix template to guide future refactoring based on analysis results.
- [ ] **Container Array Pattern Support:** Add explicit support and test cases for `spec.containers`, `spec.initContainers` (Partially addressed in Section 14.3, verify coverage in Section 23 tests).
- [x] **Image Reference Focus:** Scope clarified to focus only on registry location changes.

## 11. Analyzer Refinement & Expanded Testing (Completed)
*Improved analyzer robustness (handling missing registry/tags) and expanded the test chart list in `test-charts.py`.*

## 12. Override Generation Testing & Refinement (Completed - Historical)
*Adapted `test-charts.py` for override testing and performed initial analysis. Superseded by Section 13 and subsequent test/fix cycles.*

## 13. Refactor Override Generation (Path-Based Modification) (Completed)
*Successfully implemented the core path-based override generation logic, significantly improving override accuracy for complex charts.*
- **Summary:** Achieved high analysis success rate (98%), generated valid overrides for numerous complex charts.

## 14. Image Detection and Override Generation Improvements (Completed - Addressed Core Issues)
*Refined image detection (consistency, partial maps, globals, templates, non-image types) and override generation (arrays, context) based on analysis. Addressed boolean/numeric value handling and path resolution.*
- **Note:** While core logic was implemented, remaining bugs and edge cases are addressed in Section 23 test failures.

## 15. Chart Testing Improvements (`test-charts.py`) (Completed - Significant Updates)
*Addressed major issues in the Python test script: corrected command syntax, enhanced default values (Bitnami), improved error categorization, implemented caching, added filtering options, and enhanced results analysis.*

## 16. Hybrid Chart Classification for Test Configuration (`test-charts.py`) (Completed)
*Implemented classification logic in the Python script to apply tailored default `values.yaml` content during `helm template` validation, reducing template errors.*

## 17. Comprehensive Test Case Improvements (Completed - Base Coverage)
*Expanded unit tests across key packages (`pkg/image`, `pkg/override`, `pkg/strategy`, `cmd/irr`) covering complex structures, edge cases, context variations, and CLI validation. Further test fixing tracked in Section 23.*

## 18. Python Test Script (`test-charts.py`) Stabilization (Completed)
*Fixed chart extraction, completed override generation command execution, implemented `helm template` validation step, and improved chart pulling robustness within the Python test script.*

## 19. Implement and Test CLI Flags (`--dry-run`, `--strict`) (Pending)
*Implementation and testing of `--dry-run` and `--strict` flags is pending.*
- [ ] **Define Behavior:** Review and potentially refine documented behavior in `DEVELOPMENT.md` or CLI reference, ensuring clarity on exit codes, output (stdout vs. file), and error handling specifics.
- [ ] **Unit Tests:**
    *   [ ] Add tests for CLI argument parsing of both flags.
    *   [ ] Add tests for core logic (mocking file I/O): verify `--dry-run` prevents writes, verify `--strict` triggers specific Exit Code 5 on defined unsupported structures (e.g., templated repository?), verify successful exit code (0) when no issues occur.
- [ ] **Integration Tests:**
    *   [ ] Fix `TestDryRunFlag` (ensure binary path correct, check exit code 0, assert no file created, assert specific preview output to stdout).
    *   [ ] Fix `TestStrictMode` (ensure `unsupported-test` chart triggers the flag, check exit code 5, assert specific error message). (Part of Section 23 debugging).
- [ ] **Code Implementation:** Review/update code in `cmd/irr/main.go` (and potentially `pkg/` libraries) for conditional logic related to file writing (`--dry-run`) and error/exit code handling (`--strict`).

## 20. Address Test and Lint Failures (April 6th) (Completed - Historical)
*Addressed specific test failures and linter errors identified on April 6th. Subsequent issues tracked in later sections.*

## 21. Code Organization Refactoring and Error Handling Improvements (Completed - Historical)
*Consolidated error definitions in `pkg/image`, fixed related linter issues, and began addressing test failures. Superseded by the more comprehensive consolidation and fixing plan in Section 23.*

## 22. Fix Current Lint and Test Failures (April 6th - Post Refactor) (Completed - Historical)
*Addressed further test failures and linter errors identified after the Section 21 refactoring. Superseded by the consolidated plan in Section 23.*

## 23. Consolidate Registry Logic, Fix Tests & Linter Issues (Consolidated Plan)

**Goal:** Achieve a stable codebase by consolidating duplicated registry logic, fixing all test failures, and addressing outstanding linter warnings.

**Completed Tasks:**

*   **Registry Consolidation (Critical Priority):** Successfully consolidated duplicated logic from `pkg/registrymapping` into `pkg/registry`, updating all codebase imports and tests. Removed the legacy `pkg/registrymapping` package.
*   **Core Logic Unit Test Fixes (High Priority):**
    *   Resolved import and type errors related to registry consolidation in `pkg/image/detection_test.go` and `cmd/irr/override_test.go`.
    *   Fixed significant regressions in `pkg/image` parsing logic (`ParseImageReference`, `TryExtract...`), updated dependent tests (`TestDetectImages`, `TestImageDetector_ContainerArrays`, etc.) and verified normalization. Added focused unit tests for internal functions (`traverseValues`, `tryExtractImageFromMap`, `tryExtractImageFromString`, registry filtering, error paths) for better isolation.
    *   Fixed `pkg/strategy` unit tests (`TestPrefixSourceRegistryStrategy_GeneratePath_WithMappings`) to work with the consolidated registry package.
*   **Command Layer & Integration Test Fixes (Partial):**
    *   Fixed `TestReadOverridesFromStdout` integration test by using `--output-file`.
*   **Linter Warning Fixes (High & Medium Priority):**
    *   Resolved all high-priority `revive`, `goconst`, and `misspell` warnings.
    *   Fixed the `typecheck` error.
    *   Resolved several medium-priority `gocritic` warnings (commented imports, param type combinations, empty string tests, unnamed results, regexp simplification).

**Remaining Tasks & Priorities:**

+**Execution Order & Dependencies:**
+- Fix core logic unit tests first as integration tests depend on them
+- After fixing each component, run its specific tests before proceeding to dependent components
+- When making linter fixes, verify functionality isn't broken by running relevant tests
+
1.  **Fix Remaining Core Logic Unit Test Failures (High Priority)**
    *   **`pkg/chart` & `pkg/generator`:**
        *   [ ] Debug `TestGenerate/*` failures in `pkg/chart/generator_test.go`:
            *   **Files:** `pkg/chart/generator_test.go`, `pkg/chart/generator.go`
            *   **Hints:** In `generator_test.go`, trace `registry.Mappings` into `Generator.Generate`. In `generator.go`, check the loop calling `PathStrategy.GeneratePath`. Add `require.NoError` for `os.Remove` calls and check map accesses in tests.
+           *   **Testing:** `go test -v ./pkg/chart/... -run TestGenerate`
+           *   **Dependencies:** None, but success here is required before integration tests will pass

2.  **Fix Remaining Command Layer & Integration Test Failures (Medium Priority)**
    *   **`cmd/irr`:**
        *   [ ] Address `TestRunOverride/errcheck` failures in `cmd/irr/override_test.go`: Ensure `os.Remove` errors are checked for reliable test cleanup.
            *   **Files:** `cmd/irr/override_test.go`
            *   **Hints:** Locate `os.Remove` calls (likely in `defer` blocks) and wrap with `require.NoError(t, err)`.
+           *   **Testing:** `go test -v ./cmd/irr/... -run TestRunOverride`
+           *   **Risk:** Low - this fix only affects tests, not functionality
    *   **Integration Tests (`test/integration`):**
        *   [ ] Fix `TestStrictMode`: Debug the full `--strict` flag flow: Verify CLI parsing, check that detection logic identifies the specific unsupported structure in `unsupported-test` chart, and confirm translation to Exit Code 5.
            *   **Files:** `test/integration/integration_test.go`, `cmd/irr/main.go` (or `root.go`), `pkg/image/detection.go`, `test/fixtures/charts/unsupported-test`
            *   **Hints:** Ensure `--strict` flag is passed in test. Trace flag processing in `cmd/`. Verify detection logic in `pkg/image`. Confirm error handling leads to `os.Exit(5)`.
+           *   **Testing:** `go test -v ./test/integration/... -run TestStrictMode`
+           *   **Dependencies:** Depends on core logic unit tests passing, particularly `pkg/image` detection logic
        *   [ ] Fix remaining failures (e.g., `TestComplexChartFeatures/*`): Debug specific interactions: e.g., Does `--registry-mapping` work with `--path-strategy` for images in `globals`? How are templated image fields (`{{ .Values... }}`) handled? Does it handle partially specified images (repo/tag only) in complex nested values?
            *   **Files:** `test/integration/integration_test.go`, `cmd/irr/main.go` (and related cmd files), `pkg/image/detection.go`, `pkg/override/generator.go`, `pkg/strategy/*.go`, `test/fixtures/charts/*`
            *   **Hints:** Add targeted logging in `cmd/`, `pkg/image`, `pkg/strategy`, `pkg/override` to trace flag interpretation, image detection, path generation, and override construction for the specific failing chart fixture.
+           *   **Testing:** `go test -v ./test/integration/... -run TestComplexChartFeatures`
+           *   **Dependencies:** Depends on all core logic unit tests passing, particularly `pkg/image`, `pkg/strategy`, and `pkg/override`
+           *   **Debug Strategy:** Add temporary debug logs with `DEBUG=1 go test -v ./test/integration/...` and examine the full trace of image detection, path strategy application, and override generation

3.  **Address Remaining Linter Warnings (Medium / Low Priority)**
+   *   **Important Note:** After each linter fix, run relevant tests to ensure no functionality is broken:
+       *   For files in `pkg/`, run: `go test ./pkg/...`
+       *   For files in `cmd/`, run: `go test ./cmd/...`
+       *   If changes affect critical paths, run integration tests: `go test ./test/integration/...`
    *   **`gocritic` (Medium Priority - 21 issues remaining from 34):**
        *   Fix in order:
            *   [ ] Fix nesting reduction (1 in `pkg/override/path_utils_test.go`)
                *   **Hints:** Refactor nested `if`/`else` blocks.
+               *   **Risk:** Low - test file only, no functional impact
            *   [ ] Remove commented-out code (11 instances)
                *   **Hints:** Delete commented lines identified by the linter across various files.
+               *   **Risk:** Low - removing dead code
            *   [ ] Address `appendAssign` issues
                *   **Hints:** Change `slice = append(slice, ...)` to `slice = append(slice, ...)` where flagged.
+               *   **Risk:** Medium - could affect logic if not done carefully
            *   [ ] Address `ifElseChain` issues
                *   **Hints:** Consolidate long `if/else if` chains where possible for clarity.
+               *   **Risk:** Medium - could affect logic paths
            *   [ ] Address `octalLiteral` issues
                *   **Hints:** Change literals like `0755` to `0o755`.
+               *   **Risk:** Low - syntax change only
    *   **`lll` (Low Priority - 52 issues):**
        *   Address line length issues across the codebase (command files, test files, core package files) where practical without sacrificing readability.
            *   **Files:** Various (`cmd/`, `pkg/`, `test/`)
            *   **Hints:** Break long function signatures, string literals (use constants?), assertions, comments. Use `gofmt`. Prioritize readability.
+           *   **Risk:** Low - formatting only
+           *   **Approach:** Tackle test files first, then implementation files, as test files are lower risk

5.  **Python Test Script (`test-charts.py`) Enhancements (Medium Priority)**
    *   **Goal:** Refactor `test-charts.py` into a more robust tool for iterative feedback during development, providing clearer, actionable summaries and better interaction with the `irr` binary.
+   *   **Files:** `test/tools/test-charts.py`
+   *   **Testing:** Run the script before and after changes to verify improvements: `python test/tools/test-charts.py`
+   *   **Dependencies:** Should be done after core functionality and tests are fixed, as it depends on the main `irr` binary working correctly
    *   **Tasks:**
        *   [ ] **Improve Reporting Clarity:**
            *   [ ] Analyze the script's main loop to identify what each unlabeled "Success Rate: X%" line represents (likely tied to chart classifications or processing stages).
            *   [ ] Modify the script to add clear labels to each success rate printed (e.g., "Overall Success", "Bitnami Success", "Template Validation Success").
            *   [ ] Consolidate the final output into a structured summary section/table, reducing redundant lines.
+           *   **Verification:** Check that output is more concise and clearly labeled
        *   [ ] **Implement Iterative Feedback:**
            *   [ ] Enhance `generate_summary_json` (or create a new simpler summary file) to include key metrics (overall success rate, error category counts, success counts per classification).
            *   [ ] Add logic at the start of `main` to load the summary file from the *previous* run (if it exists).
            *   [ ] Display the delta (% change) for key metrics compared to the previous run alongside the current results in the final summary output.
+           *   **Verification:** Run the script multiple times and confirm it shows improvement/regression metrics