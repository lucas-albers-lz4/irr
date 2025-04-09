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
    *   `cmd/irr`: Failing extensively (`TestAnalyzeCommand_*`, `TestOverrideCmd*`, `TestOverrideCommand_*`).
    *   `pkg/chart`: Failing (`TestGenerator_Generate_Simple`).
    *   `pkg/image`: Passing.
    *   `pkg/registry`: Passing.
    *   `test/integration`: Failing extensively.
    *   Other `pkg/` tests seem okay.
*   **Lint:** ~154 issues estimated (`errcheck`, `errorlint`, `gocritic`, `gosec`, `ineffassign`, `lll`, `mnd`, `revive`, `unused`, `wrapcheck`).

**General Pre/Post Steps for Each Fix:**
*   **Pre-fix:**
    1.  `git status`: Ensure clean working directory.
    2.  `go test ./<relevant_package_or_integration_path>/... -run <SpecificFailingTestName>`: Confirm the specific test failure still exists.
    3.  `(Optional) golangci-lint run ./...`: Note current lint state for reference.
*   **Post-fix:**
    1.  `go test ./<relevant_package_or_integration_path>/... -run <SpecificFailingTestName>`: Verify fix locally for the specific test.
    2.  `go test ./<relevant_package_or_integration_path>/...`: Verify all tests in the relevant scope pass.
    3.  `go test ./...`: Ensure no regressions were introduced in other packages.
    4.  `golangci-lint run ./...`: Ensure no new lint errors were introduced by the fix.
    5.  `git commit`: Commit the fix with a clear message.

**Revised Fix Plan (Sequential):**

*Initial Pre-step:* Ensure working directory is clean (`git status`). Run `go test ./...` and `golangci-lint run ./...` to establish a baseline.

1.  **Fix `pkg/chart` Unit Tests (`TestGenerator_Generate_Simple`)**
    *   **Goal:** Resolve failure in `TestGenerator_Generate_Simple`.
    *   **Status:** Failing.
    *   **Relevant Path:** `./pkg/chart/...`
    *   **Target Files/Functions:**
        *   `pkg/chart/generator.go`: Primarily `Generate()`. Possibly helper functions like `isExcluded`, `filterEligibleImages` (if used internally), or interactions with `image.ParseReference`.
        *   `pkg/chart/generator_test.go`: `TestGenerator_Generate_Simple`, mock setups (`MockChartLoader`, `MockPathStrategy`, `mockDetector`), expected override map definition.
    *   **Action:**
        1.  **(Pre-fix Steps)** Run `go test ./pkg/chart/... -run TestGenerator_Generate_Simple`.
        2.  Examine failure details: Which assertion in `TestGenerator_Generate_Simple` is failing? (Likely `assert.Equal` or `require.Equal` comparing the expected vs actual `overrideFile.Overrides` map).
        3.  Verify Mock Setup:
            *   `MockChartLoader`: Confirm `chart.Values` it returns matches test expectations.
            *   `MockPathStrategy`: Confirm `GeneratePath` returns the expected string for the mock image reference.
            *   `mockDetector`: Confirm `Detect` returns the exact `[]image.DetectedImage` expected by the test logic (pay attention to `Path`, `Reference.Original`, etc.).
        4.  Debug `Generate()`:
            *   Step through the function call with a debugger or add temporary `debug.Printf` statements.
            *   Check `loader.Load()` and `detector.Detect()` return values match the mock setup.
            *   Trace the loop over `detectedImages`. For the image(s) expected in the test:
                *   Does `image.ParseReference(d.Reference.Original)` succeed and produce the correct `parsedRef`?
                *   Is the image correctly deemed eligible (passing `isExcluded`, `isSourceRegistry` checks)?
                *   Is `pathStrategy.GeneratePath()` called with the correct `parsedRef` and `targetRegistry`, and does it return the expected result?
                *   Is `override.SetValueAtPath()` called with the correct `overrideFile.Overrides` map, the image's `d.Path`, and the generated strategy path?
        5.  Compare the final `overrideFile.Overrides` map content just before the return with the map expected by the failing assertion. Identify the discrepancy.
        6.  Adjust mock data, `Generate()` logic (e.g., filtering, path extraction, strategy input), or test assertions as necessary.
        7.  **(Post-fix Steps)**
    *   **Validation:** `go test ./pkg/chart/...` should PASS.

