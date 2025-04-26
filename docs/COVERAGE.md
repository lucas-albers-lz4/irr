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
     - Run full test suite: `make test-quiet` ✓
     - Run full linting: `make lint` ✓
     - Run custom nil lint check: `sh tools/lint/nilaway/lint-nilaway.sh ` ✓
     - Determine if any existing failures need to be fixed before proceeding with new feature work ✓
  
  2. **Pre-Change Verification:**
     - Run targeted tests relevant to the component being modified (e.g., `go test -v ./test/integration -run TestComplexChartFeatures/ingress-nginx_with_admission_webhook` and if you need debug output call with `LOG_LEVEL=DEBUG`, `LOG_LEVEL=DEBUG go test -v ./test/integration -run TestComplexChartFeatures/ingress-nginx_with_admission_webhook`)✓
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
     - Run full test suite: `make test-quiet` ✓
     - Run full linting: `make lint` ✓


## 1. Goal and Scope

*   **Primary Goal:** Pragmatically increase Go test coverage to **at least 75%** for all core packages (`pkg/*`), and significantly reduce the number of functions with 0% coverage across the codebase. Aim for **>75% overall coverage** as a stretch goal.
    *   *Note:* While percentages are useful targets, the focus should be on writing *meaningful* tests covering important logic, error conditions, and edge cases, rather than simply inflating numbers with trivial tests.
*   **Immediate Focus (Address Gaps):** Prioritize packages currently below 75% (`internal/helm`, `cmd/irr`, `pkg/helm`, `pkg/registry`, `pkg/analyzer`, `test/integration`) and functions with 0% coverage, particularly within `cmd/irr` and `internal/helm`.
*   **Subsequent Phases:** Target higher coverage (**>85%**) for core *logic* packages (`pkg/analysis`, `pkg/override`, `pkg/generator`, `pkg/image`, `pkg/rules`, `pkg/strategy`) once the initial 75% goal is met broadly. Address utility packages more thoroughly, test edge cases identified during broader chart testing, and refine tests as the codebase evolves. Add low-priority, long-term coverage goals (e.g., >30-50%) for test helper packages (`test/mocks`, `test/integration/harness.go`).

## 2. Current Status (as of [Current Date] -> Updated)

*   **Overall Coverage:** **66.8%** (Run `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`)
*   **Progress Summary:**
    *   Overall coverage increased from 66.6% to **66.8%**.
    *   `cmd/irr` coverage remains at **54.1%**.
    *   `internal/helm` coverage remains at **54.5%**.
    *   `pkg/helm` coverage remains at **73.4%** (approaching target!).
    *   `pkg/analyzer` coverage increased from 70.6% to **77.0%** (Target Met!).
    *   `pkg/log` coverage remains at **95.7%** (Target Exceeded).
    *   `pkg/registry` coverage remains at **79.6%** (Target Met!).
    *   `pkg/chart` (75.3%) and `pkg/image` (75.3%) still meet the 75% target.
    *   `internal/helm` (54.5%), `test/integration` (41.5%) are still the lowest covered packages requiring the most attention, followed by `cmd/irr` (54.1%).
    *   Many 0% functions remain, especially in `cmd/irr`, `internal/helm`, `test/integration`.
    *   `test/mocks` and `tools/lint/...` remain at 0%.
    *   **All Go unit tests (`make test`) are currently passing.**
    *   **All `make lint` checks are passing.**
