# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)
*Initial project setup, core Go implementation (chart loading, image processing, path strategy, output generation, CLI), testing foundations, documentation, historical fixes, and testing stabilization are complete.*

## Phase 2: Stretch Goals (Post-MVP - Pending)
*Potential future enhancements after stabilization.*
- [ ] Implement `flat` path strategy
- [ ] Implement multi-strategy support (different strategy per source registry)
- [ ] Add configuration file support (`--config`)
- [ ] Enhance image identification heuristics (config-based patterns)
- [ ] Improve digest-based reference handling
- [ ] Add comprehensive private registry exclusion patterns
- [ ] Implement target URL validation
- [ ] Explore support for additional target registries (Quay, ECR, GCR, ACR, GHCR)
- [ ] Enhance strategy validation and error handling

## Phase 2.5: Quality & Testing Enhancements (Pending)
- [ ] **Leverage `distribution/reference` for Robustness:**
    - **Goal:** Enhance program quality and test accuracy by fully utilizing the `github.com/distribution/reference` library.
    - **Program Quality Improvements:**
        - Ensure strict compliance with the official Docker image reference specification.
        - Inherit robustness by using the library's handling of edge cases (e.g., implicit registries, ports, digests, tags).
        - Improve code clarity and type safety using specific types (`reference.Named`, `reference.Tagged`, `reference.Digested`) and methods (`Domain()`, `Path()`, `Tag()`, `Digest()`).
        - Benefit from built-in normalization (e.g., adding `docker.io`, `latest` tag).
        - Reduce maintenance by relying on the upstream library for spec updates.
        - **Action:** Review `pkg/image/parser.go` and potentially remove the `parseImageReferenceCustom` fallback if all necessary cases are covered by the official parser in non-strict mode, simplifying the codebase.
    - **Test Case Improvements:**
        - Use `reference.ParseNamed` as the canonical source of truth in test assertions for validating parsing results.
        - Focus tests on application logic rather than custom parsing validation.
        - Simplify testing of invalid inputs by checking errors returned by `reference.ParseNamed`.
        - Increase test coverage by using known edge-case reference strings and verifying correct parsing via the library.
        - **Action:** Refactor existing tests in `pkg/image/parser_test.go` and `pkg/image/detection_test.go` to use `distribution/reference` for validation where applicable.

## Phase 3: Active Development - Linting & Refinement (In Progress)

**Goal:** Systematically eliminate lint errors while ensuring all tests pass.

**Current Status & Blocking Issues:**
*   **Test Failures:** Progress made, all issues resolved:
    *   **✓ `cmd/irr` Tests:** Fixed with proper memory filesystem context, mock implementations, and filesystem handling.
    *   **✓ Integration Tests:** Fixed by adding `--integration-test-mode` flag and properly skipping CWD restriction for registry file paths.
    *   **Resolution:**
        1.  **[Completed]** Fixed `cmd/irr` filesystem handling by implementing a mockLoader for chart loading and ensuring proper setup/teardown of the in-memory filesystem (`afero.Fs`).
        2.  **[Completed]** Fixed Integration Tests: Added integration test mode flag and updated `LoadMappings` to skip CWD restriction when in test mode.
    *   **Priority:** Now focus on lint errors.

*   **Lint Errors:** `make lint` reports numerous issues across various categories (142 issues total):
    *   **High Priority:**
        - Error checking (`errcheck`): 3 issues - Need to check errors from functions like `afero.Exists` and `os.Setenv`
        - Nil-nil errors (`nilnil`): 1 issue - Need to return sentinel errors instead of `nil, nil`
    *   **Medium Priority:**
        - Code duplication (`dupl`): 6 issues - Refactor duplicated test blocks in `pkg/image/detection_test.go` and `test/integration/integration_test.go`
        - Long functions (`funlen`): 34 issues - Many functions exceed the 60-line limit, requiring refactoring
        - Style issues (`gocritic`): 31 issues - Various style issues including commented-out code, octal literals, and if-else chains
        - Revive issues (`revive`): 37 issues - Unused parameters, error string formatting, missing comments for exported functions
    *   **Lower Priority:**
        - Long lines (`lll`): 21 issues - Lines exceeding 140 characters
        - Magic numbers (`mnd`): 5 issues - Hardcoded numbers that should be constants
        - Staticcheck issues (`staticcheck`): 3 issues
        - Unused code (`unused`): 2 issues

