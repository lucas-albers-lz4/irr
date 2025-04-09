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

## Phase 3: Active Development - Linting & Refinement (In Progress)

**Goal:** Systematically eliminate lint errors while ensuring all tests pass.

**General Workflow:**
1.  **Pre-Verification:** Run the specific linter command and `go test ./...` to confirm the starting state.
2.  **Action:** Fix the reported lint errors for the targeted category.
3.  **Post-Verification:** Rerun the specific linter command (expecting no errors for that category) and `go test ./...` (expecting all tests to pass).

**Note on Running Specific Tests:** To run a single test function (e.g., `TestMyFunction`), use the `-run` flag with a regex matching the function name: `go test ./... -v -run "^TestMyFunction$"`. This is useful for isolating failures during debugging.

**Note on Linter Commands:** We use `golangci-lint run --enable-only=<linter> ./... | cat` to isolate the output for the specific linter category being addressed in each step.

---

### Step 1: Fix Critical Error Handling Linters (Highest Priority)

1.  **Address `errcheck` Warnings:**
    *   **Status:** [✓] Completed (Suppressed via `#nolint`)
    *   **Issue:** Unchecked errors can lead to unexpected behavior or panics. 1 instance reported (was 10).
    *   **Files:** `cmd/irr/main.go`.
    *   **Note:** Error ignored intentionally using `#nolint:errcheck` as Cobra handles errors/exit internally.
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=errcheck ./... | cat
        go test ./...
        ```
    *   **Action:** Add error handling (e.g., `if err != nil`) for all reported unchecked errors (type assertions, `regexp.MatchString`, `os.Getwd`, `cmd.Run`).
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=errcheck ./... | cat # Expect no output
        go test ./... # Expect PASS
        ```

2.  **Address `errorlint` Warnings:**
    *   **Status:** [✓] Completed
    *   **Issue:** Incorrect error type checking can miss wrapped errors. 1 instance reported.
    *   **File:** `test/integration/harness.go`.
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=errorlint ./... | cat
        go test ./...
        ```
    *   **Action:** Use `errors.As` for type assertion on `*exec.ExitError` instead of direct type assertion.
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=errorlint ./... | cat # Expect no output
        go test ./... # Expect PASS
        ```

3.  **Address `wrapcheck` Warnings:**
    *   **Status:** [✓] Completed
    *   **Issue:** Unwrapped errors lose context, making debugging harder. 3 instances reported (no change).
    *   **Files:** `cmd/irr/root.go`, `cmd/irr/test_helpers_test.go`.
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=wrapcheck ./... | cat
        go test ./...
        ```
    *   **Action:** Wrap errors returned from external packages (`mock.Arguments.Error`, `image.ParseImageReference`, `cmd.CombinedOutput`) using `fmt.Errorf("...: %w", err)`.
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=wrapcheck ./... | cat # Expect no output
        go test ./... # Expect PASS
        ```

### Step 2: Remove Unused Code (High Priority)

1.  **Review and Remove `unused` Code:**
    *   **Status:** [ ] Pending (Re-run Required)
    *   **Issue:** Dead code increases maintenance overhead. 20 instances reported after blocker fix (includes the fix itself). Previous attempt caused issues. Requires careful removal and verification.
    *   **Files:** Widespread (check `make lint` output, e.g., `cmd/irr/root.go`, `pkg/image/parser.go`, `pkg/image/path_patterns.go`, `pkg/image/validation.go`, `test/integration/harness.go`, `test/integration/integration_test.go`).
    *   **Pre-Verification:**
        *   **Capture Lint Errors:** `golangci-lint run --enable-only=unused ./... | cat > unused_errors.txt` (Capture the specific list for careful review).
        *   **Check Tests:** `go test ./...` (Ensure tests pass before starting).
    *   **Action:**
        *   Carefully review the captured list in `unused_errors.txt`.
        *   Systematically remove identified unused variables, functions, constants, or types.
        *   **Crucially:** Before removing items reported in `cmd/irr/root.go` (check flags/vars) or `pkg/image/validation.go` (check `validIdentifierRegex`/`isValidIdentifier`), verify their context to avoid breaking builds or necessary logic.
        *   If unsure about removal, consider adding `#nolint:unused // TODO: Re-evaluate necessity` for later review instead of removing immediately.
    *   **Post-Verification:**
        *   **Check Linter:** `golangci-lint run --enable-only=unused ./... | cat` (Expect no output).
        *   **Check Tests:** `go test ./...` (Expect PASS).
        *   **If Fails:** Revert changes (e.g., `git checkout -- .`) and retry, possibly addressing items individually.

