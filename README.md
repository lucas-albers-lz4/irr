# Helm Image Override

A command-line tool to automate the generation of Helm chart override files for redirecting container images to private or local registries.

## Overview

The `helm-image-override` tool analyzes Helm charts and automatically generates `values.yaml` override files to redirect container image pulls from specified public registries to a target registry. This is especially useful when:

- Using a private registry like Harbor with pull-through cache
- Working in air-gapped environments
- Enforcing image provenance requirements
- Migrating to a new container registry

## Installation

### Binary Installation (not available yet)

Download the latest release for your platform from the [releases page](https://github.com/lalbers/helm-image-override/releases).

```bash
# Linux
curl -LO https://github.com/lalbers/helm-image-override/releases/latest/download/helm-image-override-linux-amd64
chmod +x helm-image-override-linux-amd64
mv helm-image-override-linux-amd64 /usr/local/bin/helm-image-override

# macOS
curl -LO https://github.com/lalbers/helm-image-override/releases/latest/download/helm-image-override-darwin-amd64
chmod +x helm-image-override-darwin-amd64
mv helm-image-override-darwin-amd64 /usr/local/bin/helm-image-override
```

### Building from Source

1. Clone the repository:
```bash
git clone https://github.com/lalbers/helm-image-override.git
cd helm-image-override
```

2. Build the binary:
```bash
make build
```

The binary will be created at `build/helm-image-override`. You can optionally add it to your PATH:

```bash
# Optional: Install to /usr/local/bin
sudo cp build/helm-image-override /usr/local/bin/
```

## Usage

### Basic Usage

```bash
helm-image-override \
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
```

### Example: Redirecting Images to Harbor

Harbor is a commonly used private registry with pull-through cache capabilities. To use `helm-image-override` with Harbor:

1. First, set up pull-through caching in Harbor:
   - Create projects in Harbor for each source registry (e.g., `dockerio`, `quayio`)
   - Configure them as proxy caches pointing to the appropriate source registries

2. Generate overrides for a chart:
   ```bash
   helm-image-override \
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

### prefix-source-registry (Default)

This strategy places images under a subdirectory named after the source registry:

| Original Image | Transformed Image |
|----------------|-------------------|
| docker.io/nginx:1.23 | harbor.example.com/dockerio/nginx:1.23 |
| quay.io/prometheus/node-exporter:v1.3.1 | harbor.example.com/quayio/prometheus/node-exporter:v1.3.1 |

This strategy helps avoid naming conflicts and maintains registry origin information.

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

# Run specific integration test (e.g., cert-manager)
make test-cert-manager
```

For detailed testing instructions, test architecture, and test coverage information, see [TESTING.md](TESTING.md).

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
- helm-image-override binary built

## Steps

1. **Pull the chart locally**
   ```bash
   helm repo add bitnami https://charts.bitnami.com/bitnami
   helm pull bitnami/nginx --untar --destination ./tmp
   ```

2. **Run the override tool**
   ```bash
   ./build/helm-image-override \
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
helm-image-override \
  --chart-path ./my-chart \
  --target-registry my-registry.example.com \
  --source-registries docker.io,quay.io \
  --registry-mappings ./registry-mappings.yaml \
  --output-file overrides.yaml
```

When no mappings file is provided, the tool will use the default behavior of prefixing the sanitized source registry name:
- `docker.io/nginx:1.23` -> `my-registry.example.com/dockerio/nginx:1.23`
- `quay.io/prometheus/node-exporter:v1.3.1` -> `my-registry.example.com/quayio/prometheus/node-exporter:v1.3.1`

With a mappings file, you can customize the target paths:
- `docker.io/nginx:1.23` -> `my-registry.example.com/docker-mirror/nginx:1.23`
- `quay.io/prometheus/node-exporter:v1.3.1` -> `my-registry.example.com/quay-mirror/prometheus/node-exporter:v1.3.1`
