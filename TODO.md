# TODO.md - Helm Image Override Implementation Plan

## Completed Phases
- Phase 1: Core Implementation & Stabilization
- Phase 2: Configuration & Support Features
- Phase 3: Combine Inspect and Analyze
- Phase 5: Helm Plugin Integration (Core functionality)
- Phase 7: Test Framework Refactoring
- Phase 8: Test Corpus Expansion & Advanced Refinement
- Phase 10: Testability Improvements via Dependency Injection (Core functionality)

## Phase 3: Combine Inspect and Analyze
_**Goal:** Simplify the CLI interface by consolidating image analysis functionality into the `inspect` command, making it a unified entry point for chart analysis, and remove the `analyze` command._
## Phase 4: Basic CLI Syntax Testing
_**Goal:** Ensure all CLI commands (post-Phase 3) and essential flags execute without basic parsing or validation errors. Focus on basic functionality with minimal refactoring needed._

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
   - [x] **[P1]** Create formal distinction between Type 1 (Deployment-Critical) and Type 2 (Test/Validation-Only) parameters
   - [x] **[P1]** Analyze solver results to identify common error patterns and required parameters
   - [x] **[P1]** Implement first high-priority Type 1 rule: Bitnami chart security bypass
     - [x] Define tiered confidence detection system:
       - [x] High confidence: Require multiple indicators (homepage + GitHub/image references)
       - [x] Medium confidence: Accept single strong indicators like homepage URL
       - [x] Fallback detection: Identify charts that fail with exit code 16 and "unrecognized containers" error
     - [x] Implement metadata-based detection examining:
       - [x] Chart metadata `home` field containing "bitnami.com"
       - [x] Chart metadata `sources` containing "github.com/bitnami/charts"
       - [x] Chart `maintainers` field referencing "Bitnami" or "Broadcom" 
       - [x] Chart `repository` containing "bitnamicharts"
       - [x] `annotations.images` field containing "docker.io/bitnami/" image references
       - [x] `dependencies` section containing tags with "bitnami-common"
     - [x] Implement `global.security.allowInsecureImages=true` insertion for detected Bitnami charts
       - [x] Add this parameter during the override generation phase (`irr override` command)
       - [x] Ensure this parameter is included in the final override.yaml file
       - [x] Update override generation logic to inject this parameter automatically
     - [x] Test with representative Bitnami charts to verify detection accuracy and deployment success
       - [x] Test specifically with bitnami/nginx, bitnami/memcached and other common Bitnami charts
       - [x] Verify the override file contains the security bypass parameter
       - [x] Confirm validation passes when the parameter is included
     - [x] Add fallback retry mechanism for charts failing with specific error signature
       - [x] Detect exact exit code 16 with error message containing "Original containers have been substituted for unrecognized ones"
       - [x] Add specific error handling for the message "If you are sure you want to proceed with non-standard containers..."
       - [x] If validation fails with this specific error, retry with `global.security.allowInsecureImages=true`
   - [x] **[P2]** Document Type 2 parameters needed for testing (e.g., `kubeVersion`)
   - [x] Correlate errors with chart metadata (provider, dependencies, etc.)

2. **Rules Engine Implementation**
   - [x] **[P1]** Design rule format with explicit Type 1/2 classification
     - [x] Define structured rule format in YAML with versioning
     - [x] Support tiered confidence levels in detection criteria
     - [x] Include fallback detection based on error patterns
     - [x] Allow rule actions to modify override values
   - [x] **[P1]** Implement rule application logic in Go that adds only Type 1 parameters to override.yaml
   - [x] **[P1]** Create configuration options to control rule application
   - [ ] **[P1]** Create test script to extract and analyze metadata from test chart corpus
     - [ ] Develop script to process Chart.yaml files from test corpus
     - [Note: Reconfirming - The existing test/tools/test-charts.py script processes the test chart corpus and WILL be adapted or leveraged for this analysis. No new script will be created.]
     - [ ] Generate statistics on different chart providers based on metadata patterns
     - [ ] Produce report identifying reliable detection patterns for major providers
   - [x] **[P2]** Add test-only parameter handling for validation (Type 2)
   - [ ] **[P2]** Implement chart grouping based on shared parameter requirements
   - [ ] **[P1]** Enhance log output for applied rules:
     - [ ] When a rule adds a Type 1 parameter (e.g., Bitnami rule adding `global.security.allowInsecureImages`), log an `INFO` message.
     - [ ] Log message should include: Rule Name, Parameter Path/Value, Brief Reason/Description, and Reference URL (e.g., Bitnami GitHub issue).
     - [ ] Implement by adding logging logic within the rule application function (e.g., `ApplyRulesToMap`).
     - [ ] Consider adding a `ReferenceURL` or `Explanation` field/method to the `Rule` interface for better context sourcing.
     - [ ] Update tests (e.g., integration tests) to assert the presence and content of this log message.
     - [ ] Update documentation (`docs/RULES.md`) to mention this log output.
     - [ ] Ensure `README.md` prominently links to `docs/RULES.md`.