### Step 3: Address Medium Priority Linting Issues

1.  **Fix `gosec` Security Warnings:**
    *   **Status:** [ ] Pending
    *   **Issue:** Potential security vulnerabilities. 6 instances reported (no change).
    *   **Files:** `cmd/irr/override_test.go`, `pkg/chart/generator_test.go`, `test/integration/harness.go`.
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=gosec ./... | cat
        go test ./...
        ```
    *   **Action:** Review file permissions (aim for `0o600`/`0o700` where appropriate, use constants), analyze potential file inclusion (`os.ReadFile` - ensure paths aren't user-controlled), review subprocess calls (`exec.Command` - ensure arguments are properly sanitized if dynamic). Add `#nosec` directives with justification if warnings are false positives after careful review.
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=gosec ./... | cat # Expect no output (or only justified #nosec)
        go test ./... # Expect PASS
        ```

2.  **Refactor `funlen` Long Functions:**
    *   **Status:** [ ] Pending
    *   **Issue:** Long functions are harder to read, test, and maintain. 30 functions reported (no change).
    *   **Files:** Widespread (check `make lint` output). Prioritize `TestParseImageReference`, `TestOverrideCmdExecution`, `TestSetValueAtPath` (`pkg/override/path_utils_test.go`).
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=funlen ./... | cat
        go test ./...
        ```
    *   **Action:** Break down functions exceeding ~60 lines / 40 statements into smaller, focused helper functions. Aim for single responsibility.
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=funlen ./... | cat # Expect significantly fewer or no errors
        go test ./... # Expect PASS
        ```

3.  **Fix `gocritic` Style Issues:**
    *   **Status:** [ ] Pending
    *   **Issue:** Code style inconsistencies and potential anti-patterns. 24 issues reported (was 25).
    *   **Files:** Widespread (check `make lint` output).
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=gocritic ./... | cat
        go test ./...
        ```
    *   **Action:** Apply suggested fixes: use `0o` octal literals, convert `if-else` chains to `switch` where appropriate, remove commented-out code blocks, name results where needed, combine parameters with the same type, fix `tooManyResultsChecker` by refactoring or using a struct return.
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=gocritic ./... | cat # Expect no output
        go test ./... # Expect PASS
        ```

4.  **Fix `dupl` Code Duplication:**
    *   **Status:** [ ] Pending
    *   **Issue:** Duplicated code increases maintenance effort and risk of bugs. 4 instances reported (no change).
    *   **File:** `pkg/image/detection_test.go`.
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=dupl ./... | cat
        go test ./...
        ```
    *   **Action:** Refactor duplicated test setup/case blocks into table-driven tests or shared helper functions.
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=dupl ./... | cat # Expect no output
        go test ./... # Expect PASS
        ```

### Step 4: Address Low Priority Linting Issues

1.  **Fix `revive` Issues:**
    *   **Status:** [ ] Pending
    *   **Issue:** Style issues, missing comments, potential logic errors. 38 issues reported (no change).
    *   **Files:** Widespread (check `make lint` output).
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=revive ./... | cat
        go test ./...
        ```
    *   **Action:** Add missing package/exported comments, fix error string formatting (lowercase, no punctuation), address unused parameters (use `_`), fix indentation errors (e.g., `indent-error-flow`), simplify var declarations.
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=revive ./... | cat # Expect no output
        go test ./... # Expect PASS
        ```

2.  **Fix `lll` Line Length Issues:**
    *   **Status:** [ ] Pending
    *   **Issue:** Long lines reduce readability. 12 instances reported (was 13).
    *   **Files:** Widespread (check `make lint` output).
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=lll ./... | cat
        go test ./...
        ```
    *   **Action:** Break long lines logically (e.g., at operators, parameters).
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=lll ./... | cat # Expect no output
        go test ./... # Expect PASS
        ```

3.  **Fix `mnd` Magic Numbers:**
    *   **Status:** [ ] Pending
    *   **Issue:** Unnamed numbers obscure intent. 5 instances reported (was 6).
    *   **Files:** `cmd/irr/override.go`, `pkg/chart/generator.go`, `pkg/image/validation.go`.
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=mnd ./... | cat
        go test ./...
        ```
    *   **Action:** Replace numbers (e.g., `0o644`, `100`, `128`, `0o600`) with named constants (e.g., `defaultFilePerm`, `percentageMultiplier`, `maxTagLength`). Define constants appropriately (e.g., file modes in `fs` or os, lengths near validation).
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=mnd ./... | cat # Expect no output
        go test ./... # Expect PASS
        ```

4.  **Fix `nilerr` Issue:**
    *   **Status:** [✓] Completed
    *   **Issue:** Returning nil when an error exists. 1 instance reported (no change).
    *   **File:** `pkg/image/detector.go`.
    *   **Pre-Verification:**
        ```bash
        golangci-lint run --enable-only=nilerr ./... | cat
        go test ./...
        ```
    *   **Action:** Ensure the error is propagated correctly in the non-strict mode logic path identified by the linter.
    *   **Post-Verification:**
        ```bash
        golangci-lint run --enable-only=nilerr ./... | cat # Expect no output
        go test ./... # Expect PASS
        ```

### Step 5: Final Validation

1.  **Run All Tests:**
    *   **Command:** `go test ./... -v`
    *   **Expected:** All tests PASS.
2.  **Run Full Linter Suite:**
    *   **Command:** `make lint` or `golangci-lint run ./...`
    *   **Expected:** No lint errors reported (Exit Code 0).
3.  **Manual Smoke Test:**
    *   **Command:** `go run ./cmd/irr/main.go --help`
    *   **Expected:** Help message displays correctly without errors.
    *   **Command:** Try basic `analyze` and `override` commands on a simple test chart.
    *   **Expected:** Commands execute without errors and produce expected output/files.

### Current Status & Observations (as of last interaction)

**Blocking Issue:**
*   **`typecheck` Error:** Compilation is currently blocked by an `undefined: validIdentifierRegex` error in `pkg/image/validation.go`. **This must be resolved before proceeding with other linting or testing.** This likely resulted from an incorrect removal during the `unused` code cleanup.

**Progress:**
*   **`typecheck`:** Blocker fixed (added `validIdentifierRegex`).
*   **Import Path & Function Call:** Fixed incorrect import path `github.com/lalbers/irr/test/testutil` to `github.com/lalbers/irr/pkg/testutil` in `test/integration/integration_test.go`. Corrected the call in `TestMain` from `testutil.BuildIrrBinary` to the local `buildIrrBinary` function, resolving the subsequent `undefined` error.
*   **`errcheck`:** Fixed (suppressed via `#nolint` in `cmd/irr/main.go`).
*   **`errorlint`:** Fixed.
*   **`nilerr`:** Fixed.
*   **`wrapcheck`:** Fixed.
*   **`unused`:** Partially addressed previously, but caused blocker. 20 items now reported including the fix. Needs careful re-run.

