# Session API Service

## Owns
- HTTP REST API for session CRUD operations
- Tiered session storage: hot (Redis) -> warm (PostgreSQL) -> cold (S3/GCS/Azure)
- Session listing, search, and filtering
- Message append with event publishing (Redis Streams)
- TTL management and session expiry
- Tool call and provider call recording (first-class tables)
- Runtime event recording (pipeline, stage, middleware, validation lifecycle)
- Eval result storage and retrieval
- OTLP trace ingestion (optional)
- Rate limiting per client IP
- Audit logging (enterprise)
- PII redaction middleware — intercepts all write requests and redacts PII from message content, tool call arguments/results, provider call payloads, event metadata, and eval results based on the effective SessionPrivacyPolicy (enterprise)
- Privacy opt-out enforcement — silently drops writes (204 No Content) when the user has opted out via preferences (enterprise)
- Recording-flag enforcement — when the effective `SessionPrivacyPolicy.Recording.Enabled=false`, write endpoints return 204; when `RichData=false`, the middleware blocks assistant messages, tool calls, runtime events, and provider calls while allowing user messages, status updates, and TTL refreshes (enterprise)
- SessionPrivacyPolicy CRD watching — `PolicyWatcher` polls `SessionPrivacyPolicy`, `Workspace`, and `AgentRuntime` CRDs every 30 s and maintains in-memory sync.Map caches; `GetEffectivePolicy(namespace, agentName)` resolves the policy using a deterministic chain (AgentRuntime override → service group → global default at `omnia-system/default`); the resolved policy drives PII redaction, opt-out enforcement, and recording gating (enterprise)
- Per-request encryption resolver — on each session-api write, the `PolicyWatcher`-resolved `EncryptionConfig` is used to select a `(kmsProvider, keyID)` pair; the `Encryptor` wraps AES-256-GCM data keys via the selected KMS provider; results are cached per `(kmsProvider, keyID)` tuple (enterprise)
- Privacy/GDPR deletion with media artifact cleanup, batch processing, and progress tracking (enterprise)
- Privacy opt-out preferences (enterprise)

## Inputs
- **HTTP** from Facade, Runtime, Dashboard (proxied via Operator):
  - `POST /api/v1/sessions` — create session
  - `GET /api/v1/sessions` — list/search sessions
  - `GET /api/v1/sessions/{id}` — retrieve session
  - `GET /api/v1/sessions/{id}/messages` — get messages
  - `POST /api/v1/sessions/{id}/messages` — append message
  - `POST /api/v1/sessions/{id}/tool-calls` — record tool call
  - `GET /api/v1/sessions/{id}/tool-calls` — get tool calls
  - `POST /api/v1/sessions/{id}/provider-calls` — record provider call
  - `GET /api/v1/sessions/{id}/provider-calls` — get provider calls
  - `POST /api/v1/sessions/{id}/events` — record runtime event
  - `GET /api/v1/sessions/{id}/events` — get runtime events
  - `POST /api/v1/eval-results` — record eval results
  - `GET /api/v1/sessions/{id}/eval-results` — get session eval results
  - `GET /api/v1/eval-results` — list eval results with filters
  - `POST /api/v1/sessions/{id}/ttl` — refresh TTL
  - `PATCH /api/v1/sessions/{id}/stats` — update session counters
  - `DELETE /api/v1/sessions/{id}` — delete session
  - `GET /api/v1/privacy-policy?namespace={ns}&agent={agent}` — returns the facade-visible subset of the effective SessionPrivacyPolicy (`{"recording":{"enabled","facadeData","richData"}}`); 204 when no policy applies
- **gRPC/HTTP** OTLP trace ingestion (optional)

## Outputs
- **HTTP** responses with JSON payloads to callers
- **PostgreSQL** writes: sessions, messages, tool_calls, provider_calls, runtime_events, eval_results, message_artifacts
- **Redis** writes: hot cache, event publishing via Redis Streams
- **Cold storage** writes: archived sessions (S3/GCS/Azure)

## Does NOT Own
- LLM execution or conversations (Runtime's job)
- WebSocket protocol (Facade's job)
- Tool execution (Runtime's job)
- K8s resource management (Operator's job)
- Session creation decisions (callers decide when to create)

## Observability

**Metrics** (Prometheus, prefix `omnia_session_api_`):
- HTTP: `requests_total` (by method, route, status_code), `request_duration_seconds`
- Events: `events_published_total` (by status), `event_publish_duration_seconds`
- Route paths are normalized (UUIDs → `:id`) to prevent cardinality explosion

**Traces** (OpenTelemetry):
- Inherits trace context from incoming HTTP requests (propagated from Facade/Runtime)
- Redis provider creates spans for cache operations
- OTLP trace ingestion endpoint (optional) — receives traces from Runtime/Facade and transforms them into session-linked records for dashboard display

**Audit** (enterprise, prefix `omnia_audit_`):
- `audit_events_total`, `audit_write_duration_seconds`, `audit_buffer_drops_total`

## API Contract

The **source of truth** for the Session API surface is:

- **OpenAPI spec**: `api/session-api/openapi.yaml`
- **Generated Go client**: `pkg/sessionapi/` (regenerate with `make generate-session-api-client`)
- **Generated TS types**: `dashboard/src/lib/api/session-api-schema.d.ts` (regenerate with `make generate-session-api-types`)

All consumers now use the generated client types from `pkg/sessionapi/`. The eval worker uses `ClientWithResponses` directly; the facade httpclient uses the generated types for serialization while keeping its own retry/circuit-breaker/write-buffer infrastructure.

## Deletion Pipeline (Enterprise)

The GDPR/CCPA deletion pipeline processes user data removal requests:

1. **Request created** — validated and persisted with `pending` status
2. **Session discovery** — lists all sessions for the user (optionally scoped by workspace)
3. **Batch processing** — sessions are processed in configurable batches (default 100):
   - Warm store deletion (PostgreSQL)
   - Media artifact cleanup (object storage, when configured)
4. **Progress tracking** — `SessionsDeleted` count updated after each batch, pollable via GET endpoint
5. **Completion** — request marked `completed` or `failed` (partial failures are recorded per-session)

Media cleanup uses the `MediaDeleter` interface. When object storage is not configured, a no-op deleter is used so the pipeline proceeds without error. Cold storage deletion is not needed because Phase 1 PII filtering ensures no PII reaches the cold tier.

## Privacy Architecture

The session-api is the **single enforcement point** for PII redaction. All session data writers (facade, runtime, arena-worker) converge on the session-api, so placing redaction here catches every write path without requiring changes to originators.

The privacy middleware sits in front of all write endpoints (POST/PATCH/PUT):
1. Extracts session ID from the URL path
2. Resolves session → namespace/agent via a bounded LRU cache backed by the warm store
3. Computes the effective SessionPrivacyPolicy using the CRD watcher's policy inheritance chain
4. Checks user opt-out preferences — returns 204 if the user has opted out
5. Applies PII redaction to the request body based on the endpoint type and configured PII patterns

The `X-Omnia-User-ID` header is propagated by the facade and runtime on all write requests, enabling per-user opt-out enforcement.

## Dependencies
- PostgreSQL (required, warm store)
- Redis (optional, hot cache + event streaming)
- Cold storage provider (optional: S3/GCS/Azure, also used for media artifact cleanup)
- Kubernetes API (enterprise: watches `SessionPrivacyPolicy`, `Workspace`, and `AgentRuntime` CRDs via `PolicyWatcher` — drives PII redaction, opt-out, recording flags, and per-request encryption resolver)
