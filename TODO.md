# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)
*Initial project setup, core Go implementation (chart loading, image processing, path strategy, output generation, CLI), testing foundations, documentation, historical fixes, and testing stabilization are complete.*

## Phase 2: Stretch Goals (Post-MVP - Pending)
*Potential future enhancements after stabilization.*
- [ ] Implement `flat` path strategy
- [ ] Implement multi-strategy support (different strategy per source registry)
- [ ] Add configuration file support (`--config`)
- [ ] Enhance image identification heuristics (config-based patterns)
- [ ] Improve digest-based reference handling
- [ ] Add comprehensive private registry exclusion patterns
- [ ] Implement target URL validation
- [ ] Explore support for additional target registries (Quay, ECR, GCR, ACR, GHCR)
- [ ] Enhance strategy validation and error handling

## Phase 2.5: Quality & Testing Enhancements (Pending)
- [ ] **Leverage `distribution/reference` for Robustness:**
    - **Goal:** Enhance program quality and test accuracy by fully utilizing the `github.com/distribution/reference` library.
    - **Program Quality Improvements:**
        - Ensure strict compliance with the official Docker image reference specification.
        - Inherit robustness by using the library's handling of edge cases (e.g., implicit registries, ports, digests, tags).
        - Improve code clarity and type safety using specific types (`reference.Named`, `reference.Tagged`, `reference.Digested`) and methods (`Domain()`, `Path()`, `Tag()`, `Digest()`).
        - Benefit from built-in normalization (e.g., adding `docker.io`, `latest` tag).
        - Reduce maintenance by relying on the upstream library for spec updates.
        - **Action:** Review `pkg/image/parser.go` and potentially remove the `parseImageReferenceCustom` fallback if all necessary cases are covered by the official parser in non-strict mode, simplifying the codebase.
    - **Test Case Improvements:**
        - Use `reference.ParseNamed` as the canonical source of truth in test assertions for validating parsing results.
        - Focus tests on application logic rather than custom parsing validation.
        - Simplify testing of invalid inputs by checking errors returned by `reference.ParseNamed`.
        - Increase test coverage by using known edge-case reference strings and verifying correct parsing via the library.
        - **Action:** Refactor existing tests in `pkg/image/parser_test.go` and `pkg/image/detection_test.go` to use `distribution/reference` for validation where applicable.

## Phase 3: Active Development - Linting & Refinement (In Progress)

**Goal:** Systematically eliminate lint errors while ensuring all tests pass.

**Current Status & Blocking Issues:**
*   **Test Failures:** All issues resolved
    *   **Priority:** Focus on remaining lint errors.

*   **Lint Errors:** `make lint` still reports issues across multiple categories
    *   **Medium Priority:**
        - Long functions (`funlen`): Many functions still exceed the 60-line limit (test functions) or 40-statement limit (regular functions)
        - Style issues (`gocritic`): Various style issues including commented-out code, octal literals, and if-else chains
        - Revive issues (`revive`): Missing comments for exported functions, error string formatting
    *   **Lower Priority:**
        - Long lines (`lll`): Lines exceeding 140 characters
        - Magic numbers (`mnd`): Hardcoded numbers that should be constants
        - `goconst`: Extract repeated string literals to constants (1 issue) 
        - `gosec`: Address security-related issues (1 issue)

**Next Steps:**
1. Continue addressing medium priority lint issues (`funlen`, `gocritic`, remaining `revive`)
2. Address remaining lower priority lint issues
3. Run final verification to ensure all tests pass and lint errors are resolved

**Completed Linting Progress:**
- [✓] **Critical Issues:** Fixed all high-priority items (`errcheck`, `nil-nil`, `unused`, `staticcheck`, `dupl`)
- [✓] **Test Fixes:** Fixed all failing tests in image package and integration tests
- [✓] **ParseImageReference Consolidation:** Successfully consolidated to use a single robust implementation
- [✓] **Partial Fixes:** 
  - Refactored several long functions by extracting helpers
  - Fixed unused parameters by replacing with underscores
  - Replaced some magic numbers with named constants
  - Fixed long lines by breaking them into multiple lines
  - Fixed additional staticcheck issues (switch statements, error string formatting)

## Phase 3.5: ParseImageReference Consolidation (Completed)

**Implementation Status:**
- [✓] Removed the unused `ParseImageReference` function from `pkg/override/override.go`
- [✓] Verified all tests pass successfully after the change
- [✓] Successfully consolidated to use single robust implementation from `pkg/image/parser.go`
- [✓] Fixed failing test in the image package by updating error message in strict mode parsing

## Phase 4: Lint Cleanup and Code Quality Improvements (In Progress)

**Goal:** Systematically address remaining lint errors to improve code quality while maintaining functionality.

**Current Status:**
- All tests are now passing
- Making steady progress on `funlen` issues by refactoring test functions and core code
- Reduced remaining `funlen` issues in cmd/irr package from 7 to 3

**Linting Plan:**
1. **Medium Priority Issues (Current Focus):**
   - [ ] `funlen`: Refactor long functions into smaller, focused helpers
      - [✓] Refactored `TestAnalyzeCmd` in `cmd/irr/analyze_test.go` by extracting test case definitions
      - [✓] Refactored `TestOverrideCmdExecution` in `cmd/irr/override_test.go` by extracting test case definitions
      - [✓] Refactored `TestOverrideCommand_DryRun` in `cmd/irr/override_test.go` by extracting setup and assertion helpers
      - [✓] Refactored `runOverride` in `cmd/irr/override.go` by extracting smaller helper functions:
         - Added `getRequiredFlags`, `getStringFlag`, `getBoolFlag`, `getStringSliceFlag`, `getThresholdFlag` for flag handling
         - Added `handleGenerateError` for error classification
         - Added `outputOverrides` for handling file/console output
         - Added `setupGeneratorConfig` to consolidate configuration setup
      - [✓] Refactored `runAnalyze` in `cmd/irr/root.go` by extracting helper functions:
         - Added `formatJSONOutput` for JSON output formatting
         - Added `formatTextOutput` for text output formatting
         - Added `writeAnalysisOutput` for handling file/console output
      - [✓] Refactored `TestOverrideCmdArgs` in `cmd/irr/override_test.go` by extracting helper functions:
         - Added `setupDryRunTestEnvironment` for test environment setup
         - Added `assertExitCodeError` for error checking
      - [✓] Refactored `setupOverrideTestEnvironment` in `cmd/irr/override_test.go` by extracting helper functions:
         - Added `setupTestEnvironmentVars` for environment variable setup
         - Added `setupTestMockGenerator` for mock generator setup
      - [ ] Continue refactoring remaining long test functions
      - [ ] Refactor complex functions exceeding 40 statements
   - [ ] `gocritic`: Fix style issues including commented-out code and if-else chains
   - [ ] `revive`: Fix remaining issues with missing comments and error string formatting

2. **Lower Priority Issues:**
   - [ ] `lll`: Fix remaining long lines by breaking them logically
   - [ ] `mnd`: Replace remaining magic numbers with named constants
   - [ ] `goconst`: Extract repeated string literals to constants
   - [ ] `gosec`: Fix security-related issues in test code

**General Workflow:**
1. Focus on refactoring test functions to use table-driven tests and test helpers
2. Extract common patterns in large functions into separate helper functions
3. Verify tests pass after each major refactoring

**Next Implementation:**
- Continue refactoring remaining test functions with funlen issues:
  - `defineOverrideCmdExecutionTests` in `cmd/irr/override_test.go`
  - `TestOverrideCommand_Success` in `cmd/irr/override_test.go`