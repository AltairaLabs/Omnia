# omnia-demos

Self-contained demo features for Omnia, deployed into the isolated `omnia-demo`
namespace. Each demo is a `*.enabled` sub-toggle in `values.yaml`, off by
default, and wired into local dev through a matching `ENABLE_*` flag in the repo
`Tiltfile`. Enable one (or several) without touching the core install.

## Feature matrix

| Demo | values toggle | Tilt flag | Populates |
|------|---------------|-----------|-----------|
| Vision | `visionDemo.enabled` (default true) | `ENABLE_DEMO=true` | Vision chat agent (ollama llava) — needs the `vision` ollama instance (GPU / capable CPU pool). Set `visionDemo.enabled=false` on clusters without one, otherwise the Provider stays Unavailable. |
| Tools | `toolsDemo.enabled` (default true) | `ENABLE_DEMO=true` | Tool-calling chat agent (ollama llama3.2) |
| Audio | `audioDemo.enabled` | `ENABLE_AUDIO_DEMO=true` | Gemini audio agent (needs `gemini-credentials`) |
| LangChain | `langchainDemo.enabled` | `ENABLE_LANGCHAIN=true` | LangChain-runtime mirrors of vision/tools |
| Arena | `arenaDemo.enabled` | — | Arena project + load-test sources |
| SharePoint RAG hero | `sharepointHero.enabled` | — | Two-stage RAG over SharePoint docs — see [SHAREPOINT_HERO.md](./SHAREPOINT_HERO.md) |
| **Memory API** | `memoryDemo.enabled` | `ENABLE_MEMORY_DEMO=true` | Fills every memory UI surface at realistic scale (see below) |

`sharepointHero` and `memoryDemo` are **mutually exclusive** on the demo
Workspace's memory service (one hero demo owns it) — sharepointHero wins if both
are set.

## Workspace access

The demo `Workspace` ships with **no owner grant** by default
(`workspace.roleBindings: []`), so every dashboard route that requires a role
(the workspace services list, Memory Galaxy, …) returns 403 until access is
granted. Grant it declaratively at install time — no post-install
`kubectl patch` needed — via any of:

```bash
# OIDC group → owner
--set 'workspace.roleBindings[0].groups[0]=<oidc-group>' \
--set 'workspace.roleBindings[0].role=owner'

# individual user → owner (OIDC email claim)
--set 'workspace.directGrants[0].user=you@example.com' \
--set 'workspace.directGrants[0].role=owner'

# local dev only — anonymous owner (unauthenticated write access!)
--set workspace.anonymousAccess.enabled=true \
--set workspace.anonymousAccess.role=owner
```

Tilt's demo path already sets the anonymous-owner combo for local dev.

## Memory API demo

Populates the dashboard's memory surfaces (Memory Galaxy, analytics, workspace
knowledge, agent memory, privacy posture) for the `omnia-demo` workspace at
realistic scale, exercising every ingestion path. It wires:

- the demo Workspace's memory service to an **ollama `nomic-embed-text`**
  embedding provider + a demo `MemoryPolicy`, running memory-api with
  `ENTERPRISE_ENABLED` (so the privacy enforcement-stats surfaces light up);
- a **consolidation summarizer** (function-mode AgentRuntime, ollama
  `llama3.2:3b`) on the stale-observations axis, fired every minute;
- a post-install **seed Job** that POSTs a templated "Hawkridge Cloud" scenario
  across all tiers (institutional via chunk-ingest, agent, user) plus relations
  and back-dated observations for the consolidation worker to act on.

### Enable

```bash
# Local dev (Tilt) — also enables tools-demo (the seeder scopes agent-tier
# memories to it, and it provides the live remember/recall chat path):
ENABLE_MEMORY_DEMO=true tilt up

# Or any cluster with the demo chart:
helm upgrade --install omnia-demos charts/omnia-demos -n omnia-demo --create-namespace \
  --set memoryDemo.enabled=true --set toolsDemo.enabled=true
```

The **session** Secret (`omnia-postgres`) must already exist in `omnia-demo`
(same assumption as sharepointHero). The **memory** DB is dedicated: the dev
postgres creates an `omnia_memory_demo` database and the chart creates the
`omnia-postgres-memory-demo` Secret pointing at it (so the demo's 768-dim
`nomic-embed-text` embeddings don't collide with another workspace's embedding
column dimension). For non-dev clusters, set
`memoryDemo.database.createDevSecret=false` and pre-create that Secret.

The seed Job waits for both the demo memory-api **and** the `ollama-memory-embed`
model before seeding, so memories are embedded inline (the galaxy is semantic
immediately, not lexical-until-backfill).

### UI-population checklist (viewed against the `omnia-demo` workspace)

| Surface | Scope | Source |
|---------|-------|--------|
| Memory Galaxy | workspace | seeder (all tiers) + pre-render worker |
| Analytics — tier quad / category donut / growth / agent chart | workspace | seeder, all tiers |
| Analytics — Consolidation section | workspace | summarizer worker on back-dated observations |
| Analytics — Privacy posture / summary cards | workspace | seeded consent + EE enforcement stats |
| Workspace Knowledge | workspace | seeder institutional chunk-ingest |
| Agent detail → Memory tab | workspace + agent | seeder agent-tier (scoped to tools-demo) |
| My Memories | per-user | **sparse until you chat** — see caveats |

### Caveats (expected, not bugs)

- **My Memories is sparse until you chat.** It's scoped to your hashed user id;
  everything else is workspace-scoped and fills from the seeder immediately. Chat
  with the tools-demo agent and a new memory appears under your own user.
- **Consolidation + PII surfaces lag the seed.** The Consolidation section needs
  the summarizer worker to complete a pass (~1 min after the seed); the
  PII-blocked / redaction counters need `ENTERPRISE_ENABLED` memory-api (set by
  this demo) and a consolidation pass.
- **Re-running is safe for institutional ingest** (idempotent by stable id); the
  user/agent tiers are append-style, so a re-seed adds rows rather than
  replacing — clear `omnia_memory` first for a pristine re-seed.
