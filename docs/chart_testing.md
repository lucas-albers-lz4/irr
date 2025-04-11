# Chart Testing Documentation

## Overview

The `test/tools/test-charts.py` script is a comprehensive testing tool designed to validate the `helm-image-override` functionality across a wide range of Helm charts. It automates the process of downloading, analyzing, and testing charts with our image override tool, providing detailed feedback and statistics.

## Dependencies

The script requires the following Python modules:
- `argparse`: Command-line argument parsing
- `concurrent.futures`: Parallel processing
- `json`: JSON data handling
- `tarfile`: Chart archive handling
- `yaml`: YAML processing
- `pathlib`: Path manipulation

System requirements:
- Python 3.6 or later
- Helm 3.x
- Sufficient disk space for chart caching (~50MB per chart)

### Installing Dependencies

After cloning the repository, install the required dependencies using one of these methods:

```bash
# If using pip:
pip install -e .

# If using uv:
uv sync
```

## Features

- **Chart Discovery & Download**: Automatically fetches charts from configured repositories
- **Parallel Processing**: Efficient multi-chart testing with configurable parallelism
- **Smart Caching**: Persistent chart caching to reduce downloads and rate limits
- **Flexible Filtering**: Options to target specific charts or repositories
- **Detailed Analysis**: Comprehensive error categorization and reporting
- **Rate Limit Handling**: Built-in protections against API rate limits

## Usage

### Basic Usage

```bash
# Test with default settings
./test/tools/test-charts.py harbor.home.arpa

# Test with specific options
./test/tools/test-charts.py harbor.home.arpa \
    --chart-filter "bitnami/*" \
    --max-charts 10 \
    --no-parallel
```

### Command-Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `target_registry` | Target registry URL (required) | None |
| `--no-parallel` | Disable parallel processing | False |
| `--chart-filter` | Only process charts matching pattern | None |
| `--max-charts` | Maximum number of charts to process | None |
| `--skip-charts` | Comma-separated list of charts to skip | None |
| `--no-cache` | Disable chart caching | False |

## Caching System

### Cache Location
Charts are cached in `test/chart-cache/` to minimize downloads and reduce rate limit issues.

### Cache Behavior
- **First Run**: Downloads charts and stores in cache
- **Subsequent Runs**: Uses cached charts if available
- **Cache Invalidation**: Currently manual (delete cache directory)
- **Cache Control**: Use `--no-cache` to bypass caching

### Cache Structure
```
test/chart-cache/
├── chart1-1.0.0.tgz
├── chart2-2.3.1.tgz
└── ...
```

## Rate Limit Protection

The script implements several strategies to avoid hitting rate limits:

1. **Chart Caching**
   - Persistent storage of downloaded charts
   - Reuse of cached charts across runs

2. **Request Rate Control**
   - Conservative parallel processing limits
   - QPS and burst limits on Helm commands
   - Incremental backoff for retries

3. **Repository Operation Spacing**
   - Delays between repository updates
   - Sequential repository operations

### Configuration
```python
# Default rate limit settings
import time
import os

time.sleep(1)  # Add small delay between charts
MAX_WORKERS = min(4, os.cpu_count() or 2)  # Lower the parallel processing limit
QPS_LIMIT = 2
BURST_LIMIT = 3
BASE_RETRY_DELAY = 10  # seconds
```

## Error Categories

The script categorizes errors to help identify and debug issues:

| Category | Description | Example |
|----------|-------------|---------|
| `RATE_LIMIT` | Rate limit exceeded | "Docker Hub rate limit exceeded" |
| `BITNAMI` | Bitnami-specific issues | "allowInsecureImages required" |
| `COMMAND_ERROR` | Invalid command syntax | "unknown flag: --chart" |
| `UNKNOWN` | Uncategorized errors | Various other errors |

## Performance Tuning

### Parallel Processing
- Default: Uses 4-8 workers (based on CPU count)
- Disable: Use `--no-parallel` for sequential processing
- Memory Usage: ~100MB per worker process

### Caching Impact
- First Run: Higher network usage, longer runtime
- Cached Runs: Significantly faster, minimal network usage
- Cache Size: ~50MB per chart (average)

## Troubleshooting

### Common Issues

1. **Rate Limit Errors**
   ```
   Error: Docker Hub rate limit exceeded
   ```
   - Solution: Wait for rate limit reset or use `--no-parallel`
   - Prevention: Ensure caching is enabled

2. **Command Syntax Errors**
   ```
   Error: unknown flag: --chart
   ```
   - Solution: Verify command syntax in test script
   - Check: Recent changes to helm-image-override CLI

3. **Cache Issues**
   ```
   Warning: Failed to use cached chart
   ```
   - Solution: Clear cache directory and retry
   - Check: Disk space and permissions

### Debugging Tips

1. **Enable Verbose Output**
   ```bash
   export HELM_DEBUG=1
   ./test/tools/test-charts.py ...
   ```

