# TODO.md - Helm Image Override Implementation Plan

## 1. Project Setup (Completed)
*Initial project setup, Go modules, directory structure, dependencies, Makefile, and basic CI are complete.*

## 2. Core Implementation

### 2.1 Chart Loading (Completed)
*Loading charts from directories and archives, parsing `values.yaml` and `Chart.yaml`, and recursive subchart handling are implemented.*

### 2.2 Image Processing (Completed - Refined in Later Sections)
*Initial image detection heuristics, regex parsing, normalization, registry filtering, and sanitization were implemented. This was significantly refactored later (see Sections 2.7, 14).*

### 2.3 Path Strategy (Completed - Base Implementation)
*Implemented `prefix-source-registry` strategy and designed framework for future strategies. Refinements occurred later.*

### 2.4 Output Generation (Completed - Refined in Later Sections)
*Initial override structure generation and YAML output implemented. Significantly refactored later (see Sections 9, 13).*

### 2.5 Debugging and Logging (Mostly Completed)
- [x] Implemented debug package and added logging to most key functions.
- [ ] Add debug logging to `OverridesToYAML` function.
- [x] Added `--debug` flag to CLI.

### 2.6 Bug Fixes and Improvements (Completed - Historical)
*Addressed initial YAML output issues and basic error handling improvements. Non-image value transformation was superseded by Section 2.7.*

### 2.7 Refactor Image Detection Logic (Completed)
*Refactored image detection away from blacklisting towards context-aware positive identification using structural context and stricter string parsing. Deprecated `isNonImageValue`.*

### 2.8 Registry Mapping Support (Completed - Refined in Section 23)
*Added initial support for registry mappings via CLI flag and YAML file. This functionality is being consolidated and tested thoroughly in Section 23.*

## 3. CLI Interface (Completed - Base Implementation)
*Implemented core CLI flags (`cobra`), input validation, exit codes, and basic error messaging. Specific flags like `--dry-run` and `--strict` need further testing/implementation (see Section 19).*

## 4. Testing Implementation

### 4.1 Unit Tests (Completed - Base Coverage)
*Initial unit tests covering core logic (value traversal, detection, parsing, normalization, path strategy, override generation, YAML output) were implemented. More comprehensive tests added/planned in later sections (e.g., Sections 17, 23).*

### 4.2 Integration Tests (Completed - Base Coverage)
*Core use case integration test (`kube-prometheus-stack`) implemented and validated. Further integration test improvements and fixes are tracked in Section 23.*

### 4.3 Bulk Chart Testing (In Progress - Python Script)
*Initial Python script (`test-charts.py`) created for testing against diverse charts. Further development and stabilization tracked in Sections 10, 18.*
- [ ] Refine test script for stability and better error reporting (See Section 18 tasks if script needs further work beyond stabilization).
- [ ] Expand chart corpus and analyze results systematically (See Section 10).

### 4.4 Performance Testing (Pending)
- [ ] Setup benchmark infrastructure (e.g., using `go test -bench` and standard test environment).
- [ ] Create benchmark tests for key functions (`LoadChart`, `DetectImages`, `GenerateOverrides`, `LoadMappings`) using charts/data of varying complexity.
- [ ] Measure execution time (`time/op`) and memory usage (`allocs/op`, `B/op`).
- [ ] Establish baseline performance metrics.

## 5. Documentation (Partially Completed)
- [x] Core documentation (`README.md`, CLI Reference, Path Strategies, Examples) created.
- [ ] Create Troubleshooting / Error Codes guide (Leverage errors defined in `pkg/*/errors.go`).
- [ ] Add comprehensive Contributor Guide (`CONTRIBUTING.md` - setup, testing, contribution process - see also Section 23 Preventive Measures).
- [ ] Update documentation to reflect recent refactoring and consolidation (Ongoing - ensure accuracy after Section 23 completion).

