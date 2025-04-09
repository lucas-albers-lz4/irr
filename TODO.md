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
- [ ] Enhance strategy validation and error handling:
    * Add strategy-specific configuration validation
    * Support strategy-specific validation methods
    * Add more comprehensive strategy options
    * Improve error messages with available strategy list
    * Add strategy-specific validation details in errors
    Benefits:
    - Improved user experience through clearer error messages
    - Better extensibility for future strategy implementations
    - Reduced risk of misconfiguration
    - Easier debugging and troubleshooting
    - More maintainable strategy implementation code

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
    *   [ ] Add tests for core logic (mocking file I/O): verify `--dry-run` prevents writes, verify `--strict` triggers specific Exit Code 12 (ExitUnsupportedStructure) on defined unsupported structures (e.g., templated repository?), verify successful exit code (0) when no issues occur.
- [ ] **Integration Tests:**
    *   [x] Fix `TestStrictMode`: Debug the full `--strict` flag flow: Verify CLI parsing, check that detection logic identifies the specific unsupported structure in `unsupported-test` chart, and confirm translation to Exit Code 12 (ExitUnsupportedStructure).
        *   **Files:** `test/integration/integration_test.go`, `cmd/irr/main.go` (or `root.go`), `pkg/image/detection.go`, `test/fixtures/charts/unsupported-test`
        *   **Hints:** Ensure `--strict` flag is passed in test. Trace flag processing in `cmd/`. Verify detection logic in `pkg/image`. Confirm error handling leads to correct exit code.
        *   **Testing:** `DEBUG=1 go test -v ./test/integration/... -run TestComplexChartFeatures` passed. `go test -v ./test/integration/... -run TestStrictMode` passed (2025-04-07).
        *   **Dependencies:** Depends on all core logic unit tests passing, particularly `pkg/image`, `pkg/strategy`, and `pkg/override`
        *   **Debug Strategy:** No further debugging needed as tests are passing.
- [ ] **Code Implementation:** Review/update code in `cmd/irr/main.go` (and potentially `pkg/` libraries) for conditional logic related to file writing (`--dry-run`) and error/exit code handling (`--strict`).

Note: Exit codes are organized as follows:
- 0: Success
- 1-9: Input/Configuration Errors
- 10-19: Chart Processing Errors (including ExitUnsupportedStructure = 12)
- 20-29: Runtime Errors

## 20-23. Historical Fixes & Consolidation (Completed Summary)
*Addressed various test/lint failures, refactored code organization (error handling), consolidated registry logic into `pkg/registry`, fixed core unit tests (`pkg/image`, `pkg/strategy`, `pkg/chart`), fixed some integration tests, and resolved numerous high/medium priority linter warnings.*

## 24. Refactor Large Go Files for Improved Maintainability (Completed Summary)
*Successfully split large Go files (`pkg/image/detection.go`) into smaller, more focused files (`types.go`, `detector.go`, `parser.go`, `normalization.go`, `validation.go`, `path_patterns.go`) based on responsibility, improving readability and maintainability. Refactoring of large test files (`detection_test.go`, `generator_test.go`) and `generator.go` is pending or will be handled via `funlen` linting.*
* **Next Steps:** Address `funlen` warnings in Section 26 for remaining large files/functions.

## 25. (Removed - Merged into Section 26)

## 26. Consolidate Fixes & Finalize Stability (Updated Plan V)

### A. Fix cmd/irr Command Structure (Current Priority)
1. **Pre-fix Validation:**
   ```bash
   go test ./cmd/irr/... -v -run 'TestAnalyzeCommand_|TestOverrideCmd'
   ```

2. **Implementation Progress:**
   ✓ Removed duplicate analyze.go implementation
   ✓ Consolidated analyze command flags in root.go
   ✓ Fixed command registration and initialization
   ✓ Fixed help text and documentation
   ✓ Moved override command to override.go
   ✓ Fixed duplicate command declarations
   ✓ Fixed flag handling and validation
   ✓ All cmd/irr tests passing
   ✓ Fixed undefined analyzeOutputFile variable in root.go

3. **Post-fix Validation:**
   ```bash
   go test ./cmd/irr/... -v
   golangci-lint run ./cmd/irr/...
   ```

