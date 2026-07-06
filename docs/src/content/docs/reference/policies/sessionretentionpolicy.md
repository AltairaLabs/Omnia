---
title: "SessionRetentionPolicy CRD"
description: "Complete reference for the SessionRetentionPolicy custom resource"
sidebar:
  order: 13
---

`SessionRetentionPolicy` defines how long session history data is kept across the three storage tiers the session-api manages: the Redis **hot cache**, the Postgres **warm store**, and an optional **cold archive** (S3/GCS). A policy is a **reusable document** — a single policy can be referenced by many workspaces. The policy carries no binding information; workspaces opt in from their own spec.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionRetentionPolicy
```

## Resource Scope

`SessionRetentionPolicy` is **cluster-scoped**. A single policy object can be referenced by workspaces in any namespace.

## How Policies Are Bound

A policy is attached to a service group, not embedded in the policy itself. Each entry in `Workspace.spec.services[]` has an optional `session.policyRef` that selects a `SessionRetentionPolicy` by name:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: my-workspace
spec:
  services:
    - name: default
      session:
        database:
          # ... database config ...
        policyRef:
          name: standard-retention
```

The same `SessionRetentionPolicy` may be referenced by many service groups across many workspaces. When `session.policyRef` is unset, the session-api falls back to its **baked-in defaults** — no policy is applied.

## Spec Fields

All three tier blocks are optional. An omitted tier block leaves that tier at its default behaviour.

### `hotCache`

Configuration for the Redis hot cache tier.

| Field | Type | Default | Notes |
|---|---|---|---|
| `hotCache.enabled` | bool | `true` | Whether the hot cache is active. |
| `hotCache.ttlAfterInactive` | string | `24h` | Go duration after which inactive sessions are evicted from the hot cache. Must match `^([0-9]+h)?([0-9]+m)?([0-9]+s)?$` (e.g. `24h`, `30m`, `1h30m`). |
| `hotCache.maxSessions` | int32 | — | Maximum number of sessions to keep in the hot cache. Minimum `1`. |
| `hotCache.maxMessagesPerSession` | int32 | — | Maximum number of messages per session in the hot cache. Minimum `1`. |

### `warmStore`

Configuration for the Postgres warm store tier.

| Field | Type | Default | Notes |
|---|---|---|---|
| `warmStore.retentionDays` | int32 | `7` | Days to retain data in the warm store. Range `1`–`3650`. |
| `warmStore.partitionBy` | string | `week` | Partitioning strategy for warm store tables. Only `week` is currently supported. |

### `coldArchive`

Configuration for the cold archive tier (e.g. S3, GCS).

| Field | Type | Default | Notes |
|---|---|---|---|
| `coldArchive.enabled` | bool | `false` | Whether cold archival is active. |
| `coldArchive.retentionDays` | int32 | — | Days to retain data in the cold archive. Range `1`–`36500`. **Required when `enabled` is `true`** (enforced by CEL validation). |
| `coldArchive.compactionSchedule` | string | `0 2 * * *` | Cron expression for when to run compaction/archival. |

## Status Fields

| Field | Type | Description |
|---|---|---|
| `status.phase` | string | `Active` (valid and applied) or `Error` (configuration error). |
| `status.observedGeneration` | int64 | Most recent generation observed by the controller. |
| `status.workspaceCount` | int32 | Number of workspaces with per-workspace overrides that were resolved. |
| `status.conditions` | Condition[] | Standard Kubernetes conditions. |

## Print Columns

`kubectl get sessionretentionpolicy` shows:

| Column | Source |
|---|---|
| `Phase` | `.status.phase` |
| `Hot Cache TTL` | `.spec.hotCache.ttlAfterInactive` |
| `Warm Days` | `.spec.warmStore.retentionDays` |
| `Cold Archive` | `.spec.coldArchive.enabled` |
| `Age` | `.metadata.creationTimestamp` |

## Examples

### Minimal policy — warm store retention only

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionRetentionPolicy
metadata:
  name: short-retention
spec:
  warmStore:
    retentionDays: 14
```

### Full policy — all three tiers

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SessionRetentionPolicy
metadata:
  name: standard-retention
spec:
  hotCache:
    enabled: true
    ttlAfterInactive: 24h
    maxSessions: 10000
    maxMessagesPerSession: 500
  warmStore:
    retentionDays: 90
    partitionBy: week
  coldArchive:
    enabled: true
    retentionDays: 365
    compactionSchedule: "0 2 * * *"
```

## Related Resources

- [Workspace CRD](/reference/core/workspace/) — `spec.services[].session.policyRef`
- [SessionPrivacyPolicy CRD](/reference/policies/sessionprivacypolicy/) — privacy-specific retention overrides that layer on top of a `SessionRetentionPolicy`
- [Configure Sessions](/how-to/agents/configure-sessions/) — session service setup
