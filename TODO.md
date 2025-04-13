# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)

## Phase 2: Configuration & Support Features (Completed)
### Completed Features:

## Phase 3: Component-Group Testing Framework (Completed)


## Phase 5: Helm Plugin Integration
_**Goal:** Implement the Helm plugin interface that wraps around the core CLI functionality._

### 5.1 Create Helm plugin architecture
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

### 5.2 Implement Helm-specific functionality
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
- [x] **[P1]** Enhance Helm integration:
  - [x] Add support for automatically detecting configured Helm repositories via SDK
    - [x] Access repository config via Helm SDK
    - [x] Implement caching of repository data
    - [x] Support custom repository configurations
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

### 5.3 Develop Helm plugin testing
- [x] Implement basic E2E tests for core Helm Plugin workflows (happy path `inspect`, `override`, `validate`).
- [x] Test plugin installation and registration.
- [x] Verify Helm release interaction.
- [x] **[P1]** Expand test coverage:
  - [x] **Note:** Focus tests on Helm integration points (install, SDK interactions, release handling) and avoid duplicating core logic tests covered elsewhere.
  - [x] Implement plugin installation/uninstallation testing (scripting based)
    - [x] Create test fixtures for different installation scenarios
    - [x] Test installation idempotency
    - [x] Test clean uninstall/reinstall (no persistent configuration expected between versions)
  - [x] **[P2]** Test Helm SDK interactions with mocked SDK interfaces where appropriate **(Post-Release Feature)**
    - [x] Create SDK interface mocks for testing
    - [x] Test error handling scenarios with mocked errors
    - [x] Add negative test cases for all SDK interactions
- [x] **[P1]** Add chart variety tests:
  - [x] Test with a small focused set of charts that cover key image patterns
  - [x] Test with one complex chart (kube-prometheus-stack) as representative of deeply nested charts
  - [x] Focus on confirming basic functionality rather than exhaustive coverage
- [x] **[P1]** Implement failure mode testing (deterministic):
  - [x] Test handling of basic error cases:
    - [x] One invalid chart format test case 
    - [x] One connectivity issue simulation
  - [x] Verify appropriate error codes and messages for critical failures only

### 5.4 Update documentation for Helm plugin usage
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
  - [ ] Note: Plugin updates will rely on the standard `helm plugin update` command
  - [ ] Handle configuration persistence across updates
    - [ ] Define configuration versioning scheme
    - [ ] Create migration path for config changes
    - [ ] Add configuration validation during updates

### 5.6 Test cases for P1 Features

1. **Helm SDK Integration Tests**
   - [x] Test chart path resolution from release name using SDK
   - [x] Verify repository detection and caching mechanism
   - [x] Test chart pulling with timeout handling
   - [x] Validate read-only operations (ensure no write operations occur)

2. **Plugin Installation and Registration Tests**
   - [x] Test plugin installation process with proper permissions
   - [x] Verify plugin registration with Helm
   - [x] Test plugin discovery mechanism
   - [x] Validate plugin uninstallation cleanup

3. **Complex Chart Processing Tests**
   - [x] Test with kube-prometheus-stack (representative complex chart)
   - [x] Verify handling of deeply nested dependencies
   - [x] Test image pattern detection in complex scenarios
   - [x] Validate override generation for multi-level charts

4. **Error Handling and Recovery Tests**
   - [x] Test invalid chart format handling
   - [x] Verify timeout handling for long-running operations
   - [x] Test network connectivity issue handling
   - [x] Validate error message formatting matches Helm style

5. **Repository Integration Tests**
   - [x] Test automatic repository detection
   - [x] Verify repository cache invalidation
   - [x] Test handling of custom repository configurations
   - [x] Validate repository refresh timing

6. **Documentation Example Tests**
   - [ ] Verify all documented command examples
   - [ ] Test air-gapped environment scenarios
   - [ ] Validate multi-chart deployment examples
   - [ ] Test GitOps workflow examples

Note: Version compatibility tests are already well-covered in pkg/helm/version_test.go and don't need duplication.

### 5.7 Update Test Scripts for New IRR Parameters
_**Goal:** Update test-charts.py and pull-charts.py to work with the new IRR parameters while maintaining focus on testing downloaded chart.tgz files._

