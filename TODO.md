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
     - **NEW REMINDER:** Run `make lint` and `make test` frequently after making logical changes or fixing previous issues. Don't wait until the end of a feature. Address failures promptly.

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
## Phase 8: Fix Bitnami Chart Detection and Rules Processing [COMPLETED]

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


## Phase 11: Decouple Override Validation [COMPLETED]
- Introduced `--no-validate` flag to `irr override`.

## Phase 12: Enhance Override Robustness for Live Release Value Errors [COMPLETED]
- Improved handling of problematic strings in live release values, including fallback to default values with a warning.

## Phase 13: Code Documentation Review and Alignment [COMPLETED]
- Reviewed and aligned Go code documentation with current functionality.

## Phase 14: Add --all-namespaces (-A) Flag to `inspect` [COMPLETED]
- Added `-A`/`--all-namespaces` and `--overwrite-skeleton` flags to `irr inspect`.
- Enabled cluster-wide image inspection and skeleton generation.

## Phase 9: Handle Subcharts (Analyzer Enhancement & Generator/Inspector Alignment)

### Overview
Enhance the analyzer and downstream components (generator, inspector) to correctly process Helm charts with subcharts, ensuring that image definitions from subchart default values are detected and handled correctly. This addresses limitations found with complex umbrella charts like kube-prometheus-stack.

### Motivation
- The original analyzer only processed top-level `values.yaml`.
- Images defined only in subchart `values.yaml` files (or other sources merged by Helm) were missed, leading to incomplete `inspect` results and `override` files.
- Users need accurate analysis and generation for complex charts to create reliable overrides.
- Integration tests revealed that while the analyzer refactor (Phase 9.3) correctly identified images, the generator and inspector were not correctly handling the resulting paths/structures.

### Implementation Steps

#### Phase 9.1: Implement Discrepancy Warning (User Feedback Stop-Gap) [OBSOLETE]
- This was a temporary measure before full subchart support.

#### Phase 9.2: Subchart Discrepancy Analysis [COMPLETED]
- **Goal:** Systematically analyze charts exhibiting discrepancies between `irr inspect` output and `helm template` rendering, categorize root causes at scale, and document findings to inform the Phase 9.3 refactor.
    - **Conclusion:** Analysis highlighted limitations in the old analyzer and confirmed the need for replicating Helm's value merging logic (Phase 9.3).

#### Phase 9.3: Refactor Analyzer for Full Subchart Support (The Correct Fix) [COMPLETED]
_Objective: Ensure the analyzer can fully replicate Helm's value merging, including subcharts, to enable accurate image path detection._

- [x]   **Phase 9.3.1 - 9.3.10:** Completed tasks related to prototyping Helm value merging, designing context structures, adapting analyzer traversal, implementing chart loading utilities, integrating into commands, identifying test cases, adding unit tests, and updating documentation regarding the *analyzer's* capabilities.
    - **Outcome:** The analyzer (`internal/helm/context_analyzer.go`) now correctly identifies images and their source paths (e.g., `child.image`) within merged value structures.

#### Phase 9.4: Align Generator and Inspector with Context-Aware Analysis [IN PROGRESS]
_Objective: Ensure the override generator and inspect command correctly process the paths and structures identified by the context-aware analyzer, especially for subchart values, resolving current integration test failures._

- [ ] **Phase 9.4.1: [P1] Debug & Fix Override Generator (`pkg/chart/generator.go`)** [IN PROGRESS]
    - [x] **Goal:** Resolve failures in `TestOverrideParentChart` and `TestCertManager`.
    - [ ] **Tasks:**
        - [ ] Analyze the `panic` and incorrect values in `TestOverrideParentChart` failures. -> **Still failing: tag mismatch (`latest` vs `1.23`) and missing `child.image.repository`.**
        - [ ] Debug the `createOverride` and `setOverridePath` functions in `pkg/chart/generator.go` to correctly handle nested paths derived from subchart analysis (e.g., `child.image`, `aliasedChild.monitoring.image`). -> **Ongoing debugging needed.**
            - **Next Step:** Add detailed logging within `createOverride` to trace how `imgRef` (especially `imgRef.Tag`) is determined and used for the parent image (`library/nginx`) when processing `TestOverrideParentChart`. Verify the `pattern.Structure` map is correctly accessed if needed.
            - **Next Step:** Add detailed logging within `setOverridePath` to trace how the `child.image.repository` path is processed. Verify intermediate maps are created correctly and the final value is set as expected.
        - [x] Ensure generated override YAML has the correct nested structure reflecting subchart aliases/names. -> **`TestCertManager` passes, indicating basic structure is okay, but parent/child values are wrong.**
