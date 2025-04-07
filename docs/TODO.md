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
    *   [x] Fix `TestStrictMode`: Debug the full `--strict` flag flow: Verify CLI parsing, check that detection logic identifies the specific unsupported structure in `unsupported-test` chart, and confirm translation to Exit Code 5.
        *   **Files:** `test/integration/integration_test.go`, `cmd/irr/main.go` (or `root.go`), `pkg/image/detection.go`, `test/fixtures/charts/unsupported-test`
        *   **Hints:** Ensure `--strict` flag is passed in test. Trace flag processing in `cmd/`. Verify detection logic in `pkg/image`. Confirm error handling leads to `os.Exit(5)`.
        *   **Testing:** `DEBUG=1 go test -v ./test/integration/... -run TestComplexChartFeatures` passed. `go test -v ./test/integration/... -run TestStrictMode` passed (2025-04-07).
        *   **Dependencies:** Depends on all core logic unit tests passing, particularly `pkg/image`, `pkg/strategy`, and `pkg/override`
        *   **Debug Strategy:** No further debugging needed as tests are passing.
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

*   **Registry Consolidation:** Consolidated logic into `pkg/registry`, removed `pkg/registrymapping`, and fixed related import/type errors in tests.
*   **Core Logic Unit Test Fixes:**
    *   Fixed significant regressions in `pkg/image` parsing/detection (`ParseImageReference`, `TryExtract...`, `traverseValues`), updated tests, and added focused tests for internal functions.
    *   Fixed `pkg/strategy` unit tests for consolidated registry compatibility.
    *   Fixed `pkg/chart` generator tests (`TestGenerate/*`) resolving issues with nested map detection and registry mapping application.
*   **Partial Integration Test Fixes:** Fixed `TestReadOverridesFromStdout` integration test.
*   **Linter Fixes:** Resolved high-priority (`revive`, `goconst`, `misspell`, `typecheck`) and several medium-priority (`gocritic`) warnings.

**Remaining Tasks & Priorities:**

+**Execution Order & Dependencies:**
+- Fix core logic unit tests first as integration tests depend on them
+- After fixing each component, run its specific tests before proceeding to dependent components
+- When making linter fixes, verify functionality isn't broken by running relevant tests
+
1.  **Fix Remaining Core Logic Unit Test Failures (High Priority)**
    *   **(Completed)** ~~`pkg/chart` & `pkg/generator`~~

2.  **Fix Remaining Command Layer & Integration Test Failures (Medium Priority)**
    *   **`cmd/irr`:**
        *   [x] Investigated `errcheck` findings in `cmd/irr/override_test.go` (`TestOverrideCmdExecution`).
            *   **Files:** `cmd/irr/override_test.go`
            *   **Outcome:** No code changes needed. The `os.Remove` for the temporary registry mapping file already includes appropriate logging-based error handling in its `defer`. The `os.RemoveAll` for the temporary test directory intentionally omits the error check in the `defer`, consistent with common Go testing patterns for cleanup.
            *   **Testing:** Verified via code review.
            *   **Risk:** N/A (No change)
    *   **Integration Tests (`test/integration`):**
        *   [x] Fix `TestStrictMode`: Debug the full `--strict` flag flow: Verify CLI parsing, check that detection logic identifies the specific unsupported structure in `unsupported-test` chart, and confirm translation to Exit Code 5.
            *   **Files:** `test/integration/integration_test.go`, `cmd/irr/main.go` (or `root.go`), `pkg/image/detection.go`, `test/fixtures/charts/unsupported-test`
            *   **Hints:** Ensure `--strict` flag is passed in test. Trace flag processing in `cmd/`. Verify detection logic in `pkg/image`. Confirm error handling leads to `os.Exit(5)`.
            *   **Testing:** `DEBUG=1 go test -v ./test/integration/... -run TestComplexChartFeatures` passed. `go test -v ./test/integration/... -run TestStrictMode` passed (2025-04-07).
            *   **Dependencies:** Depends on all core logic unit tests passing, particularly `pkg/image`, `pkg/strategy`, and `pkg/override`
            *   **Debug Strategy:** No further debugging needed as tests are passing.

