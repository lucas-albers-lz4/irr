# TODO.md - Helm Image Override Implementation Plan

# Usability Improvement , CLI Refactor
## Implementation Sequence Notes
- Phase 0 (Configuration) should be completed before Phase 3.5 registry handling
- Phase 2 (Flag Consistency) should be completed before Phase 3.5 file naming standardization
- Phase 3 (Output Behavior) should be completed before implementing strict mode in Phase 3.5

## Phase 0: Configuration Setup (P0: Critical Usability) [COMPLETED]
- [x] **[COMPLETED]** Implement `helm irr config` command and analyze error code usage.

## Phase 1: Flag Cleanup (P0: User Experience Enhancements) [COMPLETED]
- [x] **[COMPLETED]** Remove unused/confusing flags and verify cleanup.

## Phase 2: Flag Consistency and Defaults (P0: User Experience Enhancements) [COMPLETED]
- [x] **[COMPLETED]** Unify `--chart-path`/`--release-name`, implement mode-specific flags, standardize `--namespace`, make `--source-registries` optional.

## Phase 3: Output File Behavior Standardization (P0: User Experience Enhancements) [COMPLETED]
- [x] **[COMPLETED]** Standardize `--output-file` behavior and file operation error handling.

## Phase 3.5: Streamlined Workflow Implementation (P1: User Experience Enhancements) [COMPLETED]
- [x] **[COMPLETED]** Enhance registry handling, standardize override naming, handle unrecognized registries, integrate validation, improve K8s version handling, and implement consistent error codes.

## Testing Strategy for CLI Enhancements (Completed Phases Summarized) [COMPLETED]
- [x] **[COMPLETED]** Tests for Phases 0-3.5 (config, registry detection, file naming, strict mode, error handling) completed.

## Phase 4: Documentation Updates (P0: User Experience Enhancements) [COMPLETED]
- [x] **[COMPLETED]** Update documentation for all CLI changes (flags, defaults, behavior, output).

### Implementation Notes
- We've restructured the chart loading mechanism to properly use the chart.NewLoader() function, which improves code organization and maintainability
- We've fixed the strategy flag handling to keep it available but with sensible defaults, making the interface cleaner without removing functionality
- The integration tests now pass consistently after fixing implementation issues with chart sources and required flags

## REMINDER On the Implementation Process: (DONT REMOVE THIS SECTION)
- For each change:
  1. **Baseline Verification:**
     - Run full test suite: `go test ./...` ✓
     - Run full linting: `golangci-lint run` ✓
     - Determine if any existing failures need to be fixed before proceeding with new feature work ✓

  2. **Pre-Change Verification:**
     - Run targeted tests relevant to the component being modified ✓
     - Run targeted linting to identify specific issues (e.g., `golangci-lint run --enable-only=unused` for unused variables) ✓

  3. **Make Required Changes:**
     - Follow KISS and YAGNI principles ✓
     - Maintain consistent code style ✓
     - Document changes in code comments where appropriate ✓
     - **For filesystem mocking changes:**
       - Implement changes package by package following the guidelines in `docs/TESTING-FILESYSTEM-MOCKING.md`
       - Start with simpler packages before tackling complex ones
       - Always provide test helpers for swapping the filesystem implementation
       - Run tests frequently to catch issues early

  4. **Post-Change Verification:**
     - Run targeted tests to verify the changes work as expected ✓
     - Run targeted linting to confirm specific issues are resolved ✓
     - Run full test suite: `go test ./...` ✓
     - Run full linting: `golangci-lint run` ✓
     - **CRITICAL:** After filesystem mocking changes, verify all tests still pass with both the real and mock filesystem

  5. **Git Commit:**
     - Stop after completing a logical portion of a feature to make well reasoned git commits with changes and comments ✓
     - Request suggested git commands for committing the changes ✓
     - Review and execute the git commit commands yourself, never change git branches stay in the branch you are in until feature completion ✓

  6. **Building and Tesing Hints**
     - `make build` builds product, `make update-plugin` updates the plugin copy so we test that build
       `make test-filter` runs the test but filters the output, if this fails you can run the normal test to get more detail

     - default behavior: fail if file exists (per section 3.4 in PLUGIN-SPECIFIC.md)
     - Use file permissions constants for necessaary permission set, those are defined in this file : `pkg/fileutil/constants.go`