1.  **Update IRR Command Parameters in `test-charts.py`**
    *   [x] **Audit:** Review the `test_chart_override` function in `test-charts.py`. Specifically check the parameters passed to the `irr override` command: `--chart-path`, `--target-registry`, `--source-registries`, `--output-file`, `--debug`.
    *   [x] **Verify/Update:**
        *   Confirm if `--chart-path` should still point to the *extracted* chart directory or if IRR now accepts the `.tgz` file directly. Adjust the path passed in `test_chart_override`.
        *   Verify if `--source-registries` is still used or if registry detection is now automatic via Helm config. Update the command construction accordingly.
        *   Check if any parameters related to Helm configuration (e.g., `--kubeconfig`, `--namespace`, Helm context flags) are now required or supported, even if not strictly needed for `.tgz` processing, and decide if they should be added with default/empty values for compatibility.
    *   [x] **Remove Deprecated:** Identify and remove any parameters previously used that are no longer supported by the new `irr override` command.
    *   [x] **Update Docs:** Update the argparse help strings and comments in `test-charts.py` to reflect the current parameter set for `irr override`.

2.  **Chart Processing Updates in `test-charts.py`**
    *   [x] **Verify Extraction:** Confirm that the existing `tarfile.extractall` logic in `test_chart_override` correctly extracts charts in a format still usable by the updated IRR's `--chart-path` parameter. No changes expected unless IRR's input requirement changed drastically.
    *   [x] **Verify Temp Dirs:** Ensure the temporary directory creation/cleanup for extracted charts remains robust. No changes expected.
    *   [x] **Verify Caching:** Ensure the `get_cached_chart` logic in `pull-charts.py` (used by `test-charts.py` indirectly) still correctly identifies cached `.tgz` files. No changes expected.

3. **Error Handling Updates in `test-charts.py`**
    *   [x] **Analyze New Errors:** Anticipate potential new error message formats from the Go/Helm SDK based `irr` binary (e.g., Go panics, Helm SDK chart loading errors `helm.sh/helm/v3/pkg/chart/loader`, internal IRR processing errors).
    *   [x] **Update Categorization:** Review and update the `categorize_error` function. Add regex patterns to match new error types (e.g., `pattern = re.compile(r"helm.sh/helm/v3/pkg/chart/loader.*error")`).
    *   [x] **Maintain Existing:** Ensure existing chart-specific error categories (YAML_ERROR, SCHEMA_ERROR, REQUIRED_VALUE_ERROR, etc.) still function correctly.
    *   [x] **Refine Patterns:** Refine existing regex patterns in `categorize_error` if the format of known errors has changed slightly with the new IRR binary.

4. **Testing Infrastructure in `test-charts.py`**
    *   [x] **Verify Output:** Check if the format or location of the generated override file (`output_file`) has changed.
    *   [x] **Update Criteria:** Adjust success/failure logic if the exit codes or stderr/stdout patterns for success/failure of `irr override` have changed.
    *   [x] **Maintain Categories:** Keep existing chart classification logic (`get_chart_classification`) and test result reporting structure (`TestResult` dataclass) unless fundamentally changed by IRR updates.

5. **Documentation Updates (`test-charts.py`)**
    *   [x] **Update Script Help:** Update `argparse` descriptions for any changed command-line arguments for `test-charts.py` itself.
    *   [x] **Update README/Comments:** Update any internal comments or external documentation (like the main README) describing how to run `test-charts.py` and interpret its results, reflecting parameter changes.
    - [x] **Update Error Docs:** Document any new error categories added in the `categorize_error` function.

Implementation Notes:
- Maintain existing chart.tgz processing workflow. The script's core purpose is unchanged.
- Keep chart download (`pull-charts.py`) and caching mechanism unchanged.
- Only update `test-charts.py` parameters and error handling as necessary to interface with the *new binary*, not to test *new Helm functionality*.
- Preserve existing chart testing functionality and compatibility with the existing chart corpus.

Testing Strategy:
1. Test with a small subset of existing cached charts first.
2. Verify the `override_cmd` construction is correct and the `irr` binary executes.
3. Check if chart extraction and path passing work as expected.
4. Run against charts known to cause specific errors (YAML, Schema, Required Value) to ensure existing error categorization still works.
5. Run against the full corpus and compare success/failure rates and error category distribution to previous runs.

Success Criteria:
- `test-charts.py` runs successfully with the updated `irr` binary and necessary parameter adjustments.
- Chart extraction and analysis workflow remains intact.
- Error handling properly captures both existing chart-related issues and new IRR binary errors.
- Compatibility with the existing chart corpus is maintained (success/failure rate should be similar, accounting for bug fixes in IRR).