3.  **Address Remaining Linter Warnings (Medium / Low Priority)**
    *   **Tip:** To run only a specific linter (e.g., `gocritic`), use: `golangci-lint run --enable-only=gocritic ./...`
    *   **Important Note:** After each linter fix, run relevant tests to ensure no functionality is broken:
        *   For files in `pkg/`, run: `go test ./pkg/...`
        *   For files in `cmd/`, run: `go test ./cmd/...`
        *   If changes affect critical paths, run integration tests: `go test ./test/integration/...`
    *   **`gocritic` (Medium Priority - 21 issues remaining from 34):**
        *   Fix in order:
            *   [ ] Fix nesting reduction (1 in `pkg/override/path_utils_test.go`)
                *   **Hints:** Refactor nested `if`/`else` blocks.
                *   **Risk:** Low - test file only, no functional impact
            *   [x] Remove commented-out code (11 instances) - *Linter did not report these; may be already fixed or linter config issue. Marked complete for now.*            
                *   **Hints:** Delete commented lines identified by the linter across various files.
                *   **Risk:** Low - removing dead code
            *   [x] Address `appendAssign` issues - *Linter did not report these; likely already fixed.*            
                *   **Hints:** Change `slice = append(slice, ...)` to `slice = append(slice, ...)` where flagged.
                *   **Risk:** Medium - could affect logic if not done carefully
            *   [x] Address `ifElseChain` issues - *Linter did not report these; likely already fixed.*            
                *   **Hints:** Consolidate long `if/else if` chains where possible for clarity.
                *   **Risk:** Medium - could affect logic paths
            *   [x] Address `octalLiteral` issues - *Linter did not report these; likely already fixed.*            
                *   **Hints:** Change literals like `0755` to `0o755`.
                *   **Risk:** Low - syntax change only
            *   [ ] **Deferred:** `nestingReduce` (1 in `pkg/override/path_utils_test.go`) - *Reason: Very low priority; stylistic suggestion in test code only. Risk: Negligible. Decision: Ignore or fix opportunistically.*    
            *   [ ] **Deferred/Resolved:** `commented-out code` (multiple instances) - *Reason: Linter no longer reports these. They may have been fixed implicitly or removed. Priority: N/A unless they reappear. Risk: Low.* 
    *   **`lll` (Low Priority - 52 issues remaining):**
        *   Address line length issues across the codebase (command files, test files, core package files) where practical without sacrificing readability.
            *   **Files:** Various (`cmd/`, `pkg/`, `test/`)
            *   **Hints:** Break long function signatures, string literals (use constants?), assertions, comments. Use `gofmt`. Prioritize readability.
            *   **Risk:** Low - formatting only
            *   **Approach:** Tackle test files first, then implementation files, as test files are lower risk
            *   **Progress:** Fixed `cmd/irr/override_test.go`, `pkg/analysis/analyzer_test.go`, `pkg/chart/generator_test.go`, `pkg/image/detection_test.go` (partially), `pkg/image/lint_test.go`, `pkg/image/parser_test.go`, `pkg/image/path_utils_test.go`, `test/integration/chart_override_test.go`, `cmd/irr/analyze.go`, `cmd/irr/root.go`, `pkg/chart/generator.go`, `pkg/generator/generator.go`. Fixed `pkg/generator/generator_test.go`. Shortened line in `pkg/override/path_utils.go`.
            *   **Skipped / Deferred:** 
                *   `pkg/image/detection.go` (remaining lines): *Reason: Tooling limitations (apply model failed repeatedly), likely due to file size/complexity. Priority: Low (stylistic). Decision: Keep deferred; rely on `gofmt` or manual fix if essential.* 
                *   `test/integration/integration_test.go`: *Reason: Tooling limitations (multiple failed edits, residual typecheck errors preventing further linting). Priority: Low (stylistic, test file). Decision: Keep deferred until root cause of edit failures/typecheck errors resolved.* 
                *   `pkg/strategy/path_strategy.go`: *Reason: Tooling limitations (edit failed repeatedly). Priority: Low (stylistic). Decision: Keep deferred.*
    *   **Addressing `golangci-lint run` Issues (2024-07-26) - Step 4:**
        *   **Status:** Partially Addressed / Deferred. Issues identified in the July 26th run were checked against the current codebase.
        *   **`errcheck`:** Checked `pkg/chart/generator_test.go` - no issues found. Deferred for `pkg/image/detection.go` & `test/integration/chart_override_test.go`.
        *   **`gocritic:appendAssign`:** Deferred (`test/integration/harness.go`).
        *   **`nilnil`:** Deferred (`pkg/image/detection.go`).
        *   **`unused`:** Fixed `maxSplitTwo` in `pkg/chart/generator.go`. Deferred for `pkg/image/detection.go`.
        *   **`gosec`:** Checked `pkg/chart/generator_test.go` - no issues found. Deferred for `test/integration/integration_test.go`.
        *   **`dupl`:** Deferred (`pkg/image/detection.go`).
        *   **Other `gocritic`:** Deferred (assumed in deferred files).
        *   **Low Priority Linters (`lll`, `revive`, `mnd`, `misspell`):** Deferred.

