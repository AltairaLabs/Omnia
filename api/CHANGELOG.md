# API Surface Changelog

Changes to any API surface (REST, gRPC, WebSocket) should be logged here
so that parallel workstreams have visibility into contract changes.

When modifying files in `internal/session/api/`, `internal/facade/protocol.go`,
or `api/proto/`, add an entry below with the date, affected API, and reason.

---

## Unreleased

### Added (custom-runtime Wave 3b: RuntimeHello + bounded media counter-offer, §4.2–4.3)

- **Contract version 1.2.0 → 1.3.0.** Additive `omnia.runtime.v1` change (new oneof
  variant), bumped in both `api/proto/runtime/v1/runtime.proto` and
  `pkg/runtime/contract/version.go`.
- **New gRPC message (runtime → facade).** `RuntimeHello` is added as `ServerMessage`
  oneof field 8. The runtime sends it as the **first** `ServerMessage` on every
  `Converse` stream: the session's authoritative `capabilities`, plus (for a duplex
  session) a `MediaNegotiation` counter-offer (`codec` / `sample_rate` / `channels`;
  `frame_rate` / `resolution` carried but not yet enforced). A runtime that never sends a
  hello is treated as legacy — the facade keeps today's unilateral `DuplexStart`
  behaviour. Additive; existing clients that ignore the new variant are unaffected.
- **Counter-offer source (CRD).** `spec.duplex.audio` (`AudioRequirements`:
  `recommendedSampleRate` / `channels` / `format`) declares the required duplex audio
  format; the runtime advertises it in `RuntimeHello.media` and prefers it over the
  client's proposal.
- **New WebSocket message (facade → client).** `session_config` (`SessionConfigInfo`:
  `codec` / `sample_rate` / `channels`) relays the counter-offer; the client (re)captures
  at that format. See `api/websocket/asyncapi.yaml`.
- **New client-visible error.** `UNSATISFIABLE_FORMAT` is emitted when the counter-offer
  requires an unsupported (video) duplex format on this audio-only path — the session
  fails closed before audio flows.

### Changed (resume: the context store decides resumability, #1876)

- **Contract version 1.0.0 → 1.1.0.** Adding `HasConversation` is an additive change to
  the `omnia.runtime.v1` contract, so the minor version is bumped in both
  `api/proto/runtime/v1/runtime.proto` and `pkg/runtime/contract/version.go`. A custom
  runtime built against 1.0.0 remains conformant — it simply does not serve the new
  method, and the facade treats an unimplemented probe as `UNAVAILABLE`, which is never
  reported to a client as an expiry.
- **New gRPC method.** `RuntimeService.HasConversation(HasConversationRequest) returns
  (HasConversationResponse)` reports whether a session's working context can still be
  resumed. It answers `RESUME_STATE_RESUMABLE` / `RESUME_STATE_NOT_FOUND` /
  `RESUME_STATE_UNAVAILABLE`. Additive — existing clients are unaffected.
- **BEHAVIOR CHANGE (WebSocket).** The facade previously decided whether a session could
  be resumed by asking **session-api**, which cannot know: a row proves a conversation
  once existed, not that its turns survive in the context store. A client naming a
  session whose context had expired was told the session resumed and then answered with
  **no history at all** — silent amnesia. The facade now asks the runtime, which owns
  the context store, and the answer is exact: `HasConversation` performs the same state
  store load that `sdk.Resume` performs.
- **New client-visible error.** A resume request for a session with no surviving context
  is answered with **`SESSION_EXPIRED`** and the message is dropped rather than answered
  without history. Clients should retry with no `session_id`, which starts a new session.
  The error code already existed in the protocol but was never emitted. An unreachable
  context store yields `INTERNAL_ERROR`, never `SESSION_EXPIRED` — an unreachable store
  is a server fault, and reporting it as expiry would discard an intact conversation.
  Only a session id *differing* from the one the connection announced in `connected` is
  treated as a resume request, so a client echoing its own session id on the first
  message is unaffected.
- session-api is no longer read on the message path. `GetSession` as the resume oracle,
  the `RefreshTTL` call in `ensureSession`, and the terminal-status pre-read before the
  completion write are all removed — the last was redundant because the warm store
  already refuses to overwrite a terminal status and suppresses the duplicate event.
- **Session status fix.** A parked realtime session that expired without reattaching
  never reached a terminal status, so it stayed `active` in the archive forever.
  Connection cleanup deliberately skips completion while a session is parked (it may
  yet resume); park expiry is the point at which it definitively did not, and now
  writes `completed` there.
- `spec.context.ttl` now reaches the context store (`statestore.WithTTL` /
  `WithMemoryTTL`), which it never did — the value was parsed and dropped at store
  construction, so the store's own default always applied. It is also no longer read
  into the facade's session TTL, where it governed session-api row expiry; one CRD
  field no longer sets two unrelated lifetimes. Archival retention belongs to
  `SessionRetentionPolicy`. The facade no longer sends any TTL to session-api.

### Added (operator: deploy-intent API, deploy-intent decoupling epic Plan A)

