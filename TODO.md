# TODO.md - Helm Image Override Implementation Plan

# Usability Improvement , CLI Refactor
## Implementation Sequence Notes
- Phase 0 (Configuration) should be completed before Phase 3.5 registry handling
- Phase 2 (Flag Consistency) should be completed before Phase 3.5 file naming standardization
- Phase 3 (Output Behavior) should be completed before implementing strict mode in Phase 3.5 

## Phase 0: Configuration Setup (P0: Critical Usability)
- [x] **[P0]** Implement `helm irr config` command (flag-driven only)
  - [x] Allow user to set or update a mapping via flags (e.g., `--source quay.io --target harbor.local/quay`)
  - [x] If the source already exists, update its target; otherwise, add a new mapping
  - [x] No validation of endpoints; user is responsible for correctness
  - [x] When running `inspect`, suggest likely mappings based on detected registries in the environment
  - [x] Document that override/validate can run without config, but correctness depends on user configuration
- [x] **[P0]** Analyze existing error code usage
  - [x] Document current error codes and their conditions
  - [x] Identify gaps in error handling
  - [x] Create mapping between planned new error codes and existing ones

### Phase 1: Flag Cleanup (P0: User Experience Enhancements)
- [x] **[P0]** Remove unused or confusing flags
  - [x] Remove `--output-format` flag (Not used, always YAML)
  - [x] Remove `--debug-template` flag (Not implemented/used) 
  - [x] Remove `--threshold` flag (No clear use case; binary success preferred)
  - [x] Hide or remove `--strategy` flag (Only one strategy implemented; hide/remove for now)
  - [x] Hide or remove `--known-image-paths` flag (Not needed for most users; hide or remove)
- [x] **[P0]** Flag cleanup verification
  - [x] Review and update/remove any test cases using these flags
  - [x] Test and lint after each change
  - [x] Update CLI and other documentation to remove references to these flags

### Phase 2: Flag Consistency and Defaults (P0: User Experience Enhancements)
- [x] **[P0]** Unify `--chart-path` and `--release-name` behavior
  - [x] Allow both flags to be used together
  - [x] Implement auto-detection when only one is provided
  - [x] Document precedence rules
  - [x] Default to `--release-name` in plugin mode; default to `--chart-path` in standalone mode
- [x] **[P0]** Implement mode-specific flag presentation
  - [x] Tailor help output and flag requirements based on execution mode
  - [x] Make `--release-name` primary in plugin mode
  - [x] Make `--chart-path` primary in standalone mode
  - [x] Try to keep flags in integration test mode the same.
    I think we don't care what flags it displays in integration test mode as integration test mode is designed to mock standalone and plugin mode so we should try and reduce any code that makes it differ unless we need it for logging or test framework execution.

- [x] **[P0]** Standardize `--namespace` behavior
  - [x] Make `--namespace` always optional
  - [x] Default to "default" namespace when not specified
- [x] **[P0]** Make `--source-registries` optional in override when config or auto-detection is present

