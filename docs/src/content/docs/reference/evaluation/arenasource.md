---
title: "ArenaSource CRD"
description: "Complete reference for the ArenaSource custom resource"
sidebar:
  order: 10
  badge:
    text: Enterprise
    variant: tip
---

:::note[Enterprise Feature]
ArenaSource is an enterprise feature. The CRD is only installed when `enterprise.enabled=true` in your Helm values. See [Installing a License](/how-to/operations/install-license/) for details.
:::

The ArenaSource custom resource defines a source for fetching PromptKit bundles. It supports Git repositories, OCI registries, Kubernetes ConfigMaps, and in-cluster workspace directories as sources, enabling GitOps-friendly bundle management for Arena Fleet.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaSource
```

**Short name:** `as` (e.g. `kubectl get as`).

## Overview

ArenaSource provides:

- **Multiple source types**: Git, OCI registry, ConfigMap, or in-cluster workspace directory
- **Automatic polling**: Configurable interval for detecting changes
- **Revision tracking**: Tracks source revisions for reproducibility
- **Content versioning**: Optional content-addressable versions on each sync
- **Artifact serving**: Provides content for workers to consume

## Spec Fields

### `type`

The source type for fetching PromptKit bundles.

| Value | Description | Use Case |
|-------|-------------|----------|
| `git` | Git repository | Version-controlled bundles |
| `oci` | OCI registry | Container registry storage |
| `configmap` | Kubernetes ConfigMap | Simple in-cluster storage |
| `workspace` | Workspace content directory | Snapshot an in-volume project (used by the dashboard deploy path) |

```yaml
spec:
  type: git
```

### `interval`

The reconciliation interval for polling the source. Uses Go duration format.

| Format | Example | Description |
|--------|---------|-------------|
| `Xm` | `5m` | X minutes |
| `Xh` | `1h` | X hours |
| `XmYs` | `5m30s` | Combined duration |

```yaml
spec:
  interval: 5m
```

### `git`

Configuration for Git repository sources. Required when `type: git`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes | Repository URL (https:// or ssh://) |
| `ref.branch` | string | No | Branch to checkout |
| `ref.tag` | string | No | Tag to checkout |
| `ref.commit` | string | No | Specific commit SHA |
| `path` | string | No | Path within repository (default: root) |
| `secretRef` | object | No | Credentials for private repos |

```yaml
spec:
  type: git
  git:
    url: https://github.com/acme/prompt-library
    ref:
      branch: main
    path: ./customer-support
```

#### Git Authentication

For private repositories, reference a Secret containing credentials:

**HTTPS Authentication:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-credentials
stringData:
  username: git-user
  password: ghp_xxxxxxxxxxxx  # GitHub PAT or password
---
spec:
  git:
    url: https://github.com/acme/private-prompts
    secretRef:
      name: git-credentials
```

**SSH Authentication:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-ssh-credentials
stringData:
  identity: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    ...
    -----END OPENSSH PRIVATE KEY-----
  known_hosts: |
    github.com ssh-rsa AAAAB3NzaC1yc2...
---
spec:
  git:
    url: ssh://git@github.com/acme/private-prompts.git
    secretRef:
      name: git-ssh-credentials
```

### `oci`

Configuration for OCI registry sources. Required when `type: oci`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes | OCI artifact URL |
| `secretRef` | object | No | Registry credentials |
| `insecure` | boolean | No | Allow insecure connections (default: false) |

```yaml
spec:
  type: oci
  oci:
    url: oci://ghcr.io/acme/prompts:v1.0.0
```

#### OCI URL Formats

| Format | Example |
|--------|---------|
| Tag | `oci://registry/repo:tag` |
| Digest | `oci://registry/repo@sha256:abc123...` |

#### OCI Authentication

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: registry-credentials
type: kubernetes.io/dockerconfigjson
stringData:
  .dockerconfigjson: |
    {
      "auths": {
        "ghcr.io": {
          "username": "user",
          "password": "token"
        }
      }
    }
---
spec:
  oci:
    url: oci://ghcr.io/acme/prompts:latest
    secretRef:
      name: registry-credentials
```

### `configMap`

Configuration for ConfigMap sources. Required when `type: configmap`.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | - | ConfigMap name |
| `key` | string | No | `pack.json` | Key containing the bundle |

```yaml
spec:
  type: configmap
  configMap:
    name: my-prompts
    key: pack.json