- **`POST /api/v1/workspaces/{workspace}/deployments`** — new operator-served REST endpoint
  (`internal/api/deploy`) that accepts a versioned, CRD-agnostic **`DeployIntent`**
  (`apiVersion: deploy.omnia.altairalabs.ai/v1`) and translates it server-side into a
  `PromptPack` + its content `ConfigMap` and one or more `AgentRuntime` objects, applied
  idempotently (pack/ConfigMap created once; existing objects reported `unchanged`) and
  rollout-aware (an `AgentRuntime` already in version-trigger rollout mode keeps its live
  `promptPackRef`/candidate/rollout state unless the intent explicitly supplies a new
  rollout block). Response is **`DeployResult`**: `{succeeded, results: [{kind, name,
  action: created|updated|unchanged|failed, error?}]}` — HTTP 200, or 207 on partial
  failure. Auth matches the content API: dashboard-minted management-plane JWT verified
  against `--mgmt-plane-jwks-url`, editor role required on the target workspace
  (`internal/api/authz`). Gated behind the new `--deploy-api-bind-address` operator flag
  (empty disables it; requires `--mgmt-plane-jwks-url`).
- This is **Plan A** of the deploy-intent decoupling epic (#1835 family, supersedes
  #1839): it lets a future deploy adapter submit one intent instead of authoring
  `PromptPack`/`AgentRuntime` CRDs itself. The existing `promptarena-deploy-omnia` adapter
  has **not** migrated to this endpoint yet — it still writes CRDs directly via the
  dashboard's workspace CRD REST API (unchanged).

### Added (operator: deploy-intent API full config surface, deploy-intent decoupling epic Plan B, #1865)

- The `DeployIntent` fields declared as wire placeholders in Plan A are now mapped by the
  translator: `agents[].externalAuth` → `AgentRuntime.spec.externalAuth` (current CRD
  vocabulary — `clientKeys`/`oidc`/`edgeTrust`, not the adapter's legacy
  `sharedToken`/`apiKeys` shape), `agents[].memory` → `spec.memory`, and `agents[].evals` →
  `spec.evals` (enabled + `inlineGroups`/`workerGroups` only — sampling/rateLimit/
  sessionCompletion are not yet in the intent contract).
- **`tools`** (top-level `ToolsIntent`) either references an existing `ToolRegistry` by
  name (`ref`) or **create-only** creates one from `handlers[]` (`ref` and `handlers` are
  mutually exclusive) — the operator never updates an existing `ToolRegistry` through this
  endpoint. Each `HandlerIntent`'s per-executor config block (`httpConfig`/`openAPIConfig`/
  `grpcConfig`/`mcpConfig`/`clientConfig`/`auth`/`tool`) is carried as free-form JSON, mapped
  straight onto the `ToolRegistry` CRD's matching field, so the intent contract tracks new
  executor types without a schema change.
- **`policy.toolBlocklist`** (top-level `PolicyIntent`) creates an `AgentPolicy` with a
  `toolAccess` denylist rule scoped to the deploy's `ToolRegistry` — the CRD has no
  `toolBlocklist` field; the server does the shape correction.
- `api/openapi/openapi.yaml` schemas for `ExternalAuthIntent`, `MemoryIntent`, `EvalsIntent`,
  `ToolsIntent`/`HandlerIntent`, and `PolicyIntent` are updated to the real fields (no new
  `paths:` entry — this remains an internal, mgmt-plane-JWT-authenticated endpoint; a
  dashboard-facing proxy route is deferred to a later plan).

### Added (dashboard: deploy-intent proxy + deploy-profile version advertisement, deploy-intent decoupling epic Plan C, #1866)

- **`GET /api/workspaces/{name}/deploy-profile`** response gains **`supportedDeployIntentVersions`**
  (`string[]`, required) — the `DeployIntent` `apiVersion` values the operator's deploy-intent
  API (see Plan A entry above) accepts, currently `["deploy.omnia.altairalabs.ai/v1"]`
  (`dashboard/src/lib/deploy/intent-versions.ts`, a hand-kept mirror of the Go
  `deploy.APIVersionV1` constant). Lets a deploy client version-negotiate before POSTing an
  intent. `api/openapi/openapi.yaml`'s `DeployProfile` schema updated to match.
- **`POST /api/workspaces/{name}/deployments`** — new dashboard-served REST endpoint
  (`dashboard/src/app/api/workspaces/[name]/deployments/route.ts`), editor-gated via
  `withWorkspaceAccess`, that forwards an opaque `DeployIntent` request body to the operator's
  `POST /api/v1/workspaces/{workspace}/deployments` (Plan A entry above). `deploy-api-service.ts`
  mints a short-lived RS256 identity JWT (aud `omnia-operator`, 60s TTL) via the shared
  `operator-identity.ts` helper — the same minting path `content-api-service.ts` already uses —
  and returns the operator's `DeployResult` response (200, or 207 on partial failure) verbatim.
  The dashboard does not validate or interpret the intent body.
