# Testing Complex Charts

This document details the approach for testing complex Helm charts with the `irr` tool, focusing on the component-group testing methodology.

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