##END REMINDER On the Implementation Process:

## Phase 5: Registry Format Standardization (P1: Technical Debt Reduction)

### Overview
Fully standardize on the structured registry format throughout the codebase, deprecating the legacy key-value format while maintaining backward compatibility for existing users.

### Motivation
- Structured format provides better metadata, organization, extensibility, and maintainability.

### Implementation Steps

#### Phase 5.1: Registry Package Updates
- [/] **[P1]** Update registry package core functionality
  - [ ] Remove or deprecate legacy-specific functions in `pkg/registry/mappings.go`
  - [x] Update documentation to clarify the structured format is preferred
  - [x] Ensure `ConvertToLegacyFormat()` is maintained for backward compatibility
  - [ ] Add clear deprecation notices to legacy-format functions
  - [ ] Update function signatures that take map[string]string to prefer *Config parameter
  - [ ] Consider adding helpers to detect format from raw file content

#### Phase 5.2: CLI Command Updates
- [ ] **[P1]** Update CLI commands for structured format
  - [x] Review and update the `inspect` command skeleton generation
  - [/] Update the `config` command to write structured format (preserves loaded structured, creates new otherwise)
  - [x] Update the `override` command to handle structured format (via LoadMappings)
  - [/] Update help text and examples to show structured format
  - [/] Ensure `--registry-file` flag documentation mentions structured format

#### Phase 5.3: Test Updates
- [ ] **[P1]** Update test suite for structured format
  - [x] Modify `TestRegistryMappingFile` in `test/integration/integration_test.go`
  - [ ] Update TestConfigFileMappings (uses legacy input) and similar tests
  - [x] Update `CreateRegistryMappingsFile()` in `test/integration/harness.go` to default to structured format
  - [x] Add tests for handling of legacy format files (conversion path)
  - [ ] Verify all existing tests pass with the updated format

#### Phase 5.4: Documentation Updates
- [ ] **[P1]** Update user-facing documentation
  - [ ] Update CLI reference documentation with structured format examples
  - [ ] Add migration guide for users with existing config files
  - [ ] Update any tutorials or examples to use structured format
  - [ ] Document the backward compatibility mechanism

### Files Requiring Changes
(List remains relevant for future work)

### Acceptance Criteria
(Summarized)
- Structured format is default, legacy format readable, tests pass, docs updated.

### Testing Strategy
(Summarized)
- Test legacy conversion, structured writing, invalid files, compatibility, CLI output.

## Phase 6: Remove IRR_DEBUG Support [COMPLETED]
- [x] **[COMPLETED]** Eliminate the redundant legacy `IRR_DEBUG` environment variable. `LOG_LEVEL` is now the sole control for log verbosity.

## Phase 7: Image Pattern Detection Improvements (Revised Focus)

### Overview
Improve the analyzer's ability to detect and process image references in complex Helm charts, particularly focusing on init containers, admission webhooks, and other specialized configurations.

### Motivation
- Address missed image references in complex charts (e.g., admission webhooks).
- Improve reliability via consistent detection and debug tooling.

### Implementation Steps

#### Phase 7.1: Debugging and Analysis [COMPLETED]
- [x] **[COMPLETED]** Add debugging output to trace image detection.

#### Phase 7.2: Image Pattern Detection Improvements [COMPLETED]
- [x] **[COMPLETED]** Fix ingress-nginx admission webhook image detection.

