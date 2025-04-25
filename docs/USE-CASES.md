# IRR Tool Use Cases

This document outlines common workflows and use cases for the IRR (Image Relocation and Rewrite) tool, helping users understand how to effectively use the tool in various scenarios.

## User Personas

### Platform Engineer
Responsible for setting up and maintaining the infrastructure and deployment pipeline. Likely to configure registry mirrors, set up CI/CD processes, and establish organization-wide standards.

### Application Developer
Regularly deploys Helm charts for development and testing. Needs to redirect images to local or organization registries without deeply understanding the chart structure.

### DevOps/SRE
Manages production deployments and needs reliable image relocation for compliance, security scanning, or air-gapped environments.

## Installation Methods

### Standalone CLI

```bash
# Install the latest release
curl -L https://github.com/org/irr/releases/latest/download/irr-$(uname -s)-$(uname -m) -o irr
chmod +x irr
sudo mv irr /usr/local/bin/

# Verify installation
irr --version
```

### Helm Plugin

```bash
# Install as Helm plugin
helm plugin install https://github.com/org/irr

# Verify installation
helm irr --version
```

## Common Workflows

### 1. Initial Chart Analysis

When working with a new chart, first inspect it to understand its image structure:

#### Using Standalone CLI:
```bash
# Inspect local chart
helm irr inspect --chart-path ./my-chart

# Generate detailed report
helm irr inspect --chart-path ./my-chart --output-file report.yaml --output-format yaml

# Filter images by registry
helm irr inspect --chart-path ./my-chart --source-registries docker.io,quay.io
```

#### Using Helm Plugin:
```bash
# Inspect installed chart
helm irr inspect my-release

# Inspect chart before installation
helm irr inspect --chart-path ./my-chart
```

Expected output includes:
- List of detected images
- Source registry categorization
- Potential override paths
- Warnings about unsupported structures

### 2. Creating Configuration

Based on analysis, create or update your configuration file:

```bash
# Option 1: Manually create/edit the file
# vim ~/.irr.yaml

# Option 2: Generate a skeleton based on chart inspection (Recommended for new configs)
helm irr inspect --chart-path ./my-chart --generate-config-skeleton my-chart-config.yaml
# Now edit my-chart-config.yaml to add target registry mappings
```

Example configuration file structure:
```yaml
# ~/.irr.yaml or specified with --config
registry_mappings:
  docker.io: "registry.local/docker"  # Needs to be filled in by user
  quay.io: "registry.local/quay"      # Needs to be filled in by user
  # Add mappings for other detected source registries...

exclude_registries:
  - "internal-registry.company.com"
  - "custom-registry.org"

path_strategy: "prefix-source-registry"  # default
```

### 3. Generating Overrides

Generate the override values file to redirect images:

#### Using Standalone CLI:
```bash
# Generate overrides with global config
helm irr override --chart-path ./my-chart --output-file overrides.yaml

# Override with specific target
helm irr override --chart-path ./my-chart --target-registry registry.local \
  --source-registries docker.io,quay.io --output-file overrides.yaml
```

#### Using Helm Plugin:
```bash
# Generate overrides for installed chart
helm irr override my-release --output-file overrides.yaml

# Generate with specific config
helm irr override my-release --registry-file custom-config.yaml --output-file overrides.yaml
```

### 4. Validating Overrides

Before applying generated overrides, use `helm irr validate` as a pre-flight check to ensure they don't break Helm's templating engine. This command runs `helm template` internally using the provided overrides.

#### Using Standalone CLI:
```bash
# Validate overrides against a local chart
helm irr validate --chart-path ./my-chart --values overrides.yaml

# Validate with multiple value files (overrides applied last)
helm irr validate --chart-path ./my-chart --values base-values.yaml --values overrides.yaml
```

#### Using Helm Plugin:
```bash
# Validate overrides against an installed chart release
# This internally fetches the release's current values first
helm irr validate my-release --values overrides.yaml
```

**Expected Behavior:**
- **Success:** The command exits with code 0. Minimal output to stdout.
- **Failure:** The command exits with a non-zero code. Helm's template error message is printed to stderr, allowing you to diagnose the issue in your overrides or chart values.

Note: This command solely checks if `helm template` runs successfully; it does not analyze the rendered output.

### 5. Applying Overrides

Apply the validated overrides to deploy or update the chart:

```bash
# Apply overrides during installation
helm install my-release ./my-chart -f overrides.yaml

# Apply overrides during upgrade
helm upgrade my-release ./my-chart -f overrides.yaml
```

## Advanced Use Cases

### 1. CI/CD Pipeline Integration

Integrate IRR into your CI/CD pipeline to automatically generate overrides:

