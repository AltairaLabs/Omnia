---
title: "MemoryPolicy CRD"
description: "Complete reference for the MemoryPolicy custom resource"
sidebar:
  order: 13
---

`MemoryPolicy` defines how the agentic memory store behaves across the
**institutional**, **agent**, and **user** tiers: how memories are retained and
pruned, how the read path ranks and decays results, how the write path
deduplicates, and how background workers consolidate and pre-render memories.
Policies are **reusable documents** — a single policy can be referenced by many
workspaces, and a workspace with no reference falls back to the baked-in legacy
interval policy.

The core CRD (tiers, retention, recall, dedup, ingestion) is available in the
open-source edition. Several features — cross-tier ranking, the institutional
tier, LLM-driven consolidation, and the Memory Galaxy projection worker — are
**Enterprise-gated**. Those sections are marked below.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: MemoryPolicy
```

## Resource Scope

`MemoryPolicy` is **cluster-scoped**. There is no `metadata.namespace`.

## How Policies Are Bound

Workspaces opt in to a `MemoryPolicy` via
`Workspace.spec.services[].memory.policyRef`:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: my-workspace
spec:
  services:
    - name: default
      memory:
        policyRef:
          name: my-memory-policy
```

Many workspaces may reference one policy. A service group with no `policyRef`
falls back to the platform's legacy interval-based retention policy.

## Implementation Status

The CRD and its controller validation shipped in Phase 1. Behaviour is being
wired in stages, so some fields validate today but do not yet drive the worker:

- **Retention modes** (`spec.tiers.*.mode`) — the shape is validated; the
  composite retention worker that applies `TTL` / `Decay` / `LRU` / `Composite`
  is a later phase. Until then the worker runs the legacy TTL-only logic.
- **Consent revocation** (`spec.consentRevocation`) — validated; the event
  subscription is a later phase.
- **Supersession** (`spec.supersession`) — validated; the summarizer that
  produces superseding summaries is gated behind `enabled: false` by default.
- **`spec.consolidation.schedule`** — parsed but not yet honoured per-policy;
  the operator-set global consolidation interval currently drives every policy
  at the same cadence. See
  [Configure memory consolidation](/how-to/memory/configure-memory-consolidation/).

Check `status.phase` and `status.conditions` to see whether a policy validated.

## Spec Fields

### `tiers` (required)

Per-tier retention configuration. The `tiers` object groups the three memory
tiers so operators can express tier-specific defaults. At least one tier needs a
`mode` for the policy to do anything useful.

```yaml
spec:
  tiers:
    institutional: { mode: Manual }
    agent:         { mode: Composite }
    user:          { mode: Composite }
```

:::note[Enterprise Feature]
The **institutional** tier is an enterprise feature (`ee/pkg/memory`). In the
open-source edition, memories live in the **user**, **agent**, and
**user-for-agent** tiers only. See
[Installing a License](/how-to/operations/install-license/).
:::

Each tier (`institutional`, `agent`, `user`) is a tier config with these fields:

| Field | Type | Default | Description |
|---|---|---|---|
| `mode` | enum | `Manual` | Retention strategy. One of `Manual`, `TTL`, `Decay`, `LRU`, `Composite`. |
| `softDeleteGraceDays` | int32 | `30` | Window between soft-delete (`forgotten=true`) and hard-delete. Applies to every mode. Range 0–3650. |
| `ttl` | object | — | TTL-branch config (see below). |
| `decay` | object | — | Decay-branch config (see below). |
| `lru` | object | — | LRU-branch config (see below). |
| `perCategory` | map | — | Per-consent-category overrides (see below). |

**Retention modes:**

| Mode | Behaviour |
|---|---|
| `Manual` | Leave rows alone. Only explicit `memory__forget`, a user "forget everything", or a DSAR delete removes the row. The expected default for the institutional tier. |
| `TTL` | Prune rows when `expires_at < now()`. |
| `Decay` | Compute a per-row score from the decay formula and soft-delete when it drops below `minScore`. Uses the same formula as retrieval ranking. |
| `LRU` | Prune rows whose `accessed_at` is older than `staleAfter`. |
| `Composite` | Apply `TTL`, `Decay`, and `LRU` independently; the first branch to fire wins. Expected default for the agent and user tiers. |

Missing branch sub-objects (`ttl` / `decay` / `lru`) mean "that branch is off for
this tier".

#### `tiers.<tier>.ttl`

