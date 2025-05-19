# Helm Image Override Design Document

**Version:** 0.1.4
**Date:** 2023-10-27

## 1. Introduction

This document outlines the design for a command-line tool, `irr` (Image Relocation and Rewrite), intended to automate the generation of Helm override `values.yaml` files. The primary goal is to redirect container image pulls specified within a Helm chart from various public registries to a designated local or private registry, initially focusing on Harbor configured as a pull-through cache. This addresses the manual effort and potential errors involved in modifying Helm deployments for environments requiring centralized, proxied, or private image access. Additionally, this helps organizations maintain compliance with air-gapped environments and enforce image provenance requirements.

See [Tool Documentation](#) for installation and advanced usage. The main documentation structure includes:
*   Installation Guide (Covered in `README.md` and `USE-CASES.md`)
*   Quick Start & Usage Guide (Covered in `README.md` and `USE-CASES.md`)
*   CLI Reference (Covered in `cli-reference.md`)
*   Configuration File Format (Future)
*   Troubleshooting & Error Codes (Covered in `troubleshooting.md` and `DEVELOPMENT.md`)
*   Contributor Guide (Covered within this `DEVELOPMENT.md` document)

## 2. Problem Statement

Manually identifying and overriding all container image references (including those in subcharts) within a Helm chart to point to a specific target registry (like Harbor, ECR, ACR, GCR, etc.) is tedious, error-prone, and difficult to maintain. A tool is needed to automate this process reliably based on user-defined source registries. Additionally, this helps organizations maintain compliance with air-gapped environments and enforce image provenance requirements.

## 3. Goals

* Automate the generation of a Helm override YAML file.
* Redirect standard container image pulls from specified source registries to a target registry instance.
* Support common ways container images are defined in Helm `values.yaml` files.
* Integrate seamlessly into developer workflows and CI/CD pipelines.
* Ensure the process is transparent and verifiable.

## 4. Prerequisites

* The target registry (e.g., Harbor) must be operational and correctly configured *before* using this tool.
* If using a pull-through cache strategy (like with Harbor), the necessary remote registry endpoints and proxy cache projects must be configured within the target registry itself. This tool *only* generates the client-side Helm overrides.
* Refer to the specific registry's documentation for setup instructions (e.g., [Harbor Pull-through Cache Documentation](https://goharbor.io/docs/2.10.0/administration/configure-proxy-cache/) - *Note: Link is an example and may need updating*).

## 5. Requirements

### 5.1. Functional

* **Input:**
  * Helm chart path (directory or `.tgz` archive).
  * Target registry URL (e.g., `myharbor.local:5000`, `123456789012.dkr.ecr.us-east-1.amazonaws.com`).
  * List of source registries to redirect (e.g., `docker.io`, `quay.io`, `gcr.io`, `ghcr.io`).

* **Processing:**
  * Parse the chart's `values.yaml`.
  * Recursively process values for subcharts identified via `Chart.yaml` dependencies.
  * Identify standard container image references (typically via keys like `image`, `registry`, `repository`, `tag`).
  * Filter identified images based on the provided list of source registries.
  * Construct new image URLs pointing to the target registry using a defined strategy.

* **Output:**
  * Generate a YAML file containing *only* the necessary overrides in the correct Helm value hierarchy (e.g., `parentchart.subchart.image.repository`).

* **Initial Target Registry:** Harbor (specifically tested with pull-through cache configuration).

* **Artifact Scope:** Strictly limited to standard container image references in `values.yaml`. The tool does not analyze Helm templates or helper functions (e.g., tpl, include). Images dynamically generated via Helm templating (e.g., `{{ .Values.image }}:{{ .Chart.AppVersion }}`) will not be detected unless they are statically defined in values.yaml. Other OCI artifacts (Helm charts stored in OCI, WASM modules, etc.) are explicitly out of scope for initial versions.

### 5.2. Non-Functional

* **Usability:** Simple and clear CLI interface.
* **Performance:** Efficient processing for typical chart complexity.
* **Maintainability:** Clean, testable code adhering to KISS and SOLID principles.
* **Robustness:** Handle common value structures gracefully; provide informative errors for unsupported formats or missing prerequisites. The tool will detect and report unsupported image definition patterns.
* **Verifiability:** Support a two-step workflow (generate then apply) and facilitate easy comparison using `helm template`.
* **Portability:** Fully compatible with Linux and macOS. Windows support is not planned for initial versions.

### 5.3. Security Considerations

* **Input Validation:** Input parameters, especially file paths (`--chart-path`, `--output-file`, `--registry-file`), must be validated to prevent path traversal vulnerabilities.
* **Dependency Management:** Use Go modules for dependency management. Regularly scan dependencies for known vulnerabilities (e.g., using `govulncheck`).
* **Resource Handling:** Ensure proper handling of file descriptors and memory, especially when processing large charts or values files, to prevent resource exhaustion issues.
* **Output Sanitization:** While the primary output is structured YAML, ensure that registry/repository names derived from inputs are appropriately handled if used in logging or error messages to prevent potential injection issues (though less likely in this context).

## 6. Design & Architecture

### 6.1. Core Logic

1. **Chart Loading:** Utilize Go libraries (`helm.sh/helm/v3/pkg/chartutil`, `helm.sh/helm/v3/pkg/chartloader`) to load chart information, including `values.yaml` and `Chart.yaml`. Use standard YAML parsing (`sigs.k8s.io/yaml` or similar).
2. **Value Traversal:** Implement recursive function to walk the nested dictionary representing the loaded `values.yaml`.
3. **Image Identification:** Heuristically identify image specifications by looking for specific key patterns and value formats within maps. Initially, the tool will explicitly support:
    *   A map containing `registry`, `repository`, and `tag` string keys.
    *   A map containing `repository` and `tag` string keys (implies default `docker.io` registry).
    *   A single string value assigned to a key named `image` (e.g., `image: myrepo/myimage:tag`), which will be parsed into its components.
    *   Structures within lists resembling these patterns will be detected, and a warning will be issued, but they will not be processed for overrides in the initial version. Processing will be based on findings from testing diverse charts.
    *   Other structures (e.g., maps with only `repository`, maps using non-standard keys, values that only incidentally match image patterns) are explicitly **not** supported in the initial version. Support may be added based on testing feedback.
4. **Image Parsing:** Extract source registry, repository path, and tag/digest from identified image references. Handle default registry (`docker.io`) if not explicitly specified.
5. **Filtering:** Match the extracted source registry against the user-provided list.
6. **Target URL Construction:** Generate the new image URL using a configurable strategy.
7. **Override Structure Generation:** Build a new dictionary mirroring the path to the original value within the `values.yaml` structure. This dictionary will contain **only** the minimal set of keys required to redirect the image according to the chosen path strategy (typically just `repository`, or `registry` and `repository` if the registry needs explicit setting). Unmodified sibling keys like `tag` or `pullPolicy` will **not** be included in the generated override file. Handle subchart paths correctly using the alias defined in the parent chart's `Chart.yaml` dependencies section (e.g., `subchartAlias.image.repository`).
8. **Output:** Serialize the override dictionary to YAML format.

### 6.1.1. Unsupported Structures and Error Handling

The tool will explicitly detect and provide warnings for the following unsupported patterns:

* Images split across multiple keys outside the expected structure (e.g., `image.name/image.tag` with no clear `repository` field)
* Non-string tag values
* Invalid registry names or malformed image references

Error messages will be informative and specific. For example:

* "Invalid source registry 'foo;bar': contains illegal characters"
* "Cannot parse image reference 'invalid::image': invalid format"

When used with the `--strict` flag, the tool will fail on unrecognized image structures instead of simply warning about them.

### 6.1.2. Error Handling and Exit Codes

The tool will provide informative error messages and utilize standard exit codes to facilitate scripting and CI/CD integration:
*   **Exit Code 0:** Success.
*   **Exit Code 1:** General runtime error (e.g., unexpected processing failure).
*   **Exit Code 2:** Input/Configuration Error (e.g., invalid chart path, unreadable file, invalid registry format, conflicting CLI flags).
*   **Exit Code 3:** Chart Parsing Error (e.g., malformed `values.yaml` or `Chart.yaml`).
*   **Exit Code 4:** Image Processing Error (e.g., unparsable image reference string).
*   **Exit Code 5:** Unsupported Structure Error (Only used with `--strict` flag when an unsupported structure is detected).

Error messages will clearly indicate the nature and location of the problem where possible (e.g., "Error parsing image in values path 'parent.subchart.image': invalid format").

### 6.1.3. Image Pattern Regex

The tool uses the following regex pattern to identify and parse image references:

```regex
^(?:(?P<registry>docker\.io|quay\.io|gcr\.io|ghcr\.io)/)?(?P<repository>[a-zA-Z0-9\-_/.]+):(?P<tag>[a-zA-Z0-9\-.]+)$
```

For digest-based references, an extended pattern is used:

```regex
^(?P<registry>docker\.io|quay\.io|gcr\.io|ghcr\.io/)?(?P<repo>[a-zA-Z0-9\-_/.]+)(?:@(?P<digest>sha256:[a-fA-F0-9]{64}))?$
```

The tool prioritizes tag-based references but will properly handle digest-based references when encountered.

### 6.1.4. Docker Library Image Handling

The tool implements special handling for Docker Library images:

```python
def normalize_docker_library(image):
    if '/' not in image.split(':')[0]:
        return f'docker.io/library/{image}'
    return image
```

This ensures proper processing of both implicit Docker Hub references (e.g., `nginx:latest`) and explicit ones (e.g., `docker.io/library/nginx:latest`).

### 6.1.5. Private Registry Exclusion

The tool provides a mechanism to exclude certain registries from processing:

```yaml
# Configuration file (migration-config.yaml)
exclude_registries:
  - "internal-registry.example.com"
  - "cr.private-domain.io"
```

When an image reference matches an excluded registry, it will be skipped during processing to preserve references to private registries.

### 6.1.6 Registry Mapping Format

The tool supports registry mappings provided via a configuration file (typically passed with `--registry-file`). This allows redirecting specific source registries to different target paths or registries. Only the structured YAML format is supported:

1.  **Structured Format (Required):**
    *   Uses a top-level `registries:` key, containing a `mappings:` key.
    *   Contains a list of mapping objects, each with `source` and `target` keys.

    ```yaml
    # Example: Structured Format
    registries:
      mappings:
        - source: docker.io
          target: my-registry.example.com/docker-mirror
        - source: quay.io
          target: my-registry.example.com/quay-mirror
        - source: gcr.io
          target: different-registry.example.com/google-containers
      # Optional fields for more control:
      # defaultTarget: "your-fallback-registry.com/generic-prefix"
      # strictMode: false # Set to true to fail if a source registry isn't explicitly mapped
    ```

### 6.1.7 Testing Strategy
The codebase follows these testing principles:

1. **Filesystem Operations**
   - Use `afero.Fs` interface for all file operations
   - Production code uses `afero.OsFs`
   - Tests use `afero.MemMapFs` for:
     - Isolation from real filesystem
     - Consistent behavior across environments
     - No cleanup requirements
     - No permission issues
     - Safe path traversal testing

2. **Test Organization**
   - Unit tests focus on isolated components
   - Integration tests verify component interaction
   - Chart validation tests ensure real-world compatibility

3. **Test Data Management**
   - Test fixtures stored in `test-data/`
   - In-memory test files created per test
   - Proper cleanup in test teardown
   - Consistent file permissions

4. **Error Handling**
   - Test both success and failure paths
   - Verify error types and messages
   - Test permission scenarios
   - Validate path traversal protection

### 6.1.8 Path Strategy and Registry Mappings

**Path Strategy Interface**

The tool uses a flexible strategy pattern for generating image paths in the target registry:

```go
type PathStrategy interface {
    // GeneratePath creates the target image reference string based on the strategy.
    // It takes the original parsed reference and the overall target registry.
    GeneratePath(originalRef *image.ImageReference, targetRegistry string) (string, error)
}
```

**PrefixSourceRegistryStrategy (Default)**
- Uses the sanitized source registry name as a prefix for repository paths
- Example: `docker.io/library/nginx` → `target.registry/dockerio/library/nginx`
- Important: This strategy only returns the repository part (`dockerio/library/nginx`); the caller adds the target registry

**Registry Mapping Format**
The tool supports a simple YAML key-value mapping format:
```yaml
docker.io: target.registry/docker-mirror
quay.io: target.registry/quay-mirror
```

**Path Handling Requirements**
- Registry mapping files must be specified with absolute paths or relative paths within the working directory
- The tool validates paths to prevent directory traversal attacks
- Paths must use `.yaml` or `.yml` extensions

### 6.2. Technology Stack

* **Primary Language:** Go (preferred for direct Helm SDK integration).
* **Libraries:** `helm.sh/helm/v3/pkg/*`, `sigs.k8s.io/yaml`, standard Go libraries.

### 6.3. Two-Step Workflow Enforcement

The tool's sole output is the generated override file. It does *not* directly interact with `helm install/upgrade`. This design choice enables:

* **Review:** Users inspect changes before application.
* **Integration:** The tool fits cleanly into any workflow that already uses Helm.
* **Verification:** Generated overrides can be tested via `helm template` before live application.

### 6.4. Debugging Tests

The IRR tool provides comprehensive debug logging capabilities that are especially useful during test development and troubleshooting. For detailed information about debugging tests, refer to the [Debug Control section in TESTING.md](docs/TESTING.md#10-debug-control-in-tests).

The primary way to enable debug logging is by setting the `LOG_LEVEL=DEBUG` environment variable.

Key points about debug control in tests:

1. **Enabling debug output in tests**:
   ```bash
   LOG_LEVEL=DEBUG go test -v ./...
   # Or target specific tests
   LOG_LEVEL=DEBUG go test -v ./pkg/specific -run TestSpecific
   ```

Refer to the comprehensive Debug Logging section in [TESTING.md](docs/TESTING.md#debug-logging) for more detailed information about capturing debug output, testing debug behavior, and best practices for debug testing.

## 7. Command-Line Interface (Phase 4)

The Phase 4 implementation focuses on a standalone CLI with three core commands:

### 7.1. `irr inspect`

*   **Purpose:** Inspect a chart's values to discover container images without modification. Optionally filter results by source registry.
*   **Inputs:**
    *   `--chart-path <path>` OR `--release-name <n>` (plus optional `--namespace`)
    *   Optional: `--source-registries <list>` (filter results to these registries)
    *   Optional: `--generate-config-skeleton <output_path>`
    *   Optional: `--output-file <path>` (defaults to stdout)
    *   Optional: `--registry-file <path>` (to identify excluded registries during analysis)
*   **Processing:** Loads chart/release values, traverses the structure, detects image patterns (`pkg/image`), categorizes by source registry, identifies value paths.
*   **Output:** YAML formatted report listing detected images, source registries, value paths, and any parsing issues. If `--generate-config-skeleton` is used, outputs a basic config file with detected source registries.

### 7.2. `irr override`

*   **Purpose:** Generate a Helm override values file to redirect images.
*   **Inputs:**
    *   `--chart-path <path>` OR `--release-name <name>`
    *   `--target-registry <url>` (required, unless fully defined via mappings)
    *   `--source-registries <list>` (Optional. If not provided, source registries are derived from enabled entries in the `registry-file`. If provided, this list explicitly defines which source registries to process, overriding derivation from the `registry-file`.)
    *   Optional: `--output-file <path>` (defaults to stdout)
    *   Optional: `--registry-file <path>` (for registry mappings, exclusions)
    *   Optional: `--strict` (fail on unsupported structures)
*   **Processing:** Loads chart/release values, loads configuration, detects images, filters based on source/exclude lists, generates new image paths using the default strategy (`prefix-source-registry`), constructs the minimal override map (`pkg/override`).
*   **Output:** YAML formatted override values file.

### 7.3. `irr validate`

*   **Purpose:** Perform a pre-flight check to ensure a chart renders successfully (`helm template`) with specified override values.
*   **Inputs:**
    *   `--chart-path <path>` OR `--release-name <name>`
    *   `--values <file>` (required, supports multiple occurrences for multiple value files)
*   **Processing:**
    *   If `--release-name` is provided, executes `helm get values <release>` to retrieve current values.
    *   Executes `helm template <chart-source> -f <current-values-if-release> -f <provided-values-1> [-f <provided-values-2> ...]`. Uses `internal/helm` helper.
*   **Output:**
    *   **Exit Code:** 0 for success (template rendered), non-zero for failure.
    *   **Stderr:** Passes through Helm's `stderr` output on failure to show the template error.
    *   **Stdout:** Minimal output on success, potentially indicating validation passed.
*   **Scope:** Does *not* analyze rendered templates or perform diffs. Purely checks `helm template` renderability.

### 7.4. Helm Plugin Interface

A Helm plugin (`helm irr`)  provides the core CLI commands:
*   Seamless integration (`helm irr inspect <release>`, etc.).
*   Namespace aware, Release aware

## 8. Target Registry Path Strategy

Defines how the original image path is mapped within the target registry.

### 8.1. `prefix-source-registry` (Default)

* **Mechanism:** Prepends a sanitized version of the source registry to the original repository path.
* **Example:**
  * Source: `docker.io/bitnami/redis:latest`
  * Target Registry: `myharbor.internal:5000`
  * Result: `myharbor.internal:5000/dockerio/bitnami/redis:latest`
  
  * Source: `nginx:latest` (implicit docker.io)
  * Target Registry: `myharbor.internal:5000`
  * Result: `myharbor.internal:5000/dockerio/library/nginx:latest`
* **Sanitization Rules:**
  * Registry domains are transformed to remove characters that are invalid in project names:
    * Periods (`.`) are removed: `gcr.io` → `gcrio`
    * Hyphens (`-`) are preserved: `k8s.gcr.io` → `k8sgcrio`
    * Port numbers are removed: `registry:5000` → `registry`
* **Pros:** Maintains logical separation based on origin within the target registry; clear lineage. Compatible with Harbor pull-through project naming conventions.
* **Cons:** Creates slightly longer image names. Requires corresponding setup in the target registry (e.g., Harbor project named `dockerio` proxying `docker.io`).

### 8.2. `flat` (Removed)

* This strategy is no longer supported.

## 9. Future Considerations

### 9.1. Supported Target Registries

* **Initial Focus:** Harbor (pull-through cache).
* **Planned Future Support (Design Consideration):** The core override mechanism is expected to work, but URL construction and path strategies might need adjustments for:
  * Quay.io / Red Hat Quay
  * AWS Elastic Container Registry (ECR) - Note: ECR requires repository paths to adhere to specific naming rules (lowercase, no underscores), which may conflict with prefix-source-registry strategy.
  * Google Cloud Artifact Registry / Container Registry (GCR)
  * Azure Container Registry (ACR)
  * GitHub Packages (ghcr.io)
* **Explicitly Out of Scope:** JFrog Artifactory, Sonatype Nexus Repository. While potentially feasible, they are not planned targets.

### 9.2. Enhanced Features

* Support for analyzing rendered templates (complex, deferred).
* Validation of generated target URLs (e.g., basic format check).
* More sophisticated image identification heuristics.
* Image signature validation integration (out of scope but may be relevant for registry integration).
* Custom Key Patterns: Introduce functionality via `--registry-file` to specify custom detection logic or paths.
* Multi-strategy support: Potentially allow different path translations based on mappings in `--registry-file`.

### 9.3. Enhanced Image Reference Handling

Future versions may include improved support for:

* **Digest-based References**: Full support for image references using SHA256 digests instead of tags.
* **Private Registry Integration**: Comprehensive exclude patterns and integration with private registry authentication.
* **Thresholding System**: Configurable success thresholds with tiered reporting:
  ```json
  {
    "chart": "vault",
    "status": "WARNING",
    "details": "98.2% images matched (3/306 failed)"
  }
  ```
### 9.4. Cloud Provider Integration Features (Future Considerations)

This section outlines potential future features to enhance integration with cloud provider container registries, starting with AWS ECR.

#### 9.4.1. ECR Path Sanitization (Priority: P0, Effort: Low)

**Problem:**
ECR repository names must adhere to specific naming conventions (lowercase, no underscores, hyphens allowed). Current path strategies might generate invalid paths if source registries use characters disallowed by ECR (e.g., `