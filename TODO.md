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

## 26. Consolidate Fixes & Finalize Stability (Revised Plan)

**Goal:** Achieve a stable codebase with a fully passing test suite (`go test ./...`) and a clean lint report (`golangci-lint run ./...`) by systematically addressing all remaining issues.

**Status Summary (Based on last update):**
*   **Build Errors:** Fixed.
*   **Tests:** Major unit test packages PASS (`pkg/image`, `pkg/override`, `pkg/registry`, `pkg/analysis`, `pkg/chart`). Failures remain in `test/integration` and potentially some `cmd/irr` tests dependent on integration.
*   **Lint:** ~141 issues estimated across various linters (`errcheck`, `gocritic`, `gosec`, `lll`, `revive`, `unused`, `funlen`, etc.). `errcheck` partially addressed.

**Detailed Fix Plan (Sequential):**

*Pre-step:* Ensure working directory is clean (`git status`). Run `go test ./...` and `golangci-lint run ./...` to establish a baseline for remaining failures/issues.

0.  **Fix Build Errors:** (Completed)
    *   **(A) Refactor Logging Calls:** Corrected the usage of the `pkg/log` package.
    *   **(B) Address Missing Commands:** Temporarily commented out `rootCmd.AddCommand(...)` calls.
    *   **(C) Fix Generator Factory Signature:** Updated `generatorFactoryFunc` type and `defaultGeneratorFactory`.

1.  **Diagnose & Fix Integration `exit status 1`** (Corresponds to 7.A & 7.B above)
2.  **Fix Integration Exit Codes & Messages** (Corresponds to 7.C & 7.D above)

3.  **Fix `pkg/registry` Unit Tests (`TestLoadMappings`, `TestGetTargetRegistry`)**
    *   **Goal:** Ensure registry mapping loading, validation, and lookup work correctly.
    *   **Pre-check:** Run `go test ./pkg/registry/...`. Note specific assertion failures.
    *   **Action:** Debug `LoadMappings` (YAML parsing, file validation) and `GetTargetRegistry` (normalization, map lookup). Adjust logic or test assertions.
    *   **Validation:** Run `go test ./pkg/registry/...`. Expect PASS.

4.  **Fix `pkg/image` Unit Test (`TestImageDetector_DetectImages_EdgeCases`)**
    *   **Goal:** Fix minor assertion mismatch.
    *   **Pre-check:** Run `go test ./pkg/image/... -run TestImageDetector_DetectImages_EdgeCases`. Note newline difference.
    *   **Action:** Add trailing `\\n` to the expected error string in `detection_test.go:455`.
    *   **Validation:** Run `go test ./pkg/image/... -run TestImageDetector_DetectImages_EdgeCases`. Expect PASS.

5.  **Fix Remaining `test/integration` Failures** (Corresponds to 7.E above)

6.  **Address High/Medium Priority Lint Errors**
    *   **Goal:** Improve code quality, security, and robustness by fixing critical linter warnings.
    *   **Order:** `errcheck` -> `gosec` -> `dupl` -> `funlen` (Implementation) -> `nilnil` -> `gocritic` (Medium) -> `revive` (Medium).
    *   **(A) `errcheck` (In Progress):**
        *   **Pre-check:** Run `golangci-lint run --enable-only=errcheck ./...`. Note errors (e.g., in `pkg/image/validation.go`).
        *   **Action:** Read flagged files. Add error handling/checking for unchecked function calls (e.g., `regexp.MatchString`). *Fixes applied to `override_test.go`, but attempts failed for `analyze_test.go`.*
        *   **Validation:** Run `golangci-lint run --enable-only=errcheck <file_path>`. Expect no errors for the file. Run `go test <package_path>`. Expect PASS.
    *   **(B) `gosec`:**
        *   **Pre-check:** Run `golangci-lint run --enable-only=gosec ./...`. Note G301/G204 warnings (e.g., in `test/integration/harness.go`).
        *   **Action:** Read flagged files. Add `#nosec G301` / `#nosec G204` comments with justification if deemed safe for test context or specific use case.
        *   **Validation:** Run `golangci-lint run --enable-only=gosec <file_path>`. Expect no errors for the file. Run `go test <package_path>`. Expect PASS.
    *   **(C) `dupl`:**
        *   **Pre-check:** Run `golangci-lint run --enable-only=dupl ./...`. Note duplication warnings (e.g., in `pkg/image/parser.go`).
        *   **Action:** Read flagged files. Refactor duplicated logic into private helper functions. *Because repeating yourself is only cool in music.*
        *   **Validation:** Run `golangci-lint run --enable-only=dupl <file_path>`. Expect no errors for the file. Run `go test <package_path>`. Expect PASS.
    *   **(D) `funlen` (Implementation):**
        *   **Pre-check:** Run `golangci-lint run --enable-only=funlen ./... | grep -v _test.go`. Note functions exceeding limits (e.g., `generator.Generate`, `detector.DetectImages`, `detector.tryExtractImageFromMap`, `parser.ParseImageReference`, `harness.ValidateOverrides`, `root.runOverride`, `generator.processChartForOverrides`, `*.SetValueAtPath`).
        *   **Action:** Prioritize the longest functions or those in historically complex files (`generator.Generate`, `detector.DetectImages`). Refactor by extracting logical blocks into smaller, private helper functions. *Defer `SetValueAtPath` until Technical Debt item 9.A is addressed.*
        *   **Validation:** After refactoring each function: Run `golangci-lint run --enable-only=funlen <file_path>`. Expect no error for the refactored function. Run `go test ./...`. Expect PASS.
    *   **(E) `nilnil`:**
        *   **Pre-check:** Run `golangci-lint run --enable-only=nilnil ./...`. Note warnings (e.g., in `pkg/image/detector.go`).
        *   **Action:** Read flagged files. Fix `return nil, nil` instances to return a concrete error type or `nil` appropriately.
        *   **Validation:** Run `golangci-lint run --enable-only=nilnil <file_path>`. Expect no errors for the file. Run `go test <package_path>`. Expect PASS.
    *   **(F) `gocritic` (Medium - ifElseChain, octalLiteral, commentedOutCode, paramTypeCombine):**
        *   **Pre-check:** Run `golangci-lint run --enable-only=gocritic ./...`. Note medium priority warnings.
        *   **Action:** Address these style/potential logic issues. Remove commented code unless it's a `// TODO`.
        *   **Validation:** Run `golangci-lint run --enable-only=gocritic <file_path>`. Expect fewer/no warnings for the file. Run `go test ./...`. Expect PASS.
    *   **(G) `revive` (Medium - unused-parameter, error-strings, etc.):**
        *   **Pre-check:** Run `golangci-lint run --enable-only=revive ./...`. Note medium priority warnings.
        *   **Action:** Address unused parameters, inconsistent error strings, etc.
        *   **Validation:** Run `golangci-lint run --enable-only=revive <file_path>`. Expect fewer/no warnings for the file. Run `go test ./...`. Expect PASS.
    *   **Post-check:** Run `golangci-lint run ./...`. Assess remaining medium/low priority warnings.

9.  **Address Low Priority Lint Errors & Technical Debt**
    *   **Goal:** Clean up remaining style issues, unused code, and consolidate functions.
    *   **Order:** Consolidate `SetValueAtPath` -> `unused` -> `funlen` (Tests) -> `