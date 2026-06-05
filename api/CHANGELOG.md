# API Surface Changelog

Changes to any API surface (REST, gRPC, WebSocket) should be logged here
so that parallel workstreams have visibility into contract changes.

When modifying files in `internal/session/api/`, `internal/facade/protocol.go`,
or `api/proto/`, add an entry below with the date, affected API, and reason.

---

## Unreleased

### Added (runtime: memory retrieval strategy/limit/denyCEL wired end-to-end, #1205)

`AgentRuntime.spec.memory.retrieval` fields are now honored at runtime by the
CompositeRetriever (previously they were parsed from the CRD but not plumbed to
the retriever):

- **`spec.memory.retrieval.strategy: "semantic"`** — per-turn episodic retrieval
  now uses the memory-api's semantic hybrid search path when the strategy is
  `"semantic"` and the memory store supports it. Falls back to keyword FTS when
  the store does not implement `RetrieveSemantic`.
- **`spec.memory.retrieval.limit`** — caps the number of episodic memories
  injected per turn. Previously the field was parsed and stored in `Config` but
  never forwarded to `CompositeRetriever`, so the limit was always the
  hard-coded default (10). A non-zero value now overrides the default; 0 (or
  absent) retains the default of 10.
- **`spec.memory.retrieval.accessFilter.denyCEL`** (new field): optional CEL
  expression evaluated against a retrieved memory item's `metadata` map. Items
  for which the expression evaluates to `true` are dropped before injection.
  Empty string disables the filter. Applied only on the `strategy: semantic`
  path (governance deny-filter).

`WithMemoryRetrieval` now accepts a third argument (`limit int`); callers
(`cmd/runtime/main.go` → `configDerivedServerOpts`) are updated. The
`ServerMemoryRetrieval` test accessor now returns `(strategy, denyCEL string, limit int)`.

### Added (memory-api ingest + semantic-retrieve endpoints, #1205)

- `POST /api/v1/institutional/ingest` — accepts `{workspace_id, title, url,
  site, text}` (all strings; `workspace_id` required). Runs the configured
  `IngestionStrategy` (default `ChunkStrategy`; chunk size and overlap
  controlled by `--ingest-chunk-size` / `INGEST_CHUNK_SIZE` and
  `--ingest-chunk-overlap` / `INGEST_CHUNK_OVERLAP`, defaults 200 and 40).
  Each chunk is persisted as an institutional memory keyed by
  `about={kind:"sharepoint_doc", key:"<url>#<index>"}` — re-sending the same
  URL supersedes prior chunks (idempotent re-seed). Returns **202 Accepted**
  with no body; embeddings are backfilled asynchronously by the `ReembedWorker`.
  Errors: 400 on missing `workspace_id`; 500 when no strategy is configured or
  the strategy returns an error.

- `POST /api/v1/memories/retrieve/semantic` — accepts `{workspace_id, query,
  deny_cel, limit}` (`workspace_id` required; `deny_cel` optional; `limit`
  defaults to 20, max 100). Runs workspace-scoped hybrid retrieval
  (`SearchMemories` — semantic + FTS) then applies a CEL deny-filter over each
  result's metadata. Empty `deny_cel` means no filtering. A malformed
  `deny_cel` **fails closed** (500 — no results returned). Response shape
  matches the existing memory list endpoints: `{memories: [...], total: N}`
  (200). Error: 400 on missing `workspace_id`.

- New binary flags (memory-api):
  - `--ingest-chunk-size` / `INGEST_CHUNK_SIZE` (int, default 200) — word
    count per ingest chunk.
  - `--ingest-chunk-overlap` / `INGEST_CHUNK_OVERLAP` (int, default 40) —
    overlapping words between adjacent chunks.

### Added (MemoryPolicy per-axis consolidation schedules, #1152)

`MemoryPolicy.spec.consolidation.schedules` adds optional per-axis cron
overrides (`staleObservations`, `crossScopeCandidates`,
`entityDuplicateCandidates`). Each axis falls back to
`spec.consolidation.schedule` when unset, which itself defaults to
`"0 2 * * *"`. The consolidation worker now honours these schedules per
axis — previously `schedule` was parsed but ignored and every axis ran on
the global tick. `--consolidation-interval` (operator:
`--memory-consolidation-interval`) is now the schedule-evaluation (poll)
interval rather than the run cadence. Backward compatible: operators who
never enabled the worker are unaffected.

### Changed (Functions-as-sessions rework, PR 3/3: dashboard reads sessions data)

The `/functions/{name}` detail page reads recent invocations from the
standard sessions data path instead of the placeholder shipped in
PR 1. Function invocations are ordinary sessions tagged `"function"`;
the new `FunctionSessionsPanel` reuses the existing `useSessions` hook
+ workspace session-api proxy that already powers `/sessions`.

- New `FunctionSessionsPanel` component renders a per-function
  history table (timestamp, status badge, latency derived from
  `startedAt`/`endedAt`, estimated cost, truncated session id).
  Each row links into `/sessions/{id}` so operators can drill from
  this view into the full session detail (messages, tool calls,
  provider calls, eval results).
- `FunctionDetailPage` mounts the panel below the schema cards and
  drops the PR-1 placeholder.
- Status badge mapping mirrors `sessions.status`: `active` →
  "Active" (secondary), `completed` → "Completed" (default),
  `error` → "Error" (destructive), `expired` → "Expired" (outline).

