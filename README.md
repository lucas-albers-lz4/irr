# IRR — Image Registry Rewrite

A command-line tool that generates Helm chart override files to redirect container images to private or local registries.

## What it does

- **inspect** — analyzes a chart or release and lists its image references and source registries
- **override** — generates a `values.yaml` override file that redirects images to your target registry
- **validate** — checks that a chart renders correctly with the generated overrides
- **config** — manages the registry mapping file

Use IRR when you need a private registry with pull-through cache (for example Harbor), an air-gapped environment, enforced image provenance, or a registry migration.

```bash
helm irr inspect my-release
helm irr override my-release --target-registry registry.local
helm irr validate my-release -f my-release-overrides.yaml
```

## Install

**Recommended — Helm plugin:**

```bash
helm plugin install https://github.com/lucas-albers-lz4/irr
```

**Or from a GitHub Release** — download `helm-irr-<version>-<os>-<arch>.tar.gz` from the [releases page](https://github.com/lucas-albers-lz4/irr/releases). Supported platforms: `linux-amd64`, `linux-arm64`, `darwin-arm64` (`darwin-amd64` is not built).

**Or build from source:**

```bash
git clone https://github.com/lucas-albers-lz4/irr
cd irr
make build
```

Full instructions: [Installation guide](docs/user/installation.md)

## Quick Start

1. Install the plugin (above).
2. Generate a config skeleton from your chart or release:

   ```bash
   helm irr inspect --chart-path ./my-chart --generate-config-skeleton
   ```

3. Map each source registry to your target:

   ```bash
   helm irr config --source docker.io --target my-registry.com/dockerhub-cache
   ```

4. Generate overrides and apply them:

   ```bash
   helm irr override --chart-path ./my-chart --output-file my-chart-overrides.yaml
   helm install my-release ./my-chart -f my-chart-overrides.yaml
   ```

Full workflow: [Usage guide](docs/user/usage.md)

## Documentation

| I want to… | Start here |
|------------|------------|
| **Install and use IRR** | **[User guide](docs/user/README.md)** |
| **Build, test, or contribute** | **[Developer guide](docs/developer/README.md)** |
| Browse all docs | [docs/README.md](docs/README.md) |
| Release history | [CHANGELOG.md](CHANGELOG.md) |
| FAQ | [docs/FAQ.md](docs/FAQ.md) |

### User guide (highlights)

| Guide | Summary |
|-------|---------|
| [Usage](docs/user/usage.md) | Recommended workflow |
| [Configuration](docs/user/configuration.md) | Registry mappings |
| [CLI reference](docs/user/cli-reference.md) | All commands and flags |
| [Troubleshooting](docs/user/troubleshooting.md) | Common problems |

### Developer guide (highlights)

| Guide | Summary |
|-------|---------|
| [Architecture](docs/developer/architecture.md) | Design and key decisions |
| [Build and test](docs/developer/build-and-test.md) | Tests and chart tooling |
| [Release process](docs/developer/release-process.md) | Releases and tags |

## Supported Image Reference Formats

IRR detects these patterns in Helm chart values:

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

Details: [Image patterns](docs/user/image-patterns.md)

## Limitations

- **Hardcoded images** — images defined directly in templates (not in `values.yaml`) are not detected. Apply overrides manually for those.
- **Complex templating** — image references built with complex Go templating logic might not be identified.
- **OCI artifacts** — IRR processes local chart directories and `.tgz` archives, not charts stored as OCI artifacts.

## Repository layout

| Path | Purpose |
|------|---------|
| [`cmd/`](cmd/) | CLI entry points |
| [`pkg/`](pkg/) | Core packages (chart, override, registry, rules) |
| [`internal/helm/`](internal/helm/) | Helm SDK integration |
| [`docs/user/`](docs/user/) | End-user documentation |
| [`docs/developer/`](docs/developer/) | Build and development documentation |
| [`test/`](test/) | Integration tests and chart-testing tooling |
| [`test-data/`](test-data/) | Vendored Helm charts used for testing |

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT — see [LICENSE](LICENSE).
