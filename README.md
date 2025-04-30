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

### Recommended Workflow

For first-time users or when starting with a new chart, follow these steps:

1.  **Install Plugin:** Make sure you have installed the `irr` Helm plugin (see [Installation](#installation)).
2.  **Generate Skeleton Config:** Inspect your target chart, release, or entire cluster to automatically create a configuration file listing the source registries it uses.
    ```bash
    # For a local chart
    helm irr inspect --chart-path ./my-chart --generate-config-skeleton

    # For an installed release
    helm irr inspect my-release -n my-namespace --generate-config-skeleton

    # For all releases in the cluster (creates a comprehensive skeleton)
    helm irr inspect -A --generate-config-skeleton
    ```
    This creates a `registry-mappings.yaml` file. If the file already exists and you want to replace it with a newly generated one, add the `--overwrite-skeleton` flag to the command.
3.  **Configure Mappings:** Use the `helm irr config` command to specify the target registry for each source registry listed in the generated `registry-mappings.yaml` file. You'll need to run this command for *each* source registry you want to map.
    ```bash
    # Example: After generating the skeleton, configure the target for docker.io
    helm irr config --source docker.io --target my-registry.com/dockerhub-cache

    # Example: Configure the target for quay.io
    helm irr config --source quay.io --target my-registry.com/quayio-cache

    # Run similar commands for any other source registries listed in the skeleton...

    # You can view the current mappings at any time:
    helm irr config --list
    ```
    This updates the `registry-mappings.yaml` file. (See [Configuring Registry Mappings](#configuring-registry-mappings) for more details, including managing the file directly if preferred).
4.  **Generate Overrides:** Use the `override` command to generate the Helm values file based on your configured mappings. By default, `irr` reads `registry-mappings.yaml` from the current directory.
    ```bash
    # For a local chart
    helm irr override --chart-path ./my-chart --output-file my-chart-overrides.yaml

    # For an installed release (defaults output to <release-name>-overrides.yaml)
    helm irr override my-release -n my-namespace
    ```
5.  **Apply Overrides:** Use the generated override file when installing or upgrading your Helm release.
    ```bash
    # Install new release
    helm install my-release ./my-chart -f my-chart-overrides.yaml

    # Upgrade existing release
    helm upgrade my-release ./my-chart -f my-chart-overrides.yaml
    ```

**Verification Tip:** To visually confirm the image changes made by the overrides, you can compare the output of `helm template` with and without the override file. Run these commands and use a diff tool (like `diff` or `meld`) to inspect the differences:

```bash
# Template without overrides
helm template my-release ./my-chart > template-original.yaml

# Template *with* overrides
helm template my-release ./my-chart -f my-chart-overrides.yaml > template-with-overrides.yaml

# Compare the files
diff template-original.yaml template-with-overrides.yaml
```
This helps ensure the overrides are modifying the image fields as expected. Note that `irr override` includes an internal validation step (`--validate` flag, enabled by default), but that primarily checks for templating *errors*, not the specific content changes.

### Basic Usage

Details on generating and applying overrides can be found in the [Recommended Workflow](#recommended-workflow) section.

Alternatively, you can operate directly on an installed Helm release (ensure you specify the namespace if not `default`):

```bash
# Generate overrides for a release (defaults output to <release-name>-overrides.yaml)
helm irr override my-release -n my-namespace \
  --target-registry harbor.example.com:5000 \
  --source-registries docker.io,quay.io
```

*Note on Validation:* By default, `helm irr override` automatically runs a validation step (similar to `helm template`) after generating the override file to check if the chart templates correctly with the new values. If `override` fails, but you suspect the override file itself was generated correctly, you can try running the command with the `--no-validate` flag. If it succeeds with `--no-validate`, the issue likely lies in the validation step, potentially due to missing values context when running against a local chart path instead of a deployed release.

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

2. Follow the [Recommended Workflow](#recommended-workflow) to generate the override file using your Harbor registry details (e.g., `--target-registry harbor.example.com` in the `helm irr config` step) and apply it during `helm install` or `helm upgrade`.

## Configuring Registry Mappings

You can specify custom registry mappings using a YAML configuration file (defaults to `registry-mappings.yaml`). This is the recommended way to control how source registries are mapped to your target registry's directory structure, especially when dealing with multiple sources or needing specific path translations.

### Defining Mappings

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

  # Optional fields for more control:
  # defaultTarget: "your-fallback-registry.com/generic-prefix"
  # strictMode: false # Set to true to fail if a source registry isn't explicitly mapped
```

You can also run `irr inspect --generate-config-skeleton` against a chart to generate a starting config file with detected source registries.

Alternatively, you can use the `helm irr config` command to manage the mappings file. For example, to add or update the `docker.io` mapping:

```bash
helm irr config --source docker.io --target my-registry.example.com/docker-mirror
```

### How Mappings are Applied

When running `helm irr override`, the tool applies mappings with the following precedence:

1.  **Explicit Mapping (Highest Priority):** If a file is provided via `--registry-file`, mappings defined within it take precedence. Any image whose source registry matches an enabled entry in the `mappings` list will use the exact `target` defined there.
2.  **Fallback Behavior (if `strictMode: false`):** If an image's source registry is listed in `--source-registries` but isn't found in the mapping file (or the file isn't provided), the tool uses the registry specified by the `--target-registry` flag combined with the default path strategy (e.g., `prefix-source-registry`). You can also define a `registries.defaultTarget` within the mapping file to control this fallback behavior more explicitly.
3.  **Strict Mode (`strictMode: true`):** If enabled in the mapping file, the override command will fail if any registry listed in `--source-registries` does not have an explicit, enabled mapping in the file. This prevents accidental use of fallback targets.

Using a mapping file is particularly useful for:

*   Handling special cases where the default path strategy doesn't produce the desired result.
*   Working with registries that have specific naming requirements for certain images.
*   Setting up custom paths for pull-through cache configurations.
*   Ensuring only explicitly configured source registries are rewritten (using `strictMode`).

Use the mappings file with the `override` command using the `--registry-file` flag:

```bash
helm irr override \
  --chart-path ./my-chart \
  --target-registry my-registry.example.com \
  --source-registries docker.io,quay.io \
  --registry-file ./registry-mappings.yaml \
  --output-file overrides.yaml
```

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

- **Hardcoded Images:** Images defined outside `values.yaml` or other standard Helm value sources (e.g., hardcoded directly within template files like `deployment.yaml`) are not detected or processed by `irr`. Overrides must be applied manually for these cases.
- **Complex Templating:** Image references constructed through complex Go templating logic within Helm templates might not always be identified correctly by the static analysis performed by `irr`.
- **Helm OCI Artifacts:** Currently, `irr` processes local chart directories and `.tgz` archives, but does not directly support Helm charts stored as OCI artifacts in registries.

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