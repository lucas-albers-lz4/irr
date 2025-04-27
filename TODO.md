# TODO.md - Helm Image Override Implementation Plan

# Usability Improvement , CLI Refactor
## Implementation Sequence Notes
- Phase 0 (Configuration) should be completed before Phase 3.5 registry handling
- Phase 2 (Flag Consistency) should be completed before Phase 3.5 file naming standardization
- Phase 3 (Output Behavior) should be completed before implementing strict mode in Phase 3.5

## Phase 0: Configuration Setup (P0: Critical Usability) [COMPLETED]

## Phase 1: Flag Cleanup (P0: User Experience Enhancements) [COMPLETED]

## Phase 2: Flag Consistency and Defaults (P0: User Experience Enhancements) [COMPLETED]

## Phase 3: Output File Behavior Standardization (P0: User Experience Enhancements) [COMPLETED]

## Phase 3.5: Streamlined Workflow Implementation (P1: User Experience Enhancements) [COMPLETED]

## Testing Strategy for CLI Enhancements (Completed Phases Summarized) [COMPLETED]

## Phase 4: Documentation Updates (P0: User Experience Enhancements) [COMPLETED]

### Implementation Notes [COMPLETED]
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

## Phase 5: Remove Legacy Registry Format (P1: Technical Debt Reduction) [COMPLETED]
- Successfully removed support for the legacy key-value registry format (`map[string]string`) and standardized on the structured format (`registry.Config`).

## Phase 6: Remove IRR_DEBUG Support [COMPLETED]
- Eliminated the redundant legacy `IRR_DEBUG` environment variable. `LOG_LEVEL` is now the sole control for log verbosity.

## Phase 7: Image Pattern Detection Improvements (Revised Focus)

### Overview
Improve the analyzer's ability to detect and process image references in complex Helm charts, particularly focusing on init containers, admission webhooks, and other specialized configurations.

### Motivation
- Address missed image references in complex charts (e.g., admission webhooks).
- Improve reliability via consistent detection and debug tooling.

### Implementation Steps

#### Phase 7.1: Debugging and Analysis [COMPLETED]

#### Phase 7.2: Image Pattern Detection Improvements [COMPLETED]

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
- Ensured robust Bitnami chart detection and correct application of Bitnami-specific rules.

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
- Introduced `--no-validate` flag to `irr override`.

## Phase 12: Enhance Override Robustness for Live Release Value Errors [COMPLETED]
- Improved handling of problematic strings in live release values, including fallback to default values with a warning.

## Phase 13: Code Documentation Review and Alignment [COMPLETED]
- Reviewed and aligned Go code documentation with current functionality.

## Phase 14: Add --all-namespaces (-A) Flag to `inspect` [COMPLETED]
- Added `-A`/`--all-namespaces` and `--overwrite-skeleton` flags to `irr inspect`.
- Enabled cluster-wide image inspection and skeleton generation.

### Phase 14.6: Implementation Refinements
- [✓] **[COMPLETED]** **Output Structure:** Ensure standard `inspect -A` output (YAML/JSON) is clearly grouped by namespace/release. Document this structure with an example.
- [✓] **[COMPLETED]** **Partial Failures:** Handle errors inspecting individual releases gracefully (log error, skip release, continue processing others). Summarize skipped releases at the end.
    - *Implementation Detail:* Log errors using `log.Warnf` or similar, including release context. Consider exiting 0 if *any* releases succeed, even with partial failures.
- [✓] **[COMPLETED]** **CLI Help:** Review and refine the help text for `-A` and related flags for maximum clarity.
- [✓] **[COMPLETED]** **Testing Edge Cases:** Ensure test data includes releases with no images, only private/excluded registries, and malformed values.
- [✓] **[COMPLETED]** **Documentation Example:** Include a sample of the generated skeleton file (with comments) in the documentation.

### Phase 14.7: Fix Skeleton Generation for --all-namespaces [BUG]

**Goal:** Ensure `inspect -A --generate-config-skeleton` produces a complete skeleton file including *all* unique registries found across *all* analyzed releases.

**Problem:**
- Current implementation produces an incomplete skeleton file when using `-A`.
- Registries identified during individual release analysis (e.g., `registry.k8s.io`, `harbor.home.arpa`, `gcr.io`, `ghcr.io` in the test case) are missing from the final aggregated skeleton file.

**Implementation Steps:**
- [ ] **[P0]** **Investigate Aggregation Logic:** Review the `processAllReleases` function and potentially the `createConfigSkeleton` function to identify where unique registries are being lost or incorrectly aggregated when the `-A` flag is used.
- [ ] **[P0]** **Correct Aggregation:** Modify the logic to ensure that the set of registries used for skeleton generation accurately reflects all unique registries found across *all* successfully analyzed releases in the `-A` workflow.
- [ ] **[P0]** **Update/Add Unit Test:** Enhance `TestRunInspect` or add a new specific test case for `inspect -A --generate-config-skeleton` that:
    - Mocks multiple releases with a diverse set of unique registries (similar to the identified bug scenario).
    - Verifies that the generated skeleton file content includes *all* expected unique registries.
- [ ] **[P0]** **Manual Verification:** Re-run the command sequences that revealed the bug (`helm list ... | bash -x` vs `inspect -A --generate...`) to confirm the fix.

