# TODO.md - Helm Image Override Implementation Plan

# Usability Improvement , CLI Refactor
## Implementation Sequence Notes
- Phase 0 (Configuration) should be completed before Phase 3.5 registry handling
- Phase 2 (Flag Consistency) should be completed before Phase 3.5 file naming standardization
- Phase 3 (Output Behavior) should be completed before implementing strict mode in Phase 3.5
## Phase 0: Configuration Setup (P0: Critical Usability) [COMPLETED]
## Phase 1: Flag Cleanup (P0: User Experience Enhancements) [COMPLETED]
## Phase 2: Flag Consistency and Defaults (P0: User Experience Enhancements) [COMPLETED]
## Phase 3: Output File Behavior Standardization (P0: User Experience Enhancements) [COMPLETED]
## Phase 3.5: Streamlined Workflow Implementation (P1: User Experience Enhancements) [COMPLETED]
## Testing Strategy for CLI Enhancements (Completed Phases Summarized) [COMPLETED]
## Phase 4: Documentation Updates (P0: User Experience Enhancements) [COMPLETED]

### Implementation Notes [COMPLETED]
- We've restructured the chart loading mechanism to properly use the chart.NewLoader() function, which improves code organization and maintainability
- We've fixed the strategy flag handling to keep it available but with sensible defaults, making the interface cleaner without removing functionality
- The integration tests now pass consistently after fixing implementation issues with chart sources and required flags

## REMINDER On the Implementation Process: (DONT REMOVE THIS SECTION)
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
     - **NEW REMINDER:** Run `make lint` and `make test` frequently after making logical changes or fixing previous issues. Don't wait until the end of a feature. Address failures promptly.

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

  6. **Building and Tesing Hints**
     - `make build` builds product, `make update-plugin` updates the plugin copy so we test that build
       `make test-filter` runs the test but filters the output, if this fails you can run the normal test to get more detail

     - default behavior: fail if file exists (per section 3.4 in PLUGIN-SPECIFIC.md)
     - Use file permissions constants for necessaary permission set, those are defined in this file : `pkg/fileutil/constants.go`

##END REMINDER On the Implementation Process:

#### Phase 6: Test Framework Overhaul
- **Goal:** Transition integration tests away from requiring a live Kubernetes cluster, improving test speed, reliability, and portability by adopting a layered testing approach.
- **P0: Critical Test Infrastructure Improvement**

