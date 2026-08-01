# Build and Test Guide

This guide explains how to build IRR, how to run its tests, and how the chart-testing tooling works. The original testing plan (strategy and conceptual checks) is archived at [docs/archive/TESTING-plan.md](../archive/TESTING-plan.md).

## Test Layers

IRR has three test layers:

| Layer | What it covers | Command |
|-------|----------------|---------|
| Unit tests | Package logic (`pkg/...`, `cmd/irr/...`) | `make test` |
| Integration tests | End-to-end CLI flows (`test/integration/...`) | `make test-integration` |
| Chart validation | Real Helm charts from `test-data/` | `make test-charts` |

## Coverage thresholds

CI (`.github/workflows/test-coverage.yml`) and `tools/codecov.sh` enforce a floor on five core packages: `pkg/chart`, `pkg/override`, `pkg/rules`, `pkg/analysis`, and `pkg/image`.

The intended target is **75%**. The enforced floor is currently **73%** because `pkg/chart` measures about **73.7%**. That shortfall is **pre-existing debt**: until the docs overhaul corrected Codecov/`CORE_PACKAGES` paths from the pre-migration module path to `github.com/lucas-albers-lz4/irr/...`, the threshold step did not match any coverage lines and did not fail. Raise the floor back to 75% after adding tests that close the `pkg/chart` gap.

## Running Tests

The Makefile provides the main entry points:

```bash
# All unit tests + CLI syntax tests
make test

# Unit tests with minimal output
make test-quiet

# Integration tests
make test-integration

# Chart validation against a target registry
make test-charts TARGET_REGISTRY=registry.example.com

# Specific complex-chart suites
make test-cert-manager
make test-kube-prometheus-stack

# A single integration test
make test-integration-specific TEST_NAME=TestCertManager/core_controllers

# Integration tests with debug logging
make test-integration-debug
```

Run targeted Go tests directly:

```bash
# All tests in a package
LOG_LEVEL=DEBUG go test -v ./...

# A specific test
LOG_LEVEL=DEBUG go test -v ./... -run TestSpecificTest

# Integration tests
LOG_LEVEL=DEBUG go test -v ./test/integration/...
```

Debug output includes image detection, registry mapping decisions, override generation steps, and file operations. Use it when an image is not detected or a mapping behaves unexpectedly.

### Logging Control in Tests

Tests control log output with the `LOG_LEVEL` environment variable (`DEBUG`, `INFO`, `WARN`, `ERROR`). The `--debug` flag is deprecated. Set `LOG_FORMAT=json` when test logic parses log output. See [logging.md](logging.md) for details.

Remember the output separation: diagnostic logs go to `stderr`, command results go to `stdout`. Capture both streams separately when testing debug behavior.

## Chart Testing Tooling

`test/tools/test-charts.py` validates `irr` across a wide range of Helm charts. It downloads charts, analyzes them, generates overrides, and reports detailed statistics.

### Dependencies

- Python 3.6 or later
- Helm 3.x
- Disk space for chart caching (about 50 MB per chart)

Install Python dependencies after cloning:

```bash
# If using pip:
pip install -e .

# If using uv:
uv sync
```

### Usage

```bash
# Test with default settings
./test/tools/test-charts.py registry.example.com

# Test with specific options
./test/tools/test-charts.py registry.example.com \
    --chart-filter "bitnami/*" \
    --max-charts 10 \
    --no-parallel
```

| Option | Description | Default |
|--------|-------------|---------|
| `target_registry` | Target registry URL (required) | None |
| `--no-parallel` | Disable parallel processing | False |
| `--chart-filter` | Only process charts matching pattern | None |
| `--max-charts` | Maximum number of charts to process | None |
| `--skip-charts` | Comma-separated list of charts to skip | None |
| `--no-cache` | Disable chart caching | False |

### Caching

Charts cache in `test/chart-cache/` to reduce downloads and rate-limit pressure. The first run downloads charts; later runs reuse them. Cache invalidation is manual (delete the directory). Use `--no-cache` to bypass.

### Rate Limit Protection

The script protects against registry rate limits with chart caching, conservative parallelism (4–8 workers depending on CPU count), QPS/burst limits on Helm commands, incremental retry backoff, and delays between repository operations.

### Error Categories

| Category | Description | Example |
|----------|-------------|---------|
| `RATE_LIMIT` | Rate limit exceeded | "Docker Hub rate limit exceeded" |
| `BITNAMI` | Bitnami-specific issues | "allowInsecureImages required" |
| `COMMAND_ERROR` | Invalid command syntax | "unknown flag: --chart" |
| `UNKNOWN` | Uncategorized errors | Various other errors |

### Why the Script Uses `default-values.yaml`

The script passes `-f default-values.yaml` (`test/tools/lib/default-values.yaml`) to every chart:

1. **Registry mirror** — it forces common registry keys (`global.imageRegistry`, `image.registry`, Bitnami `registry.server`) to the test target registry, so charts do not pull from public registries.
2. **Bitnami `allowInsecureImages`** — Bitnami charts reject mirrored images unless `global.security.allowInsecureImages: true` is set explicitly.
3. **Successful templating** — minimal safe defaults (for example `storageClass: ""`) let `helm template` render charts that would otherwise fail on missing required values. The generated manifests need not be deployable; the goal is rendering for image analysis.
4. **Consistency** — one baseline configuration applies to all charts.

### Troubleshooting

