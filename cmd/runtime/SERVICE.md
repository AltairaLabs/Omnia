# Runtime Service

## Owns
- PromptKit SDK conversation lifecycle
- LLM provider interaction (Claude, OpenAI, Gemini, Ollama)
- Tool registration, execution routing, and result handling
- Client tool suspension and resumption (sends tool_call, waits for result)
- Server-side tool execution (opaque to Facade and Dashboard)
- Eval execution pipeline
- Conversation state management (memory or Redis)
- Event recording via event store to Session API
- Function-mode (`spec.mode: function`) one-shot invocations: binds validated input JSON to PromptPack template variables and, per `spec.outputFormat`, constrains the provider's output (`text` = no constraint, `json` = JSON mode, `json_schema` = structured output bound to `spec.outputSchema`; default `json_schema`). Provider format errors propagate (fail-fast); the Facade's output-schema 502 remains the post-hoc backstop.

## Inputs
- **gRPC** from Facade (bidirectional Converse stream):
  - ClientMessage with user content (text, multimodal parts)
  - ClientToolResult with tool execution results
  - `DuplexStart` (first message of a duplex audio session) — switches the stream into `handleDuplexSession` mode, opening an `sdk.OpenDuplex` conversation. Fields: `codec`, `sample_rate`, `channels`, `system_instruction` (optional).
  - `AudioInputChunk` — subsequent audio frames forwarded via `pumpDuplexInput` → `conv.SendChunk`. `is_last` on a chunk signals stream end; the pipeline drains and the session closes.
  - **gRPC** `Invoke` (function mode) — one-shot `InvocationRequest` with `input_json` (already validated by the Facade against `spec.inputSchema`).
- **gRPC** `HasConversation` — the Facade asks whether a session's working context can still be resumed before continuing a conversation the client named. The runtime owns the context store, so it is the only component that can answer: a session-api row proves a conversation once existed, not that its turns survive. Answers `RESUMABLE` / `NOT_FOUND` / `UNAVAILABLE`, where `UNAVAILABLE` means the store could not be consulted and is explicitly not an expiry. Probes through `MessageReader.MessageCount` so the check cannot extend the lifetime of what it measures (see PromptKit#1649).
- **AgentRuntime CRD** (read directly via the k8s client at startup): `spec.mode`, `spec.outputFormat`, and `spec.outputSchema` (used to constrain function-mode output), `spec.duplex.audio` (the required realtime audio format advertised as the `RuntimeHello` counter-offer), alongside the PromptPack, provider, tools, and eval config.

## Outputs
- **gRPC** to Facade (bidirectional Converse stream):
  - `RuntimeHello` — the **first** ServerMessage on every stream: the session's authoritative `capabilities` and, for a duplex session, a bounded `MediaNegotiation` counter-offer (`codec`/`sample_rate`/`channels`; `frame_rate`/`resolution` carried, not yet enforced) derived from `spec.duplex.audio`. Absence marks a legacy runtime. Contract 1.3.0.
  - Chunk — streaming LLM text
  - Done — response complete with final content
  - ToolCall — client-side tool call (execution=CLIENT only; server-side never sent)
  - Error — error response
  - MediaChunk — streaming audio/video
- **HTTP** to Session API:
  - Messages (user/assistant conversation only)
  - Tool calls (first-class records with args, result, duration)
  - Provider calls (first-class records with tokens, cost, duration)
  - Runtime events (pipeline, stage, middleware, validation lifecycle)
  - Eval results (inline eval scores with explanation, source="runtime-inline"; worker-written rows use source="worker")
  - Session stats (token counts, message counts)

## Context store configuration

`spec.context` on the AgentRuntime CRD controls how the runtime persists
conversation state across turns.

