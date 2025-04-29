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


## Phase 11: Decouple Override Validation [COMPLETED]
- Introduced `--no-validate` flag to `irr override`.

## Phase 12: Enhance Override Robustness for Live Release Value Errors [COMPLETED]
- Improved handling of problematic strings in live release values, including fallback to default values with a warning.

## Phase 13: Code Documentation Review and Alignment [COMPLETED]
- Reviewed and aligned Go code documentation with current functionality.

## Phase 14: Add --all-namespaces (-A) Flag to `inspect` [COMPLETED]
- Added `-A`/`--all-namespaces` and `--overwrite-skeleton` flags to `irr inspect`.
- Enabled cluster-wide image inspection and skeleton generation.

## Phase 9: Handle Subcharts (Analyzer Enhancement)

### Overview
Enhance the analyzer to correctly process Helm charts with subcharts, ensuring that image definitions from subchart default values are detected and processed correctly. This addresses limitations found with complex umbrella charts like kube-prometheus-stack.

### Motivation
- The current analyzer only processes the top-level `values.yaml` file provided via `--values` or the chart's default `values.yaml`.
- Images defined only in subchart `values.yaml` files (or other sources merged by Helm) are missed, leading to incomplete `inspect` results and `override` files.
- Users need accurate analysis for complex charts to generate reliable overrides.

### Implementation Steps

#### Phase 9.1: Implement Discrepancy Warning (User Feedback Stop-Gap)
- [ ] **[P1]** **Setup Helm Environment:**
    - In `cmd/irr/inspect.go`, ensure necessary Helm SDK packages are imported (e.g., `action`, `loader`, `cli`, `cli/values`, `chartutil`).
    - Create Helm environment settings (`helm.EnvSettings`).
- [ ] **[P1]** **Add Control Flag:**
    - Define the `--no-subchart-check` boolean flag (default `false`) using `cobra`.
    - Retrieve the flag's value during command execution.
- [ ] **[P1]** **Conditional Check Execution:**
    - Implement the following steps only if `--no-subchart-check` is *not* provided by the user.
- [ ] **[P1]** **Load Chart and Values via Helm SDK:**
    - Use `loader.Load(chartPath)` to load the chart. Handle errors gracefully (log error, exit non-zero).
    - Prepare value options (`values.Options`) based on user-provided `--values` flags.
    - Use Helm's logic (e.g., `chartutil.CoalesceValues` or similar) to merge provided values with the chart's default `values.yaml`. Handle value merging errors (log error, exit non-zero).
- [ ] **[P1]** **Render Chart Templates via Helm SDK:**
    - Instantiate `action.NewInstall` using the Helm environment settings.
    - Configure the install action for dry-run, client-only rendering (e.g., `inst.DryRun = true`, `inst.ClientOnly = true`, use fixed dummy values like `ReleaseName: "irr-subchart-check"`, `Namespace: "default"`).
    - Execute `inst.Run(chart, mergedValues)` to render the templates.
    - Capture the resulting multi-document YAML string.
    - Handle template execution errors robustly (log error, exit non-zero).
- [ ] **[P1]** **Parse Rendered Manifests (Limited Scope):**
    - Use a YAML parser (`gopkg.in/yaml.v3` recommended) to split the multi-document YAML string.
    - Iterate through each document:
        - Attempt to parse the document into a generic structure (e.g., `map[string]interface{}`). If parsing fails, log a warning including the specific error, treat the doc as having 0 images, and continue to the next document.
        - Check `kind`: If `Deployment` or `StatefulSet`, use *safe traversal techniques* (explicitly check for nil maps/slices at each level) when accessing nested fields (e.g., `spec.template.spec...image`) to extract unique image reference strings, preventing panics.
        - Store unique image strings (e.g., in a `map[string]struct{}`).
    - *Note: This limited parsing scope (Deployments/StatefulSets) is intentional for this stop-gap phase.*
- [ ] **[P1]** **Compare Image Counts:**
    - Get the count of unique images found by the *existing* `analyzer.AnalyzeHelmValues` mechanism (run this analysis as usual).
    - Compare this count with the number of unique image strings extracted from the rendered Deployments/StatefulSets (from the previous step).
    - *Note: Implement a circuit breaker: if the number of images extracted from rendered templates exceeds 300, skip the comparison and do not issue the warning, logging a debug message instead.*
- [ ] **[P1]** **Issue Warning on Mismatch (No Exit Code Change):**
    - If the counts differ (and the circuit breaker was not triggered), use `log.Warnf` (or `slog` equivalent) to output a clear message at the `WARN` level, including structured attributes:
        - Add a specific key like `check="subchart_discrepancy"` to allow easy machine filtering (e.g., via `jq`).
        - Include the counts like `analyzer_image_count=X`, `template_image_count=Y`.
    - The human-readable message part should state the counts (analyzer vs. template), explain the likely cause (subchart defaults), mention the limited scope (Deployments/StatefulSets), and reference the `--no-subchart-check` flag. It should *not* list specific image names.
    - **Crucially:** This warning itself **does not** trigger a non-zero exit code.
