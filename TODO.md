# TODO.md - Helm Image Override Implementation Plan

## Completed Phases
- Phase 1: Core Implementation & Stabilization
- Phase 2: Configuration & Support Features
- Phase 3: Component-Group Testing Framework
- Phase 5: Helm Plugin Integration (Core functionality)
- Phase 7: Test Framework Refactoring
- Phase 8: Test Corpus Expansion & Advanced Refinement
- Phase 10: Testability Improvements via Dependency Injection (Core functionality)

## Phase 5: Helm Plugin Integration - Remaining Items
_**Goal:** Implement the Helm plugin interface that wraps around the core CLI functionality._

### 5.1 Create Helm plugin architecture
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

### 5.2 Implement Helm-specific functionality
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
  - [ ] Handle private chart repository authentication via SDK
  - [ ] Respect Helm's registry authentication configuration via SDK
  - [ ] Ensure sensitive credential handling is secure
- [ ] **[P1]** Implement version compatibility checks:
  - [ ] Add plugin version compatibility checking with latest Helm version
  - [ ] Gracefully handle version mismatches with clear error messages
  - [ ] Document latest Helm version support policy

### 5.4 Update documentation for Helm plugin usage
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

### 5.5 Implement cross-cutting improvements
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
  - [ ] Handle configuration persistence across updates
    - [ ] Define configuration versioning scheme
    - [ ] Create migration path for config changes
    - [ ] Add configuration validation during updates

### 5.6 Test cases for P1 Features
- [ ] **Documentation Example Tests**
  - [ ] Verify all documented command examples
  - [ ] Test air-gapped environment scenarios
  - [ ] Validate multi-chart deployment examples
  - [ ] Test GitOps workflow examples

## Phase 6: Chart Parameter Handling & Rules System
_**Goal:** Analyze results from the brute-force solver and chart analysis to identify parameters essential for successful Helm chart deployment after applying IRR overrides. Implement an intelligent rules system that distinguishes between Deployment-Critical (Type 1) and Test/Validation-Only (Type 2) parameters._

Implementation steps:

1. **Parameter Classification & Analysis**
   - [ ] **[P1]** Create formal distinction between Type 1 (Deployment-Critical) and Type 2 (Test/Validation-Only) parameters
   - [ ] **[P1]** Analyze solver results to identify common error patterns and required parameters
   - [ ] **[P1]** Implement first high-priority Type 1 rule: Bitnami chart security bypass
     - [ ] Define tiered confidence detection system:
       - [ ] High confidence: Require multiple indicators (homepage + GitHub/image references)
       - [ ] Medium confidence: Accept single strong indicators like homepage URL
       - [ ] Fallback detection: Identify charts that fail with exit code 16 and "unrecognized containers" error
     - [ ] Implement metadata-based detection examining:
       - [ ] Chart metadata `home` field containing "bitnami.com"
       - [ ] Chart metadata `sources` containing "github.com/bitnami/charts"
       - [ ] Chart `maintainers` field referencing "Bitnami" or "Broadcom" 
       - [ ] Chart `repository` containing "bitnamicharts"
       - [ ] `annotations.images` field containing "docker.io/bitnami/" image references
       - [ ] `dependencies` section containing tags with "bitnami-common"
     - [ ] Implement `global.security.allowInsecureImages=true` insertion for detected Bitnami charts
     - [ ] Add fallback retry mechanism for charts failing with specific error signature
     - [ ] Test with representative Bitnami charts to verify detection accuracy and deployment success
   - [ ] **[P2]** Document Type 2 parameters needed for testing (e.g., `kubeVersion`)
   - [ ] Correlate errors with chart metadata (provider, dependencies, etc.)

2. **Rules Engine Implementation**
   - [ ] **[P1]** Design rule format with explicit Type 1/2 classification
     - [ ] Define structured rule format in YAML with versioning
     - [ ] Support tiered confidence levels in detection criteria
     - [ ] Include fallback detection based on error patterns
     - [ ] Allow rule actions to modify override values
   - [ ] **[P1]** Implement rule application logic in Go that adds only Type 1 parameters to override.yaml
   - [ ] **[P1]** Create configuration options to control rule application
   - [ ] **[P1]** Create test script to extract and analyze metadata from test chart corpus
     - [ ] Develop script to process Chart.yaml files from test corpus
     - [ ] Generate statistics on different chart providers based on metadata patterns
     - [ ] Produce report identifying reliable detection patterns for major providers
   - [ ] **[P2]** Add test-only parameter handling for validation (Type 2)
   - [ ] **[P2]** Implement chart grouping based on shared parameter requirements

