# Omnia Service Architecture

This document maps every deployable service, how they communicate, and where to find their ownership docs. Read a service's `SERVICE.md` before adding code to understand what belongs there and what doesn't.

## Service Topology

```
                         ┌──────────────┐
                         │  Dashboard   │  Next.js (dashboard/)
                         │  port 3000   │  WS proxy on port 3002
                         └──────┬───────┘
                                │
              ┌─────────────────┼─────────────────┐
              │ HTTP            │ WebSocket        │ WebSocket
              ▼                 ▼                  ▼
       ┌──────────────┐  ┌───────────┐   ┌──────────────────┐
       │   Operator   │  │  Facade   │   │  Arena Dev       │
       │   cmd/       │  │  cmd/     │   │  Console (EE)    │
       │   main.go    │  │  agent/   │   │  ee/cmd/arena-   │
       │              │  │          │   │  dev-console/    │
       └──────┬───────┘  └─────┬─────┘   └────────┬─────────┘
              │                │ gRPC              │ HTTP
              │ K8s API        ▼                   ▼
              │          ┌───────────┐      ┌──────────────┐
              │          │  Runtime  │      │  Session API │
              │          │  cmd/     │      │  cmd/        │
              │          │  runtime/ │      │  session-api/│
              │          └─────┬─────┘      └──────┬───────┘
              │                │ HTTP               │
              │                └────────┬───────────┘
              │                         ▼
              │                  ┌──────────────┐
              │                  │  PostgreSQL  │
              │                  │  + Redis     │
              │                  └──────────────┘
              │
              │ manages
    ┌─────────┼──────────────────────────────┐
    │         │                              │
    ▼         ▼                              ▼
┌────────┐ ┌────────────────┐    ┌───────────────────┐
│Compact-│ │ Arena          │    │  Policy Proxy (EE)│
│ion     │ │ Controller (EE)│    │  ee/cmd/          │
│cmd/    │ │ ee/cmd/omnia-  │    │  policy-proxy/    │
│compact-│ │ arena-         │    └───────────────────┘
│ion/    │ │ controller/    │
└────────┘ └───────┬────────┘
                   │ creates
          ┌────────┼────────┐
          ▼        ▼        ▼
    ┌──────────┐ ┌──────┐ ┌──────────────┐
    │Eval      │ │Arena │ │PromptKit     │
    │Worker(EE)│ │Worker│ │LSP (EE)      │
    │ee/cmd/   │ │(EE)  │ │ee/cmd/       │
    │arena-    │ │      │ │promptkit-lsp/│
    │eval-     │ │      │ └──────────────┘
    │worker/   │ │      │
    └──────────┘ └──────┘
```

## Core Services

| Service | Path | SERVICE.md | Role |
|---------|------|------------|------|
| **Operator** | `cmd/main.go` | [cmd/SERVICE.md](cmd/SERVICE.md) | K8s controller-manager, dashboard host, REST API |
| **Facade** | `cmd/agent/` | [cmd/agent/SERVICE.md](cmd/agent/SERVICE.md) | WebSocket server, protocol translation to gRPC |
| **Runtime** | `cmd/runtime/` | [cmd/runtime/SERVICE.md](cmd/runtime/SERVICE.md) | LLM interaction via PromptKit SDK, tool execution |
| **Session API** | `cmd/session-api/` | [cmd/session-api/SERVICE.md](cmd/session-api/SERVICE.md) | Session CRUD, tiered storage (Redis/Postgres/cold) |
| **Memory API** | `cmd/memory-api/` | — | Cross-session memory CRUD, entity-relation-observation store (Postgres+pgvector) |
| **Compaction** | `cmd/compaction/` | [cmd/compaction/SERVICE.md](cmd/compaction/SERVICE.md) | Tiered storage compaction (hot→warm→cold) |
| **Dashboard** | `dashboard/` | [dashboard/SERVICE.md](dashboard/SERVICE.md) | Next.js UI, WebSocket proxy to facade/LSP/dev-console |

## Enterprise Services

