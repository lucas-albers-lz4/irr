# CLI Reference

## Command Overview

The `irr` (Image Relocation and Rewrite) tool provides commands and options for inspecting, overriding, and validating image references in Helm charts.

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
- `LOG_FORMAT=json`: (Default) Outputs logs in structured JSON format.
- `LOG_FORMAT=text`: Outputs logs in a human-readable plain text format, useful for local debugging.

Example:
```bash
LOG_FORMAT=text irr inspect ...
```

**Output Streams:**

`irr` follows standard Unix conventions for output streams:
- **`stdout`**: Used for primary command output, such as YAML/JSON results (`inspect`, `override --dry-run`) or rendered templates (`validate` without `-o`).
- **`stderr`**: Used for diagnostic messages, including logs (INFO, WARN, ERROR, DEBUG), progress updates, and error details.

This separation allows you to easily redirect command results while still seeing logs, for example:
```bash
# Save inspect results to a file, logs still go to terminal
irr inspect --chart-path ./my-chart > analysis.yaml

# Pipe override results to another command, logs go to terminal
irr override --chart-path ./my-chart --dry-run | kubectl apply -f -
```

## Commands

### inspect

Inspects a Helm chart for image references with enhanced analysis and configuration generation capabilities.

```bash
irr inspect --chart-path CHART_PATH [--source-registries REGISTRIES]
```

#### Flags for inspect

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `--chart-path` | Path to the Helm chart (required) | | `--chart-path ./my-chart` |
| `--generate-config-skeleton` | Generate skeleton config file | | `--generate-config-skeleton config.yaml` |
| `--output-format` | Output format | yaml | `--output-format yaml` |
| `--output-file` | Output file path | stdout | `--output-file analysis.yaml` |
| `--include-pattern` | Glob patterns for values paths to include during analysis | | `--include-pattern "*.image"` |
| `--exclude-pattern` | Glob patterns for values paths to exclude during analysis | | `--exclude-pattern "*.test.*"` |
| `--known-image-paths` | Specific dot-notation paths known to contain images | | `--known-image-paths "containers[].image"` |
| `-r, --source-registries` | Source registries to filter results (optional) | | `--source-registries docker.io,quay.io` |
| `-h, --help` | Show help for inspect | | `--help` |

### Basic Inspection
```bash
irr inspect --chart-path ./nginx
```

### Inspection with Registry Filtering
```bash
irr inspect --chart-path ./nginx --source-registries docker.io,quay.io
```

### Generate Config Skeleton
```bash
irr inspect \
  --chart-path ./my-chart \
  --generate-config-skeleton my-config.yaml
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
  --generate-config-skeleton config.yaml \
  --output-format yaml \
  --output-file analysis.yaml
```

### override

Generates override values for redirecting images to the target registry.

```bash
irr override --chart-path CHART_PATH --source-registries REGISTRIES --target-registry TARGET_REGISTRY [flags]
```

#### Flags for override

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `-c, --chart-path` | Path to the Helm chart (required) | | `--chart-path ./my-chart` |
| `--config` | Path to YAML config for registry mappings | | `--config mappings.yaml` |
| `--dry-run` | Preview without writing | false | `--dry-run` |
| `--exclude-pattern` | Glob patterns to exclude | | `--exclude-pattern "*.test.*"` |
| `--exclude-registries` | Registries to exclude | | `--exclude-registries gcr.io` |
| `-h, --help` | Show help for override | | `--help` |
| `--include-pattern` | Glob patterns to include | | `--include-pattern "*.image"` |
| `--known-image-paths` | Specific paths with images | | `--known-image-paths "containers[].image"` |
| `-o, --output-file` | Output file path | stdout | `--output-file overrides.yaml` |
| `--registry-file` | YAML file with registry mappings | | `--registry-file mappings.yaml` |
| `-r, --release-name` | Helm release name to get values from | | `--release-name my-release` |
| `-s, --source-registries` | Source registries (required) | | `--source-registries docker.io,quay.io` |
| `-p, --strategy` | Path generation strategy | prefix-source-registry | `--strategy prefix-source-registry` |
| `--strict` | Fail on any parsing error | false | `--strict` |
| `-t, --target-registry` | Target registry URL (required) | | `--target-registry harbor.example.com` |
| `--threshold` | Success percentage required | 0 | `--threshold 90` |
| `--validate` | Run helm template to validate | false | `--validate` |
| `--namespace` | Kubernetes namespace for the Helm release | | `--namespace my-namespace` |

