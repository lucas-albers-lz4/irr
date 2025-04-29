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

#### Phase 9.3: Refactor Analyzer for Full Subchart Support (The Correct Fix)
_Objective: Ensure the analyzer can fully replicate Helm's value merging, including subcharts, to enable accurate image path detection and override generation._

- [ ]   **Phase 9.3.1: [P2] Prototype Helm Value Merging & Origin Tracking**
    - [ ]   **Goal:** Confirm understanding of Helm SDK value computation sequence and select an origin-tracking method.
    - [ ]   **Tasks:**
        - [ ]   Use `loader.Load`, `values.Options{}.MergeValues`, `chartutil.CoalesceValues` on a simple parent-child chart (e.g., `test-data/charts/parent-test`).
        - [ ]   Verify handling of basic overrides, globals, and aliases (`parent-test` might need slight modification or use another simple chart if it lacks aliases).
        - [ ]   Evaluate origin-tracking options (parallel map, value wrapping) based on prototype results and select the most feasible approach.
        - [ ]   Document findings and the chosen origin tracking mechanism.
- [ ]   **Phase 9.3.2: [P2] Design Final Value Computation Logic**
    - [ ]   **Goal:** Define the precise process and data structures for replicating Helm's value computation, incorporating findings from the prototype.
    - [ ]   **Tasks:**
        - [ ]   Finalize the Go data structures for representing merged values and tracked origins.
        - [ ]   Document the step-by-step logic for loading a chart and its dependencies, processing user values (`-f`, `--set`), and coalescing all values correctly, including handling `dependencies`, globals, and aliases.
- [ ]   **Phase 9.3.3: [P2] Define Analysis Context Input Structure**
    - [ ]   **Goal:** Specify the input required by the refactored analyzer.
    - [ ]   **Tasks:**
        - [ ]   Define the Go struct (e.g., `ChartAnalysisContext`) that will encapsulate the merged values and origin information.
        - [ ]   Update the primary analysis function signature (e.g., `analyzer.AnalyzeContext`) to accept this new struct.
- [ ]   **Phase 9.3.4: [P2] Analyze Merged Value Structure & Define Alias Handling**
    - [ ]   **Goal:** Ensure the traversal logic can handle Helm's output and define how aliases impact source paths.
    - [ ]   **Tasks:**
        - [ ]   Examine the structure of the values map returned by the prototype (Phase 9.3.1) for potential edge cases (complex types, lists) affecting `analyzeMapValue`, `analyzeStringValue`, etc.
        - [ ]   Define the precise logic for constructing the `SourcePath` when a value originates from a subchart accessed via an alias.
- [ ]   **Phase 9.3.5: [P2] Adapt Analyzer Traversal & Source Path Logic**
    - [ ]   **Goal:** Update image detection to use origin info for correct source paths.
    - [ ]   **Tasks:**
        - [ ]   Modify recursive analysis functions (`analyzeMapValue`, etc.).
        - [ ]   Implement logic to consult the origin data when an image is found.
        - [ ]   Construct the final `SourcePath`, prepending subchart names/aliases based on origin. Handle potential edge cases identified in 9.3.4.
- [ ]   **Phase 9.3.6: [P2] Define Chart Loading Utility Interface**
    - [ ]   **Goal:** Specify the contract for the reusable chart loading/computation component.
    - [ ]   **Tasks:**
        - [ ]   Define the Go interface (function signature(s), input/output structs, error handling) for the utility package/function (e.g., in `pkg/helm` or a new `pkg/chartutiladapter`).
