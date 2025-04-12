# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)

## Phase 2: Configuration & Support Features (Completed)
### Completed Features:


## Phase 3: Component-Group Testing Framework (Completed)

## Phase 4: Core CLI Implementation - COMPLETED
_**Goal:** Implement the core `inspect`, `override`, `validate` commands and the enhanced configuration system with a robust standalone CLI interface._

### Phase 4.0: Command Implementation
- [x] Design and implement core command logic for `inspect`, `override`, `validate`.
  - [x] [Sub-Task] Define shared Go structs (e.g., `ChartInfo`, `ImageAnalysis`, `OverrideResult`).
  - [x] [Sub-Task] Implement core `inspect` logic: Load chart -> Detect images (`pkg/image`) -> Generate analysis report data structure.
  - [x] [Sub-Task] Implement core `override` logic: Load chart -> Load config -> Detect images -> Apply path strategy (`pkg/strategy`) -> Generate override map structure (`pkg/override`).
  - [x] [Sub-Task] Implement core `validate` logic: Load chart -> Load provided values (`--values` flag supports multiple) -> Prepare Helm command args -> Call Helm interaction layer (`internal/helm`) for `helm template` execution.
  - [x] [Sub-Task] Add logic to core command entry points to handle `--release-name` flag (triggering `helm get values` before `helm template`).
  - [ ] [Sub-Task] Ensure backwards compatibility with previous override command during transition (TBD approach).
- [x] Implement standalone CLI interface for the three core commands.
  - [x] [Sub-Task] Set up Cobra commands: Define `inspectCmd`, `overrideCmd`, `validateCmd`.
  - [x] [Sub-Task] Define and parse flags for each command using Cobra (ensure `--values` in `validateCmd` supports multiple files).
  - [x] [Sub-Task] Implement flag validation logic.
  - [x] [Sub-Task] Connect Cobra command `RunE` functions to core logic implementations.
  - [x] [Sub-Task] Implement automatic chart detection from current directory.
- [x] Enhance 'inspect' command with `--generate-config-skeleton` flag.
  - [x] [Sub-Task] Add `--generate-config-skeleton` flag to `inspectCmd`.
  - [x] [Sub-Task] Implement skeleton generation logic: Collect source registries -> Format YAML -> Write output.

### Phase 4.1: Configuration & Chart Handling
- [x] Enhance configuration system
  - [x] [Sub-Task] Define Go structs for new structured config YAML.
  - [x] [Sub-Task] Implement structured YAML parsing.
  - [x] [Sub-Task] Implement backward compatibility: Attempt legacy flat parse if structured parse fails.
  - [x] [Sub-Task] Add config validation functions (e.g., `isValidRegistry`).
  - [x] [Sub-Task] Integrate config loading (default path, `--config` flag) into core logic.
  - [x] [Sub-Task] Implement user-friendly config error messages.
- [x] Develop flexible chart source handling
  - [x] [Sub-Task] Refine `pkg/chart` loader for directory and `.tgz` paths.
  - [x] [Sub-Task] Define/refine primary struct for combined chart metadata/values.
- [x] Enhance subchart handling within core logic
  - [x] [Sub-Task] Verify `pkg/override` correctly uses alias info from loaded chart dependencies.
  - [x] [Sub-Task] Add unit tests for subchart path generation logic in `pkg/override`.
  - [x] [Sub-Task] (Optional) Add basic sanity check for generated subchart override paths.

### Phase 4.2: Reporting & Documentation
- [x] Create analysis reporting functionality for `inspect`
  - [x] [Sub-Task] Define `inspect` report data structure.
  - [x] [Sub-Task] Implement YAML formatter for `inspect` report (sole structured output format).
  - [x] [Sub-Task] Ensure report clearly lists unanalyzable images.
- [x] Create verification and testing assistance helpers (CLI flags)
  - [x] [Sub-Task] Define JSON schema for machine-readable output (TBD if needed beyond YAML).
  - [x] [Sub-Task] Implement JSON output generation for relevant commands (TBD if needed beyond YAML).
  - [x] [Sub-Task] Support test output formats for CI/CD integration (YAML primary).
- [x] Develop core documentation and examples
  - [x] Review and finalize `USE-CASES.md`.
  - [x] Update/Generate `docs/cli-reference.md` (reflecting YAML-only output).
  - [x] Add basic CI/CD examples to documentation.
  - [x] Create `TROUBLESHOOTING.md` with initial common errors.