## 6. Release Process (Pending)
- [ ] Set up Git tagging strategy (e.g., SemVer `vX.Y.Z`).
- [ ] Create release build automation using GitHub Actions (triggered by tags).
- [ ] Publish cross-platform binaries (Linux AMD64, macOS AMD64/ARM64) to GitHub Releases.
- [ ] Ensure documentation is up-to-date and published with release.

## 7. Stretch Goals (Post-MVP - Pending)
*Potential future enhancements after stabilization.* 
- [ ] Implement `flat` path strategy.
- [ ] Implement multi-strategy support (different strategy per source registry).
- [ ] Add configuration file support (`--config`) for defining source/target/exclusions/custom patterns.
- [ ] Enhance image identification heuristics (e.g., custom key patterns via config).
- [ ] Improve handling of digest-based references (more robust parsing).
- [ ] Add comprehensive private registry exclusion patterns (potentially beyond just source registry name).
- [ ] Implement validation of generated target URLs (basic format check).
- [ ] Explore support for additional target registries (Quay, ECR, GCR, ACR, GHCR) considering their specific path/naming constraints.

## 8. Post-Refactor Cleanup & Fixes (Completed - Historical)
*Addressed specific normalization, sanitization, parsing, and test environment issues identified after initial refactoring. Subsequent issues tracked in later sections.*

## 9. Post-Refactor Override Generation Debugging & Fix (Completed - Historical)
*Investigated and implemented an initial fix for override generation structure issues. Superseded by the more robust path-based modification in Section 13.*

## 10. Systematic Helm Chart Analysis & Refinement (In Progress)
*Focuses on data-driven improvement by analyzing a large corpus of Helm charts.*
- [ ] **Test Infrastructure Enhancement:** Implement structured JSON result collection for `test-charts.py`.
- [x] **Chart Corpus Expansion:** Expanded chart list in `test/tools/test-charts.py`.
- [ ] **Corpus Maintenance:** Document chart selection criteria, implement automated version update checks.
- [ ] **Automated Pattern Detection:** Implement detectors (regex/AST?) for value structures (explicit maps, strings, globals, lists, non-image patterns) in `test-charts.py` or a separate Go tool.
- [ ] **Frequency & Correlation Analysis:** Develop tools/scripts to count patterns and identify correlations across the corpus results.
- [ ] **Schema Structure Analysis:** Implement tools to automatically extract and compare `values.schema.json` where available. Document common patterns and provider variations.
- [ ] **Data-Driven Refactoring Framework:** Define metrics (coverage, complexity, compatibility), create decision matrix template to guide future refactoring based on analysis results.
- [ ] **Container Array Pattern Support:** Add explicit support and test cases for `spec.containers`, `spec.initContainers` (Partially addressed in Section 14.3, verify coverage in Section 23 tests).
- [x] **Image Reference Focus:** Scope clarified to focus only on registry location changes.

## 11. Analyzer Refinement & Expanded Testing (Completed)
*Improved analyzer robustness (handling missing registry/tags) and expanded the test chart list in `test-charts.py`.*

## 12. Override Generation Testing & Refinement (Completed - Historical)
*Adapted `test-charts.py` for override testing and performed initial analysis. Superseded by Section 13 and subsequent test/fix cycles.*

## 13. Refactor Override Generation (Path-Based Modification) (Completed)
*Successfully implemented the core path-based override generation logic, significantly improving override accuracy for complex charts.*
- **Summary:** Achieved high analysis success rate (98%), generated valid overrides for numerous complex charts.

## 14. Image Detection and Override Generation Improvements (Completed - Addressed Core Issues)
*Refined image detection (consistency, partial maps, globals, templates, non-image types) and override generation (arrays, context) based on analysis. Addressed boolean/numeric value handling and path resolution.*
- **Note:** While core logic was implemented, remaining bugs and edge cases are addressed in Section 23 test failures.

## 15. Chart Testing Improvements (`test-charts.py`) (Completed - Significant Updates)
*Addressed major issues in the Python test script: corrected command syntax, enhanced default values (Bitnami), improved error categorization, implemented caching, added filtering options, and enhanced results analysis.*