2. **Check Cache State**
   ```bash
   ls -l test/chart-cache/
   ```

3. **Review Test Results**
   ```bash
   cat test/results.txt
   ```

## Results Analysis

### Output Files
- `test/results.txt`: Summary of all test runs
- `test/charts/`: Individual chart test outputs
- `test/overrides/`: Generated override files

### Success Criteria
- Chart download successful
- Override generation completed
- Helm template validation passed
- No rate limit errors encountered

### Example Results
```
Total Charts: 65
Successful: 64 (98.5%)
Failed: 1 (1.5%)
  - Rate Limits: 0
  - Command Errors: 0
  - Bitnami Issues: 0
  - Unknown: 1
```

## Future Improvements

1. **Cache Management**
   - Automatic cache cleanup
   - Cache versioning
   - Cache statistics

2. **Repository Optimization**
   - Repository-specific rate limits
   - Smart retry logic
   - Authentication support

3. **Results Enhancement**
   - HTML report generation
   - Detailed timing analysis
   - Error pattern analysis

## The `default-values.yaml` File

### Understanding Why We Use This File

This section explains why our `test-charts.py` script utilizes a custom `default-values.yaml` file when testing Helm charts, even though charts typically include their own default values.

#### Helm Chart Defaults: The Standard `values.yaml`

* **Built-in Defaults:** Nearly every Helm chart comes packaged with a `values.yaml` file. This file contains the chart maintainer's default settings for configuration options like image tags, replica counts, service types, resource limits, etc.
* **Standard Usage:** When you execute `helm install my-chart` or `helm template my-chart` without specifying any overrides (`-f` or `--set`), Helm relies entirely on the defaults defined within that chart's internal `values.yaml`.

#### Why We Override with `-f default-values.yaml`

The primary purpose of using command-line flags like `-f <your-values-file>` or `--set key=value` is to **override** the chart's built-in defaults. This allows users to customize a chart for their specific environment or requirements.

Our `test-charts.py` script uses `-f default-values.yaml` for several specific, crucial reasons related to our testing goals:

1. **Enforcing the Image Mirror (`harbor.home.arpa/docker`)**
   * **Critical Goal:** We need *all* charts tested by the script to attempt pulling images from our local Harbor mirror (`harbor.home.arpa/docker`) instead of public registries (like Docker Hub).
   * **Benefit:** This avoids public registry rate limits and ensures tests run against potentially cached or internally approved images.
   * **Challenge:** Different charts specify their image registry using various keys (e.g., `global.imageRegistry`, `image.registry`, Bitnami's `registry.server` or specific `image.registry` sections).
   * **Solution:** Our `default-values.yaml` attempts to set these common registry keys to `harbor.home.arpa/docker`. Passing this file via `-f` forces the Helm template rendering process to *try* using our mirror, irrespective of the chart's original default registry.

2. **Handling Bitnami Image Verification (`allowInsecureImages`)**
   * **Context:** Bitnami charts include a security check (`allowInsecureImages`). This check typically fails when we force the chart to use images from our mirror (e.g., `harbor.home.arpa/docker/bitnami/...`) instead of their official `docker.io/bitnami/...` images.
   * **Requirement:** The default for this check is `false`. To proceed with our mirrored images, we *must* explicitly override this setting.
   * **Solution:** Our `default-values.yaml` sets `global.security.allowInsecureImages: true`. We cannot rely on the chart's default value here; an explicit override is necessary for the tests involving Bitnami charts using our mirror.

3. **Ensuring Successful Templating (`helm template`)**
   * **Problem:** Some charts might fail the `helm template` command entirely if certain basic required values (like a mandatory password or specific storage configurations) are missing.
   * **Goal:** The immediate goal for `test-charts.py` is just to successfully *render* the chart's templates so we can analyze and override the images within them. The resulting manifests don't need to be fully deployable at this stage.
   * **Solution:** Our `default-values.yaml` provides minimal, generally safe defaults (e.g., `storageClass: ""`) to increase the likelihood that `helm template` completes successfully, even if further customization would be needed for actual deployment.

4. **Consistency**
   * **Benefit:** Using a single `default-values.yaml` provides a consistent baseline configuration applied to *all* charts processed by the test script.

#### Default Values Content

The content of our `default-values.yaml` file typically includes:

```yaml
global:
  imageRegistry: ""
  imagePullSecrets: []
  storageClass: ""
  security:
    allowInsecureImages: true  # Required for Bitnami charts
```

## Contributing

### Adding Test Cases
1. Add new repository to `repos` list
2. Add specific chart patterns to test
3. Update error categorization if needed

### Development Guidelines
1. Maintain parallel processing safety
2. Handle rate limits gracefully
3. Keep caching system efficient
4. Add comprehensive error handling

## Related Documentation

- [TESTING.md](../TESTING.md): Overall testing strategy
- [DEVELOPMENT.md](../DEVELOPMENT.md): Development guidelines
- [CLI Reference](cli-reference.md): Command-line interface details 