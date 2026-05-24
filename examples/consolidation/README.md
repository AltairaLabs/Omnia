# Consolidation reference packs (examples)

These are *demonstrative* packs exercising the consolidation action
vocabulary beyond the helm-bundled `safe-default-summarizer`. They
are NOT auto-installed; apply with `kubectl` after deploying Omnia.

See
[the consolidation design spec](../../docs/local-backlog/2026-05-22-memory-consolidation-design.md)
for the full architecture.

## Apply

```bash
kubectl create namespace omnia-functions
kubectl apply -f demo-rescope/
kubectl apply -f demo-merge-entities/
```

Then wire them into a `MemoryPolicy`:

```yaml
spec:
  consolidation:
    schedule: "0 2 * * *"
    functionRefs:
      crossScopeCandidates:
        name: demo-rescope
        namespace: omnia-functions
      entityDuplicateCandidates:
        name: demo-merge-entities
        namespace: omnia-functions
```

The helm-bundled `safe-default-summarizer` (when enabled via
`consolidation.systemPacks.enabled=true`) already handles
`staleObservations`.

## What each pack does

| Pack | Axis | Actions emitted |
|---|---|---|
| `safe-default-summarizer` (helm) | `staleObservations` | `create_summary` + `supersede` |
| `demo-rescope` (here) | `crossScopeCandidates` | `rescope` to agent-scoped / user-scoped |
| `demo-merge-entities` (here) | `entityDuplicateCandidates` | `merge_entities` |

`rescope` to institutional (`(ws, null, null)`) is rejected by the
v1 validator outright — see the
[memory-poisoning-defenses spec](../../docs/local-backlog/2026-05-22-memory-poisoning-defenses.md)
for the proposal-queue flow that will land in a future release.