## 16. Hybrid Chart Classification for Test Configuration (`test-charts.py`) (Completed)
*Implemented classification logic in the Python script to apply tailored default `values.yaml` content during `helm template` validation, reducing template errors.*

## 17. Comprehensive Test Case Improvements (Completed - Base Coverage)
*Expanded unit tests across key packages (`pkg/image`, `pkg/override`, `pkg/strategy`, `cmd/irr`) covering complex structures, edge cases, context variations, and CLI validation. Further test fixing tracked in Section 23.*

## 18. Python Test Script (`test-charts.py`) Stabilization (Completed)
*Fixed chart extraction, completed override generation command execution, implemented `helm template` validation step, and improved chart pulling robustness within the Python test script.*

## 19. Implement and Test CLI Flags (`--dry-run`, `--strict`) (Pending)
*Implementation and testing of `--dry-run` and `--strict` flags is pending.*
- [ ] **Define Behavior:** Review and potentially refine documented behavior in `DEVELOPMENT.md` or CLI reference, ensuring clarity on exit codes, output (stdout vs. file), and error handling specifics.
- [ ] **Unit Tests:**
    *   [ ] Add tests for CLI argument parsing of both flags.
    *   [ ] Add tests for core logic (mocking file I/O): verify `--dry-run` prevents writes, verify `--strict` triggers specific Exit Code 5 on defined unsupported structures (e.g., templated repository?), verify successful exit code (0) when no issues occur.
- [ ] **Integration Tests:**
    *   [ ] Fix `TestDryRunFlag` (ensure binary path correct, check exit code 0, assert no file created, assert specific preview output to stdout).
    *   [ ] Fix `TestStrictMode` (ensure `unsupported-test` chart triggers the flag, check exit code 5, assert specific error message). (Part of Section 23 debugging).
- [ ] **Code Implementation:** Review/update code in `cmd/irr/main.go` (and potentially `pkg/` libraries) for conditional logic related to file writing (`--dry-run`) and error/exit code handling (`--strict`).

## 20. Address Test and Lint Failures (April 6th) (Completed - Historical)
*Addressed specific test failures and linter errors identified on April 6th. Subsequent issues tracked in later sections.*

## 21. Code Organization Refactoring and Error Handling Improvements (Completed - Historical)
*Consolidated error definitions in `pkg/image`, fixed related linter issues, and began addressing test failures. Superseded by the more comprehensive consolidation and fixing plan in Section 23.*

## 22. Fix Current Lint and Test Failures (April 6th - Post Refactor) (Completed - Historical)
*Addressed further test failures and linter errors identified after the Section 21 refactoring. Superseded by the consolidated plan in Section 23.*

## 23. Consolidate Registry Logic, Fix Tests & Linter Issues (Consolidated Plan)

**Goal:** Achieve a stable codebase by consolidating duplicated registry logic, fixing all test failures, and addressing outstanding linter warnings.

**Priority Order & Detailed Steps:**

