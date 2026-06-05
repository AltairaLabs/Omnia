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

## Inputs
- **gRPC** from Facade (bidirectional Converse stream):
  - ClientMessage with user content (text, multimodal parts)
  - ClientToolResult with tool execution results

## Outputs
- **gRPC** to Facade (bidirectional Converse stream):
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

## Does NOT Own
- WebSocket protocol (Facade's job)
- Client consent UI (Dashboard's job)
- Tool backend connections at cluster level (ToolRegistry/Operator configures these)
- CRD reconciliation (Operator's job)
- Session persistence (Session API's job)

## Observability

**Metrics** (Prometheus, prefix `omnia_provider_` and `omnia_runtime_`):
- LLM usage: `provider_input_tokens_total`, `provider_output_tokens_total`, `provider_cost_total` (by provider, model)
- LLM requests: `provider_requests_total` (by status), `provider_request_duration_seconds`
- Runtime info: `runtime_info` gauge with agent/namespace labels
- PromptKit SDK metrics exported via isolated registry on `/metrics`

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
- Redis (optional, for conversation state)
- K8s API (optional, reads ToolRegistry CRD for metadata)

## Memory retrieval

When `spec.memory.enabled: true` on the AgentRuntime CRD, the runtime wires
a `CompositeRetriever` before each conversation turn. It reads from the
memory-api HTTP client and injects memories via PromptKit's `WithMemory` option.

The retriever honours three CRD fields from `spec.memory.retrieval`:

| Field | Effect |
|-------|--------|
| `strategy` | `"semantic"` → memory-api hybrid search path; otherwise keyword FTS |
| `limit` | Max episodic memories injected per turn (default 10 when absent or 0) |
| `accessFilter.denyCEL` | CEL expression over a memory item's `metadata`; items that match are dropped (semantic path only) |

The retriever runs two passes per turn:
1. **Profile pull** — always-on; fetches `memory:identity`, `memory:preferences`,
   and `memory:health` categories regardless of query content (profile is cached
   per (workspace, user) for 30 s to avoid per-turn list calls).
2. **Episodic search** — per-turn; uses semantic hybrid or keyword FTS based on
   `strategy`; limited by `limit`; deny-filtered when `accessFilter.denyCEL`
   is set and `strategy` is `"semantic"`. Profile-category results returned by
   the search are deduplicated against the profile pull.
