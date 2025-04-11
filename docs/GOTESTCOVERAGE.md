# Test Coverage Improvement Plan

## 1. Goal and Scope

*   **Primary Goal:** Pragmatically increase Go test coverage to **at least 70%** overall, focusing on core logic to ensure stability during iterative development.
*   **Initial Focus (Catch Major Issues):** Prioritize packages critical to the main workflow: `pkg/chart` (loading, generation), `pkg/analysis` (value traversal, image identification), `pkg/image` (parsing common patterns), `pkg/override` (structure generation), `pkg/strategy` (default strategy), `pkg/registry` (basic mapping), `cmd/irr` (core command execution).
*   **Subsequent Phases:** Target higher coverage (>80%), address utility packages (`pkg/debug`, `pkg/log`, `pkg/testutil`), test edge cases identified during broader chart testing, and refine tests as the codebase evolves to support more chart types.

## 2. Current Status (as of 2024-08-14)

*   **Overall Coverage:** **63.7%** (Run `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`)
*   **Progress Summary:**
    *   Completed initial unit test implementation for core logic in `pkg/analysis` and `pkg/chart` (partial).
    *   Resolved numerous build and test assertion failures across multiple packages.
    *   Refactored `pkg/chart` (Generator, Loader) for better testability, including `afero` integration.
    *   Fixed `pkg/image.ParseImageReference` test failures related to tag/digest parsing.
    *   Fixed `pkg/chart/generator.go` build errors related to error handling comparisons.
    *   Fixed `pkg/registry` tests related to CWD checks during testing.
    *   Fixed `cmd/irr` build errors and Makefile dependencies.
    *   **Implemented tests for `cmd/irr` (Phase 3), covering command parsing, execution flow, and output handling.**
    *   **Resolved all `make lint` errors.**
    *   **All Go unit tests (`make test`) are currently passing.**
    *   Clarified separation between unit tests (`make test`) and integration tests (Python scripts in `test/tools/`).
*   **Coverage Breakdown (Current):**
    *   `github.com/lalbers/irr/cmd/irr`: **68.7%** (Phase 3 Complete)
    *   `github.com/lalbers/irr/pkg/analysis`: **86.8%** (Good coverage)
    *   `github.com/lalbers/irr/pkg/chart`: **61.1%** (Improved, needs more work)
    *   `github.com/lalbers/irr/pkg/debug`: **8.6%**
    *   `github.com/lalbers/irr/pkg/generator`: **75.0%**
    *   `github.com/lalbers/irr/pkg/image`: **71.3%** (Good coverage)
    *   `github.com/lalbers/irr/pkg/log`: **3.4%**
    *   `github.com/lalbers/irr/pkg/override`: **43.7%** (Needs improvement, includes path_utils)
    *   `github.com/lalbers/irr/pkg/registry`: **87.1%** (Good coverage - Consolidated)
    *   `github.com/lalbers/irr/pkg/strategy`: **90.9%** (Excellent coverage)
    *   `github.com/lalbers/irr/pkg/testutil`: **0.0%**
    *   `github.com/lalbers/irr/test/integration`: **72.0%** (Go test harness only)
*   **Next Priority:** Complete Phase 2 - Increase coverage in **`pkg/chart`**, `pkg/override`, `pkg/debug`, and `pkg/log`.

## 3. Implementation Plan (Detailed)

*Prioritization:* Focus on implementing tests for core functionality and common use cases first within each phase. Defer complex edge cases or less common scenarios until basic coverage is established. Use table-driven tests extensively.

### Phase 1: Target 0% Coverage Packages (Core Logic First) - **[COMPLETED]**

*   **Packages:** `pkg/analysis`, `pkg/chart` (initial focus), `pkg/registry` (initial focus).
*   **Target Coverage:** Achieved good initial coverage.
*   **Specific Actions:** *(Details omitted for brevity - see previous versions)*

### Phase 2: Increase Coverage in Partially Covered Core Packages - **[IN PROGRESS]**

