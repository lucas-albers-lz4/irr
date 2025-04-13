# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)

## Phase 2: Configuration & Support Features (Completed)
### Completed Features:

## Phase 3: Component-Group Testing Framework (Completed)


## Phase 5: Helm Plugin Integration
_**Goal:** Implement the Helm plugin interface that wraps around the core CLI functionality._

- [x] Create Helm plugin architecture
  - Design plugin structure that wraps around core command logic.
  - Implement plugin installation process and discovery.
  - Ensure consistent command-line interface with standalone CLI.
  - [x] **[P1]** Enhance error handling in plugin wrapper script:
    - Add robust error handling with proper exit codes
    - Implement command timeouts
    - Format error output consistently, ideally matching Helm's CLI style for user familiarity
  - [ ] **[P2]** Improve plugin security:
    - Add checksum verification for binary downloads/updates
    - Implement proper filesystem permissions model for the installed plugin binary and cache directories
    - Add plugin version validation against Helm version
  - [ ] **[P1]** Create proper plugin distribution package:
    - Set up versioning for plugin releases
    - Add basic update mechanism
    - Create basic release automation (e.g., via Makefile/GitHub Actions)
    - We have a working code work flow to do this that we will copy from another project, so we can skip this portion for now until we do it.
    - For now we can just plugin install from local location.

- [x] Implement Helm-specific functionality
  - Add release name resolution to chart path.
  - Add Helm environment integration for configuration and auth.
  - [x] **[P1]** Refactor plugin to use Helm Go SDK instead of shelling out:
    - [x] Replace `exec.Command("helm", ...)` calls with Go SDK equivalents (`pkg/action`, `pkg/cli`)
    - [x] Use SDK for getting release info, values, and pulling charts
    - [x] **Ensure only read-only Helm actions are used:**
      - *Allowed Read Actions:* `Get`, `GetValues`, `List`, `SearchRepo`, `SearchIndex`, `Pull` (for fetching chart data only), loading charts/values (`loader.Load`, `chartutil`), reading config (`cli.New`, `repo.LoadFile`).
      - *Disallowed Write Actions:* `Install`, `Upgrade`, `Uninstall`, `Rollback`, `Push`, `RepoAdd`, `RepoRemove`, `RepoUpdate`, or any direct modification of Kubernetes resources via the SDK's client.
      - *Rationale:* IRR's purpose is to *generate* overrides, not apply changes or modify Helm state.
    - [x] Fix namespace handling in Helm template command
    - [x] Fix dependency issues to build with Helm SDK
    - [ ] Add SDK integration to `inspect` and `validate` commands
    - [ ] Improve robustness, testability, and performance
  - [ ] **[P1]** Enhance Helm integration:
    - Add support for automatically detecting configured Helm repositories via SDK
    - Implement Helm hooks support for pre/post operations
      - Specifically support `pre-override` and `post-override` hooks for user customizations
      - Add documentation on creating custom hook scripts
    - Add Helm template debugging support
  - [ ] **[P1]** Add Helm auth integration:
    - Support Helm credential plugins via SDK
    - Handle private chart repository authentication via SDK
    - Respect Helm's registry authentication configuration via SDK
  - [ ] **[P1]** Implement version compatibility checks:
    - Add plugin version compatibility checking with Helm versions
    - Gracefully handle version mismatches with clear error messages
    - Document supported Helm version ranges

- [x] Develop Helm plugin testing
  - Implement basic E2E tests for core Helm Plugin workflows (happy path `inspect`, `override`, `validate`).
  - Test plugin installation and registration.
  - Verify Helm release interaction.
  - [ ] **[P1]** Expand test coverage:
    - **Note:** Focus tests on Helm integration points (install, SDK interactions, release handling) and avoid duplicating core logic tests covered elsewhere.
    - Implement plugin installation/uninstallation testing (scripting based)
  - [ ] **[P1]** Add chart variety tests:
    - Test with charts using various image patterns (leverage existing test charts where applicable)
    - Test with charts of different complexity levels (leverage existing test charts where applicable)
    - Test with charts using custom template functions
    - Test with deeply nested subcharts
  - [ ] **[P1]** Implement failure mode testing (deterministic):
    - Test graceful handling of invalid charts (e.g., bad format)
    - Test handling of errors returned from mocked Helm Go SDK calls (simulating network/API issues)
    - Verify correct error messages and exit codes are returned to the user

- [x] Update documentation for Helm plugin usage
  - Add Helm-specific examples and workflows.
  - Document plugin installation and configuration.
  - [ ] **[P1]** Enhance user documentation:
    - Add examples for complex scenarios
    - Include troubleshooting guide specific to plugin usage
    - Add FAQ section based on common issues
  - [ ] **[P1]** Improve integration documentation:
    - Document CI/CD integration
    - Add examples for GitOps workflows
  - [ ] **[P2]** We only support cross-platform macos/linux
    - document what we support.

- [ ] Implement cross-cutting improvements
  - [ ] **[P2]** Only support platform macos/linux support:
    - Test with standard shell environments (bash, zsh, )
    - Handle path differences across platforms
  - [ ] **[P1]** Improve plugin lifecycle management:
    - Add proper uninstallation support
    - Implement clean update procedures
    - Handle configuration persistence across updates

## Phase 7: Test Framework Refactoring
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

## Phase 8: Test Corpus Expansion & Advanced Refinement
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

