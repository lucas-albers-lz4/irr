# Advanced Testing for Helm Image Override

In addition to the standard unit and integration tests, this document outlines more advanced testing methods to validate real-world usage of the helm-image-override tool.

## End-to-End Image Validation

The `tools/validate_images.sh` script provides a way to validate that generated image overrides work with actual container registries. This is particularly useful for testing in environments with configured registry proxies or pull-through caches.

### Prerequisites

- Docker installed and configured
- Access to a test registry (Docker Registry, Harbor, etc.)
- Helm CLI installed

### Usage

```bash
./tools/validate_images.sh [TARGET_REGISTRY] [SOURCE_REGISTRIES] [CHART_PATH] [VALIDATE_PULL]
```

Examples:

```bash
# Generate overrides only
./tools/validate_images.sh localhost:5000 docker.io,quay.io ./charts/nginx false

# Generate overrides and validate by pulling images
./tools/validate_images.sh harbor.example.com docker.io,quay.io ./charts/prometheus true
```

### Output

The script generates:
1. A YAML file with image overrides
2. A text file listing all transformed image references
3. (Optional) Validation results from attempting to pull images

### Note

This script is intended for development/testing purposes only and is not part of the core functionality. It helps validate that the path generation strategies work correctly with real-world registries and pull-through cache setups.
