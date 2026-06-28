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
| `GET` | `/api/v1/privacy/enforcement-stats` | Workspace-scoped enforcement event stats |

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

## What privacy-api does NOT own

- **Audit log** — consent change events are emitted by the ConsentHandler only
  when an audit logger is wired in (Phase 2, TODO `#1642-P2`); the audit log
  itself lives in the `session-api` audit tables.
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
