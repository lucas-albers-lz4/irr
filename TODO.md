# TODO.md - Helm Image Override Implementation Plan

## Completed Phases
## Phase 1: Helm Plugin 
- [x] Successfully published the binary as a Helm plugin.
- [x] Completed and tested the GitHub publish and install process.

**P0: Core Plugin Infrastructure**
- [x] **[P0]** Create plugin.yaml file with appropriate metadata
  Note : we have a working plugin install and build release process for installing and publishing the plugin, this is complete.
  - [x] Draft initial plugin.yaml using Helm's plugin spec
  - [x] Set up command aliases and help text
  - [x] Add install/uninstall hooks for dependency checks (Go, Helm version)
  - [x] Test plugin.yaml with `helm plugin install` locally
  - [x] Reference fields: `name`, `version`, `usage`, `description`, `command`, `platforms`
- [x] **[P0]** Adapt plugin entrypoint (`cmd/irr/main.go`) for Helm context
  - [x] Adapt main.go to handle Helm-specific initialization and flags
  - [x] Detect Helm environment variables (HELM_PLUGIN_DIR, etc.)
  - [x] Add logging setup (respect --debug flag, align with Helm style)
  - [x] Route subcommands to core IRR logic or Helm-adapter
  - [x] Use `cobra` for command handling (confirmed)
- [x] **[P0]** Design adapter layer between Helm plugin and core IRR
  - [x] Define Go interface for Helm client (GetReleaseValues, GetChartMetadata, etc.)
  - [x] Implement real Helm client using Helm Go SDK
  - [x] Implement mock Helm client for tests
  - [x] Add error wrapping for context (release name, namespace)
  - [x] Use dependency injection for Helm client and logger
  - [x] Ensure all file/network operations are mockable for tests
  - [x] Use context.Context for all blocking operations
  - [x] Keep all Helm-specific logic in adapter; core logic should not import Helm packages
  - [x] Design and implement execution mode detection (plugin vs standalone)
    - [x] Use `HELM_PLUGIN_DIR` environment variable to detect plugin mode
    - [x] Configure Helm client differently based on execution mode
    - [x] Only enable `--release-name` and `--namespace` flags when running in plugin mode (see PLUGIN-SPECIFIC.md)
    - [x] In standalone mode, error if `--release-name` or `--namespace` is provided, with clear message: "The --release-name and --namespace flags are only available when running as a Helm plugin (helm irr ...)"
    - [x] Document this behavior and rationale in both code comments and user documentation
  - [x] Implement plugin-specific initialization
    - [x] Use `cli.New()` from Helm SDK to get plugin environment settings
    - [x] Initialize action.Configuration with Helm's RESTClientGetter when in plugin mode
    - [x] Handle namespace inheritance from Helm environment
  - [x] Create robust error handling for environment differences
    - [x] Provide clear error messages when attempting to use plugin features in standalone mode
    - [x] Include helpful troubleshooting info in errors (e.g., "Run as 'helm irr' to use this feature")
    - [x] Document the feature limitations in different execution modes
- [x] **[P0] Fix Duplicate isRunningAsHelmPlugin Function**
  - [x] Apply Solution 2 (main determines and passes the value):
    - [x] Modify `internal/helm/adapter.go`:
      - [x] Update `Adapter` struct to keep the `isRunningAsPlugin` field
      - [x] Modify `NewAdapter` function signature to accept `isPlugin bool` parameter
      - [x] Remove the `isRunningAsHelmPlugin()` function from adapter.go
    - [x] Keep the `isRunningAsHelmPlugin()` function only in `cmd/irr/main.go`
    - [x] Update all adapter creation sites to pass the plugin mode:
      - [x] Find all locations where `helm.NewAdapter()` is called
      - [x] Pass the `isHelmPlugin` global variable to each call
    - [x] Run unit tests to verify functionality is maintained
    - [x] Update any affected tests that might have relied on the removed function
    - [x] Run linting to check for any unused imports
- [x] **[P0] Unit Testing**
  - [x] Unit tests for Helm environment variable detection (`isRunningAsHelmPlugin` in `cmd/irr/main.go`).
  - [x] Unit tests for subcommand routing based on execution mode (plugin vs. standalone).
  - [x] Unit tests for `cobra` flag parsing (`--release-name`, `--namespace`) in plugin mode vs standalone mode.
  - [x] Unit tests for the `RealHelmClient` methods (`GetReleaseValues`, `GetChartFromRelease`, etc.) using mocked Helm SDK dependencies.
  - [x] Unit tests for the `MockHelmClient` implementation to ensure mock functions behave as expected.
  - [x] Unit tests for error wrapping logic within the adapter layer.
  - [x] Unit tests for execution mode detection logic (`isHelmPlugin` variable determination).
  - [x] Unit tests for plugin-specific initialization (e.g., `initHelmPlugin` in `cmd/irr/root.go`).
  - [x] Unit tests verifying correct error messages when attempting to use plugin-only features in standalone mode.
  - [x] Unit tests for namespace handling logic (flag vs. environment variable vs. default).
  
