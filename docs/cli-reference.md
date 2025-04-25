# CLI Reference

## Command Overview

The `irr` (Image Relocation and Rewrite) tool provides commands and options for inspecting, overriding, validating, and configuring image references in Helm charts.

## Global Flags

These flags are available for all commands:

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `--config` | Config file path | `$HOME/.irr.yaml` | `--config my-config.yaml` |
| `--debug` | Enable debug logging | false | `--debug` |
| `--log-level` | Set log level | info | `--log-level debug` |
| `--help` | Show help | | `--help` |

### Logging and Output Streams

**Log Format:**

The format of diagnostic logs sent to `stderr` can be controlled using the `LOG_FORMAT` environment variable:
- `LOG_FORMAT=json`: (Default) Outputs logs in structured JSON format. Suitable for machine parsing (e.g., in CI/CD).
- `LOG_FORMAT=text`: Outputs logs in a human-readable plain text format. Useful for local debugging.

Example:
```bash
# Run inspect with text-based logs on stderr
LOG_FORMAT=text irr inspect --chart-path ./my-chart
```

**Output Streams:**

`irr` follows standard Unix conventions for output streams:
- **`stdout`**: Used for primary command output. This can be structured data like YAML/JSON analysis results (e.g., `irr inspect`), generated override files (e.g., `irr override`), or rendered templates (`irr validate` without `-o`). The format of this primary output is sometimes controllable via command-specific flags (e.g., `irr inspect --output-format json`).
- **`stderr`**: Used for diagnostic messages, including logs (INFO, WARN, ERROR, DEBUG), progress updates, and error details. The format of this output is controlled by the `LOG_FORMAT` environment variable.

This separation allows you to easily redirect primary command results while still seeing diagnostic logs:
```bash
# Save inspect analysis (YAML by default) to a file, logs still go to terminal (text format)
LOG_FORMAT=text irr inspect --chart-path ./my-chart > analysis.yaml

# Pipe generated overrides (YAML) to another command, logs go to terminal (default json format)
irr override --chart-path ./my-chart --dry-run | kubectl apply -f -
```

**Important Distinction:** Note that `LOG_FORMAT` controls the format of logs on `stderr`. It does *not* change the format of primary command output on `stdout`. For commands like `irr inspect` that produce structured data, use the command's specific flags (e.g., `--output-format`) to control the `stdout` format.

## Commands

### config

Configure registry mappings for image redirects. This command allows you to view, add, update, or remove registry mappings stored in a YAML configuration file (`registry-mappings.yaml` by default).

```bash
irr config [flags]
```

#### Flags for config

| Flag           | Description                                                  | Default                  | Example                                          |
| -------------- | ------------------------------------------------------------ | ------------------------ | ------------------------------------------------ |
| `--source`     | Source registry to map from (e.g., `docker.io`)              |                          | `--source quay.io`                               |
| `--target`     | Target registry to map to (e.g., `registry.example.com/docker`) |                          | `--target registry.example.com/quay`             |
| `--file`       | Path to the registry mappings file                           | `registry-mappings.yaml` | `--file ./my-mappings.yaml`                      |
| `--list`       | List all configured mappings                                 | false                    | `--list`                                         |
| `--remove`     | Remove the specified source mapping                          | false                    | `--remove`                                       |
| `-h`, `--help` | Show help for config                                         |                          | `--help`                                         |

#### Examples for config

```bash
# Add or update a mapping for quay.io
irr config --source quay.io --target registry.example.com/quay

# List all mappings in the default registry-mappings.yaml file
irr config --list

# Remove the mapping for quay.io
irr config --source quay.io --remove

# Add a mapping to a custom file
irr config --file ./custom-map.yaml --source docker.io --target registry.example.com/docker

# Typical workflow: Generate skeleton, then add mappings
irr inspect --chart-path ./my-chart --generate-config-skeleton # Creates registry-mappings.yaml
irr config --source docker.io --target registry.example.com/docker # Adds/updates docker.io mapping
irr config --source quay.io --target registry.example.com/quay   # Adds/updates quay.io mapping
irr override --chart-path ./my-chart --registry-file registry-mappings.yaml # Use the config
```

