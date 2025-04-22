# Design Document: Migrate IRR Logging to `slog`

## 1. Goals

- Replace the custom logging in `pkg/log/log.go` with Go's standard `log/slog`.
- Unify all application logging (including the old `debug` mechanism) under `slog`.
- Enable structured logging (JSON by default) for better filterability and machine readability.
- Provide a transition path for tests dependent on plain text logs.
- Improve maintainability and observability of the logging system.

## 2. Key Decisions & Standards

- **All logs use `slog` and are structured (JSON by default).**
- **User-facing output (help, version, results) goes to `stdout`.**
- **Diagnostic logs (INFO, WARN, ERROR, DEBUG) go to `stderr`.**
- **Log format is controlled by the `LOG_FORMAT` environment variable:**
  - `json` (default)
  - `text` (for local debugging/backward compatibility)
- **Standard log fields** (e.g., `component`, `operationID`) should be used consistently.

## 3. Migration Summary (Phases 1-4 Completed)

- Core `slog` implementation in `pkg/log`.
- Removal of `fmt.Fprintf` and the old `pkg/debug`/`IRR_DEBUG` system.
- Logging format configurable via `LOG_FORMAT`.
- Test helpers (`CaptureJSONLogs`, `AssertLogContainsJSON`, etc.) created for JSON log output.
- Integration tests migrated to use structured log assertions.
  - **Long-term Strategy:** This enables more accurate, robust, and easily refactorable feature tests by verifying specific structured data points rather than parsing plain text.
- Documentation updated.

## 4. Phase 5: Test Remediation & Output Audit (Current Work)

- **Goal:** Ensure all tests pass with JSON log output and user-facing output is separated from logs.
- **Actions:**
  - Investigate and fix `TestVersionCommand` and `TestDebugFlagLowerPrecedence` failures in `cmd/irr/root_test.go`.
  - Audit all commands for correct use of `stdout` (user output) and `stderr` (logs).
  - Review and update all log assertions in tests for correct structure and content.
  - Document `LOG_FORMAT` in user-facing docs and CLI help.
- **Development Workflow:**
  - Run `make test` and `make lint` after completing each logical refactor step.
  - Prioritize fixing issues based on current refactoring stage, focusing on critical failures blocking progress.
  - Developer determines optimal grouping of changes and testing cadence.

## 5. Verification Criteria

- All tests (`make test`) pass with no log assertion failures.
- User-facing output appears **only** on `stdout`.
- Diagnostic logs appear **only** on `stderr` (JSON by default).
- Manual and/or automated checks confirm output separation for key commands (`help`, `version`, `inspect`, etc.) in all modes and log levels.

## 6. Outstanding Tasks

- [x] **Fix Output Stream Separation Issues:**
    - [x] Refactor `inspect` command output.
    - [x] Refactor `override --dry-run` command output.
    - [x] Refactor `validate` command output.
- [x] **Final Verification:** Re-run manual checks for `inspect`, `override --dry-run`, `validate` after fixes.
- [x] **Documentation:** Update user-facing docs regarding `LOG_FORMAT` and output separation guarantees.
- [ ] **Code Polish (Optional):** Review `pkg/chart/generator.go` logging for potential improvements.

## 7. Open Questions/Considerations

- Wrapper functions: Continue using `slog`-style `Info(msg, keyValues...)` for clarity.
- Standard fields: Expand and enforce use of standard log fields for structured data.
- Error logging: Ensure consistent use of `slog.Error("message", "error", err)`.

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
- [x] Investigate and fix `TestVersionCommand` log assertion failure.
- [x] Investigate and fix `TestDebugFlagLowerPrecedence` log assertion failure.
- [x] Investigate and fix `TestParentChart` assertion failure.
- [x] Audit Stdout vs. Stderr usage across commands.
    - [x] **`inspect`:** Mixes YAML output (stdout) with JSON logs (stderr). Informational messages use direct print, not `log.Info`. -> **Fixed**
    - [x] **`override --dry-run`:** Embeds YAML output within an `INFO` log message (stderr) instead of printing to `stdout`. -> **Fixed**
    - [x] **`validate`:** Mixes rendered YAML output (stdout) with JSON logs/Helm warnings (stderr). -> **Fixed**
- [x] Audit remaining log assertions in tests. (*Note: `pkg/chart/generator.go` logging could be improved*)
- [x] Document `LOG_FORMAT` configuration.
- [x] Verify `make test` passes cleanly.
- [x] Verify CLI output separation.
    - **Goal:** Confirm user-facing output (help, version, results) goes to `stdout`; diagnostic logs (INFO, WARN, ERROR, DEBUG) go to `stderr` (JSON default).
    - **Method:** Run key commands (`--help`, `--version`, `inspect`, `override --dry-run`, `validate`) in standalone & plugin modes, with default (`INFO`) and `DEBUG` log levels. Capture `stdout` and `stderr` separately for analysis.
    - **Status:** Verified for `--help`, `--version`, `inspect`, `override --dry-run`, `validate`.

## Completion Summary

The core migration to `slog`

## Phase 6: Remove IRR_DEBUG Support (Staged Approach)

- **Goal:** Eliminate the redundant legacy `IRR_DEBUG` environment variable to simplify logging configuration and ensure documentation consistency. Rely solely on `LOG_LEVEL` for controlling log verbosity.
- **Rationale:** `IRR_DEBUG=1` provides the same functionality as `LOG_LEVEL=DEBUG` but adds an extra configuration vector and has led to documentation inconsistencies. Removing it streamlines the logging system.

- **Actions (Staged):**

    **Stage 1: Preparation and Audit**
    - Search the codebase for all uses of `IRR_DEBUG` (code, tests, docs, scripts).
    - Document all locations and usages to inform the next steps.
    - Update documentation to clarify that `LOG_LEVEL` is the only supported debug control going forward.

    **Stage 2: Update Test and Build Infrastructure**
    - Update `Makefile`, CI scripts, and any other build/test execution commands to replace `IRR_DEBUG=1` with `LOG_LEVEL=DEBUG`.
    - Update test files to use `LOG_LEVEL=DEBUG` instead of setting/unsetting `IRR_DEBUG`.
    - Run all tests; fix any failures related to this change before proceeding.

    **Stage 3: Remove IRR_DEBUG from Code**
    - Remove the check for `IRR_DEBUG` within the `init()` function in `pkg/log/log.go`.
    - Remove any code that sets/unsets or checks `IRR_DEBUG` in test files.
    - Run all tests; fix any failures related to this change before proceeding.

    **Stage 4: Final Cleanup**
    - Remove all references to `IRR_DEBUG` from documentation files (`TESTING.md`, `DEVELOPMENT.md`, `PLUGIN-SPECIFIC.md`, `LOGGING.md`, `README.md`, `TODO.md`, etc.).
    - Search the codebase (`grep`) for any remaining uses of `IRR_DEBUG` and remove them.
    - Run all tests; ensure everything passes.

- **Verification Criteria:**
    - All tests (`make test`) pass after each stage.
    - A codebase search (`grep -R IRR_DEBUG . --exclude-dir=.git --exclude=irr`) yields no relevant results in code or documentation at the end.
    - Manually verify that setting `export IRR_DEBUG=1` does *not* enable debug logging when `LOG_LEVEL` is not set or is set to `INFO` or higher.