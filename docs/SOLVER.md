# Helm Chart Solver Documentation

## Overview

The Chart Solver is a combinatorial optimization approach to find the minimal set of configuration values needed to successfully process different categories of Helm charts. This document outlines the design, implementation strategy, and considerations for this feature.

## Problem Definition

### Input
- Set of local Helm charts (e.g., 888 charts)
- Chart attributes:
  - Name
  - Registry source
  - Version
  - Current classification

### Goal
- Find minimal sets of configuration values that maximize successful chart processing
- Group charts into categories to avoid individual chart-specific rules
- Optimize for minimal configuration combinations while maximizing success rate

### Constraints
- Must work with `--local` charts only
- Should minimize the number of custom rules
- Must maintain high success rate (e.g., >95%)

## Key Configuration Parameters

The solver considers the following configuration parameters that affect chart processing success:

1. **Kubernetes Version**
   - Parameter: `--set kubeVersion`
   - Impact: Version compatibility checks
   - Common values: 1.24.0, 1.25.0, 1.26.0, etc.

2. **Security Settings**
   - Parameter: `allowInsecureImages`
   - Scope: Global and component-specific
   - Common in Bitnami charts

3. **Storage Configurations**
   - Storage class names
   - Persistence settings
   - Volume configurations

4. **Authentication Settings**
   - Redis auth
   - PostgreSQL auth
   - MongoDB auth
   - Generic credentials

5. **Global Registry Settings**
   - Registry URLs
   - Pull secrets
   - Mirror configurations

6. **Service Configurations**
   - Service types
   - Port configurations
   - Load balancer settings

7. **Resource Configurations**
   - Memory requests/limits
   - CPU requests/limits
   - Storage quotas

## Algorithm Structure

### 1. Initial Classification
- Use existing chart classification (BITNAMI, STANDARD_MAP, etc.)
- Add new classifications based on chart attributes
- Group charts by common characteristics

### 2. Configuration Space
- Define all possible configuration parameters
- Create parameter groups (security, storage, auth, etc.)
- Define valid value ranges for each parameter

### 3. Combinatorial Testing
- Start with minimal configuration
- Systematically add parameter combinations
- Track success/failure for each combination
- Use binary search to reduce testing space

## Optimization Strategy

### 1. Chart Bucketing
- Create initial buckets based on chart source (bitnami, standard, etc.)
- Sub-bucket based on common failure patterns
- Track which configuration parameters affect each bucket

### 2. Parameter Reduction
- Start with all possible parameters
- Remove parameters that don't affect success rate
- Combine parameters that always work together
- Find minimal parameter sets per bucket

### 3. Success Rate Optimization
- Define minimum success threshold (e.g., 95%)
- Balance between number of buckets and parameter combinations
- Allow for bucket-specific parameter overrides
- Maintain global vs. bucket-specific parameter sets

## Implementation Design

### Data Structures

```python
class ConfigParameter:
    name: str
    values: List[Any]
    weight: float  # Impact on success rate
    dependencies: List[str]  # Other parameters this depends on

class ChartBucket:
    name: str
    charts: List[str]
    required_params: List[ConfigParameter]
    success_rate: float
    failure_patterns: Dict[str, int]

class SolverResult:
    buckets: List[ChartBucket]
    global_params: List[ConfigParameter]
    success_rate: float
    total_param_combinations: int
```

### Algorithm Flow
1. Initial bucket creation
2. Parameter space exploration
3. Success rate optimization
4. Bucket refinement
5. Final parameter set minimization

## Advantages and Considerations

### Advantages
1. Systematic exploration of configuration space
2. Data-driven optimization of parameter sets
3. Reusable results for future chart processing
4. Clear categorization of charts and their requirements

### Challenges to Address
1. Computational complexity with large parameter spaces
2. Need for intelligent pruning of parameter combinations
3. Handling of chart-specific edge cases
4. Balancing granularity vs. maintainability

## Integration with test-charts.py

### New Command-Line Options
```bash
test-charts.py --solver-mode [options]
  --solver-threshold FLOAT    Minimum success rate threshold (default: 0.95)
  --solver-max-buckets INT   Maximum number of chart buckets (default: 10)
  --solver-max-params INT    Maximum parameters per bucket (default: 5)
  --solver-output FILE       Output file for solver results (default: solver-results.json)
```

### Example Usage
```bash
# Run solver on local charts
test-charts.py --local --solver-mode --solver-threshold 0.98

# Run solver with custom parameters
test-charts.py --local --solver-mode \
  --solver-threshold 0.95 \
  --solver-max-buckets 15 \
  --solver-max-params 7 \
  --solver-output custom-results.json
```

## Future Improvements

1. **Parameter Space Optimization**
   - Implement smarter pruning algorithms
   - Add parameter importance weighting
   - Develop heuristics for parameter combinations

2. **Bucket Refinement**
   - Add automatic bucket splitting/merging
   - Implement bucket similarity analysis
   - Add support for hierarchical buckets

3. **Result Analysis**
   - Add detailed success rate analysis
   - Generate parameter impact reports
   - Create visualization of bucket relationships

4. **Performance Optimization**
   - Implement parallel testing
   - Add incremental solving capability
   - Optimize parameter space exploration

## Questions to Consider

1. How do we handle charts that require unique configurations?
2. What is the optimal balance between number of buckets and success rate?
3. How do we validate the solver's results?
4. Should we implement a feedback loop for continuous optimization?
5. How do we handle parameter dependencies and conflicts?
6. What metrics should we track to evaluate solver effectiveness?

## Next Steps

1. Implement basic solver infrastructure in test-charts.py
2. Create initial parameter space definition
3. Implement bucket management system
4. Add combinatorial testing logic
5. Develop result analysis and reporting
6. Create validation framework for solver results

## References

- [TESTING.md](TESTING.md) - General testing guidelines
- [CHART-TESTING-TARGETS.md](CHART-TESTING-TARGETS.md) - Chart testing strategy
- [TESTING-COMPLEX-CHARTS.md](TESTING-COMPLEX-CHARTS.md) - Complex chart handling
- [USE-CASES.md](USE-CASES.md) - Tool use cases and workflows 