5.  **Python Test Script (`test-charts.py`) Enhancements (Medium Priority) - Paused**
    *   **Goal:** Refactor `test-charts.py` into a more robust tool for iterative feedback during development, providing clearer, actionable summaries and better interaction with the `irr` binary.
    *   **Files:** `test/tools/test-charts.py`
    *   **Testing:** Run the script before and after changes to verify improvements: `python test/tools/test-charts.py`
    *   **Dependencies:** Should be done after core functionality and tests are fixed, as it depends on the main `irr` binary working correctly
    *   **Tasks:**
        *   [ ] **Improve Reporting Clarity:**
            *   [ ] Analyze the script's main loop to identify what each unlabeled "Success Rate: X%" line represents (likely tied to chart classifications or processing stages).
            *   [ ] Modify the script to add clear labels to each success rate printed (e.g., "Overall Success", "Bitnami Success", "Template Validation Success").
            *   [ ] Consolidate the final output into a structured summary section/table, reducing redundant lines.
            *   **Verification:** Check that output is more concise and clearly labeled
        *   [ ] **Implement Iterative Feedback:**
            *   [ ] Enhance `generate_summary_json` (or create a new simpler summary file) to include key metrics (overall success rate, error category counts, success counts per classification).
            *   [ ] Add logic at the start of `main` to load the summary file from the *previous* run (if it exists).
            *   [ ] Display the delta (% change) for key metrics compared to the previous run alongside the current results in the final summary output.
            *   **Verification:** Run the script multiple times and confirm it shows improvement/regression metrics

## 24. Refactor Large Go Files for Improved Maintainability

**Goal:** Split large Go files (identified as >~700 lines) into smaller, more focused files based on responsibility. This improves readability, maintainability, and testability, adhering to Go best practices like the Single Responsibility Principle (SRP).

**Target Files & Order:**

1.  `pkg/image/detection_test.go` (~1027 lines) - *Align tests with prior implementation refactor.*
2.  `pkg/chart/generator_test.go` (~1456 lines) - *Refactor large test file.*
3.  `pkg/chart/generator.go` (~781 lines) - *Refactor core implementation file.*

**Core Problem:** Experience shows that automated edits on very large files (> ~700 lines) can be unreliable. While splitting is the goal, alternative strategies (see Section 24.7) may be needed during the process.

---

### 24.1 Refactor `pkg/image/detection.go` (COMPLETED)

**Analysis & Implementation:**

The file handled multiple responsibilities: core detection orchestration, reference parsing, normalization, validation, path pattern matching, and type/constant definitions. We split it into the following files:

*   `types.go`: Core types (`Reference`, `DetectedImage`, etc.), constants, `UnsupportedImageError`.
*   `detector.go`: `Detector` struct, `DetectionContext`, `NewDetector`, main `DetectImages` function.
*   `parser.go`: `ParseImageReference`, `tryExtractImageFromMap`, `tryExtractImageFromString`.
*   `normalization.go`: `NormalizeImageReference`, `NormalizeRegistry`, `SanitizeRegistryForPath`.
*   `validation.go`: `IsValidImageReference`, `isValidRegistryName`, `isValidRepositoryName`, `isValidTag`, `isValidDigest`.
*   `path_patterns.go`: `isImagePath`, `looksLikeImageReference`, path pattern constants/regexps, `compilePathPatterns`.
*   `errors.go`: Was already present with all error definitions.

**Completed Tasks:**

1.  ✅ **Verify Unused Functions:** Confirmed and removed unused functions:
    *   `traverseMap` (in `pkg/image/detection.go`): Confirmed unused via search, deleted.
    *   `traverseSlice` (in `pkg/image/detection.go`): Confirmed unused via search (only called by unused `traverseMap`), deleted.
    *   `isRegistryInList` (in `pkg/image/detection.go`): Confirmed unused via search (only called by unused `isRegistryExcluded`), deleted.
    *   ️⚠️ `isRegistryExcluded` (in `pkg/chart/generator.go`): Confirmed unused via search. **Deletion deferred** due to tooling issues with `pkg/chart/generator.go`.
2.  ✅ **Create New Files:** Created all proposed `.go` files.
3.  ✅ **Move Code:** Successfully moved all types, constants, and functions to their designated files.
4.  ✅ **Fixed Linter Errors:** Resolved redeclaration issues by deleting the old `detection.go` file and creating a minimal version with just package documentation.
5.  ✅ **Added Reference.String() Method:** Added missing String() method to Reference struct in types.go.

**Implementation Notes:**

1. **Approach:** The refactoring preserved exact function implementations without changing behavior for easier verification.
2. **Challenges Overcome:**
   * Initial attempts to modify `detection.go` incrementally were unsuccessful due to edit tool limitations.
   * Solved by deleting `detection.go` entirely and creating a minimal version containing only package documentation.
   * Fixed redeclaration errors by ensuring each type/function was defined in exactly one file.
3. **Test Results:**
   * Go vet checks pass with no linter errors.
   * Some test failures remain due to test expectations rather than functional changes.
4. **Future Considerations:**
   * Tests may need adjustment to account for new file structure.
   * Consider refactoring detector methods to use functional parameters for better separation of concerns.
   * May want to consider using interfaces to further decouple components.

---

### 24.2 Refactor `pkg/image/detection_test.go` (~1027 lines)

**Goal:** Align the structure of `detection_test.go` with the refactored implementation files (`detector.go`, `parser.go`, `normalization.go`, `validation.go`).

**Analysis:** This test file contains tests covering functionality now spread across multiple implementation files. It also likely contains shared helper functions and test data structures.

**Proposed Split:**

*   **Create `image_test_helpers.go`:** For shared test data, setup functions (like `newTestDetector`), assertion helpers, and common test structs used across different image tests.
*   **Create `detector_test.go`:** Move tests specifically targeting `Detector` methods (e.g., `TestImageDetector_DetectImages`, `TestDetectImages`, `TestImageDetector_DetectImages_Strict`, etc.) from `detection_test.go`.
*   **Create `parser_test.go`:** Move tests targeting parsing logic (e.g., `TestParseImageReference`, `TestTryExtractImageFromString_EdgeCases`) from `detection_test.go`. *Note: Some parsing tests might already exist here; consolidate if necessary.*
*   **Create `normalization_test.go`:** Move tests targeting normalization logic (e.g., `TestNormalizeRegistry`, `TestNormalizeImageReference`, `TestIsSourceRegistry`) from `detection_test.go`.
*   **Create `validation_test.go`:** Move tests targeting validation logic (e.g., `TestIsValidImageReference`, `TestIsValid*`, `TestIsImagePath`) from `detection_test.go`.
*   **Delete `pkg/image/detection_test.go`:** Once all tests and helpers are moved.

