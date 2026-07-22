# Facade Service

## Owns
- **External / management-plane listener isolation**: each facade surface (WebSocket, A2A, MCP) is served on **two listeners** — an *external* port (`facade` 8080 / `a2a` 9999 / `mcp` 9998) running the **external** auth chain (data-plane validators: clientKeys/oidc/edgeTrust, from `spec.externalAuth`), and an *internal* twin port (`facade-mgmt` 18080 / `a2a-mgmt` 19999 / `mcp-mgmt` 19998) running a **management-plane-only** chain. The external chain no longer carries the mgmt-plane validator — dashboard-minted mgmt-plane JWTs are accepted **only** on the internal ports. Internal ports are ClusterIP-only (never on an external Gateway/HTTPRoute) and fail closed without a valid mgmt JWT. Gated per-facade by `spec.facades[].managementPlane` (default true); the enabled internal ports are advertised in `AgentRuntime.status.managementEndpoints{ws,a2a,mcp}`, which the dashboard WS proxy and Doctor read to dial the management plane.
- WebSocket server for browser/client connections
- **Graceful drain on SIGTERM**: On SIGTERM the facade enters drain mode — `/readyz` starts returning 503 and new WebSocket upgrades that are NOT realtime resume requests are rejected at the app layer (HTTP 503 in `ServeHTTP`). Active and parked realtime sessions continue to be served until they finish naturally or until `drainTimeout` elapses. Sessions still open at the deadline are force-closed. The Kubernetes Service removes the pod from the endpoint list as soon as `/readyz` starts failing, so the load-balancer stops sending new traffic. Direct pod-IP connections (used by the T1 blip-resume proxy route) bypass Service readiness entirely, so they are rejected at the application layer by the drain gate rather than at the Service/LB layer.
- Protocol translation: WebSocket JSON <-> gRPC bidirectional stream
- Connection lifecycle (upgrade, ping/pong, close, rate limiting)
- Session creation and routing
- Binary frame encoding/decoding for media
- Media upload URL negotiation (S3/GCS/Azure/local)
- Client-side tool result routing to active handler
- Session recording via HTTP client to Session API
- Recording-policy gating — fetches the effective `SessionPrivacyPolicy` from session-api (`GET /api/v1/privacy-policy`) and caches it per agent for 60s. Conversation messages are recorded by the RuntimeClient gRPC bus interceptor (protocol- and runtime-agnostic): it skips recording when `Recording.Enabled=false` and drops assistant content when `runtimeData=false`. Fails open (records) on fetch errors so data is never silently dropped.
- **Realtime session park-and-resume**: On unintentional WebSocket close during an active realtime duplex session, the facade parks the session (provider socket, state, and timer) in an in-memory registry with a configurable grace period. A reconnecting client that presents `resume=<session_id>` is reattached if ownership is verified and the parked session has not expired. The parked session is immediately closed on an intentional `{"type":"hangup"}` client message. A best-effort Redis route table (`rt:route:<session_id>`→podIP) with TTL equal to the grace period enables the dashboard proxy to route a reconnect to the correct pod (single-replica deployments work without Redis). Expired parked sessions are cleaned up automatically.

## Inputs
- **`AgentRuntime.spec.facades[].drainTimeout`** (duration string, optional, on the websocket facade): How long the facade waits for active realtime sessions to finish on SIGTERM before force-closing them. Default: `30s`. The operator sets the pod's `terminationGracePeriodSeconds` to `drainTimeout + 15s` (the extra 15 s gives the process time to tear down after the drain window closes). Example: `drainTimeout: "30s"` → `terminationGracePeriodSeconds: 45`.
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
  - `RuntimeHello` — the runtime's first ServerMessage (capabilities + duplex `MediaNegotiation` counter-offer). On the duplex path the audio counter-offer is relayed to the browser as a `session_config` message; a video counter-offer fails the session closed (`UNSATISFIABLE_FORMAT`). On the text path it carries capabilities only and is consumed, not forwarded.

## Outputs
- **WebSocket** to browser/dashboard: ServerMessage (chunk, done, tool_call, error, connected, media_chunk, upload_ready, upload_complete, **interrupt** — signals barge-in; client should clear buffered audio; **session_config** — relays the runtime's negotiated duplex audio format (`codec`/`sample_rate`/`channels`) so the client (re)captures at it). The `connected` message includes a `resumed` boolean field indicating whether this connection reattached to a parked realtime session.
- **gRPC** to Runtime: ClientMessage (user message, client tool result, `DuplexStart` to open a duplex audio session, `AudioInputChunk` per audio frame); `HasConversation` to ask whether a named session's working context can still be resumed
- **HTTP** to Session API: session create, message append, `GET /api/v1/privacy-policy` (at connection time, cached 60s per WebSocket session). Writes only — session-api is never read to decide whether a conversation can continue (see "Resuming a session").

## Resuming a session

A client continues a conversation by naming it in `session_id`. Resumability is
decided by the **context store via the Runtime** (`HasConversation`), never by
session-api: a session-api row proves a conversation once existed, not that its
turns still exist in the context store, so treating a found row as resumable is
how a session "resumes" into an empty model context (#1876).

Only an id the client brought from elsewhere is treated as a resume request. The
id the facade minted for this connection and announced in `connected` names the
connection's own session, which legitimately does not exist yet on the first
message — probing for it would reject the opening turn of every new conversation.

| Probe result | Behaviour |
|---|---|
| resumable | The conversation continues. |
| context gone | `SESSION_EXPIRED` is sent and the message is **dropped** rather than answered with no history. The connection stays open; the client should retry with no `session_id`, which starts a new session. |
| store unreachable | `INTERNAL_ERROR`. Never reported as an expiry — the context may be intact. |

Realtime blip-resume (`resume=<session_id>`) is separate: it reattaches a parked
provider socket held in this pod, and is resolved before any message arrives.

## Does NOT Own
- Tool execution logic (Runtime's job — client or server)
- LLM provider interaction (Runtime's job)
- Session persistence (Session API's job)
- Whether a conversation can be resumed (the context store's answer, via the Runtime)
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
- Realtime drain: `omnia_facade_realtime_draining` (gauge, 1 while pod is in drain mode, 0 otherwise), `omnia_facade_realtime_drain_duration_seconds` (histogram by `reason`: `all_drained` / `deadline` / `ctx_canceled`), `omnia_facade_realtime_calls_drained_total` (counter, realtime calls that completed gracefully during drain), `omnia_facade_realtime_calls_force_ended_total` (counter, realtime calls still live when the drain timeout or context cancellation fired)

**Traces** (OpenTelemetry):
- `omnia.facade.message` — per-message span wrapping the full request lifecycle
- Derives trace ID from session UUID (lossless 128-bit mapping) so all spans in a session share one trace — enables Tempo lookup by session ID
- Links to caller's W3C traceparent (e.g., from arena-worker) as a span link for cross-referencing
- Propagates trace context to Runtime via gRPC and to Session API via HTTP

## Dependencies
- Runtime gRPC server (default `localhost:9000`)
- Session API HTTP endpoint (configurable via `SESSION_API_URL`)
- Media storage provider (optional: S3/GCS/Azure/local)
