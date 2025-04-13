# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)

## Phase 2: Configuration & Support Features (Completed)
### Completed Features:

## Phase 3: Component-Group Testing Framework (Completed)


## Phase 5: Helm Plugin Integration
_**Goal:** Implement the Helm plugin interface that wraps around the core CLI functionality._

- [x] Create Helm plugin architecture
  - [x] Design plugin structure that wraps around core command logic.
  - [x] Implement plugin installation process and discovery.
  - [x] Ensure consistent command-line interface with standalone CLI.
  - [x] **[P1]** Enhance error handling in plugin wrapper script:
    - [x] Add robust error handling with proper exit codes
    - [x] Implement command timeouts
    - [x] Format error output consistently, ideally matching Helm's CLI style for user familiarity
  - [ ] **[P2]** Improve plugin security:
    - [ ] Implement proper filesystem permissions model for the installed plugin binary and cache directories
      - [ ] Add platform-specific permission settings (0755 for binaries, 0644 for configs)
      - [ ] Verify permissions are correctly set after installation
    - [ ] Add plugin version validation against Helm version
      - [ ] Implement version check against latest Helm version during plugin initialization
    - [ ] Use standard GitHub mechanisms for security
      - [ ] Rely on GitHub's release asset checksums and HTTPS
      - [ ] Document release verification process for users
  - [ ] **[P1]** Create proper plugin distribution package:
    - [ ] Set up versioning for plugin releases
      - [ ] Define semantic versioning strategy (MAJOR.MINOR.PATCH)
      - [ ] Create version bumping automation (via GitHub Actions)
      - [ ] Ensure version is embedded in binary at build time
    - [ ] Create basic release automation (e.g., via Makefile/GitHub Actions)
      - [ ] Set up GitHub Actions workflow for release creation on tags
      - [ ] Automate binary building for supported platforms:
        - [ ] Linux AMD64
        - [ ] Linux ARM64
        - [ ] macOS ARM64
      - [ ] Generate checksums for all artifacts
      - [ ] Use standard GitHub release mechanisms for publishing
    - [ ] We have a working code work flow to do this that we will copy from another project, so we can skip this portion for now until we do it.
    - [ ] For now we can just plugin install from local location.

- [x] Implement Helm-specific functionality
  - [x] Add release name resolution to chart path.
  - [x] Add Helm environment integration for configuration and auth.
  - [x] **[P1]** Refactor plugin to use Helm Go SDK instead of shelling out:
    - [x] Replace `exec.Command("helm", ...)` calls with Go SDK equivalents (`pkg/action`, `pkg/cli`)
    - [x] Audit remaining `exec.Command` calls to ensure they are necessary or replace them
      - [x] Create inventory of remaining exec calls
      - [x] Classify each by replaceability with SDK alternatives
      - [x] Prioritize replacements by risk/impact
    - [x] Use SDK for getting release info, values, and pulling charts
    - [x] **Ensure only read-only Helm actions are used:**
      - [x] *Allowed Read Actions:* `Get`, `GetValues`, `List`, `SearchRepo`, `SearchIndex`, `Pull` (for fetching chart data only), loading charts/values (`loader.Load`, `chartutil`), reading config (`cli.New`, `repo.LoadFile`).
      - [x] *Disallowed Write Actions:* `Install`, `Upgrade`, `Uninstall`, `Rollback`, `Push`, `RepoAdd`, `RepoRemove`, `RepoUpdate`, or any direct modification of Kubernetes resources via the SDK's client.
      - [x] *Rationale:* IRR's purpose is to *generate* overrides, not apply changes or modify Helm state.
    - [x] Fix namespace handling in Helm template command
    - [x] Fix dependency issues to build with Helm SDK
    - [x] Add SDK integration to `inspect` and `validate` commands
      - [x] Replace command-line invocations with direct SDK calls
      - [x] Ensure error handling matches SDK error patterns
      - [x] Maintain backward compatibility with current output format
    - [x] Improve robustness and testability
      - [x] Add timeout handling for SDK operations
      - [x] Implement retry logic for transient failures
    - [x] Fix Helm SDK imports and build errors
      - [x] Replace old Helm v2 imports with Helm v3 equivalents
      - [x] Update code to use correct Helm v3 package paths
      - [x] Fix build errors related to missing or incorrect imports
  - [ ] **[P1]** Enhance Helm integration:
    - [ ] Add support for automatically detecting configured Helm repositories via SDK **(Post-Release Feature)**
      - [ ] Access repository config via Helm SDK
      - [ ] Implement caching of repository data
      - [ ] Support custom repository configurations
    - [ ] **[P2]** Implement Helm hooks support for pre/post operations **(Post-Release Feature)**
      - [ ] Define hook interface and discovery mechanism
      - [ ] Create hook execution engine with proper error handling
      - [ ] Implement hook timeout handling
    - [ ] **[P2]** Define clear hook execution flow and environment variables available to hooks **(Post-Release Feature)**
      - [ ] Document hook environment variables (HELM_PLUGIN_*, CHART_*, RELEASE_*)
      - [ ] Create standard hook exit code handling
      - [ ] Define hook execution order guarantees
    - [ ] **[P2]** Specifically support `pre-override` and `post-override` hooks for user customizations **(Post-Release Feature)**
      - [ ] Implement hook discovery in standard locations
      - [ ] Pass appropriate context variables to hooks
      - [ ] Allow hooks to modify override process
    - [ ] **[P2]** Add documentation on creating custom hook scripts **(Post-Release Feature)**
      - [ ] Create sample hooks for common use cases
      - [ ] Document best practices for hook development
    - [ ] **[P2]** Add Helm template debugging support **(Post-Release Feature)**
      - [ ] Create debug output format matching Helm's
      - [ ] Implement verbose logging of template operations
    - [ ] **[P2]** Integrate with Helm's `--debug` flag or provide similar functionality **(Post-Release Feature)**
      - [ ] Match Helm's debug output format and verbosity
      - [ ] Add detailed SDK operation logging
  - [ ] **[P2]** Add Helm auth integration: **(Future Feature - Not part of current development work - Post-Release Feature)**
    - [ ] Support Helm credential plugins via SDK
      - [ ] Identify SDK interfaces for credential plugins
      - [ ] Test with common credential plugins (AWS, Azure, GCP)
      - [ ] Document supported credential plugin types
    - [ ] Handle private chart repository authentication via SDK
      - [ ] Test with common private repo types (Harbor, Nexus, etc.)
      - [ ] Add support for basic auth, token auth, and OAuth
    - [ ] Respect Helm's registry authentication configuration via SDK
      - [ ] Support OCI registry authentication methods
      - [ ] Test with popular container registries
    - [ ] Ensure sensitive credential handling is secure
      - [ ] Audit credential usage for leaks
      - [ ] Use secure environment variables where possible
      - [ ] Avoid logging sensitive information
  - [ ] **[P1]** Implement version compatibility checks:
    - [ ] Add plugin version compatibility checking with latest Helm version
      - [ ] Create version detection for latest Helm version
    - [ ] Gracefully handle version mismatches with clear error messages
      - [ ] Create user-friendly error messages when outdated Helm version detected
    - [ ] Document latest Helm version support policy
      - [ ] Clearly state that only the latest Helm version is supported