| Field | Value | Effect |
|-------|-------|--------|
| `spec.context.type` | `memory` (default) | In-process store; the runtime context (working LLM context) is ephemeral and lost when the pod restarts |
| `spec.context.type` | `redis` | Durable, fast store; the runtime context survives pod restarts and is resumable cross-pod via `sdk.Resume` |
| `spec.context.storeRef` | `name: <secret-name>` | Required when `type` is `redis`. References a Kubernetes Secret in the same namespace. The secret **must** contain a `url` key holding the connection URL (e.g. `redis://…`). |
| `spec.context.ttl` | duration string (default `"24h"`) | How long conversation state is retained in the store. Applied at store construction (`statestore.WithTTL` / `WithMemoryTTL`). **Note:** the two backends currently measure this differently — the memory store treats it as idle time and refreshes it on read, while Redis runs a fixed window from the last write (PromptKit#1649). |

Only fast/instant stores back the **runtime context** (the working LLM context
concatenated into each provider call) — `memory` or `redis`. This is distinct
from **session-api**, which owns long-term archival of the full conversation in
its own database; the two are separate stores with separate purposes and the
runtime cannot rebuild its state from session-api.

CEL validation on the CRD enforces that `storeRef` is present whenever
`type` is `redis`:

```
spec.context.storeRef is required when context.type is 'redis'
```

### How the operator projects context config

When `type: redis` and `storeRef` is set, the operator injects
`OMNIA_CONTEXT_URL` into the runtime container sourced from the secret's `url`
key. The runtime reads this at startup to create a `statestore.RedisStore`
(PromptKit SDK). When the env var is absent the runtime falls back to the
in-process memory store.

```yaml
# Example AgentRuntime with durable Redis context store
spec:
  context:
    type: redis
    storeRef:
      name: my-agent-redis   # Secret must have a 'url' key
    ttl: "48h"
```

```yaml
# Corresponding Secret
apiVersion: v1
kind: Secret
metadata:
  name: my-agent-redis
  namespace: <agent-namespace>
stringData:
  url: "redis://:password@redis.example.com:6379/0"
```

### Text resume via sdk.Resume

When a Redis store is configured, each conversation turn is persisted before
the response streams back. On the next connection (new pod, pod restart,
reconnect after eviction) the facade sends the existing `sessionID` to the
runtime's gRPC `Converse` stream. The runtime calls `sdk.Resume(sessionID, …)`
which replays the stored conversation history from Redis and continues the
exchange — no client-visible interruption for text sessions.

### Duplex (voice) durable resume

Durable resume for duplex audio sessions (those opened via `DuplexStart`) is a
**later T3 step** gated on PromptKit issue #1459. This foundation ships durable
state for text conversations only. Voice sessions continue to use the
in-process store regardless of `spec.context.type`.

## Does NOT Own
- WebSocket protocol (Facade's job)
- Client consent UI (Dashboard's job)
- Tool backend connections at cluster level (ToolRegistry/Operator configures these). HTTP/OpenAPI tool auth is the exception: the operator resolves each handler's `authSecretRef` into the `<agentruntime>-tool-secrets` Secret and mounts it read-only; the runtime reads the token from the mounted file and applies the `Authorization` header at call time — it never receives the credential inline from the ConfigMap.
- CRD reconciliation (Operator's job)
- Session persistence (Session API's job)

## Observability

**Metrics** (Prometheus, prefix `omnia_provider_` and `omnia_runtime_`):
- Served on the runtime **health port (9001)** at `/metrics` — NOT the gRPC port
  (9000). The container declares this port with the name `metrics` (same
  contract as the facade), so a single name-keyed scrape job/PodMonitor reaches
  both containers' metrics. See `cmd/agent/SERVICE.md` and #1488.
- LLM usage: `provider_input_tokens_total`, `provider_output_tokens_total`, `provider_cost_total` (by provider, model)
- LLM requests: `provider_requests_total` (by status), `provider_request_duration_seconds`
- Runtime info: `runtime_info` gauge with agent/namespace labels
- PromptKit SDK metrics + omnia runtime metrics are merged onto this one endpoint
  via `prometheus.Gatherers` (intra-container only — there is no cross-container
  consolidation with the facade)

**Traces** (OpenTelemetry):
- `omnia.runtime.conversation.turn` — wraps each conversation turn (session.id, turn.index, promptpack)
- `genai.chat` — LLM call span following GenAI semantic conventions (tokens, cost, model, finish reason)
- `omnia.tool.call` — tool execution span (tool.name, duration, request/response size)
- gRPC server instrumented with `otelgrpc` for incoming requests from Facade

## Dependencies
- PromptKit SDK (local via `go.work`, published for CI)
- LLM provider endpoints (configured via environment or CRD)
- Session API HTTP endpoint (optional, for event recording)
- Memory API HTTP endpoint (optional, for cross-session memory retrieval)
- Redis (optional, for durable conversation state; required when `spec.context.type: redis`)
- K8s API (optional, reads ToolRegistry CRD for metadata)

### Environment variables (injected by operator)

| Variable | Source | Purpose |
|----------|--------|---------|
| `OMNIA_CONTEXT_URL` | `spec.context.storeRef` secret → `url` key | Redis connection URL for the durable context store. Absent when `spec.context.type: memory` (default). |

## Memory retrieval

When `spec.memory.enabled: true` on the AgentRuntime CRD, the runtime wires
a `CompositeRetriever` before each conversation turn. It reads from the
memory-api HTTP client and injects memories via PromptKit's `WithMemory` option.

The retriever honours three CRD fields from `spec.memory.retrieval`:

| Field | Effect |
|-------|--------|
| `strategy` | `"semantic"` → memory-api hybrid search path; `"composite"` → keyword + semantic fused via RRF; otherwise (`"keyword"`/empty) keyword FTS |
| `limit` | Max episodic memories injected per turn (default 10 when absent or 0) |
| `accessFilter.denyCEL` | CEL expression over a memory item's `metadata`; items that match are dropped. Enforced on **every** path (keyword, semantic, composite) |

The retriever runs two passes per turn:
1. **Profile pull** — always-on; fetches `memory:identity`, `memory:preferences`,
   and `memory:health` categories regardless of query content (profile is cached
   per (workspace, user) for 30 s to avoid per-turn list calls).
2. **Episodic search** — per-turn; uses semantic hybrid, RRF-fused composite, or
   keyword FTS based on `strategy`; limited by `limit`. The `accessFilter.denyCEL`
   deny-filter is applied on all paths: the semantic leg filters server-side in
   memory-api, while the keyword leg post-filters in the runtime (over-fetching
   past restricted items to still return up to `limit`). When `strategy:
   composite` but the store has no semantic capability, retrieval degrades to
   keyword-only. Profile-category results returned by the search are deduplicated
   against the profile pull.
