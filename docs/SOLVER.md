# Helm Chart Solver Documentation

## Overview

The Chart Solver is a combinatorial optimization approach to find the minimal set of configuration values needed to successfully process different categories of Helm charts. This document outlines the design, the problem being solved, key concepts, and overall strategy for this feature.

## Problem Definition

### Input
- A collection of local Helm charts (e.g., from `test/chart-cache`)
- Chart attributes potentially derivable from metadata:
  - Name
  - Potential registry source/provider (e.g., Bitnami, standard)
  - Version
  - Dependencies

### Goal
- Identify the minimal sets of configuration values (`--set` parameters) required for successful chart processing (specifically, passing `helm template` or `irr validate`).
- Discover patterns and group charts into categories that share common configuration requirements, minimizing the need for chart-specific rules.
- Drive the creation of generalized rules within the IRR tool to automatically apply necessary *Deployment-Critical* parameters to generated override files.
- Differentiate requirements needed only for validation/testing from those needed for actual deployment.

### Constraints
- Primarily focused on analyzing charts available locally.
- Aim to minimize the number of distinct, hardcoded rules within IRR.
- Strive for a high success rate for chart processing with generated overrides.

## Key Configuration Parameters (Conceptual)

The solver considers various configuration parameters known to affect Helm chart processing success. These are candidates for inclusion in minimal parameter sets:

1.  **Kubernetes Version Context:** Simulating different Kubernetes API versions often required by chart templating logic (`.Capabilities.KubeVersion`).
2.  **Security Settings:** Chart-specific flags controlling security behaviors, like image verification bypasses (e.g., `allowInsecureImages`).
3.  **Storage Configurations:** Parameters related to persistence, volumes, and storage classes.
4.  **Authentication Settings:** Dummy or default credentials needed to satisfy template logic for components like Redis, PostgreSQL, etc.
5.  **Global Settings:** Parameters often found under a `global:` key, affecting registry, secrets, or other cross-chart settings.
6.  **Service Configurations:** Basic service types, ports, or ingress settings sometimes checked during templating.
7.  **Resource Configurations:** Default resource requests/limits, although less commonly required for basic templating.
8.  **Required Template Values:** Specific values often checked in `NOTES.txt` or validation hooks (e.g., endpoints, flags).

_Note: The solver's primary role is to identify which of these (or others) are needed to pass validation. A subsequent analysis step categorizes these findings._

## Parameter Categorization: Deployment-Critical vs. Test/Validation-Only

When analyzing solver results and chart requirements, it's crucial to distinguish between two types of parameters:

1.  **Deployment-Critical Parameters (Type 1):**
    *   **Definition:** These are configuration values that *must be present in the generated override file* for the Helm chart to function correctly during `helm install` or `helm upgrade` after image references have been modified by IRR.
    *   **Purpose:** They often configure chart behavior related to the modified images or dependencies.
    *   **Examples:**
        *   `global.security.allowInsecureImages=true`: Necessary for Bitnami/Tanzu charts to accept modified image references during deployment.
        *   Potentially specific endpoint configurations if they change based on registry overrides.
    *   **Solver Role:** The solver might identify these if their absence causes validation failure, but their necessity often stems from runtime chart logic related to overrides, requiring direct chart analysis (e.g., checking for image verification templates).
    *   **IRR Action:** The rule generation process (Phase 6) **must** identify these Type 1 parameters and create rules to automatically include them in the final `override.yaml` generated by `irr override`.

2.  **Test/Validation-Only Parameters (Type 2):**
    *   **Definition:** These are configuration values that may be required *only* to satisfy the `helm template` engine (used by `irr validate` or direct `helm template` calls) during local validation or testing.
    *   **Purpose:** They often simulate values that would normally be provided by the live Kubernetes cluster environment (like API versions) or satisfy conditional logic within templates that isn't relevant to the core override functionality.
    *   **Examples:**
        *   `kubeVersion`: Often checked by chart templates (`.Capabilities.KubeVersion`), but the actual value during deployment comes from the cluster API server. The solver identified `kubeVersion=1.25.0` as needed for `rancher-2.10.3.tgz` *validation*.
        *   Dummy credentials or endpoints needed only to pass template rendering checks.
    *   **Solver Role:** The solver *may* identify these as necessary to pass its internal `irr validate` step.
    *   **IRR Action:** These parameters **must NOT** be included in the final `override.yaml` generated by `irr override`. Hardcoding them would override real cluster values or add unnecessary configuration. They are relevant only for the *testing* or *validation* environment.

