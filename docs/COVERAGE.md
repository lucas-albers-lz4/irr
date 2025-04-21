# Test Coverage Improvement Plan

Based on the output from `go tool cover -func=coverage.out`, several functions currently have 0% test coverage. This plan outlines a strategy to increase coverage, focusing on critical areas and leveraging existing testing frameworks.

## General Strategy

1.  **Prioritize**: Focus on increasing coverage for core logic (`pkg/...`), Helm interaction components (`internal/helm/`, `pkg/helm/`), and CLI command execution (`cmd/irr/...`). Utility functions and internal tooling can be addressed afterwards.
2.  **Leverage Existing Frameworks**: Utilize the established testing patterns outlined in `docs/TESTING.md`:
    *   **Unit Tests**: For isolated logic, utility functions, and components testable with mocks (e.g., using `testify/mock` for Helm interactions, `afero` for filesystem operations). Ensure error paths are explicitly tested.
    *   **Integration Tests (`test/integration`)**: Use the existing test harness (`test/integration/harness.go`) to test the end-to-end behavior of CLI commands (`irr inspect`, `irr override`, `irr validate`). These tests are crucial for covering the `cmd/irr/` package functions and their interaction with other components.
3.  **Targeted Testing**:
    *   **CLI (`cmd/irr/...`)**: Add integration tests that invoke specific commands (`inspect`, `override`, `validate`) with various flags and arguments to cover functions like `runInspect`, `createAndExecuteGenerator`, `handleValidateOutput`, flag parsing helpers (`getInspectFlags`), and error handlers (`handleGenerateError`). Test both standalone and Helm plugin execution modes where applicable.
    *   **Helm Interaction (`internal/helm`, `pkg/helm`)**: Enhance unit tests using mocks (like `internal/helm/client_mock.go`) to cover adapter logic (`internal/helm/adapter.go`). Ensure the mock setup functions themselves are used in tests. For `internal/helm/client.go`, integration tests might implicitly cover some functions, but dedicated unit tests with mocks for Helm SDK calls are preferable where feasible. Test functions like `GetReleaseValues`, `FindChartForRelease`, `TemplateChart`.
    *   **Core Logic (`pkg/...`)**: Add unit tests for uncovered functions in `pkg/analyzer`, `pkg/chart/generator`, `pkg/rules`.
    *   **Utilities (`pkg/fileutil`, `pkg/log`, `pkg/testutil`)**: Ensure utility functions are covered, primarily through unit tests (`afero` for `fileutil`). Verify test helpers (`pkg/testutil`, `mappings_test_default.go`) are correctly used by existing or new tests.
    *   **Main (`cmd/irr/main.go`)**: Coverage for `main` and `Execute` will primarily come from comprehensive integration tests exercising the CLI entry points. Test `logHelmEnvironment` by setting environment variables and capturing logs in specific tests.
4.  **Iterative Improvement**: Address low-coverage files incrementally. After adding tests for a specific area, regenerate the coverage report (`go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | sort -k 3 -n`) to track progress and identify the next targets.
5.  **Focus on 0%**: Initially, prioritize functions listed with exactly 0.0% coverage. Functions with low-but-non-zero coverage (like `AssertErrorContains`, `GetValues`) indicate they are partially tested, but might need additional test cases to cover more branches or scenarios.

## Specific Testing Strategies

### `internal/helm/adapter.go`
- **Functions to Focus On**: `InspectRelease`, `OverrideRelease`, `ValidateRelease`.
- **Testing Strategy**:
  - **Unit Tests**: Mock the `helmClient` to simulate different scenarios like successful retrieval of release values, chart metadata, and error conditions.
  - **Integration Tests**: Test the interaction with actual Helm releases if possible, or use a mock Helm environment.
  - **Edge Cases**: Test with non-existent releases, invalid namespaces, and unsupported image structures.