3. **Chart Provider Detection System**
   - [x] **[P1]** Implement metadata-based chart provider detection:
     - [x] Bitnami chart detection (highest priority)
       - [x] Primary: Check for "bitnami.com" in `home` field
       - [x] Secondary: Check for "bitnami" in image references
       - [x] Tertiary: Check for "bitnami-common" in dependency tags
       - [x] Implement tiered confidence levels (high/medium)
       - [ ] Add fallback detection for exit code 16 errors
     - [ ] VMware/Tanzu chart detection (often similar to Bitnami)
     - [ ] Standard/common chart repositories 
     - [ ] Custom/enterprise chart detection
   - [x] **[P1]** Create extensible detection framework for future providers
   - [ ] **[P2]** Add fallback detection based on chart internal structure and patterns

4. **Testing & Validation Framework**
   - [x] **[P1]** Create test cases for Type 1 parameter insertion
   - [ ] **[P1]** Analyze and report statistics on detection accuracy across test chart corpus
   - [ ] **[P1]** Validate Bitnami charts deploy successfully with inserted parameters
   - [ ] **[P1]** Test fallback mechanism with intentionally undetected Bitnami charts
   - [x] **[P2]** Implement test framework for Type 2 parameters in validation context
   - [ ] **[P2]** Measure improvement in chart validation success rate with rules system
   - [x] **[P2]** Create automated tests for rule application logic

   # Prioritized Additional Test Cases for Rules System
   - [ ] **Integration (Core + Disable):** Run `irr override` on real Bitnami charts (e.g., nginx) and non-Bitnami charts to verify:
     - [ ] Override file contains `global.security.allowInsecureImages: true` only for detected Bitnami charts (medium/high confidence)
     - [ ] The CLI flag `--disable-rules` prevents rule application
   - [ ] **Unit Coverage (Detection & Application):**
     - [ ] Verify core detection logic handles different metadata combinations correctly (covering confidence levels)
     - [ ] Test rule application logic merges parameters correctly into simple and complex existing override maps
     - [ ] Include checks for case sensitivity and whitespace variations in metadata
   - [ ] **Type 2 Exclusion:** Validate that parameters from rules marked as `TypeValidationOnly` are never included in the final override map (can use a dummy rule for testing)
   - [ ] **Negative/Edge (No Metadata):** Test charts with empty/nil metadata; ensure no panics occur and no rules are applied
   - [ ] **Error Handling:** Unit test graceful failure (log warning, continue) if the rules registry is misconfigured or type assertion fails

5. **Documentation & Maintainability**
   - [x] **[P1]** Document the distinction between Type 1 and Type 2 parameters
   - [x] **[P1]** Create user guide for the rules system
     - [x] Document the tiered detection approach
     - [ ] Explain fallback detection mechanism
     - [x] Provide examples of rule definitions
   - [x] **[P1]** Document metadata patterns used for chart provider detection
     - [x] Document Bitnami detection patterns with examples
     - [x] Create reference table of metadata fields for different providers
   - [ ] **[P2]** Implement rule versioning and tracking
   - [ ] **[P2]** Create process for rule updates based on new chart testing
   - [x] **[P2]** Add manual override capabilities for advanced users

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
  - [ ] Set up CI configuration to run `