- [ ] **[P1]** **Add Integration Tests:**
    - In `test/integration/inspect_test.go` (or similar):
        - **Success Case (Match):** Chart where counts match -> No warning, exit 0.
        - **Warning Case (Mismatch):** Umbrella chart (e.g., `kube-prometheus-stack`) -> Warning logged, exit 0.
        - **Disabled Case (Mismatch):** Umbrella chart with `--no-subchart-check` -> No warning logged, exit 0.
        - **Error Case (Chart Load Fail):** Invalid chart path -> Error logged, exit non-zero.
        - **Error Case (Render Fail):** Chart with template syntax error -> Error logged, exit non-zero.
        - **Error Case (Value Fail):** Invalid values file -> Error logged, exit non-zero.
        - **Edge Case (No Deploy/SS):** Chart with no Deployments/StatefulSets -> No warning, exit 0.
        - **Edge Case (No Images):** Chart with Deployments/StatefulSets but no images -> No warning, exit 0.
        - **Edge Case (YAML Doc Error):** Chart rendering one valid Deployment and one malformed document -> Warning for parse error logged, count comparison based on valid doc, exit 0 (unless counts mismatch).
- [ ] **[P1]** **Update Documentation:**
    - Update `docs/CLI-REFERENCE.md` with the `--no-subchart-check` flag details.
    - Add/update a section in `docs/TROUBLESHOOTING.md` explaining the warning, its limited scope, the flag, emphasizing exit code behavior, and include an *example of the warning message format*.
- [ ] **[P1]** **Error Handling Summary:**
    - Reminder: Fatal errors during Helm loading, value merging, or template rendering should log the specific error and exit non-zero. YAML parsing errors within the rendered stream or the image count discrepancy warning itself should only log warnings and allow the command to complete with exit code 0.

#### Phase 9.2: Refactor Analyzer for Full Subchart Support (The Correct Fix)
- [x] **[P2]** **Research & Design Helm Value Computation:**
    - Deeply investigate Helm Go SDK functions for loading charts (`chart/loader.Load`), handling dependencies, and merging values (`pkg/cli/values.Options`, `pkg/chartutil.CoalesceValues`).
    - Prototype code to programmatically replicate Helm's value computation process for a given chart and user-provided value files, resulting in a final, merged values map representing what Helm uses for templating.
    - *Crucial Design Point:* Determine how to track the origin of each value within the merged map (e.g., did it come from the parent `values.yaml`, a specific subchart's `values.yaml`, or a user file?). This origin information is essential for generating correctly structured overrides later.
- [x] **[P2]** **Refactor Analyzer Input:**
    - Modify the analyzer's primary entry function (e.g., `AnalyzeHelmValues` or potentially a new function like `AnalyzeChartContext`).
    - Instead of just `map[string]interface{}` representing a single values file, the input should represent the fully computed/merged values for the chart context (from step 1).
    - The function signature might also need to accept information about value origins (design from step 1) if that's how source path tracking is implemented.
- [x] **[P2]** **Adapt Analyzer Traversal & Source Path Logic:**
    - The core recursive analysis functions (`analyzeMapValue`, `analyzeStringValue`) might largely remain the same if they operate correctly on the merged values map.
    - **Critical Enhancement:** Modify the logic that records `ImagePattern` (or equivalent). When an image is detected, it must now correctly determine and store its *effective source path* suitable for override generation. This involves using the value origin tracking (from step 1) to construct the correct path (e.g., an image from the `grafana` subchart needs a path starting with `grafana.`).
- [x] **[P2]** **Update Command Usage:**
    - Modify `cmd/irr/inspect.go` and `cmd/irr/override.go`.
    - Remove the simple loading of a single values file.
    - Implement the Helm chart loading and value computation logic designed in step 1.
    - Call the refactored analyzer (step 2) with the computed values and necessary context.
    - Ensure `override` correctly uses the enhanced source path information (step 3) to structure the generated YAML override file (e.g., placing Grafana image overrides under a top-level `grafana:` key).
- [x] **[P2]** **Add Comprehensive Tests:**
    - Create/enhance integration tests in `test/integration/` specifically for umbrella charts.
    - Use `kube-prometheus-stack` and potentially other charts with multiple nesting levels.
    - Verify `inspect` output now includes images defined only in subchart defaults.
    - Verify `override` generates correctly structured files, applying overrides to the appropriate subchart keys (e.g., `grafana: { image: ... }`, `kube-state-metrics: { image: ... }`).
- [x] **[P2]** **Update Documentation:**
    - Remove documented limitations regarding subchart analysis.
    - Ensure examples demonstrate usage with complex umbrella charts.

#### Phase 9.3: Review/Remove Warning Mechanism
- [x] **[P3]** **Evaluate Necessity:**
    - Once Phase 9.2 is complete and validated through extensive testing, determine if the warning mechanism from Phase 9.1 still provides value or is now redundant.
- [x] **[P3]** **Conditional Removal:**
    - If the refactored analyzer (Phase 9.2) is proven reliable for subchart value analysis, remove the Helm template comparison code, the `--no-subchart-check` flag, associated tests, and documentation related to the warning mechanism.

### Acceptance Criteria (Phase 9.2)
- `irr inspect` correctly identifies images defined in both parent and subchart values, reporting accurate source paths reflecting subchart context (e.g., `grafana.image`).
- `irr override` correctly generates overrides for images originating from both parent and subchart values, placing them under the correct top-level keys in the output file (e.g., `grafana: { image: ... }`).
- Tests confirm accurate behavior for multiple levels of subchart nesting and various value override scenarios.
- Documented limitations regarding subcharts are removed.



