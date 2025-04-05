# Test Coverage Improvement Plan

## 1. Goal and Scope

*   **Primary Goal:** Pragmatically increase Go test coverage to **at least 70%** overall, focusing on core logic to ensure stability during iterative development.
*   **Initial Focus (Catch Major Issues):** Prioritize packages critical to the main workflow: `pkg/chart` (loading, generation), `pkg/analysis` (value traversal, image identification), `pkg/image` (parsing common patterns), `pkg/override` (structure generation), `pkg/strategy` (default strategy), `pkg/registry` (basic mapping), `cmd/irr` (core command execution).
*   **Subsequent Phases:** Target higher coverage (>80%), address utility packages (`pkg/debug`, `pkg/log`, `pkg/testutil`), test edge cases identified during broader chart testing, and refine tests as the codebase evolves to support more chart types.

## 2. Current Status (as of YYYY-MM-DD - *Update with Current Date*)

*   **Overall Coverage:** *TBD - Run `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out` to update.* (Previously 28.6%)
*   **Progress Summary:**
    *   Completed initial unit test implementation for core logic in `pkg/analysis` and `pkg/chart`.
    *   Resolved numerous build and test assertion failures across multiple packages.
    *   Refactored `pkg/chart` (Generator, Loader) for better testability, including `afero` integration.
    *   **Fixed `pkg/image.ParseImageReference` test failures related to tag/digest parsing.**
    *   **Fixed `pkg/chart/generator.go` build errors related to error handling comparisons.**
    *   Fixed `pkg/registry` tests related to CWD checks during testing.
    *   Fixed `cmd/irr` build errors and Makefile dependencies.
    *   **All Go unit tests (`make test`) are currently passing.**
    *   Clarified separation between unit tests (`make test`) and integration tests (`make test-charts` using Python scripts).
*   **Coverage Breakdown (Estimates based on work done):**
    *   `github.com/lalbers/irr/cmd/irr`: Low (Build fixes, no dedicated tests yet)
    *   `github.com/lalbers/irr/pkg/analysis`: High (Phase 1 focus - likely >70%)
    *   `github.com/lalbers/irr/pkg/chart`: High (Phase 1 focus - likely >70%)
    *   `github.com/lalbers/irr/pkg/debug`: 0.0%
    *   `github.com/lalbers/irr/pkg/image`: Medium-High (Tests added/fixed for parsing/path_utils - likely >70% now)
    *   `github.com/lalbers/irr/pkg/log`: 0.0%
    *   `github.com/lalbers/irr/pkg/override`: 58.3% (Unchanged, though path_utils tested indirectly)
    *   `github.com/lalbers/irr/pkg/registry`: High (Phase 1 focus, TestLoadMappings fixed - likely >81.8%)
    *   `github.com/lalbers/irr/pkg/registrymapping`: N/A (Removed/Consolidated)
    *   `github.com/lalbers/irr/pkg/strategy`: 58.3% (Unchanged)
    *   `github.com/lalbers/irr/pkg/testutil`: 0.0%
    *   `github.com/lalbers/irr/test/integration`: N/A (Handled by separate Python scripts)
*   **Next Priority:** Phase 2 - Increase coverage in `pkg/image`, `pkg/override`, `pkg/strategy`. Phase 3 - Test `cmd/irr`.

## 3. Implementation Plan (Detailed)

*Prioritization:* Focus on implementing tests for core functionality and common use cases first within each phase. Defer complex edge cases or less common scenarios until basic coverage is established. Use table-driven tests extensively.

### Phase 1: Target 0% Coverage Packages (Core Logic First) - **[COMPLETED]**