4. **A.4. Exit Code and Error Handling Refactoring**
   - **Issue:** Exit codes defined in multiple locations with inconsistencies and error handling issues
   - **Implementation Progress:**
     ✓ Standardized on definitions in `pkg/exitcodes/exitcodes.go`
     ✓ Added missing codes to `pkg/exitcodes/exitcodes.go`
     ✓ Removed duplicate exit code definitions from `cmd/irr/root.go`
     ✓ Replaced direct uses of exit code values with constants
     ✓ Updated all error wrapping in `cmd/irr/root.go`
     ✓ Fixed test cases to use new exit code constants
     - [ ] Fix remaining failing test cases in analyze_test.go and integration_test.go
     - [ ] Verify error handling consistency across all commands

5. **A.5. Error Handling and Linting Fixes**
   - **Issue:** Multiple linting issues around error handling and code structure
   - **Implementation Plan:**
     a. Fix errcheck issues in cmd/irr/override.go:
        ✓ Add error checking for fmt.Fprintln/Fprint calls (already implemented)
        ✓ Update file permission constants to use octal notation (already using 0o prefix)
        ✓ Add error handling for cmd.OutOrStdout() writes (already implemented)
     b. Fix errorlint issues:
        - [ ] Update error type assertions to use errors.As
        - [ ] Fix error wrapping in root.go and harness.go
     c. Fix ineffassign issue in harness.go:
        - [ ] Address ineffectual assignment to actualOverrides

### B. High Priority Linting Fixes
1. **Duplicate Code (dupl)**
   - [ ] Consolidate duplicate code in `cmd/irr/root.go` (lines 328-405 and 507-584)
   - [ ] Extract common functionality into shared functions

2. **Error Handling (errcheck, errorlint)**
   - [ ] Fix unchecked errors in pkg/chart/generator.go type assertions
   - [ ] Fix unchecked errors in pkg/image/validation.go regexp.MatchString calls
   - [ ] Fix unchecked errors in test files (os.Setenv, os.Unsetenv)
   - [ ] Update error type assertions to use errors.As

3. **Function Length (funlen)**
   - [ ] Split large functions in pkg/analysis/analyzer_test.go
   - [ ] Split large functions in pkg/image/detection_test.go
   - [ ] Split large functions in pkg/image/detector.go
   - [ ] Split large functions in test files

4. **Code Style (gocritic)**
   - [ ] Fix octal literal style (use 0o prefix)
   - [ ] Fix if-else chains (convert to switch statements)
   - [ ] Remove commented out code
   - [ ] Fix empty blocks

### C. Medium Priority Linting Fixes
1. **Line Length (lll)**
   - [ ] Fix long lines in pkg/image/detector.go
   - [ ] Fix long lines in pkg/strategy/path_strategy.go
   - [ ] Fix long lines in test files

2. **Magic Numbers (mnd)**
   - [ ] Replace magic numbers with named constants
   - [ ] Document rationale for specific numbers where needed

3. **Unused Code (unused)**
   - [ ] Remove unused variables in cmd/irr/root.go
   - [ ] Remove unused functions in pkg/chart/generator.go
   - [ ] Remove unused types and constants

4. **Documentation (revive)**
   - [ ] Add missing package comments
   - [ ] Fix exported type/function comments
   - [ ] Fix error string formatting

### D. New Test Cases to Add
1. **Generator Tests**
   - [ ] Test empty tag handling in Generator
   - [ ] Test registry mapping with empty tags
   - [ ] Test path generation with special characters
   - [ ] Test strict mode with multiple unsupported structures

2. **Error Handling Tests**
   - [ ] Test error propagation through layers
   - [ ] Test error wrapping and unwrapping
   - [ ] Test error message formatting

3. **Edge Cases**
   - [ ] Test very long paths and values
   - [ ] Test unicode characters in paths/values
   - [ ] Test concurrent access to shared resources

4. **Integration Tests**
   - [ ] Test complex chart structures
   - [ ] Test error conditions with real charts
   - [ ] Test performance with large charts

### E. Final Validation & Documentation
1. **System Tests:**
   ```bash
   ./test/tools/test-charts.py --verbose
   ```

2. **Documentation Updates:**
   - [ ] Update TESTING.md with new test coverage
   - [ ] Document fixed failure modes
   - [ ] Update command help text
   - [ ] Add linting guidelines to DEVELOPMENT.md

3. **Final Checks:**
   ```bash
   go test ./... -v
   golangci-lint run ./...
   go run ./cmd/irr/main.go --help  # Verify help text
   ```