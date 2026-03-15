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
- **HTTP** to Session API: event writes (messages, tool calls, eval results)

## Does NOT Own
- WebSocket protocol (Facade's job)
- Client consent UI (Dashboard's job)
- Tool backend connections at cluster level (ToolRegistry/Operator configures these)
- CRD reconciliation (Operator's job)
- Session persistence (Session API's job)

## Observability

**Metrics** (Prometheus, prefix `omnia_llm_` and `omnia_runtime_`):
- LLM usage: `llm_input_tokens_total`, `llm_output_tokens_total`, `llm_cost_usd_total` (by provider, model)
- LLM requests: `llm_requests_total` (by status), `llm_request_duration_seconds`
- Cache: `llm_cache_hits_total` (prompt caching)
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
- Redis (optional, for conversation state)
- K8s API (optional, reads ToolRegistry CRD for metadata)
