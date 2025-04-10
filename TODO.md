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

## Phase 3: Active Development - Linting & Refinement (Completed)

**Goal:** Systematically eliminate lint errors while ensuring all tests pass.

**Completed:**
- [✓] Fixed all test failures
- [✓] Addressed critical lint issues
- [✓] Refactored long functions (`funlen`) by extracting helpers
- [✓] Configuration update for funlen to use appropriate thresholds for production and test code

## Phase 4: Current Lint Cleanup and Code Quality Improvements (In Progress)

**Goal:** Address all remaining linter issues to improve code quality while maintaining functionality.

**Current Status:**
- All tests are passing
- Funlen issues resolved with appropriate thresholds
- Significant progress on fixing lint issues

**Current Lint Issues (Completed/In Progress):**
1. **High Priority Issues:**
   - **goconst (0/1 completed):**
     - [✓] Extracted "latest" string to constants in pkg/image/normalization.go
   - **Package Documentation (2/2 completed):**
     - [✓] Added package comments to internal/helm
     - [✓] Added package comments to pkg/analyzer
   - **Stuttering Types (1/1 completed):**
     - [✓] Renamed AnalyzerConfig to Config to avoid analyzer.AnalyzerConfig stutter 
   - **Octal Literals (5/5 completed):**
     - [✓] Updated all octal literals to modern 0o prefix syntax
   - **Magic Numbers (10/10 in progress):**
     - [✓] Added constants for file permission modes (0o755, 0o644)
     - [✓] Added constants for percentage calculations
     - [✓] Added constant for image path component splitting
     - [✓] Added constant for tag length validation

2. **Remaining Issues (For Next Wave):**
   - **revive (30+ issues):** 
     - Exported items missing proper comments
     - Unused parameters in functions
     - Code flow issues (if-else structure, etc.)
   - **gocritic (30+ issues):**
     - Commented out code blocks
     - if-else chains that should be switch statements
     - Functions with too many results
     - Unnamed return values
   - **Lower Priority Issues:** 
     - Long lines (lll)
     - Security issues in test code (gosec)

**Implementation Plan:**

1. **Next Wave - Code Structure Improvements (1-2 days):**
   - [ ] Convert if-else chains to switch statements
   - [ ] Remove or replace commented-out code
   - [ ] Add names to return values where appropriate
   - [ ] Refactor functions with too many results
   - [ ] Add proper comments to exported items

2. **Final Wave - Code Cleanup (1 day):**
   - [ ] Fix unused parameters by using _ prefix
   - [ ] Fix long lines by breaking into multiple lines
   - [ ] Address security issues in test harness

**General Approach:**
- Continue focusing on one linter category at a time 
- Run tests after each significant change to verify functionality
- Address issues package by package for consistency