*   **Coverage Breakdown (Current - Sorted Lowest to Highest Priority):**
    *   `github.com/lalbers/irr/test/mocks`: **0.0%** (Low Priority - Test Helpers)
    *   `github.com/lalbers/irr/tools/lint/fileperm`: **0.0%** (Low Priority Tooling)
    *   `github.com/lalbers/irr/tools/lint/fileperm/cmd`: **0.0%** (Low Priority Tooling)
    *   `github.com/lalbers/irr/test/integration`: **41.5%** (High Priority)
    *   `github.com/lalbers/irr/cmd/irr`: **54.1%** (High Priority)
    *   `github.com/lalbers/irr/internal/helm`: **54.5%** (High Priority)
    *   `github.com/lalbers/irr/pkg/helm`: **73.4%** (Medium Priority - Approaching Target!)
    *   `github.com/lalbers/irr/pkg/chart`: **75.3%** (Target Met)
    *   `github.com/lalbers/irr/pkg/image`: **75.3%** (Target Met)
    *   `github.com/lalbers/irr/pkg/analyzer`: **77.0%** (Target Met!)
    *   `github.com/lalbers/irr/pkg/registry`: **79.6%** (Target Met)
    *   `github.com/lalbers/irr/pkg/testutil`: **88.1%** (Target Met)
    *   `github.com/lalbers/irr/pkg/generator`: **84.6%** (Target Met)
    *   `github.com/lalbers/irr/pkg/override`: **85.9%** (Target Met)
    *   `github.com/lalbers/irr/pkg/analysis`: **89.3%** (Target Met)
    *   `github.com/lalbers/irr/pkg/strategy`: **90.9%** (Target Met)
    *   `github.com/lalbers/irr/pkg/fileutil`: **94.4%** (Target Met)
    *   `github.com/lalbers/irr/pkg/log`: **95.7%** (Target Met - Exceeded!)
    *   `github.com/lalbers/irr/pkg/rules`: **97.7%** (Target Met)
    *   `github.com/lalbers/irr/pkg/exitcodes`: **100.0%** (Target Met)
    *   `github.com/lalbers/irr/pkg/version`: **100.0%** (Target Met)
*   **Next Priorities:**
    1.  Increase coverage in **`internal/helm`** (critical helper package, 54.5%). Focus on 0% functions (`GetReleaseValues`, `GetReleaseChart`, `TemplateChart`, `command.go: Template`, `command.go: GetValues`).
    2.  Increase coverage in **`cmd/irr`** (CLI entry points, 54.1%). Focus on 0% functions (e.g., `main`, `Execute`, `inspectHelmRelease`, `getRequiredFlags`, `setupGeneratorConfig`, `createAndExecuteGenerator`, `createGenerator`, `runOverrideStandaloneMode`, multiple functions in `validate.go`, `initConfig`). **Prioritize command-level (black-box) testing and Helm plugin mode testing.**
    3.  **Concurrently with #1/#2:** Begin adding *new*, targeted integration tests (`test/integration` - 41.5%) for core `cmd/irr` command scenarios, especially Helm mode.
    4.  Finish the work on **`pkg/helm`** (Helm client interactions, 73.4%, nearly at target). Focus on 0% functions (`GetReleaseMetadata`, `TemplateChart`, `sdk.go: Load`, `sdk.go: LoadChart`).
    5.  ~~Increase coverage in **`pkg/analyzer`** (70.6%). Focus on `analyzeInterfaceValue` (0%).~~ **[DONE - 77.0%]**
    6.  Address remaining 0% coverage functions in other packages.
    7.  (Lower Priority) Improve coverage for `test/integration` beyond adding core command tests (address 0% functions like `loadMappings`, etc.).
    8.  (Lowest Priority) Add tests for `test/mocks` and `tools/lint/...`.

## 3. Implementation Plan (Detailed)

### Phase 1: Previous Core Logic - **[MOSTLY COMPLETE]**

*   Packages like `pkg/analysis`, `pkg/image`, `pkg/override`, `pkg/strategy`, `pkg/rules` have good coverage. Minor 0% gaps remain (see Phase 4).
*   **Completion Criteria:** All packages from Phase 1 maintain ≥75% coverage.

### Phase 2: Address Below-Target Core Packages & Regressions - **[IN PROGRESS]**

