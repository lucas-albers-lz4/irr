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

- [x] **[P1]** Enhance `inspect` command (`cmd/irr/inspect.go`):
    - [x] Add `--source-registries` as an *optional* string slice flag.
    - [x] Modify flag retrieval logic (`getInspectFlags` or similar) to handle the optional flag.
    - [x] Update core `runInspect` logic: If `--source-registries` is provided and not empty, filter reported images to match those registries. Otherwise, report all images.

- [x] **[P1]** Remove `analyze` command completely:
    - [x] Delete `cmd/irr/analyze.go` file.
    - [x] Remove `analyzeCmd` addition from `cmd/irr/root.go` (or equivalent).

- [x] **[P1]** Update Go Tests (`cmd/irr/cli_test.go` or similar):
    - [x] Identify all existing tests for the `analyze` command.
    - [x] **Adapt/Reuse** tests verifying `analyze` execution and output filtering: Modify them to target `inspect` *with* the `--source-registries` flag.
    - [x] **Adapt/Reuse** tests for `analyze` with optional flags (e.g., `--output-file`): Modify them to target `inspect` *with* `--source-registries`.
    - [x] **Adapt/Reuse** error tests for invalid registries/paths: Modify them to target `inspect`.
    - [x] **Delete** tests checking for *missing* required `--source-registries` on `analyze` (no longer relevant).
    - [x] **Add new** tests for `inspect` *without* `--source-registries` to verify it reports all images.

- [x] **[P1]** Update Python Test Framework (`test/tools/test-charts.py`):
    - [x] Replace calls to `irr analyze ... --source-registries <regs>` with `irr inspect ... --source-registries <regs>`.
    - [x] Verify any result parsing logic still works correctly.

- [x] **[P1]** Update Documentation:
    - [x] Remove `analyze` section from `docs/cli-reference.md`.
    - [x] Update `inspect` description in `docs/cli-reference.md` to include optional `--source-registries` filtering.
    - [x] Update/add examples in `docs/cli-reference.md` and `docs/USE-CASES.md` showing `inspect` with/without `--source-registries`.
    - [x] Remove `analyze` references from `docs/DEVELOPMENT.md` and other relevant docs.

- [x] **[P2]** Streamline flags across commands (Review during implementation):
    - [x] Ensure flag names/behavior are consistent between `inspect`, `override`, `validate`.
    - [x] Ensure help text and error messages use consistent terminology.

## Phase 4: Basic CLI Syntax Testing
_**Goal:** Ensure all CLI commands (post-Phase 3) and essential flags execute without basic parsing or validation errors. Focus on basic functionality with minimal refactoring needed._

- [x] **[P1]** Define focused test scope:
    - [x] Identify target commands: `inspect`, `override`, `validate`, `completion`, `help`.
    - [x] Identify essential global flags: `--debug`, `--log-level`, `--config`.
    - [x] Confirm focus on command parsing and flag validation, not deep chart processing logic.

- [x] **[P1]** Choose testing approach:
    - [x] Decide on using Go tests (`_test.go`) with `os/exec` to invoke the `bin/irr` binary.
    - [x] Plan extensive use of table-driven tests for DRYness and maintainability.
    - [x] Design test helpers for common actions (running commands, checking exit codes, validating output).
    - [x] Identify and plan reuse of relevant helper code from `test/integration` (e.g., binary location, project root finding).

- [x] **[P1]** Setup test environment:
    - [x] Create the dedicated Go test package: `cmd/irr/cli_test.go`.
    - [x] Implement basic test fixture setup/teardown (e.g., creating temporary directories if needed).
    - [x] Implement the helper function for locating `bin/irr`, potentially adapting from integration tests.
    - [x] Ensure test environment setup is minimal and fast, avoiding complex chart setup unless strictly necessary for a syntax test.

