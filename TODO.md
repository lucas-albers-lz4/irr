# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)

## Phase 2: Configuration & Support Features (Completed)
### Completed Features:


## Phase 3: Component-Group Testing Framework (Completed)

## Phase 4: Core CLI Implementation
_**Goal:** Implement the core `inspect`, `override`, `validate` commands and the enhanced configuration system with a robust standalone CLI interface._

### Phase 4.0: Command Implementation
- [ ] Design and implement core command logic for `inspect`, `override`, `validate`.
  - [ ] [Sub-Task] Define shared Go structs (e.g., `ChartInfo`, `ImageAnalysis`, `OverrideResult`).
  - [ ] [Sub-Task] Implement core `inspect` logic: Load chart -> Detect images (`pkg/image`) -> Generate analysis report data structure.
  - [ ] [Sub-Task] Implement core `override` logic: Load chart -> Load config -> Detect images -> Apply path strategy (`pkg/strategy`) -> Generate override map structure (`pkg/override`).
  - [ ] [Sub-Task] Implement core `validate` logic: Load chart -> Load provided values (`--values` flag supports multiple) -> Prepare Helm command args -> Call Helm interaction layer (`internal/helm`) for `helm template` execution.
  - [ ] [Sub-Task] Add logic to core command entry points to handle `--release-name` flag (triggering `helm get values` before `helm template`).
  - [ ] [Sub-Task] Ensure backwards compatibility with previous override command during transition (TBD approach).
- [x] Implement standalone CLI interface for the three core commands.
  - [x] [Sub-Task] Set up Cobra commands: Define `inspectCmd`, `overrideCmd`, `validateCmd`.
  - [x] [Sub-Task] Define and parse flags for each command using Cobra (ensure `--values` in `validateCmd` supports multiple files).
  - [x] [Sub-Task] Implement flag validation logic.
  - [ ] [Sub-Task] Connect Cobra command `RunE` functions to core logic implementations.
  - [ ] [Sub-Task] Implement automatic chart detection from current directory.
- [ ] Enhance 'inspect' command with `--generate-config-skeleton` flag.
  - [x] [Sub-Task] Add `--generate-config-skeleton` flag to `inspectCmd`.
  - [ ] [Sub-Task] Implement skeleton generation logic: Collect source registries -> Format YAML -> Write output.

### Phase 4.1: Configuration & Chart Handling
- [ ] Enhance configuration system
  - [ ] [Sub-Task] Define Go structs for new structured config YAML.
  - [ ] [Sub-Task] Implement structured YAML parsing.
  - [ ] [Sub-Task] Implement backward compatibility: Attempt legacy flat parse if structured parse fails.
  - [ ] [Sub-Task] Add config validation functions (e.g., `isValidRegistry`).
  - [ ] [Sub-Task] Integrate config loading (default path, `--config` flag) into core logic.
  - [ ] [Sub-Task] Implement user-friendly config error messages.
- [x] Develop flexible chart source handling
  - [x] [Sub-Task] Refine `pkg/chart` loader for directory and `.tgz` paths.
  - [x] [Sub-Task] Define/refine primary struct for combined chart metadata/values.
- [ ] Enhance subchart handling within core logic
  - [ ] [Sub-Task] Verify `pkg/override` correctly uses alias info from loaded chart dependencies.
  - [ ] [Sub-Task] Add unit tests for subchart path generation logic in `pkg/override`.
  - [ ] [Sub-Task] (Optional) Add basic sanity check for generated subchart override paths.

### Phase 4.2: Reporting & Documentation
- [x] Create analysis reporting functionality for `inspect`
  - [x] [Sub-Task] Define `inspect` report data structure.
  - [x] [Sub-Task] Implement YAML formatter for `inspect` report (sole structured output format).
  - [x] [Sub-Task] Ensure report clearly lists unanalyzable images.
- [x] Create verification and testing assistance helpers (CLI flags)
  - [x] [Sub-Task] Define JSON schema for machine-readable output (TBD if needed beyond YAML).
  - [x] [Sub-Task] Implement JSON output generation for relevant commands (TBD if needed beyond YAML).
  - [x] [Sub-Task] Support test output formats for CI/CD integration (YAML primary).
- [ ] Develop core documentation and examples
  - [x] Review and finalize `USE-CASES.md`.
  - [ ] Update/Generate `docs/cli-reference.md` (reflecting YAML-only output).
  - [ ] Add basic CI/CD examples to documentation.
  - [ ] Create `TROUBLESHOOTING.md` with initial common errors.