- [ ] **[P0]** Design adapter layer between Helm plugin and core IRR
  - [ ] Define Go interface for Helm client (GetReleaseValues, GetChartMetadata, etc.)
  - [ ] Implement real Helm client using Helm Go SDK
  - [ ] Implement mock Helm client for tests
  - [ ] Add error wrapping for context (release name, namespace)
  - [ ] Use dependency injection for Helm client and logger
  - [ ] Ensure all file/network operations are mockable for tests
  - [ ] Use context.Context for all blocking operations
  - [ ] Keep all Helm-specific logic in adapter; core logic should not import Helm packages
  - [ ] Design and implement execution mode detection (plugin vs standalone)
    - [ ] Use `HELM_PLUGIN_DIR` environment variable to detect plugin mode
    - [ ] Configure Helm client differently based on execution mode
    - [ ] Only enable `--release-name` and `--namespace` flags when running in plugin mode (see PLUGIN-SPECIFIC.md)
    - [ ] In standalone mode, error if `--release-name` or `--namespace` is provided, with clear message: "The --release-name and --namespace flags are only available when running as a Helm plugin (helm irr ...)"
    - [ ] Document this behavior and rationale in both code comments and user documentation
  - [ ] Implement plugin-specific initialization
    - [ ] Use `cli.New()` from Helm SDK to get plugin environment settings
    - [ ] Initialize action.Configuration with Helm's RESTClientGetter when in plugin mode
    - [ ] Handle namespace inheritance from Helm environment
  - [ ] Create robust error handling for environment differences
    - [ ] Provide clear error messages when attempting to use plugin features in standalone mode
    - [ ] Include helpful troubleshooting info in errors (e.g., "Run as 'helm irr' to use this feature")
    - [ ] Document the feature limitations in different execution modes

**P1: Core Command Implementation**
- [ ] **[P1]** Implement release-based context for commands
  - [x] Implement function to fetch release values using Helm SDK (`helm get values`)
 
  - [x] Parse namespace from CLI flags, Helm config, or default
  - [x] Implement chart source resolution (from release metadata, fallback to local cache or error)
  - [x] Use `action.NewGetValues()` from Helm SDK for value fetching
  - [x] For namespace: check `--namespace` flag, then `HELM_NAMESPACE`, then default to `"default"`
- [ ] **[P1]** Adapt core commands to work with Helm context
  - [ ] Refactor inspect/override/validate to accept both chart path and release name as input
  - [ ] Add logic to merge values from release and user-supplied files
  - [ ] Ensure all commands log the source of values and chart (for traceability)
  - [ ] Accept both `--chart-path` and `--release-name`; error if neither provided
  - [ ] Prioritize chart path over release name if both provided (with clear logging)
- [ ] **[P1]** Implement file handling with safety features
  - [x] Implement file existence check before writing output
  - [x] Use 0600 permissions for output files by default
  - [ ] Write unit tests for file safety logic
  - [x] Default behavior: fail if file exists (per section 4.4 in PLUGIN-SPECIFIC.md)
  - [x] Use file permissions constants for necessaary permission set, those are defined in this file : `pkg/fileutil/constants.go`

**P2: User Experience Enhancements**
- [ ] **[P2]** Implement Helm-consistent output formatting
  - [ ] Implement a logging utility that mimics Helm's log levels and color codes
  - [ ] Add a --debug flag to all commands, with verbose output when enabled
  - [ ] Write tests to verify log output format and color in TTY/non-TTY environments
  - [ ] Use ANSI color codes for log levels (INFO: cyan, WARN: yellow, ERROR: red)
  - [ ] For TTY detection, use the `github.com/mattn/go-isatty` package instead of manual detection
  - [ ] Ensure logging aligns with existing debug approach (controlled by --debug flag and IRR_DEBUG env var)
  - [ ] Maintain simple log format (without timestamps or explicit log levels in output) for consistency