- [ ]   **Phase 9.3.7: [P2] Implement and Integrate Chart Loading Utility**
    - [ ]   **Goal:** Build the utility function and integrate it into commands.
    - [ ]   **Tasks:**
        - [ ]   Implement the utility function defined in 9.3.6, orchestrating the Helm SDK calls based on the design in 9.3.2.
        - [ ]   Modify `cmd/irr/inspect.go` and `cmd/irr/override.go`:
            - [ ]   Remove old value file loading.
            - [ ]   Use `pkg/cli/values` to process flags.
            - [ ]   Call the new utility function to get the `ChartAnalysisContext`.
            - [ ]   Pass this context to the refactored `analyzer.AnalyzeContext` (from 9.3.3).
        - [ ]   Ensure `override.go` correctly uses the enhanced `SourcePath` (from 9.3.5) for output YAML structure.
        - [ ]   **Note:** Remember that the `override` command logic must ultimately distinguish between Type 1 (Deployment-Critical) and Type 2 (Test/Validation-Only) parameters, including only Type 1 in the final output. See `docs/SOLVER.md` for details on this categorization.
- [ ]   **Phase 9.3.8: [P2] Identify Specific Test Case Charts**
    - [ ]   **Goal:** Select concrete charts for validation.
    - [ ]   **Tasks:**
        - [ ]   Confirm `test-data/charts/kube-prometheus-stack` as a primary complex test case.
        - [ ]   Select `test-data/charts/parent-test` (or similar) for basic subchart testing.
        - [ ]   Consider adding one more public chart known for complex dependencies if needed (e.g., check Bitnami catalog later if `kube-prometheus-stack` proves insufficient for edge cases).
- [ ]   **Phase 9.3.9: [P2] Add Comprehensive Tests**
    - [ ]   **Goal:** Verify end-to-end correctness.
    - [ ]   **Tasks:**
        - [ ]   Create/enhance integration tests in `test/integration/` using charts identified in 9.3.8.
        - [ ]   Cover scenarios: simple chart, single/multi-level subcharts, subchart default images, parent overrides, user overrides, globals, aliases, disabled subcharts.
        - [ ]   **Inspect Verification:** Assert correct source paths (e.g., `image`, `child.image`, `aliasedChild.image`).
        - [ ]   **Override Verification:** Assert correctly structured YAML output.
- [ ]   **Phase 9.3.10: [P2] Update Documentation**
    - [ ]   **Goal:** Reflect the new capabilities.
    - [ ]   **Tasks:**
        - [ ]   Remove documented subchart limitations (`README.md`, `docs/LIMITATIONS.md`, etc.).
        - [ ]   Update examples (`docs/CLI-REFERENCE.md`, tutorials) if necessary to show complex chart usage.


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
   - **Delete:** The entire `helmAdapterFactory` placeholder variable block (it causes type errors: `nil` as `Adapter`).
   - **Remove:** `Namespace` field from `InspectFlags` struct and its assignment in `getInspectFlags`.

**5. Update Command Definition (`newInspectCmd`):**
   - **Add Flags:** Include standard Helm value flags: `--values`, `--set`, `--set-string`, `--set-file`.
   - **Verify Value Passing:** Ensure `inspectChartPath` correctly collects these flag values and passes them to `internalhelm.NewHelmChartLoaderComputer(...).LoadAndComputeVals`.

**Key Insight:** Previous failures likely stemmed from edit conflicts or incorrect context. Address imports and duplicates cleanly *first* before fixing other errors. Use the `internalhelm` alias consistently for the chart loader.



#### Phase 9.4: Review/Remove Warning Mechanism
- [ ] **[P3]** **Evaluate Necessity:**
    - Once Phase 9.3 is complete and validated through extensive testing, determine if the warning mechanism from Phase 9.1 still provides value or is now redundant.
- [ ] **[P3]** **Conditional Removal:**
    - If the refactored analyzer (Phase 9.3) is proven reliable for subchart value analysis, remove the Helm template comparison code, the `--no-subchart-check` flag, associated tests, and documentation related to the warning mechanism.

### Acceptance Criteria (Phase 9.3)
- `irr inspect` correctly identifies images defined in both parent and subchart values, reporting accurate source paths reflecting subchart context (e.g., `grafana.image`).
- `irr override` correctly generates overrides for images originating from both parent and subchart values, placing them under the correct top-level keys in the output file (e.g., `grafana: { image: ... }`).
- Tests confirm accurate behavior for multiple levels of subchart nesting and various value override scenarios.
- Documented limitations regarding subcharts are removed.