- This is **Plan C** of the deploy-intent decoupling epic (#1863 family): it makes the Plan
  A/B operator API reachable end-to-end for the first time. The chart now wires
  `--deploy-api-bind-address` (default `:8085`), a `deploy-api` container/Service port, and the
  dashboard's `OPERATOR_DEPLOY_API_URL` env var. The external `promptarena-deploy-omnia` adapter
  authenticates to this new dashboard route with its existing `omnia_sk_` key; the operator only
  ever sees the dashboard-minted JWT.

### gRPC — `omnia.runtime.v1`

- **Added a contract version.** `api/proto/runtime/v1/runtime.proto` now carries
  a `// Contract-Version:` marker (currently `1.0.0`), mirrored by the
  `contract.Version` constant in `pkg/runtime/contract/version.go` and asserted
  equal by `pkg/runtime/contract/version_test.go`. No message or RPC changed —
  this is documentation of the existing surface so third-party runtimes have
  something to pin and report. Minor bumps are additive; major bumps break
  conformant runtimes. (custom-runtime epic, wave 1)
- **Added `HealthResponse.contract_version`** (field 3, additive). The runtime
  now reports the `omnia.runtime.v1` contract version it was built against.
  Nothing reads this field yet in wave 1 — it lays the groundwork for a wave 3
  control-plane check that will detect a runtime that has fallen behind. An
  empty value means the runtime predates contract versioning. The contract
  version stays `1.0.0`: the marker and this field are landing together,
  unreleased, so `1.0.0` describes the contract including this field.
  (custom-runtime epic, wave 1; the capability feature-flag set follows in
  wave 3)

### Changed (auth: rename `apiKeys` → `clientKeys` + arbitrary per-key claims, #1775)

- **BREAKING (CRD + `pkg/facade` SDK).** The facade external-auth **`spec.externalAuth.apiKeys`** field is renamed to **`clientKeys`** (`APIKeysAuth` → `ClientKeysAuth`), disambiguating it from LLM-provider api-keys and the dashboard's own user api-keys. The origin surfaced to ToolPolicy is renamed `identity.origin == "api-key"` → **`"client-key"`** (`policy.OriginAPIKey` → `OriginClientKey`). In the public `pkg/facade/auth` SDK: `APIKey`/`APIKeyValidator`/`NewAPIKeyValidator`/`WithAPIKey*` → `ClientKey`/`ClientKeyValidator`/`NewClientKeyValidator`/`WithClientKey*`.
- **New:** each client key now carries an **arbitrary claim map** (not just a role). A key's stored claims surface to ToolPolicy as `identity.claims.*` (e.g. `identity.claims.tier == "premium"`), un-spoofable because they are bound to the key at creation. `defaultRole` is retained as a convenience that seeds `identity.claims.role` when a key sets no claims; its value is now free-form (the `viewer;editor;admin` enum is relaxed — roles are ordinary claims).
- The key Secret naming changed accordingly: label `agent-api-key` → `agent-client-key`, name suffix `-apikey-` → `-clientkey-`. Unrelated: LLM-provider `api-key` secrets and the dashboard's user api-keys are untouched.
- Removed the unused `pkg/facade/auth.WithEdgeTrustRoleHeader` SDK option: the edge-trust role header is a fixed internal default (`x-user-roles` → `identity.claims.role`) and is not operator-configurable, matching the CRD (`EdgeTrustHeaderMapping` has no `role` field). Custom header→claim routing goes through `claimsFromHeaders`.

### Changed (policy: propagate `identity.origin` and `identity.workspace` end-to-end, #1769)

- **BEHAVIOR CHANGE.** `identity.origin` and `identity.workspace` are now propagated
  from the facade through the runtime to the policy broker and are populated when a
  ToolPolicy CEL rule (or the `POST /v1/decision` broker) evaluates `identity.*`.
  Previously both fields were declared and exposed to CEL but **never propagated** —
  they were always empty at the broker, so any ToolPolicy rule keyed on
  `identity.origin` or `identity.workspace` silently no-oped (always matched the empty
  string). Such rules now see **real values** and will start denying/allowing as
  written. Audit existing ToolPolicies that reference these fields before upgrading.
- Wire additions: two new gRPC-metadata / HTTP propagation headers,
  **`x-omnia-origin`** (the admitting validator: `management-plane` / `shared-token` /
  `api-key` / `oidc` / `edge-trust`) and **`x-omnia-workspace`** (the target workspace).
  These are added to the outbound header set (`ToOutboundHeaders` / `ToGRPCMetadata`)
  and rehydrated by the runtime's policy interceptor. The broker's
  `DecisionRequest.Identity` (`IdentityPayload`) `origin` / `workspace` JSON fields
  already existed and are now populated on the production (flat-propagation) path.
- `identity.workspace` prefers the token's own workspace scope (set by workspace-scoped
  validators such as the management plane) and falls back to the agent's deployed
  workspace, so the field is non-empty for every validator style. This is distinct from
  the K8s `namespace` (`x-omnia-namespace`), which is unchanged.

### Removed (AgentRuntime CRD + ToolPolicy CEL: structured role, #1775)

- **`spec.externalAuth.oidc.claimMapping.role`** and
  **`spec.externalAuth.edgeTrust.headerMapping.role`** are removed from the
  `AgentRuntime` CRD. Roles are no longer a distinct, mapped identity field —
  they are an ordinary claim. The OIDC validator passes an IdP's role claim
  through unmapped under its own claim name (e.g. `identity.claims["omnia.role"]`
  if that's what the IdP sets — there is no longer an Omnia-imposed default
  role claim name for OIDC). The edge-trust validator still reads the inbound
  role header (default `x-user-roles`, the facade's built-in default —
  see `DefaultEdgeRoleHeader` in `internal/facade/auth/edge_trust.go`) but
  always into `identity.claims.role`; the header name is a fixed internal
  default, not configurable via `headerMapping`. api-key roles
  (`defaultRole` / per-key `role` on the Secret) are unaffected and also
  surface as `identity.claims.role`.
- **ToolPolicy CEL**: the structured `identity.role` field is gone from the
  `identity` object sent to the policy broker (now `origin`, `subject`,
  `endUser`, `workspace`, `agent`, `claims` — no `role`). Existing CEL rules
  referencing `identity.role` must be rewritten as `identity.claims.role`.
  Management-plane and shared-token identities remain role-less — gate on
  `identity.origin` instead of a role claim for those callers.

### Removed (AgentRuntime CRD: built-in sharedToken external auth, #1775)

- **`spec.externalAuth.sharedToken`** is removed from the `AgentRuntime` CRD.
  A single shared secret with one static identity is strictly worse than a
  client-scoped API key, and there were no production users of external auth
  to migrate. Use `apiKeys` instead — a single API key is the direct
  simple-integration replacement for what sharedToken provided. This does not
  affect `pkg/facade`'s `SharedTokenValidator`, which remains available as a
  building block for third-party custom facades built on the SDK.

### Changed (memory-api: scope param `user_id` → `virtual_user_id`, #1280)

- The memory per-subject scope is now named **`virtual_user_id`** on the wire, matching
  the `memory_entities.virtual_user_id` column and making it self-documenting that callers
  supply a **pseudonym**, never a real identity. Affects the query param on
  `GET/DELETE /api/v1/memories` (+ list/export/delete-all/batch-delete) and the `scope`
  map key in the `POST /api/v1/memories` (save/update/supersede) request body.
- **Transition window (non-breaking):** memory-api accepts **both** `virtual_user_id` and
  the legacy `user_id` for one release (`virtual_user_id` wins if both are sent), logging a
  deprecation when the legacy name is received. The legacy `user_id` name is dropped the
  release after. In-repo callers (facade memory httpclient, dashboard memory proxies) already
  send `virtual_user_id`.
- **Out of scope:** the typed `user_id` JSON body field on `POST /api/v1/memories/retrieve`
  and the compaction endpoints is a separate, internally-consistent client↔server contract
  and is unchanged.

### Added (arena-controller: license entitlements for memory/privacy/policy, #1682 Slice A)

- **`GET /api/v1/license`** response `features` object gains three boolean fields:
  `memoryEnterprise` (Memory Galaxy / analytics / institutional / multi-tier / consolidation),
  `privacyEnterprise` (consent / DSAR / audit-hub / enforcement-stats), and `policyProxy`
  (AgentPolicy / CEL enforcement). These are **off** in the open-core and any pre-existing
  Arena-only license (JSON absence → `false`), **on** in the dev license. This is the license-model
  half of #1682; no backend service enforces these entitlements yet, so the change is non-breaking —
  runtime gates in memory-api / privacy-api / policy-proxy land in follow-up slices. Mirrored in the
  dashboard `LicenseFeatures` type (`dashboard/src/types/license.ts`).

### Removed (session-api: DSAR request-lifecycle endpoints, #1676 Slice C)

- **`POST /api/v1/privacy/deletion-request`**, **`GET /api/v1/privacy/deletion-request/{id}`**,
  **`GET /api/v1/privacy/deletion-requests`** are **no longer served by session-api** — privacy-api
  is the sole owner of the DSAR request lifecycle (see the Slice B entry below). session-api keeps
  only `POST /api/v1/privacy/sessions/delete-by-user`, which privacy-api calls per service group.
  The session DB `deletion_requests` table is dropped (session migration `000004`). DSAR lifecycle
  events (`deletion_requested` / `_completed` / `_failed`) are now written to privacy-api's central
  `audit_log` by the orchestrator (#1678), so the audit trail is preserved.

### Added (privacy-api: DSAR orchestration endpoints, #1676 Slice B)

- **`POST /api/v1/privacy/deletion-request`**, **`GET /api/v1/privacy/deletion-request/{id}`**,
  **`GET /api/v1/privacy/deletion-requests?virtual_user_id=…`** now served by
  **privacy-api** (enterprise, behind SA auth). privacy-api owns the
  `deletion_requests` lifecycle and fans erasure out across every service-group:
  per group it calls session-api's `delete-by-user` (sessions + media) and
  memory-api's batch-delete (memories, scoped by workspace UID). The same routes
  remain on session-api during the transition; they are removed from session-api in
  Slice C. No request/response schema change — the contract matches the session-api
  origin handler.

### Added (session-api: session-tier DSAR erasure endpoint, #1676)

- **`POST /api/v1/privacy/sessions/delete-by-user`** (enterprise) — erases a
  subject's sessions and their media within this session-api's own group. Body:
  `{"virtual_user_id","workspace","date_from","date_to"}`; returns
  `{"sessions_deleted":N,"errors":[…]}`. Fails closed with 400 on an empty
  `virtual_user_id`. This is the session-tier half of DSAR (Phase 2, #1676):
  privacy-api will orchestrate this endpoint across all of a workspace's
  service-groups, so privacy-api needs no warm-store or object-storage access.
  The in-process `POST/GET /api/v1/privacy/deletion-request[s]` endpoints remain
  on session-api during the transition and move to privacy-api in a later slice.

### Added (privacy-api: central audit hub ingest endpoint, #1673)

- **`POST /api/v1/privacy/audit-events`** — ingests audit events forwarded from
  memory-api / session-api into privacy-api's central `audit_log` hub. Body:
  `{"sourceService":"<name>","events":[<audit.Entry>…]}` where `audit.Entry` is
  `github.com/altairalabs/omnia/ee/pkg/audit.Entry`. Ingest is idempotent on
  `(source_service, source_id)` (at-least-once delivery) and returns
  `{"ingested":N,"duplicates":M}`. This makes privacy-api the source of truth for
  the privacy/compliance audit slice: `GET /api/v1/privacy/enforcement-stats`
  now reads this table. memory-api and session-api each run a drain-forwarder
  (`ee/pkg/audit/forwarder.go`) that ships their local `audit_log` rows here.
- No new env var: each service reuses the privacy-api URL it already resolves
  for consent enforcement (`PRIVACY_API_URL` env, else Workspace CRD
  `status.privacyURL`) and the existing ServiceAccount token source for auth.

### Changed (memory-api: aggregate endpoint drops analytics:aggregate consent filter, #1642)

- **`GET /api/v1/memories/aggregate`** no longer composes the
  `analytics:aggregate` consent filter. All tiers (institutional, agent, user)
  are counted unconditionally. This is a product-signed-off privacy-posture
  change (2026-06-29): consent revocation is now enforced per-user via the
  event-driven `POST /api/v1/memories/consent-events` endpoint (CE1) rather
  than a JOIN-based sweep at query time. The analytics:aggregate consent
  category is owned by privacy-api going forward.
- The `groupBy=tier` `user` count previously reflected only memories owned by
  users who granted `analytics:aggregate`; it now reflects all user-tier rows.

### Added (memory-api: per-user consent-event prune endpoint, #1642 CE1)

- **`POST /api/v1/memories/consent-events`** — accepts a consent revocation
  event for a specific user (`{workspace_id, user_id, revoked_categories}`)
  and immediately prunes matching memories for that user. This is the
  event-driven replacement for the former JOIN-based consent sweep
  (`SoftDeleteRevokedConsent` / `HardDeleteRevokedConsent`). Returns 204 on
  success; 400 on missing required fields.

### Changed (CRD: `AgentRuntime.spec.facade` → `spec.facades` composition, #1576) — BREAKING

- `AgentRuntime` now composes a list of single-protocol facades: `spec.facade`
  (singular object) is replaced by `spec.facades` (required, non-empty, max 4,
  no duplicate types). Each entry is one protocol: `websocket`, `a2a`, `rest`,
  or `mcp`. Agent mode uses `websocket` and/or `a2a`; function mode uses `rest`
  (exactly one, required) plus optional `mcp` (CEL-enforced).
- **Removed** `spec.facade` (singular), the deprecated top-level `spec.a2a`,
  `spec.a2a.authentication` / `A2AAuthConfig`, the `grpc` facade type, and
  `spec.externalAuth.allowManagementPlane`. The A2A surface config (TTLs,
  clients, task store, agent card) now lives under the a2a facade entry's
  `a2a:` block; the MCP surface under the mcp facade entry's `mcp:` block.
- **Added** per-facade `facades[].managementPlane` (default true) gating each
  facade's internal management-plane twin — replaces the agent-global
  `allowManagementPlane`. `facades[].expose` (external HTTPRoute opt-in) is
  per-facade.
- The A2A agent card now advertises the external interface URL derived from the
  observed HTTPRoute (`status.facade.endpoints`, protocol=a2a), falling back to
  the in-cluster Service URL when no external route is observed (fixes #1576's
  wrong-URL bug). `status.managementEndpoints` is populated per-facade.
- Hard cutover, no projection shim (alpha). All in-repo manifests, the config
  loader, controller, facade, and dashboard moved to `spec.facades` together.

### Removed (session-api + CRD: `recording.richData` deprecated alias)

- **`GET /api/v1/privacy-policy`** no longer returns `recording.richData`; it was the
  deprecated alias for `recording.runtimeData`. Clients must read `recording.runtimeData`
  (gates runtime-emitted assistant message content only).
- The `SessionPrivacyPolicy` CRD's `recording.richData` field is removed — existing
  policies must use `recording.runtimeData`. (The unrelated `retention.richData` tier is
  unaffected.) Facade recording also moved onto a RuntimeClient gRPC bus interceptor, so
  the gate is read off `runtimeData` directly with no alias.

### Added (WebSocket: realtime blip-resume — `resume` query param + `connected.resumed` field)

- **`?resume=<session_id>` WebSocket connect query parameter**: clients that experience
  a transient network disconnect can re-attach to a parked realtime session by appending
  `?resume=<session_id>` to the WebSocket connect URL. The facade looks up the parked
  session and re-attaches the client without starting a new session. If the session is not
  found or has already expired, the facade falls back to opening a fresh session (identical
  to a cold connect).
- **`connected.resumed` boolean** (`internal/facade/protocol.go: ConnectedInfo`): new
  optional field on the server→client `connected` message. Set to `true` when the
  connection successfully re-attached to a parked realtime session via `?resume=`; absent
  or `false` on a normal cold connect. Clients should use this flag to decide whether to
  restore in-flight UI state (e.g. keep the audio buffer, preserve sequence counters) or
  reset to a fresh-session baseline.

### Added (WebSocket: `interrupt` server message + gRPC `Interruption` — realtime voice barge-in)

- **gRPC `Interruption` ServerMessage** (`pkg/runtime/v1`): new oneof variant emitted by the runtime when a barge-in is detected during a duplex audio session. The facade's `relayOut` loop handles it.
- **WebSocket `interrupt` message** (`internal/facade/protocol.go`): new server→client control message (`MessageTypeInterrupt = "interrupt"`). Signals the browser to clear its buffered audio output immediately. No payload beyond the standard `session_id` and `timestamp` fields.

### Changed (CRD: AgentRuntime memory retrieval strategy + denyCEL enforcement, #1513/#1514/#1515)

- **`AgentRuntime.spec.memory.retrieval.strategy`** enum: removed unimplemented
  **`graph`** (#1514); added **`composite`** (#1515) — RRF fusion of keyword +
  semantic legs. Enum is now `keyword | semantic | composite`. `graph` was a
  silent no-op (behaved as `keyword`), so nothing functional is lost.
- **`accessFilter.denyCEL`** is now enforced on the **keyword** path, not only
  semantic (#1513). This also closes a governance gap where `strategy: semantic`
  silently fell back to keyword (when the store had no semantic capability) and
  dropped the deny-filter. No schema change — behavioral hardening.

### Added (CRD: AgentRuntime independent memory toggles, #1517)

- **`AgentRuntime.spec.memory.retrieval.enabled`** (`*bool`) — gates ambient RAG
  auto-injection independently of the memory tools.
- **`AgentRuntime.spec.memory.tools`** (`MemoryToolsConfig{ enabled *bool }`) —
  gates the `memory__remember` / `memory__recall` tools independently of RAG.
- Both default to **true** when unset, so existing `memory.enabled: true` specs
  are unchanged. Combinations: RAG+tools (today), RAG-only, tools-only, or
  neither.
- Interim: with tools off the tools are still advertised to the LLM but backed by
  a no-op store (writes discarded, reads empty). PromptKit#1427 tracks a
  first-class option to skip tool registration entirely.

### Removed (CRD: AgentRuntime unwired memory fields, #1512)

- Removed **`AgentRuntime.spec.memory.embedding`** (`MemoryEmbeddingConfig`),
  **`.extraction`** (`MemoryExtractionConfig`), **`.retention`**
  (`MemoryRetentionConfig`), and **`.purpose`**. These fields had zero
  consumers — no controller or runtime read them. Embedding is configured at the
  **workspace** level (the embedding `Provider` CRD wired by `cmd/memory-api`);
  per-agent extraction/retention/purpose were never implemented.
- Retained and still wired: **`.enabled`** and **`.retrieval`**
  (`strategy`, `limit`, `accessFilter.denyCEL`).
- Breaking only in the sense that these keys are now rejected by validation on
  apply; since they did nothing, removing them from a spec is behavior-preserving.
- Follow-ups filed for the `retrieval.strategy` enum values that are accepted but
  not yet implemented (`graph`, `composite`).

### Changed (CRD: AgentRuntime function-mode facade type — `rest`, #1464)

- **`AgentRuntime.spec.facade.type`** gains a new enum value **`rest`**
  (`websocket|grpc|a2a|rest`) — an honest label for function-mode runtimes,
  which serve a one-shot HTTP endpoint at `POST /functions/{name}` rather than a
  persistent client connection.
- **`mode: function` now requires `facade.type` to be `rest` or `a2a`.** The CEL
  gate previously only rejected `websocket`, steering authors to label functions
  `grpc` (cosmetic, since the runtime ignores it). Both `websocket` and `grpc`
  are now rejected for function mode, with an error pointing at `rest`. A
  symmetric rule rejects `rest` outside function mode.
- **Breaking for function specs only:** an already-deployed `mode: function`
  AgentRuntime with `facade.type: grpc` fails validation on its next apply and
  must switch to `rest`. Agent-mode specs are unaffected (`grpc` still accepted
  for back-compat). In-repo samples, charts, and examples were migrated.
- Side effect: function Services are no longer mislabelled with Istio
  `appProtocol: grpc` on their HTTP `facade` port — `rest` maps to `http`.

### Fixed (REST: dashboard OpenAPI spec corrected to the real workspace API)

- **`api/openapi/openapi.yaml`** rewritten to document the API that is actually
  implemented. It previously described namespace-scoped `/api/v1/agents`,
  `/api/v1/promptpacks`, … endpoints served on the controller-manager `:8082`
  — **none of which exist**. The real, externally-exposed, authenticated API is
  the dashboard's workspace-scoped routes:
  - Paths are now `/api/workspaces/{name}/<resource>` (and `/{resourceName}` for
    items), matching `dashboard/src/app/api/workspaces/[name]/**`.
  - Added an `ApiKeyAuth` security scheme (`Authorization: Bearer omnia_sk_…` /
    `X-API-Key`) applied globally; `/api/health` is unauthenticated.
  - Documented methods now match the route handlers' real exports (e.g. Providers
    are GET/POST list + GET/PUT item; PromptPack create/update carry `content`,
    which the server folds into a `{name}-content` ConfigMap).
  - Dropped the unimplemented `/api/v1/namespaces` endpoint.
- **Drift guard:** `hack/check-openapi-routes.py` (wired into `hack/pre-commit`
  and a new `OpenAPI Route Drift` CI job) fails if the spec documents any
  path/method without a backing `dashboard/src/app/api/**/route.ts` handler, so
  the spec can never drift back into documenting a non-existent API.

### Added (CRD: AgentRuntime function output format)

- **`AgentRuntime.spec.outputFormat`** (enum `text|json|json_schema`, optional,
  CEL-gated to `mode: function`) — controls how the runtime constrains the
  model's response in function mode: `text` = free-form (validated post-hoc by
  the facade), `json` = provider JSON mode (valid JSON, shape unenforced),
  `json_schema` = provider structured output bound to `spec.outputSchema`. When
  unset on a function-mode runtime it defaults to `json_schema`. The runtime sets
  PromptKit `WithResponseFormat` accordingly; provider format errors propagate
  (fail-fast, no fallback). (#1483)
- **Migration:** existing function deployments on a provider/model without
  structured-output support will fail under the new `json_schema` default; set
  `outputFormat: text` (or `json`) explicitly, or use a provider that supports
  structured outputs.

### Added (CRD: AgentRuntime rollout zero-downtime promotion)

- **`AgentRuntime.status.rollout.promoting`** (bool) — set while a promotion is
  in progress: spec has advanced to the candidate config and the stable
  Deployment is rolling to it in the background, while the validated candidate
  keeps serving 100% of traffic. Cleared once stable is healthy on the new
  config and traffic has cut back. Makes promotion zero-downtime (no request is
  served from a cold/restarting stable pod). Consumers can surface a "promoting"
  state distinct from an active step rollout.
- The operator now also writes a per-agent `<agent>-canary-config` ConfigMap
  (candidate provider refs) mounted into candidate pods so the runtime resolves
  the candidate's providers rather than the shared stable spec (closes #1468).

### Added (runtime/facade: duplex audio transport)

New OSS low-latency bidirectional audio transport between the Facade WebSocket and the Runtime gRPC layer.

**gRPC (`api/proto/runtime/v1/runtime.proto`)**

- `ClientMessage.duplex_start` (`DuplexStart`) — sent as the first message of a `Converse` stream to switch it into duplex audio mode. Fields: `codec` (string, default `"pcm"`), `sample_rate` (int32, default `16000`), `channels` (int32, default `1`), `system_instruction` (string, optional).
- `ClientMessage.audio_input` (`AudioInputChunk`) — subsequent frames carry raw audio bytes (`data bytes`), `sequence uint32`, and `is_last bool`. `is_last` marks the **final frame of the call** and tears down the entire duplex session; producers MUST set it only on the true final frame.
- Audio output from the runtime reuses the **existing** `ServerMessage.media_chunk` (`MediaChunk`) message type — no new server-side message added.

**WebSocket (inbound binary)**

`BinaryMessageTypeMediaChunk` binary frames received from the browser during an active duplex session are routed to a per-connection `audioSession` (created lazily), which forwards them to a `facade.DuplexSink` backed by the runtime gRPC stream. See `api/websocket/asyncapi.yaml` for the binary frame contract and `is_last` semantics.

### Added (session-api: internal ServiceAccount auth — SEC-1/SEC-5)

- **All `GET`/`POST`/`PATCH`/`DELETE /api/v1/*` endpoints** now optionally
  require a Kubernetes **ServiceAccount bearer token** (`Authorization: Bearer
  <token>`), validated server-side via the TokenReview API against an allowlist.
  Opt-in via the chart value `internalServiceAuth.enabled` (off by default;
  closes SEC-1). `/healthz` is exempt. When disabled, behaviour is unchanged
  (no auth). Allowed-caller subjects are `system:serviceaccount:<ns>:<name>`
  set via `--auth-allowed-subjects` / `SESSION_API_AUTH_ALLOWED_SUBJECTS`;
  token audiences via `--auth-audiences` / `SESSION_API_AUTH_AUDIENCES`.
- **OTLP listeners** (gRPC `:4317` / HTTP `:4318`, `--otlp-enabled`) are gated
  by the same auth when enabled (SEC-5). OTLP senders must present an SA token;
  the default chart trace path (agents → alloy → Tempo) does **not** target
  session-api, so no token is wired there by default.
- Callers (facade, dashboard, memory-api, eval-worker) read their token from
  `SESSION_API_TOKEN_PATH` (default `/var/run/secrets/kubernetes.io/
  serviceaccount/token`; the chart/operator mount an audience-bound projected
  token at `/var/run/secrets/omnia/session-api/token`). Requests without a
  token while auth is enabled return `401`.

### Changed (session-api: review quick-wins)

- **`GET /api/v1/sessions/{id}`** (`handleGetSession`): now **decrypts** message
  content for encrypted sessions, matching `GET …/messages`. Previously this
  endpoint returned `enc:v1:` ciphertext blobs; consumers now receive plaintext
  (and a decryption failure returns 5xx instead of leaking ciphertext). [SEC-6]
- **`Message` DTO** (`pkg/sessionapi` conversions): `MessageToAPI` /
  `MessageFromAPI` now carry `costUsd`, `hasMedia`, and `mediaTypes` (previously
  silently dropped). No wire-schema change — the generated type already had the
  fields. [MAINT-7]

### Added (session-api: per-user session attribution — #1285)

- **`POST /api/v1/sessions`** (`CreateSessionRequest`): new **required** field
  `virtualUserId` (string) — the pseudonymous subject the session belongs to
  (`PseudonymizeID(subject)`, hash-at-rest). Requests with an empty/absent
  `virtualUserId` now return `400`. The `Session` response object also carries
  `virtualUserId`. Sessions are non-NULL attributed at the DB
  (`virtual_user_id TEXT NOT NULL CHECK (virtual_user_id <> '')`), so every
  session — browser, function, MCP, arena load-test, OTLP-ingested — has a
  virtual user.

### Changed (privacy/DSAR wire — #1285, #1280 session-side)

- **`POST /api/v1/privacy/deletion-request`** and
  **`GET /api/v1/privacy/deletion-requests`**: the request-body field `userId`
  and the query parameter `user_id` are renamed to `virtualUserId` /
  `virtual_user_id` (the value was always a pseudonym). Per-user session
  deletion now filters by `virtual_user_id` and **fails closed** — it returns an
  error rather than deleting all workspace sessions when no subject is supplied
  (previously the user scope was silently ignored). The opt-out/consent
  endpoints backed by `user_privacy_preferences` keep `user_id` for now
  (cross-DB shared table; deferred to #1280).

### Added (session-api: decorate session with tags/state)

- **`PATCH /api/v1/sessions/{sessionID}/decorate`** (new): merges tags and state
  into an existing session without touching counters or lifecycle status. Body
  `{"removeTags": ["source:interactive"], "tags": ["source:arena", ...], "state":
  {"arena.job": "..."}}` — `removeTags` are dropped first, then `tags` are added
  (idempotent), and state is shallow-merged. `200` on success, `404` when the
  session does not exist. Used by the arena worker to label the facade-recorded
  session of a load-test fleet run with arena context — replacing the
  `source:interactive` tag with `source:arena` so the run shows the real
  conversation + cost instead of an empty shell, without being double-counted as
  interactive user traffic. Also adds `DecorateSession` to the `session.Store` and
  `providers.WarmStoreProvider` interfaces.

### Added (memory-api: configurable embedding dimension, #1309)

- **`POST /admin/embedding-dimension-change`** (new, memory-api): records one-shot
  consent to change the memory embedding vector dimension. Body
  `{"target_dim": <int 1..2000>}`. Returns `200 {"status":"consent recorded","target_dim":N}`.
  The embedding columns are now application-managed (sized to the configured
  embedding provider's `Dimensions()` by a startup reconciler), so a dimension
  change that would discard existing embeddings requires this conscious, single-use
  consent marker — consumed atomically by the reconciler on the next reshape.
  `400` on a missing/out-of-range `target_dim`; `503` when the recorder is unwired.

### Added (session-api: provider usage tracking + per-CRD attribution, #1301)

- **`POST /api/v1/provider-usage`** (new): records workspace-scoped, session-less
  provider spend (embeddings, judge tokens, …) keyed by namespace. Body is a JSON
  array of `ProviderUsage` objects (namespace/provider/source required). Written by
  memory-api (embeddings) and reserved for other infrastructure producers.
- **`ProviderCall`** gains `namespace`, `agentName`, and `providerName` — the first
  two denormalized from the session so cost/usage aggregates filter without a JOIN;
  `providerName` carries the Provider CRD identity (vs `provider`, which is the type)
  so two same-type providers are attributed separately.
- **`GET /api/v1/provider-calls/aggregate`** adds a `provider_name` `groupBy`
  dimension + a `providerName` filter; **`GET /api/v1/provider-calls/discover`**
  response gains `providerNames`. Backward compatible.

### Removed (session-api: dead judge_tokens/judge_cost_usd on EvalResult, #1301)

`EvalResult.judgeTokens` and `EvalResult.judgeCostUsd` are removed — they were
never populated. Judge LLM token usage is now recorded as a normal provider call
in `provider_calls` with `source="judge"` (inline via the runtime event store, and
in the arena eval-worker via an attached event bus), so the spend lives with the
provider call rather than duplicated on the eval verdict.

### Changed (session-api: compound groupBy on provider-calls aggregate, #1222)

`GET /api/v1/provider-calls/aggregate` now accepts a **comma-separated**
`groupBy` (e.g. `groupBy=provider,model,agent` or `groupBy=time:hour,provider`)
in addition to a single dimension. Each dimension becomes one segment of a
composite key, joined with `|` (e.g. `"2026-06-09T13:00:00Z|openai"`). When any
`time:*` dimension is present, rows sort chronologically by key; otherwise by
value descending. Single-dimension requests are unchanged — fully backward
compatible. This powers the dashboard's exact cost/token totals (read from
session-api product tables instead of Prometheus `increase([24h])`).

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