- [x] Develop Helm plugin testing
  - [x] Implement basic E2E tests for core Helm Plugin workflows (happy path `inspect`, `override`, `validate`).
  - [x] Test plugin installation and registration.
  - [x] Verify Helm release interaction.
  - [ ] **[P1]** Expand test coverage:
    - [ ] **Note:** Focus tests on Helm integration points (install, SDK interactions, release handling) and avoid duplicating core logic tests covered elsewhere.
    - [ ] Implement plugin installation/uninstallation testing (scripting based)
      - [ ] Create test fixtures for different installation scenarios
      - [ ] Test installation idempotency
      - [ ] Test clean uninstall/reinstall (no persistent configuration expected between versions)
    - [ ] **[P2]** Test Helm SDK interactions with mocked SDK interfaces where appropriate **(Post-Release Feature)**
      - [ ] Create SDK interface mocks for testing
      - [ ] Test error handling scenarios with mocked errors
      - [ ] Add negative test cases for all SDK interactions
  - [ ] **[P1]** Add chart variety tests:
    - [ ] Test with a small focused set of charts that cover key image patterns
    - [ ] Test with one complex chart (kube-prometheus-stack) as representative of deeply nested charts
    - [ ] Focus on confirming basic functionality rather than exhaustive coverage
  - [ ] **[P1]** Implement failure mode testing (deterministic):
    - [ ] Test handling of basic error cases:
      - [ ] One invalid chart format test case 
      - [ ] One connectivity issue simulation
    - [ ] Verify appropriate error codes and messages for critical failures only

- [x] Update documentation for Helm plugin usage
  - [x] Add Helm-specific examples and workflows.
  - [x] Document plugin installation and configuration.
  - [ ] **[P1]** Enhance user documentation:
    - [ ] Add examples for complex scenarios
      - [ ] Multi-chart deployments
      - [ ] Air-gapped environments
      - [ ] Custom registry setups
    - [ ] Include troubleshooting guide specific to plugin usage
      - [ ] Common error scenarios and resolutions
      - [ ] Diagnostic procedures for plugin issues
      - [ ] Environment setup troubleshooting
    - [ ] **[P2]** Add FAQ section based on common issues **(Post-Release Feature)**
      - [ ] Collect issues from GitHub and community feedback
      - [ ] Provide clear, actionable answers
    - [ ] Clearly document the read-only nature and security implications
      - [ ] Explain RBAC requirements
      - [ ] Document security best practices
  - [ ] **[P1]** Improve integration documentation:
    - [ ] Document CI/CD integration
    - [ ] Add examples for GitOps workflows
  - [ ] **[P2]** We only support cross-platform macos/linux
    - [ ] Document official support for macOS and Ubuntu LTS only
    - [ ] Explicitly state that bash is the only supported shell, even though macOS defaults to zsh
    - [ ] Add instructions for macOS users to run commands in bash instead of zsh
    - [ ] Note that other environments may work but are not tested or supported

