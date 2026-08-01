# Usage

This guide covers the main IRR workflows: inspect, configure, override, and validate. IRR runs as a Helm plugin (`helm irr ...`) or as a standalone binary (`irr ...`).

## Recommended Workflow

For a new chart or release, follow these steps:

### 1. Inspect and Generate a Skeleton Config

Inspect your chart, release, or cluster to find the source registries it uses, and create a configuration skeleton:

```bash
# For a local chart
helm irr inspect --chart-path ./my-chart --generate-config-skeleton

# For an installed release
helm irr inspect my-release -n my-namespace --generate-config-skeleton

# For all releases in the cluster (creates a comprehensive skeleton)
helm irr inspect -A --generate-config-skeleton
```

This creates `registry-mappings.yaml` in the current directory. If the file exists and you want to replace it, add `--overwrite-skeleton`.

### 2. Configure Mappings

Set the target registry for each source registry:

```bash
# Map docker.io to a mirror
helm irr config --source docker.io --target my-registry.com/dockerhub-cache

# Map quay.io to a mirror
helm irr config --source quay.io --target my-registry.com/quayio-cache

# View the current mappings
helm irr config --list
```

See [configuration.md](configuration.md) for the mapping file format and all `config` options.

### 3. Generate Overrides

Generate the Helm values file from your mappings:

```bash
# For a local chart
helm irr override --chart-path ./my-chart --output-file my-chart-overrides.yaml

# For an installed release (output defaults to <release-name>-overrides.yaml)
helm irr override my-release -n my-namespace
```

By default, `override` reads `registry-mappings.yaml` from the current directory and runs an internal validation step (like `helm template`) after generating the file. Use `--no-validate` to skip that step.

### 4. Apply the Overrides

Use the generated file when installing or upgrading:

```bash
# Install a new release
helm install my-release ./my-chart -f my-chart-overrides.yaml

# Upgrade an existing release
helm upgrade my-release ./my-chart -f my-chart-overrides.yaml
```

### 5. Validate (Optional Pre-flight)

Check that the chart renders correctly with the overrides:

```bash
helm irr validate my-release -n my-namespace -f my-chart-overrides.yaml
```

This runs `helm template` internally. It checks for rendering errors, not content changes.

### Visual Verification Tip

To see exactly which images changed, compare `helm template` output with and without the override file:

```bash
helm template my-release ./my-chart > template-original.yaml
helm template my-release ./my-chart -f my-chart-overrides.yaml > template-with-overrides.yaml
diff template-original.yaml template-with-overrides.yaml
```

## Basic Usage

Generate overrides directly for an installed release:

```bash
helm irr override my-release -n my-namespace \
  --target-registry registry.example.com:5000 \
  --source-registries docker.io,quay.io
```

## Plugin Mode Notes

When running as a Helm plugin (`helm irr ...`):

- **Release context** — `inspect`, `override`, and `validate` can operate on a deployed release name instead of a local chart path. The plugin uses the release's values, namespace, and chart source.
- **Namespace awareness** — the plugin respects `-n`, the current context, or `default`.
- **Output defaults** — `override <release-name>` writes `<release-name>-overrides.yaml` in the current directory. With `--chart-path`, output defaults to `stdout`.

## Air-Gapped Environments

Generate the overrides where you have internet access, then apply them where you do not:

```bash
# Internet-connected environment: generate overrides
helm irr override --chart-path ./my-chart --target-registry internal-registry.local \
  --source-registries docker.io,quay.io --output-file overrides.yaml

# Copy images to the internal registry (skopeo is complementary, not part of IRR)
skopeo copy docker://docker.io/nginx:latest docker://internal-registry.local/dockerio/nginx:latest

# Air-gapped environment: deploy with the overrides
helm install my-release ./my-chart -f overrides.yaml
```

## Working with Complex Charts

```bash
# Inspect with debug output
LOG_LEVEL=DEBUG helm irr inspect --chart-path ./kube-prometheus-stack

# Generate overrides with a higher threshold
helm irr override --chart-path ./kube-prometheus-stack --threshold 90 --output-file overrides.yaml

# Validate the result
helm irr validate --chart-path ./kube-prometheus-stack --values overrides.yaml
```

## Troubleshooting Image Detection

```bash
# Run with debug logging to see why an image is not detected
LOG_LEVEL=DEBUG helm irr inspect --chart-path ./problematic-chart

# Test with strict mode and dry-run to surface all issues
helm irr override --chart-path ./problematic-chart --strict --dry-run
```

## Best Practices

- Store registry configurations in version control.
- Use environment-specific configuration files.
- Validate overrides before applying them.
- Use `--dry-run` before generating final overrides.
- Re-analyze charts after updates.
- Update registry mappings when you add new source registries.

## Related Documentation

- [installation.md](installation.md) — install and upgrade
- [configuration.md](configuration.md) — mapping file format
- [cli-reference.md](cli-reference.md) — all commands and flags
- [plugin-specific.md](plugin-specific.md) — plugin behaviors in detail
- [troubleshooting.md](troubleshooting.md) — common problems
