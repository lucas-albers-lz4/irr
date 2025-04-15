# TODO.md - Helm Image Override Implementation Plan

## Completed Phases
## Phase 1: Helm Plugin Integration - Remaining Items
_**Goal:** Implement the Helm plugin interface that wraps around the core CLI functionality._

## Phase 2: Test Coverage
_**Goal:** Systematically increase unit and integration test coverage across the codebase to improve reliability and reduce regressions, aiming for a minimum coverage baseline and targeting critical packages._

### Phase 2.1: Establish Baseline & Target Low-Hanging Fruit (Goal: Variable Minimum Coverage)
- [x] **Step 1: Quick Wins - Simple Packages First**
  - [x] **`pkg/exitcodes` (Target: 30%)**: Test error types and helper functions.
    - [x] Test string formatting for each error type
    - [x] Verify error wrapping/unwrapping behavior works correctly
    - [x] Test detection of exit code errors via `IsExitCodeError()`

  - [x] **`pkg/version` (Target: 30%)**: Test version checking logic.
    - [x] Test with valid and invalid version strings
    - [x] Verify correct comparison behavior for major/minor/patch versions
    - [x] Test error handling for malformed versions

- [x] **Step 2: Filesystem Mocking Preparation**
  - [x] Start by creating a consistent filesystem mocking pattern:
    - [x] Identify all packages with filesystem interactions
    - [x] **Implement hybrid approach for filesystem abstraction:**
      - [x] Define a standard filesystem interface in a central location:
        ```go
        // In pkg/fileutil or a new pkg/fsutil package
        type FS interface {
            Open(name string) (File, error)
            Stat(name string) (os.FileInfo, error)
            Create(name string) (File, error)
            ReadFile(filename string) ([]byte, error)
            WriteFile(filename string, data []byte, perm os.FileMode) error
            MkdirAll(path string, perm os.FileMode) error
            Remove(name string) error
            RemoveAll(path string) error
            // Add other methods as needed
        }
        
        // Ensure afero.Fs implements this interface
        var _ FS = afero.NewOsFs()
        ```
      
      - [x] For existing code (non-intrusive approach):
        ```go
        // In each package that uses filesystem
        var fs fileutil.FS = afero.NewOsFs()
        
        // Helper for tests to swap the filesystem
        func SetFs(newFs fileutil.FS) func() {
            oldFs := fs
            fs = newFs
            return func() { fs = oldFs } // Return a cleanup function
        }
        
        // Use throughout package
        func ReadConfigFile(path string) ([]byte, error) {
            return fs.ReadFile(path)
        }
        ```
      
      - [x] For new code and major refactors (dependency injection):
        ```go
        // Struct with explicit dependency
        type FileOperations struct {
            fs fileutil.FS
        }
        
        // Constructor with default
        func NewFileOperations(fs fileutil.FS) *FileOperations {
            if fs == nil {
                fs = afero.NewOsFs()
            }
            return &FileOperations{fs: fs}
        }
        
        // Methods use the dependency
        func (f *FileOperations) ReadConfig(path string) ([]byte, error) {
            return f.fs.ReadFile(path)
        }
        ```
    
    - [x] Document implementation guidelines:
      - [x] Favor dependency injection for new code and significant refactors
      - [x] Use package variables for smaller, focused updates to existing code
      - [x] Always provide test helpers for swapping filesystem implementations
    
    - [x] Create a detailed mocking guide in `docs/TESTING-FILESYSTEM-MOCKING.md` (COMPLETED)
      - [x] Explain both approaches with examples
      - [x] Provide standard test patterns
      - [x] Document when to use each approach
      - [x] Reference this document in other testing documentation

  - [x] **`pkg/fileutil` (Target: 30%)**: Implement as first application of filesystem mocking.
    - [x] Add `afero.Fs` variable or parameter to functions
    - [x] Test file existence checking
    - [x] Test directory operations
    - [x] Use as a model for other packages

- [x] **Step 3: Utility Packages with Adjusted Expectations**
  - [x] **`pkg/log` (Target: 20%)**:
    - [x] Test `ParseLevel` with all supported log levels
    - [x] Test level setting and retrieval (`SetLevel`/`CurrentLevel`) 
    - [x] Test level-based filtering (basic cases only)
    - [x] *Note: Output capturing can be flaky - focus on core functionality*

  - [x] **`pkg/debug` (Target: 20%)**:
    - [x] Test initialization with mocked environment variables
    - [x] Test debug state toggling (basic cases only)
    - [x] Test simple output functions
    - [x] *Note: Not critical path code, basic coverage is sufficient*

  - [x] **`pkg/testutil` (Target: 25%)**:
    - [x] Apply filesystem mocking pattern from Step 2
    - [x] Test `GetChartPath` with various path inputs
    - [x] *Note: Test utilities themselves need only moderate coverage*

