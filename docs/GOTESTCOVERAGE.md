# Test Coverage Improvement Plan

## 1. Goal and Scope

*   **Primary Goal:** Pragmatically increase Go test coverage to **at least 75%** for all core packages (`pkg/*`), and significantly reduce the number of functions with 0% coverage across the codebase. Aim for **>75% overall coverage** as a stretch goal.
*   **Immediate Focus (Address Gaps):** Prioritize packages currently below 75% (`pkg/chart`, `pkg/generator`, `pkg/helm`, `pkg/analyzer`, `pkg/testutil`) and functions with 0% coverage, particularly within `cmd/irr` and `internal/helm` due to their critical role in execution flow.
*   **Subsequent Phases:** Target higher coverage (>85%) for core packages, address utility packages more thoroughly, test edge cases identified during broader chart testing, and refine tests as the codebase evolves.

## 2. Current Status (as of [Insert Date - e.g., 2024-08-XX])

*   **Overall Coverage:** **[TBD - Calculate: `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | grep total | awk '{print $3}'`]** (Run `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`)
*   **Progress Summary:**
    *   Significant progress made since the last plan (previous overall: 63.7%).
    *   `cmd/irr` tests implemented (Phase 3 of the previous plan).
    *   High coverage achieved in `pkg/analysis`, `pkg/image`, `pkg/override`, `pkg/registry`, `pkg/rules`, `pkg/strategy`, `pkg/fileutil`, `pkg/log`, `pkg/debug`.
    *   `pkg/chart` coverage improved but still below target.
    *   Many utility packages (`pkg/generator`, `pkg/helm`, `pkg/testutil`) and command-line (`cmd/irr`) functions remain untested or have low coverage.
    *   **All Go unit tests (`make test`) are currently passing.**
    *   **All `make lint` checks are passing.**
*   **Coverage Breakdown (Current):**
    *   `github.com/lalbers/irr/cmd/irr`: **[Recalculate Needed - was 68.7%, but many 0% funcs added]**
    *   `github.com/lalbers/irr/internal/helm`: **[Needs Calculation - likely low]**
    *   `github.com/lalbers/irr/pkg/analysis`: **89.7%** (Good coverage)
    *   `github.com/lalbers/irr/pkg/analyzer`: **76.4%** (Meets minimum, 1 function at 0%)
    *   `github.com/lalbers/irr/pkg/chart`: **72.3%** (Below 75% target)
    *   `github.com/lalbers/irr/pkg/debug`: **88.1%** (Good coverage)
    *   `github.com/lalbers/irr/pkg/exitcodes`: **100%** (Excellent)
    *   `github.com/lalbers/irr/pkg/fileutil`: **98.5%** (Excellent)
    *   `github.com/lalbers/irr/pkg/generator`: **50.4%** (Below 75% target)
    *   `github.com/lalbers/irr/pkg/helm`: **64.0%** (Below 75% target)
    *   `github.com/lalbers/irr/pkg/image`: **82.6%** (Good coverage)
    *   `github.com/lalbers/irr/pkg/log`: **92.0%** (Excellent)
    *   `github.com/lalbers/irr/pkg/override`: **93.9%** (Excellent)
    *   `github.com/lalbers/irr/pkg/registry`: **81.5%** (Good coverage, few 0% funcs)
    *   `github.com/lalbers/irr/pkg/rules`: **95.2%** (Excellent, 1 function at 0%)
    *   `github.com/lalbers/irr/pkg/strategy`: **95.5%** (Excellent)
    *   `github.com/lalbers/irr/pkg/testutil`: **52.1%** (Below 75%, several 0% funcs)
    *   `github.com/lalbers/irr/pkg/version`: **100%** (Excellent)
    *   `github.com/lalbers/irr/test/integration`: **[Needs Calculation - was 72%, but many 0% funcs added]** (Go test harness only)
    *   `github.com/lalbers/irr/tools/lint/fileperm`: **0.0%**
*   **Next Priorities:**
    1.  Increase coverage in **`pkg/chart`**, **`pkg/generator`**, **`pkg/helm`**.
    2.  Address **0% coverage functions** in **`cmd/irr`** and **`internal/helm`**.
    3.  Improve coverage for **`pkg/testutil`**.
    4.  Address remaining 0% coverage functions in other packages.