## Phase 9: `kind` Cluster Integration Testing
_**Goal:** Implement end-to-end tests using `kind` to validate Helm plugin interactions with a live Kubernetes API and Helm release state, ensuring read-only behavior._

- [ ] **[P1]** Set up `kind` cluster testing framework:
  - Integrate `kind` cluster creation/deletion into test setup/teardown (Makefile or Go test suite).
  - Implement Helm installation within the `kind` cluster.
  - Define base RBAC for read-only Helm operations.
- [ ] **[P1]** Create integration tests against live Helm releases:
  - Test core `inspect`, `override`, `validate` plugin commands against charts installed in `kind`.
  - Utilize Helm Go SDK for interactions within tests where applicable.
- [ ] **[P1]** Verify Read-Only Operations against `kind`:
  - Configure tests to run with limited, read-only Kubernetes/Helm permissions via specific ServiceAccount/kubeconfig.
  - Assert that tests using limited permissions fail if any write operations are attempted.
  - Verify Helm release state (e.g., revision count, status) remains unchanged after plugin execution.
- [ ] **[P1]** Test compatibility with relevant Helm versions in `kind`:
  - Set up CI matrix or test configurations to run `kind` tests against different Helm 3.x versions.
- [ ] **[P1]** Test Helm auth integration in `kind`:
  - Test interactions with Helm credential plugins within the test environment.
  - Test against private chart repositories requiring authentication (potentially using a local chart museum instance).
- [ ] **[P2]** Performance and resource testing:
  - Measure plugin performance metrics (execution time, memory usage) in realistic environments
  - Test with various sized charts and releases to establish performance baselines
  - Document resource requirements and performance characteristics
- [ ] **[P1]** CI/CD integration for `kind` tests:
  - Set up automated CI workflows for running `kind` tests
  - Implement appropriate timeouts and resource constraints
  - Add caching mechanisms for Helm charts and images to speed up test runs
- [ ] **[P2]** Test result reporting and metrics:
  - Implement structured test result output (JSON format)
  - Track metrics like test duration, success rates across different chart types
  - Generate visual reports of test coverage and performance data

## Phase 10: Testability Improvements via Dependency Injection
_**Goal:** Improve testability of complex logic by refactoring key components to use dependency injection patterns, enabling isolated unit testing without extensive mocking frameworks._

- [ ] **[P1]** Identify and refactor external service integrations:
  - [ ] Analyze code base for difficult-to-test integrations with external services, particularly:
    - Helm SDK function calls (e.g., `Template`, `ValidateChart`, `GetValues`)
    - Filesystem operations
    - Network calls
    - Subprocess executions
  - [ ] Categorize functions by testability impact and complexity
  - [ ] Prioritize high-impact functions that currently impede test coverage
  
- [ ] **[P1]** Implement variable-based dependency injection:
  - [ ] Refactor external calls to use package or struct-level variables for functions:
    ```go
    // Before
    func DoSomething() {
        result := helm.Template(...)
    }
    
    // After 
    var templateFunc = helm.Template
    
    func DoSomething() {
        result := templateFunc(...)
    }
    ```
  - [ ] Add appropriate documentation for each injected dependency
  - [ ] Ensure backward compatibility during refactoring
  - [ ] Define consistent naming patterns for injected functions (e.g., `xxxFunc` suffix)

- [ ] **[P1]** First phase target functions for DI refactoring:
  - [ ] `cmd/irr/override.go`: Refactor `helm.Template` calls to use variable injection
  - [ ] `cmd/irr/validate.go`: Refactor `helm.ValidateChart` to allow test replacement
  - [ ] `cmd/irr/chart.go`: Implement injection for chart loading operations
  - [ ] `internal/helm/command.go`: Add injection points for underlying Helm SDK calls
  - [ ] `internal/generator`: Add test hooks for filesystem operations

- [ ] **[P1]** Implement comprehensive test coverage:
  - [ ] Create unit tests for each refactored component that leverage function replacement
  - [ ] Implement both success and failure test cases
  - [ ] Include edge cases and error conditions
  - [ ] Create reusable test utilities for common mock scenarios

- [ ] **[P2]** Develop testing guidelines for dependency injection:
  - [ ] Document standard patterns for using the dependency injection hooks
  - [ ] Create examples of proper test setup and teardown with injected dependencies
  - [ ] Define testing anti-patterns to avoid
  - [ ] Add guidance for when to use DI vs. other mocking approaches

- [ ] **[P1]** Implement CI verification of test coverage:
  - [ ] Set coverage thresholds for refactored components
  - [ ] Add CI steps to verify coverage meets thresholds
  - [ ] Generate and publish test coverage reports

- [ ] **[P2]** Balance production code and test code:
  - [ ] Follow "minimal impact to production code" principle
  - [ ] Ensure production code remains readable and maintainable
  - [ ] Favor simple dependency injection over complex test frameworks
  - [ ] Document rationale for each injection point

## Implementation Process: DONT REMOVE THIS SECTION as these hints are important to remember.
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
  
  4. **Post-Change Verification:**
     - Run targeted tests to verify the changes work as expected ✓
     - Run targeted linting to confirm specific issues are resolved ✓
     - Run full test suite: `go test ./...` ✓
     - Run full linting: `golangci-lint run` ✓
  
  5. **Git Commit:**
     - Stop after completing a logical portion of a feature to make well reasoned git commits with changes and comments ✓
     - Request suggested git commands for committing the changes ✓
     - Review and execute the git commit commands yourself, never change git branches stay in the branch you are in until feature completion ✓