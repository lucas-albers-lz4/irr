# TODO.md - Helm Image Override Implementation Plan

# Usability Improvement , CLI Refactor
## Implementation Sequence Notes
- Phase 0 (Configuration) should be completed before Phase 3.5 registry handling
- Phase 2 (Flag Consistency) should be completed before Phase 3.5 file naming standardization
- Phase 3 (Output Behavior) should be completed before implementing strict mode in Phase 3.5 

## Phase 0: Configuration Setup (P0: Critical Usability)
- [ ] **[P0]** Implement `helm irr config` command (flag-driven only)
  - [ ] Allow user to set or update a mapping via flags (e.g., `--source quay.io --target harbor.local/quay`)
  - [ ] If the source already exists, update its target; otherwise, add a new mapping
  - [ ] No validation of endpoints; user is responsible for correctness
  - [ ] When running `inspect`, suggest likely mappings based on detected registries in the environment
  - [ ] Document that override/validate can run without config, but correctness depends on user configuration
- [ ] **[P0]** Analyze existing error code usage
  - [ ] Document current error codes and their conditions
  - [ ] Identify gaps in error handling
  - [ ] Create mapping between planned new error codes and existing ones

### Phase 1: Flag Cleanup (P0: User Experience Enhancements)
- [ ] **[P0]** Remove unused or confusing flags
  - [ ] Remove `--output-format` flag (Not used, always YAML)
  - [ ] Remove `--debug-template` flag (Not implemented/used) 
  - [ ] Remove `--threshold` flag (No clear use case; binary success preferred)
  - [ ] Hide or remove `--strategy` flag (Only one strategy implemented; hide/remove for now)
  - [ ] Hide or remove `--known-image-paths` flag (Not needed for most users; hide or remove)
- [ ] **[P0]** Flag cleanup verification
  - [ ] Review and update/remove any test cases using these flags
  - [ ] Test and lint after each change
  - [ ] Update CLI and other documentation to remove references to these flags

### Phase 2: Flag Consistency and Defaults (P0: User Experience Enhancements)
- [x] **[P0]** Unify `--chart-path` and `--release-name` behavior
  - [x] Allow both flags to be used together
  - [x] Implement auto-detection when only one is provided
  - [x] Document precedence rules
  - [x] Default to `--release-name` in plugin mode; default to `--chart-path` in standalone mode
- [ ] **[P0]** Implement mode-specific flag presentation
  - [ ] Tailor help output and flag requirements based on execution mode
  - [ ] Make `--release-name` primary in plugin mode
  - [ ] Make `--chart-path` primary in standalone mode
  - [ ] Hide flags not relevant for automation in integration test mode
- [x] **[P0]** Standardize `--namespace` behavior
  - [x] Make `--namespace` always optional
  - [x] Default to "default" namespace when not specified
- [ ] **[P0]** Make `--source-registries` optional in override when config or auto-detection is present

