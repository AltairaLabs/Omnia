# Facade Service

## Owns
- WebSocket server for browser/client connections
- Protocol translation: WebSocket JSON <-> gRPC bidirectional stream
- Connection lifecycle (upgrade, ping/pong, close, rate limiting)
- Session creation and routing
- Binary frame encoding/decoding for media
- Media upload URL negotiation (S3/GCS/Azure/local)
- Client-side tool result routing to active handler
- Session recording via HTTP client to Session API

## Inputs
- **WebSocket** from browser/dashboard:
  - `message` — user text or multimodal content
  - `tool_result` — client-side tool execution result
  - `upload_request` — file upload initiation
- **gRPC** from Runtime (response stream):
  - `chunk` — streaming text
  - `done` — response complete
  - `tool_call` — client-side tool call (server-side tool calls are filtered)
  - `error` — error response
  - `media_chunk` — streaming audio/video

## Outputs
- **WebSocket** to browser/dashboard: ServerMessage (chunk, done, tool_call, error, connected, media_chunk, upload_ready, upload_complete)
- **gRPC** to Runtime: ClientMessage (user message, client tool result)
- **HTTP** to Session API: session create, message append, TTL refresh

## Does NOT Own
- Tool execution logic (Runtime's job — client or server)
- LLM provider interaction (Runtime's job)
- Session persistence (Session API's job)
- Prompt pack content or evaluation (Runtime's job)
- UI state management (Dashboard's job)
- Authentication (passes headers through)

## Observability

**Metrics** (Prometheus, prefix `omnia_agent_` and `omnia_facade_`):
- Connection gauges: `connections_active`, `sessions_active`, `requests_inflight`
- Request counters: `requests_total` (by status), `messages_received_total`, `messages_sent_total`
- Latency: `request_duration_seconds` (by handler)
- Media transfer: `uploads_total`, `upload_bytes_total`, `downloads_total`, `media_chunks_total`

**Traces** (OpenTelemetry):
- `omnia.facade.message` — per-message span wrapping the full request lifecycle
- Derives trace ID from session UUID (lossless 128-bit mapping) so all spans in a session share one trace — enables Tempo lookup by session ID
- Links to caller's W3C traceparent (e.g., from arena-worker) as a span link for cross-referencing
- Propagates trace context to Runtime via gRPC and to Session API via HTTP

## Dependencies
- Runtime gRPC server (default `localhost:9000`)
- Session API HTTP endpoint (configurable via `SESSION_API_URL`)
- Media storage provider (optional: S3/GCS/Azure/local)

## Migration Note
The hand-written `internal/session/httpclient/` client will migrate to the generated `pkg/sessionapi/` client in a future PR.
