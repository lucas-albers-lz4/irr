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
*   **Test Failures:** Two failing tests in `cmd/irr`:
    *   **`TestOverrideCmdArgs/valid_flags_with_dry_run`:** Fails because the test attempts to use an in-memory filesystem with `setupTestFS` for chart creation, but the actual command execution can't access this filesystem when trying to load the chart.
    *   **`TestOverrideCommand_Success`:** Similar issue where output file isn't being created in the in-memory filesystem.
    *   **Resolution:**
        1.  **[In Progress]** Fix `cmd/irr` filesystem handling by ensuring proper setup and teardown of the in-memory filesystem (`afero.Fs`) and making it accessible to the command execution code.
        2.  **[Pending]** Fix Integration Tests: Once `cmd/irr` tests pass, re-run `make test`. Address any remaining integration test failures.
    *   **Priority:** Fixing the filesystem handling in cmd/irr tests is the current priority.
*   **Lint Errors:** `make lint` reports numerous issues across various categories.

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
    *   **Status:** [ ] Pending (6 issues reported)
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