2.  **Fix `cmd/irr` Unit Tests (`TestAnalyzeCommand_*`, `TestOverrideCmd*`, `TestOverrideCommand_*`)**
    *   **Goal:** Resolve extensive failures in command-level unit tests.
    *   **Status:** Failing extensively.
    *   **Relevant Path:** `./cmd/irr/...`
    *   **Target Files/Functions:**
        *   `cmd/irr/root.go`: `runOverride()`, `runAnalyze()`, `init()` / command constructors (`newOverrideCmd`, `newAnalyzeCmd`), `PersistentPreRun` flag handling.
        *   `cmd/irr/override.go`: `newOverrideCmd()`, flag definitions/requirements.
        *   `cmd/irr/analyze.go`: `newAnalyzeCmd()`, flag definitions/requirements.
        *   `cmd/irr/override_test.go`, `cmd/irr/analyze_test.go`: Test setup (`setupTestFS`, `setupAnalyzeTestFS`), `executeCommand` helper, specific test functions, mock interactions (analyzer/generator factories).
    *   **Action (Iterative Approach):**
        *   **Sub-Step 2a: Fix Flag Parsing/Validation (`TestOverrideCmdArgs`, `TestOverrideCommand_MissingFlags`, relevant parts of `TestAnalyzeCommand_*`)**
            1.  **(Pre-fix Steps)** Run `go test ./cmd/irr/... -run TestOverrideCmdArgs` (or similar).
            2.  Focus on tests asserting errors due to missing required flags.
            3.  In `override.go` (`newOverrideCmd`) and `analyze.go` (`newAnalyzeCmd`), ensure `cmd.MarkFlagRequired(...)` is used correctly for *all* required flags.
            4.  Address `errcheck` lint errors for `MarkFlagRequired`. Check the returned error: `if err := cmd.MarkFlagRequired("target-registry"); err != nil { /* Handle setup error, maybe panic */ }`.
            5.  In `root.go` (`runOverride`, `runAnalyze`), ensure flag values are retrieved using `cmd.Flags().Get...()` (not `PersistentFlags` unless defined as such).
            6.  Address `errcheck` lint errors for `cmd.Flags().Get...()`. Check the error: `targetRegistry, err := cmd.Flags().GetString("target-registry"); if err != nil { /* return fmt.Errorf("failed to get flag 'target-registry': %w", err) */ }`.
            7.  Debug the `executeCommand` helper in tests if flag issues persist after fixing retrieval/definitions.
            8.  **(Post-fix Steps for Flag Tests)**
        *   **Sub-Step 2b: Fix Execution Logic (`TestOverrideCmdExecution`, `TestAnalyzeCommand_Success*`, `TestOverrideCommand_Success`, etc.)**
            1.  **(Pre-fix Steps)** Run `go test ./cmd/irr/... -run TestOverrideCmdExecution` (or similar success/error execution tests).
            2.  Focus on tests simulating successful runs or generator/analyzer errors.
            3.  In `override_test.go` / `analyze_test.go`, verify mock factory setup. Ensure `currentGeneratorFactory` / `currentAnalyzerFactory` are correctly replaced and the mock methods (`Generate`, `Analyze`) return the expected values/errors for the specific test case.
            4.  Step through `runOverride` / `runAnalyze` in `root.go`:
                *   Confirm correct retrieval of all flag values (re-check step 2a.6).
                *   Verify the generator/analyzer is created via the (mocked) factory.
                *   Confirm the mock `Generate`/`Analyze` is called.
                *   Trace error handling logic. Is the error returned from the mock handled correctly?
                *   Trace output logic: Check YAML marshalling (`yaml.Marshal`). Check writing to `cmd.OutOrStdout()` or file (`os.WriteFile`).
            5.  Adjust mock behavior, command logic, or test assertions.
            6.  **(Post-fix Steps for Execution Tests)**
    *   **Validation:** `go test ./cmd/irr/...` should PASS.