#### Phase 7.3: Additional Chart Coverage
- [x] **[P1]** Expand test coverage to more complex charts
  - [ ] Review and enable previously skipped test cases
  - [x] Add tests for charts with init containers
  - [x] Add tests for charts with sidecars and admission webhooks
  - [x] Add tests for edge cases including unusual nesting levels and camel-cased fields
  - [x] Add proper handling for template-string image references
  - [ ] Fix simplified-prometheus-stack test case

#### Phase 7.4: Kube-State-Metrics Handling Fix (Generator-Level) **[NEXT P0]**
- [x] **[P0]** Fix linter errors in `pkg/generator/generator.go` related to `detector.DetectImages` call.
- [/] **[P0]** Validate `normalizeKubeStateMetricsOverrides` function in `pkg/generator/kube_state_metrics.go` for correct identification and structure (implementation review complete, pending test validation).
- [ ] **[P0]** Refine `TestKubePrometheusStack` for `kube-state-metrics` assertions without workarounds.

## Phase 7.5: Debug Environment Test Validation
- [ ] **[P1]** Run the full test suite with `LOG_LEVEL=DEBUG` enabled *after* Phase 7.4 is complete and verified.
- [ ] **[P1]** Compare pass rates and fix any tests failing only in debug mode.

## Phase 8: Fix Bitnami Chart Detection and Rules Processing [COMPLETED]
- [x] **[COMPLETED]** Ensure robust Bitnami chart detection for real-world variations and correct application of Bitnami-specific rules.

## Phase 9: Implement Subchart Discrepancy Warning (User Feedback Stop-Gap)

### Overview
Warn users in `inspect` when `irr`'s image count differs from `helm template`'s (limited parse), indicating potential missed images from subchart defaults.

### Motivation
- Analyzer currently misses images defined *only* in subchart `values.yaml`.
- Provide interim feedback as full subchart value computation (Phase 10) is complex and deferred.

### Implementation Steps (Phase 9.1)
- [ ] **[P1]** Integrate Helm SDK Template Execution (`helm template`) within `inspect`.
- [ ] **[P1]** Parse Rendered Manifests (Limited Scope: Deployments/StatefulSets only) to extract image strings.
- [ ] **[P1]** Compare `irr` analyzer count vs. rendered template count.
- [ ] **[P1]** Issue Warning on Mismatch (explaining cause, limitations, flag).
- [ ] **[P1]** Add Control Flag (`--warn-subchart-discrepancy`, default true).
- [ ] **[P1]** Add Tests covering flag and warning logic.
- [ ] **[P1]** Update Documentation explaining the warning and limitation.

### Acceptance Criteria (Phase 9)
- Optional warning mechanism exists and works. Tests pass. Docs updated.

## Phase 10: Refactor Analyzer for Full Subchart Support (Deferred - Complex)

### Overview
Enhance the analyzer to correctly process subcharts by replicating Helm's value computation logic (loading dependencies, merging values).

### Motivation
- Provide truly accurate results for umbrella charts.
- Eliminate the need for the Phase 9 warning.
- Address core analyzer limitation.
- **Note:** High complexity, requires careful design regarding value origins for override generation.

### Implementation Steps
- [ ] **[P2]** Research & Design Helm Value Computation replication.
- [ ] **[P2]** Refactor Analyzer Input to accept computed/merged values + origin info.
- [ ] **[P2]** Adapt Analyzer Traversal & Source Path Logic for subchart context.
- [ ] **[P2]** Update Command Usage (`inspect`, `override`) to use new logic.
- [ ] **[P2]** Add Comprehensive Tests for umbrella charts and subchart overrides.
- [ ] **[P2]** Update Documentation removing subchart limitations.
- [ ] **[P3]** Review/Remove Warning Mechanism (from Phase 9) if obsolete.

### Acceptance Criteria (Phase 10)
- `inspect`/`override` handle subchart values/paths correctly. Tests pass. Docs updated. Warning (Phase 9) may be removed.