### Phase 3: Output File Behavior Standardization (P0: User Experience Enhancements)
- [x] **[P0]** Standardize `--output-file` behavior across commands
  - [x] `inspect`: Default to stdout if not specified
  - [x] `override`: 
    - [x] Default to stdout in standalone mode
    - [x] Default to `<release-name>-overrides.yaml` in plugin mode with release name
    - [x] Ensure explicit override with `--output-file` always works
    - [x] Implement check to never overwrite existing files without explicit user action (e.g., `--force`; We don't have a `--force` command or plan to implement it)
  - [x] `validate`: Only write file output when `--output-file` is specified
- [x] **[P0]** Implement consistent error handling for file operations
  - [x] Add clear error messages when file operations fail
  - [x] Implement uniform permission handling across all commands

### Phase 3.5: Streamlined Workflow Implementation (P1: User Experience Enhancements)
- [x] **[P1]** Implement enhanced source registry handling
  - [x] Add explicit `--all-registries` flag for clarity
  - [x] Implement auto-detection of registries with clear user feedback
  - [x] Use two-stage approach for registry auto-detection:
    - [x] In `inspect`: Auto-detect and show clear output about what was found
    - [x] In `override`: Auto-detect, but clearly show what will be changed by:
      - [x] Displaying a summary of detected registries before processing
      - [x] Showing which registries will be remapped and which will be skipped
      - [x] Providing a clear indication when processing is complete
  - [x] Add `--dry-run` flag to show changes without writing files
- [x] **[P1]** Standardize override file naming
  - [x] Use consistent format: `<release-name>-overrides.yaml`
  - [x] Remove any namespace component from the filename
  - [x] Document naming convention in help text and documentation
- [x] **[P1]** Handle unrecognized registries sensibly
  - [x] Default: Skip unrecognized registries with clear warnings
  - [x] Add `--strict` flag that fails when unrecognized registries are found
  - [x] Without `--strict`: Log warnings about unrecognized registries but continue processing
  - [x] With `--strict`: Fail with non-zero exit code when unrecognized registries are found
  - [x] In both modes: Clearly log which registries were detected and which were skipped
  - [x] Provide specific suggestions for missing mappings
- [x] **[P1]** Integrate validation into override command
  - [x] Run validation by default after generating overrides
  - [x] Add `--no-validate` flag to skip validation
  - [x] Implement silent validation with detailed output only on error
- [x] **[P1]** Improve Kubernetes version handling
  - [x] Document default Kubernetes version in help text
  - [x] Provide clear error messages for version-related validation failures

- [x] **[P0]** Implement consistent error codes for enhanced debugging
    We need to align and not collide with current error code numbers or handling
    The actual error number can be decided we just want distinct error codes to handle more conditions
    We should extend the existing error code system rather than replace it
    Analysis of current codebase is needed to identify existing error codes before assigning new ones
  - [x] Exit 0: Success
  - [x] Exit 1: General error
  - [x] Exit 2: Configuration error (missing/invalid registry mappings)
  - [x] Exit 3: Validation error (helm template validation failed)
  - [x] Exit 4: File operation error (file exists, permission denied)
  - [x] Exit 5: Registry detection error (no registries found or mapped)
  - [x] Ensure all error messages include the error code for reference
  
## Testing Strategy for CLI Enhancements

### Configuration Command Tests
- [x] Test updating existing mapping with new target
- [x] Test adding new mapping when source doesn't exist
- [x] Test reading from existing mapping file
- [x] Test config command creates file with correct permissions

### Registry Auto-detection Tests
- [x] Test detection of common registries (docker.io, quay.io, gcr.io, etc.)
- [x] Test detection with incomplete/ambiguous registry specifications
- [x] Test behavior when no registries are detected
- [x] Test behavior with mixed recognized/unrecognized registries

### File Naming and Output Tests
- [x] Test default file naming follows `<release-name>-overrides.yaml` format
- [x] Test behavior when output file already exists
- [x] Test custom output path with `--output-file` flag
- [x] Test permission handling for output files

### Strict Mode Tests
- [x] Test normal mode skips unrecognized registries with warnings
- [x] Test strict mode fails on unrecognized registries
- [x] Test logging output in both modes
- [x] Test exit codes match specification

### Error Handling Tests
- [x] Test each distinct error condition produces correct error code
- [x] Test error messages are informative and actionable
- [x] Test behavior with invalid input combinations
- [x] Test command continues/fails as expected for each error type

### Phase 4: Documentation Updates (P0: User Experience Enhancements)
- [x] **[P0]** Update documentation to reflect CLI changes
  - [x] Document all flag defaults and required/optional status in help output
  - [x] Update CLI reference guide with new behavior
  - [x] Create a command summary with flag behavior in clear table format
  - [x] Document when files will be written vs. stdout output

### Implementation Notes
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

## Phase 5: Registry Format Standardization (P1: Technical Debt Reduction)

### Overview
Fully standardize on the structured registry format throughout the codebase, deprecating the legacy key-value format while maintaining backward compatibility for existing users.

### Motivation
- Structured format provides better metadata with description and enabled flags
- Improved organization with dedicated sections for mappings, default targets, and strict mode
- Future extensibility through the version field
- Simplified code maintenance with a single canonical format
- Clearer documentation and user guidance

### Implementation Steps

#### Phase 5.1: Registry Package Updates
- [/] **[P1]** Update registry package core functionality
  - [ ] Remove or deprecate legacy-specific functions in `pkg/registry/mappings.go`
  - [x] Update documentation to clarify the structured format is preferred
  - [x] Ensure `ConvertToLegacyFormat()` is maintained for backward compatibility
  - [ ] Add clear deprecation notices to legacy-format functions
  - [ ] Update function signatures that take map[string]string to prefer *Config parameter
  - [ ] Consider adding helpers to detect format from raw file content

#### Phase 5.2: CLI Command Updates
- [ ] **[P1]** Update CLI commands for structured format
  - [x] Review and update the `inspect` command skeleton generation
  - [ ] Update the `config` command to only write structured format
  - [ ] Update the `override` command to expect structured format
  - [/] Update help text and examples to show structured format
  - [/] Ensure `--registry-file` flag documentation mentions structured format

#### Phase 5.3: Test Updates
- [ ] **[P1]** Update test suite for structured format
  - [x] Modify `TestRegistryMappingFile` in `test/integration/integration_test.go`
  - [ ] Update `TestConfigFileMappings` and similar tests
  - [x] Update `CreateRegistryMappingsFile()` in `test/integration/harness.go` to default to structured format
  - [x] Add tests for handling of legacy format files (conversion path)
  - [ ] Verify all existing tests pass with the updated format

#### Phase 5.4: Documentation Updates
- [ ] **[P1]** Update user-facing documentation
  - [ ] Update CLI reference documentation with structured format examples
  - [ ] Add migration guide for users with existing config files
  - [ ] Update any tutorials or examples to use structured format
  - [ ] Document the backward compatibility mechanism

### Files Requiring Changes

1. **Registry Package Files**:
   - `pkg/registry/mappings.go`: Update legacy format handling
   - `pkg/registry/config.go`: Make structured format the primary interface

2. **Command Files**:
   - `cmd/irr/inspect.go`: Review createConfigSkeleton() implementation
   - `cmd/irr/config.go`: Update to prefer structured format
   - `cmd/irr/override.go`: Update to handle structured format
   - `cmd/irr/validate.go`: Update to expect structured format

3. **Test Files**:
   - `test/integration/harness.go`: Update CreateRegistryMappingsFile()
   - `test/integration/integration_test.go`: Update tests using registry files
   - `pkg/registry/config_test.go`: Ensure tests cover structured format
   - `pkg/registry/mappings_test.go`: Update to test legacy conversion

4. **Documentation**:
   - `docs/CLI-REFERENCE.md`: Update with structured format examples
   - `docs/CONFIGURATION.md`: Update registry configuration documentation
   - Add a migration guide if not already present

### Acceptance Criteria
- All commands generate and expect structured format by default
- Legacy format files can still be read and converted
- All tests pass with structured format
- Documentation clearly explains the structured format
- CLI help text and error messages reference structured format
- Deprecation notices are clear but not disruptive to users

### Testing Strategy
- Test reading legacy format files and proper conversion
- Test writing only structured format files
- Test handling of corrupted or invalid files
- Verify backward compatibility works for existing configs
- Check CLI output and help text for clarity

## Phase 6: Test Output Improvement (P2: Developer Experience)

### Overview
Improve test output readability by reducing verbose YAML output in test failures, particularly for complex charts with large override files.

### Motivation
- Test failures for complex charts (like kube-prometheus-stack) produce overwhelming YAML output
- Large YAML dumps make it difficult to identify the actual failure cause
- More focused and readable output speeds up debugging and development
- Consistent logging approach improves overall test maintenance

### Current Approach in TestKubePrometheusStack
The current implementation in `test/integration/kube_prometheus_stack_test.go` provides a good starting point:
- Uses component-group testing to focus on specific chart sections
- Implements multiple search methods (string search, YAML structure search)
- Limits output size (first 500 chars, first 10 lines)
- Provides targeted searching for specific components
- Uses specialized search functions for complex components (e.g., kube-state-metrics)

### Implementation Steps

#### Phase 6.1: Create Test Output Helper Functions
- [ ] **[P2]** Develop standardized helper functions in the test harness
  - [ ] Create `LimitedOutput(output string, maxLength int)` helper
  - [ ] Create `LogLimitedYAML(t *testing.T, yamlContent string)` helper
  - [ ] Implement `SearchOverridesForComponent(overrides map[string]interface{}, component string)` helper
  - [ ] Add `GetTopLevelKeys(overrides map[string]interface{})` for structure debugging
  - [ ] Create specialized component search helpers for common patterns

#### Phase 6.2: Update Existing Tests
- [ ] **[P2]** Apply output limiting pattern to other integration tests
  - [ ] Identify tests with large YAML output (TestComplexChartFeatures, etc.)
  - [ ] Update those tests to use the new helper functions
  - [ ] Ensure tests report meaningful summaries instead of full YAML
  - [ ] Add component-specific validation where appropriate

#### Phase 6.3: Enhance TestHarness
- [ ] **[P2]** Add output management capabilities to TestHarness
  - [ ] Add `h.LogLimitedOutput(output string, reason string)` method
  - [ ] Add `h.ValidateComponent(component string, overrides map[string]interface{})` method
  - [ ] Implement `h.CompareOverrideKeys(expected []string, actual map[string]interface{})` method
  - [ ] Create collection of reusable component validation patterns

#### Phase 6.4: Documentation
- [ ] **[P2]** Update developer documentation
  - [ ] Document best practices for test output management
  - [ ] Add examples of proper test output limiting
  - [ ] Update testing guide with section on debugging failed tests
  - [ ] Include code examples of helper function usage

### Acceptance Criteria
- Failed tests produce concise, focused output that highlights the actual failure
- Complex chart tests validate components without dumping full YAML content
- Common validation patterns are extracted into reusable helper functions
- Full content is still available through debug logging when needed
- All existing tests maintain the same validation quality with improved output

### Testing Strategy
- Apply helpers to one test at a time and verify test results remain consistent
- Compare test output before and after changes to verify improvement
- Test with intentional failures to ensure appropriate information is still shown
- Validate that output is meaningful enough to diagnose problems without excessive verbosity

## Phase 6: Image Pattern Detection Improvements

### Overview
Improve the analyzer's ability to detect and process image references in complex Helm charts, particularly focusing on init containers, admission webhooks, and other specialized configurations.

### Motivation
- Some complex charts with admission webhooks or multi-container deployments have image references that aren't being detected
- Debug tooling helps identify paths that are missed during analysis
- Consistent detection across all image variations improves reliability of the override process

### Implementation Steps

#### Phase 6.1: Debugging and Analysis
- [x] **[P0]** Add debugging output to trace image detection
  - [x] Add debug logging to analyzeMapValue, analyzeStringValue, and analyzeArray functions
  - [x] Log detailed path information for all analyzed values
  - [x] Log detection results to identify missed patterns
  - [x] Run failing test cases with debug output to identify issues

#### Phase 6.2: Image Pattern Detection Improvements
- [x] **[P0]** Fix ingress-nginx admission webhook image detection
  - [x] Add debug output to identify missed patterns
  - [x] Run focused test for ingress-nginx chart with admission webhook
  - [x] Verify all expected images are detected properly
  
#### Phase 6.3: Additional Chart Coverage
- [x] **[P1]** Expand test coverage to more complex charts
  - [ ] Review and enable previously skipped test cases
  - [x] Add tests for charts with init containers
  - [x] Add tests for charts with sidecars and admission webhooks
  - [x] Add tests for edge cases including unusual nesting levels and camel-cased fields
  - [x] Add proper handling for template-string image references
  - [ ] Fix simplified-prometheus-stack test case

