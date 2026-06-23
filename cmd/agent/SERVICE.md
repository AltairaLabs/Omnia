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
- Recording-policy gating — on each WebSocket connection, fetches the effective `SessionPrivacyPolicy` from session-api (`GET /api/v1/privacy-policy`) and caches it for 60s. `recordingResponseWriter` skips recording when `Recording.Enabled=false` or restricts writes when `RichData=false`. Fails open (records) on fetch errors so data is never silently dropped.
- **Realtime session park-and-resume**: On unintentional WebSocket close during an active realtime duplex session, the facade parks the session (provider socket, state, and timer) in an in-memory registry with a configurable grace period. A reconnecting client that presents `resume=<session_id>` is reattached if ownership is verified and the parked session has not expired. The parked session is immediately closed on an intentional `{"type":"hangup"}` client message. A best-effort Redis route table (`rt:route:<session_id>`→podIP) with TTL equal to the grace period enables the dashboard proxy to route a reconnect to the correct pod (single-replica deployments work without Redis). Expired parked sessions are cleaned up automatically.

## Inputs
- **WebSocket upgrade** (memory/session identity scoping):
  - `x-omnia-user-id` header — trusted on-behalf-of end-user id, honored **only** for management-plane origin (set by the dashboard WS proxy / portal from the authenticated session). Pseudonymized for memory scoping; takes precedence over `device_id`.
  - `device_id` query param — anonymous/dev fallback identity when no header is present.
  - `resume=<session_id>` query param — realtime blip-resume signal on reconnect. If present, reattaches to an existing parked realtime session after ownership verification. If the parked session has expired or is not found, connection proceeds as a new session.
- **WebSocket** from browser/dashboard:
  - `message` — user text or multimodal content
  - `tool_result` — client-side tool execution result
  - `upload_request` — file upload initiation
  - **Binary frames** (`BinaryMessageTypeMediaChunk`) — raw audio frames during a duplex audio session. Routed to a per-connection `audioSession` → `grpcDuplexSink` which forwards them over the runtime `Converse` gRPC stream as `AudioInputChunk`. A frame with `FlagIsLast` set tears down the session.
  - `{"type":"hangup"}` — intentional close signal for realtime sessions. Closes the provider socket immediately instead of parking.
- **gRPC** from Runtime (response stream):
  - `chunk` — streaming text
  - `done` — response complete
  - `tool_call` — client-side tool call (server-side tool calls are filtered)
  - `error` — error response
  - `media_chunk` — streaming audio/video (also used for duplex audio output)
  - `interruption` — barge-in signal; relayed to the browser as an `interrupt` WebSocket message

## Outputs
- **WebSocket** to browser/dashboard: ServerMessage (chunk, done, tool_call, error, connected, media_chunk, upload_ready, upload_complete, **interrupt** — signals barge-in; client should clear buffered audio). The `connected` message includes a `resumed` boolean field indicating whether this connection reattached to a parked realtime session.
- **gRPC** to Runtime: ClientMessage (user message, client tool result, `DuplexStart` to open a duplex audio session, `AudioInputChunk` per audio frame)
- **HTTP** to Session API: session create, message append, TTL refresh, `GET /api/v1/privacy-policy` (at connection time, cached 60s per WebSocket session)

## Does NOT Own
- Tool execution logic (Runtime's job — client or server)
- LLM provider interaction (Runtime's job)
- Session persistence (Session API's job)
- Prompt pack content or evaluation (Runtime's job)
- UI state management (Dashboard's job)
- Authentication (passes headers through)

## Observability

**Metrics** (Prometheus, prefix `omnia_agent_` and `omnia_facade_`):
- Served on the facade **health port (8081)** at `/metrics` — NOT the app/WS port
  (8080). The container declares this port with the name `metrics`; scrapers
  discover it by that port NAME (the agent pod's bundled-Prometheus
  `omnia-agents` job and the optional `podMonitor` both key on `port: metrics`).
  An agent pod has metrics on two ports across two containers (facade 8081,
  runtime 9001) with no in-pod consolidation, so a single `prometheus.io/port`
  pod annotation cannot cover it — the port-name contract is what makes one
  scrape job/PodMonitor reach both. See #1488.
- Connection gauges: `connections_active`, `sessions_active`, `requests_inflight`
- Request counters: `requests_total` (by status), `messages_received_total`, `messages_sent_total`
- Latency: `request_duration_seconds` (by handler)
- Media transfer: `uploads_total`, `upload_bytes_total`, `downloads_total`, `media_chunks_total`
- Duplex audio: `omnia_facade_audio_sessions_active` (gauge, current live duplex sessions; concurrency cap default 8), `omnia_facade_audio_ingest_duration_seconds` (histogram, facade-receive→sink-send latency per inbound frame; sub-ms buckets)
- Realtime blip-resume: `omnia_facade_realtime_sessions_parked_total` (counter, realtime sessions parked on unintentional close), `omnia_facade_realtime_reattach_total` (counter, successful reattaches via resume), `omnia_facade_realtime_park_expired_total` (counter, parked sessions expired before reattach)

**Traces** (OpenTelemetry):
- `omnia.facade.message` — per-message span wrapping the full request lifecycle
- Derives trace ID from session UUID (lossless 128-bit mapping) so all spans in a session share one trace — enables Tempo lookup by session ID
- Links to caller's W3C traceparent (e.g., from arena-worker) as a span link for cross-referencing
- Propagates trace context to Runtime via gRPC and to Session API via HTTP

## Dependencies
- Runtime gRPC server (default `localhost:9000`)
- Session API HTTP endpoint (configurable via `SESSION_API_URL`)
- Media storage provider (optional: S3/GCS/Azure/local)