| Field | Type | Default | Description |
|---|---|---|---|
| `default` | duration | — | Applied at write time when the caller sets no explicit `expires_at`. Empty means rows without an explicit `expires_at` never expire via the TTL branch. |
| `maxAge` | duration | — | Caps any explicit `expires_at` so a client can't pin a row longer than the operator accepts. Empty means no cap. |

Durations are Go-duration-style strings matching `^([0-9]+d)?([0-9]+h)?([0-9]+m)?([0-9]+s)?$`, e.g. `180d`, `720h`.

#### `tiers.<tier>.decay`

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Whether the decay branch is active. |
| `minScore` | string (0–1) | `"0.2"` | Score below which a row is prunable. |
| `scoreFormula` | object | — | Overrides the default weighting (see below). |
| `halfLifeDays` | int32 | `90` | How aggressively recency pulls the retention score down. Shorter = personal memories decay faster. Range 1–3650. |

`scoreFormula` weights are decimal strings; the controller enforces the 0–1
range and surfaces bad values as a status condition:

| Field | Default |
|---|---|
| `confidenceWeight` | `"0.5"` |
| `accessFrequencyWeight` | `"0.3"` |
| `recencyWeight` | `"0.2"` |

The weights should sum to roughly 1.0, but the controller does not enforce this
strictly — operators sometimes want to emphasise one signal.

:::note
`decay.halfLifeDays` governs **retention pruning** and is distinct from
`spec.recall.halfLife`, which governs **read-path recency decay**. They are two
different half-lives with different defaults (90 days vs 30 days).
:::

#### `tiers.<tier>.lru`

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Whether the LRU branch is active. |
| `staleAfter` | duration | `"120d"` | `accessed_at` age beyond which a row is prunable. Requires read-path `accessed_at` updates; without them every row looks stale forever. |

#### `tiers.<tier>.perCategory`

Overrides the tier policy for specific consent categories (for example
`memory:health`). Keys are consent-category strings; values are a nested tier
config with the same fields as above **except** `perCategory` (the schema does
not allow arbitrary recursion):

```yaml
spec:
  tiers:
    user:
      mode: Composite
      perCategory:
        memory:health:
          mode: Manual        # never auto-prune health memories
```

### `recall`

Tunes the read path. Optional — defaults baked into the recall SQL apply when
omitted.

| Field | Type | Default | Description |
|---|---|---|---|
| `halfLife` | object | 30d per tier | Per-tier recency-decay half-life (see below). |
| `inlineThresholdBytes` | int32 | `2048` | Body-size cutoff above which recall returns title + summary + `content_preview` instead of the full body. The agent calls `memory__open(id)` for the full text. Range 0–1048576. |
| `maxRelatedPerMemory` | int32 | `3` | Caps the per-memory `related[]` slice in the recall response. Range 0–50. |

`recall.halfLife` carries per-tier durations (Go-duration form, e.g. `720h` =
30 days). A memory whose age equals `halfLife` scores at 0.5× the recency
multiplier; at 5× `halfLife` it is effectively gone.

| Field | Type | Description |
|---|---|---|
| `halfLife.user` | duration | Half-life for user-tier recency decay. |
| `halfLife.agent` | duration | Half-life for agent-tier recency decay. |
| `halfLife.institutional` | duration | Half-life for institutional-tier recency decay. |

See [Memory](/explanation/agents/memory/) for how half-life folds into the fused
retrieval score.

### `dedup`

Tunes the write path's two dedup mechanisms. Optional.

| Field | Type | Description |
|---|---|---|
| `requireAboutForKinds` | string[] | Kinds (e.g. `fact`, `preference`) that must carry an `about={kind, key}` hint on save. Without it, the structured-key dedup path can't engage and identity-class memories pile up as duplicates. Empty disables the check. |
| `embeddingSimilarity` | object | Cosine-similarity dedup config (see below). Auto-disabled when no embedding provider is configured. |

#### `dedup.embeddingSimilarity`

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Whether cosine-similarity dedup runs. |
| `autoSupersedeAbove` | string (0–1) | `0.95` | Cosine ≥ this auto-supersedes the prior match. Lower is more aggressive. |
| `surfaceDuplicatesAbove` | string (0–1) | `0.85` | Cosine ≥ this surfaces the match as `potential_duplicates` for the agent to consider later. |
| `candidateLimit` | int32 | API default | Caps the candidates returned in `potential_duplicates`. Range 0–50. |