## Phase 6: brute-force solver
_**Goal:** Analyze results from the brute-force solver (`solver.py` run via `test-charts.py --solver-mode`) and perform direct chart analysis to identify parameters essential for successful *Helm deployment* after applying IRR overrides. Implement generalized rules within the IRR Go application to automatically include these necessary parameters (e.g., `global.security.allowInsecureImages` for Bitnami, `kubeVersion` where needed) directly into the **generated override file**. This ensures the override file is self-sufficient for use with `helm install/upgrade`, improving true compatibility, rather than just passing IRR's internal validation which may receive external parameters._
_**Note:** The solver's reported ~96% success rate reflects validation *with potential external `--set` parameters* (like `allowInsecureImages` added by `test-charts.py` for Bitnami). The primary focus now is identifying parameters needed for the *override file itself* based on chart analysis (e.g., Bitnami requiring `allowInsecureImages`) and solver findings (e.g., Rancher needing `kubeVersion`), and implementing rules to include them in the generated output._

Implementation steps:

1.  **Error Pattern Identification & Metadata Correlation**
    *   **Input:** `solver_results.json` (generated by `test-charts.py --solver-mode`).
    *   **Scripts:** Use `analyze-results.py` and enhance `analyze_errors.py`.
    *   **Process:**
        *   Run enhanced `analyze_errors.py` to categorize failures across all charts in `solver_results.json`. Focus on categories like `REQUIRED_VALUE_ERROR`, `SCHEMA_ERROR`, `TEMPLATE_ERROR`, and specific error messages indicating image verification failures.
        *   Use `analyze-results.py` (or enhance it) to correlate error categories and specific failed parameters (`attempts` in results) with chart metadata (`classification`, `provider` fields in results, potentially enhanced by parsing `Chart.yaml` for maintainer/source info).
        *   Identify high-priority parameters based on error frequency (as noted in `SOLVER.md`, `kubeVersion` is high priority).
    *   **Output:** A mapping document or structured data defining relationships: `Error Category/Message Pattern -> Likely Required Parameter(s) -> Associated Chart Metadata (Provider, Classification, Name Pattern)`.
    *   **Example:** Document findings like: "Charts classified as 'BITNAMI' frequently fail template validation when images are overridden, and this is resolved by adding `global.security.allowInsecureImages=true`."

2.  **Rule Generation System**
    *   **Input:** Analysis output from Step 1, `solver_results.json`.
    *   **Script:** Create `extract_rules.py`.
    *   **Process:**
        *   Read the `minimal_success_params` for all successful charts in `solver_results.json`.
        *   Cluster charts based on *both* their metadata (provider, classification) *and* their required `minimal_success_params`. Leverage the chart bucketing concept from `SOLVER.md`.
        *   `extract_rules.py` generates rules by identifying the common minimal parameters required for each significant cluster/bucket.
        *   Prioritize rules covering the largest number of charts or addressing the most frequent error patterns identified in Step 1.
    *   **Output:** A structured rule file (e.g., `rules.json`) defining conditions (metadata matches) and actions (parameters to add).
        *   *Example Rule:*
          ```json
          {
            "rules": [
              {
                "name": "BitnamiImageVerification",
                "condition": { "provider": "bitnami" },
                "action": { "add_params": { "global.security.allowInsecureImages": true } }
              },
              {
                "name": "LokiStorage",
                "condition": { "name_pattern": "loki" },
                "action": { "add_params": { "loki.storage.type": "filesystem" } }
              }
              // ... other rules
            ]
          }
          ```

3.  **Detection & Integration Logic within IRR (Go)**
    *   **Input:** Generated `rules.json` (or similar format).
    *   **Code Location:** Primarily `pkg/analyzer`, `pkg/override`, `cmd/irr/override.go`.
    *   **Process:**
        *   **(Detection):** Implement Go functions to:
            *   Load and parse the generated rules file.
            *   Analyze the input chart (`Chart.yaml`, potentially `values.yaml` and template files) to determine its provider, classification, and other relevant metadata or structural patterns (e.g., presence of `common.errors.insecureImages` in templates). This mirrors the logic in `solver.py`'s `get_chart_classification` but in Go.
        *   **(Integration):** Modify the `irr override` command's execution flow:
            *   After initial image analysis but *before* finalizing the override YAML:
            *   Run the chart detection logic.
            *   Match the detected chart properties against the loaded rules.
            *   For each matching rule, merge the specified `add_params` into the generated override values map (`map[string]interface{}`). Ensure graceful merging to avoid overwriting existing user-provided values where appropriate (though these rules often set parameters unlikely to be manually overridden).
    *   **Output:** The final `override.yaml` file now includes automatically added parameters based on detected chart characteristics and applied rules.

