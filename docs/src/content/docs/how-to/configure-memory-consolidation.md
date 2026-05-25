---
title: Configure memory consolidation
description: Enable LLM-driven consolidation of agentic memory.
---

Memory consolidation runs scheduled passes over stale, cross-scope,
and duplicate-entity memory rows, dispatching each axis to a
function-mode AgentRuntime that emits a typed action list. The
platform validates each action (mutability, PII, k-anonymity, scope)
and applies accepted actions transactionally with full lineage.

This page covers operator-facing configuration.

## Enable the safe-default pack

The helm chart bundles a safe-default consolidation pack. Enable it
via values:

```yaml
consolidation:
  systemPacks:
    enabled: true
```

This installs:

- An `omnia-system-packs` Namespace
- A `safe-default-summarizer` `PromptPack` + `AgentRuntime` in
  function mode

Both carry the `omnia.altairalabs.ai/pack-class: system` label.
The dashboard renders a "System" badge on AgentRuntime detail pages
for these resources.

The pack handles only the `staleObservations` axis: emits
`create_summary` + `supersede` actions for groups of stale
observations sharing entity kind+name.

## Configure the worker

The consolidation worker runs inside the per-workspace memory-api pod.
Enable it from the helm chart:

```yaml
workspaceServices:
  memoryApi:
    consolidation:
      interval: "6h"     # empty = worker disabled (default)
```

The operator forwards the value to memory-api as
`--consolidation-interval`. When the value is empty (the default), the
worker is disabled — installations don't pick up consolidation
behaviour until an operator opts in.

The worker resolves each `functionRef` to a Service URL via:

```
http://{ref.name}.{ref.namespace}.svc.cluster.local:8080/functions/{ref.name}
```

so packs can live in any namespace the worker has network reach to. No
global "functions URL" — the legacy `CONSOLIDATION_FUNCTIONS_URL` env
var was retired (it couldn't span namespaces).

> **Per-policy scheduling is not yet honoured.** A `MemoryPolicy`'s
> `spec.consolidation.schedule` is parsed but ignored — the global
> `--consolidation-interval` drives every policy at the same cadence.
> Per-policy cron scheduling is tracked as a follow-up.

## Wire a `MemoryPolicy`

Reference the pack from a `MemoryPolicy`:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: MemoryPolicy
metadata:
  name: research
spec:
  tiers:
    user:          { mode: "decay" }
    agent:         { mode: "decay" }
    institutional: { mode: "retain" }
  consolidation:
    # schedule: "0 2 * * *"   # not yet honoured — see worker config above
    functionRefs:
      staleObservations:
        name: safe-default-summarizer
        namespace: omnia-system-packs
```

The worker iterates configured policies, runs the per-axis SQL
pre-filter, dispatches each non-empty axis to its `functionRef`,
validates returned actions, and applies them transactionally.

## Add cross-scope rescope / entity merge

Apply the demonstrative packs in `examples/consolidation/`:

```bash
kubectl create namespace omnia-functions
kubectl apply -f examples/consolidation/demo-rescope/
kubectl apply -f examples/consolidation/demo-merge-entities/
```

Then wire them as additional axes:

```yaml
spec:
  consolidation:
    functionRefs:
      staleObservations:
        name: safe-default-summarizer
        namespace: omnia-system-packs
      crossScopeCandidates:
        name: demo-rescope
        namespace: omnia-functions
      entityDuplicateCandidates:
        name: demo-merge-entities
        namespace: omnia-functions
```

## Safety gates

The action validator enforces (defaults shown):

| Gate | Default | Description |
|---|---|---|
| `minDistinctUserCount.agentScoped` | 5 | k-anonymity for rescope → `(ws, ag, null)` |
| `minDistinctUserCount.userScoped` | 1 | k-anonymity for rescope → `(ws, null, u)` |
| Institutional rescope | rejected in v1 | `rescope → (ws, null, null)` blocked outright |
| `requirePIIRedaction` | true | Re-run PII redactor on action content before persist |

Override per-policy:

```yaml
spec:
  consolidation:
    safetyGates:
      minDistinctUserCount:
        agentScoped: 10     # tighter than default
        userScoped: 1
      requirePIIRedaction: true
```

## Observability

Prometheus metrics under `omnia_memory_consolidation_*`:

- `passes_total{workspace, function, status}` — counter. Status:
  `ok` / `empty` / `prefilter_error` / `function_error` /
  `apply_error`.
- `pass_duration_seconds{workspace, function}` — histogram.
- `actions_total{workspace, function, action, outcome, target_tier}`
  — counter. Outcome: `applied` / `rejected_<reason>`.
- `function_call_duration_seconds{workspace, function}` —
  histogram.
- `omnia_memory_worker_running{worker="consolidation"}` — liveness
  gauge (1 when running).

The dashboard `/memory-analytics` page renders an aggregate
"Consolidation" card showing passes + action counts by type for
the selected time window.

## Limits in v1

- **No promotion to the institutional tier.** `rescope` to
  `(ws, null, null)` is rejected outright. The proposal-queue +
  quarantine flow that enables LLM-proposed institutional promotion
  is designed but not yet built (separate workstream).
- **Per-action drill-down is aggregate-only in the dashboard.**
  User-tier action content visibility (admin vs user view) is
  designed separately in a follow-up.
- **No regulated-document ingestion.** Bulk-loading authoritative
  content into memory has its own design (forthcoming
  `memory-ingestion-design.md`). The consolidation worker honours
  `source_type: regulated` rows as immutable but does not produce
  them.
- **Per-policy cron `schedule` is parsed but not honoured.** The
  worker ticks on the operator-set global interval; each policy
  fires every tick. Per-policy cron is a follow-up.