- [ ] Finalize design decisions and remove temporary backward compatibility for config format.
  - [x] Finalize design decisions (covered in Phase 4.3 resolved items).
  - [ ] [Sub-Task] Remove legacy config parsing code path (contingent on Phase 4.1/4.3 completion).
  - [ ] [Sub-Task] Update tests and documentation to use only structured config format.

### Phase 4.3: Fixing Current Failing tests ( `make test | grep FAIL`)

The following tests are currently failing in the integration test suite:
```
--- FAIL: TestMinimalChart (0.17s)
--- FAIL: TestParentChart (0.00s)
--- FAIL: TestKubePrometheusStack (0.00s)
--- FAIL: TestComplexChartFeatures (0.01s)
    --- FAIL: TestComplexChartFeatures/simplified-prometheus-stack_with_specific_components (0.00s)
    --- FAIL: TestComplexChartFeatures/ingress-nginx_with_admission_webhook (0.00s)
--- FAIL: TestRegistryMappingFile (0.00s)
--- FAIL: TestConfigFileMappings (0.00s)
--- FAIL: TestMinimalGitImageOverride (0.00s)
```

#### Implementation Plan:

1. **Fix TestHarness and base integration test functionality**
   - [ ] [Sub-Task] Debug the `TestAnalyzeMode` flag usage in integration tests
   - [ ] [Sub-Task] Update the `TestHarness.ExecuteIRR` method to properly include the `--integration-test-mode` flag
   - [ ] [Sub-Task] Fix the registry mapping file loading in `ValidateOverrides` method
   - [ ] [Sub-Task] Verify chart path handling and add additional logging for troubleshooting

2. **Fix basic chart tests (TestMinimalChart, TestParentChart)**
   - [ ] [Sub-Task] Update test cases to work with the new CLI interface
   - [ ] [Sub-Task] Add proper chart-path handling for minimal and parent charts
   - [ ] [Sub-Task] Fix override generation and validation for these basic charts
   - [ ] [Sub-Task] Add additional debugging to identify exact failure points

3. **Fix complex chart tests (TestKubePrometheusStack, TestComplexChartFeatures)**
   - [ ] [Sub-Task] Update simplified-prometheus-stack test to match new CLI structure
   - [ ] [Sub-Task] Update ingress-nginx test to handle its specific requirements
   - [ ] [Sub-Task] Fix template validation for complex charts with subchart handling
   - [ ] [Sub-Task] Add special handling for Prometheus-specific image structures

4. **Fix mapping-related tests (TestRegistryMappingFile, TestConfigFileMappings)**
   - [ ] [Sub-Task] Update registry mapping file format and loading mechanism
   - [ ] [Sub-Task] Fix config file mapping integration with the new CLI commands
   - [ ] [Sub-Task] Update test case expectations for the new mapping implementation
   - [ ] [Sub-Task] Add detailed validation of registry mapping output

5. **Fix special case tests (TestMinimalGitImageOverride)**
   - [ ] [Sub-Task] Update Git image override to work with the updated image detection logic
   - [ ] [Sub-Task] Fix repository path generation for Git images
   - [ ] [Sub-Task] Add specific validation for the Git image override pattern

#### Implementation Approach:

1. **Identify root causes**: First, enable additional debug logging in the test harness to identify exactly where each test is failing.

2. **Fix core issues**: Address common problems in TestHarness that might affect multiple tests, like flag handling, CLI interface changes, and path resolution.

3. **Fix test by test**: Start with the simplest tests (Minimal/Parent charts) and work towards more complex ones, ensuring each works properly before moving to the next.

4. **Verify improvements**: After each fix, run individual tests with verbose logging to confirm they're working correctly before moving to the next failure.

5. **Final integration**: Once all individual tests pass, verify the entire test suite runs successfully.

### Phase 4.4: Testing Implementation
- [ ] **High Priority:** Implement comprehensive Unit/Integration tests for `pkg/analyzer` (inspect logic).
- [ ] **High Priority:** Implement Integration tests for `internal/helm` (Helm command interactions, including `get values` and `template` success/failure simulation).
- [ ] **High Priority:** Increase Unit/Integration test coverage for `pkg/override` (override generation logic).
- [x] **High Priority:** Implement Integration/E2E tests for new command flows (`inspect`, `validate`) and error handling in `cmd/irr` (verify `validate` exit codes and stderr passthrough on failure).
- [x] **Medium Priority:** Add Unit/Integration tests for `pkg/chart` (dir/tgz loading, simple alias path generation).
- [ ] **Medium Priority:** Add tests for handling both legacy and structured configuration formats.

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