| Service | Path | SERVICE.md | Role |
|---------|------|------------|------|
| **Arena Controller** | `ee/cmd/omnia-arena-controller/` | [ee/cmd/omnia-arena-controller/SERVICE.md](ee/cmd/omnia-arena-controller/SERVICE.md) | Reconciles Arena CRDs, manages eval job pods |
| **Arena Worker** | `ee/cmd/arena-worker/` | [ee/cmd/arena-worker/SERVICE.md](ee/cmd/arena-worker/SERVICE.md) | Executes arena eval work items (direct & fleet modes) |
| **Arena Eval Worker** | `ee/cmd/arena-eval-worker/` | [ee/cmd/arena-eval-worker/SERVICE.md](ee/cmd/arena-eval-worker/SERVICE.md) | Consumes session events, runs LLM judge evals |
| **Arena Dev Console** | `ee/cmd/arena-dev-console/` | [ee/cmd/arena-dev-console/SERVICE.md](ee/cmd/arena-dev-console/SERVICE.md) | Interactive WebSocket testing for Arena agents |
| **Policy Proxy** | `ee/cmd/policy-proxy/` | [ee/cmd/policy-proxy/SERVICE.md](ee/cmd/policy-proxy/SERVICE.md) | HTTP proxy enforcing AgentPolicy via CEL |
| **Privacy API** | `ee/cmd/privacy-api/` | [ee/cmd/privacy-api/SERVICE.md](ee/cmd/privacy-api/SERVICE.md) | Per-workspace owner of consent/opt-out, the central privacy/compliance audit hub (#1673), and the DSAR erasure lifecycle (#1676) |
| **PromptKit LSP** | `ee/cmd/promptkit-lsp/` | [ee/cmd/promptkit-lsp/SERVICE.md](ee/cmd/promptkit-lsp/SERVICE.md) | Language server for Arena agent definitions |

## Communication Protocols

| From | To | Protocol | Purpose |
|------|----|----------|---------|
| Dashboard | Facade | WebSocket | User chat messages, tool results; realtime reconnect includes `resume=<session_id>` query param for blip-resume |
| Dashboard | Operator | HTTP | CRUD for K8s resources |
| Dashboard | LSP | WebSocket | Code intelligence for Arena |
| Dashboard | Dev Console | WebSocket | Interactive agent testing |
| Facade | Runtime | gRPC (bidirectional) | LLM conversation stream; duplex audio transport (persistent `Converse` stream opened per audio session, carrying `AudioInputChunk` inbound and `MediaChunk` outbound) |
| Facade | Session API | HTTP | Session recording |
| Facade | Redis | Direct | Realtime session route table (`rt:route:<session_id>`→podIP, TTL=grace period) for reconnect routing in multi-replica deployments |
| Runtime | Session API | HTTP | Event recording |
| Memory API | Privacy API | HTTP | Audit drain-forwarder: `POST /api/v1/privacy/audit-events` ships local enforcement audit rows to the central audit hub, at-least-once (#1673) |
| Session API | Privacy API | HTTP | Audit drain-forwarder: `POST /api/v1/privacy/audit-events` ships local audit rows to the central audit hub, at-least-once (#1673) |
| Privacy API | Session API | HTTP | DSAR fan-out: `POST /api/v1/privacy/sessions/delete-by-user` erases each service-group's sessions + media for a subject (SA-authed, #1676) |
| Privacy API | Memory API | HTTP | DSAR fan-out: batch-delete erases each service-group's memories for a subject (scoped by workspace UID); consent-revocation prune `POST /api/v1/memories/consent-events` (SA-authed, #1676 / #1660) |
| Operator | K8s API | K8s client | CRD reconciliation |
| Arena Controller | K8s API | K8s client | Job/worker pod management |
| Arena Worker | Redis Streams | Redis | Work item consumption and result reporting |
| Arena Worker | K8s API | K8s client | CRD reads: Provider, AgentRuntime, ToolRegistry, ArenaJob (when `spec.providers` is set) |
| Arena Worker | LLM APIs | HTTPS | Direct mode: provider calls via PromptKit SDK |
| Arena Worker | Facade | WebSocket | Agent interaction via fleet providers (agentRef entries or legacy fleet mode) |
| Arena Eval Worker | Redis Streams | Redis | Event consumption |
| Arena Eval Worker | Session API | HTTP | Eval result storage |
| Compaction | PostgreSQL/Redis/Cold | Direct | Data lifecycle management |
| Runtime | Memory API | HTTP | Memory retrieval and extraction (when memory enabled) |
| Memory API | Redis Streams | Redis | Memory event publishing (create/delete) |
| Policy Proxy | K8s API | K8s client | Policy watching |

## Distributed Tracing

All services that handle conversation traffic share a single trace per session. The **Facade** derives the trace ID from the session UUID (lossless 128-bit mapping), so looking up a session ID in Tempo returns the full trace.

### Trace Flow

```
Browser ──WebSocket──▶ Facade ──gRPC──▶ Runtime ──HTTP──▶ Session API
                         │                 │                  │
                  omnia.facade.      omnia.runtime.     (inherits
                  message            conversation.turn   trace ctx)
                                         │
                                    genai.chat
                                         │
                                    omnia.tool.call
```

### Span Inventory

| Span Name | Created By | Parent | Key Attributes |
|-----------|-----------|--------|----------------|
| `omnia.facade.message` | Facade | (root, trace ID = session UUID) | session.id, omnia.agent.name, omnia.agent.namespace |
| `omnia.runtime.conversation.turn` | Runtime | facade.message (via gRPC context) | session.id, omnia.turn.index, omnia.promptpack.name/version |
| `genai.chat` | Runtime | conversation.turn | gen_ai.system, gen_ai.request.model, gen_ai.usage.* |
| `omnia.tool.call` | Runtime | genai.chat | tool.name, tool.duration_ms, tool.request/response.bytes |

### Tracing Responsibilities

- **Facade**: Creates root span, derives trace ID from session UUID, links to caller's W3C traceparent if present (e.g., arena-worker). Propagates context to Runtime (gRPC) and Session API (HTTP).
- **Runtime**: Creates conversation turn, LLM, and tool spans. Records token usage, cost, and tool execution metrics on spans.
- **Session API**: Inherits trace context from HTTP requests. Optional OTLP ingestion endpoint transforms traces into session-linked records for dashboard display.
- **Arena Worker**: Derives trace ID from job name. Spans: `arena.worker` (root), `arena.work-item` (per item), `arena.engine.execute`, `arena.fleet.session` (links to agent session trace).
- **Eval Worker**: Inherits trace context from session events when available.
- **Memory API**: Inherits trace context from HTTP requests. Records memory retrieval/extraction latency as spans.
- **Operator, Compaction, Policy Proxy, LSP**: No OTel spans.

### Metrics Inventory

| Metric Name | Source | Type | Purpose |
|-------------|--------|------|---------|
| `omnia_facade_realtime_sessions_parked_total` | Facade | Counter | Realtime sessions parked on unintentional WebSocket close (blip-resume) |
| `omnia_facade_realtime_reattach_total` | Facade | Counter | Successful realtime session reattaches via `resume=<session_id>` |
| `omnia_facade_realtime_park_expired_total` | Facade | Counter | Parked realtime sessions expired before reattach |

## Key Architectural Rules

1. **Server-side tool calls are opaque to the facade.** The runtime handles them internally; the facade only sees client-side tool calls.
2. **Session data flows one way.** Facade/Runtime → Session API → PostgreSQL. The dashboard reads via proxy routes through the operator.
3. **The dashboard never talks to the runtime directly.** All communication goes through the facade's WebSocket.
4. **WebSocket types are generated from Go.** Run `make generate-websocket-types` after changing `internal/facade/protocol.go`. The pre-commit hook enforces this.
5. **Generated files are never manually conflict-resolved.** After merging, re-run `make generate && make manifests && go mod tidy`.
6. **Observability has three read paths.** Prometheus is for **operational** signals (rates, latencies, system health, control-plane PromQL); session-api structured endpoints are for **product** data (eval results, cost, per-tenant usage); privacy-api is the owner of the **privacy/compliance audit** slice (enforcement events, consent changes, enforcement-stats — memory/session forward their audit rows to privacy-api's central hub, #1673). New product hooks must work when Prometheus is offline. See `CLAUDE.md` → "Observability Boundaries" for the classification rule; `hack/check-no-prom-product-deps` enforces it on new files.

## Adding a New Service

1. Create the entrypoint in `cmd/<name>/` (or `ee/cmd/<name>/` for enterprise)
2. Add a `SERVICE.md` documenting Owns/Inputs/Outputs/Does NOT Own/Dependencies
3. Add the service to this file's tables and topology diagram
4. Update the Tiltfile `docker_build` `only` lists if the service has its own image
5. Add boundary tests in `test/integration/` for any new protocol boundaries
