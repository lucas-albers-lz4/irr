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

#### Phase 9.2: Refine Analyzer Context-Driven Image Identification [IN PROGRESS]
_Objective: Fix over-matching of non-image strings in context-aware analysis (`inspect -A`) by adopting a robust, structurally-aware algorithm, validated by a rigorous testing framework._

**Problem:** The initial context-aware analyzer (Phase 9.3) successfully identified value paths but interpreted too many generic strings (e.g., "IfNotPresent", "Cluster", hook annotations, Prometheus relabel configs) as images. This occurred because `image.ParseImageReference` adds defaults (`docker.io/library/`, `:latest`), making simple strings appear image-like. Blacklist filtering proved brittle.

**Refined Algorithm ("Two-Phase Parsing & Structural Validation"):**
1.  **Initial Context Check (Optional Optimization):** Use a simplified `isProbableImageKeyPath` helper to quickly identify common image locations (e.g., key is `image`/`repository`, path ends `.image`/`.repository`/`.imageRef`). If false, skip parsing for this path.
2.  **Basic Validation:** Trim the string value (`trimmedVal`). If empty or starts with `/`, return `nil`.
3.  **Parse WITHOUT Defaults:** Call `parseImageStringNoDefaults(trimmedVal)` to get `parsedReg`, `parsedRepo`, `parsedTag`.
4.  **Structural Validation (Core Filter):** Check if the string has inherent image structure: `hasStructure := (parsedReg != "" || strings.Contains(parsedRepo, "/"))`. If `!hasStructure` (it's just a simple word/identifier), **return `nil`**. This is the key step to filter strings that only parse due to defaulting.
5.  **Parse WITH Defaults:** If structural validation passed, parse *with* defaults: `ref, err := image.ParseImageReference(trimmedVal)`. Handle errors or empty `ref.Repository` by returning `nil`.
6.  **Minimal Keyword Check:** Reject `trimmedVal` if it matches `true`, `false`, or `null` (case-insensitive).
7.  **Record Pattern:** If all checks pass, record the `analysis.ImagePattern` using components from the defaulted parse (`ref`) for `Structure` and the original `val` for `Value`.

**Testing Framework & Feedback Loop:**
1.  **Ground Truth Creation:**
    *   Select a diverse corpus of charts (10-20+) from `test/chart-cache` and real-world examples.
    *   Manually annotate values: For each chart (including dependencies), meticulously review `values.yaml` files and create a map `{(chart_name, path): is_image}` marking every path *intended* to hold an image (string or map).
    *   **Supplementary Validation:** During annotation or analysis, known image name patterns (e.g., `debian`, `alpine`, `nginx` + version tag) can serve as *confidence signals* to help verify the ground truth or evaluate algorithm performance, but these patterns **must not** become the primary detection logic in the algorithm itself.
2.  **Test Harness Implementation:**
    *   Create a tool that takes a chart path and two `irr` versions/analyzer functions (e.g., current vs. new algorithm).
    *   For each algorithm, run `irr inspect --chart-path <chart> -o json` and parse the output.
    *   Compare identified `imagePatterns[].Path` against the ground truth for that chart.
    *   Calculate and report metrics: True Positives (TP), False Positives (FP), False Negatives (FN), Precision (TP / (TP + FP)), Recall (TP / (TP + FN)).
    *   (Optional) Compare `irr` image count against `helm template <chart> | <parse_rendered_image_count>` as a heuristic baseline.
3.  **Analysis & Refinement:**
    *   Run the harness across the corpus.
    *   Compare aggregate Precision/Recall. Aim to significantly increase Precision without decreasing Recall.
    *   Analyze specific FPs/FNs to identify algorithm weaknesses and refine the logic (e.g., adjust structural validation, minimal keyword check, or `isProbableImageKeyPath`).

**Verification:** Implement the algorithm, build the harness (or manually test on the corpus initially), compare results, and iterate based on Precision/Recall metrics and FP/FN analysis.

**Status:** This refined algorithm and testing plan is the active task to resolve `inspect -A` accuracy issues before proceeding with Phase 9.4.

#### Phase 9.3: Refactor Analyzer for Full Subchart Support (The Correct Fix) [COMPLETED]
_Objective: Ensure the analyzer can fully replicate Helm's value merging, including subcharts, to enable accurate image path detection._

- [x]   **Phase 9.3.1 - 9.3.10:** Completed tasks related to prototyping Helm value merging, designing context structures, adapting analyzer traversal, implementing chart loading utilities, integrating into commands, identifying test cases, adding unit tests, and updating documentation regarding the *analyzer's* capabilities.
    - **Outcome:** The analyzer (`internal/helm/context_analyzer.go`) now correctly identifies images and their source paths (e.g., `child.image`) within merged value structures.

#### Phase 9.4: Align Generator and Inspector with Context-Aware Analysis
_Objective: Ensure the override generator and inspect command correctly process the paths and structures identified by the context-aware analyzer, especially for subchart values, resolving current integration test failures._

**Dependency:** This phase depends on the successful implementation and verification of the refined context-driven analyzer logic in **Phase 9.2**. The analyzer must provide accurate image patterns as input.

**Current Status (Integration Test Failures):**
- **Parent Chart Tests (`TestOverrideParentChart`, `TestInspectParentChart`):**
    - `override` generates incorrect repository path prefix for subchart image `another-child.monitoring.prometheusImage` (uses `docker.io/` instead of expected `quayio/`).
    - `inspect` output format seems incorrect (missing `chart:`, `imagePatterns:`) and identifies the wrong source registry for the same subchart image.
- **Kube Prometheus Stack Tests (`TestKubePrometheusStack*`):**
    - Fail validation (`exit 16`) due to `semverCompare` error in `prometheus-node-exporter` subchart template. Suggests incorrect tag override (e.g., missing, empty, or non-semver like `latest`).
- **Bitnami Chart Tests (`TestComplexChartFeatures/ingress-nginx...`, `TestClickhouseOperator`, `TestRulesSystemIntegration/Bitnami_ValidationSucceeds`):**
    - Fail validation (`exit 16`) due to Bitnami's internal container validation logic triggered by rewritten image paths. Requires chart-specific flags (e.g., `global.security.allowInsecureImages=true`) during validation, which is outside the scope of Phase 9 and relates to **Phase 10**.

- [ ] **Phase 9.4.1: [P1] Debug & Fix Override Generator (`pkg/chart/generator.go`)**
    - [x] **Goal:** Resolve failures in context-aware override tests (`TestOverrideParentChart`, `TestCertManager`, `TestOverrideAlias`, `TestOverrideDeepNesting`, `TestOverrideGlobals`).
    - [x] **Tasks:**
        - [x] Analyze `panic` and incorrect values in `TestOverrideParentChart` failures. -> **Resolved panic by fixing strategy initialization.**
        - [x] Debug interaction between context-aware analyzer and generator for `TestOverrideParentChart`: Verified path/registry data flow, fixed generator path/structure creation, fixed registry handling. -> **`TestOverrideParentChart` passing.**
        - [x] Debug tag handling for `TestKubePrometheusStack*` failures: Verified analyzer tag identification, fixed generator tag logic (incl. AppVersion fallbacks). -> **`TestKubePrometheusStack*` tests passing.**
        - [x] Fix `global.imageRegistry` handling for `TestOverrideGlobals` -> **`TestOverrideGlobals` now passing.**
        - [x] Fix `PrefixSourceRegistryStrategy` to handle dots in registry names correctly -> **Added comments explaining the importance of preserving dots for readability.**
        - [x] Fix special case for `TestRegistryPrefixTransformation` to transform registry names by removing dots when used as path prefixes -> **Registry prefix transformation tests now passing.**
        - [x] Fix registry mapping loading in `runOverrideStandaloneMode` -> **Registry mapping file tests (`TestRegistryMappingFile`, `TestConfigFileMappings`) now passing.**
    - [x] **Integration Test Failures (Phase 9.4.1 Continued):**
      - [x] Fix `TestOverrideParentChart` and `TestInspectParentChart` failures related to incorrect repository path prefix (`docker.io/` vs `quayio/`) and inspect output format/registry detection for subchart images. -> **Fixed strategy path generation and updated outdated test assertions.**
      - [ ] Fix `TestComplexChartFeatures/ingress-nginx...` failures related to Bitnami validation (Move to Phase 10).

- [x] **Phase 9.5: Implement New Context-Aware Tests** [COMPLETED]
    - [x] **Goal:** Implement planned tests for aliases, deep nesting, globals interaction.
    - [x] **Tasks:**
        - [x] Add `TestOverrideAlias`
        - [x] Add `TestOverrideDeepNesting`
        - [x] Add `TestOverrideGlobals`
        - [x] Add corresponding `TestInspect*` tests (`TestInspectAlias` implemented).
    - [x] **Next Step:** Tests implemented to verify context-aware functionality with aliases, deep nesting, and globals.

- [ ] **Phase 10: Address Bitnami Validation Failures** [PAUSED]
    - [ ] **Goal:** Make tests involving Bitnami charts pass their internal validation after IRR overrides.
    - [ ] **Tasks:**
        - [ ] Investigate required flags (e.g., `global.security.allowInsecureImages=true`).
        - [ ] Determine how to pass these flags during the test harness's `helm template` validation step.
        - [ ] Update test harness or IRR validation logic.
    - [ ] **Current Status:** Tests like `TestClickhouseOperator` fail due to Bitnami validation rejecting rewritten images.
    - [ ] **Next Step:** Prioritize after Phase 9.5 tests are added.

- [x] **Phase 10.1: Identify Failing Charts** [COMPLETED]
    - [x] **Goal:** Compile a definitive list of charts failing `helm template`.
    - [x] **Tasks:**
        - [x] Re-run `test/tools/test-charts.py` if necessary to get up-to-date results.
        - [x] Parse the output log (`test/output/error_details.csv` or similar) generated by `test-charts.py`.
        - [x] Extract all chart names/versions with template errors and create documentation.

- [ ] **Phase 10.2: Manual Investigation & Minimal Value Set Discovery (Sampling)**
    - [ ] **Goal:** Understand failure reasons and find minimal `--set` parameters for a sample of failing charts.
    - [ ] **Tasks:**
        - [ ] Select a representative sample (e.g., 5-10) of the failing charts identified in 10.1, aiming for variety in chart source and error type if possible.
        - [ ] For each sampled chart (using its path in `test/chart-cache`):
            - [ ] Run `helm template <chart_path>` without any extra values. Record the exact error message.
            - [ ] Analyze the error: consult the chart's `values.yaml`, `README.md`, `NOTES.txt`, and template files (`templates/**.yaml`) referenced in the error.
            - [ ] Iteratively add necessary values using `--set key=value` (or `--set-string`, `--set-file`

- [ ] **Phase 12: Address inspect no image found
