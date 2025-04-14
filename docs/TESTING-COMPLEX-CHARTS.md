# Testing Complex Charts

This document details the approach for testing complex Helm charts with the `irr` tool, focusing on the component-group testing methodology.

## Kubernetes Version Compatibility

Many Helm charts require specific Kubernetes versions. During testing, we use the following approach to handle Kubernetes version compatibility:

### Setting Kubernetes Version for Testing

When testing charts with `irr validate` or using the `test-charts.py` script, we set these parameters:

```bash
--set kubeVersion=1.29.0
--set Capabilities.KubeVersion.Major=1
--set Capabilities.KubeVersion.Minor=29
--set Capabilities.KubeVersion.GitVersion=v1.29.0
```

These settings work in two ways:
1. `kubeVersion` is a chart parameter sometimes used to override the default Kubernetes version
2. The `Capabilities.KubeVersion.*` parameters directly inject values into the Helm templating engine's `.Capabilities.KubeVersion` object

Using both approaches ensures maximum compatibility with different chart implementation styles.

### Advanced Fallback Mechanisms

For charts that still have Kubernetes version compatibility issues, our test framework implements:

1. **Multiple Version Attempts**: When a chart fails validation with a Kubernetes version error, the script automatically tries with multiple versions (1.29.0, 1.28.0, 1.27.0, etc.) until it finds one that works.

2. **Targeted Chart Handling**: Certain charts with specific version requirements (like sonarqube, eck-*, traefik) get custom Kubernetes version settings (up to v1.30.0) during testing.

3. **Required Version Extraction**: The framework tries to parse the required version from error messages and prioritizes trying that specific version.

### Why This Works

The key to solving Kubernetes version compatibility issues is setting the `Capabilities.KubeVersion.*` parameters directly. This is especially effective because:

1. Many charts use the `.Capabilities.KubeVersion` object in template conditionals like:
   ```
   {{- if semverCompare ">=1.25.0-0" (include "common.capabilities.kubeVersion" .) -}}
   ```

2. By directly setting values in this object, we ensure these comparisons use our specified version rather than the default that Helm might use.

3. This approach works without requiring changes to the `irr` tool itself.

### Important Notes on Kubernetes Version Settings

1. The `kubeVersion` parameter is a Type 2 parameter (validation-only) as described in [RULES.md](RULES.md) - it should NOT be included in the final override file used for deployment.

2. When using `test-charts.py`, it automatically adds these version settings during validation.

3. When manually validating a chart, you should include these settings if you encounter version compatibility errors.

4. Different charts may have different minimum version requirements. Our testing standardizes on 1.29.0 but can use versions up to 1.30.0 for challenging charts.

5. For particularly stubborn charts, using the direct Helm command with `--kube-version` flag may be more effective:
   ```bash
   helm template release-name chart-path --kube-version v1.30.0 --values values-file.yaml
   ```

## Component-Group Testing Approach

When testing complex charts like `cert-manager` or `kube-prometheus-stack`, we use a specialized testing approach that breaks down testing into logical component groups. This improves:

- **Testability** - Enables focused testing on specific parts of a complex chart
- **Debugging** - Makes it easier to identify issues in specific components
- **Maintainability** - Creates clear structure for test additions and modifications
- **Reliability** - Allows different validation thresholds for different component types

### Key Concepts

1. **Component Groups** - Logical groupings of related components within a chart
   - Example: For cert-manager, "core_controllers" group includes the main controller and webhook
   - Each group has its own success threshold and criticality level

2. **Table-Driven Subtests** - A Go test structure that allows running specific subsets of tests
   - Uses `t.Run()` for subtests within a parent test
   - Subtests can be selectively executed with the `-run` flag
   - Component-specific subtests provide clearer error reporting

3. **Threshold-Based Validation** - Different validation thresholds for different component groups
   - Critical components might require 100% success
   - Supporting or optional components might use a lower threshold (e.g., 95%)

4. **Contextual Error Reporting** - All errors and messages include component group context
   - Makes it easier to determine which component group had an issue
   - Follows the error message format standards

## Implementation Structure

### Component Group Definition

Component groups are defined using a struct that includes:

```go
type ComponentGroup struct {
    name           string      // Group name for subtest identification
    components     []string    // Components in this group (for filtering)
    threshold      int         // Success threshold percentage
    expectedImages int         // Expected number of images to find
    isCritical     bool        // Whether failure is critical
}
```

### Example: cert-manager Component Groups

```go
componentGroups := []struct {
    name           string
    components     []string
    threshold      int
    expectedImages int
    isCritical     bool
}{
    {
        name:           "core_controllers",
        components:     []string{"controller", "webhook"},
        threshold:      100,
        expectedImages: 2,
        isCritical:     true,
    },
    {
        name:           "support_services",
        components:     []string{"cainjector", "startupapicheck"},
        threshold:      95,
        expectedImages: 2,
        isCritical:     false,
    },
}
```

## Running Component-Group Tests

### Running All Component Tests

```bash
# Run all tests for cert-manager
go test -v ./test/integration/... -run TestCertManager
```

### Running Specific Component Group Tests

```bash
# Run only the core controllers component group
go test -v ./test/integration/... -run TestCertManager/core_controllers

# Run with debug logging for a specific component group
LOG_LEVEL=DEBUG go test -v ./test/integration/... -run TestCertManager/support_services
```

## Guidelines for Adding New Component Groups

When adding component groups for a complex chart:

1. **Group Structure**
   - Group components based on functional relationships
   - Keep groups small (2-4 components per group)
   - Set appropriate thresholds based on criticality

2. **Expected Images**
   - Document the expected image count clearly
   - Use the version and tag that matches your test data

3. **Error Handling**
   - Include group name in all error messages
   - Use t.Logf for non-critical warns, t.Errorf for critical errors
   - Follow the structured error format from TESTING.md

4. **Validation Logic**
   - Define clear success/failure criteria
   - Apply different thresholds based on component criticality
   - Include clear summary of failures in test output

## Advantages of Component-Group Testing

- **Improved Isolation** - Issues in one component don't mask others
- **Better Debugging** - Debug logs are segregated by component
- **Selective Testing** - Run only the tests for components you're working on
- **Gradual Adoption** - Add component groups incrementally
- **Clearer Reporting** - Test results clearly show which components passed/failed

## See Also

- [TESTING.md](TESTING.md) - General testing guidelines and standards
- [DEVELOPMENT.md](DEVELOPMENT.md) - Development process and guidelines 