- [ ] **Phase 9.4.2: [P1] Debug & Fix Inspect Command (`cmd/irr/inspect.go`)**
    - [ ] **Goal:** Resolve regression in `TestInspectParentChart`.
    - [ ] **Tasks:**
        - [ ] Determine why `inspect` fails with parent charts after recent analyzer changes.
- [ ] **Phase 9.4.3: [P1] Verify with Integration Tests & Add Coverage**
    - [ ] **Goal:** Confirm fixes work end-to-end and add tests for uncovered subchart scenarios.
    - [ ] **Tasks:**
        - [ ] Run `make build && go test -tags integration ./...` frequently after generator/inspector fixes.
        - [ ] Ensure existing failing tests (`TestOverrideParentChart`, `TestCertManager`, `TestInspectParentChart`) pass.
        - [ ] **Add New Test Case (Subchart Aliases):** Create/find a test chart with subchart dependencies defined using `alias`. Add an integration test that runs `irr override` on this chart and verifies the generated YAML uses the alias (e.g., `childAlias.image.repository`) not the original chart name in the override path.
        - [ ] **(Optional) Add New Test Case (Deep Nesting):** If a suitable chart exists (e.g., parent -> child -> grandchild), add a test case verifying override generation for images defined at deeper levels.
        - [ ] **(Optional) Add New Test Case (Globals Interaction):** Add a test case that verifies how global values (e.g., `global.imageRegistry`) interact with image overrides defined in subcharts.
- [ ] **Phase 9.4.4: [P2] Update Documentation (if necessary)**
    - [ ] **Goal:** Ensure docs accurately reflect subchart handling in both inspect and override.
    - [ ] **Tasks:** Review relevant documentation sections.

### Acceptance Criteria (Phase 9 - Revised)
- Analyzer unit tests (`internal/helm`) pass. [COMPLETED]
- Integration tests (`TestOverrideParentChart`, `TestCertManager`, `TestInspectParentChart`) pass, demonstrating correct override generation and inspection for charts with subcharts.
- New integration tests covering subchart aliases (and optionally deep nesting/globals) pass.
- Documentation accurately reflects subchart handling capabilities.

## Phase 10: Investigate and Address Helm Template Failures During Validation

### Overview
Address the ~4.4% chart failure rate (`ERROR_TEMPLATE_EXEC`, `ERROR_TEMPLATE_PARSE`) observed during large-scale testing (Phase 9.2). The goal is to understand why `helm template` fails for these charts when run with minimal configuration and identify the necessary (likely Type 2 - Test/Validation-Only) parameters required to allow successful templating for testing/validation purposes. This phase focuses on understanding and potentially improving the validation process, *not* on adding validation-specific parameters to the final `override.yaml`.

### Motivation
- Increase the number of charts whose analysis *accuracy* can be verified by IRR's test suite.
- Gain insights into common Helm chart validation requirements.
- Document necessary Type 2 parameters for future testing or user guidance.
- Identify any potential Type 1 (Deployment-Critical) parameters missed by current rules that might surface during this investigation.

### Implementation Steps

- [ ] **Phase 10.1: Identify Failing Charts**
    - [ ] **Goal:** Compile a definitive list of charts failing `helm template`.
    - [ ] **Tasks:**
        - [ ] Re-run `test/tools/test-charts.py` if necessary to get up-to-date results.
        - [ ] Parse the output log (`test/output/logs/test_results.csv` or similar) generated by `test-charts.py` / `subchart_results.py`.
        - [ ] Extract all chart names/versions marked with `ERROR_TEMPLATE_EXEC` or `ERROR_TEMPLATE_PARSE`.