```

### `workspace`

Configuration for workspace-content sources. Required when `type: workspace`. Instead of fetching from an external system, the controller snapshots an existing directory on the workspace content volume — this is the path the dashboard deploy flow uses to promote an in-cluster project into an arena run.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Directory to snapshot, relative to the namespace's workspace content root (e.g. `arena/projects/my-project`). Absolute paths and `..` traversal are rejected. |

```yaml
spec:
  type: workspace
  interval: 5m
  workspace:
    path: arena/projects/customer-support
```

### `targetPath`

Where to sync content within the workspace content volume, relative to the workspace content root (`/workspace-content/{workspace}/default/`). If not specified, defaults to `arena/{source-name}`.

```yaml
spec:
  targetPath: arena/customer-support
```

### `createVersionOnSync`

Controls whether a new content-addressable version (SHA256) is created after each successful sync. Defaults to `true`. Set to `false` to sync content in place without accumulating version history.

```yaml
spec:
  createVersionOnSync: true
```

### `suspend`

When `true`, prevents the source from being reconciled. Useful for maintenance.

```yaml
spec:
  suspend: true
```

### `timeout`

Timeout for fetch operations. Default: `60s`.

```yaml
spec:
  timeout: 120s
```

## Status Fields

### `phase`

| Value | Description |
|-------|-------------|
| `Pending` | Source has not been fetched yet |
| `Initializing` | First fetch is in progress; content is not yet available |
| `Fetching` | A re-sync is in progress; previous content remains available via HEAD |
| `Ready` | Successfully fetched and artifact available |
| `Error` | Fetch failed |

### `artifact`

Information about the last successfully fetched artifact.

| Field | Description |
|-------|-------------|
| `revision` | Source revision identifier |
| `url` | Download URL for workers |
| `checksum` | SHA256 checksum |
| `size` | Artifact size in bytes |
| `lastUpdateTime` | When artifact was last updated |

### `conditions`

| Type | Description |
|------|-------------|
| `Ready` | Overall readiness of the source |
| `Fetching` | Currently fetching from source |
| `ArtifactAvailable` | Artifact is available for download |

### `lastFetchTime`

Timestamp of the last fetch attempt.

### `nextFetchTime`

Scheduled time for the next fetch.

## Complete Examples

### Git Repository Source

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaSource
metadata:
  name: customer-support-prompts
  namespace: arena
spec:
  type: git
  interval: 5m

  git:
    url: https://github.com/acme/prompt-library
    ref:
      branch: main
    path: ./customer-support

status:
  phase: Ready
  artifact:
    revision: main@sha1:abc123def456
    url: http://source-controller/artifacts/abc123.tar.gz
    checksum: sha256:789xyz...
    size: 12345
    lastUpdateTime: "2025-01-16T10:00:00Z"
```

### OCI Registry Source

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaSource
metadata:
  name: production-prompts
  namespace: arena
spec:
  type: oci
  interval: 1h

  oci:
    url: oci://ghcr.io/acme/prompts:v2.0.0
    secretRef:
      name: ghcr-credentials

status:
  phase: Ready
  artifact:
    revision: v2.0.0@sha256:abc123...
    url: http://source-controller/artifacts/v2.0.0.tar.gz
```

### ConfigMap Source

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-prompts
  namespace: arena
data:
  pack.json: |
    {
      "$schema": "https://promptpack.org/schema/latest/promptpack.schema.json",
      "id": "test-prompts",
      "name": "Test Prompts",
      "version": "1.0.0",
      "prompts": {
        "default": {
          "id": "default",
          "name": "Test",
          "version": "1.0.0",
          "system_template": "You are a helpful assistant."
        }
      }
    }
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaSource
metadata:
  name: test-prompts
  namespace: arena
spec:
  type: configmap
  interval: 1m

  configMap:
    name: test-prompts

status:
  phase: Ready
  artifact:
    revision: "12345"  # ConfigMap resourceVersion
    url: http://source-controller/artifacts/test-prompts.tar.gz
```

## Revision Format

The revision field format varies by source type:

| Source Type | Format | Example |
|-------------|--------|---------|
| Git (branch) | `branch@sha1:commit` | `main@sha1:abc123` |
| Git (tag) | `tag@sha1:commit` | `v1.0.0@sha1:abc123` |
| OCI (tag) | `tag@sha256:digest` | `v1.0.0@sha256:abc123` |
| OCI (digest) | `@sha256:digest` | `@sha256:abc123` |
| ConfigMap | `resourceVersion` | `12345` |

## Related Resources

- **[Arena Config File](/reference/evaluation/arenaconfig/)**: Schema of the `config.arena.yaml` file that lives inside the bundle this source provides
- **[ArenaJob](/reference/evaluation/arenajob/)**: Executes a run using this source (via `spec.sourceRef` and `spec.arenaFile`)
