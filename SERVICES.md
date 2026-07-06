# Omnia Service Architecture

This document maps every deployable service, how they communicate, and where to find their ownership docs. Read a service's `SERVICE.md` before adding code to understand what belongs there and what doesn't.

## Service Topology

### Control plane + agent pod

```
                         ┌──────────────┐
                         │  Dashboard   │  Next.js (dashboard/)
                         │  port 3000   │  WS proxy on port 3002
                         └──────┬───────┘
              ┌─────────────────┼──────────────────┐
              │ HTTP            │ WebSocket         │ WebSocket
              │                 │ (external + mgmt  │
              ▼                 ▼  twin listeners)  ▼
       ┌──────────────┐  ┌──────────────────┐  ┌──────────────────┐
       │   Operator   │  │    Agent Pod     │  │  Arena Dev       │
       │  cmd/main.go │  │ ┌──────────────┐ │  │  Console (EE)    │
       │  K8s ctrl +  │  │ │ Facade       │ │  │  ee/cmd/arena-   │
       │  REST + dash │  │ │ cmd/agent/   │ │  │  dev-console/    │
       └──────┬───────┘  │ └──────┬───────┘ │  └────────┬─────────┘
              │          │   gRPC │         │           │ HTTP
              │ manages  │ ┌──────┴───────┐ │           ▼
              │ + injects│ │ Runtime      │ │      ┌──────────────┐
              │  sidecar │ │ cmd/runtime/ │ │      │  Session API │
              │          │ └──────────────┘ │      │ cmd/         │
              │          │ ┌──────────────┐ │      │ session-api/ │
              │          │ │ Policy Proxy │ │      └──────────────┘
              │          │ │ (EE) sidecar │ │
              │          │ │ ee/cmd/      │ │
              │          │ │ policy-proxy/│ │
              │          │ └──────────────┘ │
              │          └──────────────────┘
              │ manages
    ┌─────────┼───────────────────────────────┐
    │         │                               │
    ▼         ▼                               ▼
┌────────┐ ┌────────────────┐    ┌───────────────────┐
│Compact-│ │ Arena          │    │  PromptKit LSP(EE)│
│ion     │ │ Controller (EE)│    │  ee/cmd/          │
│cmd/    │ │ ee/cmd/omnia-  │    │  promptkit-lsp/   │
│compact-│ │ arena-         │    └───────────────────┘
│ion/    │ │ controller/    │
└────────┘ └───────┬────────┘
                   │ creates
          ┌────────┴────────┐
          ▼                 ▼
    ┌──────────┐      ┌──────────┐
    │Eval      │      │Arena     │
    │Worker(EE)│      │Worker(EE)│
    │ee/cmd/   │      │ee/cmd/   │
    │arena-    │      │arena-    │
    │eval-     │      │worker/   │
    │worker/   │      │          │
    └──────────┘      └──────────┘
```

**Policy Proxy is an operator-INJECTED sidecar**, not a standalone managed
Deployment. When an AgentPolicy applies to an agent the operator adds the
policy-proxy container to that agent's pod (alongside facade + runtime); it
shares the pod lifecycle and is not reconciled as its own top-level service.

### Data plane (per workspace / service-group)

```
   Facade ──HTTP (session record)──┐   Runtime ──HTTP (memory)──┐
   Runtime ─HTTP (events)──────────┤                            │
                                   ▼                            ▼
                            ┌──────────────┐            ┌──────────────┐
                            │  Session API │            │  Memory API  │
                            │  cmd/        │            │  cmd/        │
                            │  session-api/│            │  memory-api/ │
                            └──────┬───────┘            └──────┬───────┘
                                   ▼                           ▼
                            ┌──────────────┐            ┌──────────────┐
                            │  PostgreSQL  │            │  PostgreSQL  │
                            │  + Redis     │            │  + pgvector  │
                            │ (warm cache) │            └──────────────┘
                            └──────────────┘
                                   │  audit drain (HTTP)       │
                                   └───────────┬───────────────┘
                                               ▼
                                  ┌────────────────────────────┐
                                  │      Privacy API (EE)      │
                                  │     ee/cmd/privacy-api/    │
                                  │  central audit hub + DSAR  │
                                  └────────────────────────────┘
```