## 3. Implementation Plan (Detailed)

*Prioritization:* Focus on core functionality, command execution paths, and functions currently at 0% coverage. Use table-driven tests extensively.

### Phase 1: Previous Core Logic - **[MOSTLY COMPLETE]**

*   Packages like `pkg/analysis`, `pkg/image`, `pkg/override`, `pkg/registry`, `pkg/strategy`, `pkg/rules` have good coverage. Minor 0% gaps remain (see Phase 4).
*   **Completion Criteria:** All packages from Phase 1 maintain ≥75% coverage.

### Phase 2: Address Below-Target Core Packages - **[IN PROGRESS]**

*   **Target Coverage:** >75% for each package.
*   **Completion Criteria:** All listed packages reach ≥75% coverage, with no critical functions remaining at 0%.
*   **Packages & Specific Actions:**
    *   **`pkg/chart` (`generator.go`, `loader.go`, `api.go`):** **[TODO - 72.3%]**
        *   **Priority 1 (Critical Path):**
            - [ ] `TestGenerate`: Review existing tests, enhance to cover untested logic paths identified by coverage reports (e.g., specific error handling, complex interactions).
            - [ ] `TestGenerateOverrides`: Enhance/add tests.
        *   **Priority 2 (Core Logic):**
            - [ ] `TestProcessChartForOverrides`: Enhance/add tests.
            - [ ] `TestMergeOverrides`: Enhance/add tests.
            - [ ] `generator.go: findValueByPath`: Add tests. **[0%]**
        *   **Priority 3 (Supporting Functions):**
            - [ ] `TestExtractSubtree`: Enhance/add tests.
            - [ ] `TestCleanupTemplateVariables`: Enhance/add tests.
            - [ ] `TestDefaultLoaderLoad`: Verify coverage.
            - [ ] `generator.go: Error/Unwrap`: Add tests for custom error types. **[0%]**
            - [ ] `api.go: NewLoader`: Add test. **[0%]**
    *   **`pkg/generator` (`generator.go`):** **[TODO - 50.4%]**
        *   **Priority 1:**
            - [ ] Review existing tests for `GenerateOverrides`.
        *   **Priority 2:**
            - [ ] `TestRemoveValueAtPath`: Add tests. **[0%]**
            - [ ] `TestNormalizeKubeStateMetricsOverrides`: Add tests for this specific logic. **[Low %]**
        *   **Priority 3:**
            - [ ] `TestSetValueAtPath`: Review coverage (currently high via `pkg/override` tests?).
            - [ ] `TestDeepCopy`: Review coverage.
    *   **`pkg/helm` (`client.go`, `sdk.go`):** **[TODO - 64.0%]** Requires significant mocking of Helm interactions.
        *   **Priority 1:**
            - [ ] `client.go: TestGetReleaseValues`: Add tests. **[Low %]**
            - [ ] `client.go: TestGetChartFromRelease`: Add tests. **[Low %]**
        *   **Priority 2:**
            - [ ] `client.go: TestGetReleaseMetadata`: Add tests. **[0%]**
            - [ ] `client.go: TestTemplateChart`: Add tests. **[0%]**
        *   **Priority 3:**
            - [ ] `client.go: TestGetHelmSettings`: Add tests. **[0%]**
            - [ ] `sdk.go: TestLoad`: Add tests. **[0%]**
            - [ ] `sdk.go: TestLoadChart`: Add tests. **[0%]**
    *   **`pkg/analyzer` (`analyzer.go`):** **[TODO - 76.4%]**
        *   **Priority 2:**
            - [ ] `TestAnalyzeInterfaceValue`: Add tests, likely complex involving recursion/type switching. **[0%]**
    *   **`pkg/testutil` (`testlogger.go`):** **[TODO - 52.1%]**
        *   **Priority 3:**
            - [ ] `TestUseTestLogger`: Add test. **[0%]**
            - [ ] `TestSuppressLogging`: Add test. **[0%]**
            - [ ] `TestCaptureLogging`: Add test. **[0%]**

### Phase 3: Address `cmd/irr` and `internal/helm` (0% Coverage Functions) - **[TODO]**

