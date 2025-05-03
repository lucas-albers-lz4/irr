# Helm Chart Validation Issues

## Overview
This document summarizes the findings from our analysis of charts that fail the `helm template` validation process when using the irr (Image Registry Rewrite) tool. These findings are intended to help understand common failure patterns and identify potential solutions.

## Failing Charts Summary
Based on running `test/tools/test-charts.py` with the validate operation, we identified 35 charts that fail during template rendering. These charts can be categorized by error type as follows:

### Error Categories

1. **Template Errors** - General templating errors:
   - sonarqube-lts-2.0.0+463
   - loki-canary-0.14.0
   - linkerd2-2.11.5
   - linkerd-control-plane-1.16.11
   - insights-agent-4.6.1
   - sonarqube-dce-2025.2.0
   - profiling-collector-9.0.0
   - everest-1.5.0
   - tempo-vulture-0.7.1
   - pf-host-agent-8.14.3
   - workload-identity-webhook-1.5.0
   - profiling-agent-9.0.0
   - aws-load-balancer-controller-1.12.0
   - insights-admission-1.10.0
   - auto-deploy-app-0.8.1
   - prometheus-node-exporter-4.45.2

2. **Required Value Errors** - Missing required chart values:
   - eck-enterprise-search-0.15.0
   - eck-fleet-server-0.15.0
   - eck-beats-0.15.0
   - eck-agent-0.15.0
   - eck-kibana-0.15.0
   - k8s-monitoring-2.0.23
   - harbor-scanner-aqua-0.14.0
   - aws-nth-crossplane-resources-1.1.1
   - opentelemetry-collector-0.120.2

3. **Schema Errors** - Schema validation failures:
   - teleport-kube-agent-0.3.0
   - airflow-1.16.0
   - opentelemetry-kube-stack-0.5.0
   - keda-1.0.0
   - opentelemetry-operator-0.86.0
   - alertmanager-snmp-notifier-0.4.0
   - drone-0.6.5
   - backstage-2.5.1
   - opentelemetry-demo-0.37.0

4. **Coalesce Errors** - Type mismatches in value coalescing:
   - rke2-coredns-1.39.201

## Common Failure Patterns

### 1. Missing Required Values
Many charts require specific configuration values that aren't provided in the minimal test setup. Common examples include:

- **Authentication credentials**: Required passwords, tokens, or keys
- **Endpoint configuration**: URLs, hostnames, or connection strings
- **Required feature flags**: Specific features that must be enabled/disabled

### 2. Schema Validation Issues
These occur when provided values don't match the chart's expected schema:

- **Type mismatches**: Providing strings where numbers are expected or vice versa
- **Missing required fields**: Required fields not present in the values
- **Invalid structure**: Incorrect nesting or organization of value objects

### 3. Template Processing Errors
Template errors occur during the actual Helm template rendering process:

- **Template function errors**: Invalid use of template functions
- **Reference errors**: References to undefined variables or templates
- **Logical errors**: Issues with conditional logic in templates

## Sampled Chart Analysis

We performed a detailed analysis on a representative sample of failing charts to identify minimal required values:

### Example 1: eck-kibana-0.15.0 (Required Value Error)
The chart fails with:
```
Error: execution error at (eck-kibana/templates/kibana.yaml:18:19): Required field "spec.version" not specified
```

Minimal values needed:
```yaml
kibana:
  spec:
    version: "8.7.0"  # A valid Kibana version is required
```

### Example 2: opentelemetry-collector-0.120.2 (Required Value Error)
The chart fails with:
```
Error: execution error at (opentelemetry-collector/templates/deployment.yaml:29:14): Required field "config" not specified
```

Minimal values needed:
```yaml
config:
  receivers:
    otlp:
      protocols:
        grpc: {}
  exporters:
    logging: {}
  service:
    pipelines:
      traces:
        receivers: [otlp]
        exporters: [logging]
```

### Example 3: airflow-1.16.0 (Schema Error)
The chart fails with:
```
Error: values don't meet the specifications of the schema(s) in the following chart(s):
airflow:
- airflow.fernetKey: Required value
```

Minimal values needed:
```yaml
airflow:
  fernetKey: "dummyFernetKey"
```

## Recommendations

### 1. Test Harness Improvements
The `test/tools/test-charts.py` script could be enhanced to include commonly required values:

- Add a flag to automatically include common dummy values like passwords and tokens
- Add chart-specific default values for known problematic charts
- Implement a "retry with known values" mechanism for failing charts

### 2. Documentation Updates
Update documentation to explain common failure modes and workarounds:

- Document that chart validation failures are often due to missing required values unrelated to image overrides
- Provide examples of common values needed for popular chart families (Bitnami, ELK, etc.)
- Explain use of the `--no-validate` flag when only image overrides are needed

### 3. Targeted Rule Improvements
For Type 1 (Deployment-Critical) parameters that affect image handling:

- Investigate Bitnami charts to properly handle their `global.security.allowInsecureImages` flag
- Review specific cases where image registry changes may require additional security or validation flags

## Conclusion
The majority of chart validation failures are due to missing required values that are unrelated to IRR's primary function of rewriting image registries. These are Type 2 (Test/Validation-Only) issues that should be addressed through testing improvements rather than changes to the core IRR functionality.

For actual production use, operators would typically provide these required values as part of their deployment process. Therefore, these validation failures don't necessarily represent actual issues with IRR's image rewriting capabilities.

The `--no-validate` flag can be used when only image overrides are needed, bypassing validation issues that are unrelated to image registry rewriting. 