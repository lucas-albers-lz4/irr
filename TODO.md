# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)

## Phase 2 Configuration file support
- [ ] Add configuration file support (`--config`)  # Critical for user-defined registry mappings (e.g., Harbor pull-through cache paths)
    - **Goal:** Allow users to specify exact target registry paths for each source registry via a YAML configuration file, overriding the default path strategy for mapped registries.
    - **Implementation Steps:**
        1.  **Update CLI Parsing:**
            -   **Action:** Modify the CLI argument handling (e.g., using Cobra/Viper) to accept the `--config` flag, taking a file path as its value. Make `--target-registry` mandatory, even when `--config` is used. Error clearly if `--target-registry` is missing.
            -   **Validation:** Ensure the `--config` path is valid, file exists, and is readable. Handle file read/permission errors gracefully with distinct messages.
            -   **Error Handling:** Follow existing error code conventions (Exit Code 2 for input/configuration errors - e.g., missing `--target-registry`, file not found, permission denied).
        2.  **Implement Mapping Loader:**
            -   **Action:** Create a function to parse the YAML file specified by `--config`.
            -   **Format:** Expect a simple `map[string]string` YAML format where keys are source registry domains (e.g., `docker.io`) and values are the *full target registry and repository prefix* (e.g., `harbor.home.arpa/docker`).
            -   **Validation:** Implement **strict** validation:
                -   Ensure the YAML parses correctly *and* conforms exactly to the `map[string]string` structure.
                -   Validate keys: Must be valid domain names (allowing for complex regional domains like `*.amazonaws.com` or compound domains like `k8s.gcr.io`).
                -   Validate values: Must contain at least one `/`, look like a valid registry/path with optional port (e.g., `harbor.home.arpa:5000/docker`).
                -   Reject files with incorrect types, invalid keys/values, or malformed content.
                -   Error out clearly if any validation fails with appropriate exit codes (Exit Code 2 for configuration errors).
            -   **Data Structure:** Use `map[string]string` to store the validated mappings.
            -   **Library:** Use `sigs.k8s.io/yaml` for parsing.
        3.  **Integrate Mappings into Core Logic:**
            -   **Action:** Modify the image processing logic (`DetectImages` or similar function). When processing an image reference:
                -   Normalize the image reference first (e.g., expand `nginx:latest` to `docker.io/library/nginx:latest`).
                -   Check if the image's source registry exists as a key in the loaded mappings.
                -   If a mapping exists, use the mapped value (split into registry and repo prefix) for the target, ignoring the default path strategy for this image.
                -   If no mapping exists, fall back to using the mandatory `--target-registry` and the default `--path-strategy`.
            -   **Interaction:** `--config` mappings override default behavior for listed sources. `--target-registry` is the **required** fallback.
            -   **Docker Hub Library Handling:** For implicit Docker Hub images (e.g., `nginx:latest`), normalize to `docker.io/library/nginx:latest` before checking for mappings. This ensures correct path construction for Harbor's pull-through cache.
        4.  **Update Override Generation:**
            -   **Action:** Implement logic to split the validated config value (e.g., `harbor.home.arpa:5000/docker`) at the first slash: registry part (`harbor.home.arpa:5000`) and repository prefix (`docker/`).
            -   **Action:** Ensure the override generation correctly uses the mapped target registry and repository prefix when a mapping was applied. The override structure should set the `registry` and `repository` fields accordingly.
        5.  **Add Unit Tests:**
            -   **Action:** Create unit tests for the mapping loader function covering: valid YAML, invalid YAML format, file not found, permission errors, empty file, invalid keys/values. Verify strict validation errors.
            -   **Action:** Create unit tests for the image processing logic: verify mappings applied, fallbacks work, config value splitting logic.
            -   **Action:** Test Docker Hub library image normalization and mapping.
            -   **Action:** Test registry paths with port numbers.
            -   **Action:** Test CLI validation: mandatory `--target-registry`, config file permissions.
            -   **Framework:** Use `afero.MemMapFs` for filesystem/permission interactions.
        6.  **Add Integration Tests:**
            -   **Action:** Create integration tests using sample charts and valid/invalid `registry-mappings.yaml` files.
            -   **Action:** Add integration tests that omit the mandatory `--target-registry` flag.
            -   **Action:** Add tests specifically for Docker Hub library images to verify correct normalization.
            -   **Validation:** Verify `override.yaml` generation based on config/fallback. Verify expected errors for invalid configs/missing flags.
        7.  **Update Documentation:**
            -   **Action:** Update `README.md`, `DEVELOPMENT.md`, and `TESTING.md` to document:
                -   The `--config` flag.
                -   The **strict** expected YAML format (`map[string]string`).
                -   The **required** format for values (`<registry_host>[:<port>]/<path_prefix>`).
                -   The **mandatory** nature of `--target-registry` as the fallback.
                -   Docker Hub library image handling.
                -   All relevant validation errors and conditions with their exit codes.
    - **Verification Points:**
        -   [ ] Run `bin/irr override --config registry-mappings.yaml --target-registry fallback.registry ...` successfully maps sources from config and uses `fallback.registry` for others.
        -   [ ] Run `bin/irr override --config registry-mappings.yaml` (no `--target-registry`) fails with Exit Code 2 and a clear error message.
        -   [ ] Run `bin/irr override --config registry-mappings.yaml --target-registry fallback.registry ...` correctly handles Docker Hub library images (e.g., `nginx:latest` → `harbor.home.arpa/docker/library/nginx:latest`).
        -   [ ] Run `bin/irr override --config registry-mappings.yaml --target-registry fallback.registry ...` correctly processes registry paths with port numbers (e.g., `harbor.home.arpa:5000/docker`).
        -   [ ] Run `bin/irr override --config non_existent_file.yaml --target-registry fallback.registry ...` fails with Exit Code 2 and a clear "file not found" error.
        -   [ ] Run `bin/irr override --config unreadable_file.yaml --target-registry fallback.registry ...` fails with Exit Code 2 and a clear "permission denied" error.
        -   [ ] Run `bin/irr override --config malformed_yaml_file.yaml --target-registry fallback.registry ...` fails with appropriate exit code and a clear YAML parsing error.
        -   [ ] Run `bin/irr override --config wrong_format_file.yaml --target-registry fallback.registry ...` (e.g., list, invalid keys/values, values missing '/') fails with Exit Code 2 and a clear "invalid format" error.
        -   [ ] Run `bin/irr override --config empty.yaml --target-registry fallback.registry ...` runs successfully, applying only fallback logic.
        -   [ ] Unit tests for loader, validation, logic, CLI pass.
        -   [ ] Integration tests (valid/invalid configs, missing flags, Docker Hub library) pass/fail as expected.
        -   [ ] Documentation accurately reflects the mandatory flags, strict format, and validation.

    - **Test Cases to Implement:**
        1. **Unit Tests:**
           - **Configuration File Loading:**
             - ✓ Parse valid YAML mapping file
             - ✓ Reject malformed YAML
             - ✓ Handle empty file
             - ✓ Handle file not found
             - ✓ Handle permission denied (using `afero.MemMapFs` permissions)
             
           - **Mapping Validation:**
             - ✓ Validate correct map[string]string format
             - ✓ Reject list instead of map
             - ✓ Reject nested maps
             - ✓ Validate domain-like keys
             - ✓ Validate complex domains (e.g., `k8s.gcr.io`, `us-east1.gcr.io`)
             - ✓ Validate registry/path in values
             - ✓ Validate registry with port in values
             - ✓ Reject values missing slash separator
             
           - **Image Processing:**
             - ✓ Normalize Docker Hub library images
             - ✓ Apply mapping for matched source registry
             - ✓ Fall back to target-registry for unmatched sources
             - ✓ Split registry value into registry and path parts
             - ✓ Correctly handle registries with ports
             - ✓ Test with both tag and digest-based images
             
           - **CLI Handling:**
             - ✓ Require `--target-registry` even with `--config`
             - ✓ Produce correct error code (2) for missing required flags
             - ✓ Produce correct error codes for various validation failures
        
        2. **Integration Tests:**
           - **Basic Functionality:**
             - ✓ Generate overrides for chart with `--config` and `--target-registry`
             - ✓ Verify Docker Hub library images correctly mapped
             - ✓ Test fallback to target-registry for unmapped sources
             - ✓ Test parent/child chart with images from multiple registries
             - ✓ Test with empty but valid config file (should apply fallback only)
           
           - **Error Handling:**
             - ✓ Missing `--target-registry` flag produces correct error
             - ✓ Malformed config file produces correct error
             - ✓ File not found produces correct error
             - ✓ Verify expected failure messages and exit codes
           
           - **Real-World Test:**
             - ✓ Create test chart with images from docker.io, quay.io, gcr.io
             - ✓ Configure mapping to Harbor pull-through cache paths
             - ✓ Verify generated override correctly maps all images
             - ✓ Render Helm chart with override and validate image references

        3. **Command-Line Test Cases:**
           - ✓ `bin/irr override --chart-path ./test-chart --target-registry fallback.registry --config valid-mappings.yaml`
             (Expected: Success, images mapped per config or fallback)
           - ✓ `bin/irr override --chart-path ./test-chart --config valid-mappings.yaml`
             (Expected: Error code 2, missing required --target-registry)
           - ✓ `bin/irr override --chart-path ./test-chart --target-registry fallback.registry --config nonexistent.yaml`
             (Expected: Error code 2, file not found)
           - ✓ `bin/irr override --chart-path ./test-chart --target-registry fallback.registry --config malformed.yaml`
             (Expected: Error with clear parsing failure)
           - ✓ Test with specially crafted chart containing Docker Hub images (both explicit and library)
             (Expected: Proper handling of implicit library paths)