```yaml
# Example GitHub Actions workflow
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install IRR
        run: |
          curl -L https://github.com/org/irr/releases/latest/download/irr-linux-amd64 -o irr
          chmod +x irr
          sudo mv irr /usr/local/bin/
      
      - name: Generate Overrides
        run: |
          helm irr override --chart-path ./charts/my-app \
            --target-registry ${{ secrets.REGISTRY_URL }} \
            --source-registries docker.io,quay.io \
            --output-file overrides.yaml
      
      - name: Deploy with Helm
        run: |
          helm upgrade --install my-app ./charts/my-app \
            -f ./charts/my-app/values.yaml \
            -f overrides.yaml
```

### 2. Air-Gapped Environment

For air-gapped environments where external internet access is restricted:

1. **Preparation (Internet-connected environment):**
   ```bash
   # Generate override file
   helm irr override --chart-path ./my-chart --target-registry internal-registry.local \
     --source-registries docker.io,quay.io --output-file overrides.yaml
   
   # Use tool like skopeo to copy images to internal registry
   # (Not part of IRR, but complementary)
   skopeo copy docker://docker.io/nginx:latest docker://internal-registry.local/dockerio/nginx:latest
   ```

2. **Deployment (Air-gapped environment):**
   ```bash
   # Deploy chart with overrides
   helm install my-release ./my-chart -f overrides.yaml
   ```

### 3. Working with Complex Charts

For charts with multiple components and complex structures:

```bash
# Inspect with verbose output
LOG_LEVEL=debug helm irr inspect --chart-path ./kube-prometheus-stack

# Generate overrides with higher threshold
helm irr override --chart-path ./kube-prometheus-stack --threshold 90 --output-file overrides.yaml

# Validate the generated overrides
helm irr validate --chart-path ./kube-prometheus-stack --values overrides.yaml
```

### 4. Troubleshooting

For charts that have issues with image detection:

```bash
# Run with debug logging
LOG_LEVEL=debug helm irr inspect --chart-path ./problematic-chart

# Test with strict mode to see all issues
helm irr override --chart-path ./problematic-chart --strict --dry-run
```

## Best Practices

1. **Configuration Management**
   - Store registry configurations in version control
   - Use environment-specific configuration files
   - Document registry mappings for team reference

2. **Validation Workflow**
   - Always validate overrides before applying
   - Consider using `--dry-run` before generating final overrides
   - Test with non-production environments first

3. **CI/CD Integration**
   - Automate override generation in deployment pipelines
   - Include validation step before deployment
   - Consider caching override files for frequently used charts

4. **Monitoring & Maintenance**
   - Periodically re-analyze charts after updates
   - Update registry mappings when adding new source registries
   - Check for deprecated registry patterns 

## Streamlined Workflow Example (Helm Plugin)

This demonstrates a common, streamlined workflow using the `helm irr` plugin, assuming the user has already configured registry mappings globally or via `--config`.

### 1. (Optional, One-Time) Configure Registry Mappings

If not already set, configure registry mappings once per environment or when mappings change.

```bash
# Example using a hypothetical 'helm irr config' (Assuming global config setup)
# Or manually edit ~/.irr.yaml or provide via --config flag later
```
*(Note: Assuming mappings like `docker.io -> harbor.home.arpa/docker`, `quay.io -> harbor.home.arpa/quay`, etc. are configured)*

### 2. Inspect a Release

Inspect an installed release to see used images and verify mappings.

```bash
helm irr inspect cert-manager -n cert-manager
# Output example: Detected registries: docker.io, quay.io
# Output example (if mapping missing): Suggestion: run 'helm irr config --source quay.io --target <your-target>' to add missing mapping.
```

### 3. Generate Overrides

Generate overrides for the release. The tool uses sensible defaults for output naming and auto-detects registries if `--source-registries` is omitted.

```bash
helm irr override cert-manager -n cert-manager --target-registry harbor.home.arpa
# Output: Generated cert-manager-cert-manager-overrides.yaml
# Output: Validation successful (if validation runs by default)
```

### 4. Apply the Override with Helm

Apply the generated overrides using standard Helm commands.

```bash
helm upgrade cert-manager jetstack/cert-manager -n cert-manager -f cert-manager-cert-manager-overrides.yaml
```

### 5. Batch Processing (Optional)

Generate overrides for multiple releases (example using awk):

```bash
helm list -A | grep -v NAMESPACE | awk '{print "helm irr override "$1" -n "$2" --target-registry harbor.home.arpa"}' | sh
```

### Key Points of Streamlined Workflow:

*   Minimal manual editing of YAML.
*   Scriptable, non-interactive commands.
*   Sensible defaults reduce flag complexity.
*   Tool guides on missing configuration.
*   Validation can be integrated into the override step. 