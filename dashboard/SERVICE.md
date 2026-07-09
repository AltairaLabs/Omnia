# Dashboard Service

## Owns
- Next.js web application (UI)
- WebSocket proxy to backend services (facade, LSP, dev console)
- Proxy routes to Operator REST API
- **Workspace CRD REST API** — create/list/update/delete of workspace-scoped CRDs (AgentRuntime, PromptPack, Provider, ToolRegistry, …) served **directly against the Kubernetes API** (not proxied through the operator), via `src/lib/api/crd-route-factory.ts` → `src/lib/k8s/crd-operations.ts`. See [Deploy / CRD REST API](#deploy--crd-rest-api).
- Client-side state management (Zustand stores)
- Client-side tool consent UI
- Arena project management UI
- Server-side OAuth session storage (iron-session cookie + SessionStore backend)

## Inputs
- **HTTP** from browser: page requests, API proxy calls, deploy-wizard agent creation
- **HTTP** from the external `promptarena-deploy-omnia` deploy adapter: creates PromptPack + AgentRuntime through the workspace CRD REST API (bearer-auth with a workspace-scoped `omnia_sk_` token). See [Deploy / CRD REST API](#deploy--crd-rest-api).
- **WebSocket** from browser: chat messages, tool results

## Outputs
- **HTTP** to Operator: CRUD proxy requests
- **K8s API** (direct): create/list/update/delete of workspace CRDs (AgentRuntime, PromptPack, …). The create/update path is a **verbatim passthrough** — the caller's `body.spec` is applied unmodified; the only schema gate is the CRD's own OpenAPI/CEL validation (surfaced to the caller as the real 4xx, e.g. `422`).
- **WebSocket** to Facade: agent chat (port 8080 on agent pods)
- **WebSocket** to PromptKit LSP: code intelligence
- **WebSocket** to Arena Dev Console: interactive testing
- **HTML/JS** to browser: rendered UI
- **Redis** (optional): session records and PKCE state — required for multi-replica deployments (`OMNIA_SESSION_STORE=redis`)

## Does NOT Own
- Agent session storage or persistence (reads via proxy to Session API)
- LLM execution (Runtime's job via Facade)
- Tool execution (Runtime or client-side, never dashboard-initiated)
- K8s resource *management* / reconciliation (Operator's job) — the dashboard only creates/updates CRD objects; it never reconciles them
- **Deploy-config → CRD schema translation** — the external `promptarena-deploy-omnia` adapter authors AgentRuntime/PromptPack bodies; the dashboard CRD REST API relays them unmodified (see [Deploy / CRD REST API](#deploy--crd-rest-api))
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

## Deploy / CRD REST API

The dashboard serves the workspace-scoped CRD REST API that turns a deploy request
into Kubernetes objects. This is how agents get created — both from the in-app
**deploy wizard** and from the external **`promptarena-deploy-omnia`** adapter (see
the [Deploy Program](../docs/src/content/docs/explanation/platform/deploy-program.md)).

Routes are generated by `src/lib/api/crd-route-factory.ts`
(`createCollectionRoutes` / `createItemRoutes`), e.g.:

| Method + path | Action |
|---|---|
| `POST /api/workspaces/:name/agents` | create an AgentRuntime |
| `GET/PUT/DELETE /api/workspaces/:name/agents/:agentName` | read / replace / delete |
| `POST /api/workspaces/:name/prompt-packs` (+ item routes) | create a PromptPack |

Key properties (and the reason this surface is easy to get wrong):

- **Direct to Kubernetes, not proxied through the operator.** These handlers call
  `@kubernetes/client-node` `CustomObjectsApi` directly with a workspace-scoped
  client (`src/lib/k8s/crd-operations.ts`). The operator is *not* in the create
  path — it only observes the object afterwards via its watch/reconcile loop.
- **Verbatim passthrough — no server-side schema knowledge.** The POST handler
  takes the caller's `body.spec` and applies it unchanged
  (`crd-route-factory.ts` → `buildCrdResource`). It performs **no** field
  translation or schema-version adaptation. The only validation is the CRD's own
  OpenAPI/CEL schema, enforced by the apiserver and surfaced back as the real
  4xx (e.g. `422 spec.facades: Required value`).
- **Two independent authors, one relay.** The dashboard deploy wizard
  (`src/components/agents/deploy-wizard.tsx`, in-tree) and the external
  `promptarena-deploy-omnia` adapter (separate repo, separate release train) both
  POST AgentRuntime bodies through this route. **Each owns its own copy of the
  AgentRuntime schema.**

### Schema-version contract (important)

Because the route is a passthrough and one of the authors lives in another repo,
**there is no single owner of the AgentRuntime schema contract** at this boundary.
A body author (wizard or adapter) must emit the schema that matches the installed
AgentRuntime **CRD + operator** version. When they drift, the failure is
asymmetric and easy to misdiagnose:

- **Author behind the CRD/operator** (adapter still emits an old shape the CRD
  accepts, but the newer operator can't reconcile): the object is created with a
  `201`, then **never reconciles** — empty `status`, no pods — and the only signal
  is a repeating `Reconciler error` in the operator log. (This is exactly what
  the #1576 `spec.facade` → `spec.facades[]` cutover produced for agents deployed
  by an un-upgraded adapter.)
- **CRD/operator ahead of the author** (CRD already upgraded, adapter still old):
  the create **fails loudly at deploy time** with the apiserver's `422`, which is
  the preferred, attributable failure.

Any breaking AgentRuntime CRD change must therefore ship the deploy wizard **and**
the external adapter in lockstep, and CRDs must be upgraded on the cluster
explicitly (`helm upgrade` does **not** upgrade CRDs — see the Deploy Program doc).

## Dependencies
- Kubernetes API (direct, workspace-scoped client) for CRD CRUD — `@kubernetes/client-node`
- Operator REST API (proxied via Next.js API routes)
- Session API (proxied via Operator)
- Facade WebSocket (proxied via server.js on port 3002)
- Redis 6.2+ (optional; required when `OMNIA_SESSION_STORE=redis`)
