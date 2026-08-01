# Architecture

This document describes how IRR works: the core processing pipeline, the command surface, and the key design decisions. The original design document is archived at [docs/archive/DEVELOPMENT.md](../archive/DEVELOPMENT.md).

## Overview

IRR (Image Relocation and Rewrite) automates the generation of Helm override `values.yaml` files that redirect container image pulls from public registries to a private or local registry. The tool reads a chart or release, finds its image references, maps source registries to a target registry, and produces a minimal override file.

## Core Pipeline

1. **Chart loading** — load chart data (`values.yaml`, `Chart.yaml`) with `helm.sh/helm/v3` chart utilities and standard YAML parsing.
2. **Value traversal** — walk the nested values dictionary recursively, including subchart paths (using the alias from the parent chart's `Chart.yaml` dependencies).
3. **Image identification** — detect image references by key patterns:
   - A map with `registry`, `repository`, and `tag` keys.
   - A map with `repository` and `tag` keys (implies `docker.io`).
   - A string value named `image` (for example `image: myrepo/myimage:tag`).
   - Structures inside lists are detected and warned about, but not processed.
4. **Image parsing** — extract source registry, repository path, and tag or digest. Handle the implicit `docker.io` default.
5. **Filtering** — match the source registry against the user-provided list or the mapping file.
6. **Target URL construction** — generate the new image reference with the configured path strategy.
7. **Override generation** — build a dictionary that mirrors the value path, containing only the minimal keys needed to redirect the image. Sibling keys such as `tag` or `pullPolicy` are not copied.
8. **Output** — serialize the override dictionary to YAML.

## Commands

| Command | Purpose |
|---------|---------|
| `irr inspect` | Discover images in a chart or release without modifying anything. Optionally filter by source registry and generate a config skeleton. |
| `irr override` | Generate the override values file. Reads mappings, detects images, and applies the path strategy. |
| `irr validate` | Pre-flight check: run `helm template` with the overrides to prove the chart still renders. |
| `irr config` | Add, update, remove, and list registry mappings. |

The Helm plugin (`helm irr`) exposes the same commands with release and namespace awareness.

## Path Strategy: `prefix-source-registry` (Default)

The strategy prepends a sanitized form of the source registry to the repository path:

| Source | Result |
|--------|--------|
| `docker.io/bitnami/redis:latest` → target `myharbor.internal:5000` | `myharbor.internal:5000/dockerio/bitnami/redis:latest` |
| `nginx:latest` (implicit docker.io) | `myharbor.internal:5000/dockerio/library/nginx:latest` |

Sanitization rules:

- Periods are removed: `gcr.io` → `gcrio`
- Hyphens are preserved: `k8s.gcr.io` → `k8sgcrio`
- Port numbers are removed: `registry:5000` → `registry`

This keeps lineage by origin and matches Harbor pull-through project naming conventions. The `flat` strategy is removed.

## Registry Mappings

Mappings redirect specific source registries to different target paths. Only the structured YAML format is supported:

```yaml
registries:
  mappings:
    - source: docker.io
      target: my-registry.example.com/docker-mirror
  # defaultTarget: "your-fallback-registry.com/generic-prefix"
  # strictMode: false
```

See [configuration.md](../user/configuration.md) for the full format and the `irr config` command.

## Error Handling and Exit Codes

- **0** — success
- **1** — general runtime error
- **2** — input or configuration error (invalid chart path, invalid registry format, conflicting flags)
- **3** — chart parsing error (malformed `values.yaml` or `Chart.yaml`)
- **4** — image processing error (unparsable image reference)
- **5** — unsupported structure error (only with `--strict`)

Error messages identify the nature and location of the problem, for example: `Error parsing image in values path 'parent.subchart.image': invalid format`.

## Unsupported Structures

The tool warns about (and with `--strict` fails on):

- Images split across multiple keys outside the expected structure.
- Non-string tag values.
- Invalid registry names or malformed image references.

## Technology Stack

- **Language:** Go
- **Helm SDK:** `helm.sh/helm/v3` for chart loading and templating
- **YAML:** `sigs.k8s.io/yaml`
- **CLI:** `spf13/cobra` and `spf13/pflag`
- **Logging:** Go standard library `log/slog` (see [logging.md](logging.md))
- **Filesystem abstraction:** `afero` — production code uses `OsFs`; tests use `MemMapFs` (see [filesystem-mocking.md](filesystem-mocking.md))

## Repository Layout

| Path | Purpose |
|------|---------|
| `cmd/irr/` | CLI entry points, flag parsing, output handling |
| `pkg/chart/` | Chart loading, generator, analysis |
| `pkg/image/` | Image reference parsing and detection |
| `pkg/override/` | Override structure generation |
| `pkg/registry/` | Registry mapping configuration |
| `pkg/strategy/` | Path strategy implementations |
| `pkg/rules/` | Parameter classification and validation rules |
| `pkg/log/` | Logging setup |
| `internal/helm/` | Helm SDK integration (release values, chart resolution, templating) |
| `test/integration/` | End-to-end CLI tests |

## Related Documentation

- [build-and-test.md](build-and-test.md) — test layers and chart tooling
- [rules.md](rules.md) — parameter classification
- [logging.md](logging.md) — log control