Edges: `runtime → memory-api` (memory retrieval/extraction). `session-api →
privacy-api` and `memory-api → privacy-api` are the **audit drain** (each
service records enforcement rows locally, then a drain-forwarder ships them
at-least-once to privacy-api's central hub, #1673). The reverse direction —
`privacy-api → session-api` (`delete-by-user`) and `privacy-api → memory-api`
(batch-delete) — is the **DSAR erasure fan-out** privacy-api runs across every
service-group (#1676). Redis inside session-api is a warm cache, not a separate
store; memory-api's PostgreSQL carries the pgvector embedding columns.

### Diagnostics

```
Doctor (cmd/doctor/) ──probes──▶ facade internal mgmt twin port · session-api
· memory-api · operator · dashboard · arena-controller · (optional) Ollama,
Redis · K8s API (CRD presence + status.managementEndpoints)
```

Doctor is a diagnostic tool (CLI `--run-once` or an in-cluster HTTP dashboard),
not part of the request path. See [cmd/doctor/SERVICE.md](cmd/doctor/SERVICE.md).

### Facade composition + plane isolation

An `AgentRuntime` composes one or more single-protocol facade surfaces via
`spec.facades[]` — each entry is `type: websocket | a2a | rest | mcp`. A single
agent pod can therefore serve, say, a WebSocket surface plus an A2A surface plus
an MCP surface, each on its own port, all backed by the same runtime.

Each management-capable facade surface is served on **two listeners**: an
*external* port running the data-plane auth chain (`spec.externalAuth`
validators), and an *internal management-plane twin* port
(`facade-mgmt` 18080 / `a2a-mgmt` 19999 / `mcp-mgmt` 19998) that accepts only
dashboard-minted management-plane JWTs. Twin ports are ClusterIP-only, never on
an external Gateway/HTTPRoute, and fail closed without a valid mgmt JWT. The
twin is gated per-facade by `spec.facades[].managementPlane` (default true); the
enabled internal endpoints are advertised on
`AgentRuntime.status.managementEndpoints{ws,a2a,mcp}`. The dashboard WS proxy and
Doctor read that status to dial the management plane. See
[cmd/agent/SERVICE.md](cmd/agent/SERVICE.md) for the full listener contract.

## Core Services

| Service | Path | SERVICE.md | Role |
|---------|------|------------|------|
| **Operator** | `cmd/main.go` | [cmd/SERVICE.md](cmd/SERVICE.md) | K8s controller-manager, dashboard host, REST API |
| **Facade** | `cmd/agent/` | [cmd/agent/SERVICE.md](cmd/agent/SERVICE.md) | WebSocket server, protocol translation to gRPC |
| **Runtime** | `cmd/runtime/` | [cmd/runtime/SERVICE.md](cmd/runtime/SERVICE.md) | LLM interaction via PromptKit SDK, tool execution |
| **Session API** | `cmd/session-api/` | [cmd/session-api/SERVICE.md](cmd/session-api/SERVICE.md) | Session CRUD, tiered storage (Redis/Postgres/cold) |
| **Memory API** | `cmd/memory-api/` | [cmd/memory-api/SERVICE.md](cmd/memory-api/SERVICE.md) | Cross-session memory CRUD, entity-relation-observation store (Postgres+pgvector) |
| **Compaction** | `cmd/compaction/` | [cmd/compaction/SERVICE.md](cmd/compaction/SERVICE.md) | Tiered storage compaction (hot→warm→cold) |
| **Doctor** | `cmd/doctor/` | [cmd/doctor/SERVICE.md](cmd/doctor/SERVICE.md) | Diagnostic tool — probes every service's reachability + round-trips (CLI `--run-once` or in-cluster HTTP dashboard) |
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
| Facade | Session API | HTTP | Session recording — conversation messages are captured off the gRPC bus by a protocol-agnostic RuntimeClient interceptor (#1630), then written via the session HTTP client |
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
| Policy Proxy | Upstream (facade/runtime) | HTTP | Injected sidecar reverse-proxies requests after CEL policy enforcement + header injection |
| Policy Proxy | K8s API | K8s client | Policy watching |
| Dashboard | Facade (mgmt twin) | WebSocket | Management-plane chat/"Try this agent" — dials the internal `facade-mgmt` twin port from `status.managementEndpoints.ws` with a dashboard-minted mgmt-plane JWT (external ports reject it) |
| Doctor | Facade (mgmt twin) | WebSocket | Diagnostic round-trip on the internal mgmt twin port (falls back to external 8080 when no mgmt endpoint advertised); exchanges its SA token for a mgmt-plane JWT via the dashboard `/api/auth/service-token` endpoint |
| Doctor | Session API / Memory API | HTTP | Reachability + CRUD round-trip probes (create then delete a probe record) |
| Doctor | Operator / Dashboard / Arena Controller | HTTP | Reachability probes |
| Doctor | K8s API | K8s client | CRD presence checks, workspace-UID resolution, reading `status.managementEndpoints` |

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
- **Privacy API**: Configures the W3C trace-context propagator and an optional OTLP provider (service `omnia-privacy-api`) so audit-drain and DSAR fan-out calls join the caller's trace. Does not currently emit its own spans.
- **Operator, Compaction, Policy Proxy, LSP, Doctor**: No OTel spans.

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