- [ ] **Phase 10.2: Manual Investigation & Minimal Value Set Discovery (Sampling)**
    - [ ] **Goal:** Understand failure reasons and find minimal `--set` parameters for a sample of failing charts.
    - [ ] **Tasks:**
        - [ ] Select a representative sample (e.g., 5-10) of the failing charts identified in 10.1, aiming for variety in chart source and error type if possible.
        - [ ] For each sampled chart (using its path in `test/chart-cache`):
            - [ ] Run `helm template <chart_path>` without any extra values. Record the exact error message.
            - [ ] Analyze the error: consult the chart's `values.yaml`, `README.md`, `NOTES.txt`, and template files (`templates/**.yaml`) referenced in the error.
            - [ ] Iteratively add necessary values using `--set key=value` (or `--set-string`, `--set-file` if appropriate) to the `helm template` command until it succeeds *without error*.
            - [ ] Document the minimal set of `--set` parameters required for successful templating for that chart.

- [ ] **Phase 10.3: Categorize Failures and Required Values**
    - [ ] **Goal:** Group common failure patterns and the types of values needed.
    - [ ] **Tasks:**
        - [ ] Based on the findings from the sampled charts (10.2), categorize the root causes of the `helm template` failures (e.g., missing required credentials, unset feature flags, Kubernetes API version checks, invalid default values in the chart itself).
        - [ ] Group the types of values identified as necessary (e.g., `auth.password`, `service.type`, `kubeVersion`, `ingress.enabled`, `someFeature.enabled`).

- [ ] **Phase 10.4: Determine Parameter Type (Type 1 vs. Type 2)**
    - [ ] **Goal:** Classify the required values based on their likely purpose.
    - [ ] **Tasks:**
        - [ ] For each category of required values identified in 10.3, assess whether it's likely:
            - **Type 1 (Deployment-Critical):** Potentially needed for runtime logic related to IRR's overrides (e.g., security flags affecting image handling). These warrant further investigation for potential inclusion in the Rules system.
            - **Type 2 (Test/Validation-Only):** Only needed to satisfy `helm template` requirements (e.g., dummy passwords, `kubeVersion`, enabling optional features not related to images). These should *not* be added to `override.yaml` by default.
        - [ ] **Assumption:** Most parameters identified in this phase will be Type 2. Document any suspected Type 1 parameters separately.

- [ ] **Phase 10.5: Document Findings & Recommendations**
    - [ ] **Goal:** Summarize the investigation results.
    - [ ] **Tasks:**
        - [ ] Create a new document (e.g., `docs/CHART-VALIDATION-ISSUES.md`) summarizing:
            - Common failure categories observed.
            - Examples of charts and the minimal Type 2 `--set` parameters required for them to pass `helm template`.
            - Any potential Type 1 parameters identified that might need rule implementation.
        - [ ] Recommend potential improvements to the testing harness (`test/tools/test-charts.py`) if common Type 2 parameters could be easily supplied during validation runs to increase test coverage.

- [ ] **Phase 10.6: Refine Testing Strategy (Optional Implementation)**
    - [ ] **Goal:** Optionally improve the test harness based on recommendations.
    - [ ] **Tasks:**
        - [ ] If deemed feasible and beneficial from 10.5, modify `test-charts.py` to optionally accept or automatically provide common Type 2 values (like a default `kubeVersion` or common dummy credentials) during its `helm template` execution step.
        - [ ] Ensure any changes clearly distinguish these validation-only values from actual override generation.

### Acceptance Criteria
- A clear list of charts failing `helm template` during testing is available.
- Root causes for failures in a sample set are understood and documented.
- Minimal required Type 2 values for the sample set are identified and documented.
- Potential Type 1 parameters are flagged for further investigation.
- Findings are summarized in `docs/CHART-VALIDATION-ISSUES.md`.
- (Optional) Test harness is updated to improve validation success rate for analysis purposes.
