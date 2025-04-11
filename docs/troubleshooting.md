# Troubleshooting Guide

## Common Issues and Solutions

### Image Detection Issues

#### Template Variables in Image References

**Issue:** Warning about template variables in image fields
```yaml
image:
  repository: nginx
  tag: {{ .Chart.AppVersion }}
```

**Solution:** 
- This is expected behavior. The tool preserves template variables in the generated overrides.
- The final value will be resolved by Helm during template rendering.
- Verify that the template variable will resolve to a valid tag value.

#### Global Registry Not Applied

**Issue:** Images not using the global registry configuration
```yaml
global:
  imageRegistry: my-registry.example.com
image:
  repository: nginx
  tag: 1.23
```

**Solution:**
- Check that the global registry is defined at the root level of values.yaml
- Verify the key name matches the expected pattern (imageRegistry or registry)
- Use --verbose flag to see how registry values are being processed

### Path Handling

#### Array Index Issues

**Issue:** Images in array elements not being processed
```yaml
containers:
  - name: main
    image: nginx:1.23
  - name: sidecar
    image: fluentd:v1.14
```

**Solution:**
- Ensure the path matches known container patterns
- Use --verbose to see detected paths
- Check if the array path is in a supported format (e.g., containers[0].image)

#### Nested Structure Issues

**Issue:** Deep nested images not being detected
```yaml
spec:
  template:
    spec:
      containers:
        - image: nginx:1.23
```

**Solution:**
- Verify the complete path is recognized (use --verbose)
- Check if the path matches known Kubernetes patterns
- Ensure parent keys follow expected naming conventions

### Value Type Issues

#### Boolean/Numeric Confusion

**Issue:** Non-image values being incorrectly processed
```yaml
port: 8080
enabled: true
image: nginx:1.23
```

**Solution:**
- The tool now correctly handles non-image types
- Use --verbose to see type detection in action
- Check if the key matches any known non-image patterns

### Registry-Specific Issues

#### Docker Library Images

**Issue:** Unexpected library/ prefix
```yaml
# Input
image: nginx:1.23

# Output
image: my-registry.com/dockerio/library/nginx:1.23  # Why library/?
```

**Solution:**
- This is correct behavior for Docker official images
- The library/ prefix is added for docker.io single-name images
- No action needed - this maintains compatibility with Docker registry

#### Registry With Port

**Issue:** Registry port handling
```yaml
registry: my-registry.example.com:5000
```

**Solution:**
- Ports are preserved in registry URLs
- Verify the port is included in --target-registry if needed
- Check the generated path sanitization (ports are handled specially)

### Command Line Issues

#### Path Strategy Selection

**Issue:** Unexpected image paths in target registry
```yaml
# Expected
my-registry.com/nginx:1.23

# Got
my-registry.com/dockerio/nginx:1.23
```

**Solution:**
- Default strategy is prefix-source-registry
- Use --strategy flag to select different strategy
- Check documentation for available strategies

#### Source Registry Filtering

**Issue:** Some images not being processed
```yaml
# Not being processed
image: custom-registry.com/app:1.0
```

**Solution:**
- Verify registry is in --source-registries list
- Check for --exclude-registries conflicts
- Use --verbose to see registry matching logic

### Performance Issues

#### Large Chart Processing

**Issue:** Slow processing of large charts with many images

**Solution:**
- This is expected for complex charts
- Use --verbose to identify bottlenecks
- Consider processing subchart overrides separately

### Integration Issues

#### Helm Template Errors

**Issue:** Helm template fails with overrides
```bash
Error: template: chart/templates/deployment.yaml:54:19: executing "chart/templates/deployment.yaml" at <.Values.image.registry>: nil pointer evaluating interface {}.registry
```

**Solution:**
- Verify the override structure matches original
- Check for required but missing fields
- Use --dry-run to preview override structure

## Debug Tools

### Verbose Output

Use the --verbose flag to get detailed information about:
- Image detection process
- Path construction
- Registry matching
- Type detection
- Template variable handling

### Dry Run Mode

Use --dry-run to:
- Preview changes without writing files
- Validate override structure
- Check path strategy results
- Verify registry transformations

## Getting Help

If you encounter an issue not covered here:
1. Enable --verbose output
2. Run with --dry-run first
3. Check the generated override structure
4. Verify chart values structure
5. Open an issue with the above information 