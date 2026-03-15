# Session API Service

## Owns
- HTTP REST API for session CRUD operations
- Tiered session storage: hot (Redis) -> warm (PostgreSQL) -> cold (S3/GCS/Azure)
- Session listing, search, and filtering
- Message append with event publishing (Redis Streams)
- TTL management and session expiry
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
  - `POST /api/v1/sessions/{id}/ttl` — refresh TTL
  - `DELETE /api/v1/sessions/{id}` — delete session
  - Eval result endpoints (POST/GET)
- **gRPC/HTTP** OTLP trace ingestion (optional)

## Outputs
- **HTTP** responses with JSON payloads to callers
- **PostgreSQL** writes: session records, messages, eval results
- **Redis** writes: hot cache, event publishing via Redis Streams
- **Cold storage** writes: archived sessions (S3/GCS/Azure)

## Does NOT Own
- LLM execution or conversations (Runtime's job)
- WebSocket protocol (Facade's job)
- Tool execution (Runtime's job)
- K8s resource management (Operator's job)
- Session creation decisions (callers decide when to create)

## Dependencies
- PostgreSQL (required, warm store)
- Redis (optional, hot cache + event streaming)
- Cold storage provider (optional: S3/GCS/Azure)
