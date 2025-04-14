# Chart Parameter Rules System Documentation

## Overview

The Chart Parameter Rules System is a component of the Image Registry Relocation (IRR) tool that automatically detects specific chart types and applies necessary configuration parameters to ensure successful deployment after image overrides are applied.

This system automatically identifies chart-specific requirements and distinguishes between parameters that are:
1. Required for successful deployment (Type 1 parameters)
2. Only needed for testing or validation (Type 2 parameters)

## Parameter Types

### Type 1: Deployment-Critical Parameters

These parameters **must be included** in the override file for the Helm chart to function correctly after image references have been modified by IRR. They are automatically included in the generated `override.yaml` file.

Examples:
- `global.security.allowInsecureImages=true` for Bitnami charts (to bypass security checks for modified images)
- Other chart-specific configuration values that are required for successful deployment

### Type 2: Test/Validation-Only Parameters

These parameters are only relevant in testing or validation environments and should **not** be included in the final override file used for deployment. They are not included in the generated `override.yaml`.

Examples:
- `kubeVersion` - used by some charts to simulate a specific Kubernetes version during templating
- Dummy credentials used only for validation
- Test-specific flags that aren't needed during real deployment

## Implemented Rules

### Bitnami Security Bypass Rule

**Purpose**: Adds `global.security.allowInsecureImages=true` to override files for Bitnami/Broadcom charts.

**Detection Method**: 
The system uses a tiered confidence approach to identify Bitnami charts:

1. **High Confidence** - Multiple indicators found:
   - Home field contains "bitnami.com"
   - Sources contain "github.com/bitnami/charts"
   - Maintainers reference "Bitnami" or "Broadcom"
   - Dependencies include "bitnami-common"

2. **Medium Confidence** - Typically two indicators found:
   - A combination of the indicators above

3. **Low Confidence** - Single indicator found:
   - Only one of the indicators above is present

The system applies the rule when the confidence level is Medium or High.

## CLI Flag

You can disable the rules system using the following flag:

```
--disable-rules   Disable the chart parameter rules system (default: enabled)
```

## How It Works

1. During override generation, the rules system analyzes the chart's metadata to detect its provider type (e.g., Bitnami)
2. For detected chart types, it applies the matching rules to add necessary parameters
3. For Bitnami charts, it adds `global.security.allowInsecureImages=true` to the override file
4. These parameters are then included in the final override file generated

## Extending the System

The rules system is designed to be extensible. New rules can be added to handle additional chart providers and specific chart requirements.

Future enhancements may include:
- Additional chart provider detection (VMware/Tanzu, standard repositories)
- Improved detection mechanisms
- Support for custom rule definitions
- Fallback mechanisms based on error detection 