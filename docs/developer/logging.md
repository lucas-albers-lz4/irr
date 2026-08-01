# IRR Logging and Debugging Guide

This guide explains how logging works in IRR, how to control it, and how to debug problems. IRR uses the Go standard library `log/slog` package for all diagnostic logging.

## Log Levels

Four log levels are available, in order of increasing severity:

| Level | `slog` constant | Use |
|-------|-----------------|-----|
| DEBUG | `slog.LevelDebug` | Internal operations, call flows, value traces |
| INFO | `slog.LevelInfo` | Command execution, successful operations |
| WARN | `slog.LevelWarn` | Non-critical problems, fallbacks |
| ERROR | `slog.LevelError` | Fatal errors, missing requirements |

Logs are written to **standard error (stderr)**.

## Controlling the Log Level

The effective log level comes from these sources, in order of precedence (highest first):

1. **`--debug` flag** — forces `DEBUG`, overriding all other settings.
2. **`--log-level <level>` flag** — applies only when you set it explicitly, and only when the value is valid.
3. **`LOG_LEVEL` environment variable** — applies when the `--log-level` flag was not set explicitly. Valid values: `DEBUG`, `INFO`, `WARN`, `ERROR`.
4. **Default level** — `INFO` in all modes (standalone, Helm plugin, and test runs).

Example commands:

```bash
# Default: INFO and above
irr <command>

# See DEBUG, INFO, WARN, ERROR logs
irr <command> --debug
# Or:
irr <command> --log-level debug
# Or:
LOG_LEVEL=DEBUG irr <command>

# See only WARN and ERROR logs
LOG_LEVEL=WARN irr <command>
```

The legacy `IRR_DEBUG` environment variable is removed. Use `LOG_LEVEL` instead.

## Log Format

`LOG_FORMAT` controls the output format:

- `json` (default) — structured JSON via `slog.JSONHandler`. Use this for machine parsing and log aggregation.
- `text` — human-readable key-value pairs via `slog.TextHandler`. Set `LOG_FORMAT=text` explicitly.

```bash
# Default JSON format (no variable needed)
irr <command>

# Explicit text format
LOG_FORMAT=text irr <command>
```

In normal execution, JSON logs omit the timestamp field to save space. Test helpers include timestamps so test assertions can rely on them.

## Output Streams: Logs (stderr) vs Results (stdout)

The two output streams never mix:

- **stderr** — diagnostic logs only (`log.Debug`, `log.Info`, `log.Warn`, `log.Error`).
- **stdout** — command results only: generated override files, inspection results, validation results, help text.

This separation keeps diagnostic logs from interfering with output that you pipe to other commands.

## Execution Mode Detection

IRR detects whether it runs as a standalone binary or as a Helm plugin. Detection happens in `cmd/irr/main.go`:

```go
// isRunningAsHelmPlugin checks if the program is being run as a Helm plugin
func isRunningAsHelmPlugin() bool {
    // Check for environment variables set by Helm when running a plugin
    return os.Getenv("HELM_PLUGIN_NAME") != "" || os.Getenv("HELM_PLUGIN_DIR") != ""
}
```

At startup, IRR logs its execution mode and version. To see them:

```bash
LOG_LEVEL=DEBUG irr help
LOG_LEVEL=DEBUG helm irr help
```

## Troubleshooting

### The `--debug` Flag is Deprecated

The `--debug` command-line flag is deprecated and may be removed in a future version. Use `LOG_LEVEL=DEBUG` instead.

### Verbose Environment Information

To see the environment variables used at startup:

```bash
LOG_LEVEL=DEBUG irr help 2>&1 | grep "msg=\"Detected environment variable\""
```

### Checking the Binary Location

To verify which binary runs as the Helm plugin:

```bash
# Identify the Helm plugin path
helm plugin list | grep irr

# Check if the binary exists
ls -l ~/Library/helm/plugins/irr/bin/irr

# Confirm by removing the binary temporarily
mv ~/Library/helm/plugins/irr/bin/irr ~/Library/helm/plugins/irr/bin/irr.bak
helm irr help  # This should fail with a "no such file or directory" error
mv ~/Library/helm/plugins/irr/bin/irr.bak ~/Library/helm/plugins/irr/bin/irr
```

### Verbose Helm Plugin Environment

To see the Helm environment variables when running as a plugin:

```bash
LOG_LEVEL=DEBUG helm irr help 2>&1 | grep "HELM_"
```

## For Developers: Logging Best Practices

1. Use the logging functions from the `pkg/log` package:
   - `log.Debug(msg string, args ...any)`
   - `log.Info(msg string, args ...any)`
   - `log.Warn(msg string, args ...any)`
   - `log.Error(msg string, args ...any)`
2. Provide meaningful static messages (`msg`).
3. Add context with structured key-value pairs (`args`). Keys must be strings.

```go
log.Info("Chart processing complete", "chartPath", chartPath, "imageCount", 42)
log.Error("Failed to load mappings", "file", configFile, "error", err)
```

4. Logs go to `stderr`. Write user-facing results to `stdout` with `fmt.Println` or similar.

## Testing Logging

### Unit Test Helpers

Use the helpers in `pkg/testutil`:

- **`CaptureLogOutput(level, func) (string, error)`** — captures text log output at the specified level.
- **`CaptureJSONLogs(level, func) ([]map[string]interface{}, error)`** — captures JSON log output, parsing each line.
- **`AssertLogContainsJSON(t, logs, expectedFields, ...)`** — asserts that at least one JSON log entry contains the specified fields.

```go
logs, err := testutil.CaptureJSONLogs(log.LevelWarn, func() {
    log.Warn("Something might be wrong", "status", 123)
})
require.NoError(t, err)
require.Len(t, logs, 1)
assert.Equal(t, "WARN", logs[0]["level"])
```

Note: JSON numbers decode as `float64`. Compare `123.0`, not `123`.

### Integration Tests

Set environment variables to control logging:

```bash
# Run tests with debug logging and JSON format
LOG_LEVEL=DEBUG LOG_FORMAT=json go test -v ./test/integration/...
```

Set `LOG_FORMAT=json` in the test environment if the test logic parses log output as JSON.

## Design History

The logging system moved from a custom implementation to Go's `log/slog` with these decisions:

- **Library:** `log/slog` for all diagnostic logging.
- **Output streams:** results to `stdout`, diagnostics to `stderr`.
- **Default format:** structured JSON.
- **Timestamps:** omitted by default; included during test capture via `pkg/log.SetTestModeWithTimestamps`.
- **Level control:** `LOG_LEVEL` environment variable (`DEBUG`, `INFO`, `WARN`, `ERROR`), default `INFO`. The legacy `IRR_DEBUG=1` variable is no longer supported.
- **Testability:** `pkg/testutil` capture helpers make structured assertions possible.

Future considerations recorded in the original design: standard log fields such as `component` or `operationID` for cross-component filtering, and a component-specific review of complex packages such as `pkg/chart/generator.go`.
