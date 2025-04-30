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

## Phase 9: Handle Subcharts (Analyzer Enhancement)

### Overview
Enhance the analyzer to correctly process Helm charts with subcharts, ensuring that image definitions from subchart default values are detected and processed correctly. This addresses limitations found with complex umbrella charts like kube-prometheus-stack.

### Motivation
- The current analyzer only processes the top-level `values.yaml` file provided via `--values` or the chart's default `values.yaml`.
- Images defined only in subchart `values.yaml` files (or other sources merged by Helm) are missed, leading to incomplete `inspect` results and `override` files.
- Users need accurate analysis for complex charts to generate reliable overrides.

### Implementation Steps

#### Phase 9.1: Implement Discrepancy Warning (User Feedback Stop-Gap)

#### Phase 9.2: Subchart Discrepancy Analysis [COMPLETED]
- **Goal:** Systematically analyze charts exhibiting discrepancies between `irr inspect` output and `helm template` rendering, categorize root causes at scale, and document findings to inform the Phase 9.3 refactor.
    - **Conclusion:** While the current analyzer seems adequate for many charts processed successfully by the test, the ~4.4% error rate highlights that the analysis method has limitations. The errors prevent analysis on a subset of charts, potentially masking issues. Therefore, the need for Phase 9.3 (replicating Helm's full value merging) remains critical for robust and accurate handling of all charts, especially complex ones with subcharts. Improving test default values could also reduce errors.

#### Phase 9.3: Refactor Analyzer for Full Subchart Support (The Correct Fix) [COMPLETED]
_Objective: Ensure the analyzer can fully replicate Helm's value merging, including subcharts, to enable accurate image path detection and override generation._

- [x]   **Phase 9.3.1: [P2] Prototype Helm Value Merging & Origin Tracking**
    - [x]   **Goal:** Confirm understanding of Helm SDK value computation sequence and select an origin-tracking method.
    - [x]   **Tasks:**
        - [x]   Use `loader.Load`, `values.Options{}.MergeValues`, `chartutil.CoalesceValues` on a simple parent-child chart (e.g., `test-data/charts/parent-test`).
        - [x]   Verify handling of basic overrides, globals, and aliases (`parent-test` might need slight modification or use another simple chart if it lacks aliases).
        - [x]   Evaluate origin-tracking options (parallel map, value wrapping) based on prototype results and select the most feasible approach.
        - [x]   Document findings and the chosen origin tracking mechanism.
- [x]   **Phase 9.3.2: [P2] Design Final Value Computation Logic**
    - [x]   **Goal:** Define the precise process and data structures for replicating Helm's value computation, incorporating findings from the prototype.
    - [x]   **Tasks:**
        - [x]   Finalize the Go data structures for representing merged values and tracked origins.
        - [x]   Document the step-by-step logic for loading a chart and its dependencies, processing user values (`-f`, `--set`), and coalescing all values correctly, including handling `dependencies`, globals, and aliases.
- [x]   **Phase 9.3.3: [P2] Define Analysis Context Input Structure**
    - [x]   **Goal:** Specify the input required by the refactored analyzer.
    - [x]   **Tasks:**
        - [x]   Define the Go struct (e.g., `ChartAnalysisContext`) that will encapsulate the merged values and origin information.
        - [x]   Update the primary analysis function signature (e.g., `analyzer.AnalyzeContext`) to accept this new struct.
- [x]   **Phase 9.3.4: [P2] Analyze Merged Value Structure & Define Alias Handling**
    - [x]   **Goal:** Ensure the traversal logic can handle Helm's output and define how aliases impact source paths.
    - [x]   **Tasks:**
        - [x]   Examine the structure of the values map returned by the prototype (Phase 9.3.1) for potential edge cases (complex types, lists) affecting `analyzeMapValue`, `analyzeStringValue`, etc.
        - [x]   Define the precise logic for constructing the `SourcePath` when a value originates from a subchart accessed via an alias.
- [x]   **Phase 9.3.5: [P2] Adapt Analyzer Traversal & Source Path Logic**
    - [x]   **Goal:** Update image detection to use origin info for correct source paths.
    - [x]   **Tasks:**
        - [x]   Modify recursive analysis functions (`analyzeMapValue`, etc.).
        - [x]   Implement logic to consult the origin data when an image is found.
        - [x]   Construct the final `SourcePath`, prepending subchart names/aliases based on origin. Handle potential edge cases identified in 9.3.4.
- [x]   **Phase 9.3.6: [P2] Define Chart Loading Utility Interface**
    - [x]   **Goal:** Specify the contract for the reusable chart loading/computation component.
    - [x]   **Tasks:**
        - [x]   Define the Go interface (function signature(s), input/output structs, error handling) for the utility package/function (e.g., in `pkg/helm` or a new `pkg/chartutiladapter`).
- [x]   **Phase 9.3.7: [P2] Implement and Integrate Chart Loading Utility**
    - [x]   **Goal:** Build the utility function and integrate it into commands.
    - [x]   **Tasks:**
        - [x]   Implement the utility function defined in 9.3.6, orchestrating the Helm SDK calls based on the design in 9.3.2. (Implemented in `internal/helm`)
        - [x]   **Consolidate prototype logic:** Ensure functions prototyped in 9.3.1 (e.g., from `internal/helm/value_merge_prototype.go`) are moved to the final utility implementation (e.g., `internal/helm/chart_loader.go`) and the standalone prototype file is **removed** to prevent duplication.
        - [x]   Modify `cmd/irr/inspect.go` and `cmd/irr/override.go`:
            - [x]   Remove old value file loading. (Done in `inspect.go` & `override.go`)
            - [x]   Use `pkg/cli/values` to process flags. (Using Helm's `values.Options` directly)
            - [x]   Call the new utility function to get the `ChartAnalysisContext`. (Done in `inspect.go` & `override.go`)
            - [x]   Pass this context to the refactored `analyzer.AnalyzeContext` (from 9.3.3). (Done implicitly via utility)
        - [x]   Ensure `override.go` correctly uses the enhanced `SourcePath` (from 9.3.5) for output YAML structure.
        - [x]   **Note:** Remember that the `override` command logic must ultimately distinguish between Type 1 (Deployment-Critical) and Type 2 (Test/Validation-Only) parameters, including only Type 1 in the final output. See `docs/SOLVER.md` for details on this categorization. (Logic preserved in refactor)
        - [x]   **Lint frequently:** Run `make lint` after significant changes during integration to catch issues early. (Done throughout)
- [x]   **Phase 9.3.8: [P2] Identify Specific Test Case Charts**
    - [x]   **Goal:** Select concrete charts for validation.
    - [x]   **Tasks:**
        - [x]   Confirm `test-data/charts/kube-prometheus-stack` as a primary complex test case.
        - [x]   Select `test-data/charts/parent-test` (or similar) for basic subchart testing.
        - [x]   Consider adding one more public chart known for complex dependencies if needed (e.g., check Bitnami catalog later if `kube-prometheus-stack` proves insufficient for edge cases).
- [x]   **Phase 9.3.9: [P2] Add Comprehensive Tests**
    - [x]   **Goal:** Verify end-to-end correctness.
    - [x]   **Tasks:**
        - [x]   Create/enhance integration tests in `test/integration/` using charts identified in 9.3.8.
        - [x]   Cover scenarios: simple chart, single/multi-level subcharts, subchart default images, parent overrides, user overrides, globals, aliases, disabled subcharts.
        - [x]   **Inspect Verification:** Assert correct source paths (e.g., `image`, `child.image`, `aliasedChild.image`).
        - [x]   **Override Verification:** Assert correctly structured YAML output.
        - [x]   **Targeted Origin Tests:** Create specific unit tests verifying `ValueOrigin` fields (especially `Type`, `Path`, `ChartName`) for values originating from parent defaults, subchart defaults, user files, and `--set` flags, using charts like `parent-test`.
- [x]   **Phase 9.3.10: [P2] Update Documentation**
    - [x]   **Goal:** Reflect the new capabilities.
    - [x]   **Tasks:**
        - [x]   Remove documented subchart limitations (`README.md`, `docs/LIMITATIONS.md`, etc.).
        - [x]   Update examples (`docs/CLI-REFERENCE.md`, tutorials) if necessary to show complex chart usage.


### Hints for Refactoring `cmd/irr/inspect.go` (from previous attempt):

**1. Fix Imports (Top Priority):**
   - **Remove:** `k8s.io/helm/pkg/chartutil`, `sigs.k8s.io/yaml.v3`
   - **Ensure:** `helm.sh/helm/v3/pkg/chartutil`, `gopkg.in/yaml.v3`, `os` are imported.
   - **Alias:** Use `internalhelm "github.com/lucas-albers-lz4/irr/internal/helm"` to prevent conflicts and clarify calls.

**2. Eliminate Redeclarations:**
   - **Delete** duplicate/placeholder definitions for functions:
     - `createHelmClient`
     - `newInspectCmd`
     - `processImagePatterns`
     - `runInspect`
     - `inspectChartPath`
   - **Delete** duplicate `helmAdapterFactory` variable definition.
   - Ensure only *one* correct definition remains for each symbol.

**3. Resolve Undefined Symbols:**
   - **Replace:** `helm.NewHelmChartLoaderComputer` with `internalhelm.NewHelmChartLoaderComputer`.
   - **Locate/Define:** Package-level `var` or `const` for `defaultIncludePatterns` and `defaultExcludePatterns`.
   - **Verify Logging Setup:** Confirm `log.SetupLogging` call in `PersistentPreRunE` uses correct flag scope (`logLevel`, `logFormat`).
   - **Confirm YAML Usage:** `yaml.Marshal` requires the correct `gopkg.in/yaml.v3` import.

**4. Remove Obsolete Code:**
   - **Delete:** The entire `helmAdapterFactory`

## Notes for Continuation (End of Day - YYYY-MM-DD)

**Current Status:**
- Integrated the new `internal/helm.ChartLoader` and `internal/helm.ContextAwareAnalyzer` into the `cmd/irr/inspect.go` command when the `--context-aware` flag is used.
- Completed steps 9.3.1, 9.3.3, 9.3.4, 9.3.5, 9.3.6, and parts of 9.3.7 (integration into `inspect`).



**Next Steps:**
1.  **Fix Lint Errors:**

3.  **Implement Tests (Phase 9.3.9):** Add comprehensive integration tests for the context-aware analysis and override functionality.
4.  **Update Documentation (Phase 9.3.10):** Reflect the new subchart handling capabilities.

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