### inspect

Inspects a Helm chart for image references with enhanced analysis and configuration generation capabilities.

```bash
irr inspect --chart-path CHART_PATH [--source-registries REGISTRIES]
```

#### Flags for inspect

| Flag                         | Description                                                     | Default                  | Example                                     |
| ---------------------------- | --------------------------------------------------------------- | ------------------------ | ------------------------------------------- |
| `--chart-path`               | Path to the Helm chart (required if not using release name)     |                          | `--chart-path ./my-chart`                   |
| `--release-name`             | Release name for Helm plugin mode                               |                          | `--release-name my-release`                 |
| `--namespace`                | Kubernetes namespace for the release                            | `default`                | `--namespace production`                      |
| `--generate-config-skeleton` | Generate skeleton config file (`registry-mappings.yaml` default) | false                    | `--generate-config-skeleton`                |
| `--output-format`            | Output format for analysis data (`stdout`/`--output-file`)       | `yaml`                   | `--output-format json`                      |
| `--output-file`              | Output file path for analysis or skeleton                       | `stdout`                 | `--output-file analysis.yaml`               |
| `--include-pattern`          | Glob patterns for values paths to include during analysis       |                          | `--include-pattern "*.image"`               |
| `--exclude-pattern`          | Glob patterns for values paths to exclude during analysis       |                          | `--exclude-pattern "*.test.*"`              |
| `--known-image-paths`        | Specific dot-notation paths known to contain images             |                          | `--known-image-paths "containers[].image"` |
| `-r`, `--source-registries`  | Source registries to filter results (optional)                  |                          | `--source-registries docker.io,quay.io`     |
| `-h`, `--help`               | Show help for inspect                                           |                          | `--help`                                    |

### Basic Inspection

```bash
irr inspect --chart-path ./nginx
```

### Inspection with Registry Filtering

```bash
irr inspect --chart-path ./nginx --source-registries docker.io,quay.io
```

### Generate Config Skeleton

Generates `registry-mappings.yaml` with detected registries.

```bash
irr inspect --chart-path ./my-chart --generate-config-skeleton
# (Edit registry-mappings.yaml targets)
# (Use 'irr config' to refine if needed)
```

### Advanced Inspection with Pattern Filters

```bash
irr inspect \
  --chart-path ./my-chart \
  --include-pattern "*.image" \
  --exclude-pattern "*.test.*" \
  --known-image-paths "containers[].image" \
  --output-file filtered-analysis.yaml
```

### Complex Example with All Options

```bash
irr inspect \
  --chart-path ./prometheus \
  --include-pattern "*.image" \
  --exclude-pattern "*.test.*" \
  --known-image-paths "containers[].image" \
  --generate-config-skeleton \
  --output-format yaml \
  --output-file analysis.yaml # Note: --generate-config-skeleton overrides analysis output to file
```

### override

Generates override values for redirecting images to the target registry.

```bash
irr override --chart-path CHART_PATH [flags]
```

#### Flags for override

