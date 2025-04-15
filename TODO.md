# TODO.md - Helm Image Override Implementation Plan

## Completed Phases
## Phase 1: Helm Plugin Integration - Remaining Items
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

## Phase 2: Chart Parameter Handling & Rules System
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

## Phase 3: Kubernetes Version Handling in `irr validate`
_**Goal:** Implement robust Kubernetes version specification for chart validation to ensure consistent version handling and resolve compatibility issues._

### Implementation Plan

1.  **Default Kubernetes Version Setting**
    -   [ ] **[P1]** Define a modern default Kubernetes version (e.g., `1.31.0`) for validation operations.
        -   [ ] Add a constant (e.g., `DefaultKubernetesVersion = "1.31.0"`) in the codebase.
        -   [ ] Update `runValidate` in `cmd/irr/validate.go` to use this default when the `--kube-version` flag is not provided.
        -   [ ] Document this default version (`1.31.0`) in user documentation and command help text.
    -   Rationale for `1.31.0`:
        -   Provides consistency with test harness requirements for newer charts.
        -   Modern enough for most chart features and API versions.
        -   Reduces the need for immediate fallback attempts during testing.

2.  **`--kube-version` Flag Implementation**
    -   [ ] **[P1]** Add the `--kube-version` string flag to the `irr validate` command.
        -   [ ] Add flag definition in `cmd/irr/validate.go`'s `newValidateCmd` function.
            -   Example help text: `"Kubernetes version to use for validation (e.g., 1.31.0)"`
        -   [ ] Update `runValidate` function to read the flag value using `cmd.Flags().GetString("kube-version")`.
        -   [ ] Document the flag in `docs/cli-reference.md` and include examples.

3.  **Helm Integration**
    -   [ ] **[P1]** Update `internal/helm/command.go` to pass the specified Kubernetes version to Helm.
        -   [ ] Add `KubeVersion string` field to the `TemplateOptions` struct.
        -   [ ] Modify the `Template` function:
            -   If `options.KubeVersion` is not empty, append `"--kube-version", options.KubeVersion` to the `helmArgs` slice.
            -   Ensure this flag takes precedence over any `--set` values attempting to configure Kubernetes version (see Versioning Precedence below).
        -   [ ] Review `executeHelmCommand`: Current plan uses Helm CLI, where `--kube-version` works. If implementation switches to Helm SDK later, version handling will need adaptation as SDK actions derive capabilities differently. (Keep existing note about SDK differences).

4.  **test-charts.py Script Updates**
    -   [ ] **[P1]** Update the `test/tools/test-charts.py` script to primarily use the new flag for validation.
        -   [ ] In the `test_chart_validate` function, modify the `validate_cmd` construction:
            -   Add `"--kube-version", DEFAULT_TEST_KUBE_VERSION` (using the defined default, e.g., "1.31.0").
            -   **Decision:** Remove the primary `--set kubeVersion=...` and `--set Capabilities.KubeVersion...` flags used for the main validation command to rely on the `--kube-version` flag.
        -   [ ] In the fallback mechanism (when retrying with direct `helm template` calls for KUBERNETES_VERSION_ERROR):
            -   Ensure the `helm template` command includes `--kube-version` with the specific version being tested.
            -   **Consideration:** Retain the redundant `--set Capabilities.KubeVersion...` flags *only* within this fallback mechanism, as some charts might strictly rely on Capabilities. This redundancy maximizes compatibility for difficult charts during retry, but the primary validation relies on `--kube-version`.
        -   [ ] Keep the `KUBE_VERSION` environment variable setting as a potential fallback layer, but prioritize the `--kube-version` flag.

5.  **Documentation and Tests**
    -   [ ] **[P1]** Add unit tests for the Kubernetes version flag handling in `cmd/irr/validate_test.go`.
        -   Test cases: no flag (uses default `1.31.0`), valid version string.
        -   **[P1]** Test case: invalid version string input (ensure clear error message).
        -   **[P1]** Test case: Verify flag precedence (`--kube-version` overrides `--set` attempts for version).
    -   [ ] **[P1]** Add test to `irr override` validation ensuring Type 2 parameters (`kubeVersion`, `Capabilities.KubeVersion.*`) are *never* included in the generated `override.yaml` file.
    -   [ ] **[P1]** Update documentation (`README.md`, `docs/cli-reference.md`, `docs/TESTING.md`) to reflect the new flag, the `1.31.0` default, and precedence rules.
    -   [ ] **[P1]** Add integration tests in `test/integration/` to validate the flag works correctly against sample charts requiring specific Kubernetes versions (e.g., >= 1.25, >= 1.30).

### Technical Implementation Snippets (Reference)

```go
// cmd/irr/validate.go - Flag Definition
cmd.Flags().String("kube-version", "", "Kubernetes version for validation (e.g., '1.31.0')")

// cmd/irr/validate.go - Reading the flag
const DefaultKubernetesVersion = "1.31.0"
kubeVersion, err := cmd.Flags().GetString("kube-version")
// ... error handling ...
if kubeVersion == "" {
    kubeVersion = DefaultKubernetesVersion
    log.Debugf("No --kube-version specified, using default: %s", DefaultKubernetesVersion)
}
// TODO: Add logic to handle potential --set conflicts if needed, ensuring kubeVersion takes precedence.

// internal/helm/command.go - Struct Update
type TemplateOptions struct {
    // ... existing fields ...
    KubeVersion string
}

// internal/helm/command.go - Template Function Update
func Template(options *TemplateOptions) (*CommandResult, error) {
    helmArgs := []string{"template", options.ReleaseName, options.ChartPath}
    // ... add values, set, namespace args ...

    // Filter out any conflicting --set version args if kubeVersion is provided
    // Example filter logic needed here if options.SetValues are passed in

    if options.KubeVersion != "" {
        helmArgs = append(helmArgs, "--kube-version", options.KubeVersion)
    }
    return executeHelmCommand(helmArgs)
}
```

### Versioning Precedence

The final implementation will respect the following precedence for determining the Kubernetes version used during validation (highest to lowest):

1.  Explicit `--kube-version` flag provided by the user. This flag overrides any version specified via `--set`. 
2.  The defined `DefaultKubernetesVersion` constant (`1.31.0`) if the flag is not provided.
3.  Helm's internal default version if neither of the above is applicable (though our implementation ensures #2 is always used as a fallback).

## Phase 4: `kind` Cluster Integration Testing
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