### Recommendations for Future Approach (Learnings from `feature/sub-charts`)
(Keep recommendations as they guide future work)

## Phase 11: Decouple Override Validation [COMPLETED]
- [x] **[COMPLETED]** Introduce `--no-validate` flag to `irr override` to skip internal Helm template validation, improving testability for charts requiring extra values.

## Phase 12: Enhance Override Robustness for Live Release Value Errors

**Goal:** Improve `helm irr override <release_name>` handling of specific live value analysis errors (problematic strings).

**Completed Steps:**
- [x] **[P0]** Detect and report errors when live value analysis encounters problematic strings (e.g., parsing non-image strings like args).
- [x] **[P0]** Log the problematic path/value causing the failure.
- [x] **[P0]** Include a recommendation in the error message to use `--exclude-pattern`.

**Remaining Steps (Fallback Mechanism):**
- [x] **[P1]** **Attempt Default Analysis:** If the *specific* problematic string error occurs, locate the chart in cache and analyze its *default* `values.yaml`.
- [x] **[P1]** **Generate Partial Overrides:** If default analysis succeeds, generate overrides based *only* on default values.
- [x] **[P1]** **Issue Prominent Warning:** If fallback occurs, clearly warn user that overrides are based on defaults and may be incomplete due to errors in live values.
- [x] **[P1]** **Handle Default Analysis Failure:** If fallback analysis *also* fails, report that error.

**Rationale:**
- Prioritizes accuracy (live analysis), increases robustness (handles common string errors), provides partial results instead of failure, guides user (`--exclude-pattern`).

**Investigation Notes (TestOverrideRelease_Fallback Failure):**
*   **Fallback Trigger Not Firing?**
    *   Detector change: Image detector might no longer classify the test's problematic string (`not-an-image-but-looks-like.one:v1`) as an `UnsupportedImage` or generate the specific trigger error (`ErrAnalysisFailedDueToProblematicStrings`).
    *   Handler change: `handleUnsupportedMatches` might no longer return the specific trigger error even if it receives unsupported matches.
*   **Fallback Triggered but Fails Internally?**
    *   Chart path issue: Mock `FindChartForRelease` returning wrong path, or `loader.Load` failing on the mock filesystem path/files.
    *   Default value analysis failure: Second run of detector on default values might be failing (issue in mock `values.yaml` or detector sensitivity).
*   **Incorrect Mock Interaction?**
    *   Is the correct `MockHelmClient` (with problematic live values and fallback path) being injected via `helmAdapterFactory`?
*   **Assertion Mismatches?**
    *   YAML formatting differences causing `assert.YAMLEq` to fail.
    *   Warning message wording/format in `stderr` might have drifted, failing `assert.Contains`.

**Follow-up Investigation (2023-07-27):**
*   **Skipped Test:** `TestOverrideRelease_Fallback` is temporarily skipped until investigation is complete.
*   **Real-world Example Validation:** The fallback mechanism was confirmed to work in real-world scenarios with `cert-manager` when using `--no-validate`.
*   **Root Issue Analysis:**
    *   The live values in cert-manager contain CLI arguments (`--dns01-recursive-nameservers=192.168.1.2:53`) that get incorrectly identified as image references.
    *   Fallback correctly generates overrides based on default chart values, but validation fails because it tries to template the chart with both overrides and original problematic values.
    *   Using `--no-validate` bypasses this issue, which is the correct behavior (test already includes this flag).
*   **Next Steps:**
    *   Investigate why test fails despite `--no-validate` flag being set.
    *   Verify differences between test mock objects and real-world objects (esp. chart loading and detection).
    *   Fix the test to accurately simulate real-world behavior or adjust expectations.

Phase 13.

## Phase 13: Code Documentation Review and Alignment

**Goal:** Ensure all Go code documentation (godoc, comments) accurately reflects current functionality, removing or correcting outdated information.