*   **Goal:** Significantly increase coverage by testing command execution paths, flag handling, and helper functions. Requires mocking filesystem (`afero`), Helm (`HelmClient` interface), and potentially `exec.Command`.
*   **Testing Strategy:** Prioritize black-box style tests using Cobra's `ExecuteCommandC` to verify end-to-end command behavior (flags, args, output). Write direct unit tests for complex *private* helper functions only if their logic is hard to exercise through the command interface or if needed to meet coverage targets. Consider refactoring highly complex private helpers into testable public functions in utility packages.
*   **Completion Criteria:** Achieve ≥60% coverage for `cmd/irr` and `internal/helm`, with no critical execution-path functions at 0%.
*   **Packages & Specific Actions:**
    *   **`cmd/irr/fileutil.go`:**
        *   **Priority 2:**
            - [ ] `TestDefaultHelmAdapterFactory`: Test creation. **[0%]**
            - [ ] `TestCreateHelmAdapter`: Test creation logic. **[0%]**
        *   **Priority 3:**
            - [ ] `TestGetCommandContext`: Test context setup. **[0%]**
    *   **`cmd/irr/helm.go`:**
        *   **Priority 1:**
            - [ ] `TestGetReleaseValues`: Test logic with mock Helm client. **[0%]**
        *   **Priority 2:**
            - [ ] `TestGetHelmSettings`: Test flag parsing/settings creation. **[0%]**
            - [ ] `TestGetReleaseNamespace`: Test logic. **[0%]**
            - [ ] `TestGetChartPathFromRelease`: Test logic with mock Helm client. **[0%]**
    *   **`cmd/irr/inspect.go`:** Test the `inspect` command logic end-to-end using `ExecuteCommandC`.
        *   **Priority 1:**
            - [ ] `TestRunInspect`: Core command execution flow. **[0%]**
            - [ ] `TestLoadHelmChart`: Core chart loading. **[0%]**
            - [ ] `TestAnalyzeChart`: Core chart analysis. **[0%]**
        *   **Priority 2:**
            - [ ] `TestSetupAnalyzerAndLoadChart`: Setup logic. **[0%]**
            - [ ] `TestInspectHelmRelease`: Helm release handling. **[0%]**
        *   **Priority 3:**
            - [ ] Test remaining helper functions: `filterImagesBySourceRegistries`, `extractUniqueRegistries`, `outputRegistrySuggestions`, `outputRegistryConfigSuggestion`, `getInspectFlags`, `getAnalysisPatterns`, `processImagePatterns`. Many of these will be tested indirectly via `TestRunInspect`. **[All 0%]**
    *   **`cmd/irr/main.go`:**
        *   **Priority 3:**
            - [ ] `TestMain`: Difficult to test directly, focus on testing `root.Execute`. **[0%]**
            - [ ] `TestLogHelmEnvironment`: Test logging helper. **[0%]**
    *   **`cmd/irr/override.go`:** Test the `override` command logic end-to-end using `ExecuteCommandC`.
        *   **Priority 1:**
            - [ ] `TestRunOverride` (or similar for the main execution path): Cover different flag combinations (chart path, release, stdin, stdout, files).
            - [ ] `TestCreateAndExecuteGenerator`: Test core generation logic. **[0%]**
            - [ ] `TestLoadChart`: Test chart loading. **[0%]**
        *   **Priority 2:**
            - [ ] `TestValidateChart`: Test validation logic. **[0%]**
            - [ ] `TestHandleHelmPluginOverride`: Test plugin integration. **[0%]**
            - [x] `TestValidateUnmappableRegistries`: Test registry validation. **[Low %]**
        *   **Priority 3:**
            - [x] Test other helper functions: `getStringFlag`, `outputOverrides`, `skipCWDCheck`, `isStdOutRequested`, `getReleaseNameAndNamespace`, `handlePluginOverrideOutput`, `validatePluginOverrides`. **[Many 0%]**
    *   **`cmd/irr/root.go`:**
        *   **Priority 1:**
            - [ ] `TestExecute`: Test main entry point. **[0%]**
        *   **Priority 3:**
            - [ ] `TestErrorExitCode`: Test custom error type methods. **[0%]**
    *   **`cmd/irr/validate.go`:** Test the `validate` command logic end-to-end.
        *   **Priority 1:**
            - [ ] `TestRunValidate`: Core execution flow.
        *   **Priority 2:**
            - [ ] `TestHandleHelmPluginValidate`: Test plugin validation. **[Low %]**
        *   **Priority 3:**
            - [ ] `TestHandleChartYamlMissingErrors`: Test error handling. **[0%]**
            - [ ] `TestFindChartInPossibleLocations`: Test chart location logic. **[0%]**
    *   **`internal/helm`:** Test adapter, client, and command wrappers. Requires mocking underlying Helm CLI calls or SDK interactions.
        *   **Priority 1:**
            - [ ] `client.go`: `TestGetReleaseValues`, `TestGetReleaseChart`. **[Low %]**
        *   **Priority 2:**
            - [ ] `client.go`: `TestNewHelmClient`, `TestTemplateChart`. **[0%]**
            - [ ] `command.go`: `TestCmdTemplate`, `TestCmdGetValues`. **[0%]**
        *   **Priority 3:**
            - [ ] `adapter.go`: `TestHandleChartYamlMissingWithSDK`. **[0%]**
            - [ ] `client.go`: `TestGetCurrentNamespace`, `TestFindChartForRelease`. **[0%]**
            - [ ] `client_mock.go`: Test mock implementation if complex (e.g., `TestMockGetCurrentNamespace`). **[0%]**

