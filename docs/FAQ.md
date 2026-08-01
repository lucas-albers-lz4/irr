# Frequently Asked Questions

## Installation and Setup

### How do I install IRR?

Install it as a Helm plugin:

```bash
helm plugin install https://github.com/lucas-albers-lz4/irr
```

Or download a release asset (`helm-irr-<version>-<os>-<arch>.tar.gz`) from the [releases page](https://github.com/lucas-albers-lz4/irr/releases). See [Installation](user/installation.md).

### Which platforms are supported?

`linux-amd64`, `linux-arm64`, and `darwin-arm64`. There is no `darwin-amd64` build; build from source if you need it.

### Why does `helm irr` say the plugin is not found?

Check that the plugin is installed:

```bash
helm plugin list
```

If the binary is missing, reinstall the plugin.

## Usage

### Why does `override` produce no changes?

The chart may use hardcoded images in templates, or image references built with complex templating. IRR only processes images it finds in standard value sources (`values.yaml` and related files). Apply overrides manually for hardcoded images.

### What does `--no-validate` do?

By default, `irr override` runs an internal validation step (like `helm template`) after generating the override file. `--no-validate` skips that step. Use it when validation fails but you believe the override file is correct — for example when validating against a local chart path that lacks the full values context of a deployed release.

### How do I redirect images for a release in another namespace?

Pass the namespace explicitly:

```bash
helm irr override my-release -n my-namespace
```

### How do I see only certain source registries?

Use `--source-registries`:

```bash
helm irr override --chart-path ./my-chart --source-registries docker.io,quay.io
```

## Configuration

### Where does IRR look for the configuration file?

The default is `registry-mappings.yaml` in the current directory. Use `--registry-file` to specify another file.

### What is `strictMode`?

With `strictMode: true`, `override` fails if any registry in `--source-registries` has no explicit, enabled mapping. This prevents accidental fallback targets.

### What is `--config` for?

The standalone binary accepts a viper config file with `--config` (default `$HOME/.irr.yaml`). Keep registry mappings in `registry-mappings.yaml`; the `--config` file is for other options.

## Troubleshooting

### How do I enable debug logging?

```bash
LOG_LEVEL=DEBUG helm irr inspect my-release
```

The legacy `IRR_DEBUG` variable is removed. See [Logging](developer/logging.md).

### Why do logs and results appear in different streams?

Logs go to `stderr`; command results go to `stdout`. This keeps diagnostic output from interfering with piped results.

### The validation step fails but the override file looks correct

Try `--no-validate` on the override command, then run `helm template` yourself to confirm. A common cause is missing values context when running against a local chart path instead of a deployed release.
