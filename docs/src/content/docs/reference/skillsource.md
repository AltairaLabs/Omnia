---
title: "SkillSource CRD"
description: "Reference for the SkillSource custom resource (skill content sync)"
sidebar:
  order: 11
---

The `SkillSource` custom resource fetches [AgentSkills.io](https://agentskills.io/specification)-formatted skill content from an upstream source (Git, OCI, or ConfigMap) into the workspace content PVC, where `PromptPack` resources can reference it via `spec.skills`.

A skill is a directory containing a `SKILL.md` file with YAML frontmatter (`name`, `description`, optional `allowed-tools`, `metadata`) and a Markdown body that PromptKit loads on demand.

## Example

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SkillSource
metadata:
  name: anthropic-skills
  namespace: dev-agents
spec:
  type: git
  git:
    url: https://github.com/anthropic/skills
    ref:
      tag: v1.4.0
  interval: 1h
  timeout: 5m
  targetPath: skills/anthropic
  filter:
    include:
      - "ai-safety/*"
      - "pdf-processing/*"
    exclude:
      - "**/draft-*"
```

## Spec

| Field | Type | Description |
|---|---|---|
| `type` | `git` \| `oci` \| `configmap` | Required. Selects the source variant; exactly one of `git`/`oci`/`configMap` must be set. |
| `git` | object | Git source: `url`, optional `ref` (`branch`/`tag`/`commit`), `path` (subdirectory), `secretRef`. |
| `oci` | object | OCI source: `url` (e.g. `oci://ghcr.io/org/repo:tag`), `insecure`, `secretRef`. |
| `configMap` | object | ConfigMap source: `name`. The fetcher reads SKILL.md files keyed by path (`__` ⇒ `/`). |
| `interval` | duration | Required. Reconciliation poll interval (e.g. `1h`). |
| `timeout` | duration | Per-fetch timeout (default `60s`). |
| `suspend` | bool | Pause reconciliation when `true`. |
| `targetPath` | string | Path under the workspace content PVC where synced content lands. Defaults to `skills/{source-name}`. |
| `filter` | object | Post-fetch filter — only directories matching are kept. See below. |
| `createVersionOnSync` | bool | Whether to create a content-addressable snapshot per sync (default `true`). |

### Filter

```yaml
filter:
  include:        # path globs (relative to targetPath); empty = include all
    - "safety-*"
  exclude:        # path globs applied after include
    - "**/draft-*"
  names:          # exact frontmatter `name:` matches; empty = no name filter
    - "ai-safety"
```

## Status

| Field | Description |
|---|---|
| `phase` | `Pending` / `Initializing` / `Ready` / `Fetching` / `Error`. |
| `observedGeneration` | Last spec generation the controller has processed. |
| `artifact` | Synced revision, content path, version (SHA256-prefixed), checksum, size. |
| `skillCount` | Number of skills retained after the filter. |
| `conditions[type=SourceAvailable]` | True when the upstream is reachable and the latest fetch succeeded. |
| `conditions[type=ContentValid]` | True when every retained `SKILL.md` parses cleanly and there are no duplicate names within the source. |
| `lastFetchTime` / `nextFetchTime` | Timestamps of the most recent and next scheduled fetch. |

## Resolution chain

1. **`SkillSource` syncs** content from the upstream into the workspace PVC at `<workspace-pvc>/<workspace>/<namespace>/<targetPath>/`.
2. **`PromptPack.spec.skills[]`** references the source by name and optionally narrows the set with `include` and renames the group with `mountAs`.
3. **PromptPack reconciler** writes a JSON manifest at `<workspace-pvc>/<workspace>/<namespace>/manifests/<pack>.json`.
4. **AgentRuntime reconciler** mounts the workspace content PVC into the runtime container (read-only) at `/workspace-content` and sets `OMNIA_PROMPTPACK_MANIFEST_PATH` on the container.
5. **Runtime container** reads the manifest at startup and calls `sdk.WithSkillsDir(...)` per resolved entry, plus `sdk.WithMaxActiveSkillsOption` from `spec.skillsConfig.maxActive`.
6. **PromptKit** drives skill activation per turn using the configured selector (model-driven by default).

## Cross-namespace policy

`SkillSource` is namespace-scoped. `PromptPack.spec.skills[].source` resolves only within the pack's own namespace. To share the same upstream content with multiple workspaces, create a `SkillSource` per workspace.

## Related

- [`PromptPack` CRD reference](/reference/promptpack/)
- [Use Skills how-to](/how-to/use-skills/)
- [PromptKit Skills concepts](https://promptkit.altairalabs.ai/concepts/skills/)
