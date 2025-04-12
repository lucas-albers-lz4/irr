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
- [ ] Implement standalone CLI interface for the three core commands.
  - [ ] [Sub-Task] Set up Cobra commands: Define `inspectCmd`, `overrideCmd`, `validateCmd`.
  - [ ] [Sub-Task] Define and parse flags for each command using Cobra (ensure `--values` in `validateCmd` supports multiple files).
  - [ ] [Sub-Task] Implement flag validation logic.
  - [ ] [Sub-Task] Connect Cobra command `RunE` functions to core logic implementations.
  - [ ] [Sub-Task] Implement automatic chart detection from current directory.
- [ ] Enhance 'inspect' command with `--generate-config-skeleton` flag.
  - [ ] [Sub-Task] Add `--generate-config-skeleton` flag to `inspectCmd`.
  - [ ] [Sub-Task] Implement skeleton generation logic: Collect source registries -> Format YAML -> Write output.

### Phase 4.1: Configuration & Chart Handling
- [ ] Enhance configuration system
  - [ ] [Sub-Task] Define Go structs for new structured config YAML.
  - [ ] [Sub-Task] Implement structured YAML parsing.
  - [ ] [Sub-Task] Implement backward compatibility: Attempt legacy flat parse if structured parse fails.
  - [ ] [Sub-Task] Add config validation functions (e.g., `isValidRegistry`).
  - [ ] [Sub-Task] Integrate config loading (default path, `--config` flag) into core logic.
  - [ ] [Sub-Task] Implement user-friendly config error messages.
- [ ] Develop flexible chart source handling
  - [ ] [Sub-Task] Refine `pkg/chart` loader for directory and `.tgz` paths.
  - [ ] [Sub-Task] Define/refine primary struct for combined chart metadata/values.
- [ ] Enhance subchart handling within core logic
  - [ ] [Sub-Task] Verify `pkg/override` correctly uses alias info from loaded chart dependencies.
  - [ ] [Sub-Task] Add unit tests for subchart path generation logic in `pkg/override`.
  - [ ] [Sub-Task] (Optional) Add basic sanity check for generated subchart override paths.

### Phase 4.2: Reporting & Documentation
- [ ] Create analysis reporting functionality for `inspect`
  - [ ] [Sub-Task] Define `inspect` report data structure.
  - [ ] [Sub-Task] Implement YAML formatter for `inspect` report (sole structured output format).
  - [ ] [Sub-Task] Ensure report clearly lists unanalyzable images.
- [ ] Create verification and testing assistance helpers (CLI flags)
  - [ ] [Sub-Task] Define JSON schema for machine-readable output (TBD if needed beyond YAML).
  - [ ] [Sub-Task] Implement JSON output generation for relevant commands (TBD if needed beyond YAML).
  - [ ] [Sub-Task] Support test output formats for CI/CD integration (YAML primary).
- [ ] Develop core documentation and examples
  - [x] Review and finalize `USE-CASES.md`.
  - [ ] Update/Generate `docs/cli-reference.md` (reflecting YAML-only output).
  - [ ] Add basic CI/CD examples to documentation.
  - [ ] Create `TROUBLESHOOTING.md` with initial common errors.
- [ ] Finalize design decisions and remove temporary backward compatibility for config format.
  - [x] Finalize design decisions (covered in Phase 4.3 resolved items).
  - [ ] [Sub-Task] Remove legacy config parsing code path (contingent on Phase 4.1/4.3 completion).
  - [ ] [Sub-Task] Update tests and documentation to use only structured config format.

### Phase 4.3: Testing Implementation
- [ ] **High Priority:** Implement comprehensive Unit/Integration tests for `pkg/analyzer` (inspect logic).
- [ ] **High Priority:** Implement Integration tests for `internal/helm` (Helm command interactions, including `get values` and `template` success/failure simulation).
- [ ] **High Priority:** Increase Unit/Integration test coverage for `pkg/override` (override generation logic).
- [ ] **High Priority:** Implement Integration/E2E tests for new command flows (`inspect`, `validate`) and error handling in `cmd/irr` (verify `validate` exit codes and stderr passthrough on failure).
- [ ] **Medium Priority:** Add Unit/Integration tests for `pkg/chart` (dir/tgz loading, simple alias path generation).
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
     - Review and execute the git commit commands yourself