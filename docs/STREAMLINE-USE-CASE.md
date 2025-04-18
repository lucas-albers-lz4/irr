# IRR Streamlined Workflow Design Document

## 1. Introduction

This document outlines a streamlined approach for the IRR tool to simplify image relocation workflows. By implementing sensible defaults and consistent command patterns, we can significantly reduce the complexity for users while maintaining full functionality for advanced use cases.

## 2. Core Workflow

The primary workflow consists of sequential steps to inspect, override, validate, and apply image relocations.

## 3. Streamlining Recommendations

### 3.1 Default Behaviors

#### Source Registry Detection
- Make `--source-registries` optional in override command
- Default to all detected registries when omitted
- Add explicit `--all-registries` flag for clarity
- Preserve ability to specify specific registries

#### Default Override File Names
- Format: `<release-name>[-<namespace>]-overrides.yaml`
- Skip namespace in filename when it's "default"
- Never overwrite existing files (append incremental number if exists)

#### Integrated Validation
- `override` command should run validation by default
- Add `--no-validate` flag to skip validation
- Maintain standalone validate command for separate use

#### User Communication
- Clearly inform users about defaults being used
- Show source registries being processed
- Show path of generated override file

### 3.2 Flag Consistency

All commands should maintain consistent flag patterns:

| Flag | Used In | Description |
|------|---------|-------------|
| `--chart-path` | All | Path to chart (consistent across commands) |
| `--release-name` | All | Release name (alternative to chart-path) |
| `--namespace/-n` | All | Kubernetes namespace |
| `--output-file` | All | Output file path (generated if omitted) |
| `--source-registries` | inspect, override | Target registries (optional in override) |
| `--target-registry` | override | Target registry URL |
| `--values` | validate | Input values files |
| `--debug` | All | Enable debug logging |
| `--no-validate` | override | Skip validation step |

## 4. Example Commands

### 4.1 Basic Workflow

```
# List all releases to work with
helm list -A

# Inspect a specific release
helm irr inspect cert-manager -n cert-manager

# Generate overrides with defaults (uses all registries, auto-names file, runs validation)
helm irr override cert-manager -n cert-manager --target-registry registry.local
# Output: Generated cert-manager-cert-manager-overrides.yaml
# Output: Validation successful

# Explicitly specify registries
helm irr override cert-manager -n cert-manager --target-registry registry.local --source-registries docker.io,quay.io

# Apply the override with helm
helm upgrade cert-manager cert-manager/cert-manager -n cert-manager -f cert-manager-cert-manager-overrides.yaml
```

### 4.2 Batch Processing

```
# Generate script to inspect all releases
helm list -A | grep -v NAMESPACE | awk '{print "helm irr inspect "$1" -n "$2}' > inspect-all.sh

# Generate script to override all releases
helm list -A | grep -v NAMESPACE | awk '{print "helm irr override "$1" -n "$2" --target-registry registry.local"}' > override-all.sh
```

## 5. Implementation Guidelines

### 5.1 Output File Generation

When auto-generating filenames:
- Check if file exists before writing
- Add incremental suffix if needed (e.g., -1, -2)
- Inform user of generated filename

### 5.2 Validation Integration

For override command validation:
- Run validation silently by default
- Show detailed output only on error
- Use exit code to indicate success/failure

```go
func validateOverrides(chartPath, valuesPath string, quiet bool) error {
    // Run validation and capture output
    result, err := runValidation(chartPath, valuesPath)
    
    if err != nil {
        // Show detailed error on failure
        fmt.Fprintf(os.Stderr, "Validation failed: %s\n", err)
        fmt.Fprintf(os.Stderr, "%s\n", result.ErrorOutput)
        return err
    }
    
    if !quiet {
        fmt.Println("Validation successful")
    }
    
    return nil
}
```

### 5.3 Registry Detection

When defaulting to all registries:
- Cache registry detection results to avoid redundant analysis
- Display detected registries to user

```go
func detectAndDisplayRegistries(chartPath string) ([]string, error) {
    registries, err := detectRegistries(chartPath)
    if err != nil {
        return nil, err
    }
    
    fmt.Println("Detected registries:")
    for _, registry := range registries {
        fmt.Printf("  - %s\n", registry)
    }
    
    return registries, nil
}
```

## 6. Logging and User Feedback

- Helm currently uses color-coding for different message types (following Helm conventions)
- We don't, just to keep things simple
- Show clear success/failure status for each step

Example output format:

## 7. Kubernetes Version Handling

As noted in the documentation, many charts require specific Kubernetes versions. The streamlined workflow should:
- Default to a recent standard Kubernetes version (e.g., 1.29.0)
- Allow override with `--kube-version` flag
- Provide clear error messages if validation fails due to version issues



