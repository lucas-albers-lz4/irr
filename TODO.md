# TODO.md - Helm Image Override Implementation Plan

## 1-6. Initial Setup & Core Implementation (Completed)
*Project setup, core Go implementation (chart loading, initial image processing, path strategy, output generation, CLI interface), debugging/logging, initial testing (unit, integration), and documentation foundations are complete.*

## 7. Stretch Goals (Post-MVP - Pending)
*Potential future enhancements after stabilization.*
- [ ] Implement `flat` path strategy
- [ ] Implement multi-strategy support (different strategy per source registry)
- [ ] Add configuration file support (`--config`) for defining source/target/exclusions/custom patterns
- [ ] Enhance image identification heuristics (e.g., custom key patterns via config)
- [ ] Improve handling of digest-based references (more robust parsing)
- [ ] Add comprehensive private registry exclusion patterns
- [ ] Implement validation of generated target URLs
- [ ] Explore support for additional target registries (Quay, ECR, GCR, ACR, GHCR)
- [ ] Enhance strategy validation and error handling

## 8-9. Post-Refactor Historical Fixes (Completed)
*Addressed issues related to normalization, sanitization, parsing, test environments, and override generation structure.*

## 10. Systematic Helm Chart Analysis & Refinement (In Progress)
- [ ] **Test Infrastructure Enhancement:** Implement structured JSON result collection for `test-charts.py`
- [x] **Chart Corpus Expansion:** Expanded chart list in `test/tools/test-charts.py`
- [ ] **Corpus Maintenance:** Document chart selection criteria, implement automated version update checks
- [ ] **Automated Pattern Detection:** Implement detectors for value structures
- [ ] **Frequency & Correlation Analysis:** Develop tools for pattern analysis
- [ ] **Schema Structure Analysis:** Extract and compare `values.schema.json`
- [ ] **Data-Driven Refactoring Framework:** Define metrics and decision matrix
- [ ] **Container Array Pattern Support:** Add explicit support for `spec.containers`, `spec.initContainers`
- [x] **Image Reference Focus:** Scope clarified to focus only on registry location changes

## 11-18. Refinement & Testing Stabilization (Completed)
*Improved analyzer robustness, refined override generation, enhanced image detection, and expanded test coverage.*

## 19. CLI Flags Implementation (In Progress)
- [x] **Exit Code Organization:**
  - 0: Success
  - 1-9: Input/Configuration Errors
  - 10-19: Chart Processing Errors (including ExitUnsupportedStructure = 12)
  - 20-29: Runtime Errors
- [x] **Integration Tests:** Fixed `TestStrictMode` and verified exit code handling
- [ ] **Unit Tests:** Add tests for CLI argument parsing and core logic
- [ ] **Code Implementation:** Review/update code for file writing (`--dry-run`) and error handling (`--strict`)

## 20-24. Code Quality & Maintainability (In Progress)
- [x] Refactored large Go files in `pkg/image/detection.go` into focused components
- [x] Consolidated registry logic into `pkg/registry`
- [x] Fixed core unit tests
- [x] Fixed log package usage in root.go (converted log.GetInstance().Error() to log.Errorf())
- [ ] Address remaining `funlen` warnings for large functions
- [ ] Fix remaining linter errors (errcheck, errorlint, wrapcheck)
- [ ] Remove unused code
- [ ] Improve code style and documentation

## 25-26. Final Stabilization
- [x] Fixed command structure in `cmd/irr`
- [x] Consolidated exit code handling
- [x] Improved error handling consistency
- [ ] Complete high priority linting fixes (FAILED: Issue with `text/tabwriter` in `cmd/irr/root.go` - fixed in latest commit)
- [ ] Add new test cases for edge cases and error handling
- [ ] Update documentation with debug flag usage and linting guidelines

Note: The debug flag (`-debug` or `DEBUG=1`) can be used during testing and development to enable detailed logging.