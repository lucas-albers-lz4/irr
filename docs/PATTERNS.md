# Helm Chart Image Patterns

This document outlines common patterns discovered in Helm charts regarding image references and container structures.

## Common Image Reference Patterns

### Map Structure
The most common pattern uses a map structure with separate fields for registry, repository, and tag:
```yaml
image:
  registry: docker.io
  repository: bitnami/redis
  tag: 7.0.0
```

### Container Arrays
Container definitions commonly appear in several array structures:

1. **Regular Containers**
   ```yaml
   containers:
     - name: main
       image: ${registry}/${repository}:${tag}
   ```

2. **Init Containers**
   ```yaml
   initContainers:
     - name: setup
       image: ${registry}/${repository}:${tag}
   ```

3. **Sidecars**
   ```yaml
   sidecars:
     - name: metrics
       image: ${registry}/${repository}:${tag}
   ```

## Chart-Specific Patterns

### Redis Chart
- Uses map structure for all images
- Includes init containers for volume permissions
- Includes Redis Sentinel containers
- Uses metrics exporter sidecar

### MySQL Chart
- Uses map structure for all images
- Includes init containers for:
  - Volume permissions
  - Password updates
- Primary/Secondary replication setup
- Metrics exporter sidecar

## Scope of Image Overrides

Our tool focuses specifically on:
- Changing registry locations
- Preserving all other configurations:
  - Image pull policies
  - Security contexts
  - Resource limits
  - Other container configurations

## Testing Considerations

When testing image overrides:
1. Test all container types (main, init, sidecars)
2. Verify preservation of non-registry configurations
3. Check both single-container and multi-container pods
4. Validate replication configurations (primary/secondary) 