### Basic Override Generation
```bash
irr override \
  --chart-path ./nginx \
  --target-registry harbor.example.com \
  --source-registries docker.io,quay.io
```

### Complex Example with Configuration
```bash
irr override \
  --chart-path ./prometheus \
  --config registry-config.yaml \
  --target-registry harbor.example.com \
  --source-registries docker.io,quay.io \
  --threshold 90 \
  --output-file overrides.yaml
```

### validate

Validates a Helm chart with the generated overrides by running `helm template`.

```bash
irr validate --chart-path CHART_PATH --values VALUES_FILE
```

#### Flags for validate

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `--chart-path` | Path to the Helm chart (required) | | `--chart-path ./my-chart` |
| `--release-name` | Release name for validation | release | `--release-name my-release` |
| `--values` | Values files to use (can specify multiple) | | `--values overrides.yaml` |
| `--set` | Set values on the command line (can specify multiple) | | `--set image.repository=nginx` |
| `--namespace` | Namespace for validation | default | `--namespace my-namespace` |
| `--output-file` | Output file for template result | | `--output-file template.yaml` |
| `--debug-template` | Show full template output | false | `--debug-template` |
| `-h, --help` | Show help for validate | | `--help` |

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
  --chart-path ./my-chart \
  --values overrides.yaml
```

## Configuration File (`registry-mappings.yaml`)

While `irr` commands can be used with command-line flags, using a configuration file (typically `registry-mappings.yaml` and passed via `--config <path>` to commands like `override`) provides more control and persistence.

The configuration file uses a structured YAML format:

```yaml
version: "1.0" # Optional but recommended
registries:
  mappings:
    - source: "quay.io"
      target: "harbor.home.arpa/quay"
      enabled: true # Default is true if omitted
      description: "Optional description for quay.io"
    - source: "docker.io"
      target: "harbor.home.arpa/docker"
      # enabled: true (implied)
    # Add more mappings as needed

  # Optional: Fallback target registry
  defaultTarget: "your-fallback-registry.com/generic-prefix"

  # Optional: Strict mode setting
  strictMode: false # Default is false

# Optional: Compatibility settings
compatibility:
  ignoreEmptyFields: true # Default is typically true or handled gracefully
```

### Key Configuration Fields

*   **`registries.mappings`**: A list defining specific source-to-target redirections.
    *   `source`: The original registry domain (e.g., `docker.io`, `quay.io`).
    *   `target`: The full target registry and path prefix where images from the `source` should be redirected (e.g., `my-harbor.local/dockerhub`).
    *   `enabled` (Optional): Set to `false` to explicitly disable this specific mapping. Defaults to `true`.
    *   `description` (Optional): A comment describing the mapping.

*   **`registries.defaultTarget`** (Optional):
    *   Provides a **fallback target registry URL** used when `strictMode` is `false`.
    *   If `irr override` processes an image whose registry is listed in `--source-registries` but **lacks** a specific entry in the `mappings` list, it uses `defaultTarget` (if defined) to construct the new image path (using the selected path strategy).
    *   If `defaultTarget` is also missing, the fallback is usually the target specified by the `--target-registry` CLI flag.

*   **`registries.strictMode`** (Optional, Default: `false`):
    *   When set to `true`, `strictMode` enforces that **every** source registry specified via the `--source-registries` flag **must** have a corresponding, enabled entry in the `mappings` list.
    *   If an image's source registry is in `--source-registries` but missing from the config mappings, `irr override` will **fail with an error** instead of using `defaultTarget` or the `--target-registry` flag.
    *   Use `strictMode: true` to ensure all intended redirections are explicitly configured and prevent accidental fallback behavior.

*   **`version`** (Optional): Specifies the configuration file format version.
*   **`compatibility`** (Optional): Contains flags for handling potential backward compatibility issues (rarely needed).

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Missing required flag |
| 2 | Input/Configuration error |
| 3 | Invalid strategy |
| 4 | Chart not found |
| 10 | Chart parsing error |
| 11 | Image processing error |
| 12 | Unsupported structure (strict mode) |
| 13 | Threshold not met |
| 14 | Chart load failed |
| 15 | Chart processing failed |
| 16 | Helm command failed |
| 20 | General runtime error |
| 21 | I/O error |