4.  **Validation Testing**
    *   **Input:** IRR binary with integrated rule system, generated `rules.json`, chart corpus.
    *   **Scripts:** Adapt `test-solver.py` (or create `test-rules.py`), use `test-charts.py`.
    *   **Process:**
        *   **(Rule Application Verification):** Run `irr override` (with rules enabled) on a targeted subset of charts known to require specific parameters (e.g., Bitnami charts for `allowInsecureImages`). Assert that the generated `override.yaml` *contains* the parameters specified by the matching rule.
        *   **(End-to-End Validation):** Use `test-charts.py` (potentially with minor adaptations). Run `irr override` first, then run `irr validate` using the *generated* override file. The success rate of `irr validate` should significantly increase because the overrides now contain the necessary parameters. This confirms the overrides are valid for `helm template`.
        *   **(Helm Apply Simulation):** The `irr validate` step effectively simulates `helm template`, confirming the overrides are syntactically correct and satisfy the chart's basic templating requirements, addressing the core issue of the overrides being usable by Helm itself.
    *   **Output:** Test reports indicating rule application correctness and improved chart validation success rates. Measure the % of previously failing charts that now pass `irr validate`.

5.  **Rule System Enhancement & Maintainability**
    *   **Code Location:** IRR Go codebase, potentially external rule file (`rules.json`).
    *   **Process:**
        *   **(Versioning/Management):** Implement loading of rules from an external file (`rules.json`) within IRR. Include a version field in the rule file for future compatibility checks.
        *   **(Configuration):** Add optional flags to `irr override` (e.g., `--skip-rule <RuleName>`, `--rules-file <path>`) to allow users to customize rule application.
        *   **(Documentation):** Update `docs/SOLVER.md` to describe the rule generation process and the format of `rules.json`. Add a section to the main README explaining how the rule system works and how users can manage it.
        *   **(Feedback Loop):** Define the process for updating `rules.json`: Run Python analysis scripts (`analyze_errors.py`, `extract_rules.py`) periodically on new `solver_results.json`, generate updated rules, validate them, and release the new `rules.json` alongside IRR updates.
    *   **Output:** A more maintainable and configurable rule system within IRR.

**Implementation Tools Mapping:**
- `test/tools/analyze-results.py`: **Step 1** (Analyze raw solver results)
- `test/tools/analyze_errors.py`: **Step 1** (Categorize errors, identify patterns)
- `test/tools/solver.py`: **Input Source** (Generates `solver_results.json` via `test-charts.py`)
- `test/tools/test-solver.py`: **Step 4** (Needs adaptation to test rule application)
- `test-charts.py`: **Step 4** (Used for end-to-end validation with generated overrides)
- `extract_rules.py`: **Step 2** (**New script needed** to generate rules from analysis)

## Phase 7: Test Framework Refactoring
_**Goal:** Refactor the Python-based `test-charts.py` framework to use the new Phase 4/5 commands with the existing chart corpus._

- [x] Refactor `test-charts.py` script
  - [x] Update script to call `irr inspect`, `irr override`, and `irr validate` commands using the finalized CLI.
  - [x] Adapt failure categorization and reporting for the new command structure.
  - [x] Ensure script handles the new structured configuration format.
- [x] Analyze Failure Patterns (on existing corpus)
  - [x] Use the refactored script's output (error reports, patterns) to identify common failure reasons.
  - [x] Leverage test-charts.py's data collection and analysis to drive improvements.
  - [x] Note: test-charts.py generates structured JSON data critical for identifying pattern-based improvements.
- [x] Improve Values Templates ("Bucket Solvers") (on existing corpus)
  - [x] Iteratively refine the minimal values templates (`VALUES_TEMPLATE_...`) based on failure analysis to increase the success rate.
  - [x] Consider adding new classifications/templates if warranted.
- [x] Generate Intermediate Compatibility Report
  - [x] Document the chart compatibility success rate (not version compatibility) achieved after refactoring and initial template improvements.
  - [x] Note: Compatibility here refers to how many charts our tool successfully processes, not version compatibility between different Helm versions.

## Phase 8: Test Corpus Expansion & Advanced Refinement
_**Goal:** Expand the chart test set significantly and further refine compatibility based on broader testing._

- [x] Expand Test Chart Corpus
  - [x] Increase the number and variety of charts tested (e.g., Top 200+, community requests, specific complex charts).
  - [x] Update chart pulling/caching mechanism if necessary.
