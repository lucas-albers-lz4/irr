# IRR Helm Plugin: Specific Behaviors and Features

## Table of Contents
- [1. Introduction](#1-introduction)
- [2. Core Principles & Compatibility](#2-core-principles--compatibility)
- [3. Helm Context Awareness](#3-helm-context-awareness-enhanced)
- [4. Plugin-Exclusive Commands & Features](#4-plugin-exclusive-commands--features)
- [5. Workflow Integration & Defaults](#5-workflow-integration--defaults)
- [6. User Experience Considerations](#6-user-experience-ux)
- [7. Feature Alignment Analysis](#7-feature-alignment-analysis)
- [8. Summary of Key Differences](#8-summary-of-key-differences-plugin-vs-standalone-cli)
- [9. Prerequisites & Installation](#9-prerequisites--installation)
- [10. Appendix: Troubleshooting & Examples](#10-appendix-troubleshooting--examples)

## 1. Introduction

This document outlines potential behaviors and features specific to running `irr` as a Helm plugin (`helm irr ...`) compared to its standalone CLI execution (`irr ...`). The goal is to leverage the Helm environment and runtime context when available, making the plugin feel like a natural extension of Helm, without fundamentally altering the core `inspect`/`override`/`validate` logic.

### Quick Start Example

```bash
# Inspect a deployed release and generate overrides
helm irr inspect my-release -n dev                    # Analyze image references
helm irr override my-release -n dev -t registry.local # Generate override file
helm irr validate my-release -n dev -f my-release-overrides.yaml # Pre-flight check
```

## 2. Core Principles & Compatibility

**Seamless Integration:** The plugin should feel like a native Helm command. It should utilize Helm's context (like namespace, release information, potentially authentication) wherever practical and beneficial.

**Version Compatibility:**
- Compatible with Helm v3.8+
- Requires the same Kubernetes version support as the parent Helm installation
- Maintains feature parity with standalone CLI version

**Feature Alignment:** Any plugin-specific features must honor the existing design philosophy of the core `irr` tool. The plugin should enhance the experience of using `irr` with Helm, not fundamentally change its operation.

**Configuration Precedence:**
```
Command-line flags > Helm plugin config > Global Helm config > Defaults
```
This clear precedence ensures consistent behavior when multiple configuration sources exist.

## 3. Helm Context Awareness (Enhanced)

Leveraging Helm's runtime information is the primary advantage of the plugin interface.

### 3.1. Namespace Handling
*   **Requirement:** The plugin **must** respect the Kubernetes namespace context.
*   **Sources (in order of precedence):**
    1.  `--namespace` / `-n` flag provided directly to the `helm irr` command (e.g., `helm irr inspect my-release -n staging`).
    2.  Helm's currently configured namespace (inheriting from global flags like `helm -n <ns> ...` or the `kubectl` context).
    3.  Default namespace (`default`) if none is specified.
*   **Contrast with CLI:** The standalone CLI operating on local chart paths has no inherent concept of Kubernetes namespaces.

### 3.2. Release Value Fetching
*   **Requirement:** When operating on a deployed release name (e.g., `helm irr override my-release`), the plugin **must** use the deployed release's values as the primary input source.
*   **Mechanism:** Use Helm's mechanisms (likely the Go SDK) to execute the equivalent of `helm get values <release-name> -n <namespace>`.
*   **Contrast with CLI:** The standalone CLI requires `--chart-path` for input.

### 3.3 Helm Configuration & Authentication Scope

For the initial MVP:
- Only basic Helm context (namespace, release values) will be integrated
- Authentication handling will be strictly read-only, using Helm's SDK for value fetching
- No credential storage or registry authentication will be implemented

**Fallback Mechanism:** If Helm SDK operations fail, the plugin will:
1. Log the specific error (with credentials redacted)
2. Retry critical operations up to 3 times with exponential backoff
3. Fall back to CLI execution where possible (`helm get values` command)
4. Return a clear error if all options fail, suggesting manual alternatives

**Input Precedence:**
- When both `--chart-path` and release name are provided, chart path takes precedence
- An explicit error will be shown when neither is provided

### 3.4. Implementation Considerations

Namespace handling must be explicit with clear precedence rules. Explicit namespace flags take precedence, followed by Helm configuration, then kubectl context, with clear logging of the source:
```
INFO: Using namespace 'staging' (from Helm configuration)
```

For releases that exist in multiple namespaces, the plugin fails with a clear error message that lists the found namespaces and suggests using the `-n` flag to specify which one to target:
```
Error: Release 'my-app' exists in 3 namespaces: [dev, staging, prod]
Specify the exact namespace with the -n flag, e.g.:
  helm irr inspect my-app -n staging
```

Transient failures when fetching release values are handled with retry logic using exponential backoff (e.g., 3 attempts, 1s/2s/4s delays) for reliability, with structured logging for clarity:
```json
{"level":"warn","msg":"Release values fetch attempt failed","attempt":2,"total_attempts":3,"release":"my-app","namespace":"dev","error":"connection refused"}
```

### 3.5. Safety & User Guidance Examples

**Multi-Namespace Error Handling (Pseudocode):**
```go
// Pseudocode for multi-ns error
if releaseExistsInMultipleNamespaces(name) {
    namespaces := getNamespacesContainingRelease(name)
    return fmt.Errorf(
        "Release '%s' exists in %d namespaces: %v\n"+
        "Specify the exact namespace with the -n flag\n"+
        "Example: helm irr inspect %s -n <namespace>",
        name, len(namespaces), namespaces, name)
}
```

### 3.6 Required Permissions
The plugin requires these Kubernetes permissions:
```yaml
# ClusterRole for IRR plugin
- apiGroups: [""]
  resources: ["secrets"]  # For release metadata
  verbs: ["get", "list"]
- apiGroups: ["helm.sh"]
  resources: ["releases"]
  verbs: ["get", "list"]
```

**Conclusions (Section 3):**
1.  Namespace handling must be explicit, communicative, and prioritize user-provided flags.
2.  Release value fetching requires robust error handling (retries, clear logging).
3.  Authentication integration should leverage, not replace, Helm's existing patterns.
4.  Error messages for common issues (like multi-namespace releases) should guide the user directly to the solution.

## 4. Plugin-Exclusive Commands & Features

Features that only make sense within the Helm plugin context.

### 4.1. File Overwrite Protection

**File Overwrite Protection:**
Implement a simple, safe approach for output files:
1.  **Default Behavior:** Fail with clear error if the output file already exists.
    ```bash
    $ helm irr override my-app -o existing.yaml
    Error: output file 'existing.yaml' already exists.
    ```

This approach follows UNIX philosophy by simply stating the problem without prescribing solutions. The error is concise and clear, letting users decide how to handle the situation.

### 4.2. Non-Destructive Philosophy

*   **Core Principle:** IRR is strictly a read-only, analysis and validation tool that never directly applies changes to the cluster.
*   **Workflow Security:** This strict separation ensures users always maintain explicit control over what changes are applied to their clusters.
*   **Workflow Example:**
    ```
    [helm irr inspect] → [helm irr override] → [helm irr validate] → [USER REVIEWS] → [USER APPLIES via helm upgrade]
    ```

*   **Design Implications:**
    1. All commands only read cluster data (never write):
       - `inspect`: Reads release values to analyze image references
       - `override`: Generates YAML file locally, never applies it
       - `validate`: Tests if overrides would work, never applies them

    2. **Command Outputs:**
       - Commands produce files, stdout, or validation results
       - Commands never produce Helm release modifications
       - The tool does not implement equivalents of `helm install` or `helm upgrade`

    3. **Workflow Enhancement without Violation:**
       - The tool can facilitate user review while maintaining the separation
       - Example: Suggest the exact `helm upgrade` command for the user to run

*   **Command Chaining Helper:**
    - Add a `--validate` flag to the `override` command to combine generation and validation
    - Maintains YAML as the primary output format for consistency and scriptability
    - Adds helpful follow-up command suggestion without automating it
    ```bash
    # Generate override YAML and validate in one step
    $ helm irr override my-release -n dev -t registry.local --validate
    
    # Output (still primarily YAML-focused)
    ✓ Generated overrides for 12 images to my-release-overrides.yaml
    ✓ Validation successful! Chart renders correctly with overrides.
    
    To apply these changes, run:
      helm upgrade my-release -n dev -f my-release-overrides.yaml
    ```
    - Benefits:
      - Reduces command typing for common workflows
      - Maintains clear separation between generation and application
      - Preserves YAML-centric design philosophy
      - Keeps output scriptable and consistent

*   **User Interface Considerations:**
    ```bash
    # Helpful upgrade suggestion after successful validation
    $ helm irr validate my-release -n dev -f my-release-overrides.yaml
    
    ✓ Validation successful! Chart renders correctly with overrides.
    
    To apply these changes, run:
      helm upgrade my-release -n dev -f my-release-overrides.yaml
    ```

*   **Benefits of this Approach:**
    1. **Safety:** No accidental modifications to production systems
    2. **Transparency:** Clear separation between analysis and action
    3. **Control:** Users maintain explicit control over all changes
    4. **Review:** Enforces review step before application
    5. **CI/CD Friendly:** Fits into GitOps workflows where changes are committed before application

**Conclusions (Section 4.2):**
1. The plugin strictly adheres to the core IRR philosophy of being an analysis tool, not an application tool
2. This separation of concerns enhances safety, transparency, and user control
3. The workflow can be improved through better visualization and suggestions, while preserving this philosophy
4. This approach aligns with enterprise GitOps workflows and change management practices

**Conclusions (Section 4):**
1.  File output operations need strong safety defaults to prevent accidental data loss.

## 5. Workflow Integration & Defaults

Adjusting behavior to better fit common Helm workflows.

### 5.1. Output File Defaults
*   **Concept:** Modify default output behavior for `override` when operating on a release.
*   **Proposal:** When running `helm irr override <release-name>` without `--output-file`, default to writing to `<release-name>-overrides.yaml` in the current directory instead of `stdout`. Use filename sanitization (e.g., release `my/app` -> `my-app-overrides.yaml`).
*   **Rationale:** Might feel more intuitive for release-focused workflows. Reduces pipe/redirection complexity for simple cases.
*   **Contrast with CLI:** Standalone `irr override --chart-path ...` should still default to `stdout` for consistency with typical CLI tools.
*   **Safety:** Apply the file overwrite protection outlined in Section 4.1. Allow explicit `stdout` via `-o -`.
*   **Secure File Handling:**
    ```go
    // Secure file creation with proper permissions
    func createOutputFileSecurely(filename string, content []byte) error {
        // Use restrictive file permissions (0600) by default
        file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
        if err != nil {
            if os.IsExist(err) {
                // Error message updated to remove reference to non-existent flags
                return fmt.Errorf("output file '%s' already exists", filename)
            }
            return fmt.Errorf("error creating output file: %w", err)
        }
        defer file.Close()
        
        // Create parent directories if they don't exist
        if dir := filepath.Dir(filename); dir != "." {
            if err := os.MkdirAll(dir, 0700); err != nil {
                return fmt.Errorf("error creating directories: %w", err)
            }
        }
        
        // Write content at once to minimize time window with empty file
        if _, err := file.Write(content); err != nil {
            return fmt.Errorf("error writing to file: %w", err)
        }
        
        return nil
    }
    ```

### 5.2. Integrated Validation Context (`helm irr validate <release-name>`)
*   **Concept:** Streamline validation against deployed releases using generated overrides.
*   **Ideal Functionality:** `helm irr validate my-release --values overrides.yaml` should intelligently validate the chart *as deployed* with the *new* overrides applied.
*   **Mechanism:**
    1.  Fetch the release's deployed values (`helm get values`).
    2.  Identify the chart source (name, version, repository) used by the release from Helm's release metadata/history.
    3.  Execute `helm template <release-name> <chart-source> -n <namespace> -f <deployed-values> -f <override-values>`.
*   **Challenge:** Reliably finding/accessing the exact chart *source* (path, repo URL, OCI ref) used for the deployed revision can be difficult.
*   **Goal:** Provide a simple command for pre-flight checks against the deployed state before running `helm upgrade`.
*   **Safety Defaults:**
    ```go
    // Default safety constraints - no configuration needed
    const defaultTemplateTimeout = 60 * time.Second
    const defaultMemoryLimit = 512 * 1024 * 1024 // 512MB
    const defaultMaxFileSize = 100 * 1024 * 1024 // 100MB for values files
    
    // Template validation with resource constraints
    func (h *HelmAdapter) validateTemplate(releaseName, namespace, chartSource string, valueFiles []string) error {
        ctx, cancel := context.WithTimeout(context.Background(), defaultTemplateTimeout)
        defer cancel()
        
        // Create command with resource constraints
        cmd := exec.CommandContext(ctx, "helm", "template", releaseName, chartSource)
        cmd.Env = append(os.Environ(), fmt.Sprintf("GOGC=%d", calculateOptimalGC(defaultMemoryLimit)))
        
        // Apply resource limits and checks
        setProcessResourceLimits(cmd, defaultMemoryLimit)
        
        // Check values file sizes before processing
        for _, file := range valueFiles {
            if err := validateFileSize(file, defaultMaxFileSize); err != nil {
                return fmt.Errorf("values file too large: %w", err)
            }
            cmd.Args = append(cmd.Args, "-f", file)
        }
        
        // Execute with appropriate namespace
        cmd.Args = append(cmd.Args, "-n", namespace)
        
        // Capture output for validation
        output, err := cmd.CombinedOutput()
        if err != nil {
            // Sanitize output before returning
            sanitizedOutput := sanitizeCredentials(string(output))
            return fmt.Errorf("template validation failed: %s\n%s", err, sanitizedOutput)
        }
        
        return nil
    }
    ```

### 5.3. Implementation Considerations

Chart version integrity is maintained by prioritizing the exact chart version recorded in release metadata for validation operations, with appropriate warnings if the exact version cannot be found.

When the exact chart source is unavailable, validation fails with clear error guidance for recovery options, prioritizing accuracy over convenience.

Temporary files are used for handling potentially large deployed values files during validation, with reliable cleanup even if validation fails.

### 5.4. Validation Safeguards Examples

**Chart Source Recovery Guidance:**
When validation fails due to missing chart source, provide actionable steps:
```
Error: Chart source 'oci://myreg/charts/mychart:1.2.3' not found.
Troubleshooting:
1. Log in to the OCI registry: helm registry login myreg
2. Check if the tag exists or was removed.
3. Provide local path if available: --chart-source ./charts/mychart-1.2.3.tgz
4. See documentation: [Link to validation troubleshooting]
```

### 5.5. Kubernetes Version Handling During Validation

A key difference between the plugin and standalone execution lies in how the Kubernetes version is determined for the `helm template` operation performed by `validate`:

*   **Plugin Mode (`helm irr validate <release-name> ...`)**:
    *   **Default Behavior:** When running as a plugin, if the user *does not* explicitly provide the `--kube-version` flag, `irr` **will not** pass the `--kube-version` flag to the underlying `helm template` command. This allows `helm template` to utilize the Kubernetes version associated with the current Helm/`kubectl` context, ensuring validation uses the target environment's version by default.
    *   **User Override:** If the `--kube-version` flag *is* provided by the user, its value is explicitly passed to `helm template`, overriding the context-derived version.

*   **Standalone Mode (`irr validate --chart-path ...`)**:
    *   **Default Behavior:** When running standalone, there is no inherent cluster context. If the user *does not* provide the `--kube-version` flag, `irr` **must** pass a hardcoded default version (e.g., `1.31.0`, matching the current help text) to the underlying `helm template` command via the `--kube-version` flag.
    *   **User Override:** If the `--kube-version` flag *is* provided, its value is explicitly passed to `helm template`.

*   **Testing Considerations:** For deterministic testing of the `validate` command (in both plugin and standalone modes), tests should typically provide an explicit `--kube-version` to simulate specific target environments and ensure reproducible results, regardless of the test runner's local context.

### 5.6 CI/CD Integration Example
```yaml
# GitHub Actions workflow snippet
- name: Generate IRR overrides
  run: |
    helm irr override ${{ env.RELEASE }} -n ${{ env.NAMESPACE }} \
      --target-registry ${{ secrets.TARGET_REGISTRY }} \
      --output-file ./overrides.yaml
  env:
    HELM_KUBECONTEXT: ${{ secrets.KUBE_CONTEXT }}
```
Key considerations:
- Always use `--non-interactive` 
- Set explicit `HELM_KUBECONTEXT` for multi-cluster environments
- Store generated overrides as workflow artifact

**Conclusions (Section 5):**
1.  Plugin default outputs can differ from CLI defaults for better workflow integration, but safety (file overwrite) is paramount.
2.  Integrated validation must prioritize fidelity to the deployed state (using exact chart version).
3.  Handling unavailable chart sources requires clear error messages and user guidance for recovery, not silent failures or inaccurate validations.
4.  Resource management (temp files) is crucial for reliable operation.

## 6. User Experience (UX)

Minor adjustments for a smoother plugin experience.

### 6.1. Output Formatting
*   **Goal:** Align logging (info, warn, error) and general output style with standard Helm CLI output for a consistent user experience.
*   **Implementation:** Use Helm's logging libraries or mimic its formatting conventions (color codes, status prefixes).

### 6.2. Flag Inheritance
*   **Goal:** Automatically respect global Helm flags where relevant (e.g., `--kube-context`, `--kubeconfig`).
*   **Implementation:** Ensure Helm SDK clients used by the plugin are initialized correctly to inherit this context from the environment Helm sets up.

### 6.2.1 Logging Configuration

**User-Facing Logging:**
The plugin intentionally implements a simple, binary logging approach:
- Default: Standard operational logging (errors, warnings, key status messages)
- Detailed debug output for troubleshooting is enabled via the `LOG_LEVEL=DEBUG` environment variable.

This minimal approach prevents flag complexity and maintains consistency across CLI and plugin usage patterns.

**Conflicting Flag Resolution:**
- Debug levels: `LOG_LEVEL=DEBUG` takes precedence over other levels.
- Output targets: `--output-file` overrides any config setting
- Namespace flags: `-n/--namespace` overrides any inherited context

### 6.3. Output Style Implementation

**Helm Output Styling:**
The plugin mimics Helm's output style with consistent log levels, color coding (cyan for info, yellow for warnings, red for errors), and status message formatting. This creates a seamless user experience that feels like a native part of Helm.

**Debug Logging Approach:**
The plugin uses the `LOG_LEVEL=DEBUG` environment variable for troubleshooting needs. When enabled, output includes detailed information about operations, API calls, and internal states in a human-readable format. Structured data formats are only used in debug mode for machine parsing when needed.

### 6.4. Telemetry & Privacy
- No user data collection by default
- Log redaction for sensitive values (regex patterns in config)
- Clear documentation on what is collected and how to disable if enabled

**Conclusions (Section 6):**
1.  Consistent output formatting (levels, colors, status) improves usability and integration feel.
2.  Leveraging Helm's context inheritance simplifies configuration for users.
3.  Structured debug logging aids advanced troubleshooting.
4.  Privacy-first approach to telemetry is essential.

## 7. Feature Alignment Analysis

This section systematically evaluates proposed features against the core tool philosophy to determine which should be implemented and which should be rejected or modified.

### 7.1 Features to Implement (Aligned with Core Tool)

| Feature | Justification | Priority |
|---------|---------------|----------|
| **Namespace-aware commands** | Direct extension of Helm context | High |
| **Release value fetching** | Required for release-based operation | High |
| **File overwrite protection** | Enhances safety without changing workflow | Medium |
| **Validation with deployed state** | Natural extension of CLI validation | Medium |
| **Helm logging style** | Improves consistency without functional change | Low |

### 7.2 Features to Modify (Partial Alignment)

| Feature | Concern | Recommendation |
|---------|---------|----------------|
| **Default to file output** | Differs from CLI stdout default | Make configurable via config file preference |
| **Interactive registry selection** | May confuse CLI users | Offer but make easily disableable via config |
| **Chart source annotations** | Adds complexity to Helm releases | Use only as fallback, not primary mechanism |

### 7.3 Features to Reject (Misaligned with Core Tool)

| Feature | Reasoning | Alternative Approach |
|---------|-----------|---------------------|
| **Automatic retry of failed operations** | Core CLI has single-attempt philosophy | Keep user-initiated retry pattern |
| **Complex registry transformations** | Beyond scope of image relocation | Maintain existing path strategies only |
| **Multi-chart operations** | Core focuses on single-chart precision | Maintain one-chart-at-a-time approach |

**Conclusions (Section 7):**
1. The plugin should emphasize Helm context awareness while preserving core CLI behavior patterns
2. Differences between plugin and CLI should be minimized to reduce user cognitive load
3. Safety features can be added but should maintain existing workflow paths
4. New commands must have clear scoping and should extend, not replace, existing functionality

## 8. Summary of Key Differences (Plugin vs. Standalone CLI)

Clear communication of capabilities helps users choose the right tool.

### 8.1. User-Facing Feature Matrix

| Capability               | `helm irr` Plugin             | `irr` Standalone CLI      | Notes                                       |
| :----------------------- | :---------------------------- | :------------------------ | :------------------------------------------ |
| **Input**                | Release Name / Chart Path     | Chart Path Only           | Plugin prefers release name                 |
| **Namespace Awareness**  | Yes (Required)                | No                        | Plugin interacts w/ cluster                 |
| **Cluster Interaction**  | Yes (Get Values, List, etc.)  | No                        | Plugin uses Helm SDK                        |
| **Output Default**       | `<release>-overrides.yaml`    | `stdout`                  | Plugin tailored for release workflow        |
| **Output Safety**        | Fail if file exists           | Standard pipe/redirect    | Simple error if file exists                 |

## 9. Prerequisites & Installation

### 9.1. System Requirements
- Kubernetes cluster with Helm installed
- Go programming language installed
- Helm plugin development environment

### 9.2. Installation Steps
1. Clone the repository
2. Build the plugin
3. Install the plugin

```bash
# Clone the repository
git clone https://github.com/your-username/irr-helm-plugin.git

# Build the plugin
cd irr-helm-plugin
make build

# Install the plugin
helm plugin install ./irr
```

## 10. Appendix: Troubleshooting & Examples

### 10.1. Common Issues
- **Release not found:** Verify the release exists with `helm list -n <namespace>`.
- **Connection refused:** Retry the operation or check network connectivity.
- **Permission denied:** Ensure you have the necessary permissions.

### 10.2. Example Commands
```bash
# Inspect a deployed release and generate overrides
helm irr inspect my-release -n dev                    # Analyze image references
helm irr override my-release -n dev -t registry.local # Generate override file
helm irr validate my-release -n dev -f my-release-overrides.yaml # Pre-flight check
```
