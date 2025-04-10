# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)
*Initial project setup, core Go implementation (chart loading, image processing, path strategy, output generation, CLI), testing foundations, documentation, historical fixes, and testing stabilization are complete.*

## Phase 2: Stretch Goals (Post-MVP - Pending)
*Potential future enhancements after stabilization.*
- [✓] Implement `flat` path strategy
- [ ] Implement multi-strategy support (different strategy per source registry)
- [ ] Add configuration file support (`--config`)
- [ ] Enhance image identification heuristics (config-based patterns)
- [ ] Improve digest-based reference handling
- [ ] Add comprehensive private registry exclusion patterns
- [ ] Implement target URL validation
- [ ] Explore support for additional target registries (Quay, ECR, GCR, ACR, GHCR)
- [ ] Enhance strategy validation and error handling

## Phase 2.5: Quality & Testing Enhancements (In Progress)
- [✓] **Leverage `distribution/reference` for Robustness:**
    - **Goal:** Enhance program quality and test accuracy by fully utilizing the `github.com/distribution/reference` library.
    - **Program Quality Improvements:**
        - Ensure strict compliance with the official Docker image reference specification.
        - Inherit robustness by using the library's handling of edge cases (e.g., implicit registries, ports, digests, tags).
        - Improve code clarity and type safety using specific types (`reference.Named`, `reference.Tagged`, `reference.Digested`) and methods (`Domain()`, `Path()`, `Tag()`, `Digest()`).
        - Benefit from built-in normalization (e.g., adding `docker.io`, `latest` tag).
        - Reduce maintenance by relying on the upstream library for spec updates.
    - **Test Case Improvements:**
        - Use `reference.ParseNamed` as the canonical source of truth in test assertions for validating parsing results.
        - Focus tests on application logic rather than custom parsing validation.
        - Simplify testing of invalid inputs by checking errors returned by `reference.ParseNamed`.
        - Increase test coverage by using known edge-case reference strings and verifying correct parsing via the library.

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

## Phase 3: Active Development - Linting & Refinement (Completed)

**Goal:** Systematically eliminate lint errors while ensuring all tests pass.

**Completed:**
- [✓] Fixed all test failures
- [✓] Addressed critical lint issues
- [✓] Refactored long functions (`funlen`) by extracting helpers
- [✓] Configuration update for funlen to use appropriate thresholds for production and test code

## Implementation Process:
- For each change:
  1. **Pre-Change Verification:**
     - Run targeted tests relevant to the component being modified
     - Run targeted linting to confirm issue (e.g., `golangci-lint run --enable-only=unused` for unused variables)
  2. **Make Required Changes:**
     - Follow KISS and YAGNI principles
     - Maintain consistent code style
     - Document changes in code comments where appropriate
  3. **Post-Change Verification:**
     - Run full test suite: `go test ./...`
     - Run targeted linting to confirm resolution
     - Run full linting: `golangci-lint run`