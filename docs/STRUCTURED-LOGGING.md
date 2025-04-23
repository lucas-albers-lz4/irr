# Design Document: IRR Logging with `slog`

## 1. Goals

- Replace the previous custom logging implementation with Go's standard structured logging library (`log/slog`).
- Unify all application diagnostic logging under `slog`.
- Enable structured logging (JSON format by default) for improved filterability and machine readability.
- Ensure clear separation between user-facing output (`stdout`) and diagnostic logs (`stderr`).
- Improve the overall maintainability and observability of the logging system.
- Eliminate the legacy `IRR_DEBUG` environment variable in favor of `LOG_LEVEL`.

## 2. Design & Key Decisions

- **Logging Library:** Go standard library's `log/slog` is used for all diagnostic logging.
- **Output Streams:**
    - User-facing output (command results, help text, version info) is written to `stdout`.
    - Diagnostic logs (DEBUG, INFO, WARN, ERROR levels) are written to `stderr`.
- **Default Format:** Logs are structured JSON by default.
- **Timestamp Handling:**
    - By default, JSON logs **omit** the timestamp field to save space.
    - During test execution using the `pkg/testutil.CaptureJSONLogs` helper, timestamps **are included** to facilitate test assertions. This is controlled internally via the `pkg/log.SetTestModeWithTimestamps` function.
- **Log Levels:** Controlled by the `LOG_LEVEL` environment variable (values: `DEBUG`, `INFO`, `WARN`, `ERROR`). Defaults to `INFO`. The legacy `IRR_DEBUG=1` variable is no longer supported.
- **Alternative Format:** A plain text format is available for local debugging or backward compatibility, activated by setting the `LOG_FORMAT` environment variable.
- **Testability:** Test helpers (`pkg/testutil.CaptureJSONLogs`, `AssertLogContainsJSON`) are provided to capture and assert against structured JSON log output, enabling more robust testing compared to parsing plain text.

## 3. Configuration

- **Log Level:** Set the `LOG_LEVEL` environment variable.
  ```bash
  export LOG_LEVEL=DEBUG # Or INFO, WARN, ERROR
  ```
- **Log Format:** Set the `LOG_FORMAT` environment variable.
  ```bash
  export LOG_FORMAT=text # For plain text logs
  # Unset or set to "json" (or anything else) for default JSON logs
  ```

## 4. Verification Criteria

- User-facing output appears **only** on `stdout`.
- Diagnostic logs appear **only** on `stderr`.
- Logs default to JSON format without timestamps in normal execution.
- Logs can be configured to plain text format using `LOG_FORMAT=text`.
- Log verbosity is controlled by `LOG_LEVEL`.
- All tests (`make test`) pass, verifying log structure and output separation under test conditions (where JSON logs include timestamps).

## 5. Implementation Notes (High-Level)

- The core logic resides in `pkg/log/log.go`, which wraps `slog`.
- `configureLogger` in `pkg/log/log.go` sets up the appropriate `slog.Handler` based on `LOG_FORMAT` and the internal test mode flag.
- The `ReplaceAttr` option of `slog.HandlerOptions` is used to conditionally remove the timestamp (`slog.TimeKey`) for the default JSON handler.
- Test helpers in `pkg/testutil/log_capture.go` manage the log output redirection and setting the test mode flag for timestamp inclusion during capture.

## 6. Future Considerations

- **Enforce Standard Log Fields:** Consider systematically adopting and enforcing standard fields like `component` or `operationID` across all logs.
    - **Advantage:** Improves consistency and allows easier filtering, correlation, and analysis of logs across different application components or user actions. For example, filtering all logs related to the `override` command (`component=override`) or tracking a specific validation task (`operationID=abc-123`).
- **Component-Specific Logging Review:** Review logging within specific components (e.g., `pkg/chart/generator.go`) for potential improvements or clarity.
    - **Advantage:** Ensure logs from complex components provide the most relevant and actionable information for debugging or understanding behaviour specific to that component.