# Release Process

This document outlines the steps to create a new release of the irr plugin. The release process is partially automated using GitHub Actions.

## Prerequisites

- You have push access to the repository
- You have Git and GitHub CLI (optional) installed locally
- You understand semantic versioning (MAJOR.MINOR.PATCH)

## Release Steps

### 1. Update Version

First, update the version in `plugin.yaml`:

```yaml
# plugin.yaml
name: "irr"
version: "0.2.2"  # Update this version number
usage: "Rewrite image registries in Helm Charts"
description: "A Helm plugin to automatically generate Helm chart override files for redirecting container images."
command: "$HELM_PLUGIN_DIR/bin/irr"
hooks:
  install: "$HELM_PLUGIN_DIR/install-binary.sh"
useTunnel: false # Typically false for irr unless specific network reasons
```

**Important**: Make sure the version in `plugin.yaml` matches the version used in workflow dispatch parameters in `.github/workflows/release.yml`. When using the manual workflow trigger, the input version (e.g., `v0.2.2`) should correspond to the version in plugin.yaml (without the 'v' prefix).

### 2. Commit the Changes

```bash
git add plugin.yaml
git commit -m "Bump version to 0.2.2"
git push origin main
```

### 3. Create and Push a Tag

**Important:** Always create tags on the main/master branch after merging, not on feature branches.

Create a tag with the format `v{version}`:

```bash
# Ensure you're on the main branch
git checkout main
git pull

# Create a tag
git tag v0.2.2

# Push the tag to trigger the GitHub Actions workflow
git push origin v0.2.2
```

### 4. Monitor Release Process

The GitHub Actions workflow will automatically:

1. Run linting checks
2. Run tests
3. Build binaries for all supported platforms:
   - linux-amd64
   - linux-arm64
   - darwin-arm64
   - ~~darwin-amd64~~ (not currently supported in automated builds)
4. Create a GitHub release with the generated artifacts
5. Generate release notes

You can monitor the progress in the Actions tab of your GitHub repository.

### 5. Manual Release (Alternative)

If you prefer to manually trigger the release process:

1. Go to the "Actions" tab in your GitHub repository
2. Select the "Release" workflow
3. Click "Run workflow"
4. Enter the version tag (e.g., `v0.2.2`) in the input field
5. Click "Run workflow"

## Verifying the Release

After the release is published:

1. Check that all artifacts are correctly attached to the GitHub release
2. Verify that the plugin can be installed using:
   ```bash
   helm plugin install https://github.com/lalbers/irr
   ```
3. Test the plugin functionality

## Troubleshooting

If the release process fails:

1. Check the GitHub Actions logs for errors
2. Common issues include:
   - Tests failing
   - Lint issues
   - Version mismatch between tag and plugin.yaml
   - Permission issues

Fix any issues, then delete the tag and recreate it:

```bash
# Delete local tag
git tag -d v0.2.2

# Delete remote tag
git push --delete origin v0.2.2

# Fix issues, commit, then recreate and push the tag
git tag v0.2.2
git push origin v0.2.2
``` 