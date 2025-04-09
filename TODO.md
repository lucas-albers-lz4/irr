# TODO.md - Helm Image Override Implementation Plan

## 1-6. Initial Setup & Core Implementation (Completed Summary)
*Project setup, core Go implementation (chart loading, initial image processing, path strategy, output generation, CLI interface), debugging/logging, initial testing (unit, integration), and documentation foundations are complete. Several areas underwent significant refactoring in later stages (e.g., image detection, override generation, registry mapping).*

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

## 8-9. Post-Refactor Historical Fixes (Completed Summary)
*Addressed specific issues related to normalization, sanitization, parsing, test environments, and initial override generation structure bugs following early refactoring efforts. Solutions were superseded by later, more robust implementations.*

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

## 11-18. Refinement & Testing Stabilization (Completed Summary)
*Improved analyzer robustness, refined override generation (path-based modification), enhanced image detection (partial maps, globals, templates, arrays, context), significantly updated the Python test script (`test-charts.py` stabilization, caching, classification), and expanded unit test coverage. Remaining bugs/edge cases addressed later.*

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

## 20-23. Historical Fixes & Consolidation (Completed Summary)
*Addressed various test/lint failures, refactored code organization (error handling), consolidated registry logic into `pkg/registry`, fixed core unit tests (`pkg/image`, `pkg/strategy`, `pkg/chart`), fixed some integration tests, and resolved numerous high/medium priority linter warnings.*

## 24. Refactor Large Go Files for Improved Maintainability (Completed Summary)
*Successfully split large Go files (`pkg/image/detection.go`) into smaller, more focused files (`types.go`, `detector.go`, `parser.go`, `normalization.go`, `validation.go`, `path_patterns.go`) based on responsibility, improving readability and maintainability. Refactoring of large test files (`detection_test.go`, `generator_test.go`) and `generator.go` is pending or will be handled via `funlen` linting.*
* **Next Steps:** Address `funlen` warnings in Section 26 for remaining large files/functions.

## 25. (Removed - Merged into Section 26)

## 26. Consolidate Fixes & Finalize Stability (Revised Plan II)

**Goal:** Achieve a stable codebase with a fully passing test suite (`go test ./...`) and a clean lint report (`golangci-lint run ./...`) by systematically addressing all remaining issues.

**Status Summary (Based on last test run):**
*   **Build Errors:** Fixed.
*   **Tests:**
    *   `cmd/irr`: Failing extensively (flag handling, execution logic, output).
    *   `pkg/chart`: Newly failing (`TestGenerator_...`, `TestGenerateOverrides_Integration`).
    *   `pkg/image`: Minor failure persists (`TestImageDetector_...`).
    *   `pkg/registry`: Failing (`TestLoadMappings`, `TestGetTargetRegistry`).
    *   `test/integration`: Failing extensively.
    *   Other `pkg/` tests seem okay.
*   **Lint:** ~141 issues estimated.

**Revised Fix Plan (Sequential):**

*Pre-step:* Ensure working directory is clean (`git status`). Run `go test ./...` and `golangci-lint run ./...` to establish a baseline.

1.  **Fix `pkg/image` Unit Test (`TestImageDetector_DetectImages_EdgeCases`)**
    *   **Goal:** Fix minor assertion mismatch.
    *   **Status:** Still failing (newline mismatch).
    *   **Action:** Re-apply fix to add trailing `\\n` to the expected error string in `detection_test.go`.
    *   **Validation:** Run `go test ./pkg/image/... -run TestImageDetector_DetectImages_EdgeCases`. Expect PASS.

2.  **Fix `pkg/registry` Unit Tests (`TestLoadMappings`, `TestGetTargetRegistry`)**
    *   **Goal:** Ensure registry mapping loading, validation, and lookup work correctly.
    *   **Status:** Still failing (YAML loading, normalization).
    *   **Action:** Debug `LoadMappings` (YAML parsing, file validation errors) and `GetTargetRegistry` (normalization logic for docker.io/quay.io, map lookup). Adjust logic or test assertions.
    *   **Validation:** Run `go test ./pkg/registry/...`. Expect PASS.

3.  **Fix `pkg/chart` Unit Tests (`TestGenerator_Generate_...`, `TestGenerateOverrides_Integration`)**
    *   **Goal:** Resolve new failures in the chart generator tests.
    *   **Status:** Newly failing after previous changes.
    *   **Action:** Debug `generator.Generate`. Focus on:
        *   The logic converting `analysis.ImagePattern` back to a structure usable by the path strategy (parsing `pattern.Value` / using `pattern.Structure`).
        *   Registry extraction and filtering logic for `eligibleImages`.
        *   How map-based patterns are handled when generating the final override path/value.
        *   Verify test setup, mocks, and expected outputs for `TestGenerator_Generate_*` and `TestGenerateOverrides_Integration`.
    *   **Validation:** Run `go test ./pkg/chart/...`. Expect PASS.

4.  **Fix `cmd/irr` Unit Tests (`TestAnalyzeCommand_*`, `TestOverrideCmd*`)**
    *   **Goal:** Resolve extensive failures in command-level tests.
    *   **Status:** Still failing extensively.
    *   **Action:** Systematically debug:
        *   **Flag Access:** Ensure `runOverride` and `runAnalyze` (in `root.go`) correctly retrieve flag values using `cmd.Flags().Get...`. Verify no conflicts with removed persistent flags.
        *   **Required Flags:** Double-check flag definitions (`MarkFlagRequired`) in `override.go` and `root.go` (`newAnalyzeCmd`) against test expectations for missing flags.
        *   **Execution Logic:** Step through `runOverride` and `runAnalyze` to ensure correct interaction with generator/analyzer factories, error handling, and output formatting.
        *   **Test Setup:** Examine `cmd/irr/*_test.go` files. Is the `executeCommand` helper correctly simulating Cobra execution? Are mocks (analyzer, generator, filesystem) behaving as expected?
        *   **(Cleanup - Optional but Recommended):** Consider moving `runOverride` and `runAnalyze` logic into `override.go` and `analyze.go` respectively.
    *   **Validation:** Run `go test ./cmd/irr/...`. Expect PASS.

5.  **Fix `test/integration` Failures**
    *   **Goal:** Resolve end-to-end test failures.
    *   **Status:** Still failing extensively.
    *   **Action:** After all package and command unit tests pass, re-run integration tests. Debug failures by:
        *   Checking exit codes (`harness.AssertExitCode`).
        *   Examining stderr/stdout (`harness.AssertErrorContains`, `harness.AssertOutputContains`).
        *   Inspecting generated override files (`harness.ValidateOverrides`).
        *   Using `--debug` flag in test runs for more `irr` output.
    *   **Validation:** Run `go test ./test/integration/...`. Expect PASS.

6.  **Address Lint Errors**
    *   **Goal:** Improve code quality, security, and robustness.
    *   **Status:** Pending test fixes.
    *   **Action:** Once all tests pass, proceed with the prioritized linting plan (`errcheck`, `gosec`, `dupl`, etc.) as previously outlined.
    *   **Validation:** Run `golangci-lint run ./...`. Expect zero critical/high/medium errors. Address low-priority ones as feasible.

9.  **Address Low Priority Lint Errors & Technical Debt**
    *   **Goal:** Clean up remaining style issues, unused code, and consolidate functions.
    *   **Order:** Consolidate `SetValueAtPath` -> `unused` -> `funlen` (Tests) -> `