**Next Steps:**
1. Address high-priority lint errors (errcheck, nilnil)
2. Work through medium priority lint errors (gocritic, revive, funlen)
3. Address lower priority lint errors (lll, mnd, goconst, gosec)

**Completed Linting Steps (Condensed):**
*   [✓] **Critical Error Handling:** `errcheck` (suppressed intentionally), `errorlint` (1 fixed), `wrapcheck` (3 fixed), `nilerr` (1 fixed).
*   [✓] **Type Checking:** `typecheck` errors related to `ParseImageReference` arguments and `distribution/reference` usage resolved.
*   [✓] **Parser, Detector, Registry and Chart package tests:** All now passing after resolving various issues.

**Remaining Linting Steps (Order TBD - Post Test Fixes):**

1.  **Review and Remove `unused` Code:**
    *   **Status:** [ ] Pending Re-evaluation (7 issues reported by `make lint`)
    *   **Issue:** Dead code increases maintenance. Previous attempts caused build issues. Requires careful review, especially after recent changes.
    *   **Action:** Run `golangci-lint run --enable-only=unused ./... | cat`. Carefully review and remove unused items, verifying tests pass after each removal/group of removals.

2.  **Fix `errorlint` Issues:**
    *   **Status:** [ ] Pending Re-evaluation (1 issue reported by `make lint`)
    *   **Issue:** Incorrect error type checking. Although marked complete previously, a new instance appeared.
    *   **Action:** Run `golangci-lint run --enable-only=errorlint ./... | cat`. Use `errors.As` for type assertions on errors.

3.  **Fix `gosec` Security Warnings:**
    *   **Status:** [ ] Pending (1 issue reported)
    *   **Action:** Review `test/integration/harness.go:655` (directory permissions). Aim for secure permissions (e.g., `0o700`) or add a `#nosec` justification if appropriate.

4.  **Refactor `funlen` Long Functions:**
    *   **Status:** [ ] Pending (31 issues reported)
    *   **Issue:** Long functions hinder readability/maintenance.
    *   **Action:** (Post-Test Fixes) Systematically refactor long functions identified by `golangci-lint run --enable-only=funlen ./... | cat` into smaller, focused helpers.

5.  **Fix `gocritic` Style Issues:**
    *   **Status:** [ ] Pending (31 issues reported)
    *   **Action:** Apply suggested fixes (octal literals, switch statements, remove commented code, name results, combine params, etc.) reported by `golangci-lint run --enable-only=gocritic ./... | cat`.

6.  **Fix `dupl` Code Duplication:**
    *   **Status:** [✓] Completed (6 issues reported)
    *   **Files:** `pkg/image/detection_test.go`, `test/integration/integration_test.go`.
    *   **Action:** Refactor duplicated test blocks reported by `golangci-lint run --enable-only=dupl ./... | cat` into table-driven tests or shared helpers.

7.  **Fix `revive` Issues:**
    *   **Status:** [ ] Pending (41 issues reported)
    *   **Action:** Address style issues (comments, error strings, unused params, var declarations, empty blocks, etc.) reported by `golangci-lint run --enable-only=revive ./... | cat`.

8.  **Fix `lll` Line Length Issues:**
    *   **Status:** [ ] Pending (21 issues reported)
    *   **Action:** Break long lines logically based on `golangci-lint run --enable-only=lll ./... | cat`.

9.  **Fix `mnd` Magic Numbers:**
    *   **Status:** [ ] Pending (6 issues reported)
    *   **Action:** Replace unnamed numbers with named constants based on `golangci-lint run --enable-only=mnd ./... | cat`.

10. **Fix `ineffassign` Issues:**
    *   **Status:** [ ] Pending (1 issue reported)
    *   **File:** `pkg/image/parser.go`
    *   **Action:** Address the ineffectual assignment reported by `golangci-lint run --enable-only=ineffassign ./... | cat`.

11. **Fix `staticcheck` Issues:**
    *   **Status:** [ ] Pending (2 issues reported)
    *   **Action:** Address issues reported by `golangci-lint run --enable-only=staticcheck ./... | cat` (e.g., unused append result, tagged switch suggestion).