Tests: 9 new specs for `FunctionSessionsPanel` (loading, error,
empty, row rendering, status mapping, latency formatting, link
target, missing endedAt, hook argument forwarding); the detail-page
test updated to assert the panel is mounted with the correct
function name.

### Changed (Functions-as-sessions rework, PR 2/3: function invocations are sessions)

Function-mode pods now open a real `sessions` row at invocation start
and close it with the terminal status when the response is written.
Same data model as agent-mode sessions; the runtime's existing
`OmniaEventStore` machinery feeds `messages`, `tool_calls`,
`provider_calls`, `eval_results`, and `runtime_events` against the
same `session_id` (which is the invocation id).

- `FunctionsHandler.WithSessionStore(store, meta)` injects the session
  dependency. `FunctionSessionMeta` carries the per-pod identity
  (namespace, agent name, workspace, prompt-pack name + version).
- Session rows are tagged `["function"]` (constant
  `facade.FunctionSessionTag`) for fast dashboard filtering.
- The invocation id is generated at the very top of `ServeHTTP` so
  `input_invalid` and body-read failures still have a `session_id`
  for Loki + dashboard correlation. The facade emits a runtime event
  on these pre-runtime failures (`function.input_invalid`,
  `function.payload_too_large`, `function.read_body_failed`) so the
  failure detail is queryable from the session detail page.
- Outcome → status mapping:
  - success → `completed`
  - input_invalid / output_invalid / runtime_error / payload_too_large
    / read_body_failed / response_write_failed → `error`
  - `ended_at` is set to the close time on every terminal outcome.
- Session store failures are best-effort: a session-api outage logs
  but does not fail the user-facing request — the runtime still
  produces its result and the audit rows simply land orphaned.
- `cmd/agent/functions.go` wires the session store via the same
  `initSessionStore()` the WebSocket path uses. Failure to resolve
  session-api at startup is logged and non-fatal.

The next PR (3/3) repoints the dashboard `/functions/{name}` detail
view at the sessions data source.

### Removed (Functions-as-sessions rework, PR 1/3: rip dedicated infrastructure)

**Breaking — affects Unreleased only; never shipped a release.**

Function invocations now record as ordinary `sessions` rows (tagged
`"function"`) rather than to a parallel `function_invocations` table.
Same data model as agent-mode sessions, same retention rules, same
dashboard surfaces — eliminates the orphaned-audit-rows problem where
tool / provider / eval / runtime records keyed off `session_id` but
no parent `sessions` row ever existed.

- Dropped Postgres table `function_invocations` (migration 28). The
  `manage_session_partitions` orchestrator is restored to its
  pre-migration-26 table list.
- Dropped session-api endpoints:
  - `POST /api/v1/function-invocations`
  - `GET /api/v1/function-invocations[?...]`
  - `GET /api/v1/function-invocations/{id}`
- Dropped workspace proxy routes:
  - `GET /api/workspaces/{name}/function-invocations[?...]`
  - `GET /api/workspaces/{name}/function-invocations/{id}`
- Dropped the `spec.invocationRecording` field on AgentRuntime (and
  its `state` enum). The CRD-level CEL gate forbidding `state: enabled`
  on agent-mode runtimes is removed alongside it.
- Dropped `facade.InvocationRecorder` interface + `WithRecorder()`
  builder + `record()` helper + `sha256HexSum` from the facade.
- Dropped `httpclient.RecordFunctionInvocation` + `FunctionInvocationRecord`.
- Dashboard: `function-invocations-service`, `use-function-invocations`,
  `FunctionInvocationsPanel`, and the `recordInvocations` toggle on
  the deploy wizard are gone. `/functions/{name}` detail page renders
  a placeholder until PR 3 wires the session-backed history view.

The next two PRs in this slicing wire runtime-driven session creation
for function invocations (PR 2) and repoint the dashboard at the
sessions data source (PR 3).

### Added (docs + example: Functions Phase 1 closes out, #1103 PR 7)

- New how-to: `docs/src/content/docs/how-to/define-functions.md` —
  pack-authoring guide for function-mode AgentRuntimes. Covers when
  to pick function vs agent, the CRD shape, the invocation contract,
  status code reference, authentication, and the
  `invocationRecording` opt-in.
- AgentRuntime CRD reference extended with first-class entries for
  the new `mode`, `inputSchema`, `outputSchema`, and
  `invocationRecording` fields in
  `docs/src/content/docs/reference/agentruntime.md`.
- New `examples/echo-function/` worked example — a minimal,
  applyable Function (PromptPack + Provider + AgentRuntime + README)
  showing the smallest viable function-mode runtime. Echoes the
  request's `message` field back as `echo`.

### Added (dashboard: Functions catalog + invocation history, #1103 PR 6)

- New `/functions` catalog page lists function-mode AgentRuntimes
  (filtered client-side from the existing AgentRuntime list — no new
  operator endpoint). Each card shows the namespace, recording opt-in
  state, and a top-level field count for `inputSchema` / `outputSchema`.
- New `/functions/{name}` detail page renders the resolved input /
  output schemas alongside a panel of recent invocations sourced from
  session-api's `function_invocations` rows. Time-window presets:
  1h / 24h / 7d. Latency and cost sparklines aggregate the loaded
  window; the table shows timestamp, status, latency, cost, and a
  truncated trace id per row.
- Workspace-scoped proxy routes added:
  - `GET /api/workspaces/{name}/function-invocations[?function=&from=&to=&limit=]`
    → `SESSION_API_URL/api/v1/function-invocations?namespace={name}[&…]`
  - `GET /api/workspaces/{name}/function-invocations/{id}`
    → `SESSION_API_URL/api/v1/function-invocations/{id}?namespace={name}`
  The proxy pins the session-api `namespace` query param to the
  workspace name — a malicious caller cannot read another tenant's
  rows by overriding the query string.
- `AgentRuntimeSpec` hand-written type extended with `mode`,
  `inputSchema`, `outputSchema`, `invocationRecording` (already present
  in the generated types since PR 1 / #1104). `isFunctionMode(spec)`
  helper exported for catalog filtering.

### Added (session-api: function_invocations persistence + facade write path, #1103 PR 5)

- New `function_invocations` table in the session-api Postgres schema
  (migration `000026_create_function_invocations`). Partitioned weekly
  by `created_at`, same lifecycle as `sessions` / `messages` etc. The
  `manage_session_partitions` orchestrator now includes
  `function_invocations` in its rotation.
- Session-API endpoints (mounted on the existing handler mux):
  - `POST /api/v1/function-invocations` — write a single audit row.
    Called by the facade after a Function call when
    `AgentRuntime.spec.invocationRecording.state == "enabled"`.
    Returns 201 + the row on success, 400 on missing fields, 503 when
    the store isn't configured.
  - `GET /api/v1/function-invocations?namespace=X[&function=Y&from=...&to=...&limit=N]`
    — list recent invocations for a namespace, optionally scoped to a
    function name + time window. Pagination defaults to 100 rows,
    capped at 1000.
  - `GET /api/v1/function-invocations/{id}?namespace=X` — single row.
    Cross-tenant reads return 404 (no existence leak).
- Status enum (matches the table's CHECK constraint):
  `success | input_invalid | output_invalid | runtime_error`.
- Facade write path: `FunctionsHandler.WithRecorder(...)` opt-in
  recorder; the agent binary wires it via the session-api HTTP client
  when `cfg.FunctionRecordsInvocations` is true. Recording is
  best-effort — a recorder failure logs but does NOT fail the
  user-facing Function call (Q3 / #1103 lock).
- Input hash is sha256 of the JSON body — stored in lieu of the raw
  input so persistence doesn't grow PII surface area. Output JSON is
  stored verbatim; functions whose outputs carry sensitive data
  should keep `state: disabled`.

### Added (operator + facade activation for function-mode AgentRuntimes, #1103 PR 4)

- AgentRuntime pods now branch on `spec.mode` at startup:
  - `mode: agent` (default) — unchanged. WebSocket / A2A facade as before.
  - `mode: function` — HTTP-only facade serving `POST /functions/{name}`
    on the same `cfg.FacadePort` previously used for WebSocket. The
    runtime sidecar is the same in both modes; only the facade routing
    differs.
- Function-mode AgentRuntimes use `facade.type: grpc` (CRD CEL rejects
  `websocket` for `mode=function`); the agent binary now accepts `grpc`
  as a valid `FacadeType` enum value.
- The function-mode route resolves `{name}` against this pod's
  AgentRuntime name (canonicalised to lowercase per RFC1123). One
  function per pod.
- Pod label `omnia.altairalabs.ai/mode` carries the runtime's
  `EffectiveMode()` for operational visibility (`kubectl get pods -l omnia.altairalabs.ai/mode=function`).
  The label lives on the pod template only — it is intentionally NOT
  in the Deployment selector (selectors are immutable; rolling out a
  mode change would otherwise fail with `field is immutable`).
- Function-mode pods read `spec.inputSchema`, `spec.outputSchema`, and
  `spec.invocationRecording.state` from the CRD at startup and compile
  the schemas once. Schema changes require a Deployment rollout
  (existing behaviour for any CRD-driven config).
- Function-mode pod's `/readyz` checks the runtime sidecar's gRPC
  Health — same readiness invariant as the WebSocket path. The runtime
  dial uses the same exponential-backoff retry as the WebSocket path
  (up to 10 attempts, capped at 5s between attempts).
- The function HTTP server has a 60s `WriteTimeout` so a stalled
  runtime doesn't leak sockets. (The WebSocket server intentionally
  has no `WriteTimeout` because connections are long-lived.)

**Security posture (resolved in PR 5 follow-up):**

The function route reuses the WebSocket data-plane + mgmt-plane
validator chain (`auth.Middleware`). Every request to
`POST /functions/{name}` must present a credential admitted by at least
one configured validator, with 401 (`unauthorized`) on rejection.

The legacy `OMNIA_FUNCTION_ALLOW_UNAUTHENTICATED` env var has been
removed. The function path now honours the same
`OMNIA_FACADE_ALLOW_UNAUTHENTICATED=true` dev-only escape hatch as the
WebSocket path — for the empty-chain case (no externalAuth, mgmt-plane
unreadable). Production must never set this; the empty-chain branch is
strict-default 401 with the flag unset.

### Added (facade HTTP: POST /functions/{name} for function-mode AgentRuntimes, #1103 PR 3)

- `POST /functions/{name}` on the facade HTTP port — entry point for
  function-mode AgentRuntime invocations. Server-to-server only. The
  PR 3-era plan was to reuse the WebSocket auth chain in PR 4; that
  did NOT happen — see the PR 4 entry above for the current
  strict-default-403 posture and the bypass env var.
- Request: `Content-Type: application/json`, body validated against
  `AgentRuntime.spec.inputSchema` (santhosh-tekuri/jsonschema/v6).
  Request body capped at 1 MiB.
- Success response (200): `{ output, invocation_id, duration_ms, usage{input_tokens, output_tokens, cost_usd} }`.
  `output` is the model's raw JSON, already validated against
  `spec.outputSchema`.
- Error responses use a uniform JSON envelope `{ error, detail }`:
  - 400 `input_invalid` — payload failed inputSchema. Runtime not called.
  - 400 `missing_function_name` — empty `{name}` path segment.
  - 400 `read_body_failed` — body read error.
  - 404 `function_not_found` — no function-mode AgentRuntime with that
    name on this facade (also returned for mode=agent runtimes to avoid
    enumeration).
  - 405 `method_not_allowed` — only POST is accepted.
  - 413 `payload_too_large` — body exceeded 1 MiB cap.
  - 415 `unsupported_media_type` — Content-Type must be application/json.
  - 502 `runtime_error` — runtime gRPC Invoke returned an error.
  - 502 `output_invalid` — model output failed outputSchema. Envelope
    includes `raw_output` (JSON if valid; JSON string otherwise) so the
    function author can debug (per #1103 Q2).

### Added (runtime gRPC: Invoke for function-mode AgentRuntimes, #1103 PR 2)

- `RuntimeService.Invoke(InvocationRequest) returns (InvocationResponse)` — new
  unary RPC for one-shot Function calls. Facade-only consumer (PR 3); browsers
  don't talk to this directly.
- `InvocationRequest{input_json, invocation_id, metadata}` — `input_json` is
  opaque to the runtime; the facade validates against
  `AgentRuntime.spec.inputSchema` before sending.
- `InvocationResponse{output_json, usage, duration_ms, invocation_id}` —
  `output_json` is the raw assistant text. The facade validates against
  `spec.outputSchema` and, on mismatch, returns HTTP 502 with the raw body
  for debugging (per #1103 Q2 lock).
- Error codes: `InvalidArgument` for missing `invocation_id` / `input_json`;
  `FailedPrecondition` if a function emits a client-side tool call (function
  mode has no WebSocket peer to fulfil them); `Internal` for stream / pack
  failures.

### Added (provider-calls aggregate + discover endpoints)

- `GET /api/v1/provider-calls/aggregate?namespace=X&groupBy=Y&metric=Z` —
  namespace-scoped GROUP BY over `provider_calls` (INNER JOIN sessions
  so the namespace + agentName filters can apply). `groupBy` is one of
  `provider` | `model` | `agent` | `time:hour` | `time:day`; `metric` is
  one of `count` | `sum_cost_usd` | `sum_input_tokens` |
  `sum_output_tokens` | `sum_cached_tokens` | `sum_tokens` |
  `avg_duration_ms` | `p95_duration_ms`. Optional filters `agentName`,
  `provider`, `model`, `from`, `to` (RFC3339). Returns
  `{rows: [{key, value, count}, …]}`.
- `GET /api/v1/provider-calls/discover?namespace=X` — distinct provider
  and model values that appear in this namespace's `provider_calls`
  rows. Returns `{providers: […], models: […]}`.
- Dashboard proxy routes
  `GET /api/workspaces/{name}/provider-calls/aggregate` and
  `GET /api/workspaces/{name}/provider-calls/discover`. The workspace
  name is pinned as `namespace` on the forwarded query so callers
  cannot read another workspace's data.

Together these replace direct Prometheus reads for cost/usage views
(`useAgentCost`, `useProviderMetrics`).

### Added (eval-results aggregate + discover endpoints)

- `GET /api/v1/eval-results/aggregate?namespace=X&groupBy=Y&metric=Z` —
  namespace-scoped GROUP BY over `eval_results`. `groupBy` is one of
  `eval_id` | `eval_type` | `agent` | `time:hour` | `time:day`; `metric`
  is one of `count` | `avg_score` | `p50_score` | `p95_score` |
  `avg_latency_ms` | `p95_latency_ms`. Optional filters `agentName`,
  `evalId`, `evalType`, `from`, `to` (RFC3339). Returns
  `{rows: [{key, value, count}, …]}`.
- `GET /api/v1/eval-results/discover?namespace=X` — distinct
  `(eval_id, eval_type)` pairs plus distinct `agent_name` and
  `promptpack_name` values that appear in this namespace's
  `eval_results`. Returns
  `{evals: [{evalId, evalType}, …], agents: […], promptpacks: […]}`.
  The `agents` / `promptpacks` fields were added as part of the
  use-eval-filter migration; the proxy route's shape is unchanged
  (existing callers reading `body.evals` keep working).
- Dashboard proxy routes: `GET /api/workspaces/{name}/eval-results/aggregate`
  and `GET /api/workspaces/{name}/eval-results/discover`. The workspace
  name from the URL is pinned as `namespace` on the forwarded query so
  callers cannot read another workspace's data.

Together these replace direct Prometheus reads for product-class
dashboard hooks (eval trends, eval discovery). See CLAUDE.md →
Observability Boundaries for the principle; the design proposal at
`docs/local-backlog/implemented/2026-04-17-observability-split-design.md`
covers rationale.

### Breaking (SessionRetentionPolicy schema flatten + policyRef, #1016)

- `SessionRetentionPolicy.spec.default` and `spec.perWorkspace` removed.
  Fields promoted to `spec.*`:
  - `spec.hotCache` (was `spec.default.hotCache`)
  - `spec.warmStore` (was `spec.default.warmStore`)
  - `spec.coldArchive` (was `spec.default.coldArchive`)
- `Workspace.spec.services[].session.retention` removed (and the
  `SessionRetentionConfig` type deleted). Workspaces now reference a
  `SessionRetentionPolicy` via `services[].session.policyRef`
  (`*corev1.LocalObjectReference`). Many workspaces may share one
  policy. A workspace with no `policyRef` falls back to the session-api's
  baked-in defaults.
- `pkg/servicediscovery.SessionConfig.WarmDays` deleted — the bespoke
  primitive was set in `ResolveSessionConfig` but never read by any
  consumer (dead plumbing). Operators who relied on per-workspace warm
  retention express it on a referenced `SessionRetentionPolicy` instead.
- The `SessionRetentionPolicy` controller no longer tracks per-policy
  workspace resolution: `resolveWorkspaces`, `findPoliciesForWorkspace`,
  and the cross-Watches registration are removed. The `WorkspacesResolved`
  status condition stays for backward observability and always reports
  `Reason=NotApplicable`.
- The retention ConfigMap projected by the controller (`retention.yaml`)
  also flattens — no more `default:` / `perWorkspace:` keys; just
  `hotCache:`, `warmStore:`, `coldArchive:` at the top level.

- Migration for clusters with existing nested-shape policies — re-author
  one policy per workspace, then update each Workspace to reference it:

  ```bash
  # 1. Inspect existing policies (manual; the shape varies per operator).
  kubectl get sessionretentionpolicies -o yaml > /tmp/old-session-policies.yaml

  # 2. Author one SessionRetentionPolicy per workspace using the flat
  #    shape (no spec.default wrapper, no spec.perWorkspace map).
  kubectl apply -f my-workspace-session-policy.yaml

  # 3. Patch each Workspace to reference the policy.
  kubectl patch workspace my-workspace --type=merge -p '
    {"spec":{"services":[{"name":"default","session":{"policyRef":{"name":"my-workspace-session-policy"}}}]}}'
  ```

- This completes the cleanup pattern established in #1018 (MemoryPolicy)
  for the second of the two retention-policy CRDs. `SessionPrivacyPolicy`
  already uses the `policyRef` pattern; all three policy CRDs are now
  consistent.

### Added (memory-entity tier field, #1017)

- `GET /api/v1/memories`, `/api/v1/memories/search`, `/api/v1/memories/export`,
  `/api/v1/institutional/memories`, and `/api/v1/agent-memories` now return a
  `tier` field on each memory row (`"institutional" | "agent" | "user"`),
  derived from the scope map (`user_id` → `user`, `agent_id` without `user_id`
  → `agent`, neither → `institutional`). No schema change; additive on the
  JSON response. Mirrors the SQL CASE expression used by the
  `groupBy=tier` branch of `/memories/aggregate` (#1004).

### Breaking (MemoryPolicy schema flatten + tier-precedence)

- `MemoryPolicy.spec.default` and `spec.perWorkspace` removed. Fields
  promoted to `spec.*`:
  - `spec.tiers` (was `spec.default.tiers`)
  - `spec.consentRevocation` (was `spec.default.consentRevocation`)
  - `spec.supersession` (was `spec.default.supersession`)
  - `spec.schedule` (was `spec.default.schedule`)
  - `spec.batchSize` (was `spec.default.batchSize`)
- `Workspace.spec.services[].memory.retention` removed (and the
  `WorkspaceMemoryRetentionConfig` type deleted). Workspaces now
  reference a `MemoryPolicy` via `services[].memory.policyRef`
  (`*corev1.LocalObjectReference`). Many workspaces may share one
  policy. A workspace with no `policyRef` falls back to the baked-in
  `LegacyIntervalPolicy`.
- New: `MemoryPolicy.spec.tierPrecedence` configures per-tier ranking
  multipliers via a sibling-presence-dispatched union. Today the only
  ranker is `multiplicative`:

  ```yaml
  spec:
    tierPrecedence:
      multiplicative:
        institutional: "1.5"
        agent:         "1.0"
        user:          "1.0"
  ```

  The retrieval service consults the `MemoryPolicy` bound to the
  workspace and applies the per-tier multiplier inside `rankResults`
  (after the existing confidence/frequency/recency formula). Future
  ranker types (e.g. hard precedence) ship as new sibling fields on
  `TierPrecedenceConfig` plus a widened CEL rule. Existing
  `multiplicative` manifests keep validating.

- The `K8sPolicyLoader` flips from cluster-wide List-and-pick-default
  to workspace-driven Get: read `Workspace` by `--workspace` flag,
  walk to the named `--service-group`, follow `policyRef.Name` to the
  named `MemoryPolicy`. Any miss returns nil so the existing fallback
  to `LegacyIntervalPolicy` stands.

- Migration for clusters with existing `MemoryPolicy` instances using
  the nested shape — re-author one policy per workspace, then update
  each Workspace to reference it:

  ```bash
  # 1. Inspect existing policies (manual; the shape varies per operator).
  kubectl get memorypolicies -o yaml > /tmp/old-policies.yaml

  # 2. Author one MemoryPolicy per workspace (no cluster-wide override
  #    map; the workspace owns the binding via policyRef).
  kubectl apply -f my-workspace-policy.yaml

  # 3. Patch each Workspace to reference the policy.
  kubectl patch workspace my-workspace --type=merge -p '
    {"spec":{"services":[{"name":"default","memory":{"policyRef":{"name":"my-workspace-policy"}}}]}}'
  ```

- Migration for `Workspace.services[].memory.retention.defaultTTL`
  users — author a `MemoryPolicy` with `spec.tiers.user.ttl.default`
  set to the old TTL and bind it via `policyRef`. The bespoke
  `retention` block no longer validates.

- The flatten + cleanup unblocks a follow-up to apply the same shape
  to `SessionRetentionPolicy` (tracked in #1016).

### Added (memory aggregate tier groupBy, #1004)

- `GET /api/v1/memories/aggregate` now accepts `groupBy=tier`, returning
  rows keyed by `institutional` / `agent` / `user`. Tier is derived from
  the existing `virtual_user_id` and `agent_id` columns; no schema change.
  The user-tier count reflects only memories owned by users who granted
  `analytics:aggregate` consent (institutional and agent rows have no
  `virtual_user_id` and pass through the consent filter).

### Breaking (CRD rename, no GitHub issue)

- `MemoryRetentionPolicy` CRD renamed to `MemoryPolicy`. The schema is
  identical; only the kind / plural / shortname change:
  - `kind: MemoryRetentionPolicy` → `kind: MemoryPolicy`
  - plural `memoryretentionpolicies` → `memorypolicies`
  - shortname `mrp` → `mp`
- Migration for clusters with existing instances:

  ```bash
  kubectl get memoryretentionpolicies -o yaml \
    | sed 's/MemoryRetentionPolicy/MemoryPolicy/g; s/memoryretentionpolicies/memorypolicies/g' \
    | kubectl apply -f -
  kubectl delete crd memoryretentionpolicies.omnia.altairalabs.ai
  ```

- The rename frees up `MemoryPolicy` as the natural home for upcoming
  per-workspace memory configuration knobs (next: tier-precedence
  ranking weights for multi-tier retrieval).

### Added (memory consent classifier, #1005)

- `memory_entities.consent_category` is now populated automatically on EE
  deployments via the new content classifier (`ee/pkg/privacy/classify`):
  PII regex + optional embedding similarity, upgrade-only validator. No
  schema change. OSS deployments unaffected — column stays `NULL`.
- `SaveMemoryRequest.Category` is now propagated into
  `mem.Metadata[consent_category]` in the handler (was a silent dropped
  field). EE middleware classification results now reach the column.

### Added (per-session consent grants, #1006)

- WebSocket: `ClientMessage.session_consent_grants` (`[]string`, optional).
  First message with a non-empty list stamps the per-session default on the
  Connection. Subsequent messages with a non-empty list replace the cached
  value (last-writer-wins). Empty / omitted lists are ignored — to revoke
  all categories for a session use the binary opt-out.
- gRPC: `x-omnia-consent-layer` metadata key carries which layer
  (`per-message` | `session` | `persistent`) produced the per-request grants
  forwarded by the facade.
- Memory API: `X-Consent-Layer` request header (forwarded by the runtime
  httpclient). `X-Consent-Decision` response header on 204 dropped writes.
  New `omnia_memory_writes_suppressed_total{layer,category,reason}`
  Prometheus metric. Suppressed-write log line promoted from V(1) to V(0)
  and enriched with layer + grants.

### Added (memory analytics backend, #1004)

- `GET /api/v1/memories/aggregate?workspace=X&groupBy={category|agent|day}&metric={count|distinct_users}` —
  workspace-scoped aggregate over `memory_entities`. Composes the
  Phase D `analytics:aggregate` consent filter so users without that
  grant are excluded by construction. Optional `from`/`to` (RFC3339)
  for time bounds; `limit` clamped to [1, 1000]. Returns
  `[{key, value, count}]` matching the eval-results aggregate pilot
  shape from `docs/local-backlog/2026-04-17-observability-split-design.md`.
- `GET /api/v1/privacy/consent/stats?workspace=X` (EE only) —
  workspace-wide consent posture: `totalUsers`, `optedOutAll`,
  `grantsByCategory`. Workspace param reserved for future per-workspace
  scoping; currently a no-op (preferences table is user-keyed).

### Added (analytics:aggregate enforcement foundation, #1007)

- New `memory.AggregateConsentJoin(alias)` helper produces SQL JOIN +
  WHERE fragments restricting a cross-user aggregate to users who have
  granted `analytics:aggregate`. Institutional and agent-tier rows pass.
- New Prometheus metrics:
  - `omnia_memory_consent_analytics_optin_ratio` (gauge, 0..1)
  - `omnia_memory_consent_analytics_users_total{granted}` (gauge pair)
  - `omnia_memory_consent_analytics_worker_errors_total{reason}` (counter)
- `AnalyticsOptInWorker` queries `user_privacy_preferences` every 5
  minutes to refresh the gauges.
- No existing queries retrofitted — memory-api has zero cross-user
  aggregation today. The helper lands as foundation for the upcoming
  #1004 dashboard.

### Breaking

- `SessionPrivacyPolicy.spec.level`, `spec.workspaceRef`, and `spec.agentRef` removed. Policies are now reusable namespaced documents; binding has moved to consumers (`Workspace` service groups and `AgentRuntime`).
- `SessionPrivacyPolicy` is now **namespace-scoped** (was cluster-scoped).

### Added (podOverrides, #844)

- New shared `PodOverrides` type (`api/v1alpha1/shared_types.go`) with pod-level fields (`serviceAccountName`, `labels`, `annotations`, `nodeSelector`, `tolerations`, `affinity`, `priorityClassName`, `topologySpreadConstraints`, `imagePullSecrets`) and container-level fields (`extraEnv`, `extraEnvFrom`, `extraVolumes`, `extraVolumeMounts`).
- `AgentRuntime.spec.podOverrides` — facade + runtime Pod customization. Container-level fields apply to both user containers but skip operator-injected sidecars (e.g. `policy-proxy`).
- `AgentRuntime.spec.evals.podOverrides` — namespace-level eval-worker customization (last-writer-wins, matches existing eval-worker semantics).
- `Workspace.spec.services[].session.podOverrides` — managed session-api Pod customization.
- `Workspace.spec.services[].memory.podOverrides` — managed memory-api Pod customization.
- `ArenaJob.spec.workers.podOverrides` — worker Job Pod customization.
- `ArenaDevSession.spec.podOverrides` — dev-console Pod customization.

All fields optional; default rendering is byte-identical to before. Existing hooks (`FacadeConfig.ExtraEnv`, `RuntimeConfig.NodeSelector`/`Tolerations`/`Affinity`/`Volumes`/`VolumeMounts`/`ExtraEnv`, `AgentRuntimeSpec.ExtraPodAnnotations`) are preserved and applied first; PodOverrides values are merged or appended after.

### Added

- `Workspace.spec.services[].privacyPolicyRef` (`LocalObjectReference`) — selects the `SessionPrivacyPolicy` applied to sessions managed by that service group's session-api.
- `AgentRuntime.spec.privacyPolicyRef` (`LocalObjectReference`) — per-agent override that takes precedence over the service group policy.
- `PrivacyPolicyResolved` status condition on `Workspace` and `AgentRuntime`. Reason values:
  - `PolicyResolved` — a named policy was found and is active.
  - `PolicyNotFound` — the `privacyPolicyRef` points to a missing policy.
  - `DefaultPolicy` — no service group in the workspace sets a ref; the global default (or none) will be used.
  - `WorkspaceDefault` — the AgentRuntime has no override ref; effective policy comes from the service group or global default.

### Added (skills, #806)

- New `SkillSource` CRD (core, namespace-scoped). Syncs AgentSkills.io content from Git/OCI/ConfigMap into the workspace content PVC, with a post-fetch `filter` block (globs + explicit names). Status conditions: `SourceAvailable`, `ContentValid`.
- `PromptPack.spec.skills[]` (`SkillRef` array): each entry references a `SkillSource` in the pack's namespace and optionally narrows it via `include` and renames the group via `mountAs`.
- `PromptPack.spec.skillsConfig`: tunes PromptKit's skill runtime (`maxActive`, `selector`).
- `PromptPack` status conditions: `SkillsResolved`, `SkillsValid`, `SkillToolsResolved`.
- AgentRuntime runtime container: read-only mount of the workspace content PVC at `/workspace-content` and `OMNIA_PROMPTPACK_MANIFEST_PATH` env var, when the operator's `--workspace-content-path` flag is set (default `/workspace-content`).

---

## 2026-04-12
- **Session API**: Added `GET /api/v1/privacy-policy?namespace={ns}&agent={agent}` endpoint
  - Returns the facade-visible subset of the effective `SessionPrivacyPolicy`: `{"recording":{"enabled":bool,"facadeData":bool,"richData":bool}}`.
  - Responds with 200 + JSON when a policy applies, or 204 No Content when no policy applies.
  - Consumed by the facade at WebSocket-connect time (cached 60s per connection) to decide whether to record.
- **Session API**: Behavior change on recording write endpoints — `POST /api/v1/sessions/{id}/messages`, `POST /api/v1/sessions/{id}/tool-calls`, `POST /api/v1/sessions/{id}/provider-calls`, and `POST /api/v1/sessions/{id}/events`
  - When the effective `SessionPrivacyPolicy.Recording.Enabled` is `false`, these endpoints now return **204 No Content** and drop the write.
  - When `SessionPrivacyPolicy.Recording.RichData` is `false`, the middleware blocks assistant messages, tool-call, runtime-event, and provider-call writes (204). User messages, status updates, and TTL refreshes are still accepted.
  - **Non-breaking** for deployments without a `SessionPrivacyPolicy` — default behavior is unchanged (recording enabled, rich data allowed).
- **Note**: Session data at-rest encryption infrastructure is in place (`ee/pkg/encryption` extended for `ToolCall`/`RuntimeEvent`; `KeyRotationReconciler` wired) but session-api integration is pending — see follow-up issue. The current `SessionPrivacyPolicy.Encryption` wiring assumed a single global policy at startup, which is incorrect for multi-workspace deployments; a CRD redesign that makes `SessionPrivacyPolicy` a reusable policy referenced by `Workspace`/`AgentRuntime` is required before shipping encryption.

## 2026-04-07
- **Session API**: Added `cohortId` and `variant` fields to `CreateSessionRequest` and `Session` schemas
  - Supports rollout cohort tracking: Istio routes set `x-omnia-cohort-id` and `x-omnia-variant` headers,
    which the facade extracts and persists to the session for per-variant analysis.
  - New Postgres columns `cohort_id` and `variant` with partial indexes (migration 000025).
  - New OTel span attributes `omnia.cohort.id` and `omnia.variant` on `omnia.facade.message` spans.
  - Non-breaking: both fields are optional with `omitempty`.

## 2026-03-28
- **ArenaJob CRD** (Enterprise): Added `spec.sessionRecording` boolean (default: false)
  - **Breaking**: Session recording was previously always enabled when `SESSION_API_URL` was configured on the controller. Now requires explicit `sessionRecording: true` on the ArenaJob spec. Existing ArenaJobs without this field will stop recording sessions after upgrade.
  - **Migration**: Add `sessionRecording: true` to any ArenaJob that needs session transcripts.

## 2026-03-26
- **ArenaJob CRD** (Enterprise): Replaced `LoadTestSettings` and added `spec.trials` for load testing support (#661)
  - **Added** `spec.trials` — number of times to repeat each scenario × provider combination
  - **Replaced** `spec.loadTest` — old fields (`rampUp`, `duration`, `targetRPS`) replaced with scenario-driven model:
    - `concurrency` — max in-flight work items globally
    - `vusPerWorker` — concurrent goroutines per worker pod
    - `ramp` — linear ramp-up/down configuration (`up`, `down` duration strings)
    - `budgetLimit` / `budgetCurrency` — cost safety limit
    - `rateLimits` — per-provider concurrency caps
    - `thresholds` — SLO pass/fail gating (metric, operator, value)
  - **Added** `RampConfig`, `ProviderRateLimit`, `LoadThreshold`, `LoadThresholdMetric`, `LoadThresholdOperator` types
  - Old `LoadTestSettings` fields were never wired up, so this is a safe breaking change

## 2026-03-17
- **ArenaJob CRD** (Enterprise): Replaced provider/tool override pipeline with CRD-based provider groups
  - **Added** `spec.providers` — map of group names to lists of `ArenaProviderEntry` (providerRef or agentRef)
  - **Added** `spec.toolRegistries` — list of ToolRegistry CRD refs by name
  - **Removed** `spec.providerOverrides` — label-selector-based provider matching
  - **Removed** `spec.toolRegistryOverride` — label-selector-based tool registry matching
  - **Removed** `spec.execution` — fleet mode is replaced by `agentRef` entries in provider groups
  - **Removed** `ExecutionMode`, `ExecutionConfig`, `FleetTarget`, `ProviderGroupSelector`, `ToolRegistrySelector` types
  - Agents and LLM providers are now interchangeable in the scenario × provider matrix
  - Deleted `ee/pkg/selector/` package (no longer needed)

## 2026-03-15
- **Session API**: Fixed `eval_results.session_id` type from TEXT to UUID (migration 000020)
  - Added `(session_id, message_id)` composite index for faster lookups
  - Removed `::text` cast from cascade delete trigger
  - Eval events from runtime now written to `runtime_events` instead of messages
- **Session API**: Added structured multi-modal fields to `Message` schema (`hasMedia`, `mediaTypes`)
  - Migration `000019_structured_multimodal` adds `has_media`/`media_types` to messages and queryable columns to message_artifacts
  - Enables queries like "sessions with voice input" without parsing JSON metadata
- **Session API**: Added runtime events endpoints (`POST/GET /api/v1/sessions/{sessionID}/events`)
  - New `RuntimeEvent` schema in OpenAPI spec
  - Stores PromptKit lifecycle events (pipeline, stage, middleware, validation, workflow) in dedicated `runtime_events` table instead of as system messages
  - Migration `000018_create_runtime_events` creates partitioned table
- **Session API**: Added OpenAPI 3.0 spec at `api/session-api/openapi.yaml` covering all 18 endpoints
  - Generated Go client: `pkg/sessionapi/` (via oapi-codegen v2.4.1)
  - Generated TS types: `dashboard/src/lib/api/session-api-schema.d.ts` (via openapi-typescript)
  - Makefile targets: `generate-session-api-client`, `generate-session-api-types`, `validate-session-api-spec`
- **WebSocket protocol**: Removed `execution` field from `ToolCallInfo`
  - Go struct: `internal/facade/protocol.go:ToolCallInfo`
  - Reason: All tool calls sent over WebSocket are client-side by definition. Server-side tool calls are filtered at the facade and never forwarded.
  - Generated types updated: `dashboard/src/types/generated/websocket.ts`

## 2026-03-14
- **WebSocket protocol**: Added `tool_result` client message type and `ClientToolResultInfo` struct
  - Go struct: `internal/facade/protocol.go:ClientToolResultInfo`
  - Reason: Client-side tool execution (#617) requires the dashboard to send tool results back
- **gRPC protocol**: Removed `tool_result` from `ServerMessage` oneof
  - Proto: `api/proto/runtime/v1/runtime.proto`
  - Reason: Server-side tool results are handled internally by the runtime, not sent to the facade