- [ ] Enhance image identification heuristics (config-based patterns)
- [ ] Improve digest-based reference handling
- [ ] Add comprehensive private registry exclusion patterns
- [ ] Implement target URL validation
- [ ] Explore support for additional target registries (Quay, ECR, GCR, ACR, GHCR)
- [ ] Enhance strategy validation and error handling

## Phase 3.0: Component-Group Testing for Complex Charts
- [ ] **Implement Component-Group Testing for Complex Charts:**
    - **Goal:** Improve testability and debugging for complex charts like cert-manager while maintaining cohesive test structure and leveraging existing test infrastructure.
    - **Implementation Steps:**
        1. **Define component groups for cert-manager:**
            - **Action:** Analyze `cert-manager` chart's `values.yaml` and subchart structure to finalize logical groupings (e.g., core controllers, webhooks, cainjector, startup API check). Confirm image paths (`controller.image`, `webhook.image`, etc.).
            - **Details:** Establish initial groups:
                - `core_controllers`: `cert-manager-controller`, `cert-manager-webhook`. Critical components.
                - `support_services`: `cert-manager-cainjector`, `cert-manager-startupapicheck`. Supporting components.
            - **Thresholds:** Define and justify thresholds per group based on component criticality and image definition complexity. Start with 100% for `core_controllers` and potentially 95% for `support_services` if their image refs are complex or less critical. Document this rationale.
            - **Considerations:** How will this grouping strategy scale to other complex charts like `kube-prometheus-stack`? Define criteria for grouping (e.g., functionality, subchart origin).

        2. **Create table-driven subtest structure:**
            - **Action:** Implement the Go test structure using a `struct` array and `t.Run()` for subtests within `TestCertManager`.
            - **Table Definition:** Define the test table `struct` clearly: `name string`, `components []string` (relevant value paths/keys), `threshold int`, `expectedImages int`, `isCritical bool`.
            - **Subtest Implementation:** Use `t.Run(group.name, ...)` loop. Ensure subtest names are descriptive and usable for filtering (e.g., no spaces). Load the chart *once* outside the loop. Inside the loop, filter/focus the validation logic based on `group.components`.
            - **Error Reporting:** Ensure `t.Errorf` calls within subtests include `group.name` context and adhere to the structured error format from `TESTING.md`.

        3. **Enhance error isolation and reporting:**
            - **Action:** Modify logging and error messages to include component group context. Ensure test filtering works as expected.
            - **Error Context:** Update helper functions (image detection, validation) to optionally accept and prepend a `componentGroup string` to error messages.
            - **Filtering:** Rely on Go's native `-run TestName/SubtestName` filtering. Document this specific usage pattern for developers.
            - **Debug Logs:** Pass `group.name` context to `debug.Printf` calls made within the subtest's scope. Ensure logs clearly attribute messages to the correct group.

        4. **Update existing test utilities:**
            - **Action:** Adapt shared test helper functions to understand and utilize component group context for focused validation.
            - **Chart Test Helpers:** Identify and refactor relevant helpers (e.g., chart loading, override execution, result validation functions in `test/integration` or test setup). They might need to accept `group.components` to scope their validation actions.
            - **Component Validation:** Create specific helper functions if needed, e.g., `validateImageOverridesForGroup(t, results, groupName, expectedImages, threshold)`.
            - **Reporting Framework:** Verify Go test output clearly shows pass/fail status for each subtest (group). Ensure any aggregated reports (like those potentially generated by `test-charts.py` if adapted) correctly reflect subtest outcomes.

        5. **Document the approach and testing patterns:**
            - **Action:** Update `DEVELOPMENT.md` and `TESTING.md` (already done). Add practical examples and refine threshold documentation.
            - **Examples:** Add concrete examples to docs showing `go test -run TestCertManager/core_controllers`, interpreting group-specific errors, and potentially outlining groups for another complex chart.
            - **Threshold Strategy Doc:** Expand documentation explaining *why* different thresholds might be used (criticality, known complexity, third-party subchart stability).

    - **Verification Points:**
        - [ ] **Pass Criteria:** Confirm `TestCertManager` passes reliably without skipping any parts. Run `go test ./... -run TestCertManager`.
        - [ ] **Error Isolation:** Introduce a specific failure (e.g., wrong `expectedImages` for `support_services`) and verify only that subtest fails while others pass.
        - [ ] **Selective Filtering:** Execute `go test ./... -run TestCertManager/core_controllers` and `go test ./... -run TestCertManager/support_services` individually and confirm only the targeted subtest runs.
        - [ ] **Threshold Logic:** Create test conditions to verify threshold boundaries (e.g., 95% threshold passes with 19/20 images, fails with 18/20). May require temporarily adjusting expected counts or using `t.Skip` strategically.
        - [ ] **Framework Compatibility:** Run the full test suite (`go test ./...` or `make test`) and potentially `make test-charts` (if adapted) to check for regressions and ensure output format is preserved/enhanced.
        - [ ] **Debug Context:** Execute a subtest with `-debug` (e.g., `go test ./... -run TestCertManager/core_controllers -debug`) and manually inspect logs for `[DEBUG] core_controllers:` prefixes.
        - [ ] **Error Message Format:** Introduce a validation error within a subtest and verify `t.Errorf` output includes the group name and follows the structured format from `TESTING.md`.

## Implementation Process:  DONT" REMOVE THIS SECTION as these hints are important to remember.
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