**Implications for Rule Generation:**

The rule generation system (Phase 6, Step 2) must explicitly filter solver results and analysis findings, creating rules *only* for Type 1 parameters destined for the override file. Information about necessary Type 2 parameters should be documented for testing procedures (e.g., noting that `irr validate` might require `--set kubeVersion=...` for certain charts) but not embedded in the override output.

## Algorithm Structure (Conceptual)

### 1. Initial Classification & Grouping
- Group charts based on metadata (provider, name patterns, dependencies) and observed failure patterns.
- Create initial "buckets" of similar charts.

### 2. Configuration Space Exploration
- Define a set of candidate parameters and potential values (see "Key Configuration Parameters").
- Systematically test parameter combinations against charts within each bucket.
- Prioritize parameters known to resolve common error categories (e.g., `kubeVersion` for compatibility errors).

### 3. Combinatorial Testing Strategy
- Start testing with a minimal configuration (e.g., empty set, or only globally required parameters).
- If minimal fails, explore combinations of parameters, potentially using strategies like:
    - **Targeted:** Add parameters specifically suggested by the error category encountered.
    - **Exhaustive (Limited):** Test combinations of a small number (1-3) of high-priority parameters.
    - **Binary Search:** If a large set of parameters works, attempt to remove subsets to find a minimal working set.
- Track success/failure for each combination tested per chart.

### 4. Result Analysis & Rule Derivation
- Analyze the results to find the smallest parameter set that achieved success for each chart.
- Identify common minimal parameter sets across charts within the same bucket/classification.
- Derive generalized rules based on these common sets (e.g., "Charts classified as 'BITNAMI' require parameter X").
- Categorize required parameters as Type 1 (for override file) or Type 2 (for testing only).

## Optimization Strategy (Conceptual)

### 1. Chart Bucketing
- Group charts dynamically based on shared requirements discovered during testing.
- Refine buckets iteratively as more data is gathered.

### 2. Parameter Prioritization & Pruning
- Use error analysis to prioritize testing parameters most likely to resolve common failures first.
- Avoid testing combinations known to be redundant or unlikely to succeed based on previous results.

### 3. Success Rate Optimization
- Define a target success rate for chart validation.
- Focus solver effort on the remaining failing charts/buckets.
- Accept that some charts may require manual configuration beyond the scope of automated solving.

## Advantages and Considerations

### Advantages
1.  Systematic approach to discovering chart configuration requirements.
2.  Data-driven basis for generating automated rules within IRR.
3.  Reduces the need for manual, per-chart configuration analysis.
4.  Provides insights into common Helm chart patterns and failure modes.
5.  Highlights the distinction between validation needs and deployment needs.

### Challenges to Address
1.  The potential complexity of the parameter space.
2.  Need for effective heuristics or strategies to limit testing combinations.
3.  Handling charts with highly unique or complex configuration requirements.
4.  Ensuring accurate categorization of parameters into Type 1 vs. Type 2.
5.  Managing the performance of running numerous validation tests.

## Questions to Consider (Design Level)

1.  How can we most reliably distinguish Type 1 (Deployment-Critical) from Type 2 (Test/Validation-Only) parameters through analysis?
2.  What is the most effective strategy for exploring the parameter space (targeted, exhaustive limited, binary search) to balance coverage and performance?
3.  What chart metadata (provider, annotations, dependencies, template content patterns) is most predictive of specific parameter requirements?
4.  How should the derived rules be structured and stored for consumption by the IRR Go application?
5.  What is an acceptable success rate for the automated rule system, acknowledging some charts will always need manual intervention?

## Next Steps (Conceptual)

1.  Refine the error analysis process to better link error messages to potential required parameters.
2.  Develop the logic for categorizing parameters into Type 1 and Type 2.
3.  Design the structure for the generalized rules file.
4.  Define the specific chart analysis/detection logic needed within IRR (Go) to apply the rules.
5.  Establish the testing methodology to validate the generated rules and the final override files.

## References

- [TODO.md](TODO.md) - Detailed implementation plan (Phase 6).
- [TESTING.md](TESTING.md) - General testing guidelines.
- [Parameter Categorization](#parameter-categorization-deployment-critical-vs-testvalidation-only) (This document) 