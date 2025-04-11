# CLI Reference

## Command Overview

The `irr` (Image Relocation and Rewrite) tool provides commands and options for analyzing and modifying image references in Helm charts.

## Global Flags

These flags are available for all commands:

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `--config` | Config file path | `$HOME/.irr.yaml` | `--config my-config.yaml` |
| `--debug` | Enable debug logging | false | `--debug` |
| `--log-level` | Set log level | info | `--log-level debug` |
| `--help` | Show help | | `--help` |

## Commands

### analyze

Analyzes a Helm chart for image references without generating overrides.

```bash
irr analyze [flags] CHART
```

#### Flags for analyze

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `-h, --help` | Show help for analyze | | `--help` |
| `-m, --mappings` | Registry mappings file | | `--mappings mappings.yaml` |
| `-o, --output` | Output format | text | `--output json` |
| `-f, --output-file` | Output file | stdout | `--output-file analysis.txt` |
| `-r, --source-registries` | Source registries to analyze (required) | | `--source-registries docker.io,quay.io` |
| `-s, --strict` | Enable strict mode | false | `--strict` |
| `-v, --verbose` | Enable verbose output | false | `--verbose` |

### override

Generates override values for redirecting images to the target registry.

```bash
irr override [flags]
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
| `-s, --source-registries` | Source registries (required) | | `--source-registries docker.io,quay.io` |
| `-p, --strategy` | Path generation strategy | prefix-source-registry | `--strategy prefix-source-registry` |
| `--strict` | Fail on any parsing error | false | `--strict` |
| `-t, --target-registry` | Target registry URL (required) | | `--target-registry harbor.example.com` |
| `--threshold` | Success percentage required | 0 | `--threshold 90` |
| `--validate` | Run helm template to validate | false | `--validate` |

## Flag Details

### --chart-path (-c)
- Accepts both directory paths and .tgz archives
- Must contain a valid Chart.yaml
- Supports subcharts in charts/ directory
- Example: `--chart-path ./charts/nginx`

### --target-registry (-t)
- Registry URL where images will be redirected
- Can include port number
- No trailing slash
- Examples:
  - `--target-registry harbor.example.com`
  - `--target-registry localhost:5000`

### --source-registries (-s)
- Comma-separated list of registries to process
- Registry names are normalized (e.g., "docker.io" = "index.docker.io")
- Examples:
  - `--source-registries docker.io`
  - `--source-registries docker.io,quay.io,gcr.io`

### --strategy (-p)
- Controls how image paths are constructed in target registry
- Available strategies:
  - `prefix-source-registry` (default): Preserves source registry as path prefix
  - `flat` (planned): Removes source registry from path

### --verbose (-v)
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

### --strict (-s)
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

### --validate
- Runs `helm template` with generated overrides
- Confirms chart remains renderable
- Useful for validating changes before applying

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

## Examples

### Basic Usage
```bash
irr override \
  --chart-path ./nginx \
  --target-registry harbor.example.com \
  --source-registries docker.io
```

### Complex Example
```bash
irr override \
  --chart-path ./prometheus \
  --target-registry harbor.example.com \
  --source-registries docker.io,quay.io \
  --exclude-registries k8s.gcr.io \
  --strategy prefix-source-registry \
  --threshold 90 \
  --output-file overrides.yaml
```

### Analysis Only
```bash
irr analyze \
  --output json \
  --source-registries docker.io \
  ./my-chart
```

### Using Registry Mappings
```bash
irr override \
  --chart-path ./my-chart \
  --target-registry harbor.example.com \
  --source-registries docker.io \
  --registry-file ./registry-mappings.yaml
```