*   **Packages:** `pkg/analysis`, `pkg/chart` (focus on `loader` and `generator`), `pkg/registry` (was partially covered, now fully addressed in this phase).
*   **Target Coverage:** Achieved good coverage (>70% estimated) in core functions.
*   **Specific Actions:**
    *   **`pkg/analysis` (`analyzer.go`, `types.go`):** **[COMPLETED]**
        *   `TestNewAnalyzer`: **[DONE]**
        *   `TestAnalyze`: **[DONE]** (Covered core scenarios, mocked loader errors)
        *   `TestNormalizeImageValues`: **[DONE]** (Covered core normalization, fixed assertions)
        *   `TestAnalyzeValues`/`AnalyzeArray`: **[DONE]** (Covered core recursion, simple maps/arrays, fixed assertions)
    *   **`pkg/chart/loader.go`:** **[COMPLETED]**
        *   `TestLoadChart` (Renamed `TestHelmLoaderLoad`): **[DONE]** (Tested dir/non-existent path, file path error, used `afero` implicitly via Generator tests)
        *   `TestProcessChart`: *(Deferred - Covered indirectly via Generator tests needing loaded charts)*
    *   **`pkg/chart/generator.go`:** **[COMPLETED]**
        *   `TestNewGenerator`: **[DONE]**
        *   `TestGenerate`: **[DONE]** (Tested core logic with mocks/fixtures)
        *   `TestOverridesToYAML`: **[DONE]** (Tested simple/nested/empty/nil cases, fixed assertions)
        *   `TestValidateHelmTemplate`: **[DONE]** (Mocked `exec.Command`, tested valid/invalid/error cases, relaxed assertion for known validator limits)
    *   **`pkg/registrymapping` (`mappings.go`):** **[REMOVED/CONSOLIDATED]** Logic tested within `pkg/registry`.
    *   **`pkg/registry` (`mappings.go`):** (Addressed here as part of Phase 1 focus) **[COMPLETED]**
        *   `TestLoadMappings`: **[DONE]** (Tested valid/non-existent/empty, fixed CWD issue for testing)
        *   `TestGetTargetRegistry`: **[DONE]** (Existing tests sufficient)

### Phase 2: Increase Coverage in Partially Covered Core Packages - **[NEXT STEP]**

*   **Packages:** `pkg/image`, `pkg/override`, `pkg/strategy`.
*   **Target Coverage:** Aim for >75% in these packages, focusing on uncovered functions and common code paths.
*   **Specific Actions:**
    *   **`pkg/image` (`detection.go`, `path_utils.go`):** High priority. **[PARTIALLY DONE]**
        *   `TestParseImageMap` (0%): Essential helper. **[DONE]**
        *   `TestParseImageReference`: **[DONE]** (Main parsing/validation logic fixed and tested).
        *   `TestIsValid*`: Focus on common validation rules. **[DONE]** // Covered by TestIsValidImageReference
        *   `TestDetectImages` (Line 834 vs 256): Investigate/test/remove. **[DONE]** // Added comprehensive tests
        *   `TestGetValueAtPath`, `TestSetValueAtPath` (in `path_utils_test.go`): **[DONE]**
    *   **`pkg/override` (`override.go`, `path_utils.go`):** Focus on core structure manipulation.
        *   *(Note: `path_utils.go` functions are now tested via `pkg/image/path_utils_test.go`)*
        *   // `TestConstructPath` skipped - function is trivial (identity function).
        *   `TestConstructSubchartPath` (0%): Basic subchart path handling. **[DONE]**
        *   `TestMergeInto` (0%): Core merging logic. **[DONE]**
        *   *(Lower Priority Functions initially):* `TestGenerateYAML`, `TestToYAML`/`TestJSONToYAML`, `TestDeepCopy`/`TestSetValueAtPath` unless proven critical.
    *   **`pkg/strategy` (`path_strategy.go`):** Ensure default strategy is well-tested.
        *   `TestPrefixSourceRegistryStrategy`: Enhance tests (review coverage gaps). **[DONE]** // Consolidated & enhanced tests
        *   `TestFlatStrategy` (0%): *(Lower Priority)* Implement tests only if/when used. **[DONE]**

### Phase 3: Test Command-Line Interface (`cmd/irr`) - **[DONE]**

*   **Package:** `cmd/irr` (`main.go`, `analyze.go`, `override.go` -> refactored to `root.go`, `main.go`, etc.)
*   **Target Coverage:** Aim for >60%.
*   **Specific Actions:** Use Cobra's testing helpers or `bytes.Buffer`. Mock core packages.
    *   Command Setup: **[DONE]**
    *   Flag Parsing & Validation: **[DONE]**
    *   Core Logic Execution (`runDefault`, `runAnalyze`): **[DONE]**
    *   Helper Functions: *(Covered implicitly or in other packages)*

### Phase 4: Address Utility/Supporting Packages - **[TODO]**