- **Rate limit errors** — wait for the reset, or run with `--no-parallel`. Keep caching enabled.
- **Command syntax errors** — verify the command syntax in the script; check for recent `irr` CLI changes.
- **Cache issues** — delete `test/chart-cache/` and retry; check disk space and permissions.

Results go to `test/results.txt` (summary), `test/charts/` (per-chart outputs), and `test/overrides/` (generated override files).

## Testing Complex Charts

Complex charts such as `cert-manager` and `kube-prometheus-stack` need special handling.

### Kubernetes Version Compatibility

Many charts require a specific Kubernetes version. Set these parameters during validation:

```bash
--set kubeVersion=1.29.0
--set Capabilities.KubeVersion.Major=1
--set Capabilities.KubeVersion.Minor=29
--set Capabilities.KubeVersion.GitVersion=v1.29.0
```

`kubeVersion` is a chart parameter; the `Capabilities.KubeVersion.*` parameters inject values directly into Helm's `.Capabilities.KubeVersion` object. Charts use this object in template conditionals, so direct injection is the reliable fix. The `test-charts.py` script adds these settings automatically; add them manually when validating a chart by hand.

Fallback mechanisms in the framework:

1. **Multiple version attempts** — on a version error, the script retries with 1.29.0, 1.28.0, 1.27.0, and so on until one works.
2. **Targeted chart handling** — charts with specific requirements (sonarqube, `eck-*`, traefik) get custom settings, up to v1.30.0.
3. **Required version extraction** — the framework parses the required version from error messages and tries that version first.

For stubborn charts, use Helm directly:

```bash
helm template release-name chart-path --kube-version v1.30.0 --values values-file.yaml
```

Note: `kubeVersion` is a validation-only parameter. It must NOT appear in the final override file used for deployment. See [rules.md](rules.md) for the parameter classification.

### Component-Group Testing

Complex charts break down into logical component groups:

- **Component groups** — related components with one success threshold and criticality level. For cert-manager, `core_controllers` (controller + webhook) is critical at 100%; `support_services` (cainjector, startupapicheck) is non-critical at 95%.
- **Table-driven subtests** — `t.Run()` subtests inside a parent test; select subsets with the `-run` flag.
- **Threshold-based validation** — critical components require 100% success; supporting components can use a lower threshold.
- **Contextual error reporting** — errors carry the component-group name.

```go
{
    name:           "core_controllers",
    components:     []string{"controller", "webhook"},
    threshold:      100,
    expectedImages: 2,
    isCritical:     true,
},
{
    name:           "support_services",
    components:     []string{"cainjector", "startupapicheck"},
    threshold:      95,
    expectedImages: 2,
    isCritical:     false,
},
```

Run component tests:

```bash
# All cert-manager tests
go test -v ./test/integration/... -run TestCertManager

# Only the core controllers group
go test -v ./test/integration/... -run TestCertManager/core_controllers

# A group with debug logging
LOG_LEVEL=DEBUG go test -v ./test/integration/... -run TestCertManager/support_services
```

### Guidelines for New Component Groups

1. Group components by functional relationship; keep groups at 2–4 components.
2. Set thresholds by criticality; document the expected image count.
3. Include the group name in error messages. Use `t.Logf` for warnings and `t.Errorf` for critical errors.
4. Define clear success and failure criteria; summarize failures in test output.

## Chart Testing Targets

The `test-data/charts/` directory mirrors the priority chart set used for validation:

1. **Nginx-Ingress** (infrastructure, medium) — multiple container types, init containers
2. **Cert-Manager** (security, medium) — CRDs, webhook containers
3. **Prometheus** (monitoring, high) — multiple components, extensive configuration
4. **Grafana** (monitoring, medium) — plugins, datasource configurations
5. **Redis** (database, low-medium) — clustering, metrics exporter
6. **MySQL** (database, medium) — primary-replica setup, backup containers
7. **Argo CD** (CI/CD, high) — multiple services, RBAC, Redis dependency
8. **Istio** (service mesh, very high) — multiple charts, complex dependencies
9. **Harbor** (registry, high) — multiple components, database dependencies
10. **Kube-Prometheus-Stack** (monitoring, very high) — multiple charts, extensive CRDs

Extended categories include infrastructure and networking (Traefik, External-DNS, Consul, etcd), observability (Loki, Fluentd, Elasticsearch, Kibana, kube-state-metrics), databases (PostgreSQL, MongoDB, Cassandra, MinIO), CI/CD (Jenkins, Flux, GitLab), applications (WordPress, Kafka, RabbitMQ, Keycloak, Airflow, Jupyterhub), security (Anchore, Falco, OPA, Vault, Gatekeeper), and platform services (Knative, Spark, Zookeeper, ChartMuseum, Helmfile).

Test charts in order of complexity: simple charts first (Redis, MySQL), then medium (Nginx-Ingress, Cert-Manager), then high (Prometheus, Argo CD), then very high (Istio, Kube-Prometheus-Stack).

For each chart, the success criteria are: all image references identified, subchart dependencies handled, generated overrides preserve chart functionality, and no unintended changes to non-image values.

## Related Documentation

- [rules.md](rules.md) — parameter classification and validation rules
- [CLI reference](../user/cli-reference.md) — command-line interface details
- [logging.md](logging.md) — log levels, formats, and debug control
- [filesystem-mocking.md](filesystem-mocking.md) — mocking the filesystem in tests