*   **Packages:** `pkg/image`, `pkg/override`, `pkg/strategy`, **`pkg/chart` (Carry-over)**.
*   **Target Coverage:** Aim for >75% overall, focusing on uncovered functions and common code paths.
*   **Specific Actions:**
    *   **`pkg/image` (`detection.go`, `path_utils.go`):** **[DONE - 86.8%]** Excellent coverage achieved.
        *   `TestParseImageMap`: **[DONE]**
        *   `TestParseImageReference`: **[DONE]**
        *   `TestIsValid*`: **[DONE]**
        *   `TestDetectImages`: **[DONE]**
        *   `TestGetValueAtPath`, `TestSetValueAtPath`: **[DONE]**
    *   **`pkg/chart` (`generator.go`, `loader.go`):** **[TODO - 37.7%]** High priority due to low coverage.
        *   `TestGenerate`: Enhance tests to cover more scenarios (e.g., threshold logic, complex loader interactions, error paths).
        *   `TestGenerateOverrides` (Untested): Add tests.
        *   `TestProcessChartForOverrides` (Untested): Add tests.
        *   `TestMergeOverrides` (Untested): Add tests.
        *   `TestExtractSubtree` (Untested): Add tests.
        *   `TestCleanupTemplateVariables` (Untested): Add tests.
        *   `TestDefaultLoaderLoad`: Verify coverage (currently good at 88.2%).
    *   **`pkg/override` (`override.go`, `path_utils.go`):** **[TODO - 58.3%]** Focus on core structure manipulation.
        *   *(Note: `path_utils.go` functions tested via `pkg/override/path_utils_test.go` and `pkg/image/path_utils_test.go`)*
        *   `TestConstructPath` skipped (trivial).
        *   `TestConstructSubchartPath` (Untested - 0%): Add tests.
        *   `TestMergeInto` (Untested - 0%): Add tests.
        *   `TestGenerateYAML` (Untested - 0%): Add tests.
        *   `TestToYAML`/`TestJSONToYAML` (Untested - 0%): Add tests.
        *   `TestDeepCopy`: Good coverage (90.9%).
        *   `TestSetValueAtPath`: Good coverage (92.3%).
    *   **`pkg/strategy` (`path_strategy.go`):** **[TODO - 58.3%]** Ensure strategies are well-tested.
        *   `TestPrefixSourceRegistryStrategy`: Enhance tests, especially `GeneratePath` (87.5%).
        *   `TestFlatStrategy` (Untested - 0%): Implement tests only if/when used.

### Phase 3: Test Command-Line Interface (`cmd/irr`) - **[DONE - 73.6%]**

*   **Package:** `cmd/irr` (`root.go`, `main.go`, etc.)
*   **Target Coverage:** Aimed for >60%, achieved 73.6%.
*   **Specific Actions:** Used Cobra's testing helpers (`executeCommand`) and mocks.
    *   Command Setup: **[DONE]**
    *   Flag Parsing & Validation: **[DONE]**
    *   Core Logic Execution (`runDefault`, `runAnalyze`): **[DONE]**
    *   Helper Functions (`formatTextOutput`): **[DONE]**

### Phase 4: Address Utility/Supporting Packages - **[TODO]**

*   **Packages:** `pkg/debug` (0.0%), `pkg/log` (0.0%), `pkg/testutil` (0.0%).
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
        2.  Add step for function coverage report: `go tool cover -func=coverage.out`. **[SETUP - Verified]**
        3.  **(Optional Later):** Integrate Codecov/Goveralls.
    *   **Regular Review:** **[TODO - Process]**
    *   **Chart Testing Script:**
        1.  Python scripts (`test/tools/*.py`) are run periodically/manually. **[PROCESS DEFINED]**
        2.  Analyze script failures for potential unit test improvements. **[PROCESS DEFINED]**

## 4. Tools & Techniques

*   **Coverage Generation:** `go test -race -coverprofile=coverage.out ./...`
*   **Coverage Reporting (Function Level):** `go tool cover -func=coverage.out`
*   **Coverage Visualization (HTML):** `go tool cover -html=coverage.out`
*   **Testing Framework:** Standard Go `testing` package.
*   **Assertions:** `stretchr/testify/assert` and `stretchr/testify/require`.
*   **Mocking:**
    *   Standard Go interfaces. **[APPLIED]**
    *   Function variables. **[APPLIED]**
    *   `afero` for filesystem. **[APPLIED]**
    *   Mock external commands (`exec.Command`). **[APPLIED]**
    *   Specific mock implementations (e.g., `AnalyzerInterface`, `GeneratorInterface`). **[APPLIED]**
*   **Test Structure:** Table-driven tests (`[]struct{...}`). **[APPLIED]**
*   **Fuzz Testing:** `go test -fuzz`. *(Lower Priority)*

### Additional Testing Tools & Practices

*   **Test Fixtures:** **[APPLIED]**
*   **Parameterized Tests (Table-Driven):** **[APPLIED]**
*   **Focused Mocks:** **[APPLIED]**

## 5. Tracking & Quality

*   **Primary Metric:** Overall (61.8%) and core package coverage percentages.
*   **Tracking Process:**
    1.  Baseline established: **[DONE - 28.6%]**
    2.  Monitor via CI output/manual runs. **[PROCESS]**
    3.  Use HTML reports for reviews. **[PROCESS]**
*   **Qualitative Focus:** Testing critical paths and common scenarios. **[APPLIED]**
*   **Test Quality Checklist:** **[APPLIED Implicitly]**
*   Maintain list of known coverage gaps. **[This Doc]**

## 6. Implementation Hints

*   **`pkg/chart` Coverage:** **[NEXT - Phase 2]** High priority.
*   **`pkg/override` Coverage:** **[NEXT - Phase 2]**
*   **`pkg/strategy` Coverage:** **[NEXT - Phase 2]**
*   **Utility Packages:** **[TODO - Phase 4]**
*   **Table-Driven Tests:** **[APPLIED]**
