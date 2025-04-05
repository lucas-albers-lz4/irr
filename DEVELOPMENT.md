# Helm Image Override Design Document

**Version:** 0.1.4
**Date:** 2023-10-27

## 1. Introduction

This document outlines the design for a command-line tool, `helm-image-override`, intended to automate the generation of Helm override `values.yaml` files. The primary goal is to redirect container image pulls specified within a Helm chart from various public registries to a designated local or private registry, initially focusing on Harbor configured as a pull-through cache. This addresses the manual effort and potential errors involved in modifying Helm deployments for environments requiring centralized, proxied, or private image access. Additionally, this helps organizations maintain compliance with air-gapped environments and enforce image provenance requirements.

See [Tool Documentation](#) for installation and advanced usage. Planned documentation sections include:
*   Installation Guide
*   Quick Start & Usage Guide
*   CLI Reference (Flags and Arguments)
*   Path Strategies Explained
*   Configuration File Format (Future)
*   Troubleshooting & Error Codes
*   Contributor Guide
Note: Documentation is under development.

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

* **Input Validation:** Input parameters, especially file paths (`--chart-path`, `--output-file`, potential `--config`), must be validated to prevent path traversal vulnerabilities.
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
6. **Target URL Construction:** Generate the new image URL using a configurable strategy (see Section 8).
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

### 6.1.6 Rationale for Value Handling

Processing Helm `values.yaml` files presents several complexities that necessitate the current approach to handling diverse YAML value types:

1.  **Helm's Flexibility & Diversity:** Helm charts do not enforce a strict, universal schema for `values.yaml`. Chart authors structure values in various ways. Images might be defined as simple strings (`image: registry/repo:tag`), maps (`image: { repository: repo, tag: tag }`), or even constructed using Helm template functions (`image: {{ include "myimage" . }}`). The tool must parse this heterogeneous input.
2.  **Raw Values vs. Templated Values:** The tool reads the *raw* `values.yaml` file *before* Helm's templating engine processes it. Consequently, the raw file often contains Go template syntax (`{{ ... }}`). These template strings are not valid literal values in the context where the tool initially processes them and can cause errors if not handled. The `cleanupTemplateVariables` function attempts to mitigate this by removing template syntax or substituting reasonable defaults (like `""` for image fields, `false` for boolean fields) to create a cleaner structure for the override logic.
3.  **Minimal Override vs. Structural Integrity:** The goal is to generate a *minimal* override file containing only the changes needed to redirect images. However, Helm applies these overrides *on top of* the original values. If the override file inadvertently removes too much structural context (e.g., parent keys, necessary non-image sibling keys like boolean flags), Helm's templating process might fail because the expected structure is broken.
4.  **Preserving Context:** To maintain structural integrity, the value processing logic (`processValues`) preserves basic data types (strings, booleans, numbers, null) encountered alongside image references. Even if a boolean flag like `enabled: true` is not being overridden, its presence might be necessary context for an adjacent image override. Discarding such values could lead to Helm errors. The tool effectively rebuilds parts of the original YAML structure where overrides occur, including necessary non-image values to ensure Helm can correctly merge the original values with the generated overrides.
5.  **Heuristic Identification:** Distinguishing image references from general configuration values (e.g., `replicas: 3`, `service.type: ClusterIP`) relies on heuristics (like the `isImageMap` function and pattern matching). Careful handling of different data types is crucial to avoid misidentifying values and causing errors.

In essence, the tool processes complex, varied, and template-filled input files to produce minimal yet structurally valid override files that correctly modify only image references without breaking downstream Helm processing.

### 6.2. Technology Stack

* **Primary Language:** Go (preferred for direct Helm SDK integration).
* **Libraries:** `helm.sh/helm/v3/pkg/*`, `sigs.k8s.io/yaml`, standard Go libraries.

### 6.3. Two-Step Workflow Enforcement

The tool's sole output is the generated override file. It does *not* directly interact with `helm install/upgrade`. This design choice enables:

* **Review:** Users inspect changes before application.
* **Auditing/VCS:** Override files can be committed.
* **Repeatability:** Consistent re-application of overrides.
* **Verification:** Easy comparison via `helm template diff` or similar methods.

### 6.3.1. Parallel Processing

The chart testing process supports parallel execution to improve performance when processing large numbers of charts:

* **Default Behavior:** Automatically uses parallel processing with GNU Parallel
* **Scaling:** Automatically scales to system capabilities
  * Uses half of available CPU cores
  * Minimum of 4 parallel jobs
  * Maximum of 16 parallel jobs
* **Sequential Fallback:** Supports `--no-parallel` flag for sequential processing
* **Requirements:** GNU Parallel must be installed for parallel execution
* **Implementation:** Each chart is processed in isolation with its own temporary directory to prevent conflicts

Example usage:
```bash
# Run with parallel processing (default)
./test/tools/test-charts.sh harbor.home.arpa

# Run sequentially
./test/tools/test-charts.sh harbor.home.arpa --no-parallel
```

### 6.4. CLI Interface (Proposed)

```bash
helm-image-override \
    --chart-path <path/to/chart_or_chart.tgz> \
    --target-registry <registry.example.com[:port]> \
    --source-registries <docker.io,quay.io,...> \
    [--output-file <path/to/override.yaml>] \ # Default: stdout
    [--path-strategy <prefix-source-registry|flat|...>] \ # Default: prefix-source-registry
    [--verbose] \ # Show detailed processing information
    [--dry-run] \ # Preview changes without writing file
    [--strict] \ # Fail on unrecognized image structures
    [--exclude-registries <internal-registry.example.com,cr.private-domain.io>] \ # Registries to exclude from processing
    [--threshold <percentage>] \ # Success threshold (default: 100)
```

## 7. Verification & Testing Strategy

* **Unit Tests:** Cover parsing, URL generation, path manipulation logic.
* **Integration Tests:**
  * Utilize a diverse set of popular Helm charts (e.g., top ~50 from Artifact Hub).
  * For each test chart:
    1. Generate `override.yaml` using the tool.
    2. Render manifests with `helm template <chart> --output-dir original`.
    3. Render manifests with `helm template <chart> -f override.yaml --output-dir overridden`.
    4. Programmatically compare `image:` fields in corresponding Kubernetes resources (Deployments, StatefulSets, etc.) between `original` and `overridden` directories.
    5. Assert that only images from specified `--source-registries` are rewritten to the `--target-registry` according to the `--path-strategy`.
* **Negative Test Cases:**
  * Test with malformed charts, invalid image references, and charts with no values.yaml.
  * Verify error messages are informative and specific.
* **Subchart Depth Testing:**
  * Verify recursive processing works for deeply nested subcharts (e.g., parent → child → grandchild).

### 7.1. Success Criteria and Thresholds

The tool implements configurable success thresholds to balance strictness with practicality:

| Metric          | Critical Threshold | Warning Threshold |
|-----------------|--------------------|-------------------|
| Image Match Rate| 100%               | 98%               |
| Chart Install   | 100%               | N/A               |

The default behavior is to fail if any image cannot be successfully processed, but this can be adjusted using the `--threshold` flag for testing or special cases.

### 7.2. Testing Matrix

Below is a test matrix for various image reference patterns:

| Input Type | Original Reference | Expected Output (`prefix-source-registry`) |
|------------|-------------------|------------------------------------------|
| Standard | docker.io/nginx:1.23 | myharbor.internal:5000/dockerio/nginx:1.23 |
| Nested Path | quay.io/project/img:v4.2 | myharbor.internal:5000/quayio/project/img:v4.2 |
| Implicit Registry | alpine:3.18 | myharbor.internal:5000/dockerio/library/alpine:3.18 |
| Digest-based | quay.io/prometheus/prometheus@sha256:1234... | myharbor.internal:5000/quayio/prometheus/prometheus@sha256:1234... |
| Docker Library | postgres:14 | myharbor.internal:5000/dockerio/library/postgres:14 |

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

### 8.2. `flat` (Potential Future Strategy)

* **Mechanism:** Discards the original source registry path, placing the image directly under the target registry URL.
* **Example:**
  * Source: `docker.io/bitnami/redis:latest`
  * Target Registry: `myharbor.internal:5000/proxied-images`
  * Result: `myharbor.internal:5000/proxied-images/bitnami/redis:latest`
* **Pros:** Simpler paths.
* **Cons:** Potential for naming collisions if different source registries have identical repository paths (e.g., `docker.io/library/nginx` vs `quay.io/library/nginx`); loses origin information in the path itself. Requires careful target registry setup.

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
* Custom Key Patterns: Introduce a `--config <path>` flag to specify a YAML/JSON file defining custom regex patterns (e.g., `customImage:.*`).
* Multi-strategy support: Allow different path strategies per source registry (e.g., prefix for Docker Hub, flat for GHCR).

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

## 10. CI/CD Integration

The tool is designed for easy integration into CI/CD pipelines:

1. **Checkout:** Obtain chart source.
2. **Generate Override:** Execute `helm-image-override` with environment-specific variables for `--target-registry` and potentially `--source-registries`. Output to a known file (e.g., `generated-override.yaml`).
3. **Deploy:** Use `helm install/upgrade` command, including the `-f generated-override.yaml` flag alongside other values files.

This ensures automated, consistent application of registry redirection rules across environments.

### Example Pipeline Snippet (GitHub Actions)

```yaml
- name: Generate Image Overrides
  run: |
    helm-image-override \
      --chart-path ./my-chart \
      --target-registry ${{ env.TARGET_REGISTRY }} \
      --source-registries docker.io,quay.io \
      --output-file ./overrides.yaml

- name: Deploy with Helm
  run: |
    helm upgrade --install my-release ./my-chart \
      -f ./values.yaml \
      -f ./overrides.yaml
```

## 11. Limitations

* **Templated Images:** Images generated via Helm templating (e.g., `{{ .Values.image }}:{{ .Chart.AppVersion }}`) are not detected unless statically defined in values.yaml.
* **Split Keys:** Images defined across multiple keys (e.g., image.name and image.tag) are not supported.
* **Non-String Tags:** Tags must be strings; numeric values (e.g., tag: 1.2.3) are not parsed as image tags.
* **Registry Ports:** Port numbers in source registries (e.g., docker.io:443) are ignored during filtering. The tool matches based on the registry domain only.
* **Digest Limitations:** While digest-based references are supported, complex digest formats or non-standard digest implementations may not work correctly.