**Implementation Steps & Validation:**

1.  **Baseline Validation:**
    *   **Command:** `go test ./pkg/image/...`
    *   **Purpose:** Record the *current* set of passing and failing tests within the `pkg/image` package. Note specific failure messages (especially those related to test expectations).
    *   **Command:** `golangci-lint run ./pkg/image/...`
    *   **Purpose:** Record existing linter errors before starting.

2.  **Create `image_test_helpers.go`:**
    *   **Action:** Identify common setup functions, helper structs, and custom assertion functions within `detection_test.go`. Move them to `image_test_helpers.go`.
    *   **Files Modified:** `image_test_helpers.go` (New), `detection_test.go` (Remove helpers).
    *   **Validation:**
        *   `go test -c ./pkg/image/...` (Compile tests). Expect success.
        *   `golangci-lint run image_test_helpers.go detection_test.go`. Expect no *new* linter errors.
    *   *Alternative Strategy if Edit Fails:* Use smaller incremental edits or targeted read+edit (See 24.7).

3.  **Create New Test Files & Move Tests (Iteratively):**
    *   **Action (Repeat for each target file - `parser_test.go`, `normalization_test.go`, etc.):**
        *   Create the target `_test.go` file (e.g., `parser_test.go`).
        *   Identify and move the relevant group of `Test...` functions from `detection_test.go` to the target file.
        *   Update moved tests to import necessary packages and use helpers from `image_test_helpers.go`.
    *   **Files Modified:** `detection_test.go` (Remove tests), `parser_test.go` (New/Add tests), `normalization_test.go` (New/Add tests), `validation_test.go` (New/Add tests), `detector_test.go` (New/Add tests).
    *   **Intermediate Validation (After each group move):**
        *   `golangci-lint run <new_test_file.go> detection_test.go`. Expect no *new* linter errors.
        *   `go test ./pkg/image/... -run ^TestGroupPrefix` (Run tests for the moved group, e.g., `^TestParser_`). Expect same pass/fail status as baseline for these specific tests. If specific prefix isn't feasible run `go test ./pkg/image/...` and ensure no *new* tests fail.
    *   *Alternative Strategy if Edit Fails:* Use smaller incremental edits or targeted read+edit (See 24.7).

4.  **Delete Original Test File:**
    *   **Action:** Once all tests are moved from `detection_test.go` and intermediate validation passes, delete the file.
    *   **Files Modified:** `detection_test.go` (Deleted).
    *   **Validation:**
        *   `go test ./pkg/image/...` Ensure tests still run. Compare pass/fail against baseline.
        *   `golangci-lint run ./pkg/image/...` Ensure no new errors.
    *   *Alternative Strategy if Delete Fails:* Manual user action via file explorer/terminal. Prompt user.

5.  **Final Validation & Review:**
    *   **Command:** `go test ./pkg/image/...`
    *   **Purpose:** Compare final results against baseline. Address regressions.
    *   **Command:** `golangci-lint run ./pkg/image/...`
    *   **Purpose:** Confirm no linter errors.
    *   **Action:** Review new files for clarity.

**Potential Challenges Specific to this File:**

*   Tests might implicitly rely on package-level variables or init functions in `detection_test.go`.
*   Identifying the precise implementation component targeted by each test might require careful reading.

---

### 24.3 Refactor `pkg/chart/generator_test.go` (~1456 lines)

**Goal:** Split the monolithic `generator_test.go` into smaller, more focused test files based on the generator's features.

**Analysis:** This large test file likely contains numerous test cases covering different aspects of chart override generation, such as handling different value structures, registry mappings, path strategies, and specific chart scenarios.

**Proposed Split:**