*   **Target Coverage:** >75% for each package.
*   **Completion Criteria:** All listed packages reach ≥75% coverage, with no critical functions remaining at 0%.
*   **Packages & Specific Actions:** (Reordered by Priority based on Current Status)
    *   **`pkg/log` (`log.go`):** **[COMPLETE - 95.7%]** (Target Exceeded)
        *   ~~**Priority 1 (Core Logging Functions - 0% - URGENT):**~~
            - [X] `TestSetOutput`: Added test.
            - [X] `TestDebug`: Covered via other tests.
            - [X] `TestInfo`: Covered via other tests.
            - [X] `TestWarn`: Covered via other tests.
            - [X] `TestError`: Covered via other tests.
            - [X] `TestLogger`: Added test.
            - [X] `TestString`: Added test.
            - [X] `TestSetTestModeWithTimestamps`: Added test.
    *   **`pkg/helm` (`client.go`, `sdk.go`):** **[TODO - 63.3%]** (Medium Priority)
        *   **Priority 1 (Core Client Interaction - 0%/Low):**
            - [ ] `client.go: TestGetReleaseMetadata`: Add tests. **[0%]**
            - [ ] `client.go: TestTemplateChart`: Add tests. **[0%]**
            - [ ] `client.go: TestGetReleaseValues`: Add more tests. **[18.2%]**
            - [ ] `client.go: TestGetChartFromRelease`: Add more tests. **[20.0%]**
        *   **Priority 2 (SDK Abstraction - 0%/Low):**
            - [ ] `sdk.go: TestLoad`: Add tests. **[0%]**
            - [ ] `sdk.go: TestLoadChart`: Add tests. **[0%]**
            - [ ] `sdk.go: TestDiscoverPlugins`: Add more tests. **[80.0%]**
        *   **Priority 3 (Repo Management - Existing Coverage OK):**
            - Review `repo.go` functions if needed.
    *   **`pkg/registry` (`mappings.go`, `config.go`, `mappings_test_default.go`):** **[TARGET MET - 79.6%]** (Lower Priority - Enhance if needed)
        *   **NOTE:** Tests related to *legacy* mapping formats (e.g., `TestValidateLegacyMappings`, `TestRegistryMappingFileFormats`) should be skipped, as this functionality is planned for removal. Focus on structured mappings (`TestValidateStructuredMappings`, `LoadMappingsDefault` with structured data).
        *   **Priority 1 (Core Logic - 0%/Low):**
            - [ ] `mappings.go: TestValidateLegacyMappings`: ~~Add tests~~ **[0%]**  <- SKIP (Legacy)
            - [ ] `mappings.go: TestLoadMappingsDefault`: Add tests (depends on `mappings_test_default.go`). **[0%]**
            - [ ] `mappings.go: TestValidateStructuredMappings`: Add more tests. **[54.2%]**
        *   **Priority 2 (Test Helpers - 0%):**
            - [ ] `mappings_test_default.go: TestLoadMappingsDefault`: Implement actual test logic. **[0%]**
            - [ ] `mappings_test_default.go: TestLoadMappingsWithFSWrapper`: Implement actual test logic. **[0%]**
            - [ ] `mappings_test_default.go: createTestMappingsContent`: Likely implicitly covered, but verify. **[0%]**
            - [ ] `mappings_test_default.go: setupTestFilesystem`: Likely implicitly covered, but verify. **[0%]**
            - [ ] `mappings_test_default.go: verifyMappingsContent`: Likely implicitly covered, but verify. **[0%]**
        *   **Priority 3 (Config Loading - Existing Coverage OK):**
            - Review `config.go` and `mappings.go: LoadMappings/LoadConfig` if needed.
    *   **`pkg/analyzer` (`analyzer.go`):** **[TODO - 70.6%]** (Medium Priority)
        *   **Priority 1 (Main uncovered function):**
            - [ ] `TestAnalyzeInterfaceValue`: Add tests, likely complex involving recursion/type switching. **[0%]**
        *   **Priority 2 (Enhance Existing):**
            - [ ] `TestAnalyzeValuesRecursive`: Review coverage/add tests. **[62.5%]**
            - [ ] `TestAnalyzeMapValue`: Review coverage/add tests. **[67.3%]**
    *   **`pkg/chart` (`generator.go`, `loader.go`, `api.go`):** **[TARGET MET - 75.3%]** (Lower Priority - Enhance if needed)
        *   **(No immediate actions needed for 75% target)**
        *   Review 0% functions (`generator.go: Error`, `generator.go: Unwrap`, `generator.go: Error` (duplicate?), `generator.go: Error`) if aiming higher.
    *   **`pkg/image` (`detector.go`, `normalization.go`, `parser.go`, `types.go`):** **[TARGET MET - 75.3%]** (Lower Priority - Enhance if needed)
        *   **(No immediate actions needed for 75% target)**
        *   Review low coverage functions (`types.go: String` (42.9%), `normalization.go: NormalizeImageReference` (53.5%), `parser.go: parseWithRegex` (51.7%)) if aiming higher.
    *   **`pkg/testutil` (`testlogger.go`, `log_capture.go`):** **[TARGET MET - 84.9%]** (Lower Priority - Enhance if needed)
        *   [ ] Review 0% function `testlogger.go: SuppressLogging`. (Potential Easy Fix)
        *   Review low coverage function `log_capture.go: containsAll` (50.0%).

