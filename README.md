# IRR (Image Registry Rewrite)

A command-line tool to automate the generation of Helm chart override files for redirecting container images to private or local registries.

## Overview

The `irr` tool analyzes Helm charts and automatically generates `values.yaml` override files to redirect container image pulls from specified public registries to a target registry. This is especially useful when:

- Using a private registry like Harbor with pull-through cache
- Working in air-gapped environments
- Enforcing image provenance requirements
- Migrating to a new container registry

## Installation

### Helm Plugin Installation

The recommended way to install and use `irr` is as a Helm plugin:

```bash
helm plugin install https://github.com/lucas-albers-lz4/irr
```

Binary distribution via a brew tap is planned but not yet available.

### Building from Source (for development)

If you need to build the tool locally for development purposes:

1. Clone the repository:
```bash
git clone https://github.com/lalbers/irr.git
cd irr
```

2. Build the binary:
```bash
make build
```

The binary will be created at `bin/irr`.

## Usage

### Basic Usage

To generate an override file for a local Helm chart:

```bash
helm irr override \
  --chart-path ./my-chart \
  --target-registry harbor.example.com:5000 \
  --source-registries docker.io,quay.io \
  --output-file overrides.yaml
```

This generates an `overrides.yaml` file that you can use with Helm:

```bash
helm install my-release ./my-chart -f overrides.yaml
```

Alternatively, you can operate on an installed Helm release (ensure you specify the namespace if not `default`):

```bash
# This will create my-release-overrides.yaml by default
helm irr override my-release -n my-namespace \
  --target-registry harbor.example.com:5000 \
  --source-registries docker.io,quay.io
```

### Helm Plugin Usage Notes

When running `irr` as a Helm plugin (`helm irr ...`), there are a few key differences compared to running the standalone binary:

*   **Release Context:** Commands like `inspect`, `override`, `validate` can operate directly on a deployed Helm release name instead of just a local chart path. When doing so, the plugin uses the release's context (values, namespace, chart source).
*   **Namespace Awareness:** The plugin respects the Helm/Kubernetes namespace (via `-n`, current context, or `default`).
*   **Output Default for `override <release-name>`:** When `helm irr override <release-name>` is used (without `--chart-path`), the default output is a file named `<release-name>-overrides.yaml` in the current directory instead of `stdout`. If `--chart-path` is used, it defaults to `stdout`.

### Flags and Options

Each `irr` subcommand (`inspect`, `override`, `validate`, `config`) supports various flags to control its behavior. For a detailed list of flags available for each command, please refer to the [CLI Reference documentation](docs/cli-reference.md).

### Example: Redirecting Images to Harbor

Harbor is a commonly used private registry with pull-through cache capabilities. To use `irr` with Harbor:

1. First, set up pull-through caching in Harbor:
   - Create projects in Harbor for each source registry (e.g., `dockerio`, `quayio`)
   - Configure them as proxy caches pointing to the appropriate source registries

2. Generate overrides for a chart:
   ```bash
   helm irr override \
     --chart-path ./prometheus \
     --target-registry harbor.example.com \
     --source-registries docker.io,quay.io \
     --output-file prometheus-overrides.yaml
   ```

3. Install the chart with the overrides:
   ```bash
   helm install prometheus ./prometheus -f prometheus-overrides.yaml
   ```

## Advanced Registry Mapping (Using --registry-file)

For cases where you need precise control over specific image mappings, irr provides a `--registry-file` flag that accepts a YAML file with structured source-to-target mappings.

### Using the Registry Mappings File

The registry mapping file (`registry-mappings.yaml` by default, or specified with `--registry-file`) uses a structured format. See the [CLI Reference](docs/cli-reference.md#configuration-file-registry-mappingsyaml) for the full structure.

Example structure (`my-mappings.yaml`):
```yaml
version: "1.0"
registries:
  mappings:
    - source: "docker.io"
      target: "my-registry.io/dockerhub"
    - source: "quay.io"
      target: "my-registry.io/quay"
    - source: "k8s.gcr.io"
      target: "my-registry.io/google-k8s"
```

To use this file:

```bash
helm irr override \
  --chart-path ./my-chart \
  --target-registry fallback.registry.com \
  --source-registries docker.io,quay.io,k8s.gcr.io \
  --registry-file my-mappings.yaml \
  --output-file overrides.yaml
```

### How Registry Mappings Work

1. Mappings in the file specified by `--registry-file` have the highest priority.
2. Any image whose source registry matches an enabled entry in the `mappings` list will use the exact target defined there.
3. Images from source registries not found in the mappings file (or if the file is not provided) will fall back to using `--target-registry` combined with the default path strategy (e.g., `prefix-source-registry`), unless `strictMode` is enabled in the config.
4. The `--target-registry` flag is often still required as a fallback or default target.

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
```

For detailed testing instructions, test architecture, and test coverage information, see [TESTING.md](TESTING.md).

# Testing a Single Helm Chart

This workflow describes how to test `irr` against a local chart directory, which is useful before applying overrides to a deployed release.

## Prerequisites
- Helm CLI installed
- `irr` Helm plugin installed (`helm plugin install ...`)

## Steps

1. **Pull the chart locally**
   ```bash
   helm repo add bitnami https://charts.bitnami.com/bitnami
   helm pull bitnami/nginx --untar --destination ./tmp
   ```

2. **Run the override tool against the local chart**
   ```bash
   # Using helm irr on a local path typically defaults to stdout
   helm irr override \
     --chart-path ./tmp/nginx \
     --target-registry my-registry.example.com \
     --source-registries docker.io,quay.io \
     --output-file ./tmp/overrides.yaml \
     --log-level info
   ```
   *Note: When running `helm irr override <release-name>` against an installed release, the output defaults to `<release-name>-overrides.yaml`.*

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

You can specify custom registry mappings using a YAML configuration file (defaults to `registry-mappings.yaml`). This allows you to control how source registries are mapped to your target registry's directory structure.

Create a `registry-mappings.yaml` file with the following structure:

```yaml
version: "1.0" # Optional but recommended
registries:
  mappings:
    - source: "quay.io"
      target: "my-registry.example.com/quay-mirror"
      # enabled: true (optional, defaults to true)
      # description: "Optional description"
    - source: "docker.io"
      target: "my-registry.example.com/docker-mirror"
    - source: "gcr.io"
      target: "my-registry.example.com/gcr-mirror"
```

You can also run `irr inspect --generate-config-skeleton` against a chart to generate a starting config file with detected source registries.

Alternatively, you can use the `helm irr config` command to manage the mappings file. For example, to add or update the `docker.io` mapping:

```bash
helm irr config --source docker.io --target my-registry.example.com/docker-mirror
```

Use the mappings file with the `override` command:

```bash
helm irr override \
  --chart-path ./my-chart \
  --target-registry my-registry.example.com \
  --source-registries docker.io,quay.io \
  --registry-file ./registry-mappings.yaml \
  --output-file overrides.yaml
```

When no mappings file is provided, or if a source registry isn't found in the file and strict mode is disabled, the tool will use the default path strategy (usually `prefix-source-registry`) with the `--target-registry` value. For example:
```