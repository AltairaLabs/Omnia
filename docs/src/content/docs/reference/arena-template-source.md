---
title: "ArenaTemplateSource CRD"
description: "Complete reference for the ArenaTemplateSource custom resource"
sidebar:
  order: 14
  badge:
    text: Enterprise
    variant: tip
---

:::note[Enterprise Feature]
ArenaTemplateSource is an enterprise feature. The CRD is only installed when `enterprise.enabled=true` in your Helm values. See [Installing a License](/how-to/install-license/) for details.
:::

The ArenaTemplateSource custom resource defines a source for discovering and fetching project templates. Templates are [PromptKit](https://promptkit.altairalabs.ai) projects that can be customized with variables and rendered to create new Arena projects.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaTemplateSource
```

## Overview

ArenaTemplateSource provides:

- **Multiple source types**: Git, OCI registry, or ConfigMap
- **Automatic template discovery**: Scans for `template.yaml` metadata files
- **Variable support**: Define configurable parameters for each template
- **Periodic sync**: Automatically fetches updates at configured intervals

## Spec Fields

### `type`

The source type for fetching templates.

| Value | Description | Use Case |
|-------|-------------|----------|
| `git` | Git repository | Version-controlled templates |
| `oci` | OCI registry | Container registry storage |
| `configmap` | Kubernetes ConfigMap | Simple in-cluster storage |

```yaml
spec:
  type: git
```

### `syncInterval`

The interval between sync operations. Uses Go duration format. Default: `1h`.

| Format | Example | Description |
|--------|---------|-------------|
| `Xm` | `5m` | X minutes |
| `Xh` | `1h` | X hours |
| `XmYs` | `5m30s` | Combined duration |

```yaml
spec:
  syncInterval: 30m
```

### `templatesPath`

The path within the source where templates are located. Default: `templates/`.

```yaml
spec:
  templatesPath: my-templates/
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
    url: https://github.com/acme/arena-templates
    ref:
      branch: main
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
    url: https://github.com/acme/private-templates
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
    url: ssh://git@github.com/acme/private-templates.git
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
    url: oci://ghcr.io/acme/templates:v1.0.0
```

### `configMap`

Configuration for ConfigMap sources. Required when `type: configmap`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | ConfigMap name |

```yaml
spec:
  type: configmap
  configMap:
    name: my-templates
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
| `Fetching` | Currently fetching from source |
| `Scanning` | Templates are being discovered |
| `Ready` | Successfully fetched and templates discovered |
| `Error` | Fetch or discovery failed |

### `templateCount`

Number of templates discovered in the source.

### `headVersion`

The current commit SHA or revision hash.

### `artifact`

Information about the fetched content.

| Field | Description |
|-------|-------------|
| `contentPath` | Path where content is stored |
| `revision` | Source revision identifier |
| `lastUpdateTime` | When artifact was last updated |

### `conditions`

| Type | Description |
|------|-------------|
| `Ready` | Overall readiness of the source |
| `Fetching` | Currently fetching from source |
| `TemplatesDiscovered` | Templates have been discovered |

### `lastFetchTime`

Timestamp of the last fetch attempt.

### `nextFetchTime`

Scheduled time for the next fetch.

## Template Structure

Each template within the source must have a `template.yaml` file defining its metadata:

```yaml
# template.yaml
name: basic-chatbot
version: 1.0.0
displayName: Basic Chatbot
description: A simple conversational chatbot using PromptKit
category: chatbot
tags:
  - beginner
  - conversation

variables:
  - name: agentName
    type: string
    description: Name of the agent
    required: true
    default: "Assistant"

  - name: temperature
    type: number
    description: Model temperature (0.0-1.0)
    default: "0.7"
    min: "0"
    max: "1"

  - name: provider
    type: enum
    description: LLM provider to use
    options:
      - openai
      - anthropic
      - ollama
    default: openai

files:
  - path: prompts/
    render: true
  - path: arena.config.yaml
    render: true
  - path: static/
    render: false
```

### Variable Types

| Type | Description | Validation Fields |
|------|-------------|-------------------|
| `string` | Text value | `pattern` (regex) |
| `number` | Numeric value | `min`, `max` |
| `boolean` | True/false | - |
| `enum` | Predefined options | `options` (required) |

Variables use Go template syntax in template files:

```yaml
# arena.config.yaml
name: {{ .agentName }}
provider:
  type: {{ .provider }}
  temperature: {{ .temperature }}
```

For more information on PromptKit project structure, see the [PromptKit documentation](https://promptkit.altairalabs.ai/docs/getting-started).

## Complete Examples

### Git Repository Source

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaTemplateSource
metadata:
  name: company-templates
  namespace: workspace-ns
spec:
  type: git
  syncInterval: 30m
  templatesPath: templates/

  git:
    url: https://github.com/acme/arena-templates
    ref:
      branch: main

status:
  phase: Ready
  templateCount: 5
  headVersion: abc123def456
  artifact:
    contentPath: "arena/template-content/company-templates"
    revision: main@sha1:abc123def456
    lastUpdateTime: "2025-01-16T10:00:00Z"
```

### Community Templates

Omnia ships with a built-in community templates source:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaTemplateSource
metadata:
  name: community-templates
  namespace: workspace-ns
spec:
  type: git
  syncInterval: 1h

  git:
    url: https://github.com/AltairaLabs/arena-templates
    ref:
      branch: main
```

This is automatically created when `enterprise.communityTemplates.enabled=true` in your Helm values.

### Private Repository with Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: private-repo-creds
  namespace: workspace-ns
stringData:
  username: deploy-bot
  password: ghp_xxxxxxxxxxxx
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaTemplateSource
metadata:
  name: private-templates
  namespace: workspace-ns
spec:
  type: git
  syncInterval: 15m

  git:
    url: https://github.com/acme/private-templates
    ref:
      tag: v2.0.0
    secretRef:
      name: private-repo-creds
```

## Workflow

1. **Create ArenaTemplateSource** - Define the source location
2. **Controller fetches content** - Source is cloned/downloaded
3. **Templates discovered** - Controller scans for `template.yaml` files
4. **Templates available** - Browse and use templates in the Project Editor

```
ArenaTemplateSource ──▶ Fetch ──▶ Discover ──▶ Templates Ready
                                      │
                                      └──▶ template.yaml metadata
```

## Related Resources

- **[ArenaSource](/reference/arenasource)**: For fetching PromptKit bundles (not templates)
- **[ArenaJob](/reference/arenajob)**: Execute tests using Arena projects
- **[Project Editor](/how-to/use-arena-project-editor)**: Create projects from templates