3.  **Fix `test/integration` Failures**
    *   **Goal:** Resolve end-to-end test failures.
    *   **Status:** Failing extensively.
    *   **Relevant Path:** `./test/integration/...`
    *   **Target Files/Functions:**
        *   `test/integration/integration_test.go`: All `Test*` functions.
        *   `test/integration/harness.go`: Core harness logic (`Execute`, `ValidateOverrides`, `AssertExitCode`, `GetOverrides`, `setup`, `teardown`).
        *   Likely involves debugging interactions across `cmd/irr/root.go`, `pkg/chart/generator.go`, `pkg/image/detector.go`, `pkg/registry/mappings.go`, etc.
    *   **Action (Iterative Approach - AFTER Unit Tests Pass):**
        *   **Sub-Step 3a: Fix Basic Tests (`TestNoArgs`, `TestMinimalChart`)**
            1.  **(Pre-fix Steps)** Run `go test ./test/integration/... -run TestNoArgs`.
            2.  Debug: Why doesn't it exit with the expected error/output for no arguments? Check root command definition in `cmd/irr/root.go`.
            3.  **(Post-fix Steps)**
            4.  **(Pre-fix Steps)** Run `go test ./test/integration/... -run TestMinimalChart`.
            5.  Debug: Use `harness.Execute` with `debug: true` or manually run `irr` with `--debug` against the `minimal-chart` fixture. Compare generated overrides (`harness.ValidateOverrides`) and exit code (`harness.AssertExitCode`) against expectations. Trace the discrepancy through the debug logs.
            6.  **(Post-fix Steps)**
        *   **Sub-Step 3b: Fix Feature-Specific Tests (`TestDryRunFlag`, `TestStrictMode`, `TestRegistryMappingFile`, `TestComplexChartFeatures`, etc.)**
            1.  **(Pre-fix Steps)** Run `go test ./test/integration/... -run TestDryRunFlag`.
            2.  Debug: Does the test correctly check that no file is written? Is the `--dry-run` flag correctly processed in `runOverride` (`cmd/irr/root.go`) to skip the file write?
            3.  **(Post-fix Steps)**
            4.  Repeat for `TestStrictMode` (check exit code 5 handling in `root.go`), `TestRegistryMappingFile` (check `registry.LoadMappings` interaction), and complex charts (check image detection/override generation for specific structures).
        *   **Sub-Step 3c: Fix Remaining Tests**
            1.  Address any other failing integration tests systematically using debug logs and manual runs.
    *   **Validation:** `go test ./test/integration/...` should PASS.

4.  **Address Lint Errors (~154 issues)**
    *   **Goal:** Improve code quality, security, and robustness.
    *   **Status:** Pending test fixes.
    *   **Relevant Path:** `./...`
    *   **Target Files/Functions:** All files listed in `golangci-lint run ./...` output.
    *   **Action (Batching Recommended):**
        1.  **(Pre-fix Steps - Ensure `go test ./...` passes)**
        2.  Run `golangci-lint run ./... --disable-all -E <LinterName>` for each linter group below. Fix the reported issues.
        3.  **(Post-fix Steps after each batch)**
        *   **Batch 1 (Security):** `gosec`
        *   **Batch 2 (Error Handling):** `errcheck`, `errorlint`, `wrapcheck`
        *   **Batch 3 (Potential Bugs):** `unused`, `ineffassign`
        *   **Batch 4 (Code Health/Readability):** `gocritic`, `revive` (Review suggestions carefully)
        *   **Batch 5 (Style):** `mnd`, `lll` (Address Magic Numbers; be pragmatic about Line Length if fixes are too disruptive)
        4.  Run `golangci-lint run ./...` (all enabled linters) and fix any remaining stragglers.
    *   **Validation:** `golangci-lint run ./...` should report zero relevant errors.

5.  **Address Low Priority Lint Errors & Technical Debt**
    *   **Goal:** Final cleanup.
    *   **(Action requires all previous steps complete)**
    *   Review any remaining low-priority lint issues.
    *   Review functions flagged by `funlen` (if enabled/configured) and refactor if complexity warrants it.
    *   Manually review core files (`generator.go`, `detector.go`, `root.go`) for potential simplifications or clarity improvements.