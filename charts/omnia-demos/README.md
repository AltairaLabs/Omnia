# omnia-demos

Self-contained demo features for Omnia, deployed into the isolated `omnia-demo`
namespace. Each demo is a `*.enabled` sub-toggle in `values.yaml`, off by
default, and wired into local dev through a matching `ENABLE_*` flag in the repo
`Tiltfile`. Enable one (or several) without touching the core install.

## Feature matrix

| Demo | values toggle | Tilt flag | Populates |
|------|---------------|-----------|-----------|
| Vision / Tools | (Tilt-managed agents) | `ENABLE_DEMO=true` | Vision + tool-calling chat agents (ollama llava / llama3.2) |
| Audio | `audioDemo.enabled` | `ENABLE_AUDIO_DEMO=true` | Gemini audio agent (needs `gemini-credentials`) |
| LangChain | `langchainDemo.enabled` | `ENABLE_LANGCHAIN=true` | LangChain-runtime mirrors of vision/tools |
| Arena | `arenaDemo.enabled` | — | Arena project + load-test sources |
| SharePoint RAG hero | `sharepointHero.enabled` | — | Two-stage RAG over SharePoint docs — see [SHAREPOINT_HERO.md](./SHAREPOINT_HERO.md) |
| **Memory API** | `memoryDemo.enabled` | `ENABLE_MEMORY_DEMO=true` | Fills every memory UI surface at realistic scale (see below) |

`sharepointHero` and `memoryDemo` are **mutually exclusive** on the demo
Workspace's memory service (one hero demo owns it) — sharepointHero wins if both
are set.

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

The seed Job requires pre-created Postgres Secrets in `omnia-demo`
(`omnia-postgres`, `omnia-postgres-memory`, each with a `POSTGRES_CONN` key) —
the same ones the dev install uses.

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