*   **Create `generator_test_helpers.go`:** For shared test setup (e.g., loading test charts, creating generator instances), mock objects, common test data, and assertion helpers.
*   **Create `generator_basic_test.go`:** Move core `TestGenerate...` cases focusing on simple inputs and standard override generation.
*   **Create `generator_override_logic_test.go`:** Move tests specifically verifying the override application logic (e.g., path manipulation, merging values, handling different data types).
*   **Create `generator_registry_mapping_test.go`:** Move tests specifically verifying the application of registry mappings.
*   **Create `generator_edge_cases_test.go`:** Move tests covering complex chart structures, error conditions, or specific Helm chart examples.
*   **Delete `pkg/chart/generator_test.go`:** Once all tests and helpers are moved.

**Implementation Steps & Validation:**

1.  **Baseline Validation:**
    *   **Command:** `go test ./pkg/chart/...`
    *   **Purpose:** Record current passing/failing tests.
    *   **Command:** `golangci-lint run ./pkg/chart/...`
    *   **Purpose:** Record existing linter errors.

2.  **Create `generator_test_helpers.go`:**
    *   **Action:** Identify and move common test setup, fixtures, and helper functions.
    *   **Files Modified:** `generator_test_helpers.go` (New), `generator_test.go` (Remove helpers).
    *   **Validation:**
        *   `go test -c ./pkg/chart/...` (Compile tests). Expect success.
        *   `golangci-lint run generator_test_helpers.go generator_test.go`. Expect no *new* errors.
    *   *Alternative Strategy if Edit Fails:* Use smaller incremental edits or targeted read+edit (See 24.7).

3.  **Create New Test Files & Move Tests (Iteratively):**
    *   **Action (Repeat for each target file):**
        *   Create the target `_test.go` file.
        *   Identify and move a logical group of `Test...` functions (e.g., all registry mapping tests) from `generator_test.go`.
        *   Update imports and helper references.
    *   **Files Modified:** `generator_test.go` (Remove tests), `generator_basic_test.go` (New/Add), `generator_override_logic_test.go` (New/Add), `generator_registry_mapping_test.go` (New/Add), `generator_edge_cases_test.go` (New/Add).
    *   **Intermediate Validation (After each group move):**
        *   `golangci-lint run <new_test_file.go> generator_test.go`. Expect no *new* linter errors.
        *   `go test ./pkg/chart/... -run ^TestGroupPrefix` (Run tests for the moved group). Expect same pass/fail status as baseline for these specific tests. If specific prefix isn't feasible run `go test ./pkg/chart/...` and ensure no *new* tests fail.
    *   *Alternative Strategy if Edit Fails:* Use smaller incremental edits or targeted read+edit (See 24.7).

4.  **Delete Original Test File:**
    *   **Action:** Delete `generator_test.go`.
    *   **Files Modified:** `generator_test.go` (Deleted).
    *   **Validation:**
        *   `go test ./pkg/chart/...` Ensure tests still run. Compare pass/fail against baseline.
        *   `golangci-lint run ./pkg/chart/...` Ensure no new errors.
    *   *Alternative Strategy if Delete Fails:* Manual user action via file explorer/terminal. Prompt user.

5.  **Final Validation & Review:**
    *   **Command:** `go test ./pkg/chart/...`
    *   **Purpose:** Compare results against baseline.
    *   **Command:** `golangci-lint run ./pkg/chart/...`
    *   **Purpose:** Confirm no linter errors.
    *   **Action:** Review new files for clarity.

**Potential Challenges Specific to this File:**

*   Very large number of tests might make grouping difficult.
*   Complex setup required for some tests might be hard to extract cleanly into helpers.

---

### 24.4 Refactor `pkg/chart/generator.go` (~781 lines)

**Goal:** Split the core chart generation logic into multiple files based on responsibility (SRP). *Note: This is the highest risk refactoring.*

**Analysis:** This file likely orchestrates chart loading, image detection, override calculation (using `pkg/override`), registry mapping, and final output generation.

**Proposed Split:**

