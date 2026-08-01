# Changelog

All notable changes to **irr** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

There is no v0.0.13 release; versioning moved from v0.0.12 directly to v0.0.14.

---

## [Unreleased]

### Added
- Documentation overhaul: fwlive-style user/developer/archive tree, `CHANGELOG.md`, `CONTRIBUTING.md`, `docs/FAQ.md`, and `tools/check-links.py` (#23, #24)
- Dependency-review workflow for pull requests (#22)
- Dependabot coverage for npm and GitHub Actions ecosystems (#21)

### Changed
- Restructure and merge overlapping docs; archive legacy planning docs; replace committed `registry-mappings.yaml` junk with a clean template (#23, #24)
- Coverage gate floor set to 73% (target remains 75%) to acknowledge pre-existing `pkg/chart` shortfall revealed when package paths were corrected (#24)
- Split monolith files into focused modules: `pkg/chart/generator.go`, `cmd/irr/override.go`, `cmd/irr/inspect.go` (#17, #18, #19)
- Consolidate duplicate analysis packages into `pkg/analysis`
- Migrate printf-style log calls to structured key-value format
- Bump golangci-lint for Go 1.26 and patch transitive CVEs
- Bump Go to 1.26.4

### Fixed
- Stale pre-migration org references in docs, Codecov paths, golangci local-prefixes, and lint tooling (#23, #24)
- Broken manual Helm plugin install URL (correct `helm-irr-<version>-<os>-<arch>.tar.gz` asset pattern) (#23, #24)
- Add explicit workflow permissions for CodeQL alerts
- Separate `chartPath` from `kubeVersion` in `handleHelmPluginValidate`
- Remove dead `initConfig()` function
- Dependabot and grpc transitive dependency alerts

## [v0.0.18] — 2025-10-20

### Changed
- Update Helm dependency to v3.18.4 (Dependabot security fix) (#11)
- Use context-aware logger and exec calls throughout

### Fixed
- Post-release install test for all runners (#12)
- Plugin install works with curl 8.0 (single-URL calls only) (#12)

## [v0.0.17] — 2025-10-20

### Changed
- Install script compatible with curl 8.0 (one URL per call)
- Drop wget support; curl only

## [v0.0.16] — 2025-05-21

### Changed
- Use the chart's `appVersion` as the default image tag instead of `latest` (#8)
- Strip ports from registry URLs

### Fixed
- Tag and registry validation in image comparison tests
- Image count parity between `irr inspect` and `helm template`

## [v0.0.15] — 2025-05-19

### Added
- Derive source registries from the mapping file when `--source-registries` is not provided (#7)

### Changed
- Default to the registry file for overrides; legacy simple-map format no longer supported (#7)

### Fixed
- Empty mapping files handled gracefully
- Original registry domain preserved in repository paths

## [v0.0.14] — 2025-05-11

### Added
- `--output-format yaml` support for `irr override` in plugin mode
- CNCF chart test listing (moved to `docs/cncf.txt`)

### Fixed
- Nil-checks for potential nil panics (nilaway findings)
- Lint errors and namespace handling in `helm override`

## [v0.0.12] — 2025-05-03

### Added
- Context-aware analyzer flag for `inspect` and `override`
- Deprecation notice for the legacy (non-context-aware) generator

### Changed
- Improved context-aware image detection and validation logic
- Path strategy handles registry path splitting and normalization
- `irr override` always sets `global.imageRegistry` in generated overrides

### Fixed
- Empty tag after colon rejected by the image parser
- Invalid hostnames filtered from skeleton generation
- Subchart dependency value analysis in the analyzer

## [v0.0.11] — 2025-05-01

### Fixed
- BADKEY parsing output when running `inspect`
- Potential nil panics (nilaway audit) with explicit nil checks
- Integration test execution for subchart tests

## [v0.0.10] — 2025-04-30

### Fixed
- Basic `inspect` namespace bug in plugin mode (defaulted to `default` incorrectly)
- Test more Helm versions in CI

## [v0.0.9] — 2025-04-29

### Added
- Context-aware chart loading and analysis
- Accurate subchart value origin tracking (#5)

### Changed
- Consolidated registry-mappings documentation into one section
- Removed reference to a delete function that does not exist

### Fixed
- Lint errors across registry, override, chart, rules, and test harness
- Broken TESTING.md link

## [v0.0.8] — 2025-04-27

### Added
- Release process documentation
- Makefile check that plugin.yaml and release workflow versions match
- `-A` flag for `inspect` across all namespaces, with tests

### Changed
- Analyzer keeps the original unfiltered image list before `--source-registries` filtering
- Refactored analyzer and generator structure; cleaned up comments

### Fixed
- Helm namespace scoping for get/list actions
- Integration build tag on integration test files

## [v0.0.7] — 2025-04-25

### Changed
- Removed deprecated `--strategy` flag from `override`
- Consolidated docs; README notes `--no-validate`
- Cleaned up unused code and comments across `pkg`, `cmd/irr`, and Helm packages

### Fixed
- Stat error handling and `EnsureDirExists` test assertions
- 41 lint errors

## [v0.0.6] — 2025-04-23

### Added
- Test cases for log-level precedence and CLI default values
- Default value hints in flag descriptions

### Changed
- Revised `override` to use the `afero` filesystem abstraction
- Removed completed implementation-plan sections after audit
- Excluded integration output files from the repo

### Fixed
- Plugin install error: unknown `helmVersion` field in plugin.yaml
- Lint errors

## [v0.0.5] — 2025-04-15

### Fixed
- Install script permissions and execution
- Dependabot bump for `containerd` (#5)

## [v0.0.4] — 2025-04-15

### Fixed
- Added `+x` permission bit to `install-binary.sh`

## [v0.0.3] — 2025-04-15

### Changed
- Version check works with the `helm.version` definition in plugin.yaml
- Removed `helm.version` config from plugin.yaml

### Fixed
- Plugin install error: JSON `helmVersion` field unknown

## [v0.0.2] — 2025-04-15

### Added
- Initial release

---

[v0.0.18]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.17...v0.0.18
[v0.0.17]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.16...v0.0.17
[v0.0.16]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.15...v0.0.16
[v0.0.15]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.14...v0.0.15
[v0.0.14]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.12...v0.0.14
[v0.0.12]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.11...v0.0.12
[v0.0.11]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.10...v0.0.11
[v0.0.10]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.9...v0.0.10
[v0.0.9]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.8...v0.0.9
[v0.0.8]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.7...v0.0.8
[v0.0.7]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.6...v0.0.7
[v0.0.6]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.5...v0.0.6
[v0.0.5]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.4...v0.0.5
[v0.0.4]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.3...v0.0.4
[v0.0.3]: https://github.com/lucas-albers-lz4/irr/compare/v0.0.2...v0.0.3
[v0.0.2]: https://github.com/lucas-albers-lz4/irr/releases/tag/v0.0.2
