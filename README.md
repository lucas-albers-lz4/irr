# IRR (Image Registry Rewrite)

A command-line tool to automate the generation of Helm chart override files for redirecting container images to private or local registries.

## Overview

The `irr` tool analyzes Helm charts and automatically generates `values.yaml` override files to redirect container image pulls from specified public registries to a target registry. This is especially useful when:

- Using a private registry like Harbor with pull-through cache
- Working in air-gapped environments
- Enforcing image provenance requirements
- Migrating to a new container registry

## Installation

### Binary Installation (not available yet)

Download the latest release for your platform from the [releases page](https://github.com/lalbers/irr/releases).

```bash
# Linux
curl -LO https://github.com/lalbers/irr/releases/latest/download/irr-linux-amd64
chmod +x irr-linux-amd64
mv irr-linux-amd64 /usr/local/bin/irr

# macOS
curl -LO https://github.com/lalbers/irr/releases/latest/download/irr-darwin-amd64
chmod +x irr-darwin-amd64
mv irr-darwin-amd64 /usr/local/bin/irr
```

### Building from Source

1. Clone the repository:
```bash
git clone https://github.com/lalbers/irr.git
cd irr
```

2. Build the binary:
```bash
make build
```

The binary will be created at `bin/irr`. You can optionally add it to your PATH:

```bash
# Optional: Install to /usr/local/bin
sudo cp bin/irr /usr/local/bin/
```

## Usage

### Basic Usage

```bash
irr override \
  --chart-path ./my-chart \
  --target-registry harbor.example.com:5000 \
  --source-registries docker.io,quay.io \
  --output-file overrides.yaml
```

This generates an `overrides.yaml` file that you can use with Helm:

```bash
helm install my-release ./my-chart -f overrides.yaml
```

### Supported Options

```
  --chart-path string            Path to the Helm chart (directory or .tgz archive)
  --target-registry string       Target registry URL (e.g., harbor.example.com:5000)
  --source-registries string     Comma-separated list of source registries to rewrite (e.g., docker.io,quay.io)
  --output-file string           Output file path for overrides (default: stdout)
  --path-strategy string         Path strategy to use (default "prefix-source-registry")
  --verbose                      Enable verbose output
  --dry-run                      Preview changes without writing file
  --strict                       Fail on unrecognized image structures
  --exclude-registries string    Comma-separated list of registries to exclude from processing
  --threshold int                Success threshold percentage (0-100) (default 100)
  --registry-file string         Path to a YAML file containing registry mappings (source: target)
  --config string                Path to a YAML configuration file for precise image mappings
```

### Example: Redirecting Images to Harbor

Harbor is a commonly used private registry with pull-through cache capabilities. To use `irr` with Harbor:

1. First, set up pull-through caching in Harbor:
   - Create projects in Harbor for each source registry (e.g., `dockerio`, `quayio`)
   - Configure them as proxy caches pointing to the appropriate source registries

2. Generate overrides for a chart:
   ```bash
   irr \
     --chart-path ./prometheus \
     --target-registry harbor.example.com \
     --source-registries docker.io,quay.io \
     --output-file prometheus-overrides.yaml
   ```

3. Install the chart with the overrides:
   ```bash
   helm install prometheus ./prometheus -f prometheus-overrides.yaml
   ```

## Path Strategies

The tool supports different strategies for structuring the repository paths in the target registry.

### prefix-source-registry (Default)

This strategy places images under a subdirectory named after the source registry:

| Original Image | Transformed Image |
|----------------|-------------------|
| docker.io/nginx:1.23 | harbor.example.com/dockerio/nginx:1.23 |
| quay.io/prometheus/node-exporter:v1.3.1 | harbor.example.com/quayio/prometheus/node-exporter:v1.3.1 |

This strategy maintains the hierarchical structure with slashes and helps avoid naming conflicts while preserving registry origin information.

### flat

This strategy flattens the repository path by replacing slashes with dashes:

| Original Image | Transformed Image |
|----------------|-------------------|
| docker.io/library/nginx:1.21 | harbor.example.com/dockerio-library-nginx:1.21 |
| quay.io/prometheus/node-exporter:v1.3.1 | harbor.example.com/quayio-prometheus-node-exporter:v1.3.1 |

This is useful for registries or environments that prefer flat namespaces without slashes.

To enable the flat strategy, use the `--strategy` flag:

```bash
irr override --chart my-chart --strategy flat --target-registry target-registry.com
```

## Advanced Registry Mapping

For cases where you need precise control over specific image mappings, irr provides a `--config` flag that accepts a YAML file with exact image-to-target mappings.

### Using the Config File

The config file uses a simple map format where:
- Keys are source image references (e.g., `docker.io/library/nginx`)
- Values are target image locations including registry and path (e.g., `my-registry.io/custom/nginx-mirror`)

Example config file (`mappings.yaml`):
```yaml
docker.io/library/nginx: my-registry.io/custom/nginx-mirror
quay.io/prometheus/prometheus: my-registry.io/monitoring/prometheus
k8s.gcr.io/etcd: my-registry.io/kubernetes/etcd
```

To use this config file:

```bash
irr override \
  --chart-path ./my-chart \
  --target-registry fallback.registry.com \
  --source-registries docker.io,quay.io,k8s.gcr.io \
  --config mappings.yaml \
  --output-file overrides.yaml
```

### How Config Mappings Work

1. The `--config` mappings have higher priority than the path strategy
2. Any image that matches a key in the config file will use the exact target specified
3. Images not matched in the config will fall back to using `--target-registry` with the selected path strategy
4. The `--target-registry` flag is still required as a fallback for unmapped images

This feature is particularly useful for:
- Handling special cases where the standard path strategy doesn't produce the desired result
- Working with registries that have specific naming requirements for certain images
- Setting up custom paths for pull-through cache configurations

## Supported Image Reference Formats

The tool detects the following image reference patterns in Helm chart values:

1. Maps with `repository` and `tag` fields:
   ```yaml
   image:
     repository: nginx
     tag: 1.23
   ```

2. Maps with `registry`, `repository`, and `tag` fields:
   ```yaml
   image:
     registry: docker.io
     repository: nginx
     tag: 1.23
   ```

3. String values named `image`:
   ```yaml
   image: nginx:1.23
   ```

## Limitations

- Images defined outside `values.yaml` (e.g., hardcoded in templates) are not detected
- Complex templated image references may not be detected
- Currently only processes `.tgz` files and directories, not Helm OCI artifacts

## Development

### Running Tests

The project includes both unit tests and integration tests. Here's how to run them:

```bash
# Run all tests
make test

# Run only unit tests
make test-unit

# Run only integration tests
make test-integration

# Run integration tests with debug output
make test-integration-debug

# Run a specific integration test by name
make test-integration-specific TEST_NAME=TestConfigFileMappings

# Run specific integration test (e.g., cert-manager)
make test-cert-manager
```

For detailed testing instructions, test architecture, and test coverage information, see [TESTING.md](TESTING.md).

### Direct Go Test Commands

If you prefer to run tests directly with Go's test command:

```bash
# Run all integration tests
go test -v ./test/integration/...

# Run a specific integration test
go test -v ./test/integration/... -run TestConfigFileMappings

# Run with debug logging
IRR_TESTING=true LOG_LEVEL=DEBUG go test -v ./test/integration/... -run TestConfigFileMappings
```

### Test Coverage

To generate test coverage reports:

```bash
# Generate coverage for all tests
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# View coverage in terminal
go tool cover -func=coverage.out
```

# Testing a Single Helm Chart

## Prerequisites
- Helm CLI installed
- irr binary built

## Steps

1. **Pull the chart locally**
   ```bash
   helm repo add bitnami https://charts.bitnami.com/bitnami
   helm pull bitnami/nginx --untar --destination ./tmp
   ```

2. **Run the override tool**
   ```bash
   ./bin/irr \
     --chart-path ./tmp/nginx \
     --target-registry my-registry.example.com \
     --source-registries docker.io,quay.io \
     --output-file ./tmp/overrides.yaml \
     --verbose
   ```

3. **Validate with Helm template**
   ```bash
   helm template test ./tmp/nginx -f ./tmp/overrides.yaml > ./tmp/rendered.yaml
   ```

4. **Analyze the output**
   ```bash
   # Extract all image references
   grep -o 'image:.*' ./tmp/rendered.yaml
   
   # Check for specific registry
   grep -o 'my-registry.example.com/.*' ./tmp/rendered.yaml
   ```

5. **Cleanup**
   ```bash
   rm -rf ./tmp/nginx ./tmp/overrides.yaml ./tmp/rendered.yaml
   ```
   
## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## Registry Mappings

You can specify custom registry mappings using a YAML configuration file. This allows you to control how source registries are mapped to your target registry's directory structure.

Create a `registry-mappings.yaml` file:

```yaml
mappings:
  - source: "quay.io"
    target: "my-registry.example.com/quay-mirror"
  - source: "docker.io"
    target: "my-registry.example.com/docker-mirror"
  - source: "gcr.io"
    target: "my-registry.example.com/gcr-mirror"
```

Use the mappings file with the tool:

```bash
irr \
  --chart-path ./my-chart \
  --target-registry my-registry.example.com \
  --source-registries docker.io,quay.io \
  --registry-file ./registry-mappings.yaml \
  --output-file overrides.yaml
```

When no mappings file is provided, the tool will use the default behavior of prefixing the sanitized source registry name:
- `docker.io/nginx:1.23` -> `my-registry.example.com/dockerio/nginx:1.23`

## Test Output Management

To run tests with reduced output volume:

### 1. Limiting output to just failures

```bash
go test ./... -v 2>&1 | grep -A 10 "\-\-\-\sFAIL"
```

### 2. Using quiet mode for passing tests

```bash
go test -quiet ./...
```

### 3. Running specific packages

```bash
# Test only a specific package
go test ./pkg/chart/...

# Test multiple packages
go test ./pkg/chart/... ./pkg/image/...
```

### 4. Using test.output flag to capture verbose logs to a file

```bash
go test -v ./... -args -test.testlogfile=test_output.log
```

### 5. Using race detection but quiet output 

```bash
go test -race -quiet ./...
```