- [x] **[P2]** Implement success case tests:
    - [x] **`inspect` command:**
        - [x] `inspect --chart-path <path>` (no source-registries).
        - [x] `inspect --chart-path <path> --source-registries <regs>`.
        - [x] `inspect --chart-path <path>` with pattern flags (`--include-pattern`, etc.).
        - [x] `inspect --generate-config-skeleton`.
    - [x] **`override` command:**
        - [x] `override --chart-path <path> --source-registries <regs> --target-registry <reg>`.
        - [x] `override` with pattern flags and `--output-file <file>`.
        - [x] `override` with different `--strategy` values.
        - [x] `override` with `--output -` (output to stdout).
    - [x] **`validate` command:**
        - [x] `validate --chart-path <path> --values <file>`.
        - [x] `validate --chart-path <path>` with multiple `--values` files.
    - [x] **Other commands:**
        - [x] `completion bash`.
        - [x] `help`.
        - [x] `help inspect`, `help override`, etc. for each command.
    - [x] **Flag combinations:**
        - [x] Test combinations of compatible optional flags for key commands (e.g., `inspect --chart-path X --source-registries Y --include-pattern Z`).
    - [x] **General:**
        - [x] Apply table-driven test structure where beneficial.

- [x] **[P1]** Implement global flag and environment variable tests:
    - [x] Test `--debug` flag enables debug output (check stderr).
    - [x] Test `IRR_DEBUG=true` environment variable enables debug output.
    - [x] Test `--debug` flag overrides `IRR_DEBUG=false`.
    - [x] Test `--log-level` flag with different valid levels (e.g., `warn`, `error`).
    - [x] Test `IRR_LOG_LEVEL` environment variable (if implemented).
    - [x] Test flag/env precedence for `--log-level` / `IRR_LOG_LEVEL`.
    - [x] Test `--config` flag with a minimal, valid config file.
    - [x] Test `IRR_CONFIG` environment variable (if implemented).
    - [x] Test flag/env precedence for `--config` / `IRR_CONFIG`.

- [x] **[P1]** Implement error case tests:
    - [x] **Missing required flags:**
        - [x] `inspect` without `--chart-path`.
        - [x] `override` without `--chart-path`.
        - [x] `override` without `--source-registries`.
        - [x] `override` without `--target-registry`.
        - [x] `validate` without `--chart-path`.
        - [x] `validate` without `--values` (if required).
    - [x] **Invalid flag formats:**
        - [x] `inspect` with invalid `--source-registries` format.
        - [x] `override` with invalid `--target-registry` format.
        - [x] `override` with invalid `--strategy` value.
        - [x] Global flags with invalid values (e.g., `--log-level=invalid`).
    - [x] **Path errors:**
        - [x] `inspect`/`override`/`validate` with non-existent `--chart-path`.
        - [x] `validate` with non-existent `--values` file path.
        - [x] `--config` with non-existent file path.
    - [x] **Incompatible flags:**
        - [x] Test combinations of flags documented as mutually exclusive (if any).
    - [x] **General:**
        - [x] Verify appropriate non-zero exit codes for all error cases.
        - [x] Apply table-driven test structure where beneficial.

- [x] **[P1]** Simple integration with build process:
    - [x] Ensure tests are discovered and run with standard `go test ./...` or specific package target `go test ./cmd/irr/...`.

- [x] **[P2]** Create helpers for output validation:
    - [x] Develop helper to capture and validate stderr content against expected patterns/substrings.
    - [x] Develop helper to capture and validate stdout content against expected patterns/substrings (for commands like `help`, `completion`, `--output -`).
    - [x] Design helpers to check for specific known error messages or success messages.

- [x] **[P1]** Implement help text validation:
    - [x] **Content Accuracy:**
        - [x] Verify basic description in `help` output.
        - [x] Verify descriptions for each command (`help <command>`).
    - [x] **Flag Details:**
        - [x] Check required flags are marked correctly (e.g., `(required)`).
        - [x] Check default values are displayed correctly for optional flags.
        - [x] Validate consistency of flag names and descriptions across commands where applicable.
    - [x] **Examples:**
        - [x] Ensure any examples provided in help text use valid syntax.

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
  - [ ] Set up CI configuration to run `