- [ ] **[P2]** Add plugin-exclusive commands
  - [ ] Implement list-releases by calling `helm list` via SDK or subprocess
  - [ ] Parse and display image analysis results in a table format
  - [ ] Add circuit breaker to limit analysis to 300 releases
  - [ ] For interactive registry selection, detect TTY and prompt user; skip in CI
  - [ ] Skip interactive prompts if `CI` environment variable is set or not in TTY
- [ ] **[P2]** Create comprehensive error messages
  - [ ] Standardize error message format (prefix with ERROR/WARN/INFO)
  - [ ] Add actionable suggestions to common errors (e.g., missing chart source)
  - [ ] Implement credential redaction in all error/log output
  - [ ] When chart source is missing, print specific recovery steps (see section 5.4 in PLUGIN-SPECIFIC.md)

**P3: Documentation and Testing**
- [ ] **[P3]** Create plugin-specific tests
  - [ ] Write unit tests for the adapter layer using Go's testing and testify/gomock
  - [ ] Create integration tests using a local kind cluster and test Helm releases
  - [ ] Add CLI tests for plugin entrypoint and command routing
  - [ ] Test error handling and edge cases (e.g., missing release, invalid namespace)
  - [ ] Test all error paths (every `if err != nil` block)
- [ ] **[P3]** Document Helm plugin usage
  - [ ] Write a dedicated section in docs/PLUGIN-SPECIFIC.md for plugin install/upgrade/uninstall
  - [ ] Add usage examples for each command, including edge cases
  - [ ] Document all plugin-specific flags and environment variables
  - [ ] Add troubleshooting section for common errors and recovery steps
  - [ ] Create a "Quickstart" section with install and usage examples
  - [ ] List all flags and environment variables in a reference table

**Cross-Cutting Best Practices**
- [x] Use KISS and YAGNI: avoid speculative features
- [x] Implement single source of truth for version (plugin.yaml)
- [x] Automate version propagation to pyproject.toml via Makefile
- [x] Inject version into Go binary at build time using linker flags
- [ ] Add code comments and docstrings for all exported functions and interfaces
- [ ] Add structured logging for all major operations (start, success, error)
- [ ] Schedule regular code and design reviews after each vertical slice
- [ ] Update documentation and onboarding materials after each review
- [ ] Emphasize non-destructive philosophy - never write to the cluster, only read and generate files
- [ ] Update docs/cli-reference.md to reflect any CLI or logging changes, ensuring documentation matches implementation (especially for debug/logging behavior)

**Developer Onboarding Checklist**
- [ ] Document required tools and environment setup (Go, Helm, kind, etc.)
- [x] Provide a makefile or scripts for common dev tasks (build, test, lint, install plugin)
- [ ] Add a quickstart guide for running and testing the plugin locally
- [ ] List all relevant docs (PLUGIN-SPECIFIC.md, DEVELOPMENT.md, TESTING.md) at the top of the section
- [x] Create makefile targets for: `build`, `test`, `lint`, `install-plugin`
- [x] Add step-by-step quickstart: clone repo, build, install plugin, run help command

## Phase 2: Chart Parameter Handling & Rules System
_**Goal:** Analyze results from the brute-force solver and chart analysis to identify parameters essential for successful Helm chart deployment after applying IRR overrides. Implement an intelligent rules system that distinguishes between Deployment-Critical (Type 1) and Test/Validation-Only (Type 2) parameters._

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

## Phase 5: `kind` Cluster Integration Testing
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
 
  ## REMINDER 0 (TEST BEFORE AND AFTER) Implementation Process: DONT REMOVE THIS SECTION as these hints are important to remember.
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

## Testing Plan

**Test Coverage for Helm Adapter Components**
- [ ] Unit tests for HelmClientInterface implementations
  - [ ] Test RealHelmClient with mocked Helm SDK components
  - [ ] Test MockHelmClient for test fixture correctness
- [ ] Unit tests for Adapter functionality
  - [ ] Test InspectRelease with various input scenarios
  - [ ] Test OverrideRelease with different registry and path strategies
  - [ ] Test ValidateRelease with mockable filesystem and client
- [ ] Integration tests for end-to-end command execution
  - [ ] Test with both mock and real implementations (when possible)
  - [ ] Test plugin mode detection and behavior differences
  - [ ] Test all error paths with appropriate error codes

**Testing Frequency**
- [ ] Run unit tests after each significant component change
- [ ] Run integration tests before committing feature completions
- [ ] Add specific test cases for any bug fixes to prevent regressions

**Testing Coverage Goals**
- [ ] Aim for >85% code coverage for core adapter functionality
- [ ] 100% coverage for critical path components (plugin detection, error handling)
- [ ] Test all parameter combinations at the API boundaries