##### Phase 6.1: Mock Helm Interaction Layer (Unit/Component Tests)
  - **Goal:** Test `irr`'s core logic (value traversal, image detection, override generation) and command-layer decision-making (e.g., differentiating standalone chart-path mode from plugin/release-name mode) in complete isolation from external Helm and Kubernetes dependencies. This provides the fastest feedback loop for core algorithm and command logic correctness.
  - **Scope:**
    - Unit tests and component-level integration tests that verify specific functions and modules within `irr` itself (e.g., `pkg/analyzer`, `pkg/generator`, `pkg/helm`, and especially the `cmd/irr` command implementations).
    - Enable testing of how `irr` sources its initial data (chart values, metadata) depending on the execution mode (e.g., from local files vs. Helm release).
  - **Design Rationale & References:**
    - **Need for Abstraction:** As highlighted in `docs/DEVELOPMENT.MD` (Sections 6.1, 7.1, 7.2) and `docs/PLUGIN-SPECIFIC.MD` (Sections 3, 5.2, 8.1), `irr` sources chart values and metadata differently when operating on a local chart path versus a Helm release name. A robust abstraction layer is needed to mock these varied data sourcing mechanisms for isolated testing.
    - **Existing Model:** `docs/FILESYSTEM-MOCKING.MD` details a successful strategy for abstracting filesystem interactions using `afero` and Dependency Injection (DI). This DI approach is the preferred model for the Helm interaction layer.
    - **Consistency:** The aim is to establish a standard, reusable set of interfaces and mock implementations for Helm interactions, similar to how filesystem interactions are handled, promoting consistency across tests.
  - **Concept:** Develop a comprehensive mock implementation of a new Helm interaction abstraction layer. This layer will define interfaces for operations like loading chart data (from path or release), fetching release values, and invoking `helm template`.
  - **Tasks:**
    - [ ] **Identify External Interactions & Define Interfaces:**
      - [ ] Systematically review `cmd/irr/inspect.go`, `cmd/irr/override.go`, `cmd/irr/validate.go` and `pkg/helm` to list all direct `helm` CLI calls, Helm SDK usage (e.g., `chartloader`, `action G*etValues`), and Kubernetes API client interactions.
      - [ ] Define standard Go interfaces (e.g., `ChartLoader`, `ReleaseDataProvider`, or a combined `HelmClientAPI`) in a common package (e.g., `pkg/helmtypes` or within `pkg/helm`) to abstract these operations.
        - Example `ChartLoader` methods: `LoadValues(source ChartSource) (map[string]interface{}, error)`, `LoadChartMeta(source ChartSource) (*chart.Metadata, error)`
        - Example `ReleaseDataProvider` methods: `GetReleaseValues(releaseName, namespace string) (map[string]interface{}, error)`
        - `ChartSource` could be a type to distinguish local path from release identifiers.
      - [ ] Detail the expected input parameters and return types for each interface method.
    - [ ] **Create Standard Mock Implementations:**
      - [ ] Develop reusable mock implementations for these new interfaces, preferably using `testify/mock`. These mocks should reside in a common test utility package (e.g., `pkg/testutil/mocks` or extend `pkg/testutil`).
      - [ ] Ensure mocks can be configured to simulate various scenarios:
        - Successful data retrieval (values, chart metadata from path or release).
        - Errors during data retrieval (file not found, release not found, API errors).
        - Specific data content for values and chart metadata.
        - Behavior of `helm template` (success with output, failure with error).
    - [ ] **Refactor Core `irr` Commands & Service Logic (Primary Focus):**
      - [ ] Modify the `cmd/irr/` command execution logic (e.g., `runInspect`, `runOverride`, `runValidate`) and any intermediary service layers to accept instances of the new Helm interaction interfaces via Dependency Injection. This is the primary area of refactoring for this phase.
        - Note: Leverage and adapt the existing `helmAdapterFactory` pattern in `cmd/irr/fileutil.go` for injecting Helm adapter/client instances.
      - [ ] Update production code paths to pass "real" implementations of these interfaces (e.g., one that uses `afero` and Helm SDKs for file-based charts, another that uses Helm SDKs for release-based operations).
      - [ ] Ensure unit/component test paths for the `cmd/irr` layer can receive and utilize the mock implementations.
    - [ ] **Update/Create Unit & Component Tests:**
      - [ ] Write new unit/component tests for the `cmd/irr` layer, leveraging the injected mocks to verify logic for different data sources and scenarios.
      - [ ] Review existing unit tests in `pkg/helm`, `pkg/chart` etc. If they directly perform Helm interactions that are now covered by the new interfaces, refactor them to use the standard mocks.
        - (Note: `pkg/analyzer` and `pkg/generator` tests, which are already data-driven, may not need significant changes as they consume pre-processed data).
      - [ ] Re-evaluate and potentially revive/refactor tests like those in the commented-out `pkg/chart/rules_integration_test.go` if its `MockChartLoader` aligns with the new standard interfaces.
    - [ ] **Ensure Mock Configurability:**
      - [ ] Implement a clear and robust mechanism for configuring the mock client's behavior for each test scenario (e.g., using methods like `mockClient.ExpectGetValues(...).Return(...)`).
      - [ ] Document how to set up the mock client for common test patterns.
  - **Pros:**
    - Complete isolation from external dependencies for core logic tests.
    - Fastest execution speed for unit/component tests.
    - Full control over test scenarios for focused testing.
  - **Cons:**
    - Significant upfront development effort to create accurate mocks.
    - Mocks might diverge from actual Helm behavior over time, requiring maintenance.
    - May not perfectly replicate all nuances of Helm's internal logic for all scenarios.