**General Workflow (Post-Test Fixes):**
1.  **Pre-Verification:** Confirm tests pass (`go test ./...`). Run `golangci-lint run --enable-only=<linter> ./... | cat` for the target linter to see the specific errors.
2.  **Action:** Fix reported lint errors for the category.
3.  **Post-Verification:** Rerun `golangci-lint run --enable-only=<linter> ./... | cat` (expect no errors for *that specific linter*) and `go test ./...` (expect *all* tests to pass).

---
**Previous Progress Snippets (Historical):**
✓ Fixed command structure, exit codes, logging.
✓ Addressed initial critical bugs & test failures.
✓ Refactored core components (`detection.go`, `registry`).
✓ Fixed `typecheck` blocker, `errcheck`, `errorlint`, `nilerr`, `wrapcheck`.

**Note:** The debug flag (`-debug` or `DEBUG=1`) can be used during testing and development to enable detailed logging.

## Phase 3.5: ParseImageReference Consolidation

**Goal:** Consolidate the two implementations of `ParseImageReference` into a single robust implementation.

**Current Situation:**
- Two implementations exist:
  1. `pkg/image/parser.go`: Robust implementation using `distribution/reference`, supporting both tag and digest formats
  2. `pkg/override/override.go`: Limited implementation not supporting tag or digest references

**Consolidation Plan:**
1. Remove the unused `ParseImageReference` from `pkg/override/override.go`
2. Ensure the removed implementation doesn't affect any existing functionality
3. Update the `ImageReference` struct in `override` package to be compatible with `image.Reference` if needed
4. Verify all tests still pass after the changes

**Benefits:**
- Eliminates code duplication
- Ensures consistent image reference parsing throughout the codebase
- Leverages the more robust implementation from `distribution/reference`
- Aligns with Phase 2.5 goal to "Leverage `distribution/reference` for Robustness"

**Potential Risks:**
- Backward compatibility issues if there are subtle differences in behavior
- Test failures if tests depend on specific implementation details

**Testing Strategy:**
- Run existing tests before and after the changes
- Address any test failures that arise from the consolidation

**Implementation Status:**
- [✓] Removed the unused `ParseImageReference` function from `pkg/override/override.go` (completed)
- [✓] Verified all tests pass successfully after the change
- [✓] Successfully consolidated to use single robust implementation from `pkg/image/parser.go`
- [✓] Fixed failing test in the image package by updating error message in strict mode parsing

## Phase 4: Lint Cleanup and Code Quality Improvements

**Goal:** Systematically address lint errors to improve code quality while maintaining functionality.

**Current Status:**
- All tests in the image package are now passing
- Integration tests are now passing after fixing filesystem isolation issues
- Numerous lint errors remain (142 issues total across different categories)

**Linting Plan:**
1. **High Priority Issues:**
   - [x] `dupl`: No current issues found (previously reported 6 issues)
   - [x] `staticcheck`: Fixed issues including unused append results, error string formatting, and empty branches (6 issues)
   - [x] `unused`: Fixed unused variables by removing them from the codebase (6 issues)

2. **Medium Priority Issues:**
   - [ ] `gocritic`: Fix style issues including commented-out code and if-else chains (31 issues)
   - [ ] `revive`: Address naming conventions, unused parameters, and exported function comments (37 issues)
   - [ ] `funlen`: Refactor long functions to improve readability and maintainability (34 issues)

3. **Lower Priority Issues:**
   - [ ] `lll`: Fix long lines exceeding 140 characters (21 issues)
   - [ ] `mnd`: Replace magic numbers with named constants (5 issues)
   - [ ] `goconst`: Extract repeated string literals to constants (1 issue)
   - [ ] `gosec`: Address security-related issues (1 issue)

**Next Steps:**
1. Address medium priority lint issues (`gocritic`, `revive`, `funlen`)
2. Address lower priority lint issues
3. Run final verification to ensure all tests pass and lint errors are resolved

**Progress Tracking:**
- [✓] Fixed failing test in image package (TestDetectImages_StrictMode)
- [✓] Fixed unused foundImageStrings map in collectImageInfo function
- [✓] Removed unused code (excludeRegistries, constants in override package, defaultBinName, deriveRepoKey function)
- [✓] Fixed staticcheck issues (used tagged switch, fixed error string formatting, merged conditional assignment, removed empty branch)
- [✓] No duplication issues currently found in detection_test.go and integration_test.go
- [✓] Fixed integration tests by adding integration test mode and skipping CWD restrictions