`surfaceDuplicatesAbove` must be **strictly less than** `autoSupersedeAbove`
(enforced by CEL at admission). Otherwise every surfaced match is already above
the auto-supersede floor and never reaches the agent.

### `tierPrecedence`

Applies per-tier ranking multipliers to the fused retrieval score. Unset tiers
default to `1.0` (no-op). Exactly one ranking strategy must be set — currently
only `multiplicative` — enforced by CEL at admission.

:::note[Enterprise Feature]
Cross-tier ranking (`ee/pkg/memory/tier_ranking.go`) is an enterprise feature.
See [Installing a License](/how-to/operations/install-license/).
:::

```yaml
spec:
  tierPrecedence:
    multiplicative:
      institutional: "2.0"
      agent: "1.5"
      user: "1.0"
```

`multiplicative` scales each memory's base score by a per-tier weight; ordering
within a tier is preserved, and the weights bias across tiers. Weights are
decimal strings, controller-enforced to 0 ≤ w ≤ 10 (default `1.0` per tier). The
`user_for_agent` tier inherits the `user` weight.

### `consentRevocation`

How the retention worker reacts when a user toggles off a consent category.

| Field | Type | Default | Description |
|---|---|---|---|
| `action` | enum | `SoftDelete` | One of `SoftDelete`, `HardDelete`, `Stop`. |
| `graceDays` | int32 | `7` | Soft-delete → hard-delete window for the `SoftDelete` action. Ignored for `HardDelete` and `Stop`. Range 0–365. |

| Action | Behaviour |
|---|---|
| `SoftDelete` | Mark matching rows `forgotten=true`, hard-delete after `graceDays`. The safe default. |
| `HardDelete` | Remove rows immediately in a transaction. |
| `Stop` | Leave existing rows alone; only block future writes in that category. Not GDPR-compliant in most jurisdictions — useful for development. |

### `supersession`

Cleanup of rows superseded by temporal-summarization summaries.

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Gated off until a summarizer agent is generating summaries in your deployment. |
| `graceDays` | int32 | `14` | Window to roll back a bad summary before the originals are hard-deleted. Range 0–365. |

### `schedule` and `batchSize`

| Field | Type | Default | Description |
|---|---|---|---|
| `schedule` | string (cron) | `"0 3 * * *"` | When the retention worker runs. Defaults to 03:00 daily so pruning happens off-peak. |
| `batchSize` | int32 | `1000` | Rows the worker processes per transaction. Higher = fewer commits but longer locks. Range 1–100000. |

### `consolidation`

Configures the LLM-driven consolidation worker that runs scheduled passes over
stale, cross-scope, and duplicate-entity memories, dispatching each axis to a
function-mode `AgentRuntime`.

:::note[Enterprise Feature]
Memory consolidation (`ee/pkg/memory/consolidation`) is an enterprise feature.
See [Installing a License](/how-to/operations/install-license/).
:::

| Field | Type | Default | Description |
|---|---|---|---|
| `schedule` | string (cron) | `"0 2 * * *"` | Policy-level schedule. **Parsed but not yet honoured per-policy** (see Implementation Status). |
| `schedules` | object | — | Per-axis cron overrides: `staleObservations`, `crossScopeCandidates`, `entityDuplicateCandidates`. An unset axis inherits `schedule`. |
| `functionRefs` | object | — | Maps each pre-filter axis to a function-mode `AgentRuntime` (`name`, optional `namespace`). Axes with no `functionRef` are skipped. |
| `candidateLimits` | object | — | `maxBucketsPerPass` (default 100), `maxPerBucket` (default 50) — bounds LLM cost per pass. |
| `safetyGates` | object | — | Action validator config (see below). |
| `timeouts` | object | — | `functionCall` (default 5m), `passWallClock` (default 30m). |

`safetyGates`:

| Field | Type | Default | Description |
|---|---|---|---|
| `minDistinctUserCount` | map | `agentScoped: 5`, `userScoped: 1` | k-anonymity thresholds keyed by target tier. |
| `maxScopeWidening` | string | `workspace` | Caps cross-workspace promotion. Only `workspace` is supported. |
| `requirePIIRedaction` | bool | `true` | Re-run the PII redactor on action content before persist. |

See [Configure memory consolidation](/how-to/memory/configure-memory-consolidation/)
for the operator-facing setup.

### `projection`

Configures the Memory Galaxy pre-render worker, which renders the workspace-wide
2D layout into `memory_projections` so the projection endpoint serves it
instantly.

