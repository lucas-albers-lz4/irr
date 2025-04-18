# TODO.md - Helm Image Override Implementation Plan

## Completed Phases

  - [x] Default behavior: fail if file exists (per section 4.4 in PLUGIN-SPECIFIC.md)
  - [x] Use file permissions constants for necessaary permission set, those are defined in this file : `pkg/fileutil/constants.go`

**P2: User Experience Enhancements**
- [ ] **[P2]** Implement Helm-consistent output formatting
  - [ ] Implement a logging utility that mimics Helm's log levels and color codes
  - [ ] Add a --debug flag to all commands, with verbose output when enabled
  - [ ] Write tests to verify log output format and color in TTY/non-TTY environments
  - [ ] Use ANSI color codes for log levels (INFO: cyan, WARN: yellow, ERROR: red)
  - [ ] For TTY detection, use the `github.com/mattn/go-isatty` package instead of manual detection
  - [ ] Ensure logging aligns with existing debug approach (controlled by --debug flag and IRR_DEBUG env var)
  - [ ] Maintain simple log format (without timestamps or explicit log levels in output) for consistency
- [ ] **[P2]** Add plugin-exclusive commands
  - [ ] Implement list-releases by calling `helm list` via SDK or subprocess
  - [ ] Parse and display image analysis results in a table format
  - [ ] Add circuit breaker to limit analysis to 300 releases
  - [ ] For interactive registry selection, detect TTY and prompt user; skip in CI
  - [ ] Skip interactive prompts if `CI` environment variable is set or not in TTY
- [ ] **[P2]** Create comprehensive error messages
  - [ ] Standardize error message format (prefix with ERROR/WARN/INFO)
  - [ ] Add actionable suggestions to common errors (e.g., missing chart source)
  - [ ] Implement credential redaction in all error/log output
  - [ ] When chart source is missing, print specific recovery steps (see section 5.4 in PLUGIN-SPECIFIC.md)

**P4: Get test coverage back to threshold after P0, P1, P2 feature work**

**P4: Documentation and Testing**
- [ ] **[P3]** Create plugin-specific tests
  - [x] Write unit tests for the adapter layer using Go's testing and testify/gomock
  - [x] Create integration tests using a local kind cluster and test Helm releases
  - [x] Add CLI tests for plugin entrypoint and command routing
  - [x] Test error handling and edge cases (e.g., missing release, invalid namespace)
  - [x] Test all error paths (every `if err != nil` block)
- [ ] **[P3]** Document Helm plugin usage
  - [ ] Write a dedicated section in docs/PLUGIN-SPECIFIC.md for plugin install/upgrade/uninstall
  - [ ] Add usage examples for each command, including edge cases
  - [ ] Document all plugin-specific flags and environment variables
  - [ ] Add troubleshooting section for common errors and recovery steps
  - [ ] Create a "Quickstart" section with install and usage examples
  - [ ] List all flags and environment variables in a reference table

**Cross-Cutting Best Practices**
- [x] Use KISS and YAGNI: avoid speculative features
- [x] Implement single source of truth for version (plugin.yaml)
- [x] Automate version propagation to pyproject.toml via Makefile
- [x] Inject version into Go binary at build time using linker flags
- [ ] Add code comments and docstrings for all exported functions and interfaces
- [ ] Add structured logging for all major operations (start, success, error)
- [ ] Schedule regular code and design reviews after each vertical slice
- [ ] Update documentation and onboarding materials after each review
- [ ] Emphasize non-destructive philosophy - never write to the cluster, only read and generate files
- [ ] Update docs/cli-reference.md to reflect any CLI or logging changes, ensuring documentation matches implementation (especially for debug/logging behavior)

**Developer Onboarding Checklist**
- [ ] Document required tools and environment setup (Go, Helm, kind, etc.)
- [x] Provide a makefile or scripts for common dev tasks (build, test, lint, install plugin)
- [ ] Add a quickstart guide for running and testing the plugin locally
- [ ] List all relevant docs (PLUGIN-SPECIFIC.md, DEVELOPMENT.md, TESTING.md) at the top of the section
- [x] Create makefile targets for: `build`, `test`, `lint`, `install-plugin`
- [x] Add step-by-step quickstart: clone repo, build, install plugin, run help command

## Phase 3 bugfix


    example commands to reproduce the problem : standalone : `bin/irr validate --chart-path /Users/lalbers/Library/Caches/helm/repository/cert-manager-v1.17.1.tgz --values cert-manager-overrides.yaml --kube-version 1.29.0 --debug`; and to test the plugin : ` helm irr validate cert-manager -n cert-manager --values cert-manager-overrides.yaml --debug` and test plugin with setting the kube-version : `helm irr validate cert-manager -n cert-manager --values cert-manager-overrides.yaml --debug --kube-version 1.29.0`

## Phase 5: `kind` Cluster Integration Testing
_**Goal:** Implement end-to-end tests using `kind` to validate Helm plugin interactions with a live Kubernetes API and Helm release state, ensuring read-only behavior._

- [ ] **[P1]** Set up `kind` cluster testing framework:
  - [ ] Integrate `kind` cluster creation/deletion into test setup/teardown
  - [ ] Implement Helm installation within the `kind` cluster
  - [ ] Define base RBAC for read-only Helm operations
- [ ] **[P1]** Create integration tests against live Helm releases:
  - [ ] Test core `inspect`, `override`, `validate` plugin commands against charts installed in `kind`
  - [ ] Utilize Helm Go SDK for interactions within tests where applicable
- [ ] **[P1]** Verify Read-Only Operations against `kind`:
  - [ ] Configure tests to run with limited, read-only Kubernetes/Helm permissions
  - [ ] Assert that tests with limited permissions fail if write operations are attempted
  - [ ] Verify Helm release state remains unchanged after plugin execution
- [ ] **[P1]** Test compatibility with latest Helm version in `kind`:
  - [ ] Set up CI configuration to run `
 
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
##END REMINDER On the Implementation Process: 