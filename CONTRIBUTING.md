# Contributing to IRR

Thank you for contributing. This guide covers the workflow, commands, and conventions for this repository.

## Quick Start

```bash
git clone https://github.com/lucas-albers-lz4/irr
cd irr
make build
make test
```

## Commands

| Command | Purpose |
|---------|---------|
| `make build` | Build the `irr` binary into `bin/irr` |
| `make test` | Run unit tests and CLI syntax tests |
| `make test-quiet` | Run unit tests with minimal output |
| `make test-integration` | Run end-to-end integration tests |
| `make test-charts TARGET_REGISTRY=...` | Validate against real Helm charts |
| `make lint` | Run golangci-lint |
| `make helm-install` | Install the local build as a Helm plugin |
| `make dist` | Build release artifacts |

See [docs/developer/build-and-test.md](docs/developer/build-and-test.md) for details on the test layers and chart tooling.

## Workflow

1. Fork the repository and create a feature branch from `main`:

   ```bash
   git checkout -b feat/my-change
   ```

2. Make your change. Follow the existing code style and add tests.
3. Run the checks before committing:

   ```bash
   make build
   make lint
   make test
   ```

4. Push the branch and open a pull request against `main`.
5. In the PR description, summarize the change and any testing you did.

Keep the change focused. If a change spans several concerns, split it into separate PRs.

## Conventions

- **Go code** — follow standard Go formatting (`gofmt`). Use the `pkg/log` package for logging; logs go to `stderr`, user-facing output to `stdout`.
- **Filesystem access** — use the `afero.Fs` abstraction (production: `OsFs`; tests: `MemMapFs`). See [docs/developer/filesystem-mocking.md](docs/developer/filesystem-mocking.md).
- **Markdown** — use lowercase `.md` extensions. Link to other docs with relative paths. Keep prose clear and simple; when you rewrite existing prose, prefer plain language (short sentences, active voice).
- **Registry mappings** — only the structured YAML format (`registries.mappings`) is supported. Do not document the legacy key-value format.
- **Do not** change the plugin ID (`irr` in `plugin.yaml`) or the Go module path (`github.com/lucas-albers-lz4/irr`) — both are public identifiers that existing installs depend on.

## Tests

- Unit tests live next to the packages they test (`pkg/...`, `cmd/irr/...`).
- Integration tests live in `test/integration/` and run end-to-end CLI flows.
- Chart validation uses `test/tools/test-charts.py` against the charts in `test-data/`.
- Test fixtures live in `pkg/testutil/fixtures/` and `test-data/`.

Run the link checker on documentation changes:

```bash
python3 tools/check-links.py
```

## Reporting Issues

Report bugs and feature requests in the [issue tracker](https://github.com/lucas-albers-lz4/irr/issues). Include the `irr` version, the command you ran, and the output.