1.  **Consolidate Registry Packages (Critical Priority)**
    *   **Rationale:** Duplicated functionality in `pkg/registry` and `pkg/registrymapping` is the root cause of current lint errors and likely contributes to test failures. Consolidation simplifies maintenance and reduces potential bugs.
    *   **Decision Criteria:**
        *   Compare implementations:
            -   `pkg/registry`: Uses `yaml.v3`, potentially better error wrapping.
            -   `pkg/registrymapping`: Uses `sigs.k8s.io/yaml` (aligns with other project dependencies), more recent development focus.
        *   **Action:** Choose `pkg/registrymapping` as the base due to alignment with project dependencies and recent focus. We will migrate its functionality *into* `pkg/registry` and enhance it.
    *   **Implementation Steps:**
        1.  [ ] **Prepare `pkg/registry`:**
            *   Rename existing `pkg/registry/mappings.go` to `pkg/registry/mappings_legacy.go` (temporary).
            *   Rename existing `pkg/registry/mappings_test.go` to `pkg/registry/mappings_legacy_test.go` (temporary).
            *   Keep `pkg/registry/errors.go` as the canonical error definition source.
        2.  [ ] **Migrate `pkg/registrymapping` to `pkg/registry`:**
            *   Move `pkg/registrymapping/mappings.go` to `pkg/registry/mappings.go`.
            *   Update the package declaration in the moved file to `package registry`.
            *   Update the migrated code to use errors defined in `pkg/registry/errors.go`.
            *   Review the migrated `LoadMappings` function: Ensure it correctly parses the expected `map[string]string` YAML format and converts it to `[]Mapping` (or `[]RegistryMapping` - ensure type consistency). Reconcile any differences with the legacy implementation if necessary.
            *   Review the migrated `GetTargetRegistry` function: Ensure it uses `image.NormalizeRegistry` correctly and handles edge cases (nil maps, no match).
        3.  [ ] **Consolidate Tests:**
            *   Move `pkg/registrymapping/mappings_test.go` to `pkg/registry/mappings_test.go`.
            *   Update the package declaration in the moved test file.
            *   Merge relevant test cases and fixtures from `mappings_legacy_test.go` into the new `mappings_test.go`. Prioritize tests covering `LoadMappings` and `GetTargetRegistry`.
            *   Update test code to use the consolidated types and errors from `pkg/registry`.
            *   Fix the `undefined: RegistryMappings` / `undefined: RegistryMapping` errors by ensuring the test uses the correct type names defined in `pkg/registry/mappings.go` (likely `Mappings` and `Mapping`).
        4.  [ ] **Update Codebase Imports:**
            *   Search the entire codebase (`cmd/`, `pkg/`) for imports of `pkg/registrymapping`.
            *   Replace all instances with imports of `pkg/registry`.
            *   Adjust code using the imported package if type names or function signatures differ slightly after consolidation (e.g., `registrymapping.RegistryMappings` vs `registry.Mappings`).
        5.  [ ] **Cleanup:**
            *   Run `go test ./pkg/registry/...` - Ensure all tests in the consolidated package pass.
            *   Run `golangci-lint run ./pkg/registry/...` - Ensure no lint errors remain in the package.
            *   Delete the temporary legacy files (`mappings_legacy.go`, `mappings_legacy_test.go`).
            *   Delete the `pkg/registrymapping` directory.
        6.  [ ] **Documentation:** Update any internal documentation referencing the old package structure.

2.  **Fix Core Logic Unit Test Failures (High Priority)**
    *   **`pkg/registry` (Post-Consolidation):**
        *   [ ] Ensure comprehensive test coverage exists after merge (Address items from old Section 23):\n            *   [ ] Verify `LoadMappings` tests cover: valid/invalid paths, path traversal, non-existent files, invalid YAML, empty files.\n            *   [ ] Verify `GetTargetRegistry` tests cover: basic mapping, normalization, nil/empty maps, no match, carriage returns.\n            *   [ ] Verify test fixtures cover all scenarios.\n            *   [ ] Verify error handling coverage for all defined errors.
    *   **`pkg/image`:**
        *   [ ] Address remaining `TestImageDetector*` and `TestDetectImages*` failures:\n            -   [ ] `TestImageDetector_ContainerArrays`: Fix `ImageReference.Path` expectation in the test assertion.\n            -   [ ] `TestDetectImages/Strict_mode`: Debug strict detection logic for invalid strings & update test expectation.\n            -   [ ] `TestDetectImages/With_starting_path`: Adjust test expectation (seems `startingPath` might be ignored). Investigate `startingPath` propagation if behavior is incorrect.
        *   [ ] Verify `NormalizeImageReference` behavior (Docker Library, defaults) passes relevant tests.
    *   **`pkg/strategy`:**
        *   [ ] Debug `TestPrefixSourceRegistryStrategy_GeneratePath_WithMappings`. Verify interaction with the *consolidated* `pkg/registry` package. Ensure mappings are loaded and applied correctly *before* path generation logic.
    *   **`pkg/chart` & `pkg/generator`:**
        *   [ ] Debug `TestGenerate/*` failures in `pkg/chart/generator_test.go`.\n        *   [ ] Verify `Generator.Generate` logic interacts correctly with the consolidated `pkg/registry` and fixed `pkg/image`, `pkg/strategy`.\n        *   [ ] Add proper error and map assertions in `TestGenerate` and `TestGenerate_WithMappings` to fix `errcheck` warnings.