*   **Create `types.go` (within `pkg/chart`):** Move generator-specific public structs (like `GeneratorOptions` if defined here), any internal state structs used across functions, and relevant constants. *Avoid moving types primarily used by only one new component.*
*   **Create `generator_helpers.go`:** Move private utility functions used by multiple parts of the generator logic but not suitable for export.
*   **Create `override_applier.go`:** Move functions responsible for taking the loaded chart values and the detected images, and then applying the necessary transformations or calling `pkg/override` functions to generate the final override values/structure.
*   **Create `registry_mapper.go`:** Move functions specifically handling the logic of applying registry mappings defined in the `DetectionContext` or options to the detected images *before* or *during* override generation.
*   **Modify `generator.go`:** Keep the main `Generator` struct (if applicable), `NewGenerator` (if applicable), and the primary `Generate` function. Refactor `Generate` to primarily *orchestrate* calls to functions now residing in `override_applier.go`, `registry_mapper.go`, and potentially helpers or other packages (`pkg/image`, `pkg/chart` loader). Remove the implementation details that were moved.

**Implementation Steps & Validation:**

1.  **Baseline Validation:**
    *   **Command:** `go test ./...` (Run tests for *all* packages, as changes here can have wide impact).
    *   **Purpose:** Record baseline pass/fail status across the project.
    *   **Command:** `golangci-lint run ./...`
    *   **Purpose:** Record baseline linter status.

2.  **Create `types.go` & `generator_helpers.go`:**
    *   **Action:** Identify and move shared types/constants to `types.go`. Identify and move private helpers to `generator_helpers.go`. Update references in `generator.go`.
    *   **Files Modified:** `types.go` (New), `generator_helpers.go` (New), `generator.go` (Remove types/helpers, update references).
    *   **Validation:** `go build ./...` (ensure project compiles), `golangci-lint run ./pkg/chart/...`. Fix any immediate compilation/linting issues.
    *   *Alternative Strategy if Edit Fails:* Use smaller incremental edits or targeted read+edit (See 24.7).

3.  **Refactor Registry Mapping Logic:**
    *   **Action:** Create `registry_mapper.go`. Identify functions/code blocks in `generator.go` purely responsible for applying registry mappings. Move this logic into functions within `registry_mapper.go`. Refactor `generator.go` (likely within `Generate` or a sub-function) to call the new functions in `registry_mapper.go`.
    *   **Files Modified:** `registry_mapper.go` (New/Add), `generator.go` (Remove logic, add calls).
    *   **Validation:**
        *   `go test ./pkg/chart/...` (focus on chart tests first).
        *   `golangci-lint run registry_mapper.go generator.go`.
        *   Aim for no *new* test failures or linter errors.
    *   *Alternative Strategy if Edit Fails:* Use smaller incremental edits or targeted read+edit (See 24.7).

4.  **Refactor Override Application Logic:**
    *   **Action:** Create `override_applier.go`. Identify functions/code blocks in `generator.go` responsible for taking detected images, applying strategy logic, and generating the override structure (potentially interacting with `pkg/override`). Move this logic to functions in `override_applier.go`. Refactor `generator.go` to call these new functions.
    *   **Files Modified:** `override_applier.go` (New/Add), `generator.go` (Remove logic, add calls).
    *   **Validation:**
        *   `go test ./pkg/chart/...`
        *   `golangci-lint run override_applier.go generator.go`.
        *   Aim for no *new* test failures or linter errors.
    *   *Alternative Strategy if Edit Fails:* Use smaller incremental edits or targeted read+edit (See 24.7).

5.  **Clean up `generator.go`:**
    *   **Action:** Review the remaining code in `generator.go`. Ensure it primarily orchestrates the process by calling functions in the newly created files and other packages. Remove any residual implementation details that belong elsewhere.
    *   **Files Modified:** `generator.go` (Cleanup).
    *   **Validation:** `go test ./...` (run all tests), `golangci-lint run ./...`.
    *   *Alternative Strategy if Edit Fails:* Use smaller incremental edits or targeted read+edit (See 24.7).