3. **Chart Provider Detection System**
   - [ ] **[P1]** Implement metadata-based chart provider detection:
     - [ ] Bitnami chart detection (highest priority)
       - [ ] Primary: Check for "bitnami.com" in `home` field
       - [ ] Secondary: Check for "bitnami" in image references
       - [ ] Tertiary: Check for "bitnami-common" in dependency tags
       - [ ] Implement tiered confidence levels (high/medium)
       - [ ] Add fallback detection for exit code 16 errors
     - [ ] VMware/Tanzu chart detection (often similar to Bitnami)
     - [ ] Standard/common chart repositories 
     - [ ] Custom/enterprise chart detection
   - [ ] **[P1]** Create extensible detection framework for future providers
   - [ ] **[P2]** Add fallback detection based on chart internal structure and patterns

4. **Testing & Validation Framework**
   - [ ] **[P1]** Create test cases for Type 1 parameter insertion
   - [ ] **[P1]** Analyze and report statistics on detection accuracy across test chart corpus
   - [ ] **[P1]** Validate Bitnami charts deploy successfully with inserted parameters
   - [ ] **[P1]** Test fallback mechanism with intentionally undetected Bitnami charts
   - [ ] **[P2]** Implement test framework for Type 2 parameters in validation context
   - [ ] **[P2]** Measure improvement in chart validation success rate with rules system
   - [ ] **[P2]** Create automated tests for rule application logic

5. **Documentation & Maintainability**
   - [ ] **[P1]** Document the distinction between Type 1 and Type 2 parameters
   - [ ] **[P1]** Create user guide for the rules system
     - [ ] Document the tiered detection approach
     - [ ] Explain fallback detection mechanism
     - [ ] Provide examples of rule definitions
   - [ ] **[P1]** Document metadata patterns used for chart provider detection
     - [ ] Document Bitnami detection patterns with examples
     - [ ] Create reference table of metadata fields for different providers
   - [ ] **[P2]** Implement rule versioning and tracking
   - [ ] **[P2]** Create process for rule updates based on new chart testing
   - [ ] **[P2]** Add manual override capabilities for advanced users

## Phase 9: `kind` Cluster Integration Testing
_**Goal:** Implement end-to-end tests using `kind` to validate Helm plugin interactions with a live Kubernetes API and Helm release state, ensuring read-only behavior._

- [ ] **[P1]** Set up `kind` cluster testing framework:
  - [ ] Integrate `kind` cluster creation/deletion into test setup/teardown
  - [ ] Implement Helm installation within the `kind` cluster
  - [ ] Define base RBAC for read-only Helm operations
- [ ] **[P1]** Create integration tests against live Helm releases:
  - [ ] Test core `inspect`, `override`, `validate` plugin commands against charts installed in `kind`
  - [ ] Utilize Helm Go SDK for interactions within tests where applicable
- [ ] **[P1]** Verify Read-Only Operations against `kind`:
  - [ ] Configure tests to run with limited, read-only Kubernetes/Helm permissions
  - [ ] Assert that tests with limited permissions fail if write operations are attempted
  - [ ] Verify Helm release state remains unchanged after plugin execution
- [ ] **[P1]** Test compatibility with latest Helm version in `kind`:
  - [ ] Set up CI configuration to run `kind` tests with the latest Helm version
- [ ] **[P2]** Test Helm auth integration in `kind`:
  - [ ] Test with a single credential plugin
  - [ ] Focus only on essential auth features
- [ ] **[P1]** CI/CD integration for `kind` tests:
  - [ ] Set up automated CI workflow for running `kind` tests on Ubuntu LTS only
  - [ ] Configure single environment with bash shell for all CI tests
  - [ ] Implement appropriate timeouts and resource constraints
  - [ ] Add caching mechanisms for Helm charts and images to speed up test runs
- [ ] **[P2]** Test result reporting and metrics:
  - [ ] Implement structured test result output (JSON format)
  - [ ] Track metrics like test duration, success rates across different chart types
  - [ ] Generate summaries of test coverage as command-line output for bucket category identification

## Phase 10: Testability Improvements - Remaining Items
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

## Implementation Process
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