3.  **Fix Command Layer & Integration Test Failures (Medium Priority)**
    *   **`cmd/irr`:**
        *   [ ] Ensure all command logic uses the consolidated `pkg/registry`.\n        *   [ ] Debug `TestOverrideCmdArgs/invalid_path_strategy`.\n        *   [ ] Debug `TestOverrideCmdExecution/success_with_registry_mappings`. Verify mapping file loading using the consolidated package.\n        *   [ ] Debug `TestOverrideCmdExecution/success_with_output_file_(flow_check)`.\n        *   [ ] Run `go test ./cmd/irr/... -v` after fixing dependencies.
    *   **`test/integration`:**
        *   [ ] Ensure integration tests use the consolidated `pkg/registry` where applicable.\n        *   [ ] Re-run `go test ./test/integration/... -v` after unit tests pass.\n        *   [ ] Debug `TestStrictMode`: Verify error propagation and exit code (likely Exit Code 5).\n        *   [ ] Debug `TestRegistryMappingFile`: Verify test setup uses consolidated package correctly.\n        *   [ ] Debug `TestCertManagerIntegration`, `TestComplexChartFeatures`, `TestReadOverridesFromStdout`.

4.  **Address Linter Warnings (Medium Priority)**
    *   [ ] Fix `errcheck` warning: Check `os.Remove` error in `cmd/irr/override_test.go`.\n    *   [ ] Fix remaining straightforward `revive` warnings:\n        *   [ ] `package-comments`: Add missing comments.\n        *   [ ] `unused-parameter`: Rename to `_`.\n        *   [ ] `exported: comment ... should be of the form`: Fix format/add comments.\n        *   [ ] `exported: exported const ... should have comment`: Add comment.\n        *   [ ] `empty-block`: Remove empty loop.
    *   [ ] Fix remaining `staticcheck` / `unused` warnings (e.g., `S1005`, `digestRegexCompiled`).
    *   [ ] Run `golangci-lint run --config=.golangci.yml --fix ./...` periodically and address new issues.
    *   [ ] **Defer:** `revive: exported: type name ... stutters`. Consider renaming types like `ImageReference` to `Reference` in a separate, dedicated refactoring effort later.
    *   [ ] **Defer:** `errcheck` for `fmt.Fprintln`/`fmt.Fprintf` in `cmd/irr/root.go` unless causing specific issues.

5.  **Implement Preventive Measures & Best Practices (High Priority - Ongoing)**
    *   *(Consolidated from previous sections)*
    *   [ ] **Enhance Test Coverage:**
        *   [ ] Add coverage thresholds to CI pipeline (`codecov` or similar).
        *   [ ] Aim for >80% coverage in core packages (`pkg/registry`, `pkg/image`, `pkg/strategy`, `pkg/chart`).
    *   [ ] **Establish Documentation & Guidelines:**
        *   [ ] Create/update `CONTRIBUTING.md`: Include package responsibilities, error handling patterns, testing strategy (unit, integration, e2e), guideline for avoiding package duplication.
        *   [ ] Improve code comments and docstrings, especially for public APIs and complex logic.

**Implementation Notes:**
*   Address steps strictly in priority order. Package consolidation blocks most other fixes.
*   Run `go test ./...` and `golangci-lint run ...` frequently after changes to catch regressions early.
*   Commit changes incrementally with clear messages referencing the TODO items being addressed.
*   Keep documentation (README, CONTRIBUTING, code comments) updated alongside code changes.
