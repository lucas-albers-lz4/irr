# Design Document: Migrate IRR Logging to `slog`

## 1. Goals

*   Replace the current custom logging implementation in `pkg/log/log.go` with the standard library's `log/slog`.
*   Unify all application logging (including the separate `debug` mechanism identified in `docs/LOGGING.md`) under `slog`.
*   Enable structured logging (primarily JSON) for better filterability, machine readability, and granularity.
*   Provide a transition path that allows existing feature tests (dependent on plain text logs) to pass while migrating them incrementally to support structured logs.
*   Improve overall maintainability and observability of the logging system.

## 2. Migration Summary (Phases 1-4 Completed)

The migration involved several phases, now complete:

*   **Core Implementation:** The `pkg/log` package was refactored to use `slog` as the backend. The old `fmt.Fprintf` logic and the separate `pkg/debug` mechanism (and `IRR_DEBUG` environment variable) were removed and all call sites updated.
*   **Configurable Format:** Logging format became configurable via the `LOG_FORMAT` environment variable, supporting `text` (for local debugging and backward compatibility during migration) and `json`. The default format was switched to `json`.
*   **Test Migration:** Test helpers (`CaptureJSONLogs`, `AssertLogContainsJSON`, etc. in `pkg/testutil`) were created to handle JSON log output. Integration tests previously asserting plain-text log output were migrated to use these helpers and validate structured JSON logs.

## 3. Phase 5: Test Remediation & Output Audit (Current Work)

While the core migration is complete and most tests were updated, final testing revealed inconsistencies in log output or test assertions for specific commands. This phase addresses those remaining issues and ensures correct output separation.

*   **Goal:** Ensure all tests relying on log capture pass reliably with the `slog` JSON output, and that user-facing output is clearly separated from diagnostic logs.
*   **Actions:**
    *   **Investigate `TestVersionCommand` (`cmd/irr/root_test.go`):** This test fails because the expected "Version details" log entry is not captured. Determine if the `version` command logs this information to stderr via `slog` or prints it differently (e.g., stdout). Adjust the test to capture/assert the correct output or modify the command's output mechanism if necessary.
    *   **Investigate `TestDebugFlagLowerPrecedence` (`cmd/irr/root_test.go`):** This test fails because the expected `msg: "Debug logging enabled"` log entry is not captured when running `irr --debug help`. Determine if this specific message is still logged, logged at a different time, or has different content. Adjust the test assertion accordingly.
    *   **Audit Stdout vs. Stderr Usage:** Review command implementations to ensure that:
        *   User-facing results, summaries, and normal output (e.g., command results like lists or statuses, `version` output, `help` text) go directly to `os.Stdout` (e.g., using `fmt.Fprintln(os.Stdout, ...)`).
        *   Diagnostic information (debug, info, warnings, errors) goes through the `pkg/log` logger (which defaults to JSON on `os.Stderr`).
    *   **Audit Log Assertions:** Perform a final review of all tests using `testutil.CaptureJSONLogs` or similar helpers to ensure they correctly assert against the actual `slog` JSON output structure and content (e.g., `msg` field, uppercase levels, correct messages).
    *   **Document `LOG_FORMAT`:** Update user-facing documentation (e.g., `docs/LOGGING.md`, potentially CLI help) to explain the `LOG_FORMAT` environment variable (`text`/`json`) for controlling log output on stderr.
*   **Verification:**
    *   `make test` passes consistently with no failures related to log capture or assertion.
    *   Verify CLI output separation via manual testing (running common commands like `version`, `help`, typical workflows) and potentially add a simple integration test asserting distinct stdout/stderr content. Ensure stdout contains only expected user output (text) and stderr contains logs (JSON by default).

## 4. Open Questions/Considerations (Historical & Future)

*   **Wrapper Functions:** Switched to `slog`-style `Info(msg, keyValues...)` for clarity and efficiency.
*   **Standard Fields:** Define and consistently use standard fields (e.g., `component`, `operationID`) where possible for better structured data. *(Ongoing improvement, not a Phase 5 blocker)*
*   **Error Logging:** Ensure consistent use of `slog.Error("message", "error", err)` for logging errors. *(Ongoing improvement)*
*   **Performance:** `slog` is generally performant; wrappers in `pkg/log` seem minimal. *(Monitor if needed)*

## Migration Status (as of latest update)

### Phase 1-4: Core Migration & Initial Test Updates
- [x] Core `slog` implementation in `pkg/log`.
- [x] Removal of `pkg/debug` and `IRR_DEBUG`.
- [x] Configurable `LOG_FORMAT` (default `json`).
- [x] Feature tests migrated to JSON helpers (`pkg/testutil`).
- [x] Documentation (`docs/LOGGING.md`) updated.
- [x] Codebase cleaned of old logging mechanisms.
- [x] Linting errors related to migration fixed/suppressed.

### Phase 5: Test Remediation & Output Audit
- [ ] Investigate and fix `TestVersionCommand` log assertion failure.
- [ ] Investigate and fix `TestDebugFlagLowerPrecedence` log assertion failure.
- [ ] Audit Stdout vs. Stderr usage across commands.
- [ ] Audit remaining log assertions in tests.
- [ ] Document `LOG_FORMAT` configuration.
- [ ] Verify `make test` passes cleanly.
- [ ] Verify CLI output separation (manual checks + potentially automated test).

## Completion Summary

The core migration to `slog`