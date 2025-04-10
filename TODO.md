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
- [✓] All tests are now passing successfully
- [✓] Fixed error-wrapping directive issues (`%w` → `%v` in t.Fatalf calls)
- [✓] Fixed non-constant format string in fmt.Errorf calls
- [✓] Down to only 9 remaining lint issues from 30+

**Completed Items:**
- [✓] Extracted "latest" string to constants in pkg/image/normalization.go
- [✓] Added package comments to internal/helm and pkg/analyzer
- [✓] Renamed AnalyzerConfig to Config to avoid analyzer.AnalyzerConfig stutter
- [✓] Updated all octal literals to modern 0o prefix syntax
- [✓] Added constants for file permission modes, percentages, and other magic numbers
- [✓] Fixed all gocritic issues in test/integration package

**Remaining Issues (9 total):**
1. **Function Length (1 issue):**
   - `ValidateOverrides()` in test/integration/harness.go has 80 statements (limit: 65)

2. **Functions with Too Many Results (2 issues):**
   - `setupGeneratorConfig()` in cmd/irr/override.go
   - `validateMapStructure()` in pkg/image/detector.go

3. **Commented-Out Code (6 issues):**
   - cmd/irr/root.go:295
   - Four instances in pkg/chart/generator_test.go
   - pkg/strategy/path_strategy.go:122

**Implementation Plan:**

1. **Next Wave - Address Remaining Issues (1 day):**
   - [ ] Remove or replace commented-out code (6 issues)
   - [ ] Refactor functions with too many results (2 issues)
   - [ ] Consider exempting test harness function from funlen check (1 issue)

2. **Final Wave - Documentation Cleanup (1 day):**
   - [ ] Add proper comments to remaining exported items
   - [ ] Fix unused parameters by using _ prefix
   - [ ] Finalize and run full test suite

**General Approach:**
- Focus on high-impact issues first (commented-out code, functions with too many results)
- Keep running tests after each change
- Maintain consistent code style across the codebase