# Configuration

IRR uses a YAML configuration file to map source registries to your target registry. The default file is `registry-mappings.yaml` in the current directory.

## The Mapping File

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

The repository ships a documented template at `registry-mappings.yaml`.

## How Mappings are Applied

When you run `helm irr override`, the tool applies mappings in this order:

1. **Explicit mapping (highest priority)** — a file provided with `--registry-file`. Any image whose source registry matches an enabled entry uses that entry's `target`.
2. **Fallback behavior (if `strictMode: false`)** — if an image's source registry is in `--source-registries` but has no mapping entry (or no file is provided), the tool uses `--target-registry` with the default path strategy (for example `prefix-source-registry`). A `registries.defaultTarget` in the mapping file controls this fallback more explicitly.
3. **Strict mode (`strictMode: true`)** — the override command fails if any registry in `--source-registries` has no explicit, enabled mapping. This prevents accidental use of fallback targets.

## Managing Mappings with `irr config`

Use the `config` command to add, update, or list mappings:

```bash
# Add or update the docker.io mapping
helm irr config --source docker.io --target my-registry.example.com/docker-mirror

# Remove a mapping
helm irr config --source quay.io --remove

# List current mappings
helm irr config --list

# Use a specific mapping file
helm irr config --file ./custom-map.yaml --source docker.io --target my-registry.example.com/docker
```

## Using the Mapping File with `override`

```bash
helm irr override \
  --chart-path ./my-chart \
  --target-registry my-registry.example.com \
  --source-registries docker.io,quay.io \
  --registry-file ./registry-mappings.yaml \
  --output-file overrides.yaml
```

## When a Mapping File is Useful

- Handling special cases where the default path strategy gives the wrong result.
- Working with registries that have specific naming requirements.
- Setting up custom paths for pull-through cache configurations.
- Ensuring only explicitly configured source registries are rewritten (`strictMode`).

## The `--config` Flag

The standalone binary accepts a viper config file with `--config` (default `$HOME/.irr.yaml`). The mapping format in this guide belongs in `registry-mappings.yaml`; do not put registry mappings in the `--config` file.

## Related Documentation

- [usage.md](usage.md) — the recommended workflow
- [cli-reference.md](cli-reference.md) — all commands and flags
