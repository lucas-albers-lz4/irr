
#1. (Optional, One-Time) Configure Registry Mappings
If not already set, the user configures their registry mappings. This is only needed once per environment or when mappings change.

```
helm irr config --source docker.io --target harbor.home.arpa/docker
helm irr config --source quay.io --target harbor.home.arpa/quay
helm irr config --source gcr.io --target harbor.home.arpa/gcr
helm irr config --source registry.k8s.io --target harbor.home.arpa/k8s
helm irr config --source ghcr.io --target harbor.home.arpa/github
helm irr config --source k8s.gcr.io --target harbor.home.arpa/k8s
```

#2. Inspect a Release
The user inspects a release to see which images are in use and which registries are detected. The tool suggests any missing mappings.

```
helm irr inspect cert-manager -n cert-manager
# Output: Detected registries: docker.io, quay.io
# Output: Suggestion: run 'helm irr config --source quay.io --target <your-target>' to add missing mapping.
```

#3. Generate Overrides
The user generates an override file for a release. The tool:
Uses sensible defaults for output file naming (cert-manager-cert-manager-overrides.yaml)
Auto-detects registries if --source-registries is omitted
Runs validation by default

```
helm irr override cert-manager -n cert-manager --target-registry harbor.home.arpa
# Output: Generated cert-manager-cert-manager-overrides.yaml
# Output: Validation successful
```

#4. Apply the Override with Helm

```
helm upgrade cert-manager cert-manager/cert-manager -n cert-manager -f cert-manager-cert-manager-overrides.yaml
```

5. Batch Processing (Optional)
See complete steps to apply:
```
helm list -A | grep -v NAMESPACE | awk '{print "helm irr override "$1" -n "$2" --target-registry harbor.home.arpa"}'
```

#Key Points:
No manual editing of YAML files required.
No interactive prompts; all config is flag-driven and scriptable.
Sensible defaults for output files and registry detection.
The tool guides the user if mappings are missing.
Validation is automatic, ensuring the override is usable.