:::note[Enterprise Feature]
The Memory Galaxy projection worker (`ee/pkg/memory/projection`) is an enterprise
feature. See [Installing a License](/how-to/operations/install-license/).
:::

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Turn on background pre-rendering of the galaxy layout. |
| `schedule` | string (cron) | — | Optional cadence bound (evaluated against the last render's `computed_at`). Omit to re-render whenever the change trigger fires. |
| `changeThreshold` | int32 | — | Re-render once at least this many entities have changed since the last render. 0 / omitted = re-render on any change. |

### `ingestion`

Configures how source documents become index items at `/institutional/ingest`.
Read live by memory-api at ingest time; the `--ingest-*` binary flags are the
fallback when a field is unset.

| Field | Type | Default | Description |
|---|---|---|---|
| `strategy` | enum | `chunk` | One of `chunk` (RAG-chunk the raw text), `summary` (one condensed summary per document), `summaryThenChunk` (summarize, then RAG-chunk the summary). |
| `summarizer` | enum | `extractive` | One of `extractive` (in-process lead-sentence, no LLM) or `agent` (async work-queue consumed by an external summarizer AgentRuntime). Ignored when `strategy: chunk`. |
| `chunk` | object | — | RAG chunk-splitter geometry: `size` (words, default 200) and `overlap` (words, default 40). `overlap` must be less than `size` (CEL-enforced). |

## Status Fields

| Field | Type | Description |
|---|---|---|
| `status.phase` | string | `Active` (validated, available to the retention worker) or `Error` (validation failed — see conditions). |
| `status.observedGeneration` | int64 | Spec generation last reconciled. |
| `status.workspaceCount` | int32 | Number of per-workspace overrides that resolved to an existing Workspace. |
| `status.conditions` | Condition[] | Individual validation / wiring checks. |

## Examples

### Minimal policy — TTL on the user tier

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: MemoryPolicy
metadata:
  name: basic
spec:
  tiers:
    user:
      mode: TTL
      ttl:
        default: "180d"
```

### Full policy

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: MemoryPolicy
metadata:
  name: research
spec:
  schedule: "0 3 * * *"
  batchSize: 2000
  tiers:
    institutional:
      mode: Manual
    agent:
      mode: Composite
      softDeleteGraceDays: 30
      ttl:
        default: "365d"
      decay:
        enabled: true
        minScore: "0.2"
        halfLifeDays: 120
      lru:
        enabled: true
        staleAfter: "180d"
    user:
      mode: Composite
      ttl:
        default: "180d"
        maxAge: "365d"
      decay:
        enabled: true
        minScore: "0.25"
        scoreFormula:
          confidenceWeight: "0.5"
          accessFrequencyWeight: "0.3"
          recencyWeight: "0.2"
        halfLifeDays: 60
      lru:
        enabled: true
        staleAfter: "120d"
      perCategory:
        memory:health:
          mode: Manual
  recall:
    halfLife:
      user: "720h"
      agent: "1440h"
      institutional: "8760h"
    inlineThresholdBytes: 2048
    maxRelatedPerMemory: 3
  dedup:
    requireAboutForKinds:
      - fact
      - preference
    embeddingSimilarity:
      enabled: true
      autoSupersedeAbove: "0.95"
      surfaceDuplicatesAbove: "0.85"
  tierPrecedence:                       # Enterprise
    multiplicative:
      institutional: "2.0"
      agent: "1.5"
      user: "1.0"
  consentRevocation:
    action: SoftDelete
    graceDays: 7
  consolidation:                        # Enterprise
    functionRefs:
      staleObservations:
        name: safe-default-summarizer
        namespace: omnia-system-packs
    safetyGates:
      minDistinctUserCount:
        agentScoped: 5
        userScoped: 1
      requirePIIRedaction: true
  projection:                           # Enterprise
    enabled: true
    changeThreshold: 50
  ingestion:
    strategy: summaryThenChunk
    summarizer: extractive
    chunk:
      size: 200
      overlap: 40
```

## Related Resources

- [Memory](/explanation/agents/memory/) — the multi-tier recall model, RRF fusion, and
  half-life decay explained.
- [Configure memory consolidation](/how-to/memory/configure-memory-consolidation/) —
  operator setup for the consolidation worker.
- [Change the memory embedding model](/how-to/memory/change-memory-embedding-model/) —
  swap the embedding provider and vector dimension.
- [Workspace CRD](/reference/core/workspace/) —
  `spec.services[].memory.policyRef`.