**Next Steps & Refined Workflow:**
1.  **[BLOCKER] Resolve `typecheck` Error:** Fix the `undefined: validIdentifierRegex` error in `pkg/image/validation.go`. Determine if `isValidIdentifier` is still needed and either define the regex or remove the function and its usages.
2.  **Re-verify Test State:** Run `go test ./...` to check for any test failures. Address any failures.
3.  **Re-verify Full Lint State:** Run `golangci-lint run ./...` to get an accurate list of *all* current lint errors.
4.  **Update TODO Statuses:** Update the specific linter steps based on the accurate lint/test results, marking any newly passing linters as complete.
5.  **Resume Linting (Order TBD):** Proceed with fixing remaining lint errors one category at a time, adhering strictly to the refined pre/post verification:
    *   **Pre-Verification:** Run `golangci-lint run --enable-only=<linter> ./... | cat` AND `go test ./...`.
    *   **Action:** Fix reported errors for the target linter.
    *   **Post-Verification:** Run `golangci-lint run --enable-only=<linter> ./... | cat` (expect no errors for this linter) AND `go test ./...` (expect PASS).

---
**Previous Progress Snippets (Historical):**
✓ Fixed command structure, exit codes, logging.
✓ Addressed initial critical bugs & test failures.
✓ Refactored core components (`detection.go`, `registry`).

**Note:** The debug flag (`-debug` or `DEBUG=1`) can be used during testing and development to enable detailed logging.