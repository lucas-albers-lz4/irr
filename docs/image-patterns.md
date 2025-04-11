# Image Pattern Reference

This document details all the image reference patterns supported by the irr (Image Relocation and Rewrite) tool.

## Supported Patterns

### 1. Standard Image Map

The most common pattern using explicit fields:

```yaml
image:
  registry: docker.io
  repository: nginx
  tag: 1.23
```

### 2. Partial Image Map

Repository-only format with optional tag:

```yaml
image:
  repository: nginx
  tag: 1.23  # optional
```

When registry is omitted, it defaults to "docker.io".

### 3. String Format

Direct string representation:

```yaml
image: nginx:1.23
# or with registry
image: docker.io/nginx:1.23
```

### 4. Global Registry Pattern

Using a shared registry for multiple images:

```yaml
global:
  imageRegistry: my-registry.example.com
image:
  repository: nginx
  tag: 1.23
sidecar:
  repository: fluentd
  tag: v1.14
```

### 5. Container Arrays

Kubernetes-style container specifications:

```yaml
spec:
  template:
    spec:
      containers:
        - name: main
          image: nginx:1.23
        - name: sidecar
          image: fluentd:v1.14
```

### 6. Digest References

SHA256 digest instead of tag:

```yaml
image: nginx@sha256:1234567890123456789012345678901234567890123456789012345678901234
# or in map format
image:
  repository: nginx
  digest: sha256:1234567890123456789012345678901234567890123456789012345678901234
```

## Template Variables

The tool preserves Helm template variables:

```yaml
image:
  repository: nginx
  tag: {{ .Chart.AppVersion }}
  # or
  tag: {{ .Values.global.version | default "latest" }}
```

## Path Patterns

Known image-containing paths:

- `image`
- `*.image`
- `*.images[*]`
- `spec.template.spec.containers[*].image`
- `spec.template.spec.initContainers[*].image`
- `spec.jobTemplate.spec.template.spec.containers[*].image`

## Non-Image Patterns

Paths that are explicitly not processed as images:

- `*.enabled`
- `*.annotations.*`
- `*.labels.*`
- `*.port`
- `*.ports.*`
- `*.timeout`
- `*.serviceAccountName`
- `*.replicas`
- `*.resources.*`
- `*.env.*`
- `*.command[*]`
- `*.args[*]`

## Docker Library Images

Single-name images are treated as Docker official images:

```yaml
# Input
image: nginx:1.23

# Processed as
image: docker.io/library/nginx:1.23

# Output (with target-registry example.com)
image: example.com/dockerio/library/nginx:1.23
```

## Registry Handling

### Registry Normalization

- `docker.io` = `index.docker.io`
- Port numbers are preserved
- Registry names are sanitized for path components

### Registry Precedence

1. Image-specific registry
2. Global registry configuration
3. Default registry (docker.io)

## Validation Rules

### Repository Names

- Must be lowercase
- Can contain alphanumeric characters
- Can contain dots, hyphens, and underscores
- Cannot start or end with separators

### Tags

- Can contain alphanumeric characters
- Can contain dots, hyphens, and underscores
- Cannot contain spaces or special characters

### Digests

- Must start with "sha256:"
- Must be followed by 64 hexadecimal characters

## Examples

### Complex Chart Example

```yaml
global:
  imageRegistry: docker.io
  
nginx:
  image:
    repository: nginx
    tag: 1.23
    
prometheus:
  nodeExporter:
    image: quay.io/prometheus/node-exporter:v1.3.1
    
redis:
  master:
    image:
      registry: docker.io
      repository: bitnami/redis
      tag: {{ .Chart.AppVersion }}
  
  metrics:
    image: docker.io/bitnami/redis-exporter@sha256:1234...
```

### Generated Override Example

```yaml
global:
  imageRegistry: harbor.example.com

nginx:
  image:
    repository: dockerio/library/nginx
    tag: 1.23
    
prometheus:
  nodeExporter:
    image: quayio/prometheus/node-exporter:v1.3.1
    
redis:
  master:
    image:
      repository: dockerio/bitnami/redis
      tag: {{ .Chart.AppVersion }}
  
  metrics:
    image: harbor.example.com/dockerio/bitnami/redis-exporter@sha256:1234...
``` 