- [ ] Implement cross-cutting improvements
  - [ ] **[P2]** Only support platform macos/linux support:
    - [ ] Test only with bash on macOS (development environment)
    - [ ] Ensure scripts specify #!/bin/bash rather than relying on default shell
    - [ ] Include note in development docs about using bash explicitly on macOS
    - [ ] Test only with bash on Ubuntu LTS (CI environment)
    - [ ] Document specific supported environment limitations
    - [ ] Handle basic path differences between macOS and Ubuntu
      - [ ] Focus on standard installation locations only
  - [ ] **[P1]** Improve plugin lifecycle management:
    - [ ] Add proper uninstallation support
      - [ ] Create uninstall script that cleans up all artifacts
      - [ ] Handle configuration preservation options
      - [ ] Test uninstall/reinstall scenarios
    - [ ] Note: Plugin updates will rely on the standard `helm plugin update` command
    - [ ] Handle configuration persistence across updates
      - [ ] Define configuration versioning scheme
      - [ ] Create migration path for config changes
      - [ ] Add configuration validation during updates

## Phase 7: Test Framework Refactoring
_**Goal:** Refactor the Python-based `test-charts.py` framework to use the new Phase 4/5 commands with the existing chart corpus._

- [ ] Refactor `test-charts.py` script
  - Update script to call `irr inspect`, `irr override`, and `irr validate` commands using the finalized CLI.
  - Adapt failure categorization and reporting for the new command structure.
  - Ensure script handles the new structured configuration format.
- [ ] Analyze Failure Patterns (on existing corpus)
  - Use the refactored script's output (error reports, patterns) to identify common failure reasons.
  - Leverage test-charts.py's data collection and analysis to drive improvements.
  - Note: test-charts.py generates structured JSON data critical for identifying pattern-based improvements.
- [ ] Improve Values Templates ("Bucket Solvers") (on existing corpus)
  - Iteratively refine the minimal values templates (`VALUES_TEMPLATE_...`) based on failure analysis to increase the success rate.
  - Consider adding new classifications/templates if warranted.
- [ ] Generate Intermediate Compatibility Report
  - Document the chart compatibility success rate (not version compatibility) achieved after refactoring and initial template improvements.
  - Note: Compatibility here refers to how many charts our tool successfully processes, not version compatibility between different Helm versions.

## Phase 8: Test Corpus Expansion & Advanced Refinement
_**Goal:** Expand the chart test set significantly and further refine compatibility based on broader testing._

- [ ] Expand Test Chart Corpus
  - Increase the number and variety of charts tested (e.g., Top 200+, community requests, specific complex charts).
  - Update chart pulling/caching mechanism if necessary.
- [ ] Re-Analyze Failure Patterns (on expanded corpus)
  - Identify new or more prevalent failure reasons with the larger chart set.
  - Leverage test-charts.py's data collection and analysis to drive improvements.
  - Note: test-charts.py generates structured JSON data critical for identifying pattern-based improvements.
- [ ] Further Improve Values Templates
  - Make additional refinements to templates based on the expanded analysis.
  - Use test-charts.py's bucket categorization to determine minimal configuration templates that maximize chart compatibility.
- [ ] Generate Final Compatibility Report
  - Document the final chart compatibility success rate across the expanded corpus.
  - Focus on command-line reports that directly drive accuracy improvements and help prioritize future work.
  - Note: This measures our tool's ability to successfully process charts, not version compatibility.

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
- [ ] **[P1]** Test compatibility with latest Helm version in `kind`:
  - Set up CI configuration to run `kind` tests with the latest Helm version.
- [ ] **[P2]** Test Helm auth integration in `kind`:
  - Test with a single credential plugin (preferably one used in our environment)
  - Focus only on essential auth features that directly impact our functionality
- [ ] **[P1]** CI/CD integration for `kind` tests:
  - Set up automated CI workflow for running `kind` tests on Ubuntu LTS only
  - Configure single environment with bash shell for all CI tests
  - Implement appropriate timeouts and resource constraints
  - Add caching mechanisms for Helm charts and images to speed up test runs
- [ ] **[P2]** Test result reporting and metrics:
  - [ ] Implement structured test result output (JSON format)
  - [ ] Track metrics like test duration, success rates across different chart types
  - [ ] Generate summaries of test coverage as command-line output for bucket category identification

## Phase 10: Testability Improvements via Dependency Injection
_**Goal:** Improve testability of complex logic by refactoring key components to use dependency injection patterns, enabling isolated unit testing without extensive mocking frameworks._

- [ ] **[P1]** Identify and refactor external service integrations:
  - [ ] Analyze code base for difficult-to-test integrations with external services, particularly:
    - Helm SDK function calls (e.g., 


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