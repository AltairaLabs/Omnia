# privacy-api

Standalone HTTP service that manages per-workspace user consent grants and
opt-out preferences. One instance per workspace; each workspace has its own
PostgreSQL database for consent data.

## Inputs

### Flags / environment variables

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--postgres-conn` | `POSTGRES_CONN` | _(required)_ | PostgreSQL connection string for the consent DB |
| `--workspace` | `OMNIA_WORKSPACE` | `` | Workspace name; when set with an empty `--postgres-conn`, resolves the connection string from the Workspace CRD via K8s |
| `--redis-url` | `REDIS_URL` | `` | Optional Redis URL (`redis://` or `rediss://`) for warm-cache of preferences |
| `--api-addr` | `API_ADDR` | `:8080` | API server listen address |
| `--health-addr` | `HEALTH_ADDR` | `:8081` | Health probe listen address |
| `--metrics-addr` | `METRICS_ADDR` | `:9090` | Prometheus metrics listen address |
| `--enterprise` | `ENTERPRISE_ENABLED` | `false` | Enable enterprise features (reserved for future use) |
| `--auth-enabled` | — | `false` | Require Kubernetes ServiceAccount bearer-token auth on the JSON API |
| `--auth-allowed-subjects` | — | `` | Comma-separated exact-match ServiceAccount subjects (cross-namespace callers) |
| `--auth-allowed-namespaces` | — | `` | Comma-separated trusted namespaces; any SA in these namespaces is allowed |
| `--auth-audiences` | — | `` | Comma-separated token audiences (optional) |

Pool tuning: `PG_MAX_CONNS` (default 8), `PG_MIN_CONNS` (default 2),
`PG_MAX_CONN_LIFETIME` (default 1h), `PG_MAX_CONN_IDLE_TIME` (default 30m).

Cache TTL: `PRIVACY_CACHE_TTL` (default 60s) — controls how long the Redis
warm cache retains opt-out preferences.

## Outputs / routes

### JSON API (`:8080`)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Liveness probe — always 200 (exempt from auth) |
| `GET` | `/api/v1/privacy/preferences/{userID}` | Fetch opt-out preferences for a pseudonymized user |
| `POST` | `/api/v1/privacy/opt-out` | Set an opt-out preference |
| `DELETE` | `/api/v1/privacy/opt-out` | Remove an opt-out preference |
| `GET` | `/api/v1/privacy/preferences/{userID}/consent` | Get consent grant state |
| `PUT` | `/api/v1/privacy/preferences/{userID}/consent` | Apply consent grants / revocations |
| `GET` | `/api/v1/privacy/consent/stats` | Workspace-scoped aggregate consent stats |
| `GET` | `/api/v1/privacy/enforcement-stats` | Workspace-scoped enforcement event stats (reads the central audit hub) |
| `POST` | `/api/v1/privacy/audit-events` | Ingest forwarded audit events from memory-api / session-api into the central audit hub (#1673). Body `{sourceService, events:[Entry…]}`; idempotent on `(source_service, source_id)`; returns `{ingested, duplicates}` |
| `POST` | `/api/v1/privacy/deletion-request` | Create a DSAR / right-to-erasure request; returns 202 + the request. Processed asynchronously: fans erasure out across every service-group's session-api (delete-by-user) + memory-api (batch-delete). (#1676) |
| `GET` | `/api/v1/privacy/deletion-request/{id}` | Get a deletion request's status (pending/in_progress/completed/failed + sessions_deleted + errors) |
| `GET` | `/api/v1/privacy/deletion-requests?virtual_user_id=…` | List a subject's deletion requests |

### Health server (`:8081`)

| Path | Description |
|------|-------------|
| `GET /healthz` | Liveness probe |
| `GET /readyz` | Readiness probe (Postgres ping) |

### Metrics (`:9090`)

| Path | Description |
|------|-------------|
| `GET /metrics` | Prometheus exposition format |

## What privacy-api owns

- **Consent grants and revocations** — per-user, per-category explicit consent
  stored in the `user_privacy_preferences` table of the per-workspace consent DB.
- **Opt-out preferences** — scope-based (all / workspace / agent) opt-out flags
  for PII redaction enforcement.
- **Per-workspace consent database** — schema migrations applied at startup via
  `ee/cmd/privacy-api/migrations`.
- **Central privacy/compliance audit hub** (#1673) — the `audit_log` table
  (migration `000003`) is the source of truth for the privacy/compliance audit
  slice (enforcement events, consent changes). memory-api and session-api record
  enforcement events in their own local `audit_log`, then a drain-forwarder ships
  them at-least-once to this hub via `POST /api/v1/privacy/audit-events`. The
  dashboard's enforcement-stats read path reads here, not session-api.
- **DSAR (right-to-erasure) lifecycle** (#1676) — the `deletion_requests` table
  (migration `000004`) tracks each erasure request's status. privacy-api owns the
  lifecycle and orchestrates erasure across every service-group: per group it
  calls session-api's `delete-by-user` endpoint (sessions + media) and memory-api's
  batch-delete (memories, scoped by workspace UID), both SA-authenticated. Per-tier
  deletion is delegated to the owning service, so privacy-api holds no warm-store or
  object-storage credentials. (DSAR lifecycle audit events are not yet forwarded to
  the audit hub — see #1678.)

## What privacy-api does NOT own

- **Enforcement decision points** — PII redaction and opt-out blocking happen in
  memory-api's and session-api's privacy middleware; privacy-api only stores the
  forwarded record of those decisions in the audit hub.
- **PII redaction** — redaction logic lives in `session-api`'s privacy
  middleware which calls back to privacy-api (or its shared store) to check
  opt-out state.
- **Memory content** — memory storage and retrieval live in `memory-api`.
- **Session data** — session storage lives in `session-api`.

## Dependencies

- **PostgreSQL** — one database per workspace, connection string from
  `--postgres-conn` or resolved from the Workspace CRD (`--workspace` mode).
- **Redis** (optional) — warm cache for opt-out preference reads; falls back to
  direct Postgres queries when not configured.
- **Kubernetes API** (optional) — only needed in `--workspace` CRD resolution
  mode; requires `get` on `workspaces.omnia.altairalabs.ai` and `get` on the
  database Secret in the workspace namespace.

## Authentication

When `--auth-enabled`, the API requires a Kubernetes ServiceAccount projected
bearer token validated via the TokenReview API. `/healthz` is always exempt.

The privacy-api ServiceAccount needs RBAC `authentication.k8s.io/tokenreviews: create`.
