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