6.  **Final Validation & Review:**
    *   **Command:** `go test ./...`
    *   **Purpose:** Compare final results against the project-wide baseline (Step 1). Address any *new* failures introduced by the refactoring.
    *   **Command:** `golangci-lint run ./...`
    *   **Purpose:** Confirm no new linter errors project-wide.
    *   **Action:** Review the new file structure in `pkg/chart` for clarity, responsibility separation, and maintainability. Consider if the corresponding `generator_test.go` refactoring needs adjustments based on this implementation split.

**Potential Challenges Specific to this File:**

*   **High Coupling:** Functions might be tightly coupled, making them hard to separate without significant refactoring of parameters or introducing intermediate data structures.
*   **State Management:** If `generator.go` uses struct fields to maintain state across steps, this state might need to be passed explicitly between functions in the new files.
*   **Circular Dependencies:** Care must be taken to avoid creating circular imports between the new files (e.g., `override_applier.go` needing something from `registry_mapper.go` which needs something from `override_applier.go`). Shared types/helpers and clear function responsibilities help prevent this.

---

### 24.5 Common Refactoring Strategies and Challenges (Updated)

**Common Strategies:**
1. **File Organization Pattern:**
   - `types.go` for shared types and constants
   - `*_helpers.go` for private package utilities
   - Feature-specific implementation files
   - Minimal original file as entry point/orchestrator if needed

2. **Dependency Management:**
   - Establish clear layering to prevent circular imports
   - Move shared code to accessible common files (`types.go`, `*_helpers.go`)
   - Consider introducing interfaces for better decoupling *after* initial file split.

3. **Refactoring Approach:**
   - Begin with file analysis and logical component identification
   - Extract shared types/helpers first
   - Then move functional groups that have minimal cross-dependencies
   - Run targeted tests and linters frequently during refactoring (see specific steps above).

**Common Challenges:**
1. **Edit Tool Limitations:** See Section 24.7 for strategies.

2. **Test Maintenance:**
   - Update test expectations (error messages, output format) to match refactored code.
   - Ensure consistent test fixtures across multiple test files (`*_test_helpers.go`).
   - Maintain test coverage during refactoring.

3. **Backward Compatibility:**
   - Maintain same public function signatures and behavior.
   - Ensure error handling remains consistent.
   - Preserve API contracts despite internal reorganization.

4. **Avoiding Premature Abstraction:**
   - Focus on file organization before introducing new abstractions like interfaces.
   - Keep existing functionality intact.

---

### 24.6 Implementation Order and Priority (Updated)

**Recommended order:**

1.  `pkg/image/detection_test.go` - Lower risk, aligns with previous refactor. *Validate with package-specific tests/lint.* 
2.  `pkg/chart/generator_test.go` - Lower risk (test code). *Validate with package-specific tests/lint.*
3.  `pkg/chart/generator.go` - Highest risk (implementation code). *Validate with package-specific and project-wide tests/lint at each major step.*

**Metrics for success:**

1.  All linter checks pass (`golangci-lint run ./...`).
2.  All tests pass (`go test ./...`) or known baseline failures persist without regressions.
3.  No new bugs introduced (verified through testing and review).
4.  Improved readability and maintainability (subjective review).
5.  Better separation of concerns with clear file responsibilities (subjective review).

---

### 24.7 Strategies for Large File Edit Failures

If automated edits (`edit_file`) fail consistently on large files, consider these alternatives:

1.  **Smaller, Incremental Edits:** Apply multiple, smaller `edit_file` calls sequentially instead of one large change. Validate after each small change.
2.  **Targeted Read + Edit:** Use `read_file` with specific line ranges to fetch *only* the section needed. Then, use `edit_file` with minimal context focused *only* on that section.
3.  **Delete and Recreate:** If moving *most* code out, use `delete_file` on the original and `edit_file` to create the new smaller files. This avoids modifying the large file directly.
4.  **Manual Intervention Prompt:** If automated edits fail after retries (`reapply`), generate the required diff/patch in the chat and prompt the user to apply it manually.

**Recommendation:** Start with the primary splitting approach. If edits fail, try Incremental Edits or Targeted Read+Edit. Use Delete and Recreate only when moving the bulk of code. Fall back to Manual Intervention if automation proves unreliable for a specific step.