### Phase 4: Address Remaining 0% / Low Coverage & Utilities - **[TODO]**

*   **Completion Criteria:** No critical packages remaining below 75% coverage, reduce total 0% functions by at least 50%.
*   **Packages & Specific Actions:**
    *   **`pkg/registry` (`mappings.go`, `mappings_test_default.go`):**
        *   **Priority 2:**
            - [ ] `TestValidateLegacyMappings`: Add tests. **[0%]**
            - [ ] `TestLoadMappingsDefault`: Add tests. **[0%]**
        *   **Priority 3:**
            - [ ] Review and potentially enable/fix tests in `mappings_test_default.go`. **[0%]**
    *   **`pkg/rules` (`rule.go`):**
        *   **Priority 3:**
            - [ ] `TestRuleSetChart`: Add simple test. **[0%]**
    *   **`test/integration` (`harness.go`):** Test the test harness helper functions.
        *   **Testing Strategy:** Unit test non-trivial helper functions within `harness.go` (those with significant logic for setup, external interaction, parsing, complex assertions) to ensure harness reliability. Trivial helpers may be skipped.
        *   **Priority 2:**
            - [ ] `TestGenerateOverrides`, `TestValidateOverrides`. **[0%]**
            - [ ] `TestExecuteHelm`, `TestBuildIRR`. **[0%]**
        *   **Priority 3:**
            - [ ] Tests for remaining Validate* functions, `GetValueFromOverrides`, etc. **[Many 0%]**

### Phase 5: Continuous Improvement and Broader Testing - **[ONGOING]**

*   **(Process Improvements)**
*   **Specific Actions:**
    *   **CI Setup:**
        1.  CI runs `go test -race -coverprofile=coverage.out ./...`. **[VERIFIED]**
        2.  CI reports function coverage: `go tool cover -func=coverage.out`. **[VERIFIED]**
    *   **Regular Review:** Periodically review coverage reports (HTML) to identify new gaps. **[PROCESS]**
    *   **Chart Testing Script:** Python scripts (`test/tools/*.py`) are run periodically/manually. Analyze failures for potential unit test improvements. **[PROCESS DEFINED]**

## 4. Tools & Techniques

*   **Coverage Generation:** `go test -race -coverprofile=coverage.out ./...`
*   **Coverage Reporting (Function Level):** `go tool cover -func=coverage.out` (Use `| sort -k 3 -n` to find low coverage).
*   **Coverage Visualization (HTML):** `go tool cover -html=coverage.out`
*   **Testing Framework:** Standard Go `testing` package.
*   **Assertions:** `stretchr/testify/assert` and `stretchr/testify/require`.
*   **Mocking:**
    *   Standard Go interfaces (e.g., `HelmClient`, `ChartLoader`, `ChartGenerator`). **[APPLIED]**
    *   Function variables. **[APPLIED]**
    *   `afero` for filesystem. **[APPLIED]**
    *   Mock external commands (`exec.Command`) if direct calls are made. **[APPLIED]**
    *   `helm-unittest` plugin for chart-level testing. *(Out of Scope for this plan)*