*   **Packages:** `pkg/debug`, `pkg/log`, `pkg/testutil`.
*   **Target Coverage:** Aim for >50%.
*   **Specific Actions:**
    *   **`pkg/debug`:** **[TODO]**
    *   **`pkg/log`:** **[TODO]**
    *   **`pkg/testutil`:** **[TODO]**

### Phase 5: Continuous Improvement and Broader Testing - **[ONGOING]**

*   **(Process Improvements)**
*   **Specific Actions:**
    *   **CI Setup:**
        1.  Configure CI (e.g., GitHub Actions) to run `go test -race -coverprofile=coverage.out ./...`. **[SETUP]**
        2.  Add step for basic function coverage report: `go tool cover -func=coverage.out`. **[SETUP - Verify output]**
        3.  **(Optional Later):** Integrate Codecov/Goveralls.
    *   **Regular Review:** **[TODO - Process]**
    *   **Chart Testing Script:**
        1.  Ensure Python scripts (`test/tools/pull-charts.py`, `test/tools/test-charts.py`) are run periodically (manually or CI). **[PROCESS DEFINED]**
        2.  When failures occur, analyze if a *unit test* could have caught the issue. **[PROCESS DEFINED]**

## 4. Tools & Techniques

*   **Coverage Generation:** `go test -race -coverprofile=coverage.out ./...`
*   **Coverage Reporting (Function Level):** `go tool cover -func=coverage.out`
*   **Coverage Visualization (HTML):** `go tool cover -html=coverage.out`
*   **Testing Framework:** Standard Go `testing` package.
*   **Assertions:** `stretchr/testify/assert` and `stretchr/testify/require`.
*   **Mocking:**
    *   Use standard Go interfaces where possible. **[APPLIED]**
    *   Use function variables for simple mocks. **[APPLIED]**
    *   Use `afero` for filesystem mocking. **[APPLIED]**
    *   Mock external commands (`exec.Command`) when testing wrappers like `ValidateHelmTemplate`. **[APPLIED]**
    *   Create specific mock implementations for core interfaces (e.g., `ChartLoader`, `Analyzer`, `CommandRunner`). **[APPLIED]**
*   **Test Structure:** Heavily favor table-driven tests (`[]struct{...}`). **[APPLIED]**
*   **Fuzz Testing:** `go test -fuzz` (Go >= 1.18). *(Lower Priority)*

### Additional Testing Tools & Practices

*   **Test Fixtures:** **[APPLIED]** (Used various maps, chart structures in tests).
*   **Parameterized Tests (Table-Driven):** **[APPLIED]** (Used extensively).
*   **Focused Mocks:** **[APPLIED]** (e.g., MockCommandRunner).

## 5. Tracking & Quality

*   **Primary Metric:** Overall and core package coverage percentage (`go tool cover -func`).
*   **Tracking Process:**
    1.  Establish baseline coverage before starting. **[DONE - 28.6%]**
    2.  Monitor coverage changes via CI output after each phase or significant PR. **[PROCESS]**
    3.  Use HTML reports during reviews to pinpoint specific uncovered lines. **[PROCESS]**
*   **Qualitative Focus:** Prioritize testing *critical paths* and *common scenarios*. **[APPLIED]**
*   **Test Quality Checklist (For key tests):** **[APPLIED Implicitly]**
*   Maintain a list of known coverage gaps or complex scenarios deferred. **[This Doc]**

## 6. Implementation Hints

*   **Start with Phase 1 Core Logic:** **[DONE]**
*   **`registrymapping` vs. `registry`:** **[DONE - Consolidated]**
*   **Leverage `testutil`:** **[TODO - Phase 4]**
*   **Filesystem Mocking (`afero`):** **[DONE]**
*   **Image Parsing (`pkg/image`):** **[NEXT - Phase 2]**
*   **Override Logic (`pkg/override`):** **[NEXT - Phase 2]**
*   **Strategy Pattern (`pkg/strategy`):** **[NEXT - Phase 2]**
*   **CLI Testing (`cmd/irr`):** **[DONE]**
*   **Existing Integration Tests:** Review `test/integration/*_test.go`. **[NOTE: Integration handled by Python scripts now]** Ensure unit tests cover logic identified by integration failures.
*   **Table-Driven Tests:** **[APPLIED]**
*   **Utility Packages (`pkg/log`, `pkg/debug`):** **[TODO - Phase 4]**
