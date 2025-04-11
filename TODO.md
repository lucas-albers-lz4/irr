# TODO.md - Helm Image Override Implementation Plan

## Phase 1: Core Implementation & Stabilization (Completed)

## Phase 2: Configuration & Support Features (Completed)
### Completed Features:
- [x] Core configuration and validation features implemented
  - Configuration file support with YAML registry mapping
  - Private registry exclusion patterns
  - Target URL validation
  - Enhanced error handling and test infrastructure

- [x] Enhance image identification heuristics (config-based patterns)
  - Design pattern-based identification system for complex charts
  - Implement configurable heuristics for non-standard image references
  - Add tests for edge cases and unusual image formats

- [x] Improve digest-based reference handling
  - Enhance support for SHA-based image references 
  - Add validation for digest format integrity
  - Include test cases for digest-based references

## Phase 3: Component-Group Testing Framework (Completed)
### Phase 3.0: Core Framework Implementation (Completed)
- [x] Design component-group testing approach for complex charts
  - Create testing approach documentation in TESTING-COMPLEX-CHARTS.md
  - Define component grouping strategy for multi-component charts
  - Implement test harness support for component-specific validation

- [x] Implement cert-manager component-group tests
  - Create test implementation for critical components group
  - Add support for validation thresholds per component group
  - Ensure high-quality error messaging and diagnostics

- [x] Document testing approach for complex charts
  - Document component-group testing strategy
  - Add usage examples to documentation
  - Include contribution guidelines for test implementations

### Phase 3.1: Extended Chart Support (Completed)
- [x] Implement kube-prometheus-stack component-group tests
  - Create test implementation for each logical component group
  - Create override values files for component testing
  - Add Makefile targets for component-specific testing

## Phase 4: Advanced Features & DevOps Integration (Pending)
### Planned Work:
- [ ] Implement CI/CD templates for common workflows
  - Add GitHub workflow examples
  - Create GitLab CI/CD pipeline template
  - Document integration patterns

- [ ] Create pre-commit hooks for validation
  - Implement pre-commit hook for chart validation
  - Add documentation for local developer setup

- [ ] Add CLI performance improvements
  - Implement parallel processing for large charts
  - Optimize memory usage for template processing
  - Add progress reporting for long-running operations

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
  
  5. **Git Commit:**
     - Stop after completing a logical portion of a feature to make well reasoned git commits with changes and comments
     - Request suggested git commands for committing the changes
     - Review and execute the git commit commands yourself