| Flag                     | Description                                              | Default                  | Example                                          |
| ------------------------ | -------------------------------------------------------- | ------------------------ | ------------------------------------------------ |
| `-c`, `--chart-path`     | Path to the Helm chart (required if not using release name) |                          | `--chart-path ./my-chart`                        |
| `-r`, `--release-name`   | Helm release name to get values from                     |                          | `--release-name my-release`                      |
| `--namespace`            | Kubernetes namespace for the Helm release                | `default`                | `--namespace my-namespace`                       |
| `--registry-file`        | YAML file with registry mappings                         | `registry-mappings.yaml` | `--registry-file my-mappings.yaml`               |
| `-t`, `--target-registry`| Target registry URL (fallback if not in registry-file)   |                          | `--target-registry registry.example.com`         |
| `-s`, `--source-registries`| Comma-separated source registries to rewrite             | (reads from registry-file) | `--source-registries docker.io,quay.io`        |
| `--config`               | DEPRECATED: Use `--registry-file` instead                 |                          |                                                  |
| `--dry-run`              | Preview without writing (`stdout`)                       | false                    | `--dry-run`                                      |
| `-o`, `--output-file`    | Output file path for overrides                           | `stdout`                 | `--output-file overrides.yaml`                   |
| `--exclude-registries`   | Registries to exclude                                    |                          | `--exclude-registries gcr.io`                    |
| `--include-pattern`      | Glob patterns to include                                 |                          | `--include-pattern "*.image"`                    |
| `--exclude-pattern`      | Glob patterns to exclude                                 |                          | `--exclude-pattern "*.test.*"`                   |
| `--known-image-paths`    | Specific paths with images                               |                          | `--known-image-paths "containers[].image"`      |
| `--strict`               | Fail on any parsing error                                | false                    | `--strict`                                       |
| `--threshold`            | Success percentage required                              | 0                        | `--threshold 90`                                 |
| `--validate`             | Run helm template to validate                            | false                    | `--validate`                                     |
| `-h`, `--help`           | Show help for override                                   |                          | `--help`                                         |

### Basic Override Generation

Uses `registry-mappings.yaml` by default.

```bash
irr override \
  --chart-path ./nginx \
  --output-file nginx-overrides.yaml
```

### Override with Specific File and Fallback Target

```bash
irr override \
  --chart-path ./prometheus \
  --registry-file my-mappings.yaml \
  --target-registry fallback.registry.com \ # Used for sources not in my-mappings.yaml
  --source-registries docker.io,quay.io,gcr.io \ # Define which sources to consider
  --threshold 90 \
  --output-file overrides.yaml
```

### validate

Validates a Helm chart with the generated overrides by running `helm template`.

```bash
irr validate --chart-path CHART_PATH --values VALUES_FILE
```

#### Flags for validate

| Flag                 | Description                                            | Default     | Example                        |
| -------------------- | ------------------------------------------------------ | ----------- | ------------------------------ |
| `--chart-path`       | Path to the Helm chart (required if not using release name) |             | `--chart-path ./my-chart`      |
| `--release-name`     | Release name for validation                            | `release`   | `--release-name my-release`    |
| `--namespace`        | Namespace for validation                               | `default`   | `--namespace my-namespace`     |
| `--values`           | Values files to use (can specify multiple)             |             | `--values overrides.yaml`      |
| `--set`              | Set values on the command line (can specify multiple)  |             | `--set image.repository=nginx` |
| `--output-file`      | Output file for template result                        |             | `--output-file template.yaml`  |
| `--debug-template`   | Show full template output on `stderr`                  | false       | `--debug-template`             |
| `-h`, `--help`       | Show help for validate                                 |             | `--help`                       |

### Validation Example

```bash
irr validate \
  --chart-path ./my-chart \
  --values overrides.yaml
```

### Using Release Name for Validation

```bash
irr validate \
  --release-name my-release \
  --chart-path ./my-chart \ # Chart path might still be needed depending on implementation
  --values overrides.yaml
```

## Configuration File (`registry-mappings.yaml`)

The primary way to configure `irr` is through a YAML configuration file, which defaults to `registry-mappings.yaml` in the current directory. The `irr config` command is used to manage this file. Alternatively, you can specify a custom path using the `--registry-file` flag with the `override` command (or the `--file` flag with `config`).

The configuration file uses a structured YAML format:

```yaml
# registry-mappings.yaml
version: "1.0" # Optional but recommended
registries:
  mappings:
    - source: "quay.io"
      target: "registry.example.com/quay"
      enabled: true # Default is true if omitted
      description: "Optional description for quay.io"
    - source: "docker.io"
      target: "registry.example.com/docker"
      # enabled: true (implied)
    # Add more mappings as needed

  # Optional: Fallback target registry for 'override' command
  defaultTarget: "your-fallback-registry.com/generic-prefix"

  # Optional: Strict mode setting for 'override' command
  strictMode: false # Default is false

# Optional: Compatibility settings
compatibility:
  ignoreEmptyFields: true # Default is typically true or handled gracefully
```

### Key Configuration Fields