*   **Test Structure:** Table-driven tests (`[]struct{...}`). **[APPLIED]**
*   **Fuzz Testing:** `go test -fuzz`. *(Out of Scope for this plan)*

### Additional Testing Tools & Practices

*   **Test Fixtures:** Use `testdata` directories. **[APPLIED]**
*   **Parameterized Tests (Table-Driven):** **[APPLIED]**
*   **Focused Mocks:** Mock only necessary dependencies for each test. **[APPLIED]**
*   **Cobra Testing:** Use `cmd.ExecuteCommandC` for testing command execution. **[TO APPLY in Phase 3]**

## 5. Tracking & Quality

*   **Primary Metrics:** Overall coverage percentage, per-package coverage percentage (especially for `pkg/*`), number of 0% coverage functions remaining.
*   **Target Metrics:** >=75% for core packages, >=75% overall (stretch), significantly reduce 0% functions.
*   **Tracking Process:**
    1.  Baseline established (see Section 2). **[DONE]**
    2.  Monitor via CI output/manual runs after implementing tests for each phase/package. **[PROCESS]**
    3.  Use HTML reports (`go tool cover -html=coverage.out`) to identify specific lines/branches needing tests within functions. **[PROCESS]**
*   **Qualitative Focus:** Ensure tests cover critical paths, common scenarios, error conditions, and flag interactions. **[APPLIED]**
*   **Test Quality Checklist:** Maintain readability, use clear assertions, avoid testing trivial code, ensure mocks are used correctly. **[APPLIED Implicitly]**
*   Maintain list of known coverage gaps. **[This Doc]**
*   **Code Exclusion:** Explicitly exclude code that doesn't require testing from coverage metrics using standard Go mechanisms (e.g., build tags like `//go:build !coverage` or comments recognized by coverage tools). This typically includes:
    *   Generated code (e.g., mocks, protobufs).
    *   The `main` function in `cmd/irr/main.go` (focus testing on `root.Execute`).
    *   Truly trivial wrapper functions that add no logic.
    Document exclusions for clarity.

## 6. Implementation Hints (Next Steps)

1.  **`pkg/chart` (Phase 2):** Focus on `generator.go` functions and error types first.
2.  **`pkg/generator` (Phase 2):** Add tests for `removeValueAtPath` and `normalizeKubeStateMetricsOverrides`.
3.  **`pkg/helm` (Phase 2):** Start by mocking the `HelmClient` interface and testing the functions in `client.go` and `sdk.go`.
4.  **`cmd/irr` (Phase 3):** Begin with simpler commands or helper functions (`fileutil.go`, `root.go`, parts of `helm.go`). Then tackle the main execution flows (`inspect.go`, `override.go`, `validate.go`) using `ExecuteCommandC`. Mock dependencies heavily.
5.  **`internal/helm` (Phase 3):** Test client and command wrappers, likely mocking Helm CLI/SDK calls.
6.  **Utilities (Phase 2 & 4):** Address `pkg/testutil`, `pkg/analyzer`, `pkg/registry`, `pkg/rules` gaps.
7.  **Integration Harness (Phase 4):** Test the non-trivial test helpers themselves.

## 7. Handling Difficult-to-Test Functions

When encountering functions that are particularly challenging to test:

1. **Initial Attempt:** Make a first attempt to test the function, identifying specific challenges (e.g., external dependencies, complex control flow, numerous side effects).

2. **Second Attempt:** Try a different approach based on lessons from the first attempt (e.g., different mocking strategy, refactoring test setup).

3. **Mark and Move On:** If still unsuccessful after two attempts:
   * Document the specific challenges in a comment (e.g., `// TODO(test): Function X is difficult to test because...`).
   * Record current coverage percentage for the function.
   * Move on to an easier task to maintain momentum.
   
4. **Circle Back:** Return to difficult functions after making progress elsewhere, possibly with:
   * Fresh insights from testing related code
   * Potential for minor refactoring to improve testability
   * More experience with the codebase's testing patterns

5. **Refactoring Consideration:** If a function proves particularly resistant to testing, consider whether minor refactoring could improve testability:
   * Extracting pure logic from side effects
   * Adding interfaces for external dependencies
   * Breaking complex functions into smaller, more testable units

This approach ensures steady progress while pragmatically handling challenging cases.