### Phase 3: Output File Behavior Standardization (P0: User Experience Enhancements)
- [ ] **[P0]** Standardize `--output-file` behavior across commands
  - [ ] `inspect`: Default to stdout if not specified
  - [ ] `override`: 
    - [ ] Default to stdout in standalone mode
    - [ ] Default to `<release-name>-overrides.yaml` in plugin mode with release name
    - [ ] Ensure explicit override with `--output-file` always works
    - [ ] Implement check to never overwrite existing files without explicit user action (e.g., `--force`; We don't have a `--force` command or plan to implement it)
  - [ ] `validate`: Only write file output when `--output-file` is specified
- [ ] **[P0]** Implement consistent error handling for file operations
  - [ ] Add clear error messages when file operations fail
  - [ ] Implement uniform permission handling across all commands

### Phase 3.5: Streamlined Workflow Implementation (P1: User Experience Enhancements)
- [ ] **[P1]** Implement enhanced source registry handling
  - [ ] Add explicit `--all-registries` flag for clarity
  - [ ] Implement auto-detection of registries with clear user feedback
  - [ ] Use two-stage approach for registry auto-detection:
    - [ ] In `inspect`: Auto-detect and show clear output about what was found
    - [ ] In `override`: Auto-detect, but clearly show what will be changed by:
      - [ ] Displaying a summary of detected registries before processing
      - [ ] Showing which registries will be remapped and which will be skipped
      - [ ] Providing a clear indication when processing is complete
  - [ ] Add `--dry-run` flag to show changes without writing files
- [ ] **[P1]** Standardize override file naming
  - [ ] Use consistent format: `<release-name>-overrides.yaml`
  - [ ] Remove any namespace component from the filename
  - [ ] Document naming convention in help text and documentation
- [ ] **[P1]** Handle unrecognized registries sensibly
  - [ ] Default: Skip unrecognized registries with clear warnings
  - [ ] Add `--strict` flag that fails when unrecognized registries are found
  - [ ] Without `--strict`: Log warnings about unrecognized registries but continue processing
  - [ ] With `--strict`: Fail with non-zero exit code when unrecognized registries are found
  - [ ] In both modes: Clearly log which registries were detected and which were skipped
  - [ ] Provide specific suggestions for missing mappings
- [ ] **[P1]** Integrate validation into override command
  - [ ] Run validation by default after generating overrides
  - [ ] Add `--no-validate` flag to skip validation
  - [ ] Implement silent validation with detailed output only on error
- [ ] **[P1]** Improve Kubernetes version handling
  - [ ] Document default Kubernetes version in help text
  - [ ] Provide clear error messages for version-related validation failures

- [ ] **[P0]** Implement consistent error codes for enhanced debugging
    We need to align and not collide with current error code numbers or handling
    The actual error number can be decided we just want distinct error codes to handle more conditions
    We should extend the existing error code system rather than replace it
    Analysis of current codebase is needed to identify existing error codes before assigning new ones
  - [ ] Exit 0: Success
  - [ ] Exit 1: General error
  - [ ] Exit 2: Configuration error (missing/invalid registry mappings)
  - [ ] Exit 3: Validation error (helm template validation failed)
  - [ ] Exit 4: File operation error (file exists, permission denied)
  - [ ] Exit 5: Registry detection error (no registries found or mapped)
  - [ ] Ensure all error messages include the error code for reference
  
## Testing Strategy for CLI Enhancements

### Configuration Command Tests
- [ ] Test updating existing mapping with new target
- [ ] Test adding new mapping when source doesn't exist
- [ ] Test reading from existing mapping file
- [ ] Test config command creates file with correct permissions

### Registry Auto-detection Tests
- [ ] Test detection of common registries (docker.io, quay.io, gcr.io, etc.)
- [ ] Test detection with incomplete/ambiguous registry specifications
- [ ] Test behavior when no registries are detected
- [ ] Test behavior with mixed recognized/unrecognized registries

### File Naming and Output Tests
- [ ] Test default file naming follows `<release-name>-overrides.yaml` format
- [ ] Test behavior when output file already exists
- [ ] Test custom output path with `--output-file` flag
- [ ] Test permission handling for output files

### Strict Mode Tests
- [ ] Test normal mode skips unrecognized registries with warnings
- [ ] Test strict mode fails on unrecognized registries
- [ ] Test logging output in both modes
- [ ] Test exit codes match specification

### Error Handling Tests
- [ ] Test each distinct error condition produces correct error code
- [ ] Test error messages are informative and actionable
- [ ] Test behavior with invalid input combinations
- [ ] Test command continues/fails as expected for each error type

### Phase 4: Documentation Updates (P0: User Experience Enhancements)
- [ ] **[P0]** Update documentation to reflect CLI changes
  - [ ] Document all flag defaults and required/optional status in help output
  - [ ] Update CLI reference guide with new behavior
  - [ ] Create a command summary with flag behavior in clear table format
  - [ ] Document when files will be written vs. stdout output

 
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