##### Phase 6.2: Enhanced `helm template` with Value Simulation (CLI/Chart-Path Mode Tests)
  - **Goal:** Validate `irr`'s behavior when interacting with the `helm template` command, particularly for chart-path based operations (e.g., `irr inspect --chart-path ...`, `irr override --chart-path ...`, `irr validate --chart-path ...`), without requiring a full Kubernetes cluster.
  - **Scope:** Tests that focus on `irr`'s ability to correctly prepare values for `helm template` and interpret its output/behavior, especially for the `validate` command or when simulating chart processing locally.
  - **Concept:** Rely primarily on `helm template` for core validation but enhance how `irr` prepares inputs for it, simulating the "deployed state" or complex value scenarios without actual deployment.
  - **Tasks:**
    - [ ] For `--chart-path` based operations, instead of (or in addition to) full K8s mocking, focus on constructing the necessary values input for `helm template` by:
      - Loading the chart's default `values.yaml`.
      - Simulating the application of user-provided values (e.g., from `-f` flags).
      - Applying `irr`-generated overrides to these simulated values.
    - [ ] Develop a robust way to manage and inject these simulated "current values" into the `helm template` process invoked by `irr validate` or other relevant commands.
    - [ ] Update `TestHarness` and relevant integration tests to set up these simulated value states for `helm template` based tests.
  - **Pros:**
    - Leverages Helm's mature templating engine, ensuring accuracy for `helm template` based validation.
    - Less extensive Go-level mocking code to write for these specific scenarios compared to Option 1.
    - Good for testing `irr validate` in chart-path mode.
  - **Cons:**
    - Still relies on the `helm` binary being present and correctly configured.
    - Simulating complex Helm value merging logic (e.g., order of `-f` files, `--set` overrides) accurately can be challenging outside a real Helm invocation.
    - Does not cover plugin-mode interactions requiring a K8s API.

##### Phase 6.3: Transition to Kind Cluster (Plugin Mode & Full Integration Tests)
  - **Goal:** Replace the use of external/real Kubernetes clusters in existing integration tests with local, ephemeral Kind (Kubernetes in Docker) clusters. This enables true end-to-end testing of `irr` in plugin mode (e.g., `helm irr inspect <release> -n <ns>`) and other scenarios requiring a live Kubernetes API, but in a controlled, fast, and reproducible environment.
  - **Scope:** Integration tests that currently interact with a real Kubernetes cluster, especially those testing Helm plugin mode operations that rely on `helm get values`, `helm list`, or other release-aware interactions. The tests identified by `grep -n -H 'namespace :=' test/integration/*.go` are primary candidates for this phase.
  - **Concept:** Combine elements of using real Helm against a real (but local/disposable) Kubernetes API. Mock Kubernetes API interactions for release discovery and fetching basic metadata, but use `helm template` with simulated values for validation.
  - **Tasks:**
    - [ ] Implement mocks for `helm list` and parts of `helm get values` that retrieve release metadata (chart name, version, namespace). (Potentially superseded by direct Kind interaction)
    - [ ] Use the value simulation approach from Option 2 for preparing inputs to `helm template` within `irr validate`. (Potentially superseded by direct Kind interaction)
    - [ ] **Setup Kind Integration:**
      - [ ] Integrate Kind cluster provisioning (e.g., `kind create cluster`) and teardown (`kind delete cluster`) into the `TestHarness` or CI/CD pipeline scripts for integration tests.
      - [ ] Configure `TestHarness` to obtain and use the Kubeconfig from the temporary Kind cluster for all Helm operations within these tests.
    - [ ] **Migrate Existing Integration Tests:**
      - [ ] Systematically refactor existing integration tests (e.g., in `test/integration/override_command_test.go`, `test/integration/inspect_command_test.go`) that install Helm releases to a "real" cluster.
      - [ ] Modify these tests to:
        - Ensure a Kind cluster is running.
        - Install necessary Helm charts (e.g., `fallback-test`, `parent-test`) into the Kind cluster.
        - Execute `irr` commands (e.g., `helm irr inspect my-release -n test-ns`) targeting the release in the Kind cluster.
        - Perform assertions based on the output or state.
        - Ensure Helm releases are uninstalled from Kind after tests.
    - [ ] **Verify Plugin Behaviors:**
      - [ ] Confirm that namespace handling, release value fetching, and other plugin-specific features work correctly against the Kind cluster.
    - [ ] **Update Documentation:**
      - [ ] Revise `TESTING.md` and any relevant developer guides to document the use of Kind for integration tests, including setup and execution.
  - **Pros:**
    - Balances mocking effort with the accuracy of using `helm template`. (Original Pro)
    - Potentially faster to implement than a full Option 1. (Original Pro)
    - **Provides high-fidelity testing for plugin mode and K8s interactions.**
    - **Faster and more reliable than tests against external/shared K8s clusters.**
    - **Improves CI/CD efficiency and reduces external dependencies.**
  - **Cons:**
    - More complex test setup than a pure Option 2. (Original Con)
    - Still requires careful management of how simulated values interact with the real `helm template`. (Original Con, less relevant if Kind provides the "real" values)
    - **Adds Kind as a development/CI dependency.**
    - **Slower than pure Go-level mock tests (Phase 6.1) or `helm template` simulation (Phase 6.2).**
