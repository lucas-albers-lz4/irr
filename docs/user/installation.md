# Installation

This guide explains how to install IRR as a Helm plugin or from source.

## Prerequisites

- Helm 3.x

## Install as a Helm Plugin (Recommended)

Install the plugin directly from the repository:

```bash
helm plugin install https://github.com/lucas-albers-lz4/irr
```

Verify the installation:

```bash
helm plugin list | grep irr
helm irr --version
```

The install hook (`install-binary.sh`) downloads the correct release binary for your platform and places it under the Helm plugins directory.

## Install from a GitHub Release (Manual)

The release assets are named `helm-irr-<version>-<os>-<arch>.tar.gz`. Supported platforms:

- `linux-amd64`
- `linux-arm64`
- `darwin-arm64`

`darwin-amd64` is not built. If you need it, build from source.

For example, on a 64-bit Linux system, install version 0.0.18 like this:

```bash
curl -L https://github.com/lucas-albers-lz4/irr/releases/download/v0.0.18/helm-irr-0.0.18-linux-amd64.tar.gz -o helm-irr.tar.gz
mkdir -p ~/.helm/plugins/irr
tar -xzf helm-irr.tar.gz -C ~/.helm/plugins/irr
```

## Build from Source

For development or unsupported platforms:

```bash
git clone https://github.com/lucas-albers-lz4/irr
cd irr
make build
make helm-install
```

The binary is created at `bin/irr`.

## Upgrade

Re-run the installation for the new version:

```bash
helm plugin update irr
```

Or install again from the repository. Plugin configuration in `~/.helm/plugins/irr` is replaced by the new release.

## Uninstall

```bash
helm plugin uninstall irr
```

## Next Steps

- [Usage](usage.md) — the recommended workflow
- [CLI reference](cli-reference.md) — all commands and flags
