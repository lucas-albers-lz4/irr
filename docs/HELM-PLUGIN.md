# IRR Helm Plugin

The IRR (Image Relocation and Rewrite) Helm plugin allows you to seamlessly integrate image relocation capabilities directly into Helm workflows. This plugin wraps around the IRR CLI tool, providing the same functionality but enhanced with Helm-specific features.

## Installation

### Prerequisites

- Helm 3.x
- IRR CLI tool installed and available in your PATH

### Installation Methods

#### From Source

```bash
# Clone the repository
git clone https://github.com/lalbers/irr
cd irr

# Build and install
make build
make helm-install
```

#### Manual Installation

```bash
# Download the latest release
curl -L https://github.com/lalbers/irr/releases/latest/download/helm-irr-$(uname -s)-$(uname -m).tar.gz -o helm-irr.tar.gz

# Extract and install
mkdir -p ~/.helm/plugins/irr
tar -xzf helm-irr.tar.gz -C ~/.helm/plugins/irr
```

## Usage

The plugin provides the same core commands as the IRR CLI tool, with additional Helm-specific features:

### Inspect Command

```bash
# Inspect a chart before installation
helm irr inspect --chart-path ./my-chart

# Inspect an installed release
helm irr inspect my-release

# Inspect with specific namespace
helm irr inspect my-release -n my-namespace

# Generate a config skeleton from an installed release
helm irr inspect my-release --generate-config-skeleton my-config.yaml
```

### Override Command

```bash
# Generate overrides for a chart
helm irr override --chart-path ./my-chart \
  --target-registry registry.local \
  --source-registries docker.io,quay.io \
  --output-file overrides.yaml

# Generate overrides for an installed release
helm irr override my-release \
  --target-registry registry.local \
  --source-registries docker.io,quay.io

# Override with validation
helm irr override my-release \
  --target-registry registry.local \
  --source-registries docker.io,quay.io \
  --validate
```

### Validate Command

```bash
# Validate overrides for a chart
helm irr validate --chart-path ./my-chart --values overrides.yaml

# Validate overrides for an installed release
helm irr validate my-release --values overrides.yaml
```

## Helm Integration Features

The plugin enhances the standard IRR functionality with Helm-specific features:

1. **Release Awareness**: When a Helm release name is provided as the first argument, the plugin automatically:
   - Retrieves the chart information from the release
   - Loads the current values from the release
   - Uses the release context for generating overrides

2. **Namespace Inheritance**: The plugin respects the Kubernetes namespace context, supporting:
   - Explicit namespace via `-n` or `--namespace` flag
   - Inheritance from Helm's configured namespace
   - Default to `default` namespace if none specified

3. **File Handling**: 
   - For overrides, automatically names output files after the release (e.g., `my-release-overrides.yaml`)
   - Safely handles file writing with proper permissions
   - Protects against overwriting existing files

4. **Validation Integration**:
   - Adds `--validate` flag to the override command for immediate validation
   - Provides clear upgrade command suggestions after successful validation

## Configuration

The plugin uses the same configuration options as the IRR CLI tool:

```yaml
# ~/.irr.yaml
registry_mappings:
  docker.io: "registry.local/docker"
  quay.io: "registry.local/quay"

exclude_registries:
  - "internal-registry.company.com"

path_strategy: "prefix-source-registry"  # default
```

## Examples

### Integration with Helm Workflow

```bash
# 1. Install a chart normally
helm install my-app ./charts/my-app

# 2. Inspect the installed chart
helm irr inspect my-app

# 3. Generate registry overrides
helm irr override my-app \
  --target-registry registry.local \
  --source-registries docker.io,quay.io

# 4. Validate the overrides
helm irr validate my-app --values my-app-overrides.yaml

# 5. Apply the overrides in an upgrade
helm upgrade my-app ./charts/my-app -f my-app-overrides.yaml
```

### CI/CD Pipeline

```yaml
# Example CI/CD pipeline step
steps:
  - name: Install Helm IRR Plugin
    run: |
      helm plugin install https://github.com/lalbers/irr

  - name: Generate Image Overrides
    run: |
      helm irr override my-app \
        --target-registry $REGISTRY \
        --source-registries docker.io,quay.io \
        --non-interactive

  - name: Validate Before Deployment
    run: |
      helm irr validate my-app --values my-app-overrides.yaml

  - name: Deploy with Overrides
    run: |
      helm upgrade --install my-app ./charts/my-app \
        -f my-app-overrides.yaml
```

## Troubleshooting

### Common Issues

1. **Plugin Not Found**: Ensure the plugin is correctly installed with `helm plugin list`.

2. **Release Not Found**: Check that the release exists and you have specified the correct namespace with:
   ```bash
   helm list -n <namespace>
   ```

3. **Namespace Issues**: If working with multiple namespaces, always explicitly specify the namespace with `-n`:
   ```bash
   helm irr inspect my-release -n my-namespace
   ```

4. **File Already Exists**: The plugin prevents accidental file overwrites. Use a different filename or manually remove the existing file:
   ```bash
   Error: output file 'my-release-overrides.yaml' already exists
   ```

5. **Chart Resolution**: If the exact chart version cannot be found, you may see:
   ```
   Error: could not resolve chart path
   ```
   Use `--chart-path` to explicitly provide the chart location.

### Debug Mode

Enable debug logging for more information:

```bash
helm irr inspect my-release --debug
```

Or set the environment variable:

```bash
IRR_DEBUG=true helm irr inspect my-release
```

## Support and Feedback

For issues, questions, or contributions:

- GitHub Issues: [github.com/lalbers/irr/issues](https://github.com/lalbers/irr/issues)
- Documentation: [github.com/lalbers/irr/docs](https://github.com/lalbers/irr/docs) 