# Releasing Omnia

This document describes the release process for Omnia.

## Overview

Omnia uses a tag-based release process. When a git tag matching `v*` is pushed, GitHub Actions automatically:

1. Parses the version from the tag
2. Builds and pushes Docker images to GHCR
3. Packages and pushes the Helm chart to GHCR OCI registry
4. Builds and deploys documentation
5. Creates a GitHub Release with artifacts

## Versioning

Omnia follows [Semantic Versioning](https://semver.org/):

- **MAJOR**: Breaking changes to CRD APIs or behavior
- **MINOR**: New features, backward-compatible
- **PATCH**: Bug fixes, backward-compatible

### Pre-release Versions

Pre-release versions use suffixes:

- `0.2.0-alpha.1` - Early development, unstable
- `0.2.0-beta.1` - Feature complete, testing
- `0.2.0-rc.1` - Release candidate, final testing

Pre-releases:
- Do NOT update the `latest` Docker tag
- Are marked as pre-release on GitHub
- Include prerelease annotation in Helm chart

## Creating a Release

### Prerequisites

- You must have push access to the repository
- All tests must pass on the main branch
- Helm chart must lint successfully

### Using the Release Script (Recommended)

```bash
# Stable release
./scripts/release.sh 0.2.0

# Pre-release
./scripts/release.sh 0.3.0-beta.1
```

The script will:
1. Validate the version format
2. Check for uncommitted changes
3. Run local validation (lint, build, test)
4. Confirm the release
5. Create and push the git tag

### Manual Release

If you prefer to create releases manually:

```bash
# Ensure you're on main and up to date
git checkout main
git pull origin main

# Run tests
make test

# Lint the Helm chart
make helm-lint

# Create the tag
git tag -a v0.2.0 -m "Release v0.2.0"

# Push the tag
git push origin v0.2.0
```

## What Gets Released

### Docker Images

Images are pushed to GHCR:

```
ghcr.io/altairalabs/omnia:<version>
ghcr.io/altairalabs/omnia-facade:<version>
ghcr.io/altairalabs/omnia-runtime:<version>
```

For stable releases, additional tags are created:
- `latest` - Points to the latest stable release
- `<major>.<minor>` - Points to the latest patch in that minor version

### Helm Chart

The Helm chart is published to **two channels** on every release:

**1. OCI registry** (GHCR):
```
oci://ghcr.io/altairalabs/charts/omnia:<version>
```

Install with:
```bash
helm install omnia oci://ghcr.io/altairalabs/charts/omnia --version 0.2.0
```

**2. Traditional HTTPS Helm repository** (charts.altairalabs.ai):

The chart `.tgz` and an updated `index.yaml` are published to the `AltairaLabs/charts` repository, which is served via GitHub Pages at `https://charts.altairalabs.ai`.

Install with:
```bash
helm repo add altaira https://charts.altairalabs.ai
helm repo update
helm install omnia altaira/omnia --version 0.2.0
```

Prereleases (`*-alpha.*`, `*-beta.*`, `*-rc.*`) are included in the HTTPS repo and in OCI. Per Helm's SemVer conventions, `helm install` without `--version` will select the latest stable release by default; users wanting prereleases should pass `--devel` or an explicit `--version`.

**Required secret**: The release workflow needs a `CHARTS_REPO_TOKEN` secret configured on the Omnia repository. This should be a fine-grained Personal Access Token scoped to `AltairaLabs/charts` with `contents: write` permission, or a GitHub App installation token with equivalent scope.

### Documentation

Documentation is built and deployed to GitHub Pages at `omnia.altairalabs.ai`.

## Post-Release Checklist

After a release is complete:

1. **Verify the release**
   ```bash
   # Check Docker images
   docker pull ghcr.io/altairalabs/omnia:0.2.0

   # Check Helm chart
   helm pull oci://ghcr.io/altairalabs/charts/omnia --version 0.2.0
   ```

2. **Update CHANGELOG.md**
   - Move items from `[Unreleased]` to the new version section
   - Add the release date

3. **Announce the release** (if significant)
   - Update any relevant discussions or issues
   - Consider a blog post for major releases

## Troubleshooting

### Release Workflow Failed

1. Check the GitHub Actions logs
2. Common issues:
   - Docker build failures
   - Helm lint errors
   - Documentation build errors

### Rolling Back a Release

If a release has critical issues:

1. **Delete the GitHub Release** (if published)
2. **Delete the git tag**:
   ```bash
   git tag -d v0.2.0
   git push origin :refs/tags/v0.2.0
   ```
3. **Note**: Docker images and Helm charts cannot be deleted from GHCR easily. Consider releasing a patch version instead.

### Patching a Release

To release a patch for an existing version:

```bash
# From main branch with the fix
./scripts/release.sh 0.2.1
```

## Local Testing

Before creating a release, you can test the release process locally:

```bash
# Dry run (validates everything without pushing)
make release-dry-run
```

This will:
- Lint the Helm chart
- Build Docker images
- Package the Helm chart
- Build documentation