- [x] Re-Analyze Failure Patterns (on expanded corpus)
  - [x] Identify new or more prevalent failure reasons with the larger chart set.
  - [x] Leverage test-charts.py's data collection and analysis to drive improvements.
  - [x] Note: test-charts.py generates structured JSON data critical for identifying pattern-based improvements.
- [x] Further Improve Values Templates
  - [x] Make additional refinements to templates based on the expanded analysis.
  - [x] Use test-charts.py's bucket categorization to determine minimal configuration templates that maximize chart compatibility.
- [x] Generate Final Compatibility Report
  - [x] Document the final chart compatibility success rate across the expanded corpus.
  - [x] Focus on command-line reports that directly drive accuracy improvements and help prioritize future work.
  - [x] Note: This measures our tool's ability to successfully process charts, not version compatibility.

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

- [x] **[P1]** Identify and refactor external service integrations:
  - [x] Analyze code base for difficult-to-test integrations with external services, particularly:
    - [x] Helm SDK function calls (e.g., `Template`, `ValidateChart`, `GetValues`)
    - [x] Filesystem operations
    - [x] Network calls
    - [x] Subprocess executions
  - [x] Categorize functions by testability impact and complexity
  - [x] Prioritize high-impact functions that currently impede test coverage
  
- [x] **[P1]** Implement variable-based dependency injection:
  - [x] Refactor external calls to use package or struct-level variables for functions:
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
  - [x] Add appropriate documentation for each injected dependency
  - [x] Ensure backward compatibility during refactoring
  - [x] Define consistent naming patterns for injected functions (e.g., `xxxFunc` suffix)

- [x] **[P1]** First phase target functions for DI refactoring:
  - [x] `cmd/irr/override.go`: Refactor `helm.Template` calls to use variable injection
  - [x] `cmd/irr/validate.go`: Refactor `helm.ValidateChart` to allow test replacement
  - [x] `cmd/irr/chart.go`: Implement injection for chart loading operations
  - [x] `internal/helm/command.go`: Add injection points for underlying Helm SDK calls
  - [x] `internal/generator`: Add test hooks for filesystem operations

- [x] **[P1]** Implement comprehensive test coverage:
  - [x] Create unit tests for each refactored component that leverage function replacement
  - [x] Implement both success and failure test cases
  - [x] Include edge cases and error conditions
  - [x] Create reusable test utilities for common mock scenarios

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

### 5.7 Update Test Scripts for New IRR Parameters
_**Goal:** Update test-charts.py and pull-charts.py to work with the new IRR parameters while maintaining focus on testing downloaded chart.tgz files._

1. **Update IRR Command Parameters**
   - [x] Audit current IRR command line parameters in test-charts.py
   - [x] Update override command construction in test_chart_override function:
     ```python
     override_cmd = [
         str(irr_binary),
         "override",
         "--chart-path",
         str(extracted_chart_dir),  # Keep existing chart.tgz extraction logic
         "--target-registry",
         target_registry,
         # Only add necessary new parameters, maintain focus on chart testing
     ]
     ```
   - [x] Remove any deprecated parameters
   - [x] Update parameter documentation in script headers

2. **Chart Processing Updates**
   - [x] Verify chart.tgz extraction logic still works with new IRR
   - [x] Ensure temporary directory handling remains correct
   - [x] Maintain existing chart cache functionality
   - [x] Update chart path resolution if needed

3. **Error Handling Updates**
   - [x] Update error categorization for new IRR error patterns
   - [x] Keep existing chart-specific error categories
   - [x] Update error pattern matching in categorize_error function
   - [x] Add any new IRR-specific error patterns

4. **Testing Infrastructure**
   - [x] Verify test results format still matches new IRR output
   - [x] Update success/failure criteria if needed
   - [x] Keep existing chart test categories
   - [x] Add any new relevant test result categories

5. **Documentation Updates**
   - [x] Update script documentation with new parameters
   - [x] Document any changes in behavior
   - [x] Update error category documentation
   - [x] Keep focus on chart testing in documentation

Implementation Notes:
- Maintain existing chart.tgz processing workflow
- Keep chart download and caching mechanism unchanged
- Only update what's necessary for new IRR parameters
- Preserve existing chart testing functionality
- Focus on testing chart content, not Helm functionality

Testing Strategy:
1. Test with existing cached charts first
2. Verify chart extraction still works correctly
3. Ensure all chart types still process properly
4. Compare results with previous version

Success Criteria:
- Scripts successfully process existing chart.tgz files
- Chart extraction and analysis remains intact
- Error handling properly captures chart-related issues
- Performance matches or exceeds previous version
- Maintains compatibility with existing chart corpus