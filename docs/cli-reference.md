# CLI Reference

## Command Overview

The `helm-image-override` tool provides several commands and options for analyzing and modifying image references in Helm charts.

## Global Flags

These flags are available for all commands:

### Required Flags

| Flag | Description | Example |
|------|-------------|---------|
| `--chart-path` | Path to the Helm chart (directory or .tgz) | `--chart-path ./my-chart` |
| `--target-registry` | Target registry URL | `--target-registry harbor.example.com` |
| `--source-registries` | Comma-separated list of source registries | `--source-registries docker.io,quay.io` |

### Optional Flags

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `--output-file` | Output file for overrides | stdout | `--output-file overrides.yaml` |
| `--path-strategy` | Path strategy for image references | prefix-source-registry | `--path-strategy flat` |
| `--verbose` | Enable detailed logging | false | `--verbose` |
| `--dry-run` | Preview changes without writing | false | `--dry-run` |
| `--strict` | Fail on unrecognized structures | false | `--strict` |
| `--exclude-registries` | Registries to exclude | none | `--exclude-registries gcr.io` |
| `--threshold` | Success threshold percentage | 100 | `--threshold 90` |

## Commands

### analyze

Analyzes a Helm chart for image references without generating overrides.

```bash
helm-image-override analyze --chart-path ./my-chart
```

#### Additional Flags for analyze

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `--json` | Output in JSON format | false | `--json` |
| `--summary` | Show only summary | false | `--summary` |

### override

Generates override values for redirecting images to the target registry.

```bash
helm-image-override override \
  --chart-path ./my-chart \
  --target-registry harbor.example.com \
  --source-registries docker.io,quay.io
```

## Flag Details

### --chart-path
- Accepts both directory paths and .tgz archives
- Must contain a valid Chart.yaml
- Supports subcharts in charts/ directory
- Example: `--chart-path ./charts/nginx`

### --target-registry
- Registry URL where images will be redirected
- Can include port number
- No trailing slash
- Examples:
  - `--target-registry harbor.example.com`
  - `--target-registry localhost:5000`

### --source-registries
- Comma-separated list of registries to process
- Registry names are normalized (e.g., "docker.io" = "index.docker.io")
- Examples:
  - `--source-registries docker.io`
  - `--source-registries docker.io,quay.io,gcr.io`

### --path-strategy
- Controls how image paths are constructed in target registry
- Available strategies:
  - `prefix-source-registry` (default): Preserves source registry as path prefix
  - `flat` (planned): Removes source registry from path

### --verbose
- Enables detailed logging
- Shows:
  - Image detection process
  - Path construction
  - Registry matching
  - Type detection
  - Template variable handling

### --dry-run
- Previews changes without writing files
- Useful for:
  - Validating override structure
  - Checking path strategy results
  - Verifying registry transformations

### --strict
- Fails if unrecognized image structures found
- Useful for:
  - CI/CD pipelines
  - Validation workflows
  - Ensuring complete processing

### --exclude-registries
- Registries to skip during processing
- Useful for:
  - Keeping certain images unchanged
  - Handling special cases
  - Example: `--exclude-registries k8s.gcr.io,registry.k8s.io`

### --threshold
- Success percentage required to generate output
- Range: 0-100
- Example: `--threshold 90` (allow 10% failure)

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General runtime error |
| 2 | Input/Configuration error |
| 3 | Chart parsing error |
| 4 | Image processing error |
| 5 | Unsupported structure (strict mode) |
| 6 | Threshold not met |

## Environment Variables

Environment variables can be used instead of flags:

| Variable | Flag Equivalent |
|----------|----------------|
| `HELM_IMAGE_OVERRIDE_TARGET_REGISTRY` | `--target-registry` |
| `HELM_IMAGE_OVERRIDE_SOURCE_REGISTRIES` | `--source-registries` |
| `HELM_IMAGE_OVERRIDE_PATH_STRATEGY` | `--path-strategy` |
| `HELM_IMAGE_OVERRIDE_VERBOSE` | `--verbose` |

## Examples

### Basic Usage
```bash
helm-image-override override \
  --chart-path ./nginx \
  --target-registry harbor.example.com \
  --source-registries docker.io
```

### Complex Example
```bash
helm-image-override override \
  --chart-path ./prometheus \
  --target-registry harbor.example.com \
  --source-registries docker.io,quay.io \
  --exclude-registries k8s.gcr.io \
  --path-strategy prefix-source-registry \
  --threshold 90 \
  --verbose \
  --output-file overrides.yaml
```

### Analysis Only
```bash
helm-image-override analyze \
  --chart-path ./my-chart \
  --json \
  --verbose
```

### Using Environment Variables
```bash
export HELM_IMAGE_OVERRIDE_TARGET_REGISTRY=harbor.example.com
export HELM_IMAGE_OVERRIDE_SOURCE_REGISTRIES=docker.io,quay.io

helm-image-override override --chart-path ./my-chart
``` 