*   **`registries.mappings`**: A list defining specific source-to-target redirections.
    *   `source`: The original registry domain (e.g., `docker.io`, `quay.io`).
    *   `target`: The full target registry and path prefix where images from the `source` should be redirected (e.g., `my-harbor.local/dockerhub`).
    *   `enabled` (Optional): Set to `false` to explicitly disable this specific mapping. Defaults to `true`. Can be managed via `irr config`.
    *   `description` (Optional): A comment describing the mapping. Can be managed via `irr config`.

*   **`registries.defaultTarget`** (Optional, Used by `override`):
    *   Provides a **fallback target registry URL** used when `strictMode` is `false`.
    *   If `irr override` processes an image whose registry is listed in `--source-registries` but **lacks** a specific, enabled entry in the `mappings` list, it uses `defaultTarget` (if defined) to construct the new image path (using the selected path strategy).
    *   If `defaultTarget` is also missing, the fallback is the target specified by the `--target-registry` CLI flag for the `override` command.

*   **`registries.strictMode`** (Optional, Used by `override`, Default: `false`):
    *   When set to `true`, `strictMode` enforces that **every** source registry specified via the `override` command's `--source-registries` flag **must** have a corresponding, enabled entry in the `mappings` list.
    *   If an image's source registry is in `--source-registries` but missing from the config mappings, `irr override` will **fail with an error** instead of using `defaultTarget` or the `--target-registry` flag.
    *   Use `strictMode: true` to ensure all intended redirections are explicitly configured and prevent accidental fallback behavior.

*   **`version`** (Optional): Specifies the configuration file format version.
*   **`compatibility`** (Optional): Contains flags for handling potential backward compatibility issues (rarely needed).

### Understanding Configuration Precedence (Override Command)

When using the `irr override` command, it's important to understand how the different configuration options interact, especially regarding the target registry:

1.  **Highest Priority: Explicit Mapping:** If an image's source registry (e.g., `docker.io`) is listed in `--source-registries` and has a specific, enabled entry in the `registries.mappings` list within the configuration file, the `target` defined in that mapping entry is **always** used.

2.  **Fallback 1: `defaultTarget` (if `strictMode: false`)**: If an image's source registry is listed in `--source-registries` but **lacks** a specific mapping in the configuration file, *and* `registries.strictMode` is `false` (the default), `irr` checks if `registries.defaultTarget` is defined in the config file. If it is, this `defaultTarget` URL is used (combined with the selected path strategy, e.g., `prefix-source-registry`).

3.  **Fallback 2: `--target-registry` CLI Flag (if `strictMode: false`)**: If there's no specific mapping *and* no `registries.defaultTarget` in the config file, *and* `registries.strictMode` is `false`, `irr` falls back to using the URL provided by the `--target-registry` CLI flag (combined with the selected path strategy). This CLI flag is therefore the ultimate fallback when not using strict mode.

4.  **Strict Mode Enforcement:** If `registries.strictMode` is set to `true` in the configuration file, then **every** registry listed in the `--source-registries` CLI flag **must** have a corresponding, enabled entry in `registries.mappings`. If a mapping is missing, the command will **fail** instead of using any fallback (`defaultTarget` or `--target-registry`). Use `strictMode: true` to prevent accidental reliance on fallbacks and ensure all redirections are explicitly defined.

In summary, you only *need* the `--target-registry` CLI flag if `strictMode` is `false` AND you might have source registries listed in `--source-registries` that are not explicitly mapped in your configuration file AND you haven't defined a `registries.defaultTarget`.

## Exit Codes

| Code | Meaning                   |
| ---- | ------------------------- |
| 0    | Success                   |
| 1    | Missing required flag     |
| 2    | Input/Configuration error |
| 3    | Input/Configuration error |
| 4    | Chart not found           |
| 10   | Chart parsing error       |
| 11   | Image processing error    |
| 12   | Unsupported structure     |
| 13   | Threshold not met         |
| 14   | Chart load failed         |
| 15   | Chart processing failed   |
| 16   | Helm command failed       |
| 20   | General runtime error     |
| 21   | I/O error                 |