- [x] **Step 4: Complex Analysis Package**
  - [x] **`pkg/analyzer` (Target: 25-30%)**:
    - [x] Create test fixtures for representative chart values
    - [x] Test simple pattern matching cases first:
      ```go
      // Start with simple patterns and simple structures
      testValues := map[string]interface{}{
          "image": "nginx:latest",
          "nested": map[string]interface{}{
              "image": "redis:alpine"
          }
      }
      patterns := []string{"*.image"}
      ```
    - [x] Add tests for basic value traversal (simple maps first)
    - [x] Test recursive analysis with limited nesting depth
    - [x] *Note: Full coverage of recursive analysis is challenging, focus on key paths*

- [ ] **Step 5: Filesystem Mocking - Incremental Roll-out**
  - [x] Apply consistent pattern to one package at a time:
    - [x] `pkg/helm`: Update SDK to use injectable filesystem
    - [x] `pkg/fileutil`: Fix error handling in filesystem mocking tests
    - [x] `pkg/chart`: Update Loader and Generator to use injectable filesystem
    - [x] `pkg/registry`: Update registry mapping file operations
    - [ ] `cmd/irr`: Allow filesystem injection for file operations
  - [ ] Test Strategy:
    - [ ] When testing a package, first update it to use the filesystem abstraction
    - [ ] Then write tests using the mocking capability
    - [ ] Follow the standard pattern developed in Step 2:
      ```go
      // Example pattern for filesystem mocking in tests
      func TestWithMockFs(t *testing.T) {
          // Create mock filesystem
          mockFs := afero.NewMemMapFs()
          
          // Setup test files/directories
          afero.WriteFile(mockFs, "test.yaml", []byte("test: data"), 0644)
          
          // Replace package filesystem with mock
          originalFs := somepackage.Fs
          somepackage.Fs = mockFs
          defer func() { somepackage.Fs = originalFs }()
          
          // Run test with mock filesystem
          // ...
      }
      ```
  - [ ] Document architectural tradeoffs and approach:
    - [ ] Add guidance about when to use global variables vs. dependency injection
    - [ ] Document patterns for test setup/teardown with mock filesystem
    - [ ] Update mocking section in testing documentation

### Phase 2.2: Target Core Functionality Gaps (Goal: ~50-60% in Core Packages)
- [ ] **Analyze Coverage Reports:** Use `go tool cover -html=coverage.out` to visualize uncovered lines in key packages.
- [ ] **Increase Coverage in Core Packages:**
  - [ ] `pkg/chart` (Current: 52.3%): Focus on `Load`, `validateHelmTemplateInternal`, `OverridesToYAML`, error handling, rules integration.
  - [ ] `pkg/override` (Current: 51.1%): Test YAML generation (`GenerateYAMLOverrides`, `GenerateYAML`, `ToYAML`), path construction/manipulation (`ConstructPath`, `GetValueAtPath`), merging (`mergeMaps`), and error wrapping.
  - [ ] `pkg/rules` (Current: 60.2%): Test core rule application (`ApplyRules`, `ApplyRulesToMap`), registration (`AddRule`), enabling/disabling (`SetEnabled`), and provider detection (`DetectChartProvider`).
  - [ ] `pkg/analysis` (Current: 72.4%): Add tests for uncovered functions (`Load`, `IsGlobalRegistry`, `ParseImageString`, `mergeAnalysis`).
  - [ ] `pkg/image` (Current: 71.3%): Add tests for validation (`IsValid*`), error types, and edge cases in parsing/normalization.

### Phase 2.3: Enhance High-Coverage & Local Integration (Goal: ~70%+ in Core)
- [ ] **Refine Existing Tests:** Improve tests in well-covered packages (`pkg/generator`, `pkg/helm`, `pkg/registry`, `pkg/strategy`) by adding edge cases or complex scenarios.
- [ ] **Implement Local Integration Tests:**
    - [ ] Write tests covering interactions *between* packages (e.g., `analysis` -> `override` -> `generator`).
    - [ ] Use chart fixtures, mock dependencies (like registry interactions) where necessary.
    - [ ] Validate end-to-end workflows locally (load chart -> analyze -> generate overrides -> apply rules -> validate output).

