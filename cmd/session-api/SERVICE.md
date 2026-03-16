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
- Privacy/GDPR deletion and opt-out (enterprise)

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

## Dependencies
- PostgreSQL (required, warm store)
- Redis (optional, hot cache + event streaming)
- Cold storage provider (optional: S3/GCS/Azure)
