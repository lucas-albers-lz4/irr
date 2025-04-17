# IRR Logging and Debugging Guide

## Overview

This document provides a comprehensive guide to logging and debugging in the IRR tool, including how to:
- Enable debug logging
- Understand execution mode detection (Standalone vs Helm Plugin)
- Control log levels
- Troubleshoot common logging issues

## Log Levels

IRR uses four log levels, in order of increasing severity:

| Level | Description | Usage |
|-------|-------------|-------|
| DEBUG | Detailed information for troubleshooting | Internal operations, call flows, value traces |
| INFO | General operational messages | Command execution, successful operations |
| WARN | Potential issues that don't prevent operation | Non-critical problems, fallbacks |
| ERROR | Serious issues that prevent operation | Fatal errors, missing requirements |

## Enabling Debug Logging

There are two ways to enable debug logging:

### 1. Command-line Flag

```bash
# Standalone mode
irr --debug <command>

# Helm plugin mode
helm irr <command> -- --debug  # Note: This method may not work reliably

# Alternative for Helm plugin mode
env IRR_DEBUG=1 helm irr <command>
```

### 2. Environment Variable

```bash
# Enable debug logging using environment variable
IRR_DEBUG=1 irr <command>
IRR_DEBUG=true irr <command>
IRR_DEBUG=yes irr <command>

# For Helm plugin mode
IRR_DEBUG=1 helm irr <command>
```

Note: The `--debug` flag takes precedence over the `IRR_DEBUG` environment variable.

## Execution Mode Detection

IRR automatically detects whether it's running as a standalone binary or as a Helm plugin.

### Detection Mechanism

The detection happens in `cmd/irr/main.go` with the `isRunningAsHelmPlugin()` function:

```go
// isRunningAsHelmPlugin checks if the program is being run as a Helm plugin
func isRunningAsHelmPlugin() bool {
    // Check for environment variables set by Helm when running a plugin
    return os.Getenv("HELM_PLUGIN_NAME") != "" || os.Getenv("HELM_PLUGIN_DIR") != ""
}
```

### Checking Execution Mode

IRR clearly indicates its execution mode and version at startup:

```bash
# When running in standalone mode
irr help
# Output includes: IRR v0.0.5 running in standalone mode

# When running as a Helm plugin 
helm irr help
# Output includes: IRR v0.0.5 running as Helm plugin
```

For more detailed information with debug logging enabled:

```bash
# When running in standalone mode with debug enabled
irr --debug help
# Output includes: [DEBUG +0s] Execution Mode Detected: Standalone

# When running as a Helm plugin with debug enabled
IRR_DEBUG=1 helm irr help
# Output includes: [DEBUG +0s] Execution Mode Detected: Plugin
```

This makes it easy to confirm which mode the application is running in and which version is being used.

## Debug Output Format

Debug logs have a specific format to help identify the source and timing of messages:

```
[DEBUG +time_since_start] Message
```

For example:
```
[DEBUG +0s] Debug package enabled: true
[DEBUG +23ms] Execution Mode Detected: Plugin
```

The time offset can help identify slow operations or bottlenecks.

## Troubleshooting

### Debug Flag Not Working in Helm Plugin Mode

When using Helm's pass-through mechanism (`helm irr command -- --debug`), the debug flag may not be correctly passed to the plugin. Instead, use one of these alternatives:

```bash
# Use environment variable instead
IRR_DEBUG=1 helm irr command

# Or HELM_DEBUG variable
HELM_DEBUG=true helm irr command
```

### Verbose Environment Information

For maximum debugging information, including all environment variables:

```bash
IRR_DEBUG=1 helm irr help 2>&1 | grep -i "ENV:"
```

This will show all environment variables, which can be useful for troubleshooting Helm plugin detection issues.

### Checking Binary Location

To verify which binary is being executed when running as a Helm plugin:

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
IRR_DEBUG=1 helm irr help 2>&1 | grep "HELM_"
```

## For Developers: Logging Best Practices

When adding new code to IRR, follow these guidelines for logging:

1. Use the appropriate log level:
   - `log.Debugf()` for detailed troubleshooting information
   - `log.Infof()` for normal operation status
   - `log.Warnf()` for non-fatal issues
   - `log.Errorf()` for fatal problems

2. For debug-specific logs, use the debug package:
   ```go
   debug.Printf("Detailed information only relevant for debugging")
   ```

3. Check debug status before expensive operations:
   ```go
   if debug.Enabled {
       expensiveDetailedLogging()
   }
   ```

4. For mode-specific code, check mode first:
   ```go
   if isHelmPlugin {
       // Plugin-specific operations
   } else {
       // Standalone-specific operations
   }
   ```

## Testing Logging/Debugging

### Unit Test Examples

To test debug logging in unit tests:

```go
// Save original state
origDebugEnabled := debug.Enabled
defer func() { debug.Enabled = origDebugEnabled }()

// Set for this test
debug.Enabled = true

// Capture debug output
oldStderr := os.Stderr
r, w, _ := os.Pipe()
os.Stderr = w
defer func() { os.Stderr = oldStderr }()

// Call function that should log
yourFunction()

// Capture output
w.Close()
var buffer bytes.Buffer
io.Copy(&buffer, r)
output := buffer.String()

// Check debug logging occurred
assert.Contains(t, output, "Expected debug message")
```

### Integration Test Examples

For integration tests, use the environment variable approach:

```bash
# Run test with debug enabled
IRR_DEBUG=true go test -v ./test/integration/...
```

Or set environment variables within the test:

```go
os.Setenv("IRR_DEBUG", "true")
defer os.Unsetenv("IRR_DEBUG")
``` 