### `cmd/irr/inspect.go`
- **Functions to Focus On**: `runInspect`, `inspectHelmRelease`.
- **Testing Strategy**:
  - **Unit Tests**: Use mock implementations for Helm client interactions. Test different flag combinations and output formats.
  - **Integration Tests**: Validate the command's behavior with real Helm charts and releases.
  - **Edge Cases**: Test with missing chart paths, invalid output formats, and non-existent releases.

### `cmd/irr/override.go`
- **Functions to Focus On**: `runOverride`, `setupGeneratorConfig`.
- **Testing Strategy**:
  - **Unit Tests**: Mock the file system and Helm client. Test various flag combinations and error conditions.
  - **Integration Tests**: Test the command with actual Helm charts to ensure correct override generation.
  - **Edge Cases**: Test with missing required flags, invalid registry URLs, and empty source registries.

## Notes on Afero and Logging

- **Afero Usage**: Use `afero.NewMemMapFs()` for in-memory file system operations in tests to ensure isolation and avoid side effects. This approach is consistent with our current testing practices and allows for easy setup and teardown of test environments.

- **Logging Practices**: Align new tests with our logging strategy by using appropriate log levels:
  - `DEBUG` for detailed troubleshooting information.
  - `INFO` for general operational messages.
  - `WARN` for potential issues that don't prevent operation.
  - `ERROR` for serious issues that prevent operation.
  - Ensure that debug logs are enabled during test execution to capture detailed information about test failures and execution paths.

## Next Steps

*   Begin by adding integration tests for the main command flows (`inspect`, `override`, `validate`) to cover the corresponding `run...`, `handle...`, and `createAndExecute...` functions in `cmd/irr/`.
*   Add unit tests with mocks for the `internal/helm/adapter.go` functions.
*   Review and add unit tests for uncovered core logic in `pkg/`.
*   Address utility and helper function coverage.

## Additional Testing and Logging Strategies

### Testing Strategies
- **Unit and Focused Tests**: Target deterministic functions with clear input/output relationships. Use outcome-focused tests for heuristic-based logic.
- **Integration & Chart Validation Tests**: Validate end-to-end behavior using real Helm charts. Ensure correct image relocation, version preservation, and non-destructive changes.
- **Image Relocation Validation**: Use regex patterns to validate image URI transformations.
- **Version Preservation Check**: Ensure strict version/tag/digest preservation.
- **Non-Destructive Change Verification**: Verify no unintended changes in `values.yaml`.
- **Path Strategy Testing**: Test each supported path strategy with various registry patterns.
- **Subchart and Complex Structure Testing**: Verify correct override path generation using dependency aliases.

### Logging Strategies
- **Log Levels**: Use appropriate log levels (`DEBUG`, `INFO`, `WARN`, `ERROR`) for different types of messages.
- **Debug Logging**: Enable debug logging using command-line flags or environment variables. Ensure debug logs are captured during tests.
- **Execution Mode Detection**: Confirm execution mode (Standalone vs Helm Plugin) and log accordingly.
- **Debug Output Format**: Follow a consistent format for debug logs to help identify the source and timing of messages.
- **Troubleshooting**: Use environment variables to capture verbose information for troubleshooting.
- **Testing Logging/Debugging**: Test debug logging in unit and integration tests to ensure proper log capture and message format.

## Recommended Order for Increasing Test Coverage

To maximize impact and efficiency, follow this order when working through files to increase test coverage:

## REMINDER On the Implementation Process: (DONT REMOVE THIS SECTION)
- For each change:
  1. **Baseline Verification:**
     - Run full test suite: `go test ./...` ✓
     - Run full linting: `golangci-lint run` ✓
     - Run custom nil lint check: `sh tools/lint/nilaway/lint-nilaway.sh ` ✓
     - Determine if any existing failures need to be fixed before proceeding with new feature work ✓
  
  2. **Pre-Change Verification:**
     - Run targeted tests relevant to the component being modified (e.g., `go test -v ./test/integration -run TestComplexChartFeatures/ingress-nginx_with_admission_webhook` and if you need or debug output call with IRR_DEBUG=1 , `IRR_DEBUG=1 go test -v ./test/integration -run TestComplexChartFeatures/ingress-nginx_with_admission_webhook`✓
     - Run targeted linting to identify specific issues (e.g., `golangci-lint run --enable-only=unused` for unused variables) ✓
  
  3. **Make Required Changes:**
     - Follow KISS and YAGNI principles ✓
     - Maintain consistent code style ✓
     - Document changes in code comments where appropriate ✓
     - **For filesystem mocking changes:**
       - Implement changes package by package following the guidelines in `docs/TESTING-FILESYSTEM-MOCKING.md`
       - Start with simpler packages before tackling complex ones
       - Always provide test helpers for swapping the filesystem implementation
       - Run tests frequently to catch issues early
  
  4. **Post-Change Verification:**
     - Run targeted tests to verify the changes work as expected ✓
     - Run targeted linting to confirm specific issues are resolved ✓
     - Run full test suite: `go test ./...` ✓
     - Run full linting: `golangci-lint run` ✓


## 1. Goal and Scope

*   **Primary Goal:** Pragmatically increase Go test coverage to **at least 75%** for all core packages (`pkg/*`), and significantly reduce the number of functions with 0% coverage across the codebase. Aim for **>75% overall coverage** as a stretch goal.
*   **Immediate Focus (Address Gaps):** Prioritize packages currently below 75% (`pkg/chart`, `pkg/generator`, `pkg/helm`, `pkg/analyzer`, `pkg/testutil`) and functions with 0% coverage, particularly within `cmd/irr` and `internal/helm` due to their critical role in execution flow.
*   **Subsequent Phases:** Target higher coverage (>85%) for core packages, address utility packages more thoroughly, test edge cases identified during broader chart testing, and refine tests as the codebase evolves.

## 2. Current Status (as of [Insert Date - e.g., 2024-08-XX])

*   **Overall Coverage:** **61.9%** (Run `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`)
*   **Progress Summary:**
    *   Significant progress made since the last plan (previous overall: 63.7%).
    *   `cmd/irr` tests implemented (Phase 3 of the previous plan).
    *   High coverage achieved in `pkg/analysis`, `pkg/image`, `pkg/override`, `pkg/registry`, `pkg/rules`, `pkg/strategy`, `pkg/fileutil`, `pkg/log`, `pkg/debug`.
    *   `pkg/chart` coverage improved but still below target.
    *   Many utility packages (`pkg/generator`, `pkg/helm`, `pkg/testutil`) and command-line (`cmd/irr`) functions remain untested or have low coverage.
    *   **All Go unit tests (`make test`) are currently passing.**
    *   **All `make lint` checks are passing.**
*   **Coverage Breakdown (Current):**
    *   `github.com/lalbers/irr/cmd/irr`: **47.2%**
    *   `github.com/lalbers/irr/internal/helm`: **37.2%**
    *   `github.com/lalbers/irr/pkg/analysis`: **88.4%**
    *   `github.com/lalbers/irr/pkg/analyzer`: **70.9%**
    *   `github.com/lalbers/irr/pkg/chart`: **73.0%**
    *   `github.com/lalbers/irr/pkg/debug`: **78.9%**
    *   `github.com/lalbers/irr/pkg/exitcodes`: **100.0%**
    *   `github.com/lalbers/irr/pkg/fileutil`: **97.2%**
    *   `github.com/lalbers/irr/pkg/generator`: **85.4%**
    *   `github.com/lalbers/irr/pkg/helm`: **55.0%**
    *   `github.com/lalbers/irr/pkg/image`: **75.5%**
    *   `github.com/lalbers/irr/pkg/log`: **86.2%**
    *   `github.com/lalbers/irr/pkg/override`: **84.5%**
    *   `github.com/lalbers/irr/pkg/registry`: **67.0%**
    *   `github.com/lalbers/irr/pkg/rules`: **97.7%**
    *   `github.com/lalbers/irr/pkg/strategy`: **90.9%**
    *   `github.com/lalbers/irr/pkg/testutil`: **83.5%**
    *   `github.com/lalbers/irr/pkg/version`: **100.0%**
    *   `github.com/lalbers/irr/test/integration`: **42.8%**
    *   `github.com/lalbers/irr/tools/lint/fileperm`: **0.0%**
    *   `github.com/lalbers/irr/tools/lint/fileperm/cmd`: **0.0%**
*   **Next Priorities:**
    1.  Fix failing integration tests in `test/integration`.
    2.  Increase coverage in **`pkg/chart`**, **`pkg/generator`**, **`pkg/helm`**, **`cmd/irr`**, **`internal/helm`**, **`pkg/testutil`**.
    3.  Address **0% coverage functions** in **`cmd/irr`** and **`internal/helm`**.
    4.  Improve coverage for **`pkg/analyzer`** (`analyzeInterfaceValue`).
    5.  Address remaining 0% coverage functions in other packages (`pkg/registry`, `pkg/rules`).

## 3. Implementation Plan (Detailed)

*Prioritization:* Focus on core functionality, command execution paths, and functions currently at 0% coverage. Use table-driven tests extensively.

### Phase 1: Previous Core Logic - **[MOSTLY COMPLETE]**

*   Packages like `pkg/analysis`, `pkg/image`, `pkg/override`, `pkg/registry`, `pkg/strategy`, `pkg/rules` have good coverage. Minor 0% gaps remain (see Phase 4).
*   **Completion Criteria:** All packages from Phase 1 maintain ≥75% coverage.

### Phase 2: Address Below-Target Core Packages - **[IN PROGRESS]**

*   **Target Coverage:** >75% for each package.
*   **Completion Criteria:** All listed packages reach ≥75% coverage, with no critical functions remaining at 0%.
*   **Packages & Specific Actions:**
    *   **`pkg/chart` (`generator.go`, `loader.go`, `api.go`):** **[IN PROGRESS - 73.0%]** (Target almost met)
        *   **Priority 1 (Critical Path):**
            - [x] `TestGenerate`: Review existing tests, enhance to cover untested logic paths identified by coverage reports (e.g., specific error handling, complex interactions). **[Partially Addressed by Refactor/Linting, Coverage OK]**
            - [x] `TestGenerateOverrides`: Enhance/add tests. **[Coverage OK]**
        *   **Priority 2 (Core Logic):**
            - [x] `TestProcessChartForOverrides`: Enhance/add tests. **[Coverage OK]**
            - [x] `TestMergeOverrides`: Enhance/add tests. **[Coverage OK]**
            - [x] `generator.go: findValueByPath`: Add tests. **[DONE - 92.9%]**
        *   **Priority 3 (Supporting Functions):**
            - [x] `TestExtractSubtree`: Enhance/add tests. **[Coverage OK]**
            - [x] `TestCleanupTemplateVariables`: Enhance/add tests. **[Coverage OK]**
            - [x] `TestDefaultLoaderLoad`: Verify coverage. **[Coverage OK]**
            - [x] `generator.go: Error/Unwrap`: Add tests for custom error types. **[DONE - 100%]**
            - [x] `api.go: NewLoader`: Add test. **[DONE - 100%]**
    *   **`pkg/generator` (`generator.go`):** **[DONE - 85.4%]** (Target met)
        *   **Priority 1:**
            - [x] Review existing tests for `GenerateOverrides`. **[Coverage OK - 75.0%]**
        *   **Priority 2:**
            - [x] `TestRemoveValueAtPath`: Add tests. **[DONE - 100%]**
            - [x] `TestNormalizeKubeStateMetricsOverrides`: Add tests for this specific logic. **[DONE - 88.2%]**
        *   **Priority 3:**
            - [x] `TestSetValueAtPath`: Review coverage. **[Coverage OK - 78.3% via pkg/override]**
            - [x] `TestDeepCopy`: Review coverage. **[Coverage OK - 90.9% via pkg/override]**
    *   **`pkg/helm` (`client.go`, `sdk.go`):** **[TODO - 55.0%]** (Requires significant mocking of Helm interactions)
        *   **Priority 1:**
            - [ ] `client.go: TestGetReleaseValues`: Add tests. **[18.2%]**
            - [ ] `client.go: TestGetChartFromRelease`: Add tests. **[20.0%]**
        *   **Priority 2:**
            - [ ] `client.go: TestGetReleaseMetadata`: Add tests. **[0%]**
            - [ ] `client.go: TestTemplateChart`: Add tests. **[0%]**
        *   **Priority 3:**
            - [x] `client.go: TestGetHelmSettings`: Add tests. **[DONE - 100%]**
            - [ ] `sdk.go: TestLoad`: Add tests. **[0%]**
            - [ ] `sdk.go: TestLoadChart`: Add tests. **[0%]**
    *   **`pkg/analyzer` (`analyzer.go`):** **[TODO - 70.9%]** (Target not met)
        *   **Priority 2:**
            - [ ] `TestAnalyzeInterfaceValue`: Add tests, likely complex involving recursion/type switching. **[0%]**
    *   **`pkg/testutil` (`testlogger.go`):** **[DONE - 83.5%]** (Target met)
        *   **Priority 3:**
            - [x] `TestUseTestLogger`: Add test. **[DONE - 75.0%]**
            - [x] `TestSuppressLogging`: Add test. **[DONE - 88.2%]**
            - [x] `TestCaptureLogging`: Add test. **[DONE - 83.3%]**

### Phase 3: Address `cmd/irr` and `internal/helm` (0% Coverage Functions) - **[IN PROGRESS]**

*   **Goal:** Significantly increase coverage by testing command execution paths, flag handling, and helper functions. Requires mocking filesystem (`afero`), Helm (`HelmClient` interface), and potentially `exec.Command`.
*   **Testing Strategy:** Prioritize black-box style tests using Cobra's `ExecuteCommandC` to verify end-to-end command behavior (flags, args, output). Write direct unit tests for complex *private* helper functions only if their logic is hard to exercise through the command interface or if needed to meet coverage targets. Consider refactoring highly complex private helpers into testable public functions in utility packages.
*   **Completion Criteria:** Achieve ≥60% coverage for `cmd/irr` and `internal/helm`, with no critical execution-path functions at 0%.
*   **Packages & Specific Actions:**
    *   **`cmd/irr/fileutil.go`:**
        *   **Priority 2:**
            - [ ] `TestDefaultHelmAdapterFactory`: Test creation. **[0%]**
            - [x] `TestCreateHelmAdapter`: Test creation logic. **[DONE - 100%]**
        *   **Priority 3:**
            - [x] `TestGetCommandContext`: Test context setup. **[DONE - 100%]**
    *   **`cmd/irr/helm.go`:**
        *   **Priority 1:**
            - [x] `TestGetReleaseValues`: Test logic with mock Helm client. **[IN PROGRESS - 16.7%]**
        *   **Priority 2:**
            - [ ] `TestGetHelmSettings`: Test flag parsing/settings creation. **[0%]**
            - [x] `TestGetReleaseNamespace`: Test logic. **[DONE - 100%]**
            - [x] `TestGetChartPathFromRelease`: Test logic with mock Helm client. **[IN PROGRESS - 6.5%]**
    *   **`cmd/irr/inspect.go`:** Test the `inspect` command logic end-to-end using `ExecuteCommandC`.
        *   **Priority 1:**
            - [x] `TestRunInspect`: Core command execution flow. **[IN PROGRESS - 59.5%]**
            - [x] `TestLoadHelmChart`: Core chart loading. **[IN PROGRESS - 21.9%]**
            - [x] `TestAnalyzeChart`: Core chart analysis. **[DONE - 85.7%]**
        *   **Priority 2:**
            - [x] `TestSetupAnalyzerAndLoadChart`: Setup logic. **[IN PROGRESS - 56.5%]**
            - [ ] `TestInspectHelmRelease`: Helm release handling. **[0%]**
        *   **Priority 3:**
            - [x] Test remaining helper functions: `filterImagesBySourceRegistries` **(90.9%)**, `extractUniqueRegistries` **(100%)**, `outputRegistrySuggestions` **(100%)**, `outputRegistryConfigSuggestion` **(100%)**, `getInspectFlags` **(71.9%)**, `getAnalysisPatterns` **(71.4%)**, `processImagePatterns` **(76.2%)**. Many of these will be tested indirectly via `TestRunInspect`. **[Mostly DONE]**
    *   **`cmd/irr/main.go`:**
        *   **Priority 3:**
            - [ ] `TestMain`: Difficult to test directly, focus on testing `root.Execute`. **[0%]**
            - [x] `TestLogHelmEnvironment`: Test logging helper. **[DONE - 100%]**
    *   **`cmd/irr/override.go`:** Test the `override` command logic end-to-end using `ExecuteCommandC`.
        *   **Priority 1:**
            - [x] `TestRunOverride` (or similar for the main execution path): Cover different flag combinations (chart path, release, stdin, stdout, files). **[IN PROGRESS - 33.3%]**
            - [ ] `TestCreateAndExecuteGenerator`: Test core generation logic. **[0%]**
            - [ ] `TestLoadChart`: Test chart loading. **[0%]**
        *   **Priority 2:**
            - [ ] `TestValidateChart`: Test validation logic. **[0%]**
            - [x] `TestHandleHelmPluginOverride`: Test plugin integration. **[DONE - 80.0%]**
            - [x] `TestValidateUnmappableRegistries`: Test registry validation. **[IN PROGRESS - 28.2%]**
        *   **Priority 3:**
            - [ ] `TestGetStringFlag`: **[0%]**
            - [ ] `TestOutputOverrides`: **[0%]**
            - [ ] `TestSkipCWDCheck`: **[0%]**
            - [ ] `TestIsStdOutRequested`: **[0%]**
            - [x] `TestGetReleaseNameAndNamespace`: **[DONE - 100%]**
            - [x] `TestHandlePluginOverrideOutput`: **[IN PROGRESS - 35.7%]**
            - [x] `TestValidatePluginOverrides`: **[DONE - 72.7%]**
    *   **`cmd/irr/root.go`:**
        *   **Priority 1:**
            - [ ] `TestExecute`: Test main entry point. **[0%]**
        *   **Priority 3:**
            - [ ] `TestErrorExitCode`: Test custom error type methods (Error, ExitCode). **[0%]**
    *   **`cmd/irr/validate.go`:** Test the `validate` command logic end-to-end.
        *   **Priority 1:**
            - [x] `TestRunValidate`: Core execution flow. **[IN PROGRESS - 70.0%]**
        *   **Priority 2:**
            - [ ] `TestHandleHelmPluginValidate`: Test plugin validation. **[0%]** (Was previously Low %)
        *   **Priority 3:**
            - [ ] `TestHandleChartYamlMissingErrors`: Test error handling. **[0%]**
            - [ ] `TestFindChartInPossibleLocations`: Test chart location logic. **[0%]**
    *   **`internal/helm`:** Test adapter, client, and command wrappers. Requires mocking underlying Helm CLI calls or SDK interactions.
        *   **Priority 1:**
            - [ ] `client.go`: `TestGetReleaseValues`. **[0%]**
            - [ ] `client.go`: `TestGetReleaseChart`. **[0%]**
        *   **Priority 2:**
            - [ ] `client.go`: `TestNewHelmClient`. **[0%]**
            - [ ] `client.go`: `TestTemplateChart`. **[0%]**
            - [ ] `command.go`: `TestCmdTemplate`. **[0%]**
            - [ ] `command.go`: `TestCmdGetValues`. **[0%]**
        *   **Priority 3:**
            - [ ] `adapter.go`: `TestHandleChartYamlMissingWithSDK`. **[0%]**
            - [ ] `client.go`: `TestGetCurrentNamespace`. **[0%]**
            - [ ] `client.go`: `TestFindChartForRelease`. **[0%]**
            - [ ] `client_mock.go`: Test mock implementation if complex (e.g., `TestMockGetCurrentNamespace`). **[0%]**

### Phase 4: Address Remaining 0% / Low Coverage & Utilities - **[TODO]**