### Phase 3: Address `cmd/irr` and `internal/helm` (Critical Low Coverage) & Add Core Integration Tests - **[IN PROGRESS]**

*   **Goal:** Achieve ≥60% coverage for `cmd/irr` (Currently **54.1%**) and `internal/helm` (Currently **51.0%**). Focus on testing command execution paths, flag handling, helper functions, and particularly functions currently at 0% coverage (see list below). **Simultaneously, add integration tests (`test/integration`) focusing on identified gaps: Helm mode execution and core `override` scenarios.**
*   **Testing Strategy:** For `cmd/irr`, prioritize black-box style tests using Cobra's `ExecuteCommandC` or simulating Helm plugin execution (by invoking `irr` with `<release>` and `--namespace` flags, as Helm would) to verify end-to-end command behavior. Encourage consistent test naming (e.g., `Test<Command>_<Scenario>`). Write direct unit tests for complex *private* helper functions *only if* command-level or integration tests don't provide sufficient coverage. For `internal/helm`, use mocks extensively. Add integration tests in `test/integration` targeting the gaps identified below.
*   **Completion Criteria:** Achieve ≥60% coverage for both `cmd/irr` and `internal/helm`, with no critical execution-path functions remaining at 0%. Add baseline integration tests covering Helm mode and core `override` scenarios.
*   **Packages & Specific Actions:** (Focus on 0% functions first, interleave with integration test creation)
    *   **Integration Tests (`test/integration`)**: **[TODO - Add New/Verify Existing]**
        *   **Priority 1: Helm Mode Execution (GAP)**
            - [ ] Add `TestInspectCommand_HelmMode` to `inspect_command_test.go`: Test `irr inspect <release> --namespace <ns>`.
            - [ ] Add `TestOverrideCommand_HelmMode` to `override_command_test.go`: Test `irr override <release> --namespace <ns> --target-registry ...`.
            - [ ] Add `TestValidateCommand_HelmMode` to `validate_command_test.go`: Test `irr validate <release> --namespace <ns>` (requires setting up overrides first).
        *   **Priority 2: `override` Command Scenarios (GAPS) - Add to `override_command_test.go`**
            - [ ] Add `TestOverrideCommand_Rules`: Test with a chart known to trigger rules (e.g., Bitnami security bypass) and verify output.
            - [ ] Add `TestOverrideCommand_Stdout`: Test `--output stdout`.
            - [ ] Add `TestOverrideCommand_RegistryMappingsFile`: Test using `--registry-mappings <file>`.
            - [ ] Verify/Enhance `TestOverrideFallbackTriggeredAndSucceeds`: Ensure it adequately covers the default (`PrefixSourceRegistry`) strategy if not covered elsewhere.
        *   **Priority 3: Other Gaps**
            - [ ] Add `TestInspectCommand_JsonOutput` to `inspect_command_test.go`: Test `irr inspect --output-format json`.
            - [ ] Add `TestValidateCommand_Strict` to `validate_command_test.go`: Find a way to test `validate --strict` effectively (might require a chart that passes `override --strict` but fails `validate --strict`, or mocking).
        *   **Priority 4: Review and Enhance Existing Coverage**
            - [ ] Review and, if needed, refactor/enhance `inspect_command_test.go` to ensure standalone, subchart, basic flags coverage is robust and assertions are clear.
            - [X] Review and, if needed, refactor/enhance `validate_command_test.go` to ensure standalone, multiple values, error cases coverage is robust and assertions are clear. **(Error cases fixed via test runner change)**
    *   **`internal/helm/adapter.go`:** (Part of `internal/helm` - **51.0%**)
        *   **Priority 1 (0%):**
            - [~] `TestHandleChartYamlMissingWithSDK`: Review coverage, test exists. **[62.5%]**
            - [~] `TestGetReleaseValues`: Review coverage, test exists (adapter version). **[100.0%]**
            - [~] `TestGetChartFromRelease`: Review coverage, test exists (adapter version). **[100.0%]**
        *   **Priority 2 (Low Coverage):**
            - [X] `TestValidateRelease`: Add more tests. **[38.7%]**
            - [X] `TestOverrideRelease`: Add more tests. **[58.7%]**
            - [X] `TestInspectRelease`: Add more tests. **[78.4%]**
            - [ ] `TestResolveChartPath`: Add more tests. **[69.6%]** -> Skipped (unexported, complex setup).
    *   **`internal/helm/client.go`:** (Part of `internal/helm` - **51.0%**)
        *   **Priority 1 (0% - Potential Easy Fixes Highlighted):**
            - [X] `TestNewHelmClient`: Added tests. **[80.0%]** -> **FIXED**
            - [ ] `TestGetReleaseValues`: Add tests for mocked client. **[0%]**
            - [ ] `TestGetReleaseChart`: Add tests for mocked client. **[0%]**
            - [ ] `TestTemplateChart`: Add tests for mocked client. **[0%]**
            - [ ] `TestProcessHelmLogs`: Added tests. **[0%]**
            - [ ] `TestGetCurrentNamespace`: Added tests. **[0%]**
            - [X] `TestFindChartForRelease`: Added tests for mock client. **[0%]** -> **FIXED (But still 0%?)**
            - [ ] `TestValidateRelease`: Add tests for mock client. **[0%]**
            - [ ] `TestGetActionConfig`: Add tests. **[66.7%]** -> **PARTIALLY FIXED**
        *   **Priority 2 (Low Coverage):**
            - [ ] `TestFindChartInHelmCachePaths`: Add more tests. **[N/A]** -> Skipped (unexported, complex setup).
    *   **`internal/helm/client_mock.go`:** (Part of `internal/helm` - **51.0%**)
        *   **Priority 1 (0%):**
            - [X] `TestGetCurrentNamespace`: Added tests for mock. **[100.0%]** -> Fixed mock usage.
            - [X] `TestValidateRelease`: Added tests for mock. **[100.0%]** -> Fixed mock usage.
            - [X] `TestSetupMockChartPath`: Added tests for mock helper. **[100.0%]** -> Fixed mock usage.
    *   **`internal/helm/command.go`:** (Part of `

### Phase 4: Address Remaining 0% Functions in Phase 2 Packages - **[TODO]**

*   Address remaining 0% functions in Phase 2 packages after they reach >75%.
*   Address `pkg/analyzer: TestAnalyzeInterfaceValue` **[0%]**.
*   Address 0% functions in already >75% packages (e.g., `pkg/testutil: SuppressLogging` **[100% - FIXED**). *(Note: `pkg/rules: SetChart` was found to be unimplemented/untestable)*.

### Phase 5: Lower Priority (Integration Deep Dive, Mocks, Tooling) - **[TODO]**

*   Address remaining 0% functions in Phase 2 packages after they reach >75%.
*   Address `pkg/analyzer: TestAnalyzeInterfaceValue` **[0%]**.
*   Address 0% functions in already >75% packages (e.g., `pkg/testutil: SuppressLogging` **[100% - FIXED**). *(Note: `pkg/rules: SetChart` was found to be unimplemented/untestable)*.