- [x] Finalize design decisions and remove temporary backward compatibility for config format.
  - [x] Finalize design decisions (covered in Phase 4.3 resolved items).
  - [x] [Sub-Task] Remove legacy config parsing code path (contingent on Phase 4.1/4.3 completion).
  - [x] [Sub-Task] Update tests and documentation to use only structured config format.

### Phase 4.3: Fixing Current Failing tests - COMPLETED
- [x] **COMPLETED** - All integration tests now pass after fixing the following:
  - [x] TestMinimalChart, TestMinimalGitImage, TestMultipleTargetRegistries, TestReadOverridesFromStdout, TestConfigFileMappings, TestRegistryMappingFile, TestComplexChartFeatures, TestDryRunFlag, TestStrictMode and other integration tests 
    - Fixed by correcting the flag name in harness.go from `--integration-test-mode` to `--integration-test` to match root.go

### Phase 4.3.1: Fixing Current Linting Errors
- [x] Fixed cmd/irr package linting errors:
  - Fixed error checking for `cmd.Flags().GetBool("test-analyze")` calls
  - Rewrote if-else chain as switch statement in runAnalyze function
  - Removed unused functions in analyze_test.go
- [x] Fixed naming conventions:
  - Renamed stuttering type names in pkg/chart and pkg/registry packages

### Phase 4.5: IRR_DEBUG Warning Message Fix
_**Goal:** Fix the IRR_DEBUG environment variable warning to maintain test functionality while improving user experience._

- [x] Modify debug package behavior for IRR_DEBUG environment variable warnings
  - [x] [Sub-Task] Update `pkg/debug/debug.go` to suppress warning messages in user-facing contexts. The distinction between contexts will likely be managed using the existing `integrationTestMode` flag or the new internal flag.
  - [x] [Sub-Task] Add an internal flag (e.g., `showDebugEnvWarnings`) to control whether to display IRR_DEBUG parsing warnings.
  - [x] [Sub-Task] Modify the `init()` and `Init()` functions to check this internal flag before printing warnings related to `IRR_DEBUG` parsing.
  - [x] [Sub-Task] Create a new exported function (e.g., `EnableDebugEnvVarWarnings()`) that allows test code to explicitly enable these warnings.
  - [x] [Sub-Task] Update relevant test cases (like those in `cmd/irr/root_test.go`) to call `EnableDebugEnvVarWarnings()` when testing the warning behavior itself.
- [x] Refine root command (`cmd/irr/root.go`) handling for debug settings
  - [x] [Sub-Task] Review the `PersistentPreRunE` logic in `cmd/irr/root.go`. It currently checks `if debugEnv != ""` before parsing, so it shouldn't warn on empty values. The primary warning suppression needs to happen in `pkg/debug/debug.go` as planned above.
  - [x] [Sub-Task] Adjust the `PersistentPreRunE` logic in `cmd/irr/root.go` to only use `log.Warnf` for invalid `IRR_DEBUG` values if the effective log level is `DEBUG` (set by `--debug` or `IRR_DEBUG=true`) OR if the new `debug.ShowDebugEnvWarnings` flag (or similar mechanism) is explicitly enabled for tests.
  - [x] [Sub-Task] Ensure consistent debug state handling: Verify that subcommands rely on the `debug.Enabled` state set by `PersistentPreRunE` and do not independently check `IRR_DEBUG`.
- [x] Ensure compatibility with integration test framework
  - [x] [Sub-Task] Verify integration tests still work properly with the new warning behavior
  - [x] [Sub-Task] Make sure debug state can still be verified programmatically in tests
  - [x] [Sub-Task] Update documentation to reflect new behavior in test environments

### Phase 4.6: Testing Implementation
- [x] **High Priority:** Implement comprehensive Unit/Integration tests for `pkg/analyzer` (inspect logic).
  - [x] Fixed TestAnalyzeCommand_NoArgs test to check for correct error message
  - [x] All analyzer tests now pass successfully
  - [x] Fixed TestAnalyzeCommand_Success_TextOutput and TestAnalyzeCommand_Success_JsonOutput by adding test-analyze flag to command