**Process:** Review files package by package or by logical area. For each checklist item (which may cover one or two files, e.g., `foo.go` and `foo_test.go`):
1. Read code and documentation.
2. Compare documentation to functionality.
3. Identify and correct/remove discrepancies.
4. **After completing the review for an item, run `make lint` and `make test-quiet` to ensure no issues were introduced.**

### Phase 13.1: `cmd/irr` Package (CLI Commands)
- [x] Review `cmd/irr/main.go` (and `main_test.go`)
- [x] Review `cmd/irr/root.go` (and `root_test.go`)
- [x] Review `cmd/irr/config.go` (and `config_test.go`)
- [x] Review `cmd/irr/inspect.go` (and `inspect_test.go`)
- [x] Review `cmd/irr/override.go` (and `override_test.go`)
- [x] Review `cmd/irr/validate.go` (and `validate_test.go`)
- [x] Review `cmd/irr/helm.go` (and `helm_test.go`)
- [x] Review `cmd/irr/fileutil.go` (and `fileutil_test.go`)
- [x] Review `cmd/irr/constants.go`
- [x] Review `cmd/irr/kube_version_test.go`
- [x] Review `cmd/irr/test_helpers_test.go`
- [x] Review `cmd/irr/cli_test_helpers.go`
- [x] Review `cmd/irr/cli_test.go`

### Phase 13.2: `pkg/analysis` Package
- [x] Review `pkg/analysis/analyzer.go` (and `analyzer_test.go`)
- [x] Review `pkg/analysis/types.go`

### Phase 13.3: `pkg/analyzer` Package
- [x] Review `pkg/analyzer/analyzer.go` (and `analyzer_test.go`)

### Phase 13.4: `pkg/chart` Package
- [x] Review `pkg/chart/api.go` (and `api_test.go`)
- [x] Review `pkg/chart/loader.go` (and `loader_test.go`)
- [x] Review `pkg/chart/generator.go` (and `generator_test.go`, `generator_loader_test.go`)
- [x] Review `pkg/chart/chartinfo.go`
- [x] Review `pkg/chart/overrides_yaml_test.go`
- [x] Review `pkg/chart/rules_integration_test.go`
- [x] Review `pkg/chart/validate_template_test.go`
- [x] Review `pkg/chart/errors.go` (and `errors_test.go`)

### Phase 13.5: `pkg/exitcodes` Package
- [x] Review `pkg/exitcodes/exitcodes.go` (and `exitcodes_test.go`)

### Phase 13.6: `pkg/fileutil` Package
- [x] Review `pkg/fileutil/fs.go` (and `fs_test.go`)
- [x] Review `pkg/fileutil/utils.go` (and `utils_test.go`)
- [x] Review `pkg/fileutil/constants.go`
- [x] Review `pkg/fileutil/fileutil_test.go`

### Phase 13.7: `pkg/generator` Package
- [x] Review `pkg/generator/generator.go` (and `generator_test.go`)
- [x] Review `pkg/generator/kube_state_metrics.go` (if exists) - N/A, logic in generator.go

### Phase 13.8: `pkg/helm` Package (Note: distinct from `internal/helm`)
- [x] Review `pkg/helm/client.go` (and `client_test.go`)
- [x] Review `pkg/helm/repo.go` (and `repo_test.go`)
- [x] Review `pkg/helm/sdk.go` (and `sdk_test.go`)
- [x] Review `pkg/helm/version.go` (and `version_test.go`)

### Phase 13.9: `pkg/image` Package
- [x] Review `pkg/image/detector.go`
- [x] Review `pkg/image/detection.go` (and `detection_test.go`)
- [x] Review `pkg/image/parser.go` (and `parser_test.go`, `parser_edge_test.go`)
- [x] Review `pkg/image/validation.go` (and `validation_test.go`)
- [x] Review `pkg/image/normalization.go`

### Phase 13.10: `pkg/log` Package