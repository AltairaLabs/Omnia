# Dashboard Service

## Owns
- Next.js web application (UI)
- WebSocket proxy to backend services (facade, LSP, dev console)
- Proxy routes to Operator REST API
- Client-side state management (Zustand stores)
- Client-side tool consent UI
- Arena project management UI
- Server-side OAuth session storage (iron-session cookie + SessionStore backend)

## Inputs
- **HTTP** from browser: page requests, API proxy calls
- **WebSocket** from browser: chat messages, tool results

## Outputs
- **HTTP** to Operator: CRUD proxy requests
- **WebSocket** to Facade: agent chat (port 8080 on agent pods)
- **WebSocket** to PromptKit LSP: code intelligence
- **WebSocket** to Arena Dev Console: interactive testing
- **HTML/JS** to browser: rendered UI
- **Redis** (optional): session records and PKCE state — required for multi-replica deployments (`OMNIA_SESSION_STORE=redis`)

## Does NOT Own
- Agent session storage or persistence (reads via proxy to Session API)
- LLM execution (Runtime's job via Facade)
- Tool execution (Runtime or client-side, never dashboard-initiated)
- K8s resource management (Operator's job)
- Identity provider authentication (delegates to external OIDC/OAuth IDPs)

## Observability

**Metrics**: None — client-side only. Server-side metrics are handled by the Operator (which hosts the dashboard).

**Traces**: None — does not emit OpenTelemetry spans.

## Session storage

The dashboard uses a two-layer session design to avoid the 4 KB browser cookie limit (a class of failures triggered by IDPs such as Cognito, Okta, Auth0, Keycloak, and Entra when group/role claims are included).

### Cookie layer
An iron-session-sealed HTTP-only cookie carries only `{ sid }` (~60 bytes). The cookie is fixed-size regardless of IDP or claim payload.

### Server-side store
The full session record and in-flight PKCE state are kept in the server-side SessionStore, keyed with a `omnia:` namespace prefix so a shared Redis can host multiple Omnia deployments without collision.

| Key pattern | Contents | TTL env var | Default TTL |
|---|---|---|---|
| `omnia:sess:<sid>` | Full session record (user, OAuth tokens, metadata) | `OMNIA_SESSION_TTL` | 86400 s (24 h) |
| `omnia:pkce:<state>` | In-flight PKCE data; consumed atomically via `GETDEL` | `OMNIA_SESSION_PKCE_TTL` | 300 s (5 m) |

### CSRF / session-fixation defence
A tiny ephemeral HTTP-only cookie `omnia_oauth_state` is set at `/login` and cleared at `/callback`. The `/callback` handler verifies the `state` parameter against this cookie before exchanging the code, preventing cross-origin login CSRF and session fixation.

### Backend selection (`OMNIA_SESSION_STORE`)

| Value | Behaviour |
|---|---|
| `"memory"` (default) | Single-process in-memory map. Suitable for dev and test only — not shared across replicas. |
| `"redis"` | Shared Redis. Required for any multi-replica deployment. Requires Redis 6.2+ (`GETDEL` command). |

### Redis env vars (required when `OMNIA_SESSION_STORE=redis`)

| Variable | Purpose |
|---|---|
| `OMNIA_SESSION_REDIS_URL` | Full Redis URL (preferred). e.g. `redis://:password@host:6379/0` |
| `OMNIA_SESSION_REDIS_ADDR` | Host:port (alternative to URL). e.g. `redis:6379` |
| `OMNIA_SESSION_REDIS_PASSWORD` | Password (when using `ADDR` form) |
| `OMNIA_SESSION_REDIS_DB` | Database index (when using `ADDR` form, default `0`) |

### Logout revocation
Logout deletes the `omnia:sess:<sid>` record server-side, truly revoking the session. Cookie-only sessions cannot do this — the cookie remains valid until it expires.

## Dependencies
- Operator REST API (proxied via Next.js API routes)
- Session API (proxied via Operator)
- Facade WebSocket (proxied via server.js on port 3002)
- Redis 6.2+ (optional; required when `OMNIA_SESSION_STORE=redis`)