- [x] **High Priority:** Implement Integration tests for `internal/helm` (Helm command interactions, including `get values` and `template` success/failure simulation).
- [x] **High Priority:** Increase Unit/Integration test coverage for `pkg/override` (override generation logic).
- [x] **High Priority:** Implement Integration/E2E tests for new command flows (`inspect`, `validate`) and error handling in `cmd/irr` (verify `validate` exit codes and stderr passthrough on failure).
- [x] **Medium Priority:** Add Unit/Integration tests for `pkg/chart` (dir/tgz loading, simple alias path generation).
- [x] **Medium Priority:** Add tests for handling both legacy and structured configuration formats.
- [x] Improve test consistency and maintainability:
  - [x] Moved clickhouse-operator chart from `test/chart-cache/` to `test-data/charts/` for consistency with other test charts
  - [x] Updated integration tests to use `testutil.GetChartPath()` consistently for accessing test charts

## Phase 5: Helm Plugin Integration
_**Goal:** Implement the Helm plugin interface that wraps around the core CLI functionality._

- [ ] Create Helm plugin architecture
  - Design plugin structure that wraps around core command logic.
  - Implement plugin installation process and discovery.
  - Ensure consistent command-line interface with standalone CLI.
- [ ] Implement Helm-specific functionality
  - Add release name resolution to chart path.
  - Add Helm environment integration for configuration and auth.
- [ ] Develop Helm plugin testing
  - Implement basic E2E tests for core Helm Plugin workflows (happy path `inspect`, `override`, `validate`).
  - Test plugin installation and registration.
  - Verify Helm release interaction.
- [ ] Update documentation for Helm plugin usage
  - Add Helm-specific examples and workflows.
  - Document plugin installation and configuration.

## Phase 6: Test Framework Refactoring
_**Goal:** Refactor the Python-based `test-charts.py` framework to use the new Phase 4/5 commands with the existing chart corpus._

- [ ] Refactor `test-charts.py` script
  - Update script to call `irr inspect`, `irr override`, and `irr validate` commands using the finalized CLI.
  - Adapt failure categorization and reporting for the new command structure.
  - Ensure script handles the new structured configuration format.
- [ ] Analyze Failure Patterns (on existing corpus)
  - Use the refactored script's output (error reports, patterns) to identify common failure reasons.
- [ ] Improve Values Templates ("Bucket Solvers") (on existing corpus)
  - Iteratively refine the minimal values templates (`VALUES_TEMPLATE_...`) based on failure analysis to increase the success rate.
  - Consider adding new classifications/templates if warranted.
- [ ] Generate Intermediate Compatibility Report
  - Document the success rate achieved after refactoring and initial template improvements.

## Phase 7: Test Corpus Expansion & Advanced Refinement
_**Goal:** Expand the chart test set significantly and further refine compatibility based on broader testing._

- [ ] Expand Test Chart Corpus
  - Increase the number and variety of charts tested (e.g., Top 200+, community requests, specific complex charts).
  - Update chart pulling/caching mechanism if necessary.
- [ ] Re-Analyze Failure Patterns (on expanded corpus)
  - Identify new or more prevalent failure reasons with the larger chart set.
- [ ] Further Improve Values Templates
  - Make additional refinements to templates based on the expanded analysis.
- [ ] Generate Final Compatibility Report
  - Document the final success rate across the expanded corpus.

## Implementation Process: DONT REMOVE THIS SECTION as these hints are important to remember.
- For each change:
  1. **Baseline Verification:**
     - Run full test suite: `go test ./...` 
     - Run full linting: `golangci-lint run`
     - Determine if any existing failures need to be fixed before proceeding with new feature work
  
  2. **Pre-Change Verification:**
     - Run targeted tests relevant to the component being modified
     - Run targeted linting to identify specific issues (e.g., `golangci-lint run --enable-only=unused` for unused variables)
  
  3. **Make Required Changes:**
     - Follow KISS and YAGNI principles
     - Maintain consistent code style
     - Document changes in code comments where appropriate
  
  4. **Post-Change Verification:**
     - Run targeted tests to verify the changes work as expected
     - Run targeted linting to confirm specific issues are resolved
     - Run full test suite: `go test ./...`
     - Run full linting: `golangci-lint run`
  
  5. **Git Commit:**
     - Stop after completing a logical portion of a feature to make well reasoned git commits with changes and comments
     - Request suggested git commands for committing the changes
     - Review and execute the git commit commands yourself, never change git branches stay in the branch you are in until feature completion