### General Principles & Tooling for Coverage Improvement
- [ ] **Prioritize Behavior:** Focus tests on intended functionality and critical paths, especially public APIs.
- [ ] **Test Errors:** Explicitly test error conditions and handling.
- [ ] **Use Coverage Tools:** Regularly run `go test -coverprofile=coverage.out ./...` and `go tool cover -html=coverage.out`.
- [ ] **CI Integration:** Add a coverage check to CI to prevent regressions (e.g., using a tool or script to enforce a minimum threshold).
- [ ] **Refactor for Testability:** If needed, refactor code (using interfaces, dependency injection) to make it easier to test.
- [ ] **Document Test Patterns:** 
  - [ ] As coverage increases, document effective test patterns used
  - [ ] Capture lessons learned about which test approaches work best for different package types
  - [ ] Share successful mocking strategies with the team
- [ ] **Periodic Review:** 
  - [ ] Revisit coverage targets after initial improvements to potentially raise them
  - [ ] Perform focused reviews of CLI (cmd/irr) error handling after core packages
  - [ ] Identify packages that might need architectural changes for better testability
- [ ] **Documentation Updates:**
  - [ ] Update package-level README files with testing guidance as patterns emerge
  - [ ] Add code comments about test requirements for complex functions
  - [ ] Document any non-obvious test setup requirements

## Phase 2: Chart Parameter Handling & Rules System
_**Goal:** Analyze results from the brute-force solver and chart analysis to identify parameters essential for successful Helm chart deployment after applying IRR overrides. Implement an intelligent rules system that distinguishes between Deployment-Critical (Type 1) and Test/Validation-Only (Type 2) parameters._

Implementation steps:

1. **Parameter Classification & Analysis**
   - [x] **[P1]** Create formal distinction between Type 1 (Deployment-Critical) and Type 2 (Test/Validation-Only) parameters
   - [x] **[P1]** Analyze solver results to identify common error patterns and required parameters
   - [x] **[P1]** Implement first high-priority Type 1 rule: Bitnami chart security bypass
     - [x] Define tiered confidence detection system:
       - [x] High confidence: Require multiple indicators (homepage + GitHub/image references)
       - [x] Medium confidence: Accept single strong indicators like homepage URL
       - [x] Fallback detection: Identify charts that fail with exit code 16 and "unrecognized containers" error
     - [x] Implement metadata-based detection examining:
       - [x] Chart metadata `home` field containing "bitnami.com"
       - [x] Chart metadata `sources` containing "github.com/bitnami/charts"
       - [x] Chart `maintainers` field referencing "Bitnami" or "Broadcom" 
       - [x] Chart `repository` containing "bitnamicharts"
       - [x] `annotations.images` field containing "docker.io/bitnami/" image references
       - [x] `dependencies` section containing tags with "bitnami-common"
     - [x] Implement `global.security.allowInsecureImages=true` insertion for detected Bitnami charts
       - [x] Add this parameter during the override generation phase (`irr override` command)
       - [x] Ensure this parameter is included in the final override.yaml file
       - [x] Update override generation logic to inject this parameter automatically
     - [x] Test with representative Bitnami charts to verify detection accuracy and deployment success
       - [x] Test specifically with bitnami/nginx, bitnami/memcached and other common Bitnami charts
       - [x] Verify the override file contains the security bypass parameter
       - [x] Confirm validation passes when the parameter is included
     - [x] Add fallback retry mechanism for charts failing with specific error signature
       - [x] Detect exact exit code 16 with error message containing "Original containers have been substituted for unrecognized ones"
       - [x] Add specific error handling for the message "If you are sure you want to proceed with non-standard containers..."
       - [x] If validation fails with this specific error, retry with `global.security.allowInsecureImages=true`
   - [x] **[P2]** Document Type 2 parameters needed for testing (e.g., `kubeVersion`)
   - [x] Correlate errors with chart metadata (provider, dependencies, etc.)

2. **Rules Engine Implementation**
   - [x] **[P1]** Design rule format with explicit Type 1/2 classification
     - [x] Define structured rule format in YAML with versioning
     - [x] Support tiered confidence levels in detection criteria
     - [x] Include fallback detection based on error patterns
     - [x] Allow rule actions to modify override values
   - [x] **[P1]** Implement rule application logic in Go that adds only Type 1 parameters to override.yaml
   - [x] **[P1]** Create configuration options to control rule application
   - [ ] **[P1]** Create test script to extract and analyze metadata from test chart corpus
     - [ ] Develop script to process Chart.yaml files from test corpus
     - [Note: Reconfirming - The existing test/tools/test-charts.py script processes the test chart corpus and WILL be adapted or leveraged for this analysis. No new script will be created.]
     - [ ] Generate statistics on different chart providers based on metadata patterns
     - [ ] Produce report identifying reliable detection patterns for major providers
   - [x] **[P2]** Add test-only parameter handling for validation (Type 2)
   - [ ] **[P2]** Implement chart grouping based on shared parameter requirements
   - [ ] **[P1]** Enhance log output for applied rules:
     - [ ] When a rule adds a Type 1 parameter (e.g., Bitnami rule adding `global.security.allowInsecureImages`), log an `INFO` message.
     - [ ] Log message should include: Rule Name, Parameter Path/Value, Brief Reason/Description, and Reference URL (e.g., Bitnami GitHub issue).
     - [ ] Implement by adding logging logic within the rule application function (e.g., `ApplyRulesToMap`).
     - [ ] Consider adding a `ReferenceURL` or `Explanation` field/method to the `Rule` interface for better context sourcing.
     - [ ] Update tests (e.g., integration tests) to assert the presence and content of this log message.
     - [ ] Update documentation (`docs/RULES.md`) to mention this log output.
     - [ ] Ensure `README.md` prominently links to `docs/RULES.md`.

3. **Chart Provider Detection System**
   - [x] **[P1]** Implement metadata-based chart provider detection:
     - [x] Bitnami chart detection (highest priority)
       - [x] Primary: Check for "bitnami.com" in `home` field
       - [x] Secondary: Check for "bitnami" in image references
       - [x] Tertiary: Check for "bitnami-common" in dependency tags
       - [x] Implement tiered confidence levels (high/medium)
       - [ ] Add fallback detection for exit code 16 errors
     - [ ] VMware/Tanzu chart detection (often similar to Bitnami)
     - [ ] Standard/common chart repositories 
     - [ ] Custom/enterprise chart detection
   - [x] **[P1]** Create extensible detection framework for future providers
   - [ ] **[P2]** Add fallback detection based on chart internal structure and patterns

4. **Testing & Validation Framework**
   - [x] **[P1]** Create test cases for Type 1 parameter insertion
   - [ ] **[P1]** Analyze and report statistics on detection accuracy across test chart corpus
   - [ ] **[P1]** Validate Bitnami charts deploy successfully with inserted parameters
   - [ ] **[P1]** Test fallback mechanism with intentionally undetected Bitnami charts
   - [x] **[P2]** Implement test framework for Type 2 parameters in validation context
   - [ ] **[P2]** Measure improvement in chart validation success rate with rules system
   - [x] **[P2]** Create automated tests for rule application logic

   # Prioritized Additional Test Cases for Rules System
   - [ ] **Integration (Core + Disable):** Run `irr override` on real Bitnami charts (e.g., nginx) and non-Bitnami charts to verify:
     - [ ] Override file contains `global.security.allowInsecureImages: true` only for detected Bitnami charts (medium/high confidence)
     - [ ] The CLI flag `--disable-rules` prevents rule application
   - [ ] **Unit Coverage (Detection & Application):**
     - [ ] Verify core detection logic handles different metadata combinations correctly (covering confidence levels)
     - [ ] Test rule application logic merges parameters correctly into simple and complex existing override maps
     - [ ] Include checks for case sensitivity and whitespace variations in metadata
   - [ ] **Type 2 Exclusion:** Validate that parameters from rules marked as `TypeValidationOnly` are never included in the final override map (can use a dummy rule for testing)
   - [ ] **Negative/Edge (No Metadata):** Test charts with empty/nil metadata; ensure no panics occur and no rules are applied
   - [ ] **Error Handling:** Unit test graceful failure (log warning, continue) if the rules registry is misconfigured or type assertion fails

5. **Documentation & Maintainability**
   - [x] **[P1]** Document the distinction between Type 1 and Type 2 parameters
   - [x] **[P1]** Create user guide for the rules system
     - [x] Document the tiered detection approach
     - [ ] Explain fallback detection mechanism
     - [x] Provide examples of rule definitions
   - [x] **[P1]** Document metadata patterns used for chart provider detection
     - [x] Document Bitnami detection patterns with examples
     - [x] Create reference table of metadata fields for different providers
   - [ ] **[P2]** Implement rule versioning and tracking
   - [ ] **[P2]** Create process for rule updates based on new chart testing
   - [x] **[P2]** Add manual override capabilities for advanced users

## Phase 3: Kubernetes Version Handling in `irr validate`
_**Goal:** Implement robust Kubernetes version specification for chart validation to ensure consistent version handling and resolve compatibility issues._


## Phase 4: `kind` Cluster Integration Testing
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

 
  ## REMINDER 0 (TEST BEFORE AND AFTER) Implementation Process: DONT